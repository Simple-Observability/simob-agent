package updater

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"agent/internal/version"
)

// Some missing stuff:
// - Rollback mechanism: Keep the old binary as .old
// - Post install: Run some kind of post install health check

// tempSuffix is appended to the downloaded binary before it's installed
const tempSuffix = ".new"

// remoteApiUrl is the URL of the remote API that is called to get
// info about the latest updates.
const remoteApiUrl = "https://tapi.simpleobservability.com"

// UpdateInfo holds information about an available update.
type UpdateInfo struct {
	Version     string // The new version string, e.g., "1.1.0"
	DownloadURL string // The URL to download the new binary
	Checksum    string // The expected SHA256 checksum of the new binary
}

// Update orchestrates the update process
func Update() error {
	fmt.Println("Starting update process ...")
	fmt.Printf("Current simob version: %s\n", version.Version)

	// Check for updates
	updateInfo, err := checkForUpdate()
	if err != nil {
		return fmt.Errorf("error checking for updates: %v", err)
	}
	fmt.Printf("%+v\n", *updateInfo)

	if version.Version == updateInfo.Version {
		fmt.Println("Agent is already up to date.")
		return nil
	}
	fmt.Println("Upgrading to version:", updateInfo.Version)

	// Dynamically get current executable's path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}
	// Resolve path to get the actual binary path if a symlink is used
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks for executable path: %v", err)
	}

	// Define path for the downloaded new binary
	// Using a temporary directory for the download.
	newBinaryPath := filepath.Join(execPath + tempSuffix)
	fmt.Printf("New binary will be temporarily stored at: %s\n", newBinaryPath)

	// Ensure cleanup of the temporary file
	defer func() {
		if _, err := os.Stat(newBinaryPath); err == nil {
			fmt.Printf("Cleaning up temporary file: %s\n", newBinaryPath)
			os.Remove(newBinaryPath)
		}
	}()

	// Download the new binary
	fmt.Printf("Downloading update from %s...\n", updateInfo.DownloadURL)
	err = downloadBinary(updateInfo.DownloadURL, newBinaryPath)
	if err != nil {
		return fmt.Errorf("failed to download update: %v", err)
	}
	fmt.Println("Download complete.")

	// Verify the downloaded binary's integrity
	fmt.Println("Verifying checksum of the downloaded binary...")
	verified, err := verifySHA256(newBinaryPath, updateInfo.Checksum)
	if err != nil {
		return fmt.Errorf("error during checksum verification: %v", err)
	}
	if !verified {
		// It's crucial to abort if the checksum doesn't match.
		return fmt.Errorf("checksum verification FAILED. Expected: %s. The downloaded file may be corrupted or tampered with. Update aborted", updateInfo.Checksum)
	}
	fmt.Println("Checksum verified successfully.")

	// Apply the update (replace the old binary with the new one)
	fmt.Println("Applying update (replacing old binary)...")
	err = applyUpdate(newBinaryPath, execPath)
	if err != nil {
		return fmt.Errorf("failed to apply update: %v", err)
	}
	return nil
}

// checkForUpdate checks the remote API for updates.
func checkForUpdate() (*UpdateInfo, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(remoteApiUrl + "/updates/")
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from update server: %d", resp.StatusCode)
	}

	var apiResp struct {
		Version  string `json:"version"`
		Checksum string `json:"checksum"`
		URL      string `json:"url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("invalid JSON in update response: %w", err)
	}

	return &UpdateInfo{
		Version:     apiResp.Version,
		DownloadURL: apiResp.URL,
		Checksum:    apiResp.Checksum,
	}, nil
}

// downloadBinary downloads a binary from a URL to a destination path.
func downloadBinary(url string, destPath string) error {
	fmt.Printf("Attempting to download from URL: %s to %s\n", url, destPath)

	// Make the HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to initiate download from '%s': %w", url, err)
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status from server: %s (URL: %s)", resp.Status, url)
	}

	// Create the destination file
	// Note: os.Create truncates the file if it already exists.
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file '%s': %w", destPath, err)
	}
	defer destFile.Close()

	// Copy the contents from the response body to the destination file
	bytesCopied, err := io.Copy(destFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to copy downloaded content to '%s': %w", destPath, err)
	}
	fmt.Printf("Successfully downloaded %d bytes to %s\n", bytesCopied, destPath)

	// Make the downloaded binary executable. This is crucial.
	err = os.Chmod(destPath, 0755) // rwxr-xr-x permissions
	if err != nil {
		return fmt.Errorf("failed to set executable permission on new binary '%s': %w", destPath, err)
	}
	fmt.Printf("Binary saved to %s and set as executable.\n", destPath)
	return nil
}

// verifySHA256 computes the SHA256 hash of the file at filePath and compares it with expectedSHA256.
func verifySHA256(filePath string, expectedSHA256 string) (bool, error) {
	calculatedSHA256, err := calculateFileSHA256(filePath)
	if err != nil {
		return false, fmt.Errorf("could not calculate SHA256 for '%s': %w", filePath, err)
	}

	fmt.Printf("Calculated SHA256: %s\n", calculatedSHA256)
	fmt.Printf("Expected SHA256  : %s\n", expectedSHA256)

	return calculatedSHA256 == expectedSHA256, nil
}

// calculateFileSHA256 computes the SHA256 hash of a file.
func calculateFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file '%s' for hashing: %w", filePath, err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to copy file content to hasher for '%s': %w", filePath, err)
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// applyUpdate replaces the current executable file with the new one.
// On Unix-like systems, os.Rename is atomic if src and dst are on the same filesystem.
func applyUpdate(newExecPath string, targetPath string) error {
	fmt.Printf("Attempting to replace running executable '%s' with new binary '%s'\n", targetPath, newExecPath)

	// Attempt to rename the new binary to the location of the current executable.
	err := os.Rename(newExecPath, targetPath)
	if err != nil {
		return fmt.Errorf("failed to rename '%s' to '%s': %w", newExecPath, targetPath, err)
	}

	fmt.Printf("Successfully replaced '%s' with the new version from '%s'.\n", targetPath, newExecPath)
	return nil
}
