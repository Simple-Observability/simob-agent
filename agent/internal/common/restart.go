package common

import (
	"os"
	"path/filepath"
	"time"

	"agent/internal/logger"
)

// RestartSignal returns a channel that notifies when the agent should restart.
//
// This is a file-based signaling mechanism used instead of relying on OS signals
// (SIGINT/SIGTERM) because signals can only be sent by the process owner or root.
// Using a restart file allows any user in the simob-admins group to request a graceful
// agent restart without needing elevated privileges.
//
// On agent startup, any stale restart file is deleted to avoid accidental triggers.
// The returned channel will emit 'true' when a new restart signal is detected.func RestartSignal(stop <-chan struct{}) <-chan bool {
func RestartSignal(stop <-chan struct{}) <-chan bool {
	deleteRestartSignalIfExists()
	out := make(chan bool, 1)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		defer close(out)

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				logger.Log.Debug("Checking for restart signal")
				if restartRequested() {
					out <- true
					return
				}
			}
		}
	}()
	return out
}

// restartRequested checks if a restart has been requested.
func restartRequested() bool {
	programDir, err := GetProgramDirectory()
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
	programDir, err := GetProgramDirectory()
	if err != nil {
		return
	}
	restartFile := filepath.Join(programDir, "restart")
	if _, err := os.Stat(restartFile); err == nil {
		logger.Log.Info("Deleting stale restart file", "file", restartFile)
		_ = os.Remove(restartFile)
	}
}
