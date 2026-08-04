package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

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

func loadLanguageState(path string) LanguageState {
	state := LanguageState{Mode: "auto", Locale: localeFromEnvironment()}
	if strings.TrimSpace(path) == "" {
		return state
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state
	}
	var saved LanguageState
	if json.Unmarshal(data, &saved) != nil {
		return state
	}
	saved.Mode = normalizeLanguageMode(saved.Mode)
	saved.Locale = normalizeLocale(saved.Locale)
	if saved.Mode == "zh-CN" || saved.Mode == "en" {
		saved.Locale = saved.Mode
	}
	return saved
}

func localeFromEnvironment() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return normalizeLocale(value)
		}
	}
	return "en"
}

func saveLanguageState(path string, state LanguageState) error {
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
	if err = temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
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
	settingsPath := a.settingsPath
	ctx := a.ctx
	a.mu.Unlock()

	if err := saveLanguageState(settingsPath, state); err != nil {
		return state, err
	}
	if ctx != nil {
		runtime.MenuSetApplicationMenu(ctx, a.applicationMenuFor(currentPlatform(), state.Locale))
	}
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
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		runtime.MenuSetApplicationMenu(ctx, a.applicationMenuFor(currentPlatform(), locale))
	}
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
	runtime.EventsEmit(ctx, openDocumentEvent, document)
	runtime.WindowShow(ctx)
	runtime.WindowUnminimise(ctx)
}
