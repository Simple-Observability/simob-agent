//go:build !windows

package bootstrap

import "syscall"

func SetUmask() {
	syscall.Umask(0o007)
}
