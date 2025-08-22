package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/hpcloud/tail"

	"agent/internal/common"
	"agent/internal/logger"
)

// Some improvements to consider
// 1. Stale entries after rotations never cleaned up
// 2. Mid-read rotations
// 3. out channel bottleneck

// TailRunner handles tailing multiple files matching a glob pattern.
type TailRunner struct {
	// pattern is the glob pattern used to match files to tail
	pattern string

	// out is the channel where the log entries are sent for processing
	out chan<- LogEntry

	// processor is the function used to transform raw log lines
	// into structured LogEntries.
	processor Processor

	// tailers stores the tailers for the matched files
	tailers []*tail.Tail

	// wg is used to wait for all tailers to complete
	wg sync.WaitGroup

	positions map[string]PositionEntry

	// positionsFilePath stores the path where the positions are saved on disk
	positionsFilePath string

	positionMutex sync.Mutex
}

// NewTailRunner creates and configures a new TailRunner.
func NewTailRunner(pattern string, processor Processor) (*TailRunner, error) {
	// Check that all files can be opened
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob pattern: %w", err)
	}
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("cannot read log file %s: %w", file, err)
		}
		f.Close()
	}

	// Load existing positions
	programDirectory, err := common.GetProgramDirectory()
	if err != nil {
		logger.Log.Error("can't get program directory", "error", err)
		return nil, err
	}
	positionPath := filepath.Join(programDirectory, "positions.json")
	positions, err := loadPositions(positionPath)
	if err != nil {
		logger.Log.Error("can't load positions file. reverting to empty map", "error", err)
	}

	return &TailRunner{
		pattern:           pattern,
		positions:         positions,
		positionsFilePath: positionPath,
	}, nil
}

func (r *TailRunner) Start(ctx context.Context, out chan<- LogEntry) error {
	r.out = out

	files, err := filepath.Glob(r.pattern)
	if err != nil {
		return fmt.Errorf("glob failed: %w", err)
	}

	// Start periodic position saving
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.savePositions()
			}
		}
	}()

	for _, file := range files {
		// Determine starting positions before tailing (warm start)
		var loc *tail.SeekInfo
		posEntry, found := matchByFingerprint(r.positions, file)
		if found {
			// Resume from saved position
			loc = &tail.SeekInfo{Offset: posEntry.Position.Offset, Whence: 0}
		} else {
			// Start from start for new files
			loc = &tail.SeekInfo{Offset: 0, Whence: 0}
		}

		tailConfig := tail.Config{
			Follow:   true, // Keep looking for new lines
			ReOpen:   true, // Reopen files when they get rotated
			Poll:     true, // Poll for file changes instead of using inotify
			Location: loc,  // Set starting position
		}

		// Tail file
		t, err := tail.TailFile(file, tailConfig)
		if err != nil {
			return fmt.Errorf("failed to tail %s %w", file, err)
		}

		// Save tailers
		r.tailers = append(r.tailers, t)

		r.wg.Add(1)
		go func(t *tail.Tail, processor Processor) {
			defer r.wg.Done()
			for {
				select {
				case <-ctx.Done():
					logger.Log.Debug("Stopping tailer", "filename", t.Filename)
					return
				case line := <-t.Lines:
					if line == nil {
						continue
					}

					// Process log entry and send it to out channel
					processedLog, _ := processor(line.Text)
					out <- processedLog

					// Update position after processing line
					if offset, err := t.Tell(); err == nil {
						r.updatePosition(file, offset)
					}
				}
			}
		}(t, r.processor)

	}
	return nil
}

func (r *TailRunner) Stop() error {
	for _, t := range r.tailers {
		t.Cleanup()
	}
	r.wg.Wait()
	r.savePositions()
	return nil
}

// updatePosition updates the position for a specific file
func (r *TailRunner) updatePosition(file string, offset int64) {
	fp, err := getFileFingerprint(file)
	if err != nil {
		logger.Log.Error("couldn't update position because of file fingerprint error", "error", err)
		return
	}
	r.positionMutex.Lock()
	r.positions[file] = PositionEntry{
		Path:        file,
		Fingerprint: fp,
		Position:    Position{Offset: offset},
	}
	r.positionMutex.Unlock()
}

// savePositions saves current positions to file
func (r *TailRunner) savePositions() {
	r.positionMutex.Lock()
	defer r.positionMutex.Unlock()
	err := savePositions(r.positionsFilePath, r.positions)
	if err != nil {
		logger.Log.Error("couldn't save positions to disk", "error", err)
	}
}

// ------------------------ Fingerprint ------------------------

// getFileFingerprint extracts inode and size for file identification
func getFileFingerprint(path string) (FileFingerprint, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return FileFingerprint{}, err
	}
	sys := stat.Sys().(*syscall.Stat_t)
	return FileFingerprint{Inode: sys.Ino, Size: stat.Size()}, nil
}

// matchByFingerprint finds position entry by matching fingerprint
func matchByFingerprint(positions map[string]PositionEntry, path string) (PositionEntry, bool) {
	currentFp, err := getFileFingerprint(path)
	if err != nil {
		return PositionEntry{}, false
	}
	// First try exact path match
	if entry, exists := positions[path]; exists {
		if entry.Fingerprint.Inode == currentFp.Inode && entry.Fingerprint.Size <= currentFp.Size {
			return entry, true
		}
	}
	// Then try fingerprint match across all entries (handles renamed files)
	for _, entry := range positions {
		if entry.Fingerprint.Inode == currentFp.Inode && entry.Fingerprint.Size <= currentFp.Size {
			return entry, true
		}
	}
	return PositionEntry{}, false
}

// ------------------------ Positions ------------------------

// PositionEntry stores complete position information for a file
type PositionEntry struct {
	Path        string          `json:"path"`
	Fingerprint FileFingerprint `json:"fingerprint"`
	Position    Position        `json:"position"`
}

// FileFingerprint represents unique file identity using inode and size
type FileFingerprint struct {
	Inode uint64 `json:"inode"`
	Size  int64  `json:"size"`
}

// Position represents the current read position in a file
type Position struct {
	Offset int64 `json:"offset"`
}

// PositionState holds all position entries for persistence
type PositionState struct {
	Positions []PositionEntry `json:"positions"`
}

// loadPositions loads position state from a JSON file
func loadPositions(path string) (map[string]PositionEntry, error) {
	positions := make(map[string]PositionEntry)

	data, err := os.ReadFile(path)
	if err != nil {
		// If file doesn't exist, return an empty map
		if os.IsNotExist(err) {
			logger.Log.Debug("Positions file not found. Starting with an empty map", "path", path)
			return positions, nil
		}
		return nil, err
	}

	var state PositionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	// NOTE: Could be removed if state and tailrunner.positions are merged
	for _, entry := range state.Positions {
		positions[entry.Path] = entry
	}

	return positions, nil
}

func savePositions(path string, positions map[string]PositionEntry) error {
	logger.Log.Debug("Saving positions to disk", "path", path)

	// NOTE: Could be removed if state and tailrunner.positions are merged
	state := PositionState{
		Positions: make([]PositionEntry, 0, len(positions)),
	}
	for _, entry := range positions {
		state.Positions = append(state.Positions, entry)
	}

	data, err := json.Marshal(&state)
	if err != nil {
		return err
	}

	// Write to temporary file first, then rename for atomic operation
	tempFile := path + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tempFile, path)
}
