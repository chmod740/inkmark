package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// UploadWebDAVWorkspaceFile selects and safely creates a Markdown document or
// validated image in the requested WebDAV directory. Local paths and bytes do
// not cross the Wails bridge.
func (a *App) UploadWebDAVWorkspaceFile(workspaceID string, parentPath string) (bridgeEntry WorkspaceEntry, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	english := a.currentLocale() == "en"
	title := "上传 Markdown 或图片到 WebDAV"
	supportedFilter := "Markdown 和图片 (*.md;*.markdown;*.png;*.jpg;*.jpeg;*.gif;*.webp)"
	if english {
		title = "Upload Markdown or image to WebDAV"
		supportedFilter = "Markdown and images (*.md;*.markdown;*.png;*.jpg;*.jpeg;*.gif;*.webp)"
	}
	selectedPath, err := runtime.OpenFileDialog(a.currentContext(), runtime.OpenDialogOptions{
		Title: title,
		Filters: []runtime.FileFilter{
			{DisplayName: supportedFilter, Pattern: "*.md;*.markdown;*.png;*.jpg;*.jpeg;*.gif;*.webp"},
		},
	})
	if err != nil {
		return WorkspaceEntry{}, fmt.Errorf("open upload dialog: %w", err)
	}
	if strings.TrimSpace(selectedPath) == "" {
		return WorkspaceEntry{}, nil
	}
	return a.uploadWebDAVWorkspaceFileFromPath(workspaceID, parentPath, selectedPath)
}

func (a *App) uploadWebDAVWorkspaceFileFromPath(workspaceID string, parentPath string, selectedPath string) (WorkspaceEntry, error) {
	parent, err := normalizeWebDAVPath(parentPath)
	if err != nil {
		return WorkspaceEntry{}, invalidWebDAVPathError("upload file", parentPath, err)
	}
	filename := filepath.Base(selectedPath)
	target, err := normalizeNonRootWebDAVPath(path.Join(parent, filename))
	if err != nil {
		return WorkspaceEntry{}, invalidWebDAVPathError("upload file", filename, err)
	}
	if !isMarkdownFilename(filename) && !isImageFilename(filename) {
		return WorkspaceEntry{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "upload file", Path: target, Err: errors.New("only Markdown files and supported images can be uploaded")}
	}

	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return WorkspaceEntry{}, err
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.closed || capability.client == nil {
		return WorkspaceEntry{}, closedWebDAVCapabilityError("upload file")
	}
	ctx := appWebDAVContext(a)
	if isMarkdownFilename(filename) {
		document, readErr := readDocument(selectedPath)
		if readErr != nil {
			return WorkspaceEntry{}, &WebDAVError{Kind: WebDAVErrorLocalStorage, Operation: "upload file", Path: target, Err: readErr}
		}
		result, writeErr := capability.client.PutMarkdown(ctx, target, document.Content, WebDAVWriteOptions{CreateOnly: true})
		if writeErr != nil {
			return WorkspaceEntry{}, writeErr
		}
		return WorkspaceEntry{Name: filename, Path: target, Kind: "markdown", Revision: result.ETag}, nil
	}

	imageData, err := readValidatedImageFile(selectedPath)
	if err != nil {
		return WorkspaceEntry{}, &WebDAVError{Kind: WebDAVErrorLocalStorage, Operation: "upload file", Path: target, Err: err}
	}
	if !sameImageExtensionFamily(filename, "image"+imageData.extension) {
		return WorkspaceEntry{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "upload file", Path: target, Err: errors.New("image filename extension does not match its content")}
	}
	if _, err := capability.client.PutImageCreateOnly(ctx, target, imageData); err != nil {
		return WorkspaceEntry{}, err
	}
	return WorkspaceEntry{Name: filename, Path: target, Kind: "image"}, nil
}

// DownloadWebDAVWorkspaceFile retrieves a validated Markdown document or image
// and writes it only after the user chooses a destination in the native dialog.
func (a *App) DownloadWebDAVWorkspaceFile(workspaceID string, relativePath string) (result SaveResult, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	normalized, err := normalizeNonRootWebDAVPath(relativePath)
	if err != nil {
		return SaveResult{}, invalidWebDAVPathError("download file", relativePath, err)
	}
	isMarkdown := isMarkdownFilename(normalized)
	isImage := isImageFilename(normalized)
	if !isMarkdown && !isImage {
		return SaveResult{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "download file", Path: normalized, Err: errors.New("only Markdown files and supported images can be downloaded")}
	}

	english := a.currentLocale() == "en"
	title := "下载 WebDAV 文件"
	filterName := "Markdown 文件 (*.md;*.markdown)"
	filterPattern := "*.md;*.markdown"
	if isImage {
		filterName = "图片文件 (*.png;*.jpg;*.jpeg;*.gif;*.webp)"
		filterPattern = "*.png;*.jpg;*.jpeg;*.gif;*.webp"
	}
	if english {
		title = "Download WebDAV file"
		if isImage {
			filterName = "Image files (*.png;*.jpg;*.jpeg;*.gif;*.webp)"
		} else {
			filterName = "Markdown files (*.md;*.markdown)"
		}
	}
	destination, err := runtime.SaveFileDialog(a.currentContext(), runtime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: path.Base(normalized),
		Filters: []runtime.FileFilter{
			{DisplayName: filterName, Pattern: filterPattern},
		},
	})
	if err != nil {
		return SaveResult{}, fmt.Errorf("open download dialog: %w", err)
	}
	if strings.TrimSpace(destination) == "" {
		return SaveResult{}, nil
	}
	if filepath.Ext(destination) == "" {
		destination += filepath.Ext(normalized)
	}
	return a.downloadWebDAVWorkspaceFileToPath(workspaceID, normalized, destination)
}

func (a *App) downloadWebDAVWorkspaceFileToPath(workspaceID string, normalized string, destination string) (SaveResult, error) {
	normalized, err := normalizeNonRootWebDAVPath(normalized)
	if err != nil {
		return SaveResult{}, invalidWebDAVPathError("download file", normalized, err)
	}
	isMarkdown := isMarkdownFilename(normalized)
	if !isMarkdown && !isImageFilename(normalized) {
		return SaveResult{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "download file", Path: normalized, Err: errors.New("unsupported download file type")}
	}
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return SaveResult{}, err
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	if capability.closed || capability.client == nil {
		return SaveResult{}, closedWebDAVCapabilityError("download file")
	}
	var payload []byte
	if isMarkdown {
		remote, readErr := capability.client.ReadMarkdown(appWebDAVContext(a), normalized)
		if readErr != nil {
			return SaveResult{}, readErr
		}
		capability.client.CloseDocument(remote.RemoteDocumentID)
		payload = []byte(remote.Content)
	} else {
		imageData, readErr := capability.client.ReadImage(appWebDAVContext(a), normalized)
		if readErr != nil {
			return SaveResult{}, readErr
		}
		payload = imageData.data
	}
	return writeDownloadedWebDAVFile(destination, payload)
}

func writeDownloadedWebDAVFile(filename string, payload []byte) (SaveResult, error) {
	if len(payload) > maxDocumentSize {
		return SaveResult{}, errors.New("downloaded file exceeds the size limit")
	}
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return SaveResult{}, fmt.Errorf("resolve download path: %w", err)
	}
	if err := os.WriteFile(absolute, payload, 0o644); err != nil {
		return SaveResult{}, fmt.Errorf("write downloaded file: %w", err)
	}
	return SaveResult{Path: absolute, Name: filepath.Base(absolute)}, nil
}
