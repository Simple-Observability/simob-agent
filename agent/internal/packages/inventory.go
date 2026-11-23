package packages

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

const (
	Snapshot    = "snapshot"
	Unchanged   = "unchanged"
	Unsupported = "unsupported"
)

var ErrUnsupported = errors.New("package inventory collector unavailable")

type Package struct {
	Manager       string `json:"manager"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Arch          string `json:"arch"`
	SourceName    string `json:"source_name"`
	SourceVersion string `json:"source_version"`
}

type Observation struct {
	SchemaVersion int       `json:"schema_version"`
	Status        string    `json:"status"`
	CollectedAt   time.Time `json:"collected_at"`
	InventoryHash string    `json:"inventory_hash,omitempty"`
}

func (p Package) tuple() [6]string {
	return [6]string{p.Manager, p.Name, p.Version, p.Arch, p.SourceName, p.SourceVersion}
}

func canonicalize(inventory []Package) ([]Package, error) {
	slices.SortFunc(inventory, func(a, b Package) int {
		right := b.tuple()
		for i, field := range a.tuple() {
			if cmp := strings.Compare(field, right[i]); cmp != 0 {
				return cmp
			}
		}
		return 0
	})
	inventory = slices.Compact(inventory)
	for i := 1; i < len(inventory); i++ {
		prev, curr := inventory[i-1].tuple(), inventory[i].tuple()
		if slices.Equal(prev[:4], curr[:4]) {
			return nil, errors.New("conflicting source fields for installed package")
		}
	}
	return inventory, nil
}

// Hash hashes the canonical schema-1 inventory returned by Collect.
func Hash(inventory []Package) string {
	digest := sha256.New()
	digest.Write([]byte("[1,["))
	for i, p := range inventory {
		if i > 0 {
			digest.Write([]byte(","))
		}
		tuple, _ := json.Marshal(p.tuple())
		digest.Write(tuple)
	}
	digest.Write([]byte("]]"))
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}
