//go:build windows
// +build windows

package hostinfo

// resolveTimezone returns the host IANA timezone name, or "" if it cannot be
// determined. Windows stores non-IANA tz IDs, we fall back to ""
func resolveTimezone() string {
	return ""
}
