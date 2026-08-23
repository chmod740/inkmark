package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	appVersion             = "1.2.10"
	appAuthor              = "Codex"
	sourceRepositoryURL    = "https://github.com/chmod740/inkmark"
	latestReleaseAPIURL    = "https://api.github.com/repos/chmod740/inkmark/releases/latest"
	updateRequestTimeout   = 12 * time.Second
	updateDownloadTimeout  = 30 * time.Minute
	maxReleaseResponseSize = 2 << 20
	updateProgressEvent    = "inkmark:update-progress"
)

type AppInfo struct {
	Version       string `json:"version"`
	Author        string `json:"author"`
	RepositoryURL string `json:"repositoryURL"`
}

type UpdateInfo struct {
	CurrentVersion    string `json:"currentVersion"`
	LatestVersion     string `json:"latestVersion"`
	UpdateAvailable   bool   `json:"updateAvailable"`
	ReleaseURL        string `json:"releaseURL"`
	DownloadURL       string `json:"downloadURL"`
	PublishedAt       string `json:"publishedAt"`
	AssetName         string `json:"assetName"`
	AssetSize         int64  `json:"assetSize"`
	Installable       bool   `json:"installable"`
	ChecksumAvailable bool   `json:"checksumAvailable"`
	InstallerKind     string `json:"installerKind"`
}

type githubRelease struct {
	TagName     string               `json:"tag_name"`
	HTMLURL     string               `json:"html_url"`
	PublishedAt string               `json:"published_at"`
	Assets      []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func newUpdateHTTPClient() httpDoer {
	return newTrustedUpdateClient(updateRequestTimeout)
}

func newUpdateDownloadHTTPClient() httpDoer {
	return newTrustedUpdateClient(updateDownloadTimeout)
}

func newTrustedUpdateClient(timeout time.Duration) httpDoer {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 6 {
				return errors.New("too many update download redirects")
			}
			if !isTrustedUpdateTransportURL(request.URL.String()) {
				return errors.New("update download redirected to an untrusted URL")
			}
			return nil
		},
	}
}

func (a *App) GetAppInfo() AppInfo {
	return AppInfo{
		Version:       appVersion,
		Author:        appAuthor,
		RepositoryURL: sourceRepositoryURL,
	}
}

func (a *App) CheckForUpdates() (UpdateInfo, error) {
	a.mu.Lock()
	if a.updateChecking {
		a.mu.Unlock()
		return UpdateInfo{}, errors.New("an update check is already in progress")
	}
	if a.updateDownloading || a.updateLaunching {
		a.mu.Unlock()
		return UpdateInfo{}, errors.New("an update is already being downloaded or installed")
	}
	a.updateChecking = true
	endpoint := a.updateEndpoint
	client := a.updateClient
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.updateChecking = false
		a.mu.Unlock()
	}()
	if strings.TrimSpace(endpoint) == "" {
		endpoint = latestReleaseAPIURL
	}
	if client == nil {
		client = newUpdateHTTPClient()
	}

	ctx := a.currentContext()
	if ctx == nil {
		ctx = context.Background()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("create update request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "InkMark/"+appVersion)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	response, err := client.Do(request)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("check for updates: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return UpdateInfo{}, fmt.Errorf("check for updates: GitHub Releases returned HTTP %d", response.StatusCode)
	}

	var release githubRelease
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxReleaseResponseSize))
	if err := decoder.Decode(&release); err != nil {
		return UpdateInfo{}, fmt.Errorf("decode update response: %w", err)
	}
	latestVersion, err := canonicalVersion(release.TagName)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("invalid release version %q: %w", release.TagName, err)
	}
	if !isTrustedGitHubURL(release.HTMLURL) {
		return UpdateInfo{}, errors.New("update response contains an untrusted release URL")
	}

	asset, hasAsset := selectReleaseAssetDetails(release.Assets, runtime.GOOS, runtime.GOARCH)
	if hasAsset && !isTrustedUpdateTransportURL(asset.BrowserDownloadURL) {
		return UpdateInfo{}, errors.New("update response contains an untrusted download URL")
	}
	checksumAsset, hasChecksumAsset := selectChecksumAsset(release.Assets)
	if hasChecksumAsset && !isTrustedUpdateTransportURL(checksumAsset.BrowserDownloadURL) {
		return UpdateInfo{}, errors.New("update response contains an untrusted checksum URL")
	}
	installerKind := installerKindForAsset(asset.Name, runtime.GOOS)
	checksumAvailable := validSHA256Digest(asset.Digest) || hasChecksumAsset
	info := UpdateInfo{
		CurrentVersion:    appVersion,
		LatestVersion:     latestVersion,
		UpdateAvailable:   compareVersions(latestVersion, appVersion) > 0,
		ReleaseURL:        release.HTMLURL,
		DownloadURL:       asset.BrowserDownloadURL,
		PublishedAt:       release.PublishedAt,
		AssetName:         asset.Name,
		AssetSize:         asset.Size,
		Installable:       hasAsset && installerKind != "",
		ChecksumAvailable: checksumAvailable,
		InstallerKind:     installerKind,
	}

	a.mu.Lock()
	a.latestUpdate = info
	a.updateAsset = asset
	if hasChecksumAsset {
		a.checksumAsset = checksumAsset
	} else {
		a.checksumAsset = githubReleaseAsset{}
	}
	// A new check invalidates any installer prepared for an older release.
	if a.downloadedUpdate.Version != info.LatestVersion || a.downloadedUpdate.AssetName != info.AssetName {
		a.downloadedUpdate = downloadedUpdate{}
		a.updateLaunching = false
	}
	a.mu.Unlock()
	return info, nil
}

func (a *App) OpenUpdatePage() error {
	a.mu.RLock()
	update := a.latestUpdate
	ctx := a.ctx
	a.mu.RUnlock()
	target := sourceRepositoryURL + "/releases/latest"
	if update.UpdateAvailable && update.DownloadURL != "" {
		target = update.DownloadURL
	} else if update.ReleaseURL != "" {
		target = update.ReleaseURL
	}
	if !isTrustedGitHubURL(target) {
		return errors.New("refusing to open an untrusted update URL")
	}
	if ctx == nil {
		return errors.New("application is not ready")
	}
	wailsruntime.BrowserOpenURL(ctx, target)
	return nil
}

func (a *App) OpenSourceRepository() error {
	ctx := a.currentContext()
	if ctx == nil {
		return errors.New("application is not ready")
	}
	wailsruntime.BrowserOpenURL(ctx, sourceRepositoryURL)
	return nil
}

func isTrustedGitHubURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com")
}

func isTrustedUpdateTransportURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "github.com" ||
		host == "api.github.com" ||
		host == "objects.githubusercontent.com" ||
		host == "release-assets.githubusercontent.com" ||
		strings.HasSuffix(host, ".githubusercontent.com")
}

func canonicalVersion(value string) (string, error) {
	parsed, err := parseSemanticVersion(value)
	if err != nil {
		return "", err
	}
	return parsed.original, nil
}

type semanticVersion struct {
	original   string
	numbers    []int
	prerelease []string
}

func parseSemanticVersion(value string) (semanticVersion, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.TrimPrefix(value, "v"), "V")
	if value == "" {
		return semanticVersion{}, errors.New("empty version")
	}
	withoutBuild := strings.SplitN(value, "+", 2)[0]
	parts := strings.SplitN(withoutBuild, "-", 2)
	numberParts := strings.Split(parts[0], ".")
	if len(numberParts) == 0 {
		return semanticVersion{}, errors.New("missing version numbers")
	}
	numbers := make([]int, 0, len(numberParts))
	for _, part := range numberParts {
		if part == "" {
			return semanticVersion{}, errors.New("empty numeric component")
		}
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return semanticVersion{}, fmt.Errorf("invalid numeric component %q", part)
		}
		numbers = append(numbers, number)
	}
	for len(numbers) < 3 {
		numbers = append(numbers, 0)
	}
	parsed := semanticVersion{original: value, numbers: numbers}
	if len(parts) == 2 {
		if parts[1] == "" {
			return semanticVersion{}, errors.New("empty prerelease")
		}
		parsed.prerelease = strings.Split(parts[1], ".")
	}
	return parsed, nil
}

func compareVersions(left string, right string) int {
	leftVersion, leftErr := parseSemanticVersion(left)
	rightVersion, rightErr := parseSemanticVersion(right)
	if leftErr != nil || rightErr != nil {
		return strings.Compare(strings.ToLower(left), strings.ToLower(right))
	}
	length := len(leftVersion.numbers)
	if len(rightVersion.numbers) > length {
		length = len(rightVersion.numbers)
	}
	for index := 0; index < length; index++ {
		leftNumber, rightNumber := 0, 0
		if index < len(leftVersion.numbers) {
			leftNumber = leftVersion.numbers[index]
		}
		if index < len(rightVersion.numbers) {
			rightNumber = rightVersion.numbers[index]
		}
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
	}
	return comparePrerelease(leftVersion.prerelease, rightVersion.prerelease)
}

func comparePrerelease(left []string, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}
	length := len(left)
	if len(right) > length {
		length = len(right)
	}
	for index := 0; index < length; index++ {
		if index >= len(left) {
			return -1
		}
		if index >= len(right) {
			return 1
		}
		leftNumber, leftErr := strconv.Atoi(left[index])
		rightNumber, rightErr := strconv.Atoi(right[index])
		switch {
		case leftErr == nil && rightErr == nil:
			if leftNumber < rightNumber {
				return -1
			}
			if leftNumber > rightNumber {
				return 1
			}
		case leftErr == nil:
			return -1
		case rightErr == nil:
			return 1
		default:
			if comparison := strings.Compare(left[index], right[index]); comparison != 0 {
				return comparison
			}
		}
	}
	return 0
}

func selectReleaseAsset(assets []githubReleaseAsset, operatingSystem string, architecture string) string {
	asset, ok := selectReleaseAssetDetails(assets, operatingSystem, architecture)
	if !ok {
		return ""
	}
	return asset.BrowserDownloadURL
}

func selectReleaseAssetDetails(assets []githubReleaseAsset, operatingSystem string, architecture string) (githubReleaseAsset, bool) {
	bestScore := -1
	bestAsset := githubReleaseAsset{}
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		score := releaseAssetScore(name, operatingSystem, architecture)
		if score > bestScore {
			bestScore = score
			bestAsset = asset
		}
	}
	if bestScore < 0 {
		return githubReleaseAsset{}, false
	}
	return bestAsset, true
}

func releaseAssetScore(name string, operatingSystem string, architecture string) int {
	score := 0
	switch operatingSystem {
	case "windows":
		if !strings.HasSuffix(name, ".exe") && !strings.HasSuffix(name, ".msi") {
			return -1
		}
		if !strings.Contains(name, "setup") && !strings.Contains(name, "installer") {
			return -1
		}
		score += 40
		score += 15
	case "darwin":
		if !strings.HasSuffix(name, ".pkg") && !strings.HasSuffix(name, ".dmg") && !strings.HasSuffix(name, ".zip") {
			return -1
		}
		if !strings.Contains(name, "mac") && !strings.Contains(name, "darwin") {
			return -1
		}
		score += 40
		switch {
		case strings.HasSuffix(name, ".pkg"):
			score += 35
		case strings.HasSuffix(name, ".dmg"):
			score += 20
		case strings.HasSuffix(name, ".zip"):
			score += 5
		}
	case "linux":
		if !strings.Contains(name, "linux") {
			return -1
		}
		if !(strings.HasSuffix(name, ".appimage") || strings.HasSuffix(name, ".deb") || strings.HasSuffix(name, ".rpm") || strings.HasSuffix(name, ".tar.gz")) {
			return -1
		}
		score += 40
	default:
		return -1
	}

	if strings.Contains(name, "universal") || strings.Contains(name, "all") {
		return score + 20
	}
	architectureTokens := map[string][]string{
		"amd64": {"amd64", "x64", "x86_64"},
		"arm64": {"arm64", "aarch64"},
	}
	matchedArchitecture := false
	containsOtherArchitecture := false
	for candidate, tokens := range architectureTokens {
		for _, token := range tokens {
			if strings.Contains(name, token) {
				if candidate == architecture {
					matchedArchitecture = true
				} else {
					containsOtherArchitecture = true
				}
			}
		}
	}
	if containsOtherArchitecture && !matchedArchitecture {
		return -1
	}
	if matchedArchitecture {
		score += 10
	}
	return score
}

func installerKindForAsset(name string, operatingSystem string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch operatingSystem {
	case "windows":
		if (strings.HasSuffix(name, ".exe") || strings.HasSuffix(name, ".msi")) &&
			(strings.Contains(name, "setup") || strings.Contains(name, "installer")) {
			return "windows-installer"
		}
	case "darwin":
		if strings.HasSuffix(name, ".pkg") {
			return "macos-package"
		}
		if strings.HasSuffix(name, ".dmg") {
			return "macos-disk-image"
		}
	case "linux":
		if strings.HasSuffix(name, ".appimage") || strings.HasSuffix(name, ".deb") || strings.HasSuffix(name, ".rpm") {
			return "linux-installer"
		}
	}
	return ""
}

func selectChecksumAsset(assets []githubReleaseAsset) (githubReleaseAsset, bool) {
	for _, asset := range assets {
		name := strings.ToLower(strings.TrimSpace(asset.Name))
		if name == "sha256sums" || name == "sha256sums.txt" || name == "checksums-sha256.txt" {
			return asset, true
		}
	}
	return githubReleaseAsset{}, false
}
