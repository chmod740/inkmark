package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxWebDAVWorkspaceCapabilities = 16
	maxWebDAVDocumentCapabilities  = 64
	maxPreparedWebDAVMutations     = 16
)

type WebDAVSaveResult struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	ETag     string `json:"etag"`
	Conflict bool   `json:"conflict"`
}

type webDAVCapability struct {
	mu        sync.RWMutex
	id        string
	client    *WebDAVClient
	documents map[string]webDAVDocumentCapability
	mutations map[string]preparedWebDAVMutation
	closed    bool
}

type webDAVDocumentCapability struct {
	path string
	etag string
}

type preparedWebDAVMutation struct {
	id        string
	operation string
	source    string
	kind      string
	lockPath  string
	lockToken string
	directory bool
	revision  string
	metadata  webDAVResourceMetadata
	createdAt time.Time
}

type WebDAVMutationPreparation struct {
	MutationID string         `json:"mutationId"`
	Entry      WorkspaceEntry `json:"entry"`
	ExpiresAt  string         `json:"expiresAt"`
}

type webDAVBridgeError struct {
	kind  WebDAVErrorKind
	cause error
}

func (err *webDAVBridgeError) Error() string {
	return fmt.Sprintf("[INKMARK_WEBDAV:%s] %s", err.kind, err.cause)
}

func (err *webDAVBridgeError) Unwrap() error {
	return err.cause
}

func exposeWebDAVBridgeError(err error) error {
	if err == nil {
		return nil
	}
	var alreadyExposed *webDAVBridgeError
	if errors.As(err, &alreadyExposed) {
		return err
	}
	var webDAVError *WebDAVError
	if !errors.As(err, &webDAVError) {
		return err
	}
	return &webDAVBridgeError{kind: webDAVError.Kind, cause: err}
}

func (a *App) ConnectWebDAV(config WebDAVConfig) (bridgeWorkspace Workspace, err error) {
	return a.connectWebDAV(config, "")
}

func (a *App) connectWebDAV(config WebDAVConfig, savedConnectionID string) (bridgeWorkspace Workspace, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	a.mu.RLock()
	atCapacity := len(a.webDAVWorkspaces) >= maxWebDAVWorkspaceCapabilities
	a.mu.RUnlock()
	if atCapacity {
		return Workspace{}, webDAVCapabilityLimitError()
	}

	client, err := NewWebDAVClient(config)
	if err != nil {
		return Workspace{}, err
	}
	ctx := a.currentContext()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := client.CheckConnection(ctx); err != nil {
		client.Close()
		return Workspace{}, err
	}
	root, err := client.ListDirectory(ctx, "")
	if err != nil {
		client.Close()
		return Workspace{}, err
	}
	workspaceID, err := newOpaqueID()
	if err != nil {
		client.Close()
		return Workspace{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "connect", Err: errors.New("could not create a workspace capability")}
	}
	capability := &webDAVCapability{
		id:        workspaceID,
		client:    client,
		documents: make(map[string]webDAVDocumentCapability),
		mutations: make(map[string]preparedWebDAVMutation),
	}
	directory := webDAVWorkspaceDirectory(root)
	workspace := Workspace{
		ID:        workspaceID,
		Provider:  "webdav",
		Name:      client.baseURL.Host,
		Path:      webDAVWorkspaceLocation(client),
		Entries:   directory.Entries,
		Truncated: directory.Truncated,
	}

	a.mu.Lock()
	if len(a.webDAVWorkspaces) >= maxWebDAVWorkspaceCapabilities {
		a.mu.Unlock()
		capability.close()
		return Workspace{}, webDAVCapabilityLimitError()
	}
	if a.webDAVWorkspaces == nil {
		a.webDAVWorkspaces = make(map[string]*webDAVCapability)
	}
	a.webDAVWorkspaces[workspaceID] = capability
	a.mu.Unlock()
	// Store only the client's already-validated endpoint. recordRecentItem
	// accepts HTTPS WebDAV URLs (plus the client's supported loopback-only HTTP
	// development case) and rejects credentials, query strings and fragments.
	a.recordRecentWebDAV(client.baseURL.String(), savedConnectionID)
	return workspace, nil
}

func (a *App) ListWebDAVDirectory(workspaceID string, relativePath string) (bridgeDirectory WorkspaceDirectory, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return WorkspaceDirectory{}, err
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	if capability.closed || capability.client == nil {
		return WorkspaceDirectory{}, closedWebDAVCapabilityError("list")
	}
	directory, err := capability.client.ListDirectory(appWebDAVContext(a), relativePath)
	if err != nil {
		return WorkspaceDirectory{}, err
	}
	return webDAVWorkspaceDirectory(directory), nil
}

func (a *App) OpenWebDAVFile(workspaceID string, relativePath string) (bridgeDocument Document, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return Document{}, err
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.closed || capability.client == nil {
		return Document{}, closedWebDAVCapabilityError("read")
	}
	if len(capability.documents) >= maxWebDAVDocumentCapabilities {
		return Document{}, &WebDAVError{Kind: WebDAVErrorTooLarge, Operation: "read", Err: errors.New("too many remote documents are open")}
	}
	remote, err := capability.client.ReadMarkdown(appWebDAVContext(a), relativePath)
	if err != nil {
		return Document{}, err
	}
	if remote.RemoteDocumentID == "" {
		return Document{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "read", Err: errors.New("server document session is invalid")}
	}
	capability.documents[remote.RemoteDocumentID] = webDAVDocumentCapability{path: remote.Path, etag: remote.ETag}
	return Document{
		Path:             remote.Path,
		Name:             remote.Name,
		Content:          remote.Content,
		StorageKind:      "webdav",
		DisplayLocation:  remote.DisplayLocation,
		WorkspaceID:      capability.id,
		WorkspacePath:    remote.Path,
		RemoteDocumentID: remote.RemoteDocumentID,
		ETag:             remote.ETag,
	}, nil
}

func (a *App) SaveWebDAVFile(workspaceID string, remoteDocumentID string, content string, etag string) (bridgeResult WebDAVSaveResult, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return WebDAVSaveResult{}, err
	}
	remoteDocumentID = strings.TrimSpace(remoteDocumentID)
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.closed || capability.client == nil {
		return WebDAVSaveResult{}, closedWebDAVCapabilityError("write")
	}
	document, ok := capability.documents[remoteDocumentID]
	if !ok || remoteDocumentID == "" {
		return WebDAVSaveResult{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "write", Err: errors.New("remote document capability is invalid")}
	}
	if etag == "" {
		return WebDAVSaveResult{}, &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: "write", Path: document.path, Err: errors.New("normal remote save requires an ETag")}
	}
	if etag != document.etag {
		return webDAVConflictResult(document), nil
	}

	result, err := capability.client.SaveMarkdownSession(appWebDAVContext(a), remoteDocumentID, content, etag)
	if err != nil {
		if IsWebDAVErrorKind(err, WebDAVErrorConflict) {
			return webDAVConflictResult(document), nil
		}
		return WebDAVSaveResult{}, err
	}
	if result.Path != document.path {
		return WebDAVSaveResult{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "write", Err: errors.New("remote document path changed unexpectedly")}
	}
	document.etag = result.ETag
	capability.documents[remoteDocumentID] = document
	return WebDAVSaveResult{
		Path: result.Path,
		Name: path.Base(result.Path),
		ETag: result.ETag,
	}, nil
}

func (a *App) OverwriteWebDAVFile(workspaceID string, remoteDocumentID string, content string) (bridgeResult WebDAVSaveResult, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return WebDAVSaveResult{}, err
	}
	remoteDocumentID = strings.TrimSpace(remoteDocumentID)
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.closed || capability.client == nil {
		return WebDAVSaveResult{}, closedWebDAVCapabilityError("write")
	}
	document, ok := capability.documents[remoteDocumentID]
	if !ok || remoteDocumentID == "" {
		return WebDAVSaveResult{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "write", Err: errors.New("remote document capability is invalid")}
	}
	result, err := capability.client.OverwriteMarkdownSession(appWebDAVContext(a), remoteDocumentID, content)
	if err != nil {
		return WebDAVSaveResult{}, err
	}
	if result.Path != document.path {
		return WebDAVSaveResult{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "write", Err: errors.New("remote document path changed unexpectedly")}
	}
	document.etag = result.ETag
	capability.documents[remoteDocumentID] = document
	return WebDAVSaveResult{
		Path: result.Path,
		Name: path.Base(result.Path),
		ETag: result.ETag,
	}, nil
}

func (a *App) CloseWebDAVDocument(workspaceID string, remoteDocumentID string) (err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return err
	}
	remoteDocumentID = strings.TrimSpace(remoteDocumentID)
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.closed || capability.client == nil {
		return closedWebDAVCapabilityError("close document")
	}
	if _, ok := capability.documents[remoteDocumentID]; !ok || remoteDocumentID == "" {
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "close document", Err: errors.New("remote document capability is invalid")}
	}
	delete(capability.documents, remoteDocumentID)
	capability.client.CloseDocument(remoteDocumentID)
	return nil
}

func (a *App) CloseWebDAVWorkspace(workspaceID string) (err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	workspaceID = strings.TrimSpace(workspaceID)
	a.mu.Lock()
	capability, ok := a.webDAVWorkspaces[workspaceID]
	if ok {
		delete(a.webDAVWorkspaces, workspaceID)
	}
	a.mu.Unlock()
	if !ok || workspaceID == "" {
		return closedWebDAVCapabilityError("close")
	}
	capability.close()
	return nil
}

func (a *App) webDAVCapabilityByID(workspaceID string) (*webDAVCapability, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, closedWebDAVCapabilityError("access")
	}
	a.mu.RLock()
	capability := a.webDAVWorkspaces[workspaceID]
	a.mu.RUnlock()
	if capability == nil {
		return nil, closedWebDAVCapabilityError("access")
	}
	return capability, nil
}

func (capability *webDAVCapability) close() {
	if capability == nil {
		return
	}
	capability.mu.Lock()
	if capability.closed {
		capability.mu.Unlock()
		return
	}
	capability.closed = true
	client := capability.client
	mutations := make([]preparedWebDAVMutation, 0, len(capability.mutations))
	for _, mutation := range capability.mutations {
		mutations = append(mutations, mutation)
	}
	capability.client = nil
	capability.documents = nil
	capability.mutations = nil
	capability.mu.Unlock()
	if client != nil {
		for _, mutation := range mutations {
			client.bestEffortUnlockTarget(mutation.lockPath, mutation.directory, mutation.lockToken)
		}
		client.Close()
	}
}

func webDAVWorkspaceDirectory(directory WebDAVDirectory) WorkspaceDirectory {
	entries := make([]WorkspaceEntry, 0, len(directory.Entries))
	for _, remoteEntry := range directory.Entries {
		kind := "markdown"
		if remoteEntry.Directory {
			kind = "directory"
		} else if isImageFilename(remoteEntry.Name) {
			kind = "image"
		} else if !isMarkdownFilename(remoteEntry.Name) {
			continue
		}
		entries = append(entries, WorkspaceEntry{
			Name:     remoteEntry.Name,
			Path:     remoteEntry.Path,
			Kind:     kind,
			Revision: webDAVWorkspaceRevision(remoteEntry, kind),
		})
	}
	sort.Slice(entries, func(left, right int) bool {
		return workspaceEntryLess(entries[left], entries[right])
	})
	truncated := len(entries) > maxWorkspaceDirectoryResults
	if truncated {
		entries = entries[:maxWorkspaceDirectoryResults]
	}
	return WorkspaceDirectory{Path: directory.Path, Entries: entries, Truncated: truncated}
}

func webDAVWorkspaceRevision(entry WebDAVEntry, kind string) string {
	if kind == "directory" {
		// Collections use the locked two-phase mutation flow because common DAV
		// servers do not advertise a strong collection ETag.
		return ""
	}
	if kind != "markdown" && kind != "image" {
		return ""
	}
	etag, ok := strongWebDAVETag(entry.ETag)
	if !ok {
		return ""
	}
	return opaqueWebDAVRevision(entry.Path, kind, etag, entry.Modified, entry.Size, entry.ContentType)
}

func webDAVMetadataRevision(relativePath string, kind string, metadata webDAVResourceMetadata) string {
	return webDAVWorkspaceRevision(WebDAVEntry{
		Path:        relativePath,
		Directory:   metadata.Directory,
		Size:        metadata.Size,
		Modified:    metadata.Modified,
		ETag:        metadata.ETag,
		ContentType: metadata.ContentType,
	}, kind)
}

func strongWebDAVETag(raw string) (string, bool) {
	etag := strings.TrimSpace(raw)
	if len(etag) < 2 || etag[0] != '"' || etag[len(etag)-1] != '"' || strings.HasPrefix(strings.ToLower(etag), "w/") {
		return "", false
	}
	for _, character := range etag {
		if character < 0x20 || character == 0x7f {
			return "", false
		}
	}
	return etag, true
}

func opaqueWebDAVRevision(relativePath string, kind string, etag string, modified string, size int64, contentType string) string {
	parts := []string{relativePath, kind, etag, strings.TrimSpace(modified), strconv.FormatInt(size, 10), strings.TrimSpace(contentType)}
	var payload strings.Builder
	for _, part := range parts {
		payload.WriteString(strconv.Itoa(len(part)))
		payload.WriteByte(':')
		payload.WriteString(part)
	}
	digest := sha256.Sum256([]byte(payload.String()))
	return fmt.Sprintf("dav-v1-%x", digest[:])
}

func rebaseWebDAVDescendantPath(candidate string, source string, destination string) (string, bool) {
	if candidate == source {
		return destination, true
	}
	prefix := source + "/"
	if strings.HasPrefix(candidate, prefix) {
		return destination + strings.TrimPrefix(candidate, source), true
	}
	return candidate, false
}

func webDAVPathAtOrBelow(candidate string, target string) bool {
	return candidate == target || strings.HasPrefix(candidate, target+"/")
}

func webDAVWorkspaceLocation(client *WebDAVClient) string {
	if client == nil || client.baseURL == nil {
		return "WebDAV"
	}
	endpoint := *client.baseURL
	endpoint.User = nil
	endpoint.RawQuery = ""
	endpoint.ForceQuery = false
	endpoint.Fragment = ""
	return endpoint.String()
}

func webDAVConflictResult(document webDAVDocumentCapability) WebDAVSaveResult {
	return WebDAVSaveResult{
		Path:     document.path,
		Name:     path.Base(document.path),
		ETag:     document.etag,
		Conflict: true,
	}
}

func appWebDAVContext(a *App) context.Context {
	if a != nil {
		if ctx := a.currentContext(); ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

func closedWebDAVCapabilityError(operation string) error {
	return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: operation, Err: errors.New("WebDAV workspace capability is closed or invalid")}
}

func webDAVCapabilityLimitError() error {
	return &WebDAVError{Kind: WebDAVErrorTooLarge, Operation: "connect", Err: errors.New("too many WebDAV workspaces are open")}
}
