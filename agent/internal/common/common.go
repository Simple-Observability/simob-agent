package common

import (
	"os"
	"path/filepath"
)

func GetProgramDirectory() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
}
