package checker

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultArtifact     = "24.04.4/ubuntu-24.04.3-live-server-amd64.iso"
	officialReleasesURL = "https://releases.ubuntu.com"
	httpTimeout         = 5 * time.Minute
)

// CheckResult represents the result of checking a mirror
type CheckResult struct {
	Success        bool
	ErrorMessage   string
	TestFile       string
	CorruptedFiles []string
	DownloadPath   string
}

// Checker handles mirror integrity checks
type Checker struct {
	client        *http.Client
	downloadDir   string
	corruptedDir  string
	checksumCache map[string]map[string]string // artifactPath -> (filename -> checksum)
	checksumMutex sync.RWMutex
}

// NewChecker creates a new Checker instance
func NewChecker(downloadDir, corruptedDir string) *Checker {
	if downloadDir == "" {
		downloadDir = "downloads"
	}
	if corruptedDir == "" {
		corruptedDir = "corrupted"
	}

	return &Checker{
		client: &http.Client{
			Timeout: httpTimeout,
		},
		downloadDir:   downloadDir,
		corruptedDir:  corruptedDir,
		checksumCache: make(map[string]map[string]string),
	}
}

// CheckMirror checks the integrity of a mirror against the official Ubuntu archive
func (c *Checker) CheckMirror(ctx context.Context, mirrorURL, artifactPath string) (*CheckResult, error) {
	if artifactPath == "" {
		artifactPath = defaultArtifact
	}

	result := &CheckResult{
		TestFile: artifactPath,
	}

	slog.Info("Starting mirror check", "mirror", mirrorURL, "artifact", artifactPath)

	// Ensure download and corrupted directories exist
	if err := os.MkdirAll(c.downloadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create download directory: %w", err)
	}
	if err := os.MkdirAll(c.corruptedDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create corrupted directory: %w", err)
	}

	// Check cache first
	c.checksumMutex.RLock()
	officialChecksums, cached := c.checksumCache[artifactPath]
	c.checksumMutex.RUnlock()

	// Download SHA256SUMS from official source if not cached
	if !cached {
		var err error
		officialChecksums, err = c.downloadChecksums(ctx, officialReleasesURL, artifactPath)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("Failed to download official checksums: %v", err)
			return result, nil
		}

		// Store in cache
		c.checksumMutex.Lock()
		c.checksumCache[artifactPath] = officialChecksums
		c.checksumMutex.Unlock()

		slog.Debug("Cached checksums for artifact", "artifact", artifactPath)
	} else {
		slog.Debug("Using cached checksums", "artifact", artifactPath)
	}

	expectedChecksum, ok := officialChecksums[filepath.Base(artifactPath)]
	if !ok {
		result.ErrorMessage = fmt.Sprintf("Checksum not found for artifact: %s", artifactPath)
		return result, nil
	}

	slog.Info("Official checksum retrieved", "file", filepath.Base(artifactPath), "checksum", expectedChecksum)

	// Download file from mirror
	mirrorURL = strings.TrimSuffix(mirrorURL, "/")
	fileURL := fmt.Sprintf("%s/%s", mirrorURL, artifactPath)

	downloadPath, actualChecksum, err := c.downloadAndChecksum(ctx, fileURL)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to download from mirror: %v", err)
		return result, nil
	}

	result.DownloadPath = downloadPath

	// Compare checksums
	if expectedChecksum != actualChecksum {
		slog.Warn("Checksum mismatch detected",
			"expected", expectedChecksum,
			"actual", actualChecksum,
			"mirror", mirrorURL)

		// Store corrupted file with hash as filename in corrupted subdirectory
		corruptedPath := filepath.Join(c.corruptedDir, actualChecksum)
		if err := os.Rename(downloadPath, corruptedPath); err != nil {
			slog.Error("Failed to rename corrupted file", "error", err)
		} else {
			result.CorruptedFiles = append(result.CorruptedFiles, actualChecksum)
			result.DownloadPath = corruptedPath
			slog.Info("Corrupted file saved", "path", corruptedPath)
		}
		result.Success = false
		return result, nil
	}

	slog.Info("Mirror check passed", "mirror", mirrorURL)
	result.Success = true

	// Clean up downloaded file on success
	if err := os.Remove(downloadPath); err != nil {
		slog.Warn("Failed to clean up downloaded file", "path", downloadPath, "error", err)
	}

	return result, nil
}

// downloadChecksums downloads and parses the SHA256SUMS file
func (c *Checker) downloadChecksums(ctx context.Context, baseURL, artifactPath string) (map[string]string, error) {
	dir := filepath.Dir(artifactPath)
	checksumURL := fmt.Sprintf("%s/%s/SHA256SUMS", baseURL, dir)

	slog.Debug("Downloading checksums", "url", checksumURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	checksums := make(map[string]string)
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			checksum := parts[0]
			// Handle both "*filename" and "filename" formats
			filename := strings.TrimPrefix(parts[1], "*")
			checksums[filename] = checksum
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse checksums: %w", err)
	}

	return checksums, nil
}

// downloadAndChecksum downloads a file and calculates its SHA256 checksum
func (c *Checker) downloadAndChecksum(ctx context.Context, url string) (string, string, error) {
	slog.Info("Downloading file", "url", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Create temporary file
	tempFile, err := os.CreateTemp(c.downloadDir, "mirror-check-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	// Download and calculate checksum simultaneously
	hash := sha256.New()
	writer := io.MultiWriter(tempFile, hash)

	_, err = io.Copy(writer, resp.Body)
	if err != nil {
		os.Remove(tempFile.Name())
		return "", "", fmt.Errorf("failed to download and hash file: %w", err)
	}

	checksum := fmt.Sprintf("%x", hash.Sum(nil))
	slog.Debug("File downloaded and checksummed", "path", tempFile.Name(), "checksum", checksum)

	return tempFile.Name(), checksum, nil
}
