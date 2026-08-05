package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const maxSettingsSize = 256 << 10

type settingsState struct {
	LanguageState
	RecentItems []RecentItem `json:"recentItems,omitempty"`
}

func currentPlatform() string {
	return goruntime.GOOS
}

func normalizeLanguageMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "zh-CN", "en":
		return strings.TrimSpace(mode)
	default:
		return "auto"
	}
}

func normalizeLocale(locale string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh") {
		return "zh-CN"
	}
	return "en"
}

func defaultSettingsPath() string {
	directory, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(directory) == "" {
		return ""
	}
	return filepath.Join(directory, "InkMark", "settings.json")
}

func loadSettingsState(path string) settingsState {
	state := settingsState{LanguageState: LanguageState{Mode: "auto", Locale: localeFromEnvironment()}}
	if strings.TrimSpace(path) == "" {
		return state
	}
	file, err := openReadOnlyNonBlocking(path)
	if err != nil {
		return state
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSettingsSize {
		return state
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSettingsSize+1))
	if err != nil || len(data) > maxSettingsSize {
		return state
	}
	saved := state
	if json.Unmarshal(data, &saved) != nil {
		return state
	}
	saved.LanguageState.Mode = normalizeLanguageMode(saved.LanguageState.Mode)
	saved.LanguageState.Locale = normalizeLocale(saved.LanguageState.Locale)
	if saved.LanguageState.Mode == "zh-CN" || saved.LanguageState.Mode == "en" {
		saved.LanguageState.Locale = saved.LanguageState.Mode
	}
	saved.RecentItems = normalizeLoadedRecentItems(saved.RecentItems)
	return saved
}

func loadLanguageState(path string) LanguageState {
	return loadSettingsState(path).LanguageState
}

func localeFromEnvironment() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return normalizeLocale(value)
		}
	}
	return "en"
}

func saveSettingsState(path string, state settingsState) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "settings-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err = temporary.Write(payload); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	if directory, err := os.Open(filepath.Dir(path)); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

// saveLanguageState is kept for compatibility with older tests and internal
// callers. It preserves recent items already present in the unified file.
func saveLanguageState(path string, language LanguageState) error {
	state := loadSettingsState(path)
	state.LanguageState = language
	return saveSettingsState(path, state)
}

func (a *App) persistSettings() error {
	a.settingsWriteMu.Lock()
	defer a.settingsWriteMu.Unlock()
	a.mu.RLock()
	state, path := a.settingsSnapshotLocked()
	a.mu.RUnlock()
	if err := saveSettingsState(path, state); err != nil {
		return fmt.Errorf("保存设置失败: %w", err)
	}
	return nil
}

// settingsSnapshotLocked requires a.mu to be held for reading or writing.
func (a *App) settingsSnapshotLocked() (settingsState, string) {
	return settingsState{
		LanguageState: a.language,
		RecentItems:   append([]RecentItem(nil), a.recentItems...),
	}, a.settingsPath
}

func (a *App) GetLanguageSettings() LanguageState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.language
}

func (a *App) SetLanguage(mode string, locale string) (LanguageState, error) {
	mode = normalizeLanguageMode(mode)
	locale = normalizeLocale(locale)
	if mode == "zh-CN" || mode == "en" {
		locale = mode
	}
	state := LanguageState{Mode: mode, Locale: locale}

	a.mu.Lock()
	a.language = state
	a.mu.Unlock()

	if err := a.persistSettings(); err != nil {
		return state, err
	}
	a.refreshApplicationMenu()
	return state, nil
}

func (a *App) currentLocale() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return normalizeLocale(a.language.Locale)
}

func (a *App) currentContext() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.ctx
}

func (a *App) UpdateMenuState(locale string, viewMode string, theme string, syncScroll bool, previewFirst bool) {
	locale = normalizeLocale(locale)
	switch viewMode {
	case "edit", "split", "preview":
	default:
		viewMode = "split"
	}
	switch theme {
	case "github", "clean", "wechat", "dark":
	default:
		theme = "github"
	}

	a.mu.Lock()
	a.language.Locale = locale
	a.menuState = MenuState{ViewMode: viewMode, Theme: theme, SyncScroll: syncScroll, PreviewFirst: previewFirst}
	a.mu.Unlock()
	a.refreshApplicationMenu()
}

func (a *App) refreshApplicationMenu() {
	a.menuRefreshMu.Lock()
	defer a.menuRefreshMu.Unlock()
	ctx := a.currentContext()
	if ctx == nil {
		return
	}
	runtime.MenuSetApplicationMenu(ctx, a.applicationMenuFor(currentPlatform(), a.currentLocale()))
}

func (a *App) currentMenuState() MenuState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	state := a.menuState
	if state.ViewMode == "" {
		state.ViewMode = "split"
	}
	if state.Theme == "" {
		state.Theme = "github"
	}
	// A zero-value App is used by tests. Preserve the product default there.
	if a.menuState.ViewMode == "" && a.menuState.Theme == "" {
		state.SyncScroll = true
	}
	return state
}

func resolveDocumentArgument(args []string, workingDirectory string) string {
	for _, argument := range args {
		candidate := strings.TrimSpace(strings.Trim(argument, "\""))
		if candidate == "" || strings.HasPrefix(candidate, "-") {
			continue
		}
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(workingDirectory, candidate)
		}
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(absolute)
		if err == nil && !info.IsDir() {
			return absolute
		}
	}
	return ""
}

func (a *App) openFileFromOS(path string) {
	a.queueOrOpenDocument(path)
}

func (a *App) handleSecondInstance(args []string, workingDirectory string) {
	if path := resolveDocumentArgument(args, workingDirectory); path != "" {
		a.queueOrOpenDocument(path)
	}
	if ctx := a.currentContext(); ctx != nil {
		runtime.WindowShow(ctx)
		runtime.WindowUnminimise(ctx)
	}
}

func (a *App) queueOrOpenDocument(path string) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return
	}

	a.mu.Lock()
	if !a.initialLoaded {
		a.initPath = absolute
		a.mu.Unlock()
		return
	}
	ctx := a.ctx
	a.mu.Unlock()

	if ctx == nil {
		return
	}
	document, err := readDocument(absolute)
	if err != nil {
		runtime.EventsEmit(ctx, openErrorEvent, err.Error())
		return
	}
	activationID, err := a.newRecentDocumentActivation(document.Path)
	if err != nil {
		runtime.EventsEmit(ctx, openErrorEvent, "创建文档激活标识失败")
		return
	}
	document.ActivationID = activationID
	runtime.EventsEmit(ctx, openDocumentEvent, document)
	runtime.WindowShow(ctx)
	runtime.WindowUnminimise(ctx)
}
