package updater

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTargetVersionIsNewer(t *testing.T) {
	tests := []struct {
		current  string
		target   string
		expected bool
	}{
		{"1.0.0", "1.1.0", true},
		{"1.0.0", "1.0.1", true},
		{"2.0.0", "1.1.0", false},
		{"1.1.0", "1.1.0", false},
		{"dev", "0.0.1", true},
		{"1.0.0", "2.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.1", "1.0.10", true},
		{"invalid", "1.0.0", false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_vs_%s", tt.current, tt.target), func(t *testing.T) {
			assert.Equal(t, tt.expected, targetVersionIsNewer(tt.current, tt.target))
		})
	}
}

func TestVerifySHA256(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-binary")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := []byte("hello world")
	_, err = tmpFile.Write(content)
	require.NoError(t, err)
	tmpFile.Close()

	expected := fmt.Sprintf("%x", sha256.Sum256(content))
	ok, err := verifySHA256(tmpFile.Name(), expected)
	assert.NoError(t, err)
	assert.True(t, ok)

	ok, err = verifySHA256(tmpFile.Name(), "wrong")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestCheckForUpdate_Logic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/updates/" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(fmt.Sprintf(`{"version": "1.1.0", "checksum": "abc", "url": "%s"}`, "http://"+r.Host)))
		} else if r.URL.Path == "/checksums" {
			w.Write([]byte(fmt.Sprintf("mock-checksum %s\n", binaryName())))
		}
	}))
	defer server.Close()

	originalRemoteUrl := remoteApiUrl
	remoteApiUrl = server.URL
	defer func() { remoteApiUrl = originalRemoteUrl }()

	info, err := checkForUpdate()
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", info.Version)
	assert.Equal(t, "mock-checksum", info.Checksum)
}

func TestDownloadBinary_Logic(t *testing.T) {
	content := []byte("binary data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	tmpDir, err := os.MkdirTemp("", "download-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dest := filepath.Join(tmpDir, "simob")
	err = downloadBinary(server.URL, dest)
	require.NoError(t, err)

	downloaded, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, content, downloaded)

	// Check permissions
	info, err := os.Stat(dest)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestApplyUpdate_Logic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "apply-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	oldPath := filepath.Join(tmpDir, "simob")
	newPath := filepath.Join(tmpDir, "simob.new")

	err = os.WriteFile(oldPath, []byte("old"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(newPath, []byte("new"), 0755)
	require.NoError(t, err)

	err = applyUpdate(newPath, oldPath)
	require.NoError(t, err)

	content, err := os.ReadFile(oldPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("new"), content)

	_, err = os.Stat(newPath)
	assert.True(t, os.IsNotExist(err), "new file should be gone")
}

func TestCreateRestartSignal_Logic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "restart-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	execPath := filepath.Join(tmpDir, "simob")
	err = createRestartSignal(execPath)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(tmpDir, "restart"))
	assert.NoError(t, err, "restart signal file should exist")
}
