package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	maxUpdateDownloadSize = int64(512 << 20)
	maxChecksumFileSize   = int64(1 << 20)
)

type UpdateDownload struct {
	SessionID       string  `json:"sessionID"`
	AssetName       string  `json:"assetName"`
	Version         string  `json:"version"`
	BytesDownloaded int64   `json:"bytesDownloaded"`
	TotalBytes      int64   `json:"totalBytes"`
	Progress        float64 `json:"progress"`
	Ready           bool    `json:"ready"`
}

type downloadedUpdate struct {
	AssetName     string
	Version       string
	Path          string
	Digest        string
	Size          int64
	InstallerKind string
	ReleaseURL    string
}

// DownloadUpdate downloads the installer selected by CheckForUpdates into a
// private cache, verifies its SHA-256 digest, and makes it ready to launch.
// URLs are never accepted from the frontend.
func (a *App) DownloadUpdate(sessionID string) (UpdateDownload, error) {
	if !safeUpdateSessionID(sessionID) {
		return UpdateDownload{}, errors.New("the update download session identifier is invalid")
	}
	a.mu.Lock()
	if a.updateChecking {
		a.mu.Unlock()
		return UpdateDownload{}, errors.New("an update check is still in progress")
	}
	if a.updateDownloading {
		a.mu.Unlock()
		return UpdateDownload{}, errors.New("an update download is already in progress")
	}
	if a.updateLaunching {
		a.mu.Unlock()
		return UpdateDownload{}, errors.New("the downloaded update is already being installed")
	}
	info := a.latestUpdate
	asset := a.updateAsset
	checksumAsset := a.checksumAsset
	client := a.updateDownloadClient
	baseContext := a.ctx
	cacheOverride := a.updateCacheDir
	allowTestURLs := a.allowTestUpdateURLs
	if !info.UpdateAvailable {
		a.mu.Unlock()
		return UpdateDownload{}, errors.New("no newer version is available")
	}
	if !info.Installable || strings.TrimSpace(asset.BrowserDownloadURL) == "" {
		a.mu.Unlock()
		return UpdateDownload{}, errors.New("this release does not contain an installer for this platform")
	}
	if !safeAssetName(asset.Name) {
		a.mu.Unlock()
		return UpdateDownload{}, errors.New("the update asset has an unsafe filename")
	}
	if !allowTestURLs && !isTrustedUpdateTransportURL(asset.BrowserDownloadURL) {
		a.mu.Unlock()
		return UpdateDownload{}, errors.New("refusing to download an update from an untrusted URL")
	}
	if client == nil {
		client = newUpdateDownloadHTTPClient()
	}
	if baseContext == nil {
		baseContext = context.Background()
	}
	downloadContext, cancel := context.WithCancel(baseContext)
	a.updateDownloading = true
	a.updateCancel = cancel
	a.mu.Unlock()

	defer func() {
		cancel()
		a.mu.Lock()
		a.updateDownloading = false
		a.updateCancel = nil
		a.mu.Unlock()
	}()

	digest, err := a.resolveAssetDigest(downloadContext, client, asset, checksumAsset, allowTestURLs)
	if err != nil {
		return UpdateDownload{}, err
	}
	cacheRoot, err := updateCacheRoot(cacheOverride)
	if err != nil {
		return UpdateDownload{}, err
	}
	versionDirectory := filepath.Join(cacheRoot, "v"+safePathSegment(info.LatestVersion))
	if err := os.MkdirAll(versionDirectory, 0o700); err != nil {
		return UpdateDownload{}, fmt.Errorf("create update cache: %w", err)
	}
	if err := os.Chmod(versionDirectory, 0o700); err != nil && !errors.Is(err, os.ErrPermission) {
		return UpdateDownload{}, fmt.Errorf("secure update cache: %w", err)
	}
	destination := filepath.Join(versionDirectory, asset.Name)

	if fileMatchesUpdate(destination, digest, asset.Size) {
		result := updateDownloadResult(sessionID, asset.Name, info.LatestVersion, fileSize(destination), fileSize(destination), true)
		a.rememberDownloadedUpdate(downloadedUpdate{
			AssetName: asset.Name, Version: info.LatestVersion, Path: destination,
			Digest: digest, Size: fileSize(destination), InstallerKind: info.InstallerKind,
			ReleaseURL: info.ReleaseURL,
		})
		a.emitUpdateProgress(result)
		return result, nil
	}

	result, err := a.downloadInstaller(downloadContext, client, asset, destination, digest, info.LatestVersion, sessionID, allowTestURLs)
	if err != nil {
		return UpdateDownload{}, err
	}
	a.rememberDownloadedUpdate(downloadedUpdate{
		AssetName: asset.Name, Version: info.LatestVersion, Path: destination,
		Digest: digest, Size: result.BytesDownloaded, InstallerKind: info.InstallerKind,
		ReleaseURL: info.ReleaseURL,
	})
	return result, nil
}

func (a *App) CancelUpdateDownload() {
	a.mu.RLock()
	cancel := a.updateCancel
	a.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) rememberDownloadedUpdate(update downloadedUpdate) {
	a.mu.Lock()
	a.downloadedUpdate = update
	a.mu.Unlock()
}

func (a *App) resolveAssetDigest(ctx context.Context, client httpDoer, asset githubReleaseAsset, checksumAsset githubReleaseAsset, allowTestURLs bool) (string, error) {
	if digest, ok := parseSHA256Digest(asset.Digest); ok {
		return digest, nil
	}
	if checksumAsset.BrowserDownloadURL == "" {
		return "", errors.New("the release does not provide a SHA-256 checksum for this installer")
	}
	if !allowTestURLs && !isTrustedUpdateTransportURL(checksumAsset.BrowserDownloadURL) {
		return "", errors.New("refusing to download checksums from an untrusted URL")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumAsset.BrowserDownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("create checksum request: %w", err)
	}
	setUpdateDownloadHeaders(request)
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download update checksums: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download update checksums: HTTP %d", response.StatusCode)
	}
	if !allowTestURLs && !isTrustedUpdateTransportURL(response.Request.URL.String()) {
		return "", errors.New("checksum download ended at an untrusted URL")
	}
	digest, err := digestFromChecksumReader(io.LimitReader(response.Body, maxChecksumFileSize+1), asset.Name)
	if err != nil {
		return "", err
	}
	return digest, nil
}

func (a *App) downloadInstaller(ctx context.Context, client httpDoer, asset githubReleaseAsset, destination string, digest string, version string, sessionID string, allowTestURLs bool) (UpdateDownload, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return UpdateDownload{}, fmt.Errorf("create update download request: %w", err)
	}
	setUpdateDownloadHeaders(request)
	response, err := client.Do(request)
	if err != nil {
		return UpdateDownload{}, fmt.Errorf("download update: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return UpdateDownload{}, fmt.Errorf("download update: HTTP %d", response.StatusCode)
	}
	if !allowTestURLs && !isTrustedUpdateTransportURL(response.Request.URL.String()) {
		return UpdateDownload{}, errors.New("update download ended at an untrusted URL")
	}
	if response.ContentLength > maxUpdateDownloadSize {
		return UpdateDownload{}, errors.New("the update installer exceeds the download size limit")
	}
	if asset.Size > maxUpdateDownloadSize {
		return UpdateDownload{}, errors.New("the update installer exceeds the download size limit")
	}

	total := asset.Size
	if total <= 0 {
		total = response.ContentLength
	}
	temporary := destination + ".part"
	_ = os.Remove(temporary)
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return UpdateDownload{}, fmt.Errorf("create update download: %w", err)
	}
	completed := false
	defer func() {
		_ = file.Close()
		if !completed {
			_ = os.Remove(temporary)
		}
	}()

	hasher := sha256.New()
	reader := io.LimitReader(response.Body, maxUpdateDownloadSize+1)
	progressWriter := &downloadProgressWriter{
		hash:  hasher,
		file:  file,
		total: total,
		emit: func(written int64, expected int64) {
			a.emitUpdateProgress(updateDownloadResult(sessionID, asset.Name, version, written, expected, false))
		},
	}
	written, err := io.CopyBuffer(progressWriter, reader, make([]byte, 128<<10))
	if err != nil {
		return UpdateDownload{}, fmt.Errorf("download update: %w", err)
	}
	if written > maxUpdateDownloadSize {
		return UpdateDownload{}, errors.New("the update installer exceeds the download size limit")
	}
	if asset.Size > 0 && written != asset.Size {
		return UpdateDownload{}, fmt.Errorf("update installer size mismatch: expected %d bytes, received %d", asset.Size, written)
	}
	actualDigest := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualDigest, digest) {
		return UpdateDownload{}, errors.New("update installer checksum verification failed")
	}
	if err := file.Sync(); err != nil {
		return UpdateDownload{}, fmt.Errorf("flush update download: %w", err)
	}
	if err := file.Close(); err != nil {
		return UpdateDownload{}, fmt.Errorf("close update download: %w", err)
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return UpdateDownload{}, fmt.Errorf("replace cached update: %w", err)
	}
	if err := os.Rename(temporary, destination); err != nil {
		return UpdateDownload{}, fmt.Errorf("finish update download: %w", err)
	}
	completed = true
	result := updateDownloadResult(sessionID, asset.Name, version, written, total, true)
	a.emitUpdateProgress(result)
	return result, nil
}

type downloadProgressWriter struct {
	file        io.Writer
	hash        hash.Hash
	written     int64
	total       int64
	emit        func(int64, int64)
	lastPercent int
}

func (writer *downloadProgressWriter) Write(payload []byte) (int, error) {
	count, err := writer.file.Write(payload)
	if count > 0 {
		_, _ = writer.hash.Write(payload[:count])
		writer.written += int64(count)
		percent := 0
		if writer.total > 0 {
			percent = int(float64(writer.written) * 100 / float64(writer.total))
		}
		if writer.emit != nil && (writer.written == int64(count) || percent >= writer.lastPercent+1 || writer.written == writer.total) {
			writer.lastPercent = percent
			writer.emit(writer.written, writer.total)
		}
	}
	return count, err
}

func (a *App) emitUpdateProgress(progress UpdateDownload) {
	ctx := a.currentContext()
	if ctx != nil {
		wailsruntime.EventsEmit(ctx, updateProgressEvent, progress)
	}
}

func updateDownloadResult(sessionID string, name string, version string, written int64, total int64, ready bool) UpdateDownload {
	progress := float64(0)
	if total > 0 {
		progress = float64(written) / float64(total)
		if progress > 1 {
			progress = 1
		}
	} else if ready {
		progress = 1
	}
	return UpdateDownload{
		SessionID: sessionID,
		AssetName: name, Version: version, BytesDownloaded: written,
		TotalBytes: total, Progress: progress, Ready: ready,
	}
}

func safeUpdateSessionID(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func setUpdateDownloadHeaders(request *http.Request) {
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "InkMark/"+appVersion)
}

func parseSHA256Digest(value string) (string, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "sha256:")
	if len(value) != sha256.Size*2 {
		return "", false
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", false
	}
	return value, true
}

func validSHA256Digest(value string) bool {
	_, ok := parseSHA256Digest(value)
	return ok
}

func digestFromChecksumReader(reader io.Reader, assetName string) (string, error) {
	if !safeAssetName(assetName) {
		return "", errors.New("the update asset has an unsafe filename")
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), int(maxChecksumFileSize+1))
	match := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name != assetName {
			continue
		}
		digest, ok := parseSHA256Digest(fields[0])
		if !ok {
			return "", errors.New("the checksum file contains an invalid SHA-256 digest")
		}
		if match != "" && match != digest {
			return "", errors.New("the checksum file contains conflicting entries for the installer")
		}
		match = digest
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read update checksums: %w", err)
	}
	if match == "" {
		return "", errors.New("the checksum file does not contain the selected installer")
	}
	return match, nil
}

func safeAssetName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name &&
		!strings.ContainsAny(name, `/\\`) && !strings.ContainsRune(name, '\x00')
}

func safePathSegment(value string) string {
	var builder strings.Builder
	for _, character := range strings.TrimSpace(value) {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '-' || character == '_' {
			builder.WriteRune(character)
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}

func updateCacheRoot(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return filepath.Abs(override)
	}
	cacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache: %w", err)
	}
	return filepath.Join(cacheDirectory, "InkMark", "updates"), nil
}

func fileMatchesUpdate(path string, digest string, expectedSize int64) bool {
	linkInfo, err := os.Lstat(path)
	if err != nil || linkInfo.Mode()&os.ModeSymlink != 0 {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxUpdateDownloadSize ||
		(expectedSize > 0 && info.Size() != expectedSize) {
		return false
	}
	actual, err := fileSHA256(path)
	return err == nil && strings.EqualFold(actual, digest)
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, io.LimitReader(file, maxUpdateDownloadSize+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
