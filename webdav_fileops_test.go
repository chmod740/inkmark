package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	xwebdav "golang.org/x/net/webdav"
)

func TestWebDAVWorkspaceDirectoryKeepsMarkdownAheadOfManyImages(t *testing.T) {
	entries := make([]WebDAVEntry, 0, maxWorkspaceDirectoryResults+2)
	for index := 0; index < maxWorkspaceDirectoryResults+1; index++ {
		entries = append(entries, WebDAVEntry{Name: fmt.Sprintf("a-%04d.png", index), Path: fmt.Sprintf("a-%04d.png", index)})
	}
	entries = append(entries, WebDAVEntry{Name: "z-document.md", Path: "z-document.md"})
	directory := webDAVWorkspaceDirectory(WebDAVDirectory{Entries: entries})
	if !directory.Truncated || len(directory.Entries) != maxWorkspaceDirectoryResults {
		t.Fatalf("unexpected bounded directory: %d truncated=%t", len(directory.Entries), directory.Truncated)
	}
	if directory.Entries[0].Kind != "markdown" || directory.Entries[0].Name != "z-document.md" {
		t.Fatalf("Markdown priority was lost after image filtering: %#v", directory.Entries[:2])
	}
}

func TestLockedWebDAVMutationsFailClosedBeforeDeleteOrMove(t *testing.T) {
	tests := []struct {
		name       string
		properties string
		expected   WebDAVErrorKind
		etag       string
	}{
		{name: "file became collection", properties: `<d:resourcetype><d:collection/></d:resourcetype><d:getetag>&quot;v1&quot;</d:getetag>`, expected: WebDAVErrorConflict, etag: `"v1"`},
		{name: "missing resource type", properties: `<d:getetag>&quot;v1&quot;</d:getetag>`, expected: WebDAVErrorProtocol, etag: `"v1"`},
		{name: "stale validator", properties: `<d:resourcetype/><d:getetag>&quot;v2&quot;</d:getetag>`, expected: WebDAVErrorConflict, etag: `"v1"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, operation := range []string{"delete", "move"} {
				t.Run(operation, func(t *testing.T) {
					var mutations atomic.Int32
					var unlocksMu sync.Mutex
					var unlocks []string
					server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
						assertWebDAVTestAuth(t, request)
						switch request.Method {
						case "LOCK":
							if operation == "move" && (request.URL.Path != "/netdisk/api/webdav/" || request.Header.Get("Depth") != "infinity") {
								t.Errorf("MOVE did not lock its common parent collection: path=%q depth=%q", request.URL.Path, request.Header.Get("Depth"))
							}
							if operation == "delete" && request.Header.Get("Depth") != "0" {
								t.Errorf("file DELETE used an unexpected LOCK depth: %q", request.Header.Get("Depth"))
							}
							writer.Header().Set("Lock-Token", "<opaquelocktoken:locked-test>")
							writer.WriteHeader(http.StatusOK)
						case "PROPFIND":
							if operation == "move" && request.URL.Path == "/netdisk/api/webdav/" {
								writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
							} else {
								writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/file.md", test.properties, ""))
							}
						case "DELETE", "MOVE":
							mutations.Add(1)
							writer.WriteHeader(http.StatusNoContent)
						case "UNLOCK":
							unlocksMu.Lock()
							unlocks = append(unlocks, request.URL.Path)
							unlocksMu.Unlock()
							writer.WriteHeader(http.StatusNoContent)
						default:
							writer.WriteHeader(http.StatusMethodNotAllowed)
						}
					}))
					defer server.Close()
					client := newWebDAVTestClient(t, server.URL)
					client.advertisedRootPath = "/api/webdav/"
					client.advertisedRootKnown = true
					var err error
					expectedRevision := opaqueWebDAVRevision("file.md", "markdown", `"v1"`, "", 0, "")
					if operation == "delete" {
						err = client.DeleteResourceLocked(context.Background(), "file.md", false, expectedRevision)
					} else {
						err = client.MoveResourceLocked(context.Background(), "file.md", "renamed.md", false, expectedRevision)
					}
					if !IsWebDAVErrorKind(err, test.expected) {
						t.Fatalf("expected %s, got %v", test.expected, err)
					}
					if mutations.Load() != 0 {
						t.Fatalf("unsafe %s request was emitted", operation)
					}
					unlocksMu.Lock()
					gotUnlocks := append([]string(nil), unlocks...)
					unlocksMu.Unlock()
					wantUnlock := "/netdisk/api/webdav/file.md"
					if operation == "move" {
						wantUnlock = "/netdisk/api/webdav/"
					}
					if len(gotUnlocks) != 1 || gotUnlocks[0] != wantUnlock {
						t.Fatalf("failed mutation unlocked the wrong resource: %q", gotUnlocks)
					}
				})
			}
		})
	}
}

func TestLockedWebDAVMutationsNeverCleanupExistingResourcesOnMisreported201(t *testing.T) {
	for _, operation := range []string{"delete", "move"} {
		t.Run(operation, func(t *testing.T) {
			var mutations atomic.Int32
			var unlocksMu sync.Mutex
			var unlocks []string
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				assertWebDAVTestAuth(t, request)
				switch request.Method {
				case "LOCK":
					writer.Header().Set("Lock-Token", "<opaquelocktoken:misreported-created>")
					writer.WriteHeader(http.StatusCreated)
				case "PROPFIND":
					if operation == "move" && request.URL.Path == "/netdisk/api/webdav/folder/" {
						writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/folder/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
						return
					}
					href := "/api/webdav/file.md"
					if operation == "move" {
						href = "/api/webdav/folder/source.md"
					}
					writeWebDAVMultiStatus(writer, webDAVResponseXML(href, `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
				case "DELETE", "MOVE":
					mutations.Add(1)
					writer.WriteHeader(http.StatusNoContent)
				case "UNLOCK":
					unlocksMu.Lock()
					unlocks = append(unlocks, request.URL.Path)
					unlocksMu.Unlock()
					writer.WriteHeader(http.StatusNoContent)
				default:
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()
			client := newWebDAVTestClient(t, server.URL)
			client.advertisedRootPath = "/api/webdav/"
			client.advertisedRootKnown = true
			var err error
			wantUnlock := "/netdisk/api/webdav/file.md"
			if operation == "delete" {
				err = client.DeleteResourceLocked(context.Background(), "file.md", false, "")
			} else {
				err = client.MoveResourceLocked(context.Background(), "folder/source.md", "folder/destination.md", false, "")
				wantUnlock = "/netdisk/api/webdav/folder/"
			}
			if !IsWebDAVErrorKind(err, WebDAVErrorConflict) {
				t.Fatalf("misreported 201 was not rejected after locked type validation: %v", err)
			}
			if mutations.Load() != 0 {
				t.Fatalf("misreported 201 triggered destructive cleanup or mutation for %s", operation)
			}
			unlocksMu.Lock()
			gotUnlocks := append([]string(nil), unlocks...)
			unlocksMu.Unlock()
			if len(gotUnlocks) != 1 || gotUnlocks[0] != wantUnlock {
				t.Fatalf("misreported 201 unlocked the wrong resource: %q", gotUnlocks)
			}
		})
	}
}

func TestLockedWebDAVMoveLocksAndUnlocksCommonParent(t *testing.T) {
	for _, test := range []struct {
		name          string
		moveStatus    int
		abortMove     bool
		wantErrorKind WebDAVErrorKind
	}{
		{name: "success", moveStatus: http.StatusCreated},
		{name: "failure", moveStatus: http.StatusPreconditionFailed, wantErrorKind: WebDAVErrorConflict},
		{name: "transport unknown", abortMove: true, wantErrorKind: WebDAVErrorNetwork},
	} {
		t.Run(test.name, func(t *testing.T) {
			var unlocks []string
			var mu sync.Mutex
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				assertWebDAVTestAuth(t, request)
				switch request.Method {
				case "LOCK":
					if request.URL.Path != "/netdisk/api/webdav/" || request.Header.Get("Depth") != "infinity" {
						t.Errorf("MOVE did not lock its common parent collection: path=%q depth=%q", request.URL.Path, request.Header.Get("Depth"))
					}
					writer.Header().Set("Lock-Token", "<opaquelocktoken:move-test>")
					writer.WriteHeader(http.StatusOK)
				case "PROPFIND":
					if request.URL.Path == "/netdisk/api/webdav/" {
						writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
					} else {
						writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/file.md", `<d:resourcetype/><d:getetag>&quot;v1&quot;</d:getetag>`, ""))
					}
				case "MOVE":
					if request.Header.Get("If") != "(<opaquelocktoken:move-test>)" || request.Header.Get("Overwrite") != "F" {
						t.Errorf("MOVE did not carry the lock/no-overwrite conditions: %#v", request.Header)
					}
					if request.Header.Get("Destination") != server.URL+"/netdisk/api/webdav/renamed.md" {
						t.Errorf("unexpected MOVE destination: %q", request.Header.Get("Destination"))
					}
					if test.abortMove {
						connection, _, err := writer.(http.Hijacker).Hijack()
						if err != nil {
							t.Errorf("hijack MOVE connection: %v", err)
							return
						}
						_ = connection.Close()
						return
					}
					writer.WriteHeader(test.moveStatus)
				case "UNLOCK":
					mu.Lock()
					unlocks = append(unlocks, request.URL.Path)
					mu.Unlock()
					writer.WriteHeader(http.StatusNoContent)
				default:
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()
			client := newWebDAVTestClient(t, server.URL)
			client.advertisedRootPath = "/api/webdav/"
			client.advertisedRootKnown = true
			expectedRevision := opaqueWebDAVRevision("file.md", "markdown", `"v1"`, "", 0, "")
			err := client.MoveResourceLocked(context.Background(), "file.md", "renamed.md", false, expectedRevision)
			if test.wantErrorKind == "" && err != nil {
				t.Fatal(err)
			}
			if test.wantErrorKind != "" && !IsWebDAVErrorKind(err, test.wantErrorKind) {
				t.Fatalf("expected %s, got %v", test.wantErrorKind, err)
			}
			mu.Lock()
			got := append([]string(nil), unlocks...)
			mu.Unlock()
			if len(got) != 1 || got[0] != "/netdisk/api/webdav/" {
				t.Fatalf("MOVE unlocked the wrong path: %q", got)
			}
		})
	}
}

func TestLockedWebDAVMutationsAgainstStrictXNetHandler(t *testing.T) {
	handler := &xwebdav.Handler{
		Prefix:     "/netdisk/api/webdav",
		FileSystem: xwebdav.NewMemFS(),
		LockSystem: xwebdav.NewMemLS(),
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertWebDAVTestAuth(t, request)
		handler.ServeHTTP(writer, request)
	}))
	defer server.Close()
	client := newWebDAVTestClient(t, server.URL)
	defer client.Close()
	ctx := context.Background()
	if err := client.CheckConnection(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.CreateDirectory(ctx, "folder"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutMarkdown(ctx, "folder/file.md", "# strict DAV\n", WebDAVWriteOptions{CreateOnly: true}); err != nil {
		t.Fatal(err)
	}
	if err := client.MoveResourceLocked(ctx, "folder/file.md", "folder/renamed.md", false, webDAVRevisionForTest(t, client, "folder/file.md", "markdown")); err != nil {
		t.Fatalf("strict x/net DAV rejected parent-locked MOVE: %v", err)
	}
	renamed, err := client.ReadMarkdown(ctx, "folder/renamed.md")
	if err != nil || renamed.Content != "# strict DAV\n" {
		t.Fatalf("strict x/net DAV MOVE changed content: %#v, %v", renamed, err)
	}
	if err := client.DeleteResourceLocked(ctx, "folder/renamed.md", false, webDAVRevisionForTest(t, client, "folder/renamed.md", "markdown")); err != nil {
		t.Fatalf("strict x/net DAV rejected locked file DELETE: %v", err)
	}
	concurrentClient := newWebDAVTestClient(t, server.URL)
	defer concurrentClient.Close()
	if err := concurrentClient.CheckConnection(ctx); err != nil {
		t.Fatal(err)
	}
	var descendantWriteErr error
	client.testAfterMutationLockAcquired = func() {
		_, descendantWriteErr = concurrentClient.PutMarkdown(ctx, "folder/concurrent.md", "unsafe", WebDAVWriteOptions{CreateOnly: true})
	}
	app, workspaceID := webDAVTestAppForClient(client)
	preparation, err := app.BeginWebDAVMutation(workspaceID, "folder", "directory", "", "delete")
	if err != nil {
		t.Fatalf("strict x/net DAV rejected prepared collection DELETE: %v", err)
	}
	if err := app.CommitWebDAVDelete(workspaceID, preparation.MutationID, true); err != nil {
		t.Fatalf("strict x/net DAV rejected locked collection DELETE: %v", err)
	}
	client.testAfterMutationLockAcquired = nil
	if !IsWebDAVErrorKind(descendantWriteErr, WebDAVErrorLocked) {
		t.Fatalf("infinite-depth collection lock did not block a descendant write: %v", descendantWriteErr)
	}
}

func TestLockedWebDAVDirectoryRenamesAgainstStrictXNetHandler(t *testing.T) {
	handler := &xwebdav.Handler{
		Prefix:     "/netdisk/api/webdav",
		FileSystem: xwebdav.NewMemFS(),
		LockSystem: xwebdav.NewMemLS(),
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertWebDAVTestAuth(t, request)
		handler.ServeHTTP(writer, request)
	}))
	defer server.Close()
	client := newWebDAVTestClient(t, server.URL)
	defer client.Close()
	ctx := context.Background()
	if err := client.CheckConnection(ctx); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"root-folder", "root-folder/nested"} {
		if err := client.CreateDirectory(ctx, directory); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.PutMarkdown(ctx, "root-folder/nested/child.md", "# descendant\n", WebDAVWriteOptions{CreateOnly: true}); err != nil {
		t.Fatal(err)
	}
	app, workspaceID := webDAVTestAppForClient(client)
	preparation, err := app.BeginWebDAVMutation(workspaceID, "root-folder", "directory", "", "rename")
	if err != nil {
		t.Fatalf("strict x/net DAV rejected root-level collection preparation: %v", err)
	}
	if _, err := app.CommitWebDAVRename(workspaceID, preparation.MutationID, "renamed-root"); err != nil {
		t.Fatalf("strict x/net DAV rejected root-level collection rename: %v", err)
	}
	if document, err := client.ReadMarkdown(ctx, "renamed-root/nested/child.md"); err != nil || document.Content != "# descendant\n" {
		t.Fatalf("root-level collection rename lost its descendant: %#v, %v", document, err)
	}
	preparation, err = app.BeginWebDAVMutation(workspaceID, "renamed-root/nested", "directory", "", "rename")
	if err != nil {
		t.Fatalf("strict x/net DAV rejected nested collection preparation: %v", err)
	}
	if _, err := app.CommitWebDAVRename(workspaceID, preparation.MutationID, "renamed-root/renamed-nested"); err != nil {
		t.Fatalf("strict x/net DAV rejected nested collection rename: %v", err)
	}
	if document, err := client.ReadMarkdown(ctx, "renamed-root/renamed-nested/child.md"); err != nil || document.Content != "# descendant\n" {
		t.Fatalf("nested collection rename lost its descendant: %#v, %v", document, err)
	}
	preparation, err = app.BeginWebDAVMutation(workspaceID, "renamed-root", "directory", "", "delete")
	if err != nil {
		t.Fatalf("strict x/net DAV rejected cleanup preparation: %v", err)
	}
	if err := app.CommitWebDAVDelete(workspaceID, preparation.MutationID, true); err != nil {
		t.Fatalf("strict x/net DAV cleanup failed: %v", err)
	}
}

func TestLockedWebDAVRootParentLockNullNeverDeletesRoot(t *testing.T) {
	var mu sync.Mutex
	methods := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertWebDAVTestAuth(t, request)
		mu.Lock()
		methods = append(methods, request.Method+" "+request.URL.Path)
		mu.Unlock()
		switch request.Method {
		case "LOCK":
			if request.URL.Path != "/netdisk/api/webdav/" || request.Header.Get("Depth") != "infinity" {
				t.Errorf("unexpected root parent LOCK: path=%q depth=%q", request.URL.Path, request.Header.Get("Depth"))
			}
			writer.Header().Set("Lock-Token", "<opaquelocktoken:root-lock-null>")
			writer.WriteHeader(http.StatusCreated)
		case "UNLOCK":
			writer.WriteHeader(http.StatusNoContent)
		case "DELETE", "MOVE":
			t.Errorf("root lock-null handling emitted unsafe %s", request.Method)
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	client := newWebDAVTestClient(t, server.URL)
	client.advertisedRootPath = "/api/webdav/"
	client.advertisedRootKnown = true
	err := client.MoveResourceLocked(context.Background(), "source.md", "destination.md", false, "")
	if !IsWebDAVErrorKind(err, WebDAVErrorProtocol) {
		t.Fatalf("root lock-null response was not rejected safely: %v", err)
	}
	mu.Lock()
	gotMethods := append([]string(nil), methods...)
	mu.Unlock()
	wantMethods := []string{"LOCK /netdisk/api/webdav/", "UNLOCK /netdisk/api/webdav/"}
	if fmt.Sprint(gotMethods) != fmt.Sprint(wantMethods) {
		t.Fatalf("root lock-null method sequence = %v, want %v", gotMethods, wantMethods)
	}
}

func TestLockedWebDAVMutationUnlocksLockNullAfterCallerCancellation(t *testing.T) {
	for _, operation := range []string{"delete"} {
		t.Run(operation, func(t *testing.T) {
			var mu sync.Mutex
			methods := make([]string, 0, 3)
			placeholderExists := false
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				assertWebDAVTestAuth(t, request)
				mu.Lock()
				methods = append(methods, request.Method)
				mu.Unlock()
				switch request.Method {
				case "LOCK":
					mu.Lock()
					placeholderExists = true
					mu.Unlock()
					writer.Header().Set("Lock-Token", "<opaquelocktoken:lock-null-cancel>")
					writer.WriteHeader(http.StatusCreated)
				case "PROPFIND":
					writer.WriteHeader(http.StatusNotFound)
				case http.MethodDelete:
					t.Error("mutation lock-null handling emitted an unsafe cleanup DELETE")
					writer.WriteHeader(http.StatusNoContent)
				case "UNLOCK":
					mu.Lock()
					placeholderExists = false
					mu.Unlock()
					writer.WriteHeader(http.StatusNoContent)
				default:
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()
			client := newWebDAVTestClient(t, server.URL)
			client.advertisedRootPath = "/api/webdav/"
			client.advertisedRootKnown = true
			ctx, cancel := context.WithCancel(context.Background())
			client.testAfterMutationLockAcquired = cancel
			var err error
			if operation == "delete" {
				err = client.DeleteResourceLocked(ctx, "missing.md", false, "")
			} else {
				err = client.MoveResourceLocked(ctx, "missing.md", "renamed.md", false, "")
			}
			if !IsWebDAVErrorKind(err, WebDAVErrorNotFound) {
				t.Fatalf("expected a not-found result after lock-null cleanup, got %v", err)
			}
			mu.Lock()
			gotMethods := append([]string(nil), methods...)
			stillExists := placeholderExists
			mu.Unlock()
			wantMethods := []string{"LOCK", "PROPFIND", "UNLOCK"}
			if fmt.Sprint(gotMethods) != fmt.Sprint(wantMethods) {
				t.Fatalf("lock-null cleanup order = %v, want %v", gotMethods, wantMethods)
			}
			if stillExists {
				t.Fatal("caller cancellation exposed a lock-null placeholder")
			}
		})
	}
}

func TestWebDAVAppFileOperationsImagesAndSessionRebase(t *testing.T) {
	server, state := newWorkspaceFileOperationsDAVServer(t)
	defer server.Close()
	app := &App{}
	workspace, err := app.ConnectWebDAV(WebDAVConfig{Endpoint: server.URL + "/netdisk/api/webdav/", Username: webDAVTestUsername, Password: webDAVTestPassword})
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	if got := workspaceKindsAndNames(workspace.Entries); !strings.Contains(got, "image:image.png") {
		t.Fatalf("WebDAV workspace omitted its image: %s", got)
	}
	preview, err := app.ReadWebDAVWorkspaceImage(workspace.ID, "image.png")
	if err != nil || preview.MIMEType != "image/png" || preview.Width != 5 || preview.Height != 3 {
		t.Fatalf("unexpected remote image preview: %#v, %v", preview, err)
	}
	created, err := app.CreateWebDAVMarkdownFile(workspace.ID, "created.md")
	if err != nil {
		t.Fatal(err)
	}
	if created.RemoteDocumentID == "" || created.ETag == "" || created.Content != "" || created.WorkspacePath != "created.md" {
		t.Fatalf("created remote document is not immediately editable: %#v", created)
	}
	if state.getCount("created.md") != 1 {
		t.Fatalf("verified create issued an avoidable second GET: %d", state.getCount("created.md"))
	}
	opened, err := app.OpenWebDAVFile(workspace.ID, "document.md")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := renameWebDAVEntryForTest(app, workspace.ID, "document.md", "Renamed.md", "markdown")
	if err != nil || renamed.Path != "Renamed.md" {
		t.Fatalf("remote rename failed: %#v, %v", renamed, err)
	}
	capability, err := app.webDAVCapabilityByID(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	capability.mu.RLock()
	bridgeSession := capability.documents[opened.RemoteDocumentID]
	client := capability.client
	capability.mu.RUnlock()
	client.mu.RLock()
	clientSession := client.sessions[opened.RemoteDocumentID]
	client.mu.RUnlock()
	if bridgeSession.path != "Renamed.md" || clientSession.Path != "Renamed.md" || bridgeSession.etag != opened.ETag || clientSession.ETag != opened.ETag {
		t.Fatalf("MOVE did not safely rebase both sessions: bridge=%#v client=%#v", bridgeSession, clientSession)
	}
	if err := deleteWebDAVEntryForTest(app, workspace.ID, "Renamed.md", "markdown", false); err != nil {
		t.Fatal(err)
	}
	capability.mu.RLock()
	_, bridgeStillOpen := capability.documents[opened.RemoteDocumentID]
	capability.mu.RUnlock()
	client.mu.RLock()
	_, clientStillOpen := client.sessions[opened.RemoteDocumentID]
	client.mu.RUnlock()
	if bridgeStillOpen || clientStillOpen {
		t.Fatal("DELETE did not revoke the deleted document sessions")
	}
	descendant, err := app.OpenWebDAVFile(workspace.ID, "folder/child.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := renameWebDAVEntryForTest(app, workspace.ID, "folder", "renamed-folder", "directory"); err != nil {
		t.Fatalf("directory rename failed: %v", err)
	}
	assertWebDAVSessionState(t, app, workspace.ID, descendant.RemoteDocumentID, "renamed-folder/child.md", descendant.ETag, true)
	if err := deleteWebDAVEntryForTest(app, workspace.ID, "renamed-folder", "directory", true); err != nil {
		t.Fatalf("recursive WebDAV directory delete failed: %v", err)
	}
	assertWebDAVSessionState(t, app, workspace.ID, descendant.RemoteDocumentID, "", "", false)
	directory, err := app.CreateWebDAVDirectory(workspace.ID, "new-folder")
	if err != nil || directory.Kind != "directory" {
		t.Fatalf("remote directory create failed: %#v, %v", directory, err)
	}
	if err := deleteWebDAVEntryForTest(app, workspace.ID, "new-folder", "directory", false); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("collection delete skipped recursive confirmation: %v", err)
	}
	if err := deleteWebDAVEntryForTest(app, workspace.ID, "new-folder", "directory", true); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	unlocks := append([]string(nil), state.unlocks...)
	state.mu.Unlock()
	if !containsString(unlocks, "") || !containsString(unlocks, "new-folder") {
		t.Fatalf("mutations did not unlock their final DAV resources: %q", unlocks)
	}
}

func TestFailedWebDAVMutationsKeepBridgeAndClientSessions(t *testing.T) {
	server, state := newWorkspaceFileOperationsDAVServer(t)
	defer server.Close()
	app := &App{}
	workspace, err := app.ConnectWebDAV(WebDAVConfig{Endpoint: server.URL + "/netdisk/api/webdav/", Username: webDAVTestUsername, Password: webDAVTestPassword})
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	document, err := app.OpenWebDAVFile(workspace.ID, "document.md")
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name         string
		moveStatus   int
		abortMove    bool
		deleteStatus int
		operation    string
	}{
		{name: "MOVE 412", moveStatus: http.StatusPreconditionFailed, operation: "move"},
		{name: "MOVE transport error", abortMove: true, operation: "move"},
		{name: "DELETE 207", deleteStatus: http.StatusMultiStatus, operation: "delete"},
		{name: "DELETE 412", deleteStatus: http.StatusPreconditionFailed, operation: "delete"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state.mu.Lock()
			unlockCountBefore := len(state.unlocks)
			state.moveStatus = test.moveStatus
			state.abortNextMove = test.abortMove
			state.deleteStatus = test.deleteStatus
			state.mu.Unlock()
			if test.operation == "move" {
				if _, err := renameWebDAVEntryForTest(app, workspace.ID, "document.md", "renamed.md", "markdown"); err == nil {
					t.Fatal("failed MOVE was reported as successful")
				}
			} else if err := deleteWebDAVEntryForTest(app, workspace.ID, "document.md", "markdown", false); err == nil {
				t.Fatal("failed DELETE was reported as successful")
			}
			assertWebDAVSessionState(t, app, workspace.ID, document.RemoteDocumentID, "document.md", document.ETag, true)
			state.mu.Lock()
			_, resourceStillExists := state.resources["document.md"]
			newUnlocks := append([]string(nil), state.unlocks[unlockCountBefore:]...)
			state.moveStatus = 0
			state.abortNextMove = false
			state.deleteStatus = 0
			state.mu.Unlock()
			if !resourceStillExists {
				t.Fatal("failed mutation removed or moved the remote resource")
			}
			if test.operation == "move" && (len(newUnlocks) != 1 || newUnlocks[0] != "") {
				t.Fatalf("failed MOVE did not unlock exactly its common parent: %q", newUnlocks)
			}
		})
	}
}

func TestBeginWebDAVMutationRejectsDisplayedFileReplacement(t *testing.T) {
	for _, operation := range []string{"rename", "delete"} {
		t.Run(operation, func(t *testing.T) {
			server, state := newWorkspaceFileOperationsDAVServer(t)
			defer server.Close()
			app := &App{}
			workspace, err := app.ConnectWebDAV(WebDAVConfig{Endpoint: server.URL + "/netdisk/api/webdav/", Username: webDAVTestUsername, Password: webDAVTestPassword})
			if err != nil {
				t.Fatal(err)
			}
			defer app.shutdown(nil)

			var displayed WorkspaceEntry
			for _, entry := range workspace.Entries {
				if entry.Path == "document.md" {
					displayed = entry
					break
				}
			}
			if displayed.Revision == "" {
				t.Fatalf("displayed file did not receive a strong revision: %#v", displayed)
			}
			capability, err := app.webDAVCapabilityByID(workspace.ID)
			if err != nil {
				t.Fatal(err)
			}
			capability.mu.RLock()
			client := capability.client
			capability.mu.RUnlock()
			client.testAfterMutationLockAcquired = func() {
				state.mu.Lock()
				replacement := state.resources["document.md"]
				replacement.content = []byte("replacement-sentinel")
				replacement.etag = `"document-v2"`
				state.resources["document.md"] = replacement
				state.mu.Unlock()
			}

			_, err = app.BeginWebDAVMutation(workspace.ID, displayed.Path, displayed.Kind, displayed.Revision, operation)
			client.testAfterMutationLockAcquired = nil
			if !IsWebDAVErrorKind(err, WebDAVErrorConflict) {
				t.Fatalf("%s accepted a same-kind replacement after display: %v", operation, err)
			}
			state.mu.Lock()
			moveCount := state.moveCount
			deleteCount := state.deleteCount
			resource := state.resources["document.md"]
			state.mu.Unlock()
			if moveCount != 0 || deleteCount != 0 {
				t.Fatalf("rejected %s emitted a destructive request: MOVE=%d DELETE=%d", operation, moveCount, deleteCount)
			}
			if string(resource.content) != "replacement-sentinel" || resource.etag != `"document-v2"` {
				t.Fatalf("rejected %s changed the replacement: %#v", operation, resource)
			}
		})
	}
}

func TestBeginWebDAVMutationRequiresStrongFileETag(t *testing.T) {
	for _, etag := range []string{"", `W/"document-v1"`} {
		t.Run(fmt.Sprintf("etag_%q", etag), func(t *testing.T) {
			server, state := newWorkspaceFileOperationsDAVServer(t)
			defer server.Close()
			state.mu.Lock()
			resource := state.resources["document.md"]
			resource.etag = etag
			state.resources["document.md"] = resource
			state.mu.Unlock()
			app := &App{}
			workspace, err := app.ConnectWebDAV(WebDAVConfig{Endpoint: server.URL + "/netdisk/api/webdav/", Username: webDAVTestUsername, Password: webDAVTestPassword})
			if err != nil {
				t.Fatal(err)
			}
			defer app.shutdown(nil)

			var displayed WorkspaceEntry
			for _, entry := range workspace.Entries {
				if entry.Path == "document.md" {
					displayed = entry
					break
				}
			}
			if displayed.Path == "" || displayed.Revision != "" {
				t.Fatalf("file with non-strong ETag received an unsafe revision: %#v", displayed)
			}
			_, err = app.BeginWebDAVMutation(workspace.ID, displayed.Path, displayed.Kind, displayed.Revision, "delete")
			if !IsWebDAVErrorKind(err, WebDAVErrorUnsupported) {
				t.Fatalf("file without a strong ETag was not rejected: %v", err)
			}
			state.mu.Lock()
			lockCount := state.lockCount
			moveCount := state.moveCount
			deleteCount := state.deleteCount
			state.mu.Unlock()
			if lockCount != 0 || moveCount != 0 || deleteCount != 0 {
				t.Fatalf("unsupported file mutation emitted DAV mutations: LOCK=%d MOVE=%d DELETE=%d", lockCount, moveCount, deleteCount)
			}
		})
	}
}

func TestPreparedWebDAVMutationCancelAndExpiredRefreshAreSafe(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		server, state := newWorkspaceFileOperationsDAVServer(t)
		defer server.Close()
		app := &App{}
		workspace, err := app.ConnectWebDAV(WebDAVConfig{Endpoint: server.URL + "/netdisk/api/webdav/", Username: webDAVTestUsername, Password: webDAVTestPassword})
		if err != nil {
			t.Fatal(err)
		}
		defer app.shutdown(nil)
		preparation, err := beginWebDAVMutationForTest(app, workspace.ID, "folder", "directory", "rename")
		if err != nil {
			t.Fatal(err)
		}
		if err := app.CancelWebDAVMutation(workspace.ID, preparation.MutationID); err != nil {
			t.Fatal(err)
		}
		if _, err := app.CommitWebDAVRename(workspace.ID, preparation.MutationID, "renamed-folder"); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
			t.Fatalf("cancelled capability could still be committed: %v", err)
		}
		state.mu.Lock()
		unlocks := append([]string(nil), state.unlocks...)
		moveCount := state.moveCount
		deleteCount := state.deleteCount
		state.mu.Unlock()
		if fmt.Sprint(unlocks) != fmt.Sprint([]string{""}) || moveCount != 0 || deleteCount != 0 {
			t.Fatalf("cancel did not only unlock the common parent: unlocks=%q MOVE=%d DELETE=%d", unlocks, moveCount, deleteCount)
		}
	})

	t.Run("refresh rejected", func(t *testing.T) {
		server, state := newWorkspaceFileOperationsDAVServer(t)
		defer server.Close()
		app := &App{}
		workspace, err := app.ConnectWebDAV(WebDAVConfig{Endpoint: server.URL + "/netdisk/api/webdav/", Username: webDAVTestUsername, Password: webDAVTestPassword})
		if err != nil {
			t.Fatal(err)
		}
		defer app.shutdown(nil)
		preparation, err := beginWebDAVMutationForTest(app, workspace.ID, "document.md", "markdown", "delete")
		if err != nil {
			t.Fatal(err)
		}
		state.mu.Lock()
		state.refreshStatus = http.StatusPreconditionFailed
		state.mu.Unlock()
		err = app.CommitWebDAVDelete(workspace.ID, preparation.MutationID, false)
		if !IsWebDAVErrorKind(err, WebDAVErrorConflict) {
			t.Fatalf("rejected lock refresh was not reported as a conflict: %v", err)
		}
		state.mu.Lock()
		_, stillExists := state.resources["document.md"]
		unlocks := append([]string(nil), state.unlocks...)
		moveCount := state.moveCount
		deleteCount := state.deleteCount
		state.mu.Unlock()
		if !stillExists || moveCount != 0 || deleteCount != 0 {
			t.Fatalf("failed refresh changed the resource: exists=%t MOVE=%d DELETE=%d", stillExists, moveCount, deleteCount)
		}
		if fmt.Sprint(unlocks) != fmt.Sprint([]string{"document.md"}) {
			t.Fatalf("failed refresh did not release exactly the prepared lock: %q", unlocks)
		}
	})
}

func TestClosingWebDAVWorkspaceUnlocksPreparedFileAtExactPath(t *testing.T) {
	server, state := newWorkspaceFileOperationsDAVServer(t)
	defer server.Close()
	app := &App{}
	workspace, err := app.ConnectWebDAV(WebDAVConfig{Endpoint: server.URL + "/netdisk/api/webdav/", Username: webDAVTestUsername, Password: webDAVTestPassword})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beginWebDAVMutationForTest(app, workspace.ID, "document.md", "markdown", "delete"); err != nil {
		t.Fatal(err)
	}
	app.shutdown(nil)
	state.mu.Lock()
	unlockPaths := append([]string(nil), state.unlockPaths...)
	moveCount := state.moveCount
	deleteCount := state.deleteCount
	state.mu.Unlock()
	wantPath := "/netdisk/api/webdav/document.md"
	if fmt.Sprint(unlockPaths) != fmt.Sprint([]string{wantPath}) || moveCount != 0 || deleteCount != 0 {
		t.Fatalf("workspace close did not only unlock the prepared file path: paths=%q MOVE=%d DELETE=%d", unlockPaths, moveCount, deleteCount)
	}
}

func TestPreparedWebDAVRenameInvalidDestinationConsumesAndUnlocks(t *testing.T) {
	for _, test := range []struct {
		name        string
		destination string
	}{
		{name: "invalid path", destination: "../renamed.md"},
		{name: "different parent", destination: "other/renamed.md"},
		{name: "wrong extension", destination: "renamed.png"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, state := newWorkspaceFileOperationsDAVServer(t)
			defer server.Close()
			app := &App{}
			workspace, err := app.ConnectWebDAV(WebDAVConfig{Endpoint: server.URL + "/netdisk/api/webdav/", Username: webDAVTestUsername, Password: webDAVTestPassword})
			if err != nil {
				t.Fatal(err)
			}
			defer app.shutdown(nil)
			document, err := app.OpenWebDAVFile(workspace.ID, "document.md")
			if err != nil {
				t.Fatal(err)
			}
			preparation, err := beginWebDAVMutationForTest(app, workspace.ID, "document.md", "markdown", "rename")
			if err != nil {
				t.Fatal(err)
			}

			if _, err := app.CommitWebDAVRename(workspace.ID, preparation.MutationID, test.destination); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
				t.Fatalf("invalid rename destination returned the wrong error: %v", err)
			}
			assertWebDAVSessionState(t, app, workspace.ID, document.RemoteDocumentID, "document.md", document.ETag, true)
			capability, err := app.webDAVCapabilityByID(workspace.ID)
			if err != nil {
				t.Fatal(err)
			}
			capability.mu.RLock()
			_, stillPrepared := capability.mutations[preparation.MutationID]
			capability.mu.RUnlock()
			if stillPrepared {
				t.Fatal("invalid commit left the prepared mutation capability reachable")
			}
			if _, err := app.CommitWebDAVRename(workspace.ID, preparation.MutationID, "renamed.md"); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
				t.Fatalf("consumed mutation could be retried: %v", err)
			}

			state.mu.Lock()
			unlocks := append([]string(nil), state.unlocks...)
			moveCount := state.moveCount
			deleteCount := state.deleteCount
			_, sourceExists := state.resources["document.md"]
			state.mu.Unlock()
			if fmt.Sprint(unlocks) != fmt.Sprint([]string{""}) {
				t.Fatalf("invalid commit did not unlock its common parent exactly once: %q", unlocks)
			}
			if moveCount != 0 || deleteCount != 0 || !sourceExists {
				t.Fatalf("invalid commit changed remote state: MOVE=%d DELETE=%d source=%t", moveCount, deleteCount, sourceExists)
			}
		})
	}
}

// TestWebDAVWorkspaceFileOperationsRealIntegration exercises the complete
// sidebar mutation lifecycle against an explicitly configured DAV service.
// Credentials are supplied only through the environment and are never logged.
func TestWebDAVWorkspaceFileOperationsRealIntegration(t *testing.T) {
	if os.Getenv("INKMARK_REAL_WEBDAV") != "1" {
		t.Skip("set INKMARK_REAL_WEBDAV=1 and the INKMARK_WEBDAV_TEST_* variables to run")
	}
	endpoint := strings.TrimSpace(os.Getenv("INKMARK_WEBDAV_TEST_ENDPOINT"))
	username := os.Getenv("INKMARK_WEBDAV_TEST_USERNAME")
	password := os.Getenv("INKMARK_WEBDAV_TEST_PASSWORD")
	if endpoint == "" || username == "" || password == "" {
		t.Fatal("real WebDAV integration configuration is incomplete")
	}
	app := &App{}
	workspace, err := app.ConnectWebDAV(WebDAVConfig{Endpoint: endpoint, Username: username, Password: password, Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	identifier, err := newOpaqueID()
	if err != nil {
		t.Fatal(err)
	}
	originalRoot := "inkmark-fileops-integration-" + identifier
	currentRoot := originalRoot
	capability, err := app.webDAVCapabilityByID(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		capability.mu.RLock()
		client := capability.client
		capability.mu.RUnlock()
		if client != nil {
			_ = client.DeleteResource(cleanupContext, currentRoot, true)
			if currentRoot != originalRoot {
				_ = client.DeleteResource(cleanupContext, originalRoot, true)
			}
		}
	}()
	if _, err := app.CreateWebDAVDirectory(workspace.ID, originalRoot); err != nil {
		t.Fatal(err)
	}
	documentPath := path.Join(originalRoot, "document.md")
	document, err := app.CreateWebDAVMarkdownFile(workspace.ID, documentPath)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := app.SaveWebDAVFile(workspace.ID, document.RemoteDocumentID, "# real WebDAV file operations\n\n中文 + English\n", document.ETag)
	if err != nil || saved.Conflict || saved.ETag == "" {
		t.Fatalf("real WebDAV save failed: result=%#v error=%v", saved, err)
	}
	capability.mu.RLock()
	client := capability.client
	capability.mu.RUnlock()
	readBack, err := client.ReadMarkdown(context.Background(), documentPath)
	if err != nil || !strings.Contains(readBack.Content, "中文 + English") {
		t.Fatalf("real WebDAV readback failed: %v", err)
	}

	pngPayload := makePNG(t, 9, 7)
	asset, err := app.ImportWebDAVImageData(workspace.ID, document.RemoteDocumentID, "integration.png", "image/png", base64.StdEncoding.EncodeToString(pngPayload))
	if err != nil {
		t.Fatal(err)
	}
	imagePath := path.Join(originalRoot, asset.MarkdownURL)
	preview, err := app.ReadWebDAVWorkspaceImage(workspace.ID, imagePath)
	if err != nil || preview.SHA256 != asset.SHA256 || preview.Width != 9 || preview.Height != 7 {
		t.Fatalf("real WebDAV image preview failed: %#v, %v", preview, err)
	}
	renamedImagePath := path.Join(path.Dir(imagePath), "renamed.png")
	if _, err := renameWebDAVEntryForTest(app, workspace.ID, imagePath, renamedImagePath, "image"); err != nil {
		t.Fatalf("real WebDAV image rename failed: %v", err)
	}
	renamedDocumentPath := path.Join(originalRoot, "renamed.md")
	if _, err := renameWebDAVEntryForTest(app, workspace.ID, documentPath, renamedDocumentPath, "markdown"); err != nil {
		t.Fatalf("real WebDAV Markdown rename failed: %v", err)
	}
	assertWebDAVSessionState(t, app, workspace.ID, document.RemoteDocumentID, renamedDocumentPath, saved.ETag, true)

	renamedRoot := originalRoot + "-renamed"
	if _, err := renameWebDAVEntryForTest(app, workspace.ID, originalRoot, renamedRoot, "directory"); err != nil {
		t.Fatalf("real WebDAV collection LOCK/MOVE failed: %v", err)
	}
	currentRoot = renamedRoot
	renamedDocumentPath = path.Join(renamedRoot, "renamed.md")
	renamedImagePath = path.Join(renamedRoot, strings.TrimPrefix(renamedImagePath, originalRoot+"/"))
	assertWebDAVSessionState(t, app, workspace.ID, document.RemoteDocumentID, renamedDocumentPath, saved.ETag, true)
	if preview, err := app.ReadWebDAVWorkspaceImage(workspace.ID, renamedImagePath); err != nil || preview.SHA256 != asset.SHA256 {
		t.Fatalf("image did not survive real collection MOVE: %#v, %v", preview, err)
	}
	if err := deleteWebDAVEntryForTest(app, workspace.ID, renamedRoot, "directory", true); err != nil {
		t.Fatalf("real WebDAV collection LOCK/DELETE failed: %v", err)
	}
	assertWebDAVSessionState(t, app, workspace.ID, document.RemoteDocumentID, "", "", false)
	rootListing, err := client.ListDirectory(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range rootListing.Entries {
		if entry.Path == originalRoot || entry.Path == renamedRoot || strings.HasPrefix(entry.Path, originalRoot+"/") || strings.HasPrefix(entry.Path, renamedRoot+"/") {
			t.Fatalf("real WebDAV cleanup left an integration resource: %q", entry.Path)
		}
	}
}

func assertWebDAVSessionState(t *testing.T, app *App, workspaceID string, documentID string, wantPath string, wantETag string, wantOpen bool) {
	t.Helper()
	capability, err := app.webDAVCapabilityByID(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	capability.mu.RLock()
	bridgeSession, bridgeOpen := capability.documents[documentID]
	client := capability.client
	capability.mu.RUnlock()
	client.mu.RLock()
	clientSession, clientOpen := client.sessions[documentID]
	client.mu.RUnlock()
	if bridgeOpen != wantOpen || clientOpen != wantOpen {
		t.Fatalf("unexpected session lifecycle: bridgeOpen=%t clientOpen=%t want=%t", bridgeOpen, clientOpen, wantOpen)
	}
	if wantOpen && (bridgeSession.path != wantPath || clientSession.Path != wantPath || bridgeSession.etag != wantETag || clientSession.ETag != wantETag) {
		t.Fatalf("unexpected session state: bridge=%#v client=%#v wantPath=%q wantETag=%q", bridgeSession, clientSession, wantPath, wantETag)
	}
}

func beginWebDAVMutationForTest(app *App, workspaceID string, relativePath string, expectedKind string, operation string) (WebDAVMutationPreparation, error) {
	directory, err := app.ListWebDAVDirectory(workspaceID, parentWebDAVPath(relativePath))
	if err != nil {
		return WebDAVMutationPreparation{}, err
	}
	for _, entry := range directory.Entries {
		if entry.Path == relativePath {
			return app.BeginWebDAVMutation(workspaceID, relativePath, expectedKind, entry.Revision, operation)
		}
	}
	return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "prepare mutation", Path: relativePath}
}

func webDAVRevisionForTest(t *testing.T, client *WebDAVClient, relativePath string, expectedKind string) string {
	t.Helper()
	directory, err := client.ListDirectory(context.Background(), parentWebDAVPath(relativePath))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range webDAVWorkspaceDirectory(directory).Entries {
		if entry.Path == relativePath && entry.Kind == expectedKind {
			if entry.Revision == "" {
				t.Fatalf("WebDAV entry %q did not advertise a strong revision", relativePath)
			}
			return entry.Revision
		}
	}
	t.Fatalf("WebDAV entry %q was not listed", relativePath)
	return ""
}

func webDAVTestAppForClient(client *WebDAVClient) (*App, string) {
	const workspaceID = "prepared-webdav-test-workspace"
	capability := &webDAVCapability{
		id: workspaceID, client: client,
		documents: make(map[string]webDAVDocumentCapability),
		mutations: make(map[string]preparedWebDAVMutation),
	}
	return &App{webDAVWorkspaces: map[string]*webDAVCapability{workspaceID: capability}}, workspaceID
}

func renameWebDAVEntryForTest(app *App, workspaceID string, sourcePath string, destinationPath string, expectedKind string) (WorkspaceEntry, error) {
	preparation, err := beginWebDAVMutationForTest(app, workspaceID, sourcePath, expectedKind, "rename")
	if err != nil {
		return WorkspaceEntry{}, err
	}
	return app.CommitWebDAVRename(workspaceID, preparation.MutationID, destinationPath)
}

func deleteWebDAVEntryForTest(app *App, workspaceID string, relativePath string, expectedKind string, recursive bool) error {
	preparation, err := beginWebDAVMutationForTest(app, workspaceID, relativePath, expectedKind, "delete")
	if err != nil {
		return err
	}
	return app.CommitWebDAVDelete(workspaceID, preparation.MutationID, recursive)
}

type workspaceDAVResource struct {
	directory bool
	content   []byte
	etag      string
	mimeType  string
}

type workspaceFileOperationsDAVState struct {
	mu            sync.Mutex
	resources     map[string]workspaceDAVResource
	lockNull      map[string]bool
	unlocks       []string
	unlockPaths   []string
	gets          map[string]int
	version       int
	lockCount     int
	moveCount     int
	deleteCount   int
	refreshStatus int
	moveStatus    int
	deleteStatus  int
	abortNextMove bool
}

func (state *workspaceFileOperationsDAVState) getCount(name string) int {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.gets[name]
}

func newWorkspaceFileOperationsDAVServer(t *testing.T) (*httptest.Server, *workspaceFileOperationsDAVState) {
	t.Helper()
	state := &workspaceFileOperationsDAVState{
		resources: map[string]workspaceDAVResource{
			"document.md":     {content: []byte("# document\n"), etag: `"document-v1"`, mimeType: "text/markdown"},
			"image.png":       {content: makePNG(t, 5, 3), etag: `"image-v1"`, mimeType: "image/png"},
			"folder":          {directory: true},
			"folder/child.md": {content: []byte("# child\n"), etag: `"child-v1"`, mimeType: "text/markdown"},
		},
		lockNull: make(map[string]bool),
		gets:     make(map[string]int),
		version:  1,
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertWebDAVTestAuth(t, request)
		const requestRoot = "/netdisk/api/webdav/"
		if !strings.HasPrefix(request.URL.Path, requestRoot) {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		relative := strings.Trim(strings.TrimPrefix(request.URL.Path, requestRoot), "/")
		switch request.Method {
		case "PROPFIND":
			state.mu.Lock()
			defer state.mu.Unlock()
			if request.Header.Get("Depth") == "0" {
				if relative == "" {
					writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
					return
				}
				resource, ok := state.resources[relative]
				if !ok {
					writer.WriteHeader(http.StatusNotFound)
					return
				}
				writeWebDAVMultiStatus(writer, workspaceDAVResponse(relative, resource))
				return
			}
			responses := []string{webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, "")}
			for name, resource := range state.resources {
				if parentWebDAVPath(name) == relative {
					responses = append(responses, workspaceDAVResponse(name, resource))
				}
			}
			writeWebDAVMultiStatus(writer, responses...)
		case "LOCK":
			state.mu.Lock()
			state.lockCount++
			if request.ContentLength == 0 && state.refreshStatus != 0 {
				status := state.refreshStatus
				state.mu.Unlock()
				writer.WriteHeader(status)
				return
			}
			_, exists := state.resources[relative]
			exists = exists || relative == ""
			if !exists {
				state.lockNull[relative] = true
			}
			state.mu.Unlock()
			writer.Header().Set("Lock-Token", "<opaquelocktoken:"+url.PathEscape(relative)+">")
			if exists {
				writer.WriteHeader(http.StatusOK)
			} else {
				writer.WriteHeader(http.StatusCreated)
			}
		case "UNLOCK":
			state.mu.Lock()
			state.unlocks = append(state.unlocks, relative)
			state.unlockPaths = append(state.unlockPaths, request.URL.Path)
			delete(state.lockNull, relative)
			state.mu.Unlock()
			writer.WriteHeader(http.StatusNoContent)
		case http.MethodPut:
			payload, _ := io.ReadAll(request.Body)
			state.mu.Lock()
			if !state.lockNull[relative] {
				state.mu.Unlock()
				writer.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			state.version++
			etag := fmt.Sprintf(`"v%d"`, state.version)
			state.resources[relative] = workspaceDAVResource{content: payload, etag: etag, mimeType: "text/markdown"}
			delete(state.lockNull, relative)
			state.mu.Unlock()
			writer.Header().Set("ETag", etag)
			writer.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			state.mu.Lock()
			resource, ok := state.resources[relative]
			if ok {
				state.gets[relative]++
			}
			state.mu.Unlock()
			if !ok || resource.directory {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			writer.Header().Set("ETag", resource.etag)
			writer.Header().Set("Content-Type", resource.mimeType)
			_, _ = writer.Write(resource.content)
		case "MKCOL":
			state.mu.Lock()
			if _, exists := state.resources[relative]; exists {
				state.mu.Unlock()
				writer.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			state.resources[relative] = workspaceDAVResource{directory: true}
			state.mu.Unlock()
			writer.WriteHeader(http.StatusCreated)
		case "MOVE":
			if request.Header.Get("Overwrite") != "F" || request.Header.Get("If") == "" {
				t.Errorf("unsafe MOVE headers: %#v", request.Header)
			}
			destinationURL, err := url.Parse(request.Header.Get("Destination"))
			if err != nil || !strings.HasPrefix(destinationURL.Path, requestRoot) {
				t.Errorf("unsafe MOVE destination: %q", request.Header.Get("Destination"))
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			destination := strings.Trim(strings.TrimPrefix(destinationURL.Path, requestRoot), "/")
			state.mu.Lock()
			state.moveCount++
			if state.abortNextMove {
				state.abortNextMove = false
				state.mu.Unlock()
				hijacker, ok := writer.(http.Hijacker)
				if !ok {
					t.Error("test server cannot abort MOVE transport")
					writer.WriteHeader(http.StatusInternalServerError)
					return
				}
				connection, _, err := hijacker.Hijack()
				if err != nil {
					t.Errorf("hijack MOVE connection: %v", err)
					return
				}
				_ = connection.Close()
				return
			}
			if state.moveStatus != 0 {
				status := state.moveStatus
				state.mu.Unlock()
				writer.WriteHeader(status)
				return
			}
			resource, exists := state.resources[relative]
			_, destinationExists := state.resources[destination]
			if exists && !destinationExists {
				delete(state.resources, relative)
				state.resources[destination] = resource
				for name, child := range state.resources {
					if strings.HasPrefix(name, relative+"/") {
						delete(state.resources, name)
						state.resources[destination+strings.TrimPrefix(name, relative)] = child
					}
				}
			}
			state.mu.Unlock()
			if !exists {
				writer.WriteHeader(http.StatusNotFound)
			} else if destinationExists {
				writer.WriteHeader(http.StatusPreconditionFailed)
			} else {
				writer.WriteHeader(http.StatusCreated)
			}
		case http.MethodDelete:
			if request.Header.Get("If") == "" {
				t.Error("DELETE did not carry a DAV lock condition")
			}
			state.mu.Lock()
			state.deleteCount++
			if state.deleteStatus != 0 {
				status := state.deleteStatus
				state.mu.Unlock()
				writer.WriteHeader(status)
				return
			}
			_, exists := state.resources[relative]
			if exists {
				delete(state.resources, relative)
				for name := range state.resources {
					if strings.HasPrefix(name, relative+"/") {
						delete(state.resources, name)
					}
				}
			}
			state.mu.Unlock()
			if !exists {
				writer.WriteHeader(http.StatusNotFound)
			} else {
				writer.WriteHeader(http.StatusNoContent)
			}
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	return server, state
}

func workspaceDAVResponse(name string, resource workspaceDAVResource) string {
	resourceType := `<d:resourcetype/>`
	href := "/api/webdav/" + strings.TrimPrefix(name, "/")
	if resource.directory {
		resourceType = `<d:resourcetype><d:collection/></d:resourcetype>`
		href += "/"
	}
	properties := resourceType
	if resource.etag != "" {
		properties += `<d:getetag>` + html.EscapeString(resource.etag) + `</d:getetag>`
	}
	if resource.mimeType != "" {
		properties += `<d:getcontenttype>` + html.EscapeString(resource.mimeType) + `</d:getcontenttype>`
	}
	return webDAVResponseXML(href, properties, "")
}

func workspaceKindsAndNames(entries []WorkspaceEntry) string {
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.Kind+":"+entry.Name)
	}
	return strings.Join(values, ",")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
