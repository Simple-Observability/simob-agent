package packages

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const dpkgFormat = `${Package}\t${Version}\t${Architecture}\t${db:Status-Status}\t${source:Package}\t${source:Version}\n`

var (
	packageName  = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]+$`)
	architecture = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	epoch        = regexp.MustCompile(`^[0-9]+$`)
	version      = regexp.MustCompile(`^[0-9][A-Za-z0-9+.~:-]*$`)
	revision     = regexp.MustCompile(`^[A-Za-z0-9+.~]+$`)
)

func Collect(ctx context.Context) ([]Package, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "dpkg-query", "--no-pager", "-W", "-f="+dpkgFormat)
	if errors.Is(cmd.Err, exec.ErrNotFound) {
		return nil, ErrUnsupported
	}
	cmd.Env = append(cmd.Environ(), "LC_ALL=C")
	cmd.WaitDelay = time.Second
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, fmt.Errorf("dpkg-query failed: %w", err)
	}
	return parseDPKG(output)
}

func parseDPKG(output []byte) ([]Package, error) {
	if len(output) > 0 && output[len(output)-1] != '\n' {
		return nil, errors.New("incomplete dpkg output")
	}

	var inventory []Package
	rows := bufio.NewScanner(bytes.NewReader(output))
	rows.Buffer(make([]byte, 4096), 4096)
	for line := 1; rows.Scan(); line++ {
		fields := strings.Split(rows.Text(), "\t")
		if len(fields) < 4 {
			return nil, fmt.Errorf("missing dpkg status on row %d", line)
		}
		switch fields[3] {
		case "installed":
		case "not-installed", "config-files", "half-installed", "unpacked", "half-configured",
			"triggers-awaited", "triggers-pending":
			continue
		default:
			return nil, fmt.Errorf("invalid dpkg status on row %d", line)
		}
		if len(fields) != 6 {
			return nil, fmt.Errorf("invalid installed package on row %d", line)
		}

		p := Package{
			Manager: "dpkg", Name: fields[0], Version: fields[1], Arch: fields[2],
			SourceName: fields[4], SourceVersion: fields[5],
		}
		if p.SourceName == "" {
			p.SourceName = p.Name
		}
		if p.SourceVersion == "" {
			p.SourceVersion = p.Version
		}
		if !validPackage(p) {
			return nil, fmt.Errorf("invalid installed package on row %d", line)
		}
		inventory = append(inventory, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read dpkg output: %w", err)
	}
	return canonicalize(inventory)
}

func validPackage(p Package) bool {
	return len(p.Name) <= 255 && packageName.MatchString(p.Name) &&
		len(p.SourceName) <= 255 && packageName.MatchString(p.SourceName) &&
		len(p.Arch) <= 64 && architecture.MatchString(p.Arch) &&
		validVersion(p.Version) && validVersion(p.SourceVersion)
}

func validVersion(value string) bool {
	if len(value) > 255 || !version.MatchString(value) {
		return false
	}
	if prefix, rest, hasEpoch := strings.Cut(value, ":"); hasEpoch &&
		(!epoch.MatchString(prefix) || !version.MatchString(rest)) {
		return false
	}
	if dash := strings.LastIndexByte(value, '-'); dash >= 0 {
		return revision.MatchString(value[dash+1:])
	}
	return true
}
