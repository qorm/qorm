package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// cmdUpdate checks for QORM CLI updates on GitHub and performs a self-update.
func cmdUpdate(args []string) int {
	fmt.Println("Checking for updates...")

	req, err := http.NewRequest("GET", "https://api.github.com/repos/qorm/qorm/releases/latest", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create request: %v\n", err)
		return 1
	}
	req.Header.Set("User-Agent", "qorm-cli-updater")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to check updates: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "error: failed to check updates, GitHub returned HTTP %d\n", resp.StatusCode)
		return 1
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to parse update response: %v\n", err)
		return 1
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(version, "v")

	if latestVersion == currentVersion {
		fmt.Printf("QORM is already up to date (version %s)\n", version)
		return 0
	}

	// Simple version comparison (stable string match or newer check)
	fmt.Printf("A new version of QORM is available: v%s (current: v%s)\n", latestVersion, currentVersion)

	// Phase 1: Try using 'go install' if Go toolchain is locally available
	if _, err := exec.LookPath("go"); err == nil {
		fmt.Println("Updating via 'go install'...")
		cmd := exec.Command("go", "install", "github.com/qorm/qorm/cmd/qorm@latest")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			fmt.Println("QORM updated successfully via go install!")
			return 0
		}
		fmt.Println("warn: go install failed, falling back to pre-compiled binary update...")
	}

	// Phase 2: Self-update by downloading the pre-compiled binary asset
	expectedAsset := fmt.Sprintf("qorm-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		expectedAsset += ".exe"
	}

	targetAsset := ""
	targetURL := ""
	for _, asset := range release.Assets {
		if strings.EqualFold(asset.Name, expectedAsset) {
			targetAsset = asset.Name
			targetURL = asset.DownloadURL
			break
		}
	}

	if targetURL == "" {
		fmt.Fprintf(os.Stderr, "error: no pre-compiled binary asset named %q found for %s/%s. Please update manually.\n", expectedAsset, runtime.GOOS, runtime.GOARCH)
		return 1
	}

	fmt.Printf("Downloading pre-compiled binary: %s...\n", targetAsset)

	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to resolve executable path: %v\n", err)
		return 1
	}

	respDL, err := client.Get(targetURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: download failed: %v\n", err)
		return 1
	}
	defer respDL.Body.Close()

	if respDL.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "error: download failed, GitHub returned HTTP %d\n", respDL.StatusCode)
		return 1
	}

	tmpDir := filepath.Dir(exePath)
	tmpFile, err := os.CreateTemp(tmpDir, "qorm_download_*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create temporary file: %v\n", err)
		return 1
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, respDL.Body); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to save download contents: %v\n", err)
		return 1
	}
	tmpFile.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to set executable permissions: %v\n", err)
		return 1
	}

	// Rename current executable to backup
	oldPath := exePath + ".old"
	_ = os.Remove(oldPath)

	if err := os.Rename(exePath, oldPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to back up current binary: %v\n", err)
		return 1
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to replace binary: %v (restoring backup...)\n", err)
		_ = os.Rename(oldPath, exePath)
		return 1
	}

	_ = os.Remove(oldPath)

	fmt.Printf("QORM updated successfully to version v%s!\n", latestVersion)
	return 0
}
