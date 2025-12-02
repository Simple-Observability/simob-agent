package manager

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"agent/internal/common"
	"agent/internal/logger"
)

// RestartWatcher manages the background process of checking for a restart signal file.
//
// This is a file-based signaling mechanism used instead of relying on OS signals
// (SIGINT/SIGTERM) because signals can only be sent by the process owner or root.
// Using a restart file allows any user in the simob-admins group to request a graceful
// agent restart without needing elevated privileges.
//
// On agent startup, any stale restart file is deleted to avoid accidental triggers.
// The returned channel will emit 'true' when a new restart signal is detected.
type RestartWatcher struct {
	restartCh chan<- bool
}

// NewRestartWatcher creates a new instance of the RestartWatcher.
func NewRestartWatcher(restartCh chan<- bool) *RestartWatcher {
	return &RestartWatcher{
		restartCh: restartCh,
	}
}

// Start launches the background goroutine to watch for the restart signal file.
func (r *RestartWatcher) Start(ctx context.Context) {
	deleteRestartSignalIfExists()
	go r.run(ctx)
}

// run is the main loop for checking the restart signal.
func (r *RestartWatcher) run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	logger.Log.Info("Running restart watcher.")

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("Restart watcher received shutdown signal.")
			return
		case <-ticker.C:
			logger.Log.Debug("Checking for restart signal")
			if restartRequested() {
				logger.Log.Info("Restart signal detected. Triggering restart.")
				r.restartCh <- true
				return
			}
		}
	}
}

// restartRequested checks if a restart has been requested.
func restartRequested() bool {
	programDir, err := common.GetProgramDirectory()
	if err != nil {
		return false
	}
	restartFile := filepath.Join(programDir, "restart")
	if _, err := os.Stat(restartFile); err == nil {
		// Remove restart file and signal restart
		_ = os.Remove(restartFile)
		return true
	}
	return false
}

// deleteRestartSignalIfExists removes the restart file if it exists, ignoring any errors.
func deleteRestartSignalIfExists() {
	programDir, err := common.GetProgramDirectory()
	if err != nil {
		return
	}
	restartFile := filepath.Join(programDir, "restart")
	if _, err := os.Stat(restartFile); err == nil {
		logger.Log.Info("Deleting stale restart file", "file", restartFile)
		_ = os.Remove(restartFile)
	}
}
