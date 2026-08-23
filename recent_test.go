package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRecentItemsAreDeduplicatedPromotedAndBounded(t *testing.T) {
	items := []RecentItem{}
	for index := 0; index < maxRecentItems+4; index++ {
		item, ok := makeRecentItem("file", filepath.Join(t.TempDir(), fmt.Sprintf("%02d.md", index)))
		if !ok {
			t.Fatal("failed to create recent item")
		}
		items = prependRecentItem(items, item)
	}
	if len(items) != maxRecentItems {
		t.Fatalf("expected %d recent items, got %d", maxRecentItems, len(items))
	}
	promoted := items[len(items)-1]
	replacement, ok := makeRecentItem("directory", promoted.Path)
	if !ok {
		t.Fatal("failed to create replacement item")
	}
	items = prependRecentItem(items, replacement)
	if items[0].Path != promoted.Path || items[0].Kind != "directory" || len(items) != maxRecentItems {
		t.Fatalf("existing path was not replaced and promoted: %#v", items)
	}
	for _, item := range items[1:] {
		if item.Path == promoted.Path {
			t.Fatalf("recent path was duplicated: %#v", items)
		}
	}
}

func TestRecentWebDAVEndpointsAreCanonicalSafeAndDeduplicated(t *testing.T) {
	item, ok := makeRecentItem("webdav", "  https://EXAMPLE.com:443/netdisk/api/webdav  ")
	if !ok {
		t.Fatal("valid WebDAV endpoint was rejected")
	}
	if item.Kind != "webdav" || item.Path != "https://example.com/netdisk/api/webdav/" || item.Name != "example.com" || item.ID == "" {
		t.Fatalf("unexpected canonical WebDAV recent item: %#v", item)
	}
	loopback, ok := makeRecentItem("webdav", "http://127.0.0.1:8080/webdav")
	if !ok || loopback.Path != "http://127.0.0.1:8080/webdav/" {
		t.Fatalf("supported loopback HTTP endpoint was rejected: %#v", loopback)
	}

	for _, unsafe := range []string{
		"http://example.com/webdav/",
		"ftp://example.com/webdav/",
		"https://user:password@example.com/webdav/",
		"https://example.com/webdav/?token=secret",
		"https://example.com/webdav/#fragment",
		"https://example.com/webdav/%2fescape/",
		"https://example.com/webdav/../escape/",
	} {
		if recent, accepted := makeRecentItem("webdav", unsafe); accepted {
			t.Errorf("unsafe WebDAV endpoint entered recent items: %q => %#v", unsafe, recent)
		}
	}

	duplicate, ok := makeRecentItem("webdav", "https://example.COM/netdisk/api/webdav/")
	if !ok {
		t.Fatal("canonical duplicate endpoint was rejected")
	}
	items := prependRecentItem([]RecentItem{item}, duplicate)
	if len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("canonical WebDAV endpoint was not deduplicated: %#v", items)
	}
}

func TestRecentWebDAVSharesGlobalMRUBound(t *testing.T) {
	items := make([]RecentItem, 0, maxRecentItems)
	for index := 0; index < maxRecentItems-1; index++ {
		item, ok := makeRecentItem("file", filepath.Join(t.TempDir(), fmt.Sprintf("%02d.md", index)))
		if !ok {
			t.Fatal("failed to create local recent fixture")
		}
		items = prependRecentItem(items, item)
	}
	remote, ok := makeRecentItem("webdav", "https://example.com/dav/")
	if !ok {
		t.Fatal("failed to create WebDAV recent fixture")
	}
	items = prependRecentItem(items, remote)
	extra, ok := makeRecentItem("directory", t.TempDir())
	if !ok {
		t.Fatal("failed to create directory recent fixture")
	}
	items = prependRecentItem(items, extra)
	if len(items) != maxRecentItems || items[0].Kind != "directory" {
		t.Fatalf("mixed recent items did not share the global bound: %#v", items)
	}
}

func TestOpenRecentWebDAVUsesOpaqueIDAndReturnsNoCredentials(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	app := &App{
		language:     LanguageState{Mode: "auto", Locale: "en"},
		settingsPath: settingsPath,
	}
	app.recordRecentItem("webdav", "https://example.com/netdisk/api/webdav/")
	item := app.recentItemsSnapshot()[0]

	if _, err := app.OpenRecentWebDAV(item.Path); err == nil {
		t.Fatal("an endpoint string must not be accepted as a recent capability")
	}
	connection, err := app.OpenRecentWebDAV(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if connection.Endpoint != item.Path || connection.Name != "example.com" {
		t.Fatalf("unexpected recent WebDAV connection: %#v", connection)
	}
	payload, err := json.Marshal(connection)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"username", "password", "token", "userinfo", "query", "fragment"} {
		if bytes.Contains(bytes.ToLower(payload), []byte(forbidden)) {
			t.Fatalf("recent WebDAV response exposed %q: %s", forbidden, payload)
		}
	}
	promoted := app.recentItemsSnapshot()[0]
	if promoted.ID != item.ID || promoted.Path != item.Path {
		t.Fatalf("opening recent WebDAV did not preserve its opaque capability: %#v", promoted)
	}
}

func TestLoadedWebDAVRecentItemsIgnoreForgedMetadataAndUnsafeURLs(t *testing.T) {
	items := normalizeLoadedRecentItems([]RecentItem{
		{ID: "attacker-controlled", Kind: "webdav", Path: "https://user:password@example.com/dav/?token=secret#fragment", Name: "forged"},
		{ID: "also-forged", Kind: "webdav", Path: "https://EXAMPLE.com:443/dav", Name: "credential-shaped-name"},
	})
	if len(items) != 1 {
		t.Fatalf("unexpected normalized WebDAV recent items: %#v", items)
	}
	if items[0].ID == "" || items[0].ID == "also-forged" || items[0].Path != "https://example.com/dav/" || items[0].Name != "example.com" {
		t.Fatalf("loaded WebDAV metadata was trusted: %#v", items[0])
	}
}

func TestUnifiedSettingsPreserveLanguageAndRecentItems(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "InkMark", "settings.json")
	filePath := filepath.Join(t.TempDir(), "missing is retained.md")
	app := &App{
		language:     LanguageState{Mode: "auto", Locale: "en"},
		settingsPath: settingsPath,
	}
	app.recordRecentItem("file", filePath)
	if _, err := app.SetLanguage("zh-CN", "en"); err != nil {
		t.Fatal(err)
	}
	loaded := loadSettingsState(settingsPath)
	if loaded.Mode != "zh-CN" || loaded.Locale != "zh-CN" {
		t.Fatalf("language did not survive unified persistence: %#v", loaded.LanguageState)
	}
	if len(loaded.RecentItems) != 1 || loaded.RecentItems[0].Path != filePath || loaded.RecentItems[0].Kind != "file" {
		t.Fatalf("recent item did not survive unified persistence: %#v", loaded.RecentItems)
	}
	if loaded.RecentItems[0].ID == "" || loaded.RecentItems[0].ID == app.recentItemsSnapshot()[0].ID {
		t.Fatal("recent runtime IDs must be regenerated when settings are loaded")
	}
	payload, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte(`"id"`)) || bytes.Contains(payload, []byte(app.recentItemsSnapshot()[0].ID)) {
		t.Fatalf("runtime recent ID leaked into settings: %s", payload)
	}
	// Loading settings intentionally does not Stat the recent target.
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("test recent path unexpectedly exists: %v", err)
	}
}

func TestLegacyLanguageOnlySettingsRemainCompatible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"mode":"en","locale":"zh-CN"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := loadSettingsState(path)
	if loaded.Mode != "en" || loaded.Locale != "en" || len(loaded.RecentItems) != 0 {
		t.Fatalf("legacy settings were not normalized: %#v", loaded)
	}
}

func TestLastPageSettingsRejectUntrustedOrUnsupportedLocators(t *testing.T) {
	directory := t.TempDir()
	valid := filepath.Join(directory, "valid.md")
	path := filepath.Join(directory, "settings.json")
	for _, payload := range []string{
		`{"lastPage":{"version":99,"kind":"local","path":"` + filepath.ToSlash(valid) + `"}}`,
		`{"lastPage":{"version":1,"kind":"local","path":"relative.md"}}`,
		`{"lastPage":{"version":1,"kind":"local","path":"` + filepath.ToSlash(filepath.Join(directory, "not-markdown.txt")) + `"}}`,
		`{"lastPage":{"version":1,"kind":"webdav","path":"https://example.test/secret.md"}}`,
		`{"lastPage":{"version":1,"kind":"builtin","builtIn":"attacker-page"}}`,
	} {
		if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		if loaded := loadSettingsState(path); loaded.LastPage != (LastPageState{}) {
			t.Fatalf("unsafe last page survived normalization: %#v", loaded.LastPage)
		}
	}
	if err := os.WriteFile(path, []byte(`{"lastPage":{"version":1,"kind":"local","path":"`+filepath.ToSlash(valid)+`"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if loaded := loadSettingsState(path); loaded.LastPage.Kind != "local" || loaded.LastPage.Path != valid {
		t.Fatalf("valid last page was rejected: %#v", loaded.LastPage)
	}
}

func TestPersistedRecentIDIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	payload := `{"mode":"auto","locale":"en","recentItems":[{"id":"attacker-controlled","kind":"file","path":"` + filepath.ToSlash(filepath.Join(t.TempDir(), "missing.md")) + `","name":"forged"}]}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := loadSettingsState(path)
	if len(loaded.RecentItems) != 1 || loaded.RecentItems[0].ID == "" || loaded.RecentItems[0].ID == "attacker-controlled" {
		t.Fatalf("persisted recent ID was trusted: %#v", loaded.RecentItems)
	}
}

func TestOversizedSettingsAreIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, make([]byte, maxSettingsSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := loadSettingsState(path)
	if loaded.Mode != "auto" || len(loaded.RecentItems) != 0 {
		t.Fatalf("oversized settings must fall back to defaults: %#v", loaded)
	}
}

func TestRecentSettingsWritesAreSerializedAndAtomic(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "InkMark", "settings.json")
	app := &App{
		language:     LanguageState{Mode: "auto", Locale: "en"},
		settingsPath: settingsPath,
	}
	recentRoot := t.TempDir()
	var group sync.WaitGroup
	for index := 0; index < 40; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			app.recordRecentItem("file", filepath.Join(recentRoot, fmt.Sprintf("concurrent-%02d.md", index)))
		}(index)
	}
	group.Wait()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded settingsState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("concurrent settings write left invalid JSON: %v", err)
	}
	loaded := loadSettingsState(settingsPath)
	if len(loaded.RecentItems) != maxRecentItems {
		t.Fatalf("expected bounded recent settings, got %d", len(loaded.RecentItems))
	}
	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(settingsPath), "settings-*.json")); err != nil || len(matches) != 0 {
		t.Fatalf("temporary settings files were left behind: %q, %v", matches, err)
	}
}

func TestRecentPersistenceFailureDoesNotUndoMRU(t *testing.T) {
	notDirectory := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notDirectory, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	app := &App{
		language:     LanguageState{Mode: "auto", Locale: "en"},
		settingsPath: filepath.Join(notDirectory, "settings.json"),
	}
	app.recordRecentItem("file", filepath.Join(t.TempDir(), "draft.md"))
	if len(app.recentItemsSnapshot()) != 1 {
		t.Fatal("metadata write failure must not undo an activated recent item")
	}
}

func TestClearRecentItemsPersists(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	app := &App{
		language:     LanguageState{Mode: "auto", Locale: "en"},
		settingsPath: settingsPath,
	}
	app.recordRecentItem("directory", t.TempDir())
	if err := app.ClearRecentItems(); err != nil {
		t.Fatal(err)
	}
	if len(app.recentItemsSnapshot()) != 0 || len(loadSettingsState(settingsPath).RecentItems) != 0 {
		t.Fatal("recent items were not cleared in memory and settings")
	}
}

func TestClearRecentItemsReportsPersistenceFailureAndRestoresMenuState(t *testing.T) {
	notDirectory := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notDirectory, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	app := &App{
		language:     LanguageState{Mode: "auto", Locale: "en"},
		settingsPath: filepath.Join(notDirectory, "settings.json"),
	}
	app.recordRecentItem("file", filepath.Join(t.TempDir(), "draft.md"))
	if err := app.ClearRecentItems(); err == nil {
		t.Fatal("expected clear persistence failure")
	}
	if len(app.recentItemsSnapshot()) != 1 {
		t.Fatal("failed clear must restore the in-memory recent menu")
	}
}

func TestFailedRecentOpenRemovesStaleEntry(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	missing := filepath.Join(t.TempDir(), "gone.md")
	app := &App{
		language:     LanguageState{Mode: "auto", Locale: "en"},
		settingsPath: settingsPath,
	}
	app.recordRecentItem("file", missing)
	recentID := app.recentItemsSnapshot()[0].ID
	if _, err := app.OpenRecentFile(recentID); err == nil {
		t.Fatal("expected missing recent file to fail")
	}
	if len(app.recentItemsSnapshot()) != 0 {
		t.Fatal("failed recent item was not removed")
	}
}

func TestOpenRecentRequiresRuntimeOpaqueID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.md")
	if err := os.WriteFile(path, []byte("# recent"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	defer app.shutdown(nil)
	app.recordRecentItem("file", path)
	item := app.recentItemsSnapshot()[0]
	if _, err := app.OpenRecentFile(path); err == nil {
		t.Fatal("an arbitrary path must not be accepted as a recent capability")
	}
	document, err := app.OpenRecentFile(item.ID)
	if err != nil || document.Content != "# recent" {
		t.Fatalf("valid recent ID did not open: %#v, %v", document, err)
	}
	if _, err := app.OpenRecentDirectory(item.ID); err == nil {
		t.Fatal("recent capability kind mismatch must be rejected")
	}
}

func TestOpenRecentDirectoryRequiresRuntimeOpaqueID(t *testing.T) {
	directory := t.TempDir()
	app := &App{}
	app.recordRecentItem("directory", directory)
	item := app.recentItemsSnapshot()[0]
	if _, err := app.OpenRecentDirectory(directory); err == nil {
		t.Fatal("an arbitrary directory path must not be accepted as a recent capability")
	}
	workspace, err := app.OpenRecentDirectory(item.ID)
	if err != nil || workspace.Path != directory {
		t.Fatalf("valid recent directory ID did not open: %#v, %v", workspace, err)
	}
	defer app.shutdown(nil)
	if promoted := app.recentItemsSnapshot()[0]; promoted.ID != item.ID || promoted.Path != directory {
		t.Fatalf("recent directory capability was not preserved on promotion: %#v", promoted)
	}
}

func TestRecentMenuEventContainsNoPath(t *testing.T) {
	event := RecentMenuEvent{ID: "opaque-id", Kind: "file", Name: "README.md"}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("path")) || !bytes.Contains(payload, []byte("opaque-id")) {
		t.Fatalf("unexpected recent menu payload: %s", payload)
	}
}

func TestRecentDocumentActivationIsOneTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "os-opened.md")
	app := &App{}
	activationID, err := app.newRecentDocumentActivation(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(app.recentItemsSnapshot()) != 0 {
		t.Fatal("pending OS-open document must not enter recent items before guard acceptance")
	}
	if err := app.ActivateRecentDocument(activationID); err != nil {
		t.Fatal(err)
	}
	if len(app.recentItemsSnapshot()) != 1 || app.recentItemsSnapshot()[0].Path != path {
		t.Fatal("accepted OS-open document was not recorded")
	}
	if err := app.ActivateRecentDocument(activationID); err == nil {
		t.Fatal("activation ID must be one-time")
	}
}
