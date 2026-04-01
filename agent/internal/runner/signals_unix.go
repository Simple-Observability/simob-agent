//go:build !windows
// +build !windows

package runner

import (
	"os"
	"os/exec"
	"syscall"
)

// configureRunCommand starts the child in its own process group on Unix so
// forwarded signals can be delivered to the full subprocess tree.
func configureRunCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// runCommandSignals returns the Unix termination signals the runner forwards.
func runCommandSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}

// forwardRunSignal sends the received signal to the child's process group.
func forwardRunSignal(command *exec.Cmd, sig os.Signal) error {
	if command.Process == nil {
		return nil
	}
	signalValue, ok := sig.(syscall.Signal)
	if !ok {
		return nil
	}
	return syscall.Kill(-command.Process.Pid, signalValue)
}
