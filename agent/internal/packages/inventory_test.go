package packages

import "testing"

func TestInventoryHashFixtures(t *testing.T) {
	row := Package{
		Manager: "dpkg", Name: "nginx-core", Version: "1.24.0-2ubuntu7.19", Arch: "amd64",
		SourceName: "nginx", SourceVersion: "1.24.0-2ubuntu7.19",
	}
	cases := []struct {
		name string
		rows []Package
		hash string
	}{
		{"snapshot", []Package{row}, "sha256:8f2100f2d82aed50b320cd52f4a413af2c1bfd42cb09120b7413ad4e1b865efc"},
		{"empty", nil, "sha256:1788f993b4772e3516f5409992386cee14668f98ca23773646f1670a8ffaf8d9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Hash(tc.rows); got != tc.hash {
				t.Fatalf("expected hash %s, got %s", tc.hash, got)
			}
		})
	}
}
