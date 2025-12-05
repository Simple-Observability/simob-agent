//go:build windows
// +build windows

package logs

import (
	"os"
	"syscall"
)

// getFileFingerprint extracts file index and size for file identification on Windows
func getFileFingerprint(path string) (FileFingerprint, error) {
	// Get file handle
	utf16Path, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return FileFingerprint{}, err
	}
	handle, err := syscall.CreateFile(
		utf16Path,
		0,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return FileFingerprint{}, err
	}
	defer syscall.CloseHandle(handle)

	// Fetch file index
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &info); err != nil {
		return FileFingerprint{}, err
	}
	fileIndex := uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)

	// Fetch file size
	stat, err := os.Stat(path)
	if err != nil {
		return FileFingerprint{}, err
	}

	return FileFingerprint{
		Inode: fileIndex,
		Size:  stat.Size(),
	}, nil
}
