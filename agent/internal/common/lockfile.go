package common

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"agent/internal/logger"
)

const PIDFilename = "pid"

// ErrAlreadyRunning is the error returned when the agent is already running.
var ErrAlreadyRunning = errors.New("agent already running")

// AcquireLock ensures only one agent instance runs at a time.
func AcquireLock() error {
	programDirectory, err := GetProgramDirectory()
	if err != nil {
		return fmt.Errorf("can't acquire lock. failed to get program directory: %w", err)
	}

	pidFilepath := filepath.Join(programDirectory, PIDFilename)
	currentPID := os.Getpid()
	file, err := os.OpenFile(pidFilepath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o660)

	// 'O_EXCL' will cause an error if file already exists
	if err != nil {
		logger.Log.Debug("Encountered an error while acquiring lock", "error", err)

		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("failed to create pid file: %w", err)
		}

		// File exists, check if the process is stale or still running.
		oldPID, err := readPID()
		if err != nil {
			// If we can't read the PID, we can't be sure, but it's likely a corrupt/stale lock.
			logger.Log.Debug("Failed to read existing PID file", "error", err)
			return overwritePIDFile(pidFilepath, currentPID)
		}

		if oldPID > 0 && isProcessRunning(oldPID) {
			logger.Log.Debug("Found process running", "PID", oldPID)
			return ErrAlreadyRunning
		}

		return overwritePIDFile(pidFilepath, currentPID)
	}

	// Successfully created the file, write the PID
	defer file.Close()

	_, err = file.WriteString((strconv.Itoa(currentPID)))
	return err
}

// ReleaseLock removes the PID file.
func ReleaseLock() error {
	programDirectory, err := GetProgramDirectory()
	if err != nil {
		return fmt.Errorf("can't release lock. failed to get program directory: %w", err)
	}
	err = os.Remove(filepath.Join(programDirectory, PIDFilename))
	return err
}

// IsLockAcquired checks if a valid lock is currently held by another process.
// It returns true if the PID file exists and the process within it is running.
// It returns false if there is no lock file or the process is not running.
func IsLockAcquired() (bool, error) {
	programDirectory, err := GetProgramDirectory()
	if err != nil {
		return false, fmt.Errorf("can't check for lock. failed to get program directory: %w", err)
	}

	pidFilepath := filepath.Join(programDirectory, PIDFilename)

	// Check if the PID file exists.
	_, err = os.Stat(pidFilepath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// File does not exist, so no lock is acquired.
			return false, nil
		}
		// An unexpected error occurred while checking the file.
		return false, fmt.Errorf("failed to stat pid file: %w", err)
	}

	// File exists, now check if the process is running.
	oldPID, err := readPID()
	if err != nil {
		// If we can't read the PID, the lock file is likely corrupted.
		return false, nil
	}

	// Check if the process ID from the file is currently running.
	if oldPID > 0 && isProcessRunning(oldPID) {
		return true, nil
	}

	return false, nil
}

// readPID reads the integer PID from the lock file.
func readPID() (int, error) {
	programDirectory, err := GetProgramDirectory()
	if err != nil {
		return 0, fmt.Errorf("can't read PID. failed to get program directory: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(programDirectory, PIDFilename))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

// overwritePIDFile opens a file for writing, truncating it if it exists, and writes the new PID.
func overwritePIDFile(pidFilePath string, pid int) error {
	file, err := os.OpenFile(pidFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o660)
	if err != nil {
		return fmt.Errorf("failed to open stale pid file for writing: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString(strconv.Itoa(pid))
	if err != nil {
		return fmt.Errorf("failed to overwrite pid in stale lock file: %w", err)
	}
	return nil
}

// isProcessRunning checks if a process with the given PID is currently running.
func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// The process does not exist
	if os.IsNotExist(err) {
		return false
	}
	// The process exists, but we do not have permission to signal it.
	if os.IsPermission(err) {
		return true
	}
	return false
}
