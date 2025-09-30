package updater

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"agent/internal/version"
)

// Some missing stuff:
// - Rollback mechanism: Keep the old binary as .old
// - Post install: Run some kind of post install health check

// tempSuffix is appended to the downloaded binary before it's installed
const tempSuffix = ".new"

// restartFileName is the name of the file created to signal a restart is needed
const restartFileName = "restart"

// httpClient is a shared HTTP client
var httpClient = &http.Client{Timeout: 10 * time.Second}

// remoteApiUrl is the URL of the remote API that is called to get
// info about the latest updates.
var remoteApiUrl = "https://api.simpleobservability.com"

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
	if envUrl := os.Getenv("API_URL"); envUrl != "" {
		remoteApiUrl = envUrl
	}
	updateInfo, err := checkForUpdate()
	if err != nil {
		return fmt.Errorf("error checking for updates: %v", err)
	}
	fmt.Printf("%+v\n", *updateInfo)

	// Check and compare versions
	if !targetVersionIsNewer(version.Version, updateInfo.Version) {
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

	// Create restart signal file
	fmt.Println("Creating restart signal file...")
	err = createRestartSignal(execPath)
	if err != nil {
		return fmt.Errorf("failed to create restart signal: %v", err)
	}

	fmt.Printf("Update completed successfully from version '%s' to version '%s'.\n", version.Version, updateInfo.Version)
	fmt.Println("\tIf the agent is running with systemd, it will auto-restart shortly.")
	fmt.Println("\tIf it's running without systemd, the agent will stop and needs manual restart.")
	return nil
}

// binaryName returns the name of the binary in the format "simob-<os>-<arch>".
// It uses the OS and ARCH environment variables if set;
// otherwise, it falls back to runtime.GOOS and runtime.GOARCH.
func binaryName() string {
	goos := os.Getenv("OS")
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := os.Getenv("ARCH")
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	return fmt.Sprintf("simob-%s-%s", goos, goarch)
}

// checkForUpdate checks the remote API for updates.
func checkForUpdate() (*UpdateInfo, error) {
	resp, err := httpClient.Get(remoteApiUrl + "/updates/")
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

	downloadURL := fmt.Sprintf("%s/%s", apiResp.URL, binaryName())

	expectedChecksum := strings.TrimSpace(apiResp.Checksum)
	// Prefer manifest approach: try to download checksums
	manifestChecksum, err := downloadChecksum(apiResp.URL, binaryName())
	if err != nil {
		fmt.Printf("Warning: could not fetch manifest checksum: %v\n", err)
	}
	if manifestChecksum != "" {
		expectedChecksum = manifestChecksum
	}

	// Fatal error if we still don’t have a checksum
	if expectedChecksum == "" {
		return nil, fmt.Errorf("no checksum available for binary %q (version %s, url %s)", binaryName(), apiResp.Version, apiResp.URL)
	}

	return &UpdateInfo{
		Version:     apiResp.Version,
		DownloadURL: downloadURL,
		Checksum:    expectedChecksum,
	}, nil
}

// downloadChecksum downloads <baseUrl>/checksums and returns the checksum for binaryName
func downloadChecksum(baseURL, binaryName string) (string, error) {
	manifestURL := strings.TrimRight(baseURL, "/") + "/checksums"
	resp, err := httpClient.Get(manifestURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch checksums manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("manifest not found: %s (status %d)", manifestURL, resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines
		if line == "" {
			continue
		}
		// Expected format: "<checksum> <filename>"
		parts := strings.Fields(line)
		checksum := parts[0]
		name := parts[1]
		if name == binaryName {
			return checksum, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error scanning checksums manifest: %w", err)
	}
	return "", fmt.Errorf("binary %q not listed in checksums manifest", binaryName)
}

// targetVersionIsNewer compares two semantic version strings in the format "MAJOR.MINOR.PATCH"
func targetVersionIsNewer(currentVersion, targetVersion string) bool {
	if currentVersion == "dev" {
		return true // always update the dev version
	}

	splitCurrent := strings.Split(currentVersion, ".")
	splitTarget := strings.Split(targetVersion, ".")
	if len(splitCurrent) != 3 || len(splitTarget) != 3 {
		fmt.Printf("Version format error: current=%q target=%q (expected 3 segments)\n", currentVersion, targetVersion)
		return false
	}
	for i := range 3 {
		currentPart, err1 := strconv.Atoi(splitCurrent[i])
		targetPart, err2 := strconv.Atoi(splitTarget[i])
		if err1 != nil || err2 != nil {
			return false
		}
		if targetPart > currentPart {
			return true
		} else if targetPart < currentPart {
			return false
		}
	}
	fmt.Println("Agent is already running the latest version.")
	return false // versions are equal
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

// createRestartSignal creates an empty "restart" file in the same directory as the executable
// to signal to the agent that a restart is needed.
func createRestartSignal(execPath string) error {
	// Get the directory containing the executable
	execDir := filepath.Dir(execPath)
	restartFilePath := filepath.Join(execDir, restartFileName)

	fmt.Printf("Creating restart signal file at: %s\n", restartFilePath)

	// Create an empty file
	file, err := os.Create(restartFilePath)
	if err != nil {
		return fmt.Errorf("failed to create restart signal file '%s': %w", restartFilePath, err)
	}
	defer file.Close()

	fmt.Printf("Successfully created restart signal file: %s\n", restartFilePath)
	return nil
}
