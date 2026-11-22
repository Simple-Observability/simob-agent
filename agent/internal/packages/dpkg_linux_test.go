package packages

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestParseDPKGInstalledRows(t *testing.T) {
	multiarch := "libnginx\t1:1.24.0-2+b1\tarm64\tinstalled\tnginx\t1:1.24.0-2\n"
	rows, err := parseDPKG([]byte(multiarch +
		"old-nginx\t1.0\tamd64\tconfig-files\t\t\n" +
		"libnginx\t1:1.24.0-2+b1\tamd64\tinstalled\tnginx\t1:1.24.0-2\n" +
		multiarch +
		"nginx\t1.24.0-2~deb12u1\tall\tinstalled\t\t\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Arch != "amd64" || rows[1].Arch != "arm64" {
		t.Fatalf("expected sorted distinct multiarch rows, got %+v", rows)
	}
	if rows[0].Version != "1:1.24.0-2+b1" || rows[0].SourceVersion != "1:1.24.0-2" {
		t.Fatalf("binary and source versions differ: %+v", rows[0])
	}
	last := rows[2]
	if last.SourceName != last.Name || last.SourceVersion != last.Version {
		t.Fatalf("expected source fallbacks, got %+v", last)
	}
}

func TestParseDPKGRejectsInvalidInventory(t *testing.T) {
	for _, input := range []string{
		"nginx\t1.24\tamd64\n",
		"nginx\t1.24\tamd64\t\tnx\t1.24\n",
		"nginx\t1.x:2\tamd64\tinstalled\t\t\n",
		"nginx\t1.24\tamd64\tinstalled\t\t",
		"nginx\t1.24\tamd64\tinstalled\tnginx\t1.24\n" +
			"nginx\t1.24\tamd64\tinstalled\tnginx\t1.23\n",
		strings.Repeat("x", 4096) + "\t1.24\tamd64\tinstalled\t\t\n",
	} {
		if rows, err := parseDPKG([]byte(input)); err == nil || rows != nil {
			t.Fatalf("expected a failed collection, got %d rows and error %v", len(rows), err)
		}
	}
}

func TestCollectUnavailableDPKG(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if rows, err := Collect(context.Background()); !errors.Is(err, ErrUnsupported) || rows != nil {
		t.Fatalf("expected unsupported collection, got %d rows and error %v", len(rows), err)
	}
}

func TestCollectHostDPKG(t *testing.T) {
	if _, err := exec.LookPath("dpkg-query"); err != nil {
		t.Skip("dpkg-query unavailable")
	}
	rows, err := Collect(context.Background())
	if err != nil || len(rows) == 0 {
		t.Fatalf("expected installed host packages, got %d rows and error %v", len(rows), err)
	}
	t.Logf("collected %d installed host packages", len(rows))
}
