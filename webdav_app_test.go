package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestWebDAVAppBridgeWorkspaceDocumentsConflictsAndIsolation(t *testing.T) {
	server, state := newWebDAVBridgeServer(t)
	defer server.Close()
	app := &App{}
	config := WebDAVConfig{
		Endpoint: server.URL + "/netdisk/api/webdav/",
		Username: webDAVTestUsername,
		Password: webDAVTestPassword,
	}
	workspace, err := app.ConnectWebDAV(config)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.ID == "" || workspace.Provider != "webdav" || workspace.Name == "" || workspace.Path == "" {
		t.Fatalf("unexpected WebDAV workspace: %#v", workspace)
	}
	if strings.Contains(workspace.Path, webDAVTestUsername) || strings.Contains(workspace.Path, webDAVTestPassword) {
		t.Fatalf("workspace path leaked credentials: %q", workspace.Path)
	}
	if len(workspace.Entries) != 2 || workspace.Entries[0].Kind != "directory" || workspace.Entries[1].Name != "readme.md" {
		t.Fatalf("workspace did not filter and order entries: %#v", workspace.Entries)
	}
	for _, entry := range workspace.Entries {
		if entry.AbsolutePath != "" {
			t.Fatalf("remote entry exposed an absolute path: %#v", entry)
		}
	}

	nested, err := app.ListWebDAVDirectory(workspace.ID, "folder")
	if err != nil {
		t.Fatal(err)
	}
	if len(nested.Entries) != 1 || nested.Entries[0].Path != "folder/nested.md" {
		t.Fatalf("unexpected nested listing: %#v", nested)
	}
	document, err := app.OpenWebDAVFile(workspace.ID, "readme.md")
	if err != nil {
		t.Fatal(err)
	}
	if document.StorageKind != "webdav" || document.WorkspaceID != workspace.ID || document.WorkspacePath != "readme.md" || document.RemoteDocumentID == "" || document.ETag != `"v1"` {
		t.Fatalf("unexpected remote document metadata: %#v", document)
	}
	if document.Path != "readme.md" || !strings.HasSuffix(document.DisplayLocation, "/readme.md") {
		t.Fatalf("remote document location is unsafe or unstable: %#v", document)
	}
	secondDocument, err := app.OpenWebDAVFile(workspace.ID, "folder/nested.md")
	if err != nil {
		t.Fatal(err)
	}
	if secondDocument.RemoteDocumentID == document.RemoteDocumentID {
		t.Fatal("remote document capabilities were reused")
	}

	saved, err := app.SaveWebDAVFile(workspace.ID, document.RemoteDocumentID, "# saved", document.ETag)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Conflict || saved.ETag != `"v2"` || saved.Path != "readme.md" || saved.Name != "readme.md" {
		t.Fatalf("unexpected save result: %#v", saved)
	}
	conflict, err := app.SaveWebDAVFile(workspace.ID, document.RemoteDocumentID, "# force-conflict", saved.ETag)
	if err != nil || !conflict.Conflict || conflict.ETag != saved.ETag {
		t.Fatalf("412 was not returned as a stable conflict result: %#v, %v", conflict, err)
	}
	if _, err := app.SaveWebDAVFile(workspace.ID, document.RemoteDocumentID, "# ambiguous empty ETag", ""); !IsWebDAVErrorKind(err, WebDAVErrorUnsupported) {
		t.Fatalf("normal save accepted an ambiguous empty ETag: %v", err)
	}
	overwritten, err := app.OverwriteWebDAVFile(workspace.ID, document.RemoteDocumentID, "# overwritten")
	if err != nil {
		t.Fatal(err)
	}
	if overwritten.Conflict || overwritten.ETag != `"v3"` {
		t.Fatalf("explicit overwrite failed: %#v", overwritten)
	}

	secondWorkspace, err := app.ConnectWebDAV(config)
	if err != nil {
		t.Fatal(err)
	}
	if secondWorkspace.ID == workspace.ID {
		t.Fatal("workspace capabilities were reused")
	}
	if _, err := app.SaveWebDAVFile(secondWorkspace.ID, document.RemoteDocumentID, "# wrong workspace", overwritten.ETag); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("document capability crossed workspaces: %v", err)
	}
	afterSecondConnection, err := app.SaveWebDAVFile(workspace.ID, document.RemoteDocumentID, "# still active", overwritten.ETag)
	if err != nil || afterSecondConnection.ETag != `"v4"` {
		t.Fatalf("connecting another endpoint invalidated the first document: %#v, %v", afterSecondConnection, err)
	}
	if err := app.CloseWebDAVWorkspace(secondWorkspace.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ListWebDAVDirectory(secondWorkspace.ID, ""); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("closed workspace remained usable: %v", err)
	}
	if err := app.CloseWebDAVDocument(workspace.ID, secondDocument.RemoteDocumentID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveWebDAVFile(workspace.ID, secondDocument.RemoteDocumentID, "# closed document", secondDocument.ETag); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("closed document capability remained usable: %v", err)
	}
	if state.putCount() != 3 {
		t.Fatalf("unexpected number of remote writes: %d", state.putCount())
	}
	if err := app.CloseWebDAVWorkspace(workspace.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveWebDAVFile(workspace.ID, document.RemoteDocumentID, "# closed", afterSecondConnection.ETag); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("closed capability remained usable: %v", err)
	}
}

func TestSuccessfulWebDAVConnectionRecordsOnlySafeRecentEndpoint(t *testing.T) {
	server, _ := newWebDAVBridgeServer(t)
	defer server.Close()
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	app := &App{
		language:     LanguageState{Mode: "auto", Locale: "en"},
		settingsPath: settingsPath,
	}
	defer app.shutdown(nil)
	config := WebDAVConfig{
		Endpoint: server.URL + "/netdisk/api/webdav/",
		Username: webDAVTestUsername,
		Password: webDAVTestPassword,
	}
	first, err := app.ConnectWebDAV(config)
	if err != nil {
		t.Fatal(err)
	}
	recent := app.recentItemsSnapshot()
	if len(recent) != 1 || recent[0].Kind != "webdav" || recent[0].Path != config.Endpoint || recent[0].Name == "" {
		t.Fatalf("successful WebDAV connection was not recorded safely: %#v", recent)
	}
	firstRecentID := recent[0].ID
	if _, err := app.ConnectWebDAV(config); err != nil {
		t.Fatal(err)
	}
	recent = app.recentItemsSnapshot()
	if len(recent) != 1 || recent[0].ID != firstRecentID {
		t.Fatalf("repeated WebDAV connection was not deduplicated: %#v", recent)
	}
	if first.ID == "" {
		t.Fatal("first workspace capability was invalid")
	}

	payload, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{webDAVTestUsername, webDAVTestPassword, "username", "password", "token", "userinfo", "query", "fragment"} {
		if strings.Contains(strings.ToLower(string(payload)), strings.ToLower(secret)) {
			t.Fatalf("persisted WebDAV recent item leaked %q: %s", secret, payload)
		}
	}
	loaded := loadSettingsState(settingsPath)
	if len(loaded.RecentItems) != 1 || loaded.RecentItems[0].Path != config.Endpoint || loaded.RecentItems[0].ID == firstRecentID {
		t.Fatalf("persisted WebDAV recent item was not safely restored: %#v", loaded.RecentItems)
	}
}

func TestFailedWebDAVConnectionDoesNotEnterRecentItems(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(writer, "authentication required", http.StatusUnauthorized)
	}))
	defer server.Close()

	app := &App{settingsPath: filepath.Join(t.TempDir(), "settings.json")}
	if _, err := app.ConnectWebDAV(WebDAVConfig{Endpoint: server.URL + "/webdav/"}); err == nil {
		t.Fatal("connection failure was not reported")
	}
	if requests.Load() == 0 {
		t.Fatal("connection test server was not contacted")
	}
	if recent := app.recentItemsSnapshot(); len(recent) != 0 {
		t.Fatalf("failed WebDAV connection polluted recent items: %#v", recent)
	}
}

func TestWebDAVWorkspaceDirectoryTruncatesOnlyVisibleEntries(t *testing.T) {
	unsupported := make([]WebDAVEntry, maxWorkspaceDirectoryResults+1)
	for index := range unsupported {
		unsupported[index] = WebDAVEntry{Name: fmt.Sprintf("asset-%d.bin", index), Path: fmt.Sprintf("asset-%d.bin", index)}
	}
	filtered := webDAVWorkspaceDirectory(WebDAVDirectory{Entries: unsupported})
	if filtered.Truncated || len(filtered.Entries) != 0 {
		t.Fatalf("filtered resources incorrectly marked the directory as truncated: %#v", filtered)
	}

	visible := make([]WebDAVEntry, maxWorkspaceDirectoryResults+1)
	for index := range visible {
		visible[index] = WebDAVEntry{Name: fmt.Sprintf("document-%d.md", index), Path: fmt.Sprintf("document-%d.md", index)}
	}
	limited := webDAVWorkspaceDirectory(WebDAVDirectory{Entries: visible})
	if !limited.Truncated || len(limited.Entries) != maxWorkspaceDirectoryResults {
		t.Fatalf("visible resources did not respect the directory limit: %#v", limited)
	}
}

func TestWebDAVAppBridgeLimitsShutdownAndCredentialRedaction(t *testing.T) {
	config := WebDAVConfig{
		Endpoint: "https://alice:secret@example.com/netdisk/api/webdav/?token=hidden#fragment",
		Username: "alice",
		Password: "secret",
	}
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{"alice", "secret", "hidden", "token", "fragment", "username", "password"} {
		if strings.Contains(string(payload), sensitive) {
			t.Fatalf("serialized config leaked %q: %s", sensitive, payload)
		}
	}

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	appAtLimit := &App{webDAVWorkspaces: make(map[string]*webDAVCapability)}
	for index := 0; index < maxWebDAVWorkspaceCapabilities; index++ {
		id := fmt.Sprintf("capability-%d", index)
		appAtLimit.webDAVWorkspaces[id] = &webDAVCapability{id: id, closed: true}
	}
	if _, err := appAtLimit.ConnectWebDAV(WebDAVConfig{Endpoint: server.URL + "/webdav/"}); !IsWebDAVErrorKind(err, WebDAVErrorTooLarge) {
		t.Fatalf("expected workspace capability limit, got %v", err)
	} else if !strings.Contains(err.Error(), "[INKMARK_WEBDAV:too_large]") {
		t.Fatalf("bridge error did not expose a stable localization code: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("limit was checked after network access: %d requests", requests.Load())
	}
	documentClient, err := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL + "/webdav/"})
	if err != nil {
		t.Fatal(err)
	}
	documentCapability := &webDAVCapability{
		id:        "document-capability-limit",
		client:    documentClient,
		documents: make(map[string]webDAVDocumentCapability),
	}
	for index := 0; index < maxWebDAVDocumentCapabilities; index++ {
		documentCapability.documents[fmt.Sprintf("document-%d", index)] = webDAVDocumentCapability{path: fmt.Sprintf("document-%d.md", index)}
	}
	documentApp := &App{webDAVWorkspaces: map[string]*webDAVCapability{documentCapability.id: documentCapability}}
	if _, err := documentApp.OpenWebDAVFile(documentCapability.id, "another.md"); !IsWebDAVErrorKind(err, WebDAVErrorTooLarge) {
		t.Fatalf("expected document capability limit, got %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("document limit was checked after network access: %d requests", requests.Load())
	}
	documentCapability.close()

	bridgeServer, _ := newWebDAVBridgeServer(t)
	defer bridgeServer.Close()
	app := &App{}
	workspace, err := app.ConnectWebDAV(WebDAVConfig{
		Endpoint: bridgeServer.URL + "/netdisk/api/webdav/",
		Username: webDAVTestUsername,
		Password: webDAVTestPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	app.mu.RLock()
	capability := app.webDAVWorkspaces[workspace.ID]
	app.mu.RUnlock()
	capability.mu.RLock()
	client := capability.client
	capability.mu.RUnlock()
	app.shutdown(nil)
	if client.username != "" || client.password != "" {
		t.Fatal("shutdown did not clear credentials")
	}
	capability.mu.RLock()
	closed := capability.closed
	documents := capability.documents
	capability.mu.RUnlock()
	if !closed || documents != nil {
		t.Fatalf("shutdown did not revoke the WebDAV capability: closed=%t documents=%#v", closed, documents)
	}
	if _, err := app.ListWebDAVDirectory(workspace.ID, ""); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("shutdown workspace remained usable: %v", err)
	}
	invalidID := "credential-shaped-id?token=must-not-leak"
	_, err = app.OpenWebDAVFile(invalidID, "document.md")
	if err == nil || strings.Contains(err.Error(), "must-not-leak") {
		t.Fatalf("invalid capability ID leaked through error: %v", err)
	}
}

func TestLocalDocumentsExposeProviderMetadata(t *testing.T) {
	root := t.TempDir()
	file := root + "/document.md"
	if err := osWriteFileForWebDAVTest(file, []byte("# local")); err != nil {
		t.Fatal(err)
	}
	document, err := readDocument(file)
	if err != nil {
		t.Fatal(err)
	}
	if document.StorageKind != "local" || document.DisplayLocation != document.Path {
		t.Fatalf("ordinary local document metadata is incomplete: %#v", document)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	if workspace.Provider != "local" {
		t.Fatalf("local workspace provider is missing: %#v", workspace)
	}
	workspaceDocument, err := app.OpenWorkspaceFile(workspace.ID, "document.md")
	if err != nil {
		t.Fatal(err)
	}
	if workspaceDocument.WorkspaceID != workspace.ID || workspaceDocument.WorkspacePath != "document.md" || workspaceDocument.StorageKind != "local" {
		t.Fatalf("workspace document metadata is incomplete: %#v", workspaceDocument)
	}
}

type webDAVBridgeServerState struct {
	mu       sync.Mutex
	contents map[string]string
	etags    map[string]string
	puts     int
}

func (state *webDAVBridgeServerState) putCount() int {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.puts
}

func newWebDAVBridgeServer(t *testing.T) (*httptest.Server, *webDAVBridgeServerState) {
	t.Helper()
	state := &webDAVBridgeServerState{
		contents: map[string]string{
			"readme.md":        "# remote\n",
			"folder/nested.md": "# nested\n",
		},
		etags: map[string]string{
			"readme.md":        `"v1"`,
			"folder/nested.md": `"nested-v1"`,
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertWebDAVTestAuth(t, request)
		const externalRoot = "/netdisk/api/webdav/"
		if !strings.HasPrefix(request.URL.Path, externalRoot) {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		relative := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, externalRoot), "/")
		switch request.Method {
		case "PROPFIND":
			if request.Header.Get("Depth") == "0" {
				if relative == "" {
					writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
					return
				}
				state.mu.Lock()
				etag, ok := state.etags[relative]
				state.mu.Unlock()
				if !ok {
					writer.WriteHeader(http.StatusNotFound)
					return
				}
				writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/"+relative, `<d:resourcetype/><d:getetag>`+etag+`</d:getetag>`, ""))
				return
			}
			if relative == "" {
				writeWebDAVMultiStatus(writer,
					webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""),
					webDAVResponseXML("/api/webdav/folder/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""),
					webDAVResponseXML("/api/webdav/readme.md", `<d:resourcetype/><d:getetag>&quot;v1&quot;</d:getetag>`, ""),
					webDAVResponseXML("/api/webdav/ignored.txt", `<d:resourcetype/>`, ""),
				)
				return
			}
			if relative == "folder" {
				writeWebDAVMultiStatus(writer,
					webDAVResponseXML("/api/webdav/folder/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""),
					webDAVResponseXML("/api/webdav/folder/nested.md", `<d:resourcetype/><d:getetag>&quot;nested-v1&quot;</d:getetag>`, ""),
				)
				return
			}
			writer.WriteHeader(http.StatusNotFound)
		case http.MethodGet:
			state.mu.Lock()
			content, ok := state.contents[relative]
			etag := state.etags[relative]
			state.mu.Unlock()
			if !ok {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			writer.Header().Set("ETag", etag)
			_, _ = writer.Write([]byte(content))
		case http.MethodPut:
			payload, _ := io.ReadAll(request.Body)
			state.mu.Lock()
			defer state.mu.Unlock()
			if string(payload) == "# force-conflict" {
				writer.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			currentETag, ok := state.etags[relative]
			if !ok {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			if ifMatch := request.Header.Get("If-Match"); ifMatch != "" && ifMatch != currentETag {
				writer.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			state.puts++
			state.contents[relative] = string(payload)
			newETag := fmt.Sprintf(`"v%d"`, state.puts+1)
			state.etags[relative] = newETag
			writer.Header().Set("ETag", newETag)
			writer.WriteHeader(http.StatusNoContent)
		case "LOCK":
			writer.Header().Set("Lock-Token", "<opaquelocktoken:bridge-test>")
			writer.WriteHeader(http.StatusOK)
		case "UNLOCK":
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	return server, state
}

func osWriteFileForWebDAVTest(filename string, data []byte) error {
	return os.WriteFile(filename, data, 0o644)
}
