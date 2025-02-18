package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	MB_OK         = 0x00000000
	MB_ICONERROR  = 0x00000010
	updateTimeout = 5 * time.Second
	bufferSize    = 32 * 1024
)

type Release struct {
	Assets  []ReleaseAsset `json:"assets"`
	TagName string         `json:"tag_name"`
}

type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	TagName            string `json:"tag_name"`
}

type UpdateManager struct {
	sync.RWMutex
	owner  string
	repo   string
	client *http.Client
}

func NewUpdateManager(owner, repo string) *UpdateManager {
	return &UpdateManager{
		owner: owner,
		repo:  repo,
		client: &http.Client{
			Timeout: updateTimeout,
		},
	}
}

func (um *UpdateManager) DownloadLatestAsset(assetName, currentVersion string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	tmpDir := filepath.Join(filepath.Dir(exePath), "temp_update")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %v", err)
	}

	assetURL, err := um.getAssetURL(assetName)
	if err != nil {
		return err
	}

	tempPath := filepath.Join(tmpDir, assetName)
	if err := um.downloadFile(assetURL, tempPath); err != nil {
		return err
	}

	if err := um.createAndExecuteUpdateScript(exePath, tempPath, tmpDir); err != nil {
		return err
	}

	os.Exit(0)
	return nil
}

func (um *UpdateManager) downloadFile(url, destPath string) error {
	resp, err := um.client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download asset: %v", err)
	}
	defer resp.Body.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer out.Close()

	buf := make([]byte, bufferSize)
	_, err = io.CopyBuffer(out, resp.Body, buf)
	return err
}

func (um *UpdateManager) getAssetURL(assetName string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", um.owner, um.repo)

	resp, err := um.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch release data: %v", err)
	}
	defer resp.Body.Close()

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to decode release data: %v", err)
	}

	for _, asset := range release.Assets {
		if asset.Name == assetName {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", errors.New("asset not found")
}

func (um *UpdateManager) createAndExecuteUpdateScript(exePath, tempPath, tmpDir string) error {
	updateScript := filepath.Join(tmpDir, "update.bat")
	scriptContent := fmt.Sprintf(`@echo off
timeout /t 1 /nobreak >nul
del "%s"
move "%s" "%s"
start "" "%s"
del "%s"
`, exePath, tempPath, exePath, exePath, updateScript)

	if err := os.WriteFile(updateScript, []byte(scriptContent), 0755); err != nil {
		return fmt.Errorf("failed to create update script: %v", err)
	}

	cmd := exec.Command("cmd", "/C", updateScript)
	return cmd.Start()
}

func ShowErrorMessageBox(message, title string) int {
	user32 := syscall.NewLazyDLL("user32.dll")
	msgbox := user32.NewProc("MessageBoxW")

	ret, _, _ := msgbox.Call(
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(message))),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(title))),
		uintptr(MB_ICONERROR|MB_OK),
	)

	return int(ret)
}

func (um *UpdateManager) CheckVersion(currentVersion string) bool {
	latestVersion, err := um.GetLatestVersion()
	if err != nil {
		return true
	}
	return currentVersion != latestVersion
}

func (um *UpdateManager) GetLatestVersion() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", um.owner, um.repo)

	resp, err := um.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch release data: %v", err)
	}
	defer resp.Body.Close()

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to decode release data: %v", err)
	}

	return release.TagName, nil
}

func ShowUpdateProgress(message string) {
	fmt.Println(message)
}
