//go:build windows
// +build windows

package cmd

import (
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc"

	"agent/internal/logger"
	"agent/internal/manager"
)

const serviceName = "simob"

type windowsService struct {
	agent  *manager.Agent
	doneCh chan struct{}
}

func isWindowsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}

func runAsWindowsService() {
	err := svc.Run(serviceName, &windowsService{})
	if err != nil {
		return
	}
}

func (ws *windowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}

	// Start the agent in a separate goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- ws.startAgent()
	}()

	// Check succeeded
	select {
	case err := <-errChan:
		if err != nil {
			// Failed to start
			changes <- svc.Status{State: svc.Stopped}
			return true, 1
		}
	case <-time.After(30 * time.Second):
		// Timeout
		changes <- svc.Status{State: svc.Stopped}
		return true, 1
	}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	// Main service loop
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			ws.stopAgent()
			changes <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}

	// If we exit the loop (channel closed), stop the service
	changes <- svc.Status{State: svc.Stopped}
	return false, 0
}

func (ws *windowsService) startAgent() error {
	agent, err := initializeAndLoadAgent(false)
	if err != nil {
		return err
	}

	ws.agent = agent
	ws.doneCh = make(chan struct{})

	// Run the agent in a goroutine
	go func() {
		ws.agent.Run(false)
		close(ws.doneCh)
	}()

	return nil
}

func (ws *windowsService) stopAgent() {
	if ws.agent != nil {
		logger.Log.Info("Stopping agent...")

		// Send SIGTERM to trigger graceful shutdown
		p, err := os.FindProcess(os.Getpid())
		if err == nil {
			p.Signal(syscall.SIGTERM)
		}

		// Wait for agent to finish with timeout
		select {
		case <-ws.doneCh:
			logger.Log.Info("Agent stopped gracefully")
		case <-time.After(30 * time.Second):
			logger.Log.Warn("Agent shutdown timed out after 30 seconds")
		}
	}
}
