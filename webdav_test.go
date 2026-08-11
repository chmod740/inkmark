package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	webDAVTestUsername = "test-user"
	webDAVTestPassword = "test-password"
)

func TestWebDAVListUsesAdvertisedHrefButKeepsConfiguredRequestRoot(t *testing.T) {
	var depthZeroRequests int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertWebDAVTestAuth(t, request)
		if request.Method != "PROPFIND" || request.URL.Path != "/netdisk/api/webdav/" {
			t.Errorf("unexpected WebDAV request: %s %s", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Type", "application/xml")
		if request.Header.Get("Depth") == "0" {
			depthZeroRequests++
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
			return
		}
		if request.Header.Get("Depth") != "1" {
			t.Errorf("unexpected Depth: %q", request.Header.Get("Depth"))
		}
		writeWebDAVMultiStatus(writer,
			webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""),
			webDAVResponseXML("/api/webdav/%E4%B8%AD%E6%96%87/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""),
			webDAVResponseXML("/api/webdav/B.md", `<d:resourcetype/><d:getcontentlength>12</d:getcontentlength><d:getetag>&quot;b&quot;</d:getetag>`, `<d:resourcetype><d:collection/></d:resourcetype><d:getetag>&quot;ignored-404&quot;</d:getetag>`),
			webDAVResponseXML("/api/webdav/a.markdown", `<d:resourcetype/><d:getcontentlength>7</d:getcontentlength>`, ""),
			webDAVResponseXML("/api/webdav/nested/too-deep.md", `<d:resourcetype/>`, ""),
			webDAVResponseXML("/outside/not-visible.md", `<d:resourcetype/>`, ""),
		)
	}))
	defer server.Close()

	client := newWebDAVTestClient(t, server.URL)
	directory, err := client.ListDirectory(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if depthZeroRequests != 1 {
		t.Fatalf("expected one Depth-0 discovery, got %d", depthZeroRequests)
	}
	if len(directory.Entries) != 3 {
		t.Fatalf("unexpected directory entries: %#v", directory.Entries)
	}
	if directory.Entries[0].Name != "中文" || !directory.Entries[0].Directory {
		t.Fatalf("directory must sort first: %#v", directory.Entries)
	}
	if directory.Entries[1].Name != "a.markdown" || directory.Entries[2].Name != "B.md" {
		t.Fatalf("unexpected Markdown ordering: %#v", directory.Entries)
	}
	if directory.Entries[2].ETag != `"b"` || directory.Entries[2].Size != 12 {
		t.Fatalf("2xx properties were not selected correctly: %#v", directory.Entries[2])
	}
	if err := client.CheckConnection(context.Background()); err != nil {
		t.Fatal(err)
	}
	if depthZeroRequests != 1 {
		t.Fatalf("advertised root discovery was not cached: %d", depthZeroRequests)
	}
}

func TestWebDAVListRejectsExcessiveMultiStatusEntriesDuringDecode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Depth") == "0" {
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
			return
		}
		writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
		writer.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(writer, `<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:">`)
		for index := 0; index <= maxWebDAVResponses; index++ {
			_, _ = io.WriteString(writer, webDAVResponseXML(
				fmt.Sprintf("/api/webdav/document-%d.md", index),
				`<d:resourcetype/>`,
				"",
			))
		}
		_, _ = io.WriteString(writer, `</d:multistatus>`)
	}))
	defer server.Close()

	client := newWebDAVTestClient(t, server.URL)
	if _, err := client.ListDirectory(context.Background(), ""); !IsWebDAVErrorKind(err, WebDAVErrorTooLarge) {
		t.Fatalf("excessive multistatus response was not rejected during decode: %v", err)
	}
}

func TestWebDAVReadAndSessionSaveUseETag(t *testing.T) {
	var putRequests int
	var lockRequests int
	var unlockRequests int
	currentETag := `"v1"`
	currentContent := "# 中英文 test"
	activeLockToken := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertWebDAVTestAuth(t, request)
		if request.Method == "PROPFIND" && request.URL.Path == "/netdisk/api/webdav/" {
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
			return
		}
		if request.URL.Path != "/netdisk/api/webdav/folder/My 文档.md" {
			t.Errorf("unexpected request path: %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		if request.URL.EscapedPath() != "/netdisk/api/webdav/folder/My%20%E6%96%87%E6%A1%A3.md" {
			t.Errorf("path was not safely escaped: %q", request.URL.EscapedPath())
		}
		switch request.Method {
		case http.MethodGet:
			writer.Header().Set("ETag", currentETag)
			writer.Header().Set("Last-Modified", "Mon, 10 Aug 2026 08:00:00 GMT")
			if currentContent == "# 中英文 test" {
				_, _ = writer.Write(append([]byte{0xef, 0xbb, 0xbf}, []byte(currentContent)...))
			} else {
				_, _ = writer.Write([]byte(currentContent))
			}
		case "PROPFIND":
			writeWebDAVMultiStatus(writer,
				webDAVResponseXML("/api/webdav/folder/other.md", `<d:resourcetype/><d:getetag>&quot;attacker&quot;</d:getetag>`, ""),
				webDAVResponseXML("/api/webdav/folder/My%20%E6%96%87%E6%A1%A3.md", `<d:resourcetype/><d:getetag>`+currentETag+`</d:getetag>`, ""),
			)
		case "LOCK":
			lockRequests++
			activeLockToken = fmt.Sprintf("<opaquelocktoken:test-%d>", lockRequests)
			writer.Header().Set("Lock-Token", activeLockToken)
			writer.WriteHeader(http.StatusOK)
		case http.MethodPut:
			putRequests++
			if request.Header.Get("Content-Type") != "text/markdown; charset=utf-8" {
				t.Errorf("unexpected content type: %q", request.Header.Get("Content-Type"))
			}
			payload, _ := io.ReadAll(request.Body)
			if request.Header.Get("If") != "("+activeLockToken+")" || request.Header.Get("If-Match") != "" {
				t.Errorf("PUT did not use the DAV lock token: If=%q If-Match=%q", request.Header.Get("If"), request.Header.Get("If-Match"))
			}
			if putRequests == 1 {
				if string(payload) != "# updated" {
					t.Errorf("unexpected concurrency-safe PUT body=%q", payload)
				}
				currentETag = `"v2"`
				currentContent = string(payload)
			} else {
				if string(payload) != "# overwrite" {
					t.Errorf("unexpected explicit overwrite PUT body=%q", payload)
				}
				currentETag = `"v3"`
				currentContent = string(payload)
			}
			writer.Header().Set("ETag", `"non-canonical-put-header"`)
			writer.WriteHeader(http.StatusNoContent)
		case "UNLOCK":
			unlockRequests++
			if request.Header.Get("Lock-Token") != activeLockToken {
				t.Errorf("unexpected UNLOCK token: %q", request.Header.Get("Lock-Token"))
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := newWebDAVTestClient(t, server.URL)
	document, err := client.ReadMarkdown(context.Background(), "folder/My 文档.md")
	if err != nil {
		t.Fatal(err)
	}
	if document.Content != "# 中英文 test" || document.ETag != `"v1"` || document.RemoteDocumentID == "" {
		t.Fatalf("unexpected remote document: %#v", document)
	}
	if strings.Contains(document.DisplayLocation, webDAVTestUsername) || strings.Contains(document.DisplayLocation, webDAVTestPassword) || !strings.HasSuffix(document.DisplayLocation, "/folder/My 文档.md") {
		t.Fatalf("display location is unsafe: %q", document.DisplayLocation)
	}
	result, err := client.SaveMarkdownSession(context.Background(), document.RemoteDocumentID, "# updated", document.ETag)
	if err != nil {
		t.Fatal(err)
	}
	if result.ETag != `"v2"` || result.Path != document.Path || result.Created || result.ConcurrencyMode != "dav-lock" {
		t.Fatalf("unexpected save result: %#v", result)
	}
	if _, err := client.SaveMarkdownSession(context.Background(), document.RemoteDocumentID, "# stale", `"v1"`); !IsWebDAVErrorKind(err, WebDAVErrorConflict) {
		t.Fatalf("expected local stale ETag conflict, got %v", err)
	}
	if putRequests != 1 {
		t.Fatalf("stale local ETag reached the server: %d PUT requests", putRequests)
	}
	overwritten, err := client.OverwriteMarkdownSession(context.Background(), document.RemoteDocumentID, "# overwrite")
	if err != nil {
		t.Fatal(err)
	}
	if overwritten.ETag != `"v3"` || overwritten.ConcurrencyMode != "dav-lock-overwrite" || putRequests != 2 || lockRequests != 2 || unlockRequests != 2 {
		t.Fatalf("explicit overwrite did not refresh the ETag: %#v, PUT=%d", overwritten, putRequests)
	}
	client.CloseDocument(document.RemoteDocumentID)
	if _, err := client.SaveMarkdownSession(context.Background(), document.RemoteDocumentID, "# closed", `"v3"`); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("expected closed session to be rejected, got %v", err)
	}
}

func TestWebDAVNormalSaveRejectsServersWithoutLockBeforePut(t *testing.T) {
	var putRequests int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			writer.Header().Set("ETag", `"v1"`)
			_, _ = writer.Write([]byte("# original"))
		case "PROPFIND":
			if request.URL.Path == "/netdisk/api/webdav/" {
				writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
			} else {
				writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/document.md", `<d:resourcetype/><d:getetag>&quot;v1&quot;</d:getetag>`, ""))
			}
		case "LOCK":
			writer.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodPut:
			putRequests++
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	client := newWebDAVTestClient(t, server.URL)
	document, err := client.ReadMarkdown(context.Background(), "document.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SaveMarkdownSession(context.Background(), document.RemoteDocumentID, "# unsafe", document.ETag); !IsWebDAVErrorKind(err, WebDAVErrorUnsupported) {
		t.Fatalf("normal session save did not reject missing LOCK support: %v", err)
	}
	if _, err := client.PutMarkdown(context.Background(), "document.md", "# unsafe", WebDAVWriteOptions{IfMatch: document.ETag}); !IsWebDAVErrorKind(err, WebDAVErrorUnsupported) {
		t.Fatalf("conditional PUT did not reject missing LOCK support: %v", err)
	}
	if putRequests != 0 {
		t.Fatalf("normal save reached PUT without an exclusive lock: %d", putRequests)
	}
}

func TestWebDAVSaveReadbackDetectsCompetingWriter(t *testing.T) {
	var putRequests int
	var unlockRequests int
	putCompleted := false
	const lockToken = "<opaquelocktoken:readback-race>"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == "PROPFIND" && request.URL.Path == "/netdisk/api/webdav/" {
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
			return
		}
		switch request.Method {
		case http.MethodGet:
			if putCompleted {
				writer.Header().Set("ETag", `"rival-v3"`)
				_, _ = writer.Write([]byte("# rival writer"))
			} else {
				writer.Header().Set("ETag", `"v1"`)
				_, _ = writer.Write([]byte("# original"))
			}
		case "LOCK":
			writer.Header().Set("Lock-Token", lockToken)
			writer.WriteHeader(http.StatusOK)
		case "PROPFIND":
			etag := `"v1"`
			if putCompleted {
				etag = `"rival-v3"`
			}
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/document.md", `<d:resourcetype/><d:getetag>`+etag+`</d:getetag>`, ""))
		case http.MethodPut:
			putRequests++
			if request.Header.Get("If") != "("+lockToken+")" {
				t.Errorf("missing lock token on PUT: %q", request.Header.Get("If"))
			}
			putCompleted = true
			writer.Header().Set("ETag", `"non-canonical-put-header"`)
			writer.WriteHeader(http.StatusNoContent)
		case "UNLOCK":
			unlockRequests++
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	client := newWebDAVTestClient(t, server.URL)
	document, err := client.ReadMarkdown(context.Background(), "document.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SaveMarkdownSession(context.Background(), document.RemoteDocumentID, "# intended", document.ETag); !IsWebDAVErrorKind(err, WebDAVErrorConflict) {
		t.Fatalf("readback did not detect competing content: %v", err)
	}
	if putRequests != 1 || unlockRequests != 1 {
		t.Fatalf("unexpected race save lifecycle: PUT=%d UNLOCK=%d", putRequests, unlockRequests)
	}
	if _, err := client.SaveMarkdownSession(context.Background(), document.RemoteDocumentID, "# retry", document.ETag); !IsWebDAVErrorKind(err, WebDAVErrorConflict) {
		t.Fatalf("failed readback incorrectly advanced the session ETag: %v", err)
	}
	if putRequests != 1 || unlockRequests != 2 {
		t.Fatalf("stale retry reached PUT: PUT=%d UNLOCK=%d", putRequests, unlockRequests)
	}
}

func TestWebDAVCreateLockNullCleanupRespectsPutAcceptance(t *testing.T) {
	for _, failure := range []string{"put", "readback"} {
		t.Run(failure, func(t *testing.T) {
			resourceExists := false
			var order []string
			const lockToken = "<opaquelocktoken:lock-null-cleanup>"
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == "PROPFIND" && request.URL.Path == "/netdisk/api/webdav/" {
					writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
					return
				}
				switch request.Method {
				case "LOCK":
					order = append(order, "LOCK")
					resourceExists = true
					writer.Header().Set("Lock-Token", lockToken)
					writer.WriteHeader(http.StatusCreated)
				case http.MethodPut:
					order = append(order, "PUT")
					if failure == "put" {
						writer.WriteHeader(http.StatusInternalServerError)
						return
					}
					writer.WriteHeader(http.StatusCreated)
				case http.MethodGet:
					order = append(order, "GET")
					if failure == "put" {
						writer.WriteHeader(http.StatusNotFound)
						return
					}
					writer.Header().Set("ETag", `"rival"`)
					_, _ = writer.Write([]byte("# rival"))
				case "PROPFIND":
					writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/new.md", `<d:resourcetype/><d:getetag>&quot;rival&quot;</d:getetag>`, ""))
				case http.MethodDelete:
					order = append(order, "DELETE")
					if request.Header.Get("If") != "("+lockToken+")" {
						t.Errorf("lock-null DELETE omitted token: %q", request.Header.Get("If"))
					}
					resourceExists = false
					writer.WriteHeader(http.StatusNoContent)
				case "UNLOCK":
					order = append(order, "UNLOCK")
					if failure == "put" && resourceExists {
						t.Error("lock-null resource was unlocked before deletion")
					}
					writer.WriteHeader(http.StatusNoContent)
				default:
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()
			client := newWebDAVTestClient(t, server.URL)
			_, err := client.PutMarkdown(context.Background(), "new.md", "# intended", WebDAVWriteOptions{CreateOnly: true})
			if err == nil {
				t.Fatal("expected lock-null creation failure")
			}
			if failure == "readback" && !IsWebDAVErrorKind(err, WebDAVErrorConflict) {
				t.Fatalf("expected readback conflict, got %v", err)
			}
			if failure == "put" {
				if resourceExists || len(order) < 4 || order[len(order)-2] != "DELETE" || order[len(order)-1] != "UNLOCK" {
					t.Fatalf("unsafe lock-null cleanup order: exists=%t order=%v", resourceExists, order)
				}
			} else if !resourceExists || strings.Contains(strings.Join(order, ","), "DELETE") || order[len(order)-1] != "UNLOCK" {
				t.Fatalf("accepted PUT verification failure deleted a possibly concurrent resource: exists=%t order=%v", resourceExists, order)
			}
		})
	}
}

func TestWebDAVCreateReconcilesCommitThenTransportErrorWithoutDelete(t *testing.T) {
	resourceExists := false
	content := ""
	deleteRequests := 0
	unlockRequests := 0
	const lockToken = "<opaquelocktoken:commit-then-error>"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == "PROPFIND" && request.URL.Path == "/netdisk/api/webdav/" {
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
			return
		}
		switch request.Method {
		case "LOCK":
			writer.Header().Set("Lock-Token", lockToken)
			writer.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			payload, _ := io.ReadAll(request.Body)
			resourceExists = true
			content = string(payload)
			writer.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			if !resourceExists {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			writer.Header().Set("ETag", `"committed"`)
			_, _ = writer.Write([]byte(content))
		case "PROPFIND":
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/new.md", `<d:resourcetype/><d:getetag>&quot;committed&quot;</d:getetag>`, ""))
		case http.MethodDelete:
			deleteRequests++
			resourceExists = false
			writer.WriteHeader(http.StatusNoContent)
		case "UNLOCK":
			unlockRequests++
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	client := newWebDAVTestClient(t, server.URL)
	client.client.Transport = &commitThenErrorRoundTripper{base: client.client.Transport}
	result, err := client.PutMarkdown(context.Background(), "new.md", "# committed", WebDAVWriteOptions{CreateOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.ETag != `"committed"` || result.ConcurrencyMode != "dav-lock-create" {
		t.Fatalf("ambiguous committed write was not reconciled: %#v", result)
	}
	if !resourceExists || content != "# committed" || deleteRequests != 0 || unlockRequests != 1 {
		t.Fatalf("committed resource was deleted after transport error: exists=%t content=%q DELETE=%d UNLOCK=%d", resourceExists, content, deleteRequests, unlockRequests)
	}
}

func TestWebDAVMissingSessionTargetCleansLockNullBeforeUnlock(t *testing.T) {
	for _, operation := range []string{"save", "overwrite"} {
		t.Run(operation, func(t *testing.T) {
			missing := false
			lockNullExists := false
			var order []string
			const lockToken = "<opaquelocktoken:missing-session>"
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == "PROPFIND" && request.URL.Path == "/netdisk/api/webdav/" {
					writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
					return
				}
				switch request.Method {
				case http.MethodGet:
					writer.Header().Set("ETag", `"v1"`)
					_, _ = writer.Write([]byte("# original"))
				case "LOCK":
					order = append(order, "LOCK")
					writer.Header().Set("Lock-Token", lockToken)
					if missing {
						lockNullExists = true
						writer.WriteHeader(http.StatusCreated)
					} else {
						writer.WriteHeader(http.StatusOK)
					}
				case http.MethodDelete:
					order = append(order, "DELETE")
					lockNullExists = false
					writer.WriteHeader(http.StatusNoContent)
				case "UNLOCK":
					order = append(order, "UNLOCK")
					if lockNullExists {
						t.Error("missing session lock-null was unlocked before deletion")
					}
					writer.WriteHeader(http.StatusNoContent)
				default:
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()
			client := newWebDAVTestClient(t, server.URL)
			document, err := client.ReadMarkdown(context.Background(), "document.md")
			if err != nil {
				t.Fatal(err)
			}
			missing = true
			if operation == "save" {
				_, err = client.SaveMarkdownSession(context.Background(), document.RemoteDocumentID, "# save", document.ETag)
			} else {
				_, err = client.OverwriteMarkdownSession(context.Background(), document.RemoteDocumentID, "# overwrite")
			}
			if !IsWebDAVErrorKind(err, WebDAVErrorNotFound) {
				t.Fatalf("expected missing target error, got %v", err)
			}
			if len(order) != 3 || order[0] != "LOCK" || order[1] != "DELETE" || order[2] != "UNLOCK" || lockNullExists {
				t.Fatalf("unsafe missing-target cleanup: exists=%t order=%v", lockNullExists, order)
			}
		})
	}
}

func TestWebDAVPutCreateOnlyFetchesMissingResponseETag(t *testing.T) {
	var propfindRequests int
	var putRequests int
	var lockRequests int
	var unlockRequests int
	resourceExists := false
	content := ""
	activeLockToken := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertWebDAVTestAuth(t, request)
		if request.Method == "PROPFIND" && request.URL.Path == "/netdisk/api/webdav/" {
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
			return
		}
		if request.URL.Path != "/netdisk/api/webdav/new.md" {
			t.Errorf("unexpected path: %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		switch request.Method {
		case "LOCK":
			lockRequests++
			activeLockToken = fmt.Sprintf("<opaquelocktoken:create-%d>", lockRequests)
			writer.Header().Set("Lock-Token", activeLockToken)
			if resourceExists {
				writer.WriteHeader(http.StatusOK)
			} else {
				writer.WriteHeader(http.StatusCreated)
			}
		case http.MethodPut:
			putRequests++
			if request.Header.Get("If") != "("+activeLockToken+")" || request.Header.Get("If-None-Match") != "" || request.Header.Get("If-Match") != "" {
				t.Errorf("unexpected lock-null create preconditions: %#v", request.Header)
			}
			payload, _ := io.ReadAll(request.Body)
			content = string(payload)
			resourceExists = true
			writer.Header().Set("ETag", `"non-canonical-put-header"`)
			writer.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			if !resourceExists {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			writer.Header().Set("ETag", `"created"`)
			_, _ = writer.Write([]byte(content))
		case "PROPFIND":
			propfindRequests++
			if request.Header.Get("Depth") != "0" {
				t.Errorf("unexpected Depth: %q", request.Header.Get("Depth"))
			}
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/new.md", `<d:resourcetype/><d:getetag>&quot;created&quot;</d:getetag>`, ""))
		case "UNLOCK":
			unlockRequests++
			if request.Header.Get("Lock-Token") != activeLockToken {
				t.Errorf("unexpected create UNLOCK token: %q", request.Header.Get("Lock-Token"))
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := newWebDAVTestClient(t, server.URL)
	result, err := client.PutMarkdown(context.Background(), "new.md", "# new", WebDAVWriteOptions{CreateOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.ETag != `"created"` || result.ConcurrencyMode != "dav-lock-create" || propfindRequests != 1 || putRequests != 1 {
		t.Fatalf("missing ETag was not recovered: %#v, PROPFIND=%d", result, propfindRequests)
	}
	if _, err := client.PutMarkdown(context.Background(), "new.md", "# must not overwrite", WebDAVWriteOptions{CreateOnly: true}); !IsWebDAVErrorKind(err, WebDAVErrorConflict) {
		t.Fatalf("existing create-only target was not rejected: %v", err)
	}
	if putRequests != 1 || lockRequests != 2 || unlockRequests != 2 {
		t.Fatalf("create-only preflight trusted ignored condition headers: PUT=%d", putRequests)
	}
}

func TestWebDAVCreateDeleteAndMove(t *testing.T) {
	var methods []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertWebDAVTestAuth(t, request)
		methods = append(methods, request.Method)
		switch request.Method {
		case "MKCOL":
			if request.URL.Path != "/netdisk/api/webdav/新 folder/" {
				t.Errorf("unexpected MKCOL path: %q", request.URL.Path)
			}
			writer.WriteHeader(http.StatusCreated)
		case "MOVE":
			if request.URL.Path != "/netdisk/api/webdav/old.md" {
				t.Errorf("unexpected MOVE source: %q", request.URL.Path)
			}
			if request.Header.Get("Destination") != server.URL+"/netdisk/api/webdav/new%20name.md" {
				t.Errorf("unexpected Destination: %q", request.Header.Get("Destination"))
			}
			if request.Header.Get("Overwrite") != "F" {
				t.Errorf("unexpected Overwrite: %q", request.Header.Get("Overwrite"))
			}
			writer.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			if request.URL.Path != "/netdisk/api/webdav/new name.md" {
				t.Errorf("unexpected DELETE path: %q", request.URL.Path)
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := newWebDAVTestClient(t, server.URL)
	if err := client.CreateDirectory(context.Background(), "新 folder"); err != nil {
		t.Fatal(err)
	}
	if err := client.Move(context.Background(), "old.md", "new name.md", false); err != nil {
		t.Fatal(err)
	}
	if err := client.Delete(context.Background(), "new name.md"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, ",") != "MKCOL,MOVE,DELETE" {
		t.Fatalf("unexpected mutation methods: %v", methods)
	}
}

func TestWebDAVStatusErrorsAreClassified(t *testing.T) {
	tests := []struct {
		status int
		kind   WebDAVErrorKind
	}{
		{http.StatusUnauthorized, WebDAVErrorAuthentication},
		{http.StatusForbidden, WebDAVErrorPermission},
		{http.StatusNotFound, WebDAVErrorNotFound},
		{http.StatusConflict, WebDAVErrorConflict},
		{http.StatusPreconditionFailed, WebDAVErrorConflict},
		{http.StatusLocked, WebDAVErrorLocked},
		{http.StatusMethodNotAllowed, WebDAVErrorUnsupported},
		{http.StatusRequestEntityTooLarge, WebDAVErrorTooLarge},
		{http.StatusTooManyRequests, WebDAVErrorRateLimited},
		{http.StatusGatewayTimeout, WebDAVErrorTimeout},
		{http.StatusInternalServerError, WebDAVErrorServer},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("HTTP_%d", test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte("server response must not enter the error"))
			}))
			defer server.Close()
			client := newWebDAVTestClient(t, server.URL)
			_, err := client.ReadMarkdown(context.Background(), "document.md")
			if !IsWebDAVErrorKind(err, test.kind) {
				t.Fatalf("HTTP %d: expected %s, got %v", test.status, test.kind, err)
			}
			if strings.Contains(err.Error(), "server response") {
				t.Fatalf("response body leaked through error: %v", err)
			}
		})
	}
}

func TestWebDAVTimeoutCancellationAndRedirectSafety(t *testing.T) {
	slowServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(80 * time.Millisecond)
		writer.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()
	client, err := NewWebDAVClient(WebDAVConfig{
		Endpoint: slowServer.URL + "/netdisk/api/webdav/",
		Timeout:  15 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReadMarkdown(context.Background(), "slow.md"); !IsWebDAVErrorKind(err, WebDAVErrorTimeout) {
		t.Fatalf("expected timeout, got %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.ReadMarkdown(canceled, "canceled.md"); !IsWebDAVErrorKind(err, WebDAVErrorCanceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}

	var redirectedRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		redirectedRequests.Add(1)
		if request.Header.Get("Authorization") != "" {
			t.Error("credentials leaked to redirected origin")
		}
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	redirectClient := newWebDAVTestClient(t, redirector.URL)
	if _, err := redirectClient.ReadMarkdown(context.Background(), "redirect.md"); !IsWebDAVErrorKind(err, WebDAVErrorNetwork) {
		t.Fatalf("expected unsafe redirect to fail, got %v", err)
	}
	if redirectedRequests.Load() != 0 {
		t.Fatalf("unsafe redirect reached target %d times", redirectedRequests.Load())
	}
}

func TestWebDAVRejectsUnsafeEndpointsPathsAndBodies(t *testing.T) {
	invalidEndpoints := []string{
		"ftp://example.com/webdav/",
		"http://example.com/webdav/",
		"https://user:secret@example.com/webdav/",
		"https://example.com/webdav/?token=secret",
		"https://example.com/webdav/#fragment",
		"https://example.com/webdav/%2Fescape/",
		"https://example.com/webdav/%2e%2e/escape/",
	}
	for _, endpoint := range invalidEndpoints {
		if _, err := NewWebDAVClient(WebDAVConfig{Endpoint: endpoint}); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
			t.Errorf("expected endpoint to be rejected: %q, %v", endpoint, err)
		}
	}
	redacted := fmt.Sprintf("%+v", WebDAVConfig{
		Endpoint: "https://alice:secret@example.com/webdav/?token=hidden#fragment",
		Username: "alice",
		Password: "secret",
	})
	for _, sensitive := range []string{"alice", "secret", "hidden", "token", "fragment"} {
		if strings.Contains(redacted, sensitive) {
			t.Fatalf("configuration string leaked %q: %s", sensitive, redacted)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client := newWebDAVTestClient(t, server.URL)
	for _, unsafePath := range []string{"../escape.md", "/absolute.md", "folder//file.md", "folder\\file.md", "folder/./file.md", "https://user:secret@example.com/file.md?token=hidden"} {
		_, err := client.ReadMarkdown(context.Background(), unsafePath)
		if !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
			t.Errorf("expected path to be rejected: %q, %v", unsafePath, err)
		}
		if err != nil && strings.Contains(err.Error(), "secret") {
			t.Fatalf("invalid path leaked credentials: %v", err)
		}
	}
	if err := client.Delete(context.Background(), ""); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("expected root deletion to be rejected, got %v", err)
	}
	if err := client.Move(context.Background(), "same.md", "same.md", true); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("expected identical MOVE to be rejected, got %v", err)
	}
	if _, err := client.PutMarkdown(context.Background(), "document.md", "content", WebDAVWriteOptions{CreateOnly: true, IfMatch: `"etag"`}); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("expected contradictory write options to be rejected, got %v", err)
	}

	largeServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == "PROPFIND" {
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
			return
		}
		writer.Header().Set("Content-Length", fmt.Sprintf("%d", maxWebDAVDocumentSize+1))
		writer.WriteHeader(http.StatusOK)
	}))
	defer largeServer.Close()
	largeClient := newWebDAVTestClient(t, largeServer.URL)
	if _, err := largeClient.ReadMarkdown(context.Background(), "large.md"); !IsWebDAVErrorKind(err, WebDAVErrorTooLarge) {
		t.Fatalf("expected oversized document to be rejected, got %v", err)
	}

	invalidUTF8Server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == "PROPFIND" {
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/api/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
			return
		}
		_, _ = writer.Write([]byte{0xff, 0xfe})
	}))
	defer invalidUTF8Server.Close()
	invalidUTF8Client := newWebDAVTestClient(t, invalidUTF8Server.URL)
	if _, err := invalidUTF8Client.ReadMarkdown(context.Background(), "invalid.md"); !IsWebDAVErrorKind(err, WebDAVErrorProtocol) {
		t.Fatalf("expected invalid UTF-8 to be rejected, got %v", err)
	}
}

func TestWebDAVIntegrationFromEnvironment(t *testing.T) {
	endpoint := os.Getenv("INKMARK_WEBDAV_URL")
	username := os.Getenv("INKMARK_WEBDAV_USERNAME")
	password := os.Getenv("INKMARK_WEBDAV_PASSWORD")
	if endpoint == "" || username == "" || password == "" {
		t.Skip("WebDAV integration credentials were not supplied at runtime")
	}
	client, err := NewWebDAVClient(WebDAVConfig{
		Endpoint: endpoint,
		Username: username,
		Password: password,
		Timeout:  30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := client.CheckConnection(ctx); err != nil {
		t.Fatal(err)
	}
	uniqueName := fmt.Sprintf(".inkmark-integration-%d", time.Now().UnixNano())
	directory := uniqueName
	listingPath := ""
	originalPath := uniqueName + ".md"
	directoryCreated := false
	if err := client.CreateDirectory(ctx, directory); err == nil {
		directoryCreated = true
		listingPath = directory
		originalPath = directory + "/document.md"
	} else if !IsWebDAVErrorKind(err, WebDAVErrorUnsupported) {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = client.Delete(cleanupContext, originalPath)
		if directoryCreated {
			_ = client.Delete(cleanupContext, directory)
		}
	})
	created, err := client.PutMarkdown(ctx, originalPath, "# WebDAV integration\n", WebDAVWriteOptions{CreateOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	listing, err := client.ListDirectory(ctx, listingPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].Path != originalPath {
		t.Fatalf("unexpected integration listing: %#v", listing)
	}
	document, err := client.ReadMarkdown(ctx, originalPath)
	if err != nil {
		t.Fatal(err)
	}
	if document.Content != "# WebDAV integration\n" {
		t.Fatalf("unexpected integration content: %q", document.Content)
	}
	if created.ETag != "" && document.ETag != "" && created.ETag != document.ETag {
		t.Fatalf("created and read ETags differ: %q != %q", created.ETag, document.ETag)
	}
	updated, err := client.SaveMarkdownSession(ctx, document.RemoteDocumentID, "# WebDAV integration updated\n", document.ETag)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ETag == "" {
		t.Fatal("server did not return or advertise an ETag after save")
	}
	if _, err := client.PutMarkdown(ctx, originalPath, "# stale update\n", WebDAVWriteOptions{IfMatch: document.ETag}); !IsWebDAVErrorKind(err, WebDAVErrorConflict) {
		t.Fatalf("server did not reject a stale ETag: %v", err)
	}
	if err := client.Delete(ctx, originalPath); err != nil {
		t.Fatal(err)
	}
	if directoryCreated {
		if err := client.Delete(ctx, directory); err != nil {
			t.Fatal(err)
		}
	}
}

func newWebDAVTestClient(t *testing.T, serverURL string) *WebDAVClient {
	t.Helper()
	client, err := NewWebDAVClient(WebDAVConfig{
		Endpoint: serverURL + "/netdisk/api/webdav/",
		Username: webDAVTestUsername,
		Password: webDAVTestPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func assertWebDAVTestAuth(t *testing.T, request *http.Request) {
	t.Helper()
	username, password, ok := request.BasicAuth()
	if !ok || username != webDAVTestUsername || password != webDAVTestPassword {
		t.Errorf("WebDAV Basic authentication was not supplied")
	}
}

func writeWebDAVMultiStatus(writer http.ResponseWriter, responses ...string) {
	writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
	writer.WriteHeader(http.StatusMultiStatus)
	_, _ = writer.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:">` + strings.Join(responses, "") + `</d:multistatus>`))
}

func webDAVResponseXML(href string, successfulProperties string, missingProperties string) string {
	missing := ""
	if missingProperties != "" {
		missing = `<d:propstat><d:prop>` + missingProperties + `</d:prop><d:status>HTTP/1.1 404 Not Found</d:status></d:propstat>`
	}
	return `<d:response><d:href>` + href + `</d:href><d:propstat><d:prop>` + successfulProperties + `</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>` + missing + `</d:response>`
}

type commitThenErrorRoundTripper struct {
	base      http.RoundTripper
	triggered atomic.Bool
}

func (transport *commitThenErrorRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil || request.Method != http.MethodPut || !transport.triggered.CompareAndSwap(false, true) {
		return response, err
	}
	if response != nil {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	return nil, fmt.Errorf("simulated connection loss after server commit")
}
