package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchUpdateInstallerPassesVerifiedPrivateInstaller(t *testing.T) {
	cache := t.TempDir()
	directory := filepath.Join(cache, "v1.2.0")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("installer ready for platform handoff")
	installerPath := filepath.Join(directory, testInstallerAssetName())
	if err := os.WriteFile(installerPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	app.updateCacheDir = cache
	app.downloadedUpdate = downloadedUpdate{
		AssetName: testInstallerAssetName(), Version: "1.2.0", Path: installerPath,
		Digest: sha256Hex(payload), Size: int64(len(payload)), InstallerKind: testInstallerKind(),
		ReleaseURL: "https://github.com/chmod740/inkmark/releases/tag/v1.2.0",
	}
	called := false
	app.updateInstallerLauncher = func(request updateLaunchRequest) error {
		called = true
		if request.InstallerPath != installerPath || request.ParentPID != os.Getpid() || request.InstallerKind != testInstallerKind() {
			t.Fatalf("unexpected launch request: %#v", request)
		}
		return nil
	}
	if err := app.LaunchUpdateInstaller(); err != nil {
		t.Fatal(err)
	}
	if !called || !app.updateLaunching {
		t.Fatalf("installer was not handed off: called=%v launching=%v", called, app.updateLaunching)
	}
}

func TestLaunchUpdateInstallerReverifiesDigest(t *testing.T) {
	cache := t.TempDir()
	path := filepath.Join(cache, testInstallerAssetName())
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	app.updateCacheDir = cache
	app.downloadedUpdate = downloadedUpdate{
		AssetName: testInstallerAssetName(), Version: "1.2.0", Path: path,
		Digest: strings.Repeat("0", 64), Size: int64(len("changed")), InstallerKind: testInstallerKind(),
	}
	if err := app.LaunchUpdateInstaller(); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected checksum failure, got %v", err)
	}
}

func TestLaunchUpdateInstallerRejectsPathEscape(t *testing.T) {
	cache := t.TempDir()
	outside := filepath.Join(filepath.Dir(cache), testInstallerAssetName())
	payload := []byte("outside")
	if err := os.WriteFile(outside, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	app := NewApp()
	app.updateCacheDir = cache
	app.downloadedUpdate = downloadedUpdate{
		AssetName: testInstallerAssetName(), Version: "1.2.0", Path: outside,
		Digest: sha256Hex(payload), Size: int64(len(payload)), InstallerKind: testInstallerKind(),
	}
	if err := app.LaunchUpdateInstaller(); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("expected cache escape rejection, got %v", err)
	}
}
