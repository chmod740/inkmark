package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestImageValidationAcceptsRasterMagicAndRejectsSpoofing(t *testing.T) {
	pngData := makePNG(t, 3, 2)
	jpegData := makeJPEG(t, 4, 3)
	gifData := makeGIF(t, 5, 4)
	webpData, err := base64.StdEncoding.DecodeString(testWebPBase64)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		data     []byte
		declared string
		mimeType string
		width    int
		height   int
	}{
		{name: "png", data: pngData, declared: "image/png", mimeType: "image/png", width: 3, height: 2},
		{name: "jpeg alias", data: jpegData, declared: "image/jpg", mimeType: "image/jpeg", width: 4, height: 3},
		{name: "gif", data: gifData, declared: "image/gif", mimeType: "image/gif", width: 5, height: 4},
		{name: "webp", data: webpData, declared: "image/webp", mimeType: "image/webp", width: 75, height: 100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validated, err := validateImage(test.data, "picture.anything", test.declared)
			if err != nil {
				t.Fatal(err)
			}
			if validated.mimeType != test.mimeType || validated.width != test.width || validated.height != test.height || validated.sha256 == "" {
				t.Fatalf("unexpected validated image: %#v", validated)
			}
		})
	}
	if _, err := validateImage(pngData, "spoofed.jpg", "image/jpeg"); err == nil {
		t.Fatal("declared MIME mismatch was accepted")
	}
	for _, invalid := range [][]byte{
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`),
		[]byte("not an image"),
		pngData[:20],
	} {
		if _, err := validateImage(invalid, "unsafe.svg", ""); err == nil {
			t.Fatalf("invalid or SVG image was accepted: %q", invalid)
		}
	}
	oversizedDimensions := makePNG(t, maxImageDimension+1, 1)
	if _, err := validateImage(oversizedDimensions, "wide.png", "image/png"); err == nil {
		t.Fatal("oversized image dimensions were accepted")
	}
	if _, err := decodeAndValidateImage("invalid.png", "image/png", "data:image/png;base64,AAAA"); err == nil {
		t.Fatal("a data URL was accepted where raw base64 is required")
	}
}

func TestImageValidationRejectsAnimatedRasterBombs(t *testing.T) {
	staticPNG := makePNG(t, 2, 2)
	animationControl := makePNGChunkForTest("acTL", []byte{0, 0, 0, 2, 0, 0, 0, 0})
	animatedPNG := append([]byte(nil), staticPNG[:len(staticPNG)-12]...)
	animatedPNG = append(animatedPNG, animationControl...)
	animatedPNG = append(animatedPNG, staticPNG[len(staticPNG)-12:]...)
	if _, err := validateImage(animatedPNG, "animated.png", "image/png"); err == nil {
		t.Fatal("APNG bypassed the single-frame decoded-memory limit")
	}

	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	var animatedGIF bytes.Buffer
	if err := gif.EncodeAll(&animatedGIF, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{1, 1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := validateImage(animatedGIF.Bytes(), "animated.gif", "image/gif"); err == nil {
		t.Fatal("animated GIF bypassed the single-frame decoded-memory limit")
	}

	staticWebP, err := base64.StdEncoding.DecodeString(testWebPBase64)
	if err != nil {
		t.Fatal(err)
	}
	animatedWebP := append([]byte(nil), staticWebP...)
	animatedWebP = append(animatedWebP, []byte{'A', 'N', 'I', 'M', 6, 0, 0, 0, 0, 0, 0, 0, 0, 0}...)
	binary.LittleEndian.PutUint32(animatedWebP[4:8], uint32(len(animatedWebP)-8))
	if animated, err := isAnimatedWebP(animatedWebP); err != nil || !animated {
		t.Fatalf("animated WebP container was not detected: animated=%t err=%v", animated, err)
	}
	animatedFlagWebP := append([]byte(nil), staticWebP...)
	animatedFlagWebP = append(animatedFlagWebP, []byte{'V', 'P', '8', 'X', 10, 0, 0, 0, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0}...)
	binary.LittleEndian.PutUint32(animatedFlagWebP[4:8], uint32(len(animatedFlagWebP)-8))
	if animated, err := isAnimatedWebP(animatedFlagWebP); err != nil || !animated {
		t.Fatalf("WebP animation feature flag was not detected: animated=%t err=%v", animated, err)
	}
}

func makePNGChunkForTest(chunkType string, payload []byte) []byte {
	chunk := make([]byte, 12+len(payload))
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(payload)))
	copy(chunk[4:8], chunkType)
	copy(chunk[8:], payload)
	binary.BigEndian.PutUint32(chunk[8+len(payload):], crc32.ChecksumIEEE(chunk[4:8+len(payload)]))
	return chunk
}

// A compact, lossless 75x100 WebP fixture from golang.org/x/image's test
// corpus. Keeping it inline makes this package's tests hermetic.
const testWebPBase64 = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="

func TestLocalImageImportResolveDeduplicateAndConfinement(t *testing.T) {
	directory := t.TempDir()
	documentPath := filepath.Join(directory, "中文 report.md")
	if err := os.WriteFile(documentPath, []byte("# report"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload := makePNG(t, 8, 6)
	encoded := base64.StdEncoding.EncodeToString(payload)
	app := &App{}
	asset, err := app.ImportLocalImageData(documentPath, "screenshot.png", "image/png", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(asset.MarkdownURL, "%E4%B8%AD%E6%96%87%20report.assets/") || !strings.HasSuffix(asset.MarkdownURL, ".png") || asset.MIMEType != "image/png" || asset.Width != 8 || asset.Height != 6 || len(asset.SHA256) != 64 {
		t.Fatalf("unexpected local image asset: %#v", asset)
	}
	writtenPath := filepath.Join(directory, "中文 report.assets", asset.Name)
	written, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, payload) {
		t.Fatal("imported image bytes changed")
	}
	second, err := app.ImportLocalImageData(documentPath, "different-name.png", "image/png", encoded)
	if err != nil || second.MarkdownURL != asset.MarkdownURL {
		t.Fatalf("content-addressed duplicate was not reused: %#v, %v", second, err)
	}
	matches, err := filepath.Glob(filepath.Join(directory, "中文 report.assets", ".inkmark-image-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary image files leaked: %#v, %v", matches, err)
	}
	resolved, err := app.ResolveLocalImage(documentPath, asset.MarkdownURL+"#preview")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(resolved.DataBase64)
	if err != nil || !bytes.Equal(decoded, payload) || resolved.SHA256 != asset.SHA256 || resolved.MIMEType != asset.MIMEType {
		t.Fatalf("resolved local image changed: %#v, %v", resolved, err)
	}

	outside := filepath.Join(filepath.Dir(directory), "outside-image.png")
	if err := os.WriteFile(outside, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outside)
	for _, source := range []string{"../outside-image.png", "/etc/passwd", "https://example.com/image.png", "folder/%2e%2e/image.png", "folder%2Fimage.png", "image.png?token=secret"} {
		if _, err := app.ResolveLocalImage(documentPath, source); err == nil {
			t.Fatalf("unsafe local image source was accepted: %q", source)
		}
	}

	if runtime.GOOS != "windows" {
		linked := filepath.Join(directory, "linked.png")
		if err := os.Symlink(writtenPath, linked); err != nil {
			t.Fatal(err)
		}
		if _, err := app.ResolveLocalImage(documentPath, "linked.png"); err == nil {
			t.Fatal("symlinked local image was accepted")
		}
		linkedDocument := filepath.Join(directory, "linked.md")
		if err := os.Symlink(documentPath, linkedDocument); err != nil {
			t.Fatal(err)
		}
		if _, err := app.ImportLocalImageData(linkedDocument, "image.png", "image/png", encoded); err == nil {
			t.Fatal("symlinked Markdown document was accepted")
		}
	}
}

func TestReadValidatedImageFileRejectsSpecialFiles(t *testing.T) {
	directory := t.TempDir()
	regular := filepath.Join(directory, "regular.png")
	if err := os.WriteFile(regular, makePNG(t, 2, 2), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := readValidatedImageFile(regular)
	if err != nil || data.name != "regular.png" {
		t.Fatalf("regular image could not be selected: %#v, %v", data, err)
	}
	if _, err := readValidatedImageFile(directory); err == nil {
		t.Fatal("directory was accepted as an image")
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(directory, "link.png")
		if err := os.Symlink(regular, link); err != nil {
			t.Fatal(err)
		}
		if _, err := readValidatedImageFile(link); err == nil {
			t.Fatal("symlink was accepted as a selected image")
		}
	}
}

func TestWebDAVImageUploadResolveDeduplicateAndCapabilityIsolation(t *testing.T) {
	payload := makePNG(t, 7, 5)
	server, state := newImageWebDAVServer(t)
	defer server.Close()
	client, err := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL + "/webdav/"})
	if err != nil {
		t.Fatal(err)
	}
	capability := &webDAVCapability{
		id:     "workspace-image-capability",
		client: client,
		documents: map[string]webDAVDocumentCapability{
			"remote-document-capability": {path: "folder/report.md", etag: `"v1"`},
		},
	}
	app := &App{webDAVWorkspaces: map[string]*webDAVCapability{capability.id: capability}}
	encoded := base64.StdEncoding.EncodeToString(payload)
	asset, err := app.ImportWebDAVImageData(capability.id, "remote-document-capability", "chart.png", "image/png", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(asset.MarkdownURL, "report.assets/") || asset.MIMEType != "image/png" || asset.Width != 7 || asset.Height != 5 {
		t.Fatalf("unexpected WebDAV image asset: %#v", asset)
	}
	if got := state.putCount(); got != 1 {
		t.Fatalf("expected one binary PUT, got %d", got)
	}
	second, err := app.ImportWebDAVImageData(capability.id, "remote-document-capability", "chart-copy.png", "image/png", encoded)
	if err != nil || second.MarkdownURL != asset.MarkdownURL || state.putCount() != 1 {
		t.Fatalf("existing content-addressed WebDAV image was not reused: %#v, %v, PUT=%d", second, err, state.putCount())
	}
	resolved, err := app.ResolveWebDAVImage(capability.id, "remote-document-capability", asset.MarkdownURL)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(resolved.DataBase64)
	if err != nil || !bytes.Equal(decoded, payload) || resolved.SHA256 != asset.SHA256 {
		t.Fatalf("resolved WebDAV image changed: %#v, %v", resolved, err)
	}

	requestsBefore := state.requestCount()
	for _, source := range []string{"../secret.png", "/root.png", "https://example.com/x.png", "report.assets%2Fimage.png"} {
		if _, err := app.ResolveWebDAVImage(capability.id, "remote-document-capability", source); err == nil {
			t.Fatalf("unsafe remote image source was accepted: %q", source)
		}
	}
	if state.requestCount() != requestsBefore {
		t.Fatal("unsafe remote image paths reached the WebDAV server")
	}
	if _, err := app.ResolveWebDAVImage("wrong-workspace", "remote-document-capability", asset.MarkdownURL); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("workspace capability isolation failed: %v", err)
	}
	if _, err := app.ResolveWebDAVImage(capability.id, "wrong-document", asset.MarkdownURL); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("document capability isolation failed: %v", err)
	}
	capability.close()
}

func TestWebDAVImageUploadRequiresSafeLockNullCreation(t *testing.T) {
	var putCount int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case "PROPFIND":
			if request.URL.Path == "/webdav/" {
				writeWebDAVMultiStatus(writer, webDAVResponseXML("/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
				return
			}
			if strings.HasSuffix(request.URL.Path, ".assets/") {
				writeWebDAVMultiStatus(writer, webDAVResponseXML(request.URL.EscapedPath(), `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
				return
			}
			writer.WriteHeader(http.StatusNotFound)
		case "LOCK":
			writer.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodPut:
			putCount++
			writer.WriteHeader(http.StatusCreated)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	client, err := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL + "/webdav/"})
	if err != nil {
		t.Fatal(err)
	}
	capability := &webDAVCapability{id: "workspace", client: client, documents: map[string]webDAVDocumentCapability{"document": {path: "report.md"}}}
	app := &App{webDAVWorkspaces: map[string]*webDAVCapability{capability.id: capability}}
	_, err = app.ImportWebDAVImageData(capability.id, "document", "image.png", "image/png", base64.StdEncoding.EncodeToString(makePNG(t, 2, 2)))
	if !IsWebDAVErrorKind(err, WebDAVErrorUnsupported) || putCount != 0 {
		t.Fatalf("unsafe WebDAV server reached PUT: err=%v PUT=%d", err, putCount)
	}
	capability.close()
}

func TestWebDAVImageCommitThenTransportErrorUsesReadbackWithoutDelete(t *testing.T) {
	payload := makePNG(t, 4, 3)
	var mu sync.Mutex
	var committed []byte
	deleteCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case "PROPFIND":
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
		case "LOCK":
			_, _ = io.Copy(io.Discard, request.Body)
			writer.Header().Set("Lock-Token", "<opaquelocktoken:commit-then-error>")
			writer.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			body, _ := io.ReadAll(request.Body)
			mu.Lock()
			committed = append([]byte(nil), body...)
			mu.Unlock()
			hijacker, ok := writer.(http.Hijacker)
			if !ok {
				t.Error("test server does not support connection hijacking")
				return
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("hijack failed: %v", err)
				return
			}
			_ = connection.Close()
		case http.MethodGet:
			mu.Lock()
			body := append([]byte(nil), committed...)
			mu.Unlock()
			if len(body) == 0 {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = writer.Write(body)
		case http.MethodDelete:
			mu.Lock()
			deleteCount++
			mu.Unlock()
			writer.WriteHeader(http.StatusNoContent)
		case "UNLOCK":
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	client, err := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL + "/webdav/"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	imageData, err := validateImage(payload, "committed.png", "image/png")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.PutImageCreateOnly(context.Background(), "assets/committed.png", imageData)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	deletes := deleteCount
	stored := append([]byte(nil), committed...)
	mu.Unlock()
	if !result.Created || !bytes.Equal(stored, payload) || deletes != 0 {
		t.Fatalf("commit-then-error was not reconciled safely: result=%#v DELETE=%d", result, deletes)
	}
}

func TestWebDAVImageAcceptedPutIsNeverDeletedWhenVerificationFails(t *testing.T) {
	for _, mode := range []string{"get-failure", "context-canceled"} {
		t.Run(mode, func(t *testing.T) {
			payload := makePNG(t, 3, 2)
			var mu sync.Mutex
			deleteCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.Method {
				case "PROPFIND":
					writeWebDAVMultiStatus(writer, webDAVResponseXML("/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
				case "LOCK":
					_, _ = io.Copy(io.Discard, request.Body)
					writer.Header().Set("Lock-Token", "<opaquelocktoken:accepted-put>")
					writer.WriteHeader(http.StatusCreated)
				case http.MethodPut:
					_, _ = io.Copy(io.Discard, request.Body)
					writer.WriteHeader(http.StatusCreated)
				case http.MethodGet:
					writer.WriteHeader(http.StatusServiceUnavailable)
				case http.MethodDelete:
					mu.Lock()
					deleteCount++
					mu.Unlock()
					writer.WriteHeader(http.StatusNoContent)
				case "UNLOCK":
					writer.WriteHeader(http.StatusNoContent)
				default:
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()
			client, err := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL + "/webdav/"})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ctx := context.Background()
			if mode == "context-canceled" {
				cancelable, cancel := context.WithCancel(context.Background())
				ctx = cancelable
				baseTransport := client.client.Transport
				client.client.Transport = imageTestRoundTripper(func(request *http.Request) (*http.Response, error) {
					response, roundTripErr := baseTransport.RoundTrip(request)
					if request.Method == http.MethodPut && roundTripErr == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
						cancel()
					}
					return response, roundTripErr
				})
			}
			imageData, err := validateImage(payload, "accepted.png", "image/png")
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.PutImageCreateOnly(ctx, "assets/accepted.png", imageData)
			if err == nil {
				t.Fatal("failed verification was reported as success")
			}
			mu.Lock()
			deletes := deleteCount
			mu.Unlock()
			if deletes != 0 {
				t.Fatalf("accepted PUT was destructively cleaned up after %s: DELETE=%d", mode, deletes)
			}
		})
	}
}

func TestWebDAVImageZeroByteLockNullIsDeletedBeforeUnlockAndRetrySucceeds(t *testing.T) {
	for _, failureMode := range []string{"http-rejected", "transport-unknown"} {
		t.Run(failureMode, func(t *testing.T) {
			payload := makePNG(t, 5, 3)
			var mu sync.Mutex
			resourceExists := false
			var stored []byte
			firstServerPut := true
			order := make([]string, 0, 8)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.Method {
				case "PROPFIND":
					writeWebDAVMultiStatus(writer, webDAVResponseXML("/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
				case "LOCK":
					_, _ = io.Copy(io.Discard, request.Body)
					mu.Lock()
					created := !resourceExists
					if created {
						resourceExists = true
						stored = nil // A real DAV lock-null is readable as HTTP 200, zero bytes.
					}
					order = append(order, "LOCK")
					mu.Unlock()
					writer.Header().Set("Lock-Token", "<opaquelocktoken:zero-byte>")
					if created {
						writer.WriteHeader(http.StatusCreated)
					} else {
						writer.WriteHeader(http.StatusOK)
					}
				case http.MethodPut:
					body, _ := io.ReadAll(request.Body)
					mu.Lock()
					order = append(order, "PUT")
					if failureMode == "http-rejected" && firstServerPut {
						firstServerPut = false
						mu.Unlock()
						writer.WriteHeader(http.StatusInternalServerError)
						return
					}
					firstServerPut = false
					stored = append([]byte(nil), body...)
					resourceExists = true
					mu.Unlock()
					writer.WriteHeader(http.StatusCreated)
				case http.MethodGet:
					mu.Lock()
					exists := resourceExists
					body := append([]byte(nil), stored...)
					order = append(order, "GET")
					mu.Unlock()
					if !exists {
						writer.WriteHeader(http.StatusNotFound)
						return
					}
					writer.Header().Set("Content-Length", fmt.Sprint(len(body)))
					writer.WriteHeader(http.StatusOK)
					_, _ = writer.Write(body)
				case http.MethodDelete:
					mu.Lock()
					order = append(order, "DELETE")
					resourceExists = false
					stored = nil
					mu.Unlock()
					writer.WriteHeader(http.StatusNoContent)
				case "UNLOCK":
					mu.Lock()
					order = append(order, "UNLOCK")
					mu.Unlock()
					writer.WriteHeader(http.StatusNoContent)
				default:
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()
			client, err := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL + "/webdav/"})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if failureMode == "transport-unknown" {
				baseTransport := client.client.Transport
				firstTransportPut := true
				client.client.Transport = imageTestRoundTripper(func(request *http.Request) (*http.Response, error) {
					if request.Method == http.MethodPut && firstTransportPut {
						firstTransportPut = false
						return nil, errors.New("simulated ambiguous transport failure")
					}
					return baseTransport.RoundTrip(request)
				})
			}
			imageData, err := validateImage(payload, "lock-null.png", "image/png")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.PutImageCreateOnly(context.Background(), "assets/lock-null.png", imageData); err == nil {
				t.Fatal("initial failed PUT was reported as success")
			}
			mu.Lock()
			firstOrder := append([]string(nil), order...)
			stillExists := resourceExists
			mu.Unlock()
			if len(firstOrder) < 4 || firstOrder[len(firstOrder)-2] != "DELETE" || firstOrder[len(firstOrder)-1] != "UNLOCK" || stillExists {
				t.Fatalf("zero-byte lock-null was not cleaned before unlock: order=%v exists=%t", firstOrder, stillExists)
			}

			result, err := client.PutImageCreateOnly(context.Background(), "assets/lock-null.png", imageData)
			if err != nil {
				t.Fatalf("retry after lock-null cleanup failed: %v", err)
			}
			mu.Lock()
			readBack := append([]byte(nil), stored...)
			finalOrder := append([]string(nil), order...)
			mu.Unlock()
			if !result.Created || !bytes.Equal(readBack, payload) || finalOrder[len(finalOrder)-1] != "UNLOCK" {
				t.Fatalf("retry did not create and verify the image: result=%#v order=%v", result, finalOrder)
			}
		})
	}
}

type imageTestRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper imageTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

// TestWebDAVImageRealIntegration is opt-in because it creates and removes a
// temporary collection on the configured WebDAV service. No credentials or
// endpoint are embedded in the source tree or test output.
func TestWebDAVImageRealIntegration(t *testing.T) {
	if os.Getenv("INKMARK_REAL_WEBDAV") != "1" {
		t.Skip("set INKMARK_REAL_WEBDAV=1 and the INKMARK_WEBDAV_TEST_* variables to run")
	}
	endpoint := strings.TrimSpace(os.Getenv("INKMARK_WEBDAV_TEST_ENDPOINT"))
	username := os.Getenv("INKMARK_WEBDAV_TEST_USERNAME")
	password := os.Getenv("INKMARK_WEBDAV_TEST_PASSWORD")
	if endpoint == "" {
		t.Fatal("INKMARK_WEBDAV_TEST_ENDPOINT is required")
	}
	client, err := NewWebDAVClient(WebDAVConfig{Endpoint: endpoint, Username: username, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if err := client.CheckConnection(context.Background()); err != nil {
		t.Fatal(err)
	}
	identifier, err := newOpaqueID()
	if err != nil {
		t.Fatal(err)
	}
	testDirectory := "inkmark-image-integration-" + identifier
	createdPaths := []string{testDirectory}
	t.Cleanup(func() {
		for index := len(createdPaths) - 1; index >= 0; index-- {
			_ = client.Delete(context.Background(), createdPaths[index])
		}
	})
	if err := client.CreateDirectory(context.Background(), testDirectory); err != nil {
		t.Fatal(err)
	}
	documentPath := testDirectory + "/rendering.md"
	if _, err := client.PutMarkdown(context.Background(), documentPath, "# image integration\n", WebDAVWriteOptions{CreateOnly: true}); err != nil {
		t.Fatal(err)
	}
	createdPaths = append(createdPaths, documentPath)
	document, err := client.ReadMarkdown(context.Background(), documentPath)
	if err != nil {
		t.Fatal(err)
	}
	capability := &webDAVCapability{
		id:     "real-webdav-image-workspace",
		client: client,
		documents: map[string]webDAVDocumentCapability{
			document.RemoteDocumentID: {path: document.Path, etag: document.ETag},
		},
	}
	app := &App{webDAVWorkspaces: map[string]*webDAVCapability{capability.id: capability}}
	payload := makePNG(t, 9, 7)
	asset, err := app.ImportWebDAVImageData(capability.id, document.RemoteDocumentID, "integration.png", "image/png", base64.StdEncoding.EncodeToString(payload))
	if err != nil {
		t.Fatal(err)
	}
	assetDirectory := testDirectory + "/rendering.assets"
	createdPaths = append(createdPaths, assetDirectory, assetDirectory+"/"+asset.Name)
	resolved, err := app.ResolveWebDAVImage(capability.id, document.RemoteDocumentID, asset.MarkdownURL)
	if err != nil {
		t.Fatal(err)
	}
	readBack, err := base64.StdEncoding.DecodeString(resolved.DataBase64)
	if err != nil || !bytes.Equal(readBack, payload) || resolved.SHA256 != asset.SHA256 {
		t.Fatalf("real WebDAV image readback did not match: metadata=%#v error=%v", resolved, err)
	}
	client.CloseDocument(document.RemoteDocumentID)
}

func TestPublicImageURLAndAddressRestrictions(t *testing.T) {
	valid, err := validatePublicImageURL("https://Example.COM:443/assets/picture.png?width=640")
	if err != nil {
		t.Fatal(err)
	}
	if valid.Host != "example.com" || valid.RawQuery != "width=640" {
		t.Fatalf("public URL was not safely normalized: %s", valid)
	}
	for _, rawURL := range []string{
		"http://example.com/image.png",
		"https://user:password@example.com/image.png",
		"https://example.com:8443/image.png",
		"https://localhost/image.png",
		"https://files.internal/image.png",
		"https://example.com/image.png#fragment",
	} {
		if _, err := validatePublicImageURL(rawURL); err == nil {
			t.Fatalf("unsafe public image URL was accepted: %q", rawURL)
		}
	}
	publicAddresses := []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888"}
	for _, raw := range publicAddresses {
		if !isPublicImageIP(net.ParseIP(raw)) {
			t.Fatalf("public address was rejected: %s", raw)
		}
	}
	privateAddresses := []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.0",
		"192.0.2.1", "192.31.196.1", "192.52.193.1", "192.88.99.1", "192.175.48.1",
		"198.18.0.1", "198.51.100.1", "203.0.113.1",
		"::1", "::10.0.0.1", "fc00::1", "fe80::1", "fec0::1", "64:ff9b::c0a8:101", "64:ff9b:1::1", "100::1",
		"100:0:0:1::1", "2001::1", "2001:db8::1", "2002:c0a8:101::1", "3fff::1", "5f00::1", "2620:4f:8000::1",
	}
	for _, raw := range privateAddresses {
		if isPublicImageIP(net.ParseIP(raw)) {
			t.Fatalf("non-public address was accepted: %s", raw)
		}
	}
	if _, err := dialPublicImageAddress(context.Background(), "tcp", "127.0.0.1:443"); err == nil {
		t.Fatal("loopback address reached the public image dialer")
	}
	if _, err := fetchPublicImage(context.Background(), "https://127.0.0.1/image.png"); err == nil {
		t.Fatal("public image fetch accepted a loopback destination")
	}
}

func TestValidateImageDataUsesNativeImageLimits(t *testing.T) {
	payload := makePNG(t, 11, 7)
	app := &App{}
	validated, err := app.ValidateImageData("embedded.png", "image/png", base64.StdEncoding.EncodeToString(payload))
	if err != nil {
		t.Fatal(err)
	}
	if validated.Name != "embedded.png" || validated.MIMEType != "image/png" || validated.Width != 11 || validated.Height != 7 || validated.Size != int64(len(payload)) || validated.DataBase64 == "" {
		t.Fatalf("unexpected validated Data URI payload: %#v", validated)
	}
	if _, err := app.ValidateImageData("fake.png", "image/png", base64.StdEncoding.EncodeToString([]byte("not an image"))); err == nil {
		t.Fatal("invalid embedded image bypassed native validation")
	}
	if _, err := app.ValidateImageData("vector.svg", "image/svg+xml", base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))); err == nil {
		t.Fatal("SVG embedded image bypassed native validation")
	}
}

type imageWebDAVServerState struct {
	mu        sync.Mutex
	directory bool
	image     []byte
	puts      int
	requests  int
	lockToken string
}

func (state *imageWebDAVServerState) putCount() int {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.puts
}

func (state *imageWebDAVServerState) requestCount() int {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.requests
}

func newImageWebDAVServer(t *testing.T) (*httptest.Server, *imageWebDAVServerState) {
	t.Helper()
	state := &imageWebDAVServerState{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		state.mu.Lock()
		state.requests++
		state.mu.Unlock()
		if request.Method == "PROPFIND" && request.URL.Path == "/webdav/" {
			writeWebDAVMultiStatus(writer, webDAVResponseXML("/webdav/", `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
			return
		}
		assetDirectory := "/webdav/folder/report.assets/"
		if request.Method == "PROPFIND" && request.URL.Path == assetDirectory {
			state.mu.Lock()
			exists := state.directory
			state.mu.Unlock()
			if !exists {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			writeWebDAVMultiStatus(writer, webDAVResponseXML(assetDirectory, `<d:resourcetype><d:collection/></d:resourcetype>`, ""))
			return
		}
		if request.Method == "MKCOL" && request.URL.Path == assetDirectory {
			state.mu.Lock()
			state.directory = true
			state.mu.Unlock()
			writer.WriteHeader(http.StatusCreated)
			return
		}
		if !strings.HasPrefix(request.URL.Path, assetDirectory) || !strings.HasSuffix(request.URL.Path, ".png") {
			t.Errorf("unexpected image WebDAV request: %s %s", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		switch request.Method {
		case "LOCK":
			state.mu.Lock()
			created := len(state.image) == 0
			state.lockToken = "<opaquelocktoken:image-test>"
			state.mu.Unlock()
			writer.Header().Set("Lock-Token", "<opaquelocktoken:image-test>")
			if created {
				writer.WriteHeader(http.StatusCreated)
			} else {
				writer.WriteHeader(http.StatusOK)
			}
		case http.MethodPut:
			if request.Header.Get("Content-Type") != "image/png" || request.Header.Get("If") != "(<opaquelocktoken:image-test>)" {
				t.Errorf("binary PUT headers were unsafe: Content-Type=%q If=%q", request.Header.Get("Content-Type"), request.Header.Get("If"))
			}
			payload, _ := io.ReadAll(request.Body)
			state.mu.Lock()
			state.image = append([]byte(nil), payload...)
			state.puts++
			state.mu.Unlock()
			writer.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			state.mu.Lock()
			payload := append([]byte(nil), state.image...)
			state.mu.Unlock()
			if len(payload) == 0 {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			writer.Header().Set("Content-Type", "application/octet-stream")
			writer.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			_, _ = writer.Write(payload)
		case "UNLOCK":
			if request.Header.Get("Lock-Token") != "<opaquelocktoken:image-test>" {
				t.Errorf("unexpected image unlock token: %q", request.Header.Get("Lock-Token"))
			}
			writer.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			state.mu.Lock()
			state.image = nil
			state.mu.Unlock()
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	return server, state
}

func makePNG(t *testing.T, width int, height int) []byte {
	t.Helper()
	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 0x80, A: 0xff})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func makeJPEG(t *testing.T, width int, height int) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	var output bytes.Buffer
	if err := jpeg.Encode(&output, canvas, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func makeGIF(t *testing.T, width int, height int) []byte {
	t.Helper()
	canvas := image.NewPaletted(image.Rect(0, 0, width, height), color.Palette{color.Black, color.White})
	var output bytes.Buffer
	if err := gif.Encode(&output, canvas, nil); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
