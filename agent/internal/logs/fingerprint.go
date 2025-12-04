//go:build !windows
// +build !windows

package logs

import (
	"os"
	"syscall"
)

// getFileFingerprint extracts inode and size for file identification on Unix-like systems
func getFileFingerprint(path string) (FileFingerprint, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return FileFingerprint{}, err
	}
	sys := stat.Sys().(*syscall.Stat_t)
	return FileFingerprint{Inode: sys.Ino, Size: stat.Size()}, nil
}
