//go:build windows

package bootstrap

func SetUmask() {
	// Windows does not support POSIX umask semantics.
}
