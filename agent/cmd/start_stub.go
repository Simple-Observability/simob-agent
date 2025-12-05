//go:build !windows
// +build !windows

package cmd

func isWindowsService() bool {
	return false
}

func runAsWindowsService() {
	// This should never be called on non-Windows platforms
}
