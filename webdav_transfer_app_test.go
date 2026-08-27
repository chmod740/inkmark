package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWebDAVWorkspaceUploadAndDownloadMarkdownAndImages(t *testing.T) {
	server, state := newWorkspaceFileOperationsDAVServer(t)
	defer server.Close()
	app := &App{}
	workspace, err := app.ConnectWebDAV(WebDAVConfig{
		Endpoint: server.URL + "/netdisk/api/webdav/",
		Username: webDAVTestUsername,
		Password: webDAVTestPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)

	temporary := t.TempDir()
	markdownPayload := []byte("# uploaded\n\n中文 + English\n")
	markdownPath := filepath.Join(temporary, "uploaded.md")
	if err := os.WriteFile(markdownPath, markdownPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	markdownEntry, err := app.uploadWebDAVWorkspaceFileFromPath(workspace.ID, "folder", markdownPath)
	if err != nil {
		t.Fatalf("upload Markdown: %v", err)
	}
	if markdownEntry.Kind != "markdown" || markdownEntry.Path != "folder/uploaded.md" || markdownEntry.Revision == "" {
		t.Fatalf("unexpected uploaded Markdown entry: %#v", markdownEntry)
	}

	imagePayload := makePNG(t, 7, 4)
	imagePath := filepath.Join(temporary, "uploaded.png")
	if err := os.WriteFile(imagePath, imagePayload, 0o600); err != nil {
		t.Fatal(err)
	}
	imageEntry, err := app.uploadWebDAVWorkspaceFileFromPath(workspace.ID, "", imagePath)
	if err != nil {
		t.Fatalf("upload image: %v", err)
	}
	if imageEntry.Kind != "image" || imageEntry.Path != "uploaded.png" {
		t.Fatalf("unexpected uploaded image entry: %#v", imageEntry)
	}

	state.mu.Lock()
	remoteMarkdown := append([]byte(nil), state.resources["folder/uploaded.md"].content...)
	remoteImage := append([]byte(nil), state.resources["uploaded.png"].content...)
	state.mu.Unlock()
	if !bytes.Equal(remoteMarkdown, markdownPayload) || !bytes.Equal(remoteImage, imagePayload) {
		t.Fatal("uploaded WebDAV representations differ from their validated local files")
	}

	downloadedMarkdown := filepath.Join(temporary, "downloaded.md")
	if _, err := app.downloadWebDAVWorkspaceFileToPath(workspace.ID, "folder/uploaded.md", downloadedMarkdown); err != nil {
		t.Fatalf("download Markdown: %v", err)
	}
	downloadedImage := filepath.Join(temporary, "downloaded.png")
	if _, err := app.downloadWebDAVWorkspaceFileToPath(workspace.ID, "uploaded.png", downloadedImage); err != nil {
		t.Fatalf("download image: %v", err)
	}
	gotMarkdown, err := os.ReadFile(downloadedMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	gotImage, err := os.ReadFile(downloadedImage)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotMarkdown, markdownPayload) || !bytes.Equal(gotImage, imagePayload) {
		t.Fatal("downloaded files differ from their WebDAV representations")
	}

	capability, err := app.webDAVCapabilityByID(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	capability.client.mu.RLock()
	openDownloadSessions := len(capability.client.sessions)
	capability.client.mu.RUnlock()
	if openDownloadSessions != 0 {
		t.Fatalf("Markdown download leaked %d remote document sessions", openDownloadSessions)
	}
}

func TestWebDAVWorkspaceTransferRejectsUnsupportedOrMismatchedLocalFiles(t *testing.T) {
	app := &App{}
	temporary := t.TempDir()
	textPath := filepath.Join(temporary, "notes.txt")
	if err := os.WriteFile(textPath, []byte("not supported"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.uploadWebDAVWorkspaceFileFromPath("unused", "", textPath); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("unsupported upload type was not rejected: %v", err)
	}

	fakeJPEG := filepath.Join(temporary, "wrong.jpg")
	if err := os.WriteFile(fakeJPEG, makePNG(t, 2, 2), 0o600); err != nil {
		t.Fatal(err)
	}
	server, _ := newWorkspaceFileOperationsDAVServer(t)
	defer server.Close()
	workspace, err := app.ConnectWebDAV(WebDAVConfig{
		Endpoint: server.URL + "/netdisk/api/webdav/",
		Username: webDAVTestUsername,
		Password: webDAVTestPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	if _, err := app.uploadWebDAVWorkspaceFileFromPath(workspace.ID, "", fakeJPEG); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("mismatched image extension was not rejected: %v", err)
	}
	if _, err := app.downloadWebDAVWorkspaceFileToPath(workspace.ID, "../escape.md", filepath.Join(temporary, "escape.md")); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) {
		t.Fatalf("traversal download path was not rejected: %v", err)
	}
}
