//go:build windows

package winevent

import (
	"context"
	"encoding/xml"
	"fmt"
	"sync"
	"syscall"
	"time"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/logs"

	"github.com/google/winops/winlog"
	"github.com/google/winops/winlog/wevtapi"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var once sync.Once

type WinEventCollector struct {
	name   string
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewWinEventCollector() *WinEventCollector {
	return &WinEventCollector{
		name: "winevent",
	}
}

func (c *WinEventCollector) Name() string {
	return c.name
}

func (c *WinEventCollector) Discover() []collection.LogSource {
	// Ensure we have the necessary privileges to discover Security logs.
	ensureSeSecurityPrivilege()

	config, subscription, err := c.subscribe("Security")
	if err != nil {
		return []collection.LogSource{}
	}
	winlog.Close(subscription)
	winlog.Close(config.SignalEvent)

	return []collection.LogSource{
		{
			Name: c.name,
			Path: "Security",
		},
	}
}

func (c *WinEventCollector) Start(ctx context.Context, out chan<- logs.LogEntry) error {
	// Ensure we have the necessary privileges before starting collection.
	ensureSeSecurityPrivilege()

	// Create a child context for cancellation
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.wg.Add(1)
	go c.run(ctx, out)

	return nil
}

func (c *WinEventCollector) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	return nil
}

// ensureSeSecurityPrivilege ensures that the SeSecurityPrivilege is enabled.
// This is only called once per process life.
func ensureSeSecurityPrivilege() {
	once.Do(func() {
		if err := enableSeSecurityPrivilege(); err != nil {
			logger.Log.Error("failed to enable SeSecurityPrivilege", "error", err)
		}
	})
}

// enableSeSecurityPrivilege enables the SeSecurityPrivilege in the process token.
// This is required to read the "Security" event log channel.
func enableSeSecurityPrivilege() error {
	var token windows.Token
	// Open the process token for the current running binary
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token)
	if err != nil {
		return fmt.Errorf("OpenProcessToken failed: %w", err)
	}
	defer token.Close()

	// Look up the LUID for the SeSecurityPrivilege
	var luid windows.LUID
	err = windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeSecurityPrivilege"), &luid)
	if err != nil {
		return fmt.Errorf("LookupPrivilegeValue failed: %w", err)
	}

	// Enable the privilege
	tp := windows.Tokenprivileges{
		PrivilegeCount: 1,
		Privileges: [1]windows.LUIDAndAttributes{
			{
				Luid:       luid,
				Attributes: windows.SE_PRIVILEGE_ENABLED,
			},
		},
	}
	err = windows.AdjustTokenPrivileges(token, false, &tp, 0, nil, nil)
	if err != nil {
		return fmt.Errorf("AdjustTokenPrivileges failed: %w", err)
	}

	return nil
}

// Registry path for storing bookmarks
const (
	registryPath     = `SOFTWARE\SimpleObservability`
	bookmarkValueFmt = "WinEventBookmark_%s"
)

// ensureRegistryKey creates the registry key if it doesn't exist
func ensureRegistryKey(root registry.Key, path string) error {
	k, _, err := registry.CreateKey(root, path, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("registry.CreateKey failed: %w", err)
	}
	k.Close()
	return nil
}

func (c *WinEventCollector) subscribe(channel string) (*winlog.SubscribeConfig, windows.Handle, error) {
	// Initialize a subscription with defaults.
	config, err := winlog.DefaultSubscribeConfig()
	if err != nil {
		return nil, 0, fmt.Errorf("winlog.DefaultSubscribeConfig failed: %w", err)
	}

	// Override the Query to only include the requested channel
	xpaths := map[string]string{channel: "*"}
	xmlQuery, err := winlog.BuildStructuredXMLQuery(xpaths)
	if err != nil {
		winlog.Close(config.SignalEvent)
		return nil, 0, fmt.Errorf("BuildStructuredXMLQuery failed: %w", err)
	}
	config.Query, err = syscall.UTF16PtrFromString(string(xmlQuery))
	if err != nil {
		winlog.Close(config.SignalEvent)
		return nil, 0, fmt.Errorf("syscall.UTF16PtrFromString failed: %w", err)
	}

	// Load bookmark from registry for resuming after restart
	bookmarkValue := fmt.Sprintf(bookmarkValueFmt, channel)
	if err := ensureRegistryKey(registry.LOCAL_MACHINE, registryPath); err != nil {
		logger.Log.Warn("failed to create registry key, starting from oldest", "error", err)
		config.Flags = wevtapi.EvtSubscribeStartAtOldestRecord
	} else if err := winlog.GetBookmarkRegistry(config, registry.LOCAL_MACHINE, registryPath, bookmarkValue); err != nil {
		logger.Log.Debug("no bookmark found, starting from oldest record", "channel", channel)
		config.Flags = wevtapi.EvtSubscribeStartAtOldestRecord
	} else {
		config.Flags = wevtapi.EvtSubscribeStartAfterBookmark
		logger.Log.Debug("loaded bookmark from registry, resuming from last position", "channel", channel)
	}

	subscription, err := winlog.Subscribe(config)
	if err != nil {
		winlog.Close(config.SignalEvent)
		return nil, 0, fmt.Errorf("winlog.Subscribe failed: %w", err)
	}

	return config, subscription, nil
}

func (c *WinEventCollector) run(ctx context.Context, out chan<- logs.LogEntry) {
	defer c.wg.Done()

	config, subscription, err := c.subscribe("Security")
	if err != nil {
		logger.Log.Error("failed to subscribe to Windows events", "error", err)
		return
	}
	defer winlog.Close(subscription)
	defer winlog.Close(config.SignalEvent)

	publisherCache := make(map[string]windows.Handle)
	defer func() {
		for _, h := range publisherCache {
			winlog.Close(h)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Continue
		}

		// Wait for events that match the query. Timeout in milliseconds.
		status, err := windows.WaitForSingleObject(config.SignalEvent, 1000)
		if err != nil {
			logger.Log.Error("windows.WaitForSingleObject failed", "error", err)
			// If the handle is invalid, it's a non-transient error. Exit.
			if err == windows.ERROR_INVALID_HANDLE {
				return
			}
			// For other errors, sleep longer before retrying to reduce noise
			time.Sleep(1 * time.Minute)
			continue
		}

		// Check for cancellation again after wait
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Get a block of events once signaled.
		if status == syscall.WAIT_OBJECT_0 {
			// Enumerate and render available events in blocks of up to 100.
			renderedEvents, err := winlog.GetRenderedEvents(config, publisherCache, subscription, 100, 1033)

			// If no more events are available reset the subscription signal.
			if err == syscall.Errno(259) { // ERROR_NO_MORE_ITEMS
				windows.ResetEvent(config.SignalEvent)
			} else if err != nil {
				logger.Log.Error("winlog.GetRenderedEvents failed", "error", err)
				// If subscription is broken or system call fails, exit loop.
				return
			}

			// Process events.
			for _, eventXML := range renderedEvents {
				var ev WinEventLog
				if err := xml.Unmarshal([]byte(eventXML), &ev); err != nil {
					logger.Log.Error("unmarshal error", "error", err)
					continue
				}

				entry := ev.ToLogEntry()

				select {
				case out <- entry:
				case <-ctx.Done():
					return
				}
			}

			// Persist bookmark after processing batch
			if config.Bookmark != 0 {
				bookmarkValue := fmt.Sprintf(bookmarkValueFmt, "Security")
				if err := winlog.SetBookmarkRegistry(config.Bookmark, registry.LOCAL_MACHINE, registryPath, bookmarkValue); err != nil {
					logger.Log.Error("failed to save bookmark to registry", "error", err)
				}
			}
		}
	}
}
