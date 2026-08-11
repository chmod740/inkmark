package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{left: "v1.2.0", right: "1.1.9", want: 1},
		{left: "1.0", right: "1.0.0", want: 0},
		{left: "2.0.0-beta.2", right: "2.0.0-beta.1", want: 1},
		{left: "2.0.0", right: "2.0.0-rc.1", want: 1},
		{left: "1.9.9", right: "1.10.0", want: -1},
	}
	for _, test := range tests {
		if got := compareVersions(test.left, test.right); sign(got) != test.want {
			t.Errorf("compareVersions(%q, %q): expected %d, got %d", test.left, test.right, test.want, got)
		}
	}
}

func TestCheckForUpdatesUsesGitHubReleaseData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("User-Agent") != "InkMark/"+appVersion {
			t.Errorf("unexpected User-Agent: %q", request.Header.Get("User-Agent"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"tag_name":"v9.9.9",
			"html_url":"https://github.com/chmod740/inkmark/releases/tag/v9.9.9",
			"published_at":"2026-08-04T12:00:00Z",
			"assets":[
				{"name":"InkMark-9.9.9-Windows-x64-Setup.exe","browser_download_url":"https://github.com/chmod740/inkmark/releases/download/v9.9.9/InkMark-9.9.9-Windows-x64-Setup.exe"},
				{"name":"InkMark-9.9.9-macOS-arm64.dmg","browser_download_url":"https://github.com/chmod740/inkmark/releases/download/v9.9.9/InkMark-9.9.9-macOS-arm64.dmg"}
			]
		}`))
	}))
	defer server.Close()

	app := NewApp()
	app.updateEndpoint = server.URL
	info, err := app.CheckForUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if !info.UpdateAvailable || info.CurrentVersion != appVersion || info.LatestVersion != "9.9.9" {
		t.Fatalf("unexpected update info: %#v", info)
	}
	if runtime.GOOS == "darwin" && !strings.HasSuffix(info.DownloadURL, "macOS-arm64.dmg") {
		t.Fatalf("unexpected macOS asset: %q", info.DownloadURL)
	}
}

func TestCheckForUpdatesRejectsUntrustedURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"tag_name":"v1.1.0","html_url":"https://example.com/fake","assets":[]}`))
	}))
	defer server.Close()

	app := NewApp()
	app.updateEndpoint = server.URL
	if _, err := app.CheckForUpdates(); err == nil || !strings.Contains(err.Error(), "untrusted") {
		t.Fatalf("expected untrusted URL error, got %v", err)
	}
}

func TestSelectReleaseAsset(t *testing.T) {
	assets := []githubReleaseAsset{
		{Name: "InkMark-1.1.0-macOS-x64.zip", BrowserDownloadURL: "https://github.com/example/mac-x64"},
		{Name: "InkMark-1.1.0-macOS-universal.dmg", BrowserDownloadURL: "https://github.com/example/mac-universal"},
		{Name: "InkMark-1.1.0-Windows-x64-Setup.exe", BrowserDownloadURL: "https://github.com/example/windows-x64"},
	}
	if got := selectReleaseAsset(assets, "darwin", "arm64"); got != "https://github.com/example/mac-universal" {
		t.Fatalf("unexpected macOS asset: %q", got)
	}
	if got := selectReleaseAsset(assets, "windows", "amd64"); got != "https://github.com/example/windows-x64" {
		t.Fatalf("unexpected Windows asset: %q", got)
	}
}

func TestAppInfoIsReleaseMetadata(t *testing.T) {
	info := NewApp().GetAppInfo()
	if info.Version != appVersion || info.Author != "PengHu" || info.RepositoryURL != sourceRepositoryURL {
		t.Fatalf("unexpected app info: %#v", info)
	}
}

func TestAppVersionMatchesWailsMetadata(t *testing.T) {
	payload, err := os.ReadFile("wails.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(payload, &config); err != nil {
		t.Fatal(err)
	}
	if config.Info.ProductVersion != appVersion {
		t.Fatalf("app version %q does not match wails.json %q", appVersion, config.Info.ProductVersion)
	}

	frontendPayload, err := os.ReadFile("frontend/package.json")
	if err != nil {
		t.Fatal(err)
	}
	var frontendConfig struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(frontendPayload, &frontendConfig); err != nil {
		t.Fatal(err)
	}
	if frontendConfig.Version != appVersion {
		t.Fatalf("app version %q does not match frontend/package.json %q", appVersion, frontendConfig.Version)
	}
}

func sign(value int) int {
	if value < 0 {
		return -1
	}
	if value > 0 {
		return 1
	}
	return 0
}
