package main

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	maxRecentItems                = 10
	maxPendingRecentActivations   = 32
	invalidRecentItemErrorMessage = "最近项目已失效，请从文件菜单重新打开"
)

// RecentItem is persisted without ID. A new random runtime-only ID is minted
// on each application launch so a native-menu action never exposes a path or
// turns an old path string into an ambient file-read capability.
type RecentItem struct {
	ID   string `json:"-"`
	Kind string `json:"kind"`
	Path string `json:"path"`
	Name string `json:"name"`
}

type RecentMenuEvent struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

func (a *App) recordRecentItem(kind string, itemPath string) {
	item, ok := makeRecentItem(kind, itemPath)
	if !ok {
		return
	}
	a.settingsWriteMu.Lock()
	a.mu.Lock()
	a.recentItems = prependRecentItem(a.recentItems, item)
	state, settingsPath := a.settingsSnapshotLocked()
	a.mu.Unlock()
	_ = saveSettingsState(settingsPath, state)
	a.settingsWriteMu.Unlock()
	// Persistence is best-effort for an already-open document, but the native
	// menu is synchronously rebuilt before the opening method returns.
	a.refreshApplicationMenu()
}

func (a *App) ClearRecentItems() error {
	a.settingsWriteMu.Lock()
	a.mu.Lock()
	previous := append([]RecentItem(nil), a.recentItems...)
	a.recentItems = nil
	state, settingsPath := a.settingsSnapshotLocked()
	a.mu.Unlock()
	err := saveSettingsState(settingsPath, state)
	if err != nil {
		a.mu.Lock()
		a.recentItems = previous
		a.mu.Unlock()
	}
	a.settingsWriteMu.Unlock()
	a.refreshApplicationMenu()
	return err
}

func (a *App) recentItemsSnapshot() []RecentItem {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]RecentItem(nil), a.recentItems...)
}

func (a *App) recentItemByID(id string, kind string) (RecentItem, error) {
	id = strings.TrimSpace(id)
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, item := range a.recentItems {
		if item.ID == id && item.Kind == kind {
			return item, nil
		}
	}
	return RecentItem{}, errors.New(invalidRecentItemErrorMessage)
}

func makeRecentItem(kind string, itemPath string) (RecentItem, bool) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "file" && kind != "directory" {
		return RecentItem{}, false
	}
	if strings.TrimSpace(itemPath) == "" || strings.ContainsRune(itemPath, 0) {
		return RecentItem{}, false
	}
	absolute, err := filepath.Abs(itemPath)
	if err != nil {
		return RecentItem{}, false
	}
	absolute = filepath.Clean(absolute)
	name := filepath.Base(absolute)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = absolute
	}
	id, err := newOpaqueID()
	if err != nil {
		return RecentItem{}, false
	}
	return RecentItem{ID: id, Kind: kind, Path: absolute, Name: name}, true
}

func normalizeLoadedRecentItems(items []RecentItem) []RecentItem {
	result := make([]RecentItem, 0, min(len(items), maxRecentItems))
	for _, saved := range items {
		if len(result) == maxRecentItems {
			break
		}
		// Do not Stat here. Missing or disconnected paths should not make app
		// startup depend on slow network shares or removable drives.
		if !filepath.IsAbs(saved.Path) {
			continue
		}
		item, ok := makeRecentItem(saved.Kind, saved.Path)
		if !ok || containsRecentPath(result, item) {
			continue
		}
		result = append(result, item)
	}
	return result
}

func prependRecentItem(items []RecentItem, item RecentItem) []RecentItem {
	for _, existing := range items {
		if sameRecentPath(existing, item) {
			item.ID = existing.ID
			break
		}
	}
	result := make([]RecentItem, 0, min(len(items)+1, maxRecentItems))
	result = append(result, item)
	for _, existing := range items {
		if sameRecentPath(existing, item) || containsRecentPath(result, existing) {
			continue
		}
		result = append(result, existing)
		if len(result) == maxRecentItems {
			break
		}
	}
	return result
}

func containsRecentPath(items []RecentItem, candidate RecentItem) bool {
	for _, existing := range items {
		if sameRecentPath(existing, candidate) {
			return true
		}
	}
	return false
}

func sameRecentPath(left RecentItem, right RecentItem) bool {
	leftPath := filepath.Clean(left.Path)
	rightPath := filepath.Clean(right.Path)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(leftPath, rightPath)
	}
	return leftPath == rightPath
}

func (a *App) removeRecentItemByID(id string) {
	a.settingsWriteMu.Lock()
	a.mu.Lock()
	filtered := make([]RecentItem, 0, len(a.recentItems))
	removed := false
	for _, existing := range a.recentItems {
		if existing.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if removed {
		a.recentItems = filtered
	}
	state, settingsPath := a.settingsSnapshotLocked()
	a.mu.Unlock()
	if removed {
		_ = saveSettingsState(settingsPath, state)
	}
	a.settingsWriteMu.Unlock()
	if removed {
		a.refreshApplicationMenu()
	}
}

func (a *App) newRecentDocumentActivation(path string) (string, error) {
	id, err := newOpaqueID()
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	if a.pendingRecentDocuments == nil {
		a.pendingRecentDocuments = make(map[string]string)
	}
	if len(a.pendingRecentDocuments) >= maxPendingRecentActivations {
		for staleID := range a.pendingRecentDocuments {
			delete(a.pendingRecentDocuments, staleID)
			break
		}
	}
	a.pendingRecentDocuments[id] = path
	a.mu.Unlock()
	return id, nil
}

// ActivateRecentDocument consumes the one-time token attached to an OS-opened
// document. The frontend calls this only after its unsaved-document guard has
// accepted and installed that document.
func (a *App) ActivateRecentDocument(id string) error {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	path, ok := a.pendingRecentDocuments[id]
	if ok {
		delete(a.pendingRecentDocuments, id)
	}
	a.mu.Unlock()
	if !ok {
		return errors.New("文档激活标识无效或已使用")
	}
	a.recordRecentItem("file", path)
	return nil
}
