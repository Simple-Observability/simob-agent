package authguard

import (
	"agent/internal/logger"
	"sync"
	"time"
)

const (
	errorThreshold   = 10
	evaluationPeriod = 1 * time.Minute
	keyCheckSignal   = true
)

var (
	instance *AuthGuard
	once     sync.Once
)

// AuthGuard is responsible for monitoring API authentication errors
// and putting the agent in hibernation mode if the API key is revoked.
type AuthGuard struct {
	errorCount    int
	lastErrorTime time.Time
	mutex         sync.Mutex
	keyCheckCh    chan<- bool
}

// Get returns the singleton instance of the AuthGuard.
func Get() *AuthGuard {
	once.Do(func() {
		instance = &AuthGuard{}
	})
	return instance
}

// Subscribe sets the channel to be used for signaling a key check.
func (ag *AuthGuard) Subscribe(keyCheckCh chan<- bool) {
	ag.keyCheckCh = keyCheckCh
}

// HandleUnauthorized is called when a 401 or 403 status code is received.
// It increments an error counter and triggers an API key check if the threshold is reached.
func (ag *AuthGuard) HandleUnauthorized() {
	ag.mutex.Lock()
	defer ag.mutex.Unlock()

	// Reset counter if the last error was too long ago
	if time.Since(ag.lastErrorTime) > evaluationPeriod {
		ag.errorCount = 0
	}

	ag.errorCount++
	ag.lastErrorTime = time.Now()

	if ag.errorCount >= errorThreshold {
		logger.Log.Warn("authentication error threshold reached, sending a key check signal")
		if ag.keyCheckCh != nil {
			ag.keyCheckCh <- keyCheckSignal
		}
		// Reset counter after signaling for a check
		ag.errorCount = 0
	}
}
