package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadUpdateVerifiesAndReusesCachedInstaller(t *testing.T) {
	payload := []byte("verified InkMark installer payload")
	digest := sha256Hex(payload)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Header.Get("User-Agent") != "InkMark/"+appVersion {
			t.Errorf("unexpected User-Agent: %q", request.Header.Get("User-Agent"))
		}
		writer.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		_, _ = writer.Write(payload)
	}))
	defer server.Close()

	app := preparedDownloadApp(t, server, payload, "sha256:"+digest)
	first, err := app.DownloadUpdate("test-first")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Ready || first.Progress != 1 || first.BytesDownloaded != int64(len(payload)) || first.SessionID != "test-first" {
		t.Fatalf("unexpected download result: %#v", first)
	}
	path := filepath.Join(app.updateCacheDir, "v1.2.0", testInstallerAssetName())
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(payload) {
		t.Fatal("cached installer payload changed")
	}

	second, err := app.DownloadUpdate("test-second")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Ready || requests.Load() != 1 {
		t.Fatalf("verified cache was not reused: result=%#v requests=%d", second, requests.Load())
	}
}

func TestCancelUpdateDownloadStopsActiveSessionAndRemovesPartialFile(t *testing.T) {
	payload := make([]byte, 256<<10)
	for index := range payload {
		payload[index] = byte(index % 251)
	}
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		_, _ = writer.Write(payload[:32<<10])
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()

	app := preparedDownloadApp(t, server, payload, "sha256:"+sha256Hex(payload))
	result := make(chan error, 1)
	go func() {
		_, err := app.DownloadUpdate("cancel-session")
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("download did not start")
	}
	app.CancelUpdateDownload()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelled download unexpectedly succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled download did not stop")
	}
	destination := filepath.Join(app.updateCacheDir, "v1.2.0", testInstallerAssetName())
	if _, err := os.Stat(destination + ".part"); !os.IsNotExist(err) {
		t.Fatalf("cancelled partial file remains: %v", err)
	}
	app.mu.RLock()
	downloading := app.updateDownloading
	app.mu.RUnlock()
	if downloading {
		t.Fatal("download state remained active after cancellation")
	}
}

func TestDownloadUpdateRejectsChecksumMismatchAndRemovesPartialFile(t *testing.T) {
	payload := []byte("tampered installer")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(payload)
	}))
	defer server.Close()

	app := preparedDownloadApp(t, server, payload, strings.Repeat("0", 64))
	if _, err := app.DownloadUpdate("test-mismatch"); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected checksum failure, got %v", err)
	}
	destination := filepath.Join(app.updateCacheDir, "v1.2.0", testInstallerAssetName())
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("unverified installer remains in cache: %v", err)
	}
	if _, err := os.Stat(destination + ".part"); !os.IsNotExist(err) {
		t.Fatalf("partial installer remains in cache: %v", err)
	}
}

func TestDownloadUpdateUsesExactChecksumManifestEntry(t *testing.T) {
	payload := []byte("manifest verified installer")
	digest := sha256Hex(payload)
	assetName := testInstallerAssetName()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/SHA256SUMS":
			_, _ = fmt.Fprintf(writer, "%s  other-file.pkg\n%s  %s\n", strings.Repeat("a", 64), digest, assetName)
		case "/installer":
			_, _ = writer.Write(payload)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	app := preparedDownloadApp(t, server, payload, "")
	app.updateAsset.BrowserDownloadURL = server.URL + "/installer"
	app.checksumAsset = githubReleaseAsset{Name: "SHA256SUMS", BrowserDownloadURL: server.URL + "/SHA256SUMS"}
	result, err := app.DownloadUpdate("test-manifest")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestDownloadUpdateRejectsUnsafeFilenameAndMissingChecksum(t *testing.T) {
	app := NewApp()
	app.allowTestUpdateURLs = true
	app.latestUpdate = UpdateInfo{LatestVersion: "1.2.0", UpdateAvailable: true, Installable: true, InstallerKind: testInstallerKind()}
	app.updateAsset = githubReleaseAsset{Name: "../installer.exe", BrowserDownloadURL: "https://github.com/chmod740/inkmark/releases/download/v1.2.0/installer.exe"}
	if _, err := app.DownloadUpdate("test-unsafe"); err == nil || !strings.Contains(err.Error(), "unsafe filename") {
		t.Fatalf("expected unsafe filename error, got %v", err)
	}

	app.updateAsset = githubReleaseAsset{Name: testInstallerAssetName(), BrowserDownloadURL: "https://github.com/chmod740/inkmark/releases/download/v1.2.0/installer"}
	if _, err := app.DownloadUpdate("test-missing-checksum"); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("expected missing checksum error, got %v", err)
	}
}

func TestDownloadUpdateRejectsInvalidSessionID(t *testing.T) {
	if _, err := NewApp().DownloadUpdate("../wrong"); err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("expected invalid session error, got %v", err)
	}
}

func TestDigestFromChecksumReaderRejectsConflicts(t *testing.T) {
	name := testInstallerAssetName()
	payload := strings.Repeat("a", 64) + "  " + name + "\n" + strings.Repeat("b", 64) + " *" + name + "\n"
	if _, err := digestFromChecksumReader(strings.NewReader(payload), name); err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestUpdateAssetSelectionPrefersInstallers(t *testing.T) {
	assets := []githubReleaseAsset{
		{Name: "InkMark-1.2.0-Windows-x64-portable.exe", BrowserDownloadURL: "portable"},
		{Name: "InkMark-1.2.0-Windows-x64-Setup.exe", BrowserDownloadURL: "setup"},
		{Name: "InkMark-1.2.0-macOS-universal.dmg", BrowserDownloadURL: "dmg"},
		{Name: "InkMark-1.2.0-macOS-universal.pkg", BrowserDownloadURL: "pkg"},
	}
	windowsAsset, ok := selectReleaseAssetDetails(assets, "windows", "amd64")
	if !ok || windowsAsset.BrowserDownloadURL != "setup" {
		t.Fatalf("Windows setup was not selected: %#v", windowsAsset)
	}
	macAsset, ok := selectReleaseAssetDetails(assets, "darwin", "arm64")
	if !ok || macAsset.BrowserDownloadURL != "pkg" {
		t.Fatalf("macOS package was not selected: %#v", macAsset)
	}
}

func preparedDownloadApp(t *testing.T, server *httptest.Server, payload []byte, digest string) *App {
	t.Helper()
	name := testInstallerAssetName()
	app := NewApp()
	app.updateDownloadClient = server.Client()
	app.updateCacheDir = t.TempDir()
	app.allowTestUpdateURLs = true
	app.latestUpdate = UpdateInfo{
		CurrentVersion: appVersion, LatestVersion: "1.2.0", UpdateAvailable: true,
		ReleaseURL:  "https://github.com/chmod740/inkmark/releases/tag/v1.2.0",
		DownloadURL: server.URL, AssetName: name, AssetSize: int64(len(payload)),
		Installable: true, ChecksumAvailable: digest != "", InstallerKind: testInstallerKind(),
	}
	app.updateAsset = githubReleaseAsset{
		Name: name, BrowserDownloadURL: server.URL, Size: int64(len(payload)), Digest: digest,
	}
	return app
}

func testInstallerAssetName() string {
	switch runtime.GOOS {
	case "windows":
		return "InkMark-1.2.0-Windows-x64-Setup.exe"
	case "darwin":
		return "InkMark-1.2.0-macOS-universal.pkg"
	default:
		return "InkMark-1.2.0-Linux-x64.AppImage"
	}
}

func testInstallerKind() string {
	return installerKindForAsset(testInstallerAssetName(), runtime.GOOS)
}

func sha256Hex(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
