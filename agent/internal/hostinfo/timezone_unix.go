//go:build linux || darwin
// +build linux darwin

package hostinfo

import (
	"os"
	"path/filepath"
	"strings"
)

// resolveTimezone returns the host IANA timezone name (e.g. "America/New_York"),
// or "" if it cannot be determined.
func resolveTimezone() string {
	// /etc/timezone (Debian/Ubuntu) holds the IANA name directly.
	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		if name := strings.TrimSpace(string(data)); name != "" {
			return name
		}
	}

	// /etc/localtime is a symlink into the zoneinfo tree (e.g.
	// /usr/share/zoneinfo/<NAME>).
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if name := extractZoneName(link); name != "" {
			return name
		}
	}

	return ""
}

// extractZoneName extracts the zone identifier (e.g. "America/New_York")
// from a zoneinfo path, or "" if none is found.
func extractZoneName(path string) string {
	path = filepath.ToSlash(path)
	for _, marker := range []string{"/zoneinfo/", "/timezone/zoneinfo/"} {
		if idx := strings.Index(path, marker); idx >= 0 {
			return strings.Trim(filepath.ToSlash(path[idx+len(marker):]), "/")
		}
	}
	return ""
}
