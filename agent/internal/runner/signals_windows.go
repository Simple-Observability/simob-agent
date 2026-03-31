//go:build windows
// +build windows

package runner

import (
	"os"
	"os/exec"
)

// configureRunCommand does not apply extra process attributes on Windows.
func configureRunCommand(command *exec.Cmd) {
	_ = command
}

// runCommandSignals returns the Windows interrupt signal the runner handles.
func runCommandSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

// forwardRunSignal sends the received signal directly to the child process on Windows.
func forwardRunSignal(command *exec.Cmd, sig os.Signal) error {
	if command.Process == nil {
		return nil
	}
	return command.Process.Signal(sig)
}
