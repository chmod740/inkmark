package main

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"
)

func (a *App) CreateWebDAVMarkdownFile(workspaceID string, relativePath string) (bridgeDocument Document, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return Document{}, err
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.closed || capability.client == nil {
		return Document{}, closedWebDAVCapabilityError("create document")
	}
	if len(capability.documents) >= maxWebDAVDocumentCapabilities {
		return Document{}, &WebDAVError{Kind: WebDAVErrorTooLarge, Operation: "create document", Err: errors.New("too many remote documents are open")}
	}
	normalized, err := normalizeWebDAVMarkdownPath(relativePath)
	if err != nil {
		return Document{}, invalidWebDAVPathError("create document", relativePath, err)
	}
	ctx := appWebDAVContext(a)
	result, err := capability.client.PutMarkdown(ctx, normalized, "", WebDAVWriteOptions{CreateOnly: true})
	if err != nil {
		return Document{}, err
	}
	remote, err := capability.client.registerCreatedMarkdownSession(normalized, result.ETag)
	if err != nil {
		return Document{}, err
	}
	if remote.RemoteDocumentID == "" {
		return Document{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "create document", Path: normalized, Err: errors.New("server document session is invalid")}
	}
	capability.documents[remote.RemoteDocumentID] = webDAVDocumentCapability{path: remote.Path, etag: remote.ETag}
	return webDAVBridgeDocument(capability.id, remote), nil
}

func (a *App) CreateWebDAVDirectory(workspaceID string, relativePath string) (bridgeEntry WorkspaceEntry, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return WorkspaceEntry{}, err
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.closed || capability.client == nil {
		return WorkspaceEntry{}, closedWebDAVCapabilityError("create directory")
	}
	normalized, err := normalizeNonRootWebDAVPath(relativePath)
	if err != nil {
		return WorkspaceEntry{}, invalidWebDAVPathError("create directory", relativePath, err)
	}
	if err := capability.client.CreateDirectory(appWebDAVContext(a), normalized); err != nil {
		return WorkspaceEntry{}, err
	}
	return WorkspaceEntry{Name: path.Base(normalized), Path: normalized, Kind: "directory"}, nil
}

func (a *App) BeginWebDAVMutation(workspaceID string, sourcePath string, expectedKind string, expectedRevision string, operation string) (preparation WebDAVMutationPreparation, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return WebDAVMutationPreparation{}, err
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.closed || capability.client == nil {
		return WebDAVMutationPreparation{}, closedWebDAVCapabilityError("prepare mutation")
	}
	source, err := normalizeNonRootWebDAVPath(sourcePath)
	if err != nil {
		return WebDAVMutationPreparation{}, invalidWebDAVPathError("prepare mutation", sourcePath, err)
	}
	operation = strings.TrimSpace(operation)
	if operation != "rename" && operation != "delete" {
		return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "prepare mutation", Path: source, Err: errors.New("operation must be rename or delete")}
	}
	entry, _, err := webDAVWorkspaceEntryAndSiblings(capability.client, appWebDAVContext(a), source)
	if err != nil {
		return WebDAVMutationPreparation{}, err
	}
	if !validWorkspaceEntryKind(expectedKind) || entry.Kind != expectedKind {
		return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "prepare mutation", Path: source, Err: errors.New("resource type changed before it could be locked")}
	}
	if entry.Kind != "directory" {
		if entry.Revision == "" || strings.TrimSpace(expectedRevision) == "" {
			return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: "prepare mutation", Path: source, Err: errors.New("destructive file mutations require a strong ETag revision")}
		}
		if entry.Revision != expectedRevision {
			return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "prepare mutation", Path: source, Err: errors.New("resource changed after it was displayed")}
		}
	}
	for mutationID, mutation := range capability.mutations {
		if time.Since(mutation.createdAt) > 2*time.Minute {
			delete(capability.mutations, mutationID)
			capability.client.bestEffortUnlockTarget(mutation.lockPath, mutation.directory, mutation.lockToken)
		}
	}
	if len(capability.mutations) >= maxPreparedWebDAVMutations {
		return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorTooLarge, Operation: "prepare mutation", Path: source, Err: errors.New("too many WebDAV mutations are awaiting confirmation")}
	}
	directory := entry.Kind == "directory"
	lockPath := source
	lockDirectory := directory
	lockDepth := "0"
	if operation == "rename" {
		lockPath = parentWebDAVPath(source)
		lockDirectory = true
		lockDepth = "infinity"
	} else if directory {
		lockDepth = "infinity"
	}
	client := capability.client
	ctx := appWebDAVContext(a)
	lockToken, lockNullCreated, err := client.lockResourceTargetWithDepth(ctx, lockPath, lockDirectory, lockDepth)
	if err != nil {
		return WebDAVMutationPreparation{}, err
	}
	unlockOnError := true
	defer func() {
		if unlockOnError {
			client.bestEffortUnlockTarget(lockPath, lockDirectory, lockToken)
		}
	}()
	if client.testAfterMutationLockAcquired != nil {
		client.testAfterMutationLockAcquired()
	}
	if lockNullCreated {
		if lockPath == "" {
			return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "prepare mutation", Path: source, Err: errors.New("server reported the WebDAV root as a lock-null resource")}
		}
		metadata, exists, metadataErr := client.resourceMetadataAfterLock(lockPath, lockDirectory)
		if metadataErr != nil {
			return WebDAVMutationPreparation{}, metadataErr
		}
		if !exists {
			return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "prepare mutation", Path: source, Err: errors.New("resource no longer exists")}
		}
		if !metadata.Directory && lockDirectory {
			return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "prepare mutation", Path: source, Err: errors.New("lock target changed type")}
		}
	}
	if operation == "rename" {
		parentMetadata, exists, metadataErr := client.resourceMetadata(ctx, lockPath, true)
		if metadataErr != nil {
			return WebDAVMutationPreparation{}, metadataErr
		}
		if !exists || !parentMetadata.Directory {
			return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "prepare mutation", Path: source, Err: errors.New("source parent changed while being locked")}
		}
	}
	metadata, exists, err := client.resourceMetadata(ctx, source, directory)
	if err != nil {
		return WebDAVMutationPreparation{}, err
	}
	if !exists {
		return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "prepare mutation", Path: source}
	}
	if metadata.Directory != directory {
		return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "prepare mutation", Path: source, Err: errors.New("resource type changed while it was being locked")}
	}
	lockedRevision := webDAVMetadataRevision(source, entry.Kind, metadata)
	if entry.Kind != "directory" {
		if lockedRevision == "" {
			return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: "prepare mutation", Path: source, Err: errors.New("server omitted a strong ETag while the file was locked")}
		}
		if lockedRevision != expectedRevision {
			return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "prepare mutation", Path: source, Err: errors.New("resource changed while it was being locked")}
		}
	}
	mutationID, err := newOpaqueID()
	if err != nil {
		return WebDAVMutationPreparation{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "prepare mutation", Path: source, Err: errors.New("could not create a mutation capability")}
	}
	createdAt := time.Now()
	capability.mutations[mutationID] = preparedWebDAVMutation{
		id: mutationID, operation: operation, source: source, kind: entry.Kind,
		lockPath: lockPath, lockToken: lockToken, directory: lockDirectory,
		revision: lockedRevision, metadata: metadata, createdAt: createdAt,
	}
	unlockOnError = false
	entry.Revision = lockedRevision
	return WebDAVMutationPreparation{MutationID: mutationID, Entry: entry, ExpiresAt: createdAt.Add(30 * time.Second).UTC().Format(time.RFC3339)}, nil
}

func (a *App) CommitWebDAVRename(workspaceID string, mutationID string, destinationPath string) (bridgeEntry WorkspaceEntry, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, mutation, destination, err := a.takePreparedWebDAVRename(workspaceID, mutationID, destinationPath)
	if err != nil {
		return WorkspaceEntry{}, err
	}
	defer capability.mu.Unlock()
	client := capability.client
	defer client.bestEffortUnlockTarget(mutation.lockPath, mutation.directory, mutation.lockToken)
	ctx := appWebDAVContext(a)
	if err := client.refreshResourceLock(ctx, mutation.lockPath, mutation.directory, mutation.lockToken); err != nil {
		return WorkspaceEntry{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "move", Path: mutation.source, Err: errors.New("prepared WebDAV lock expired; retry the rename")}
	}
	if err := validatePreparedWebDAVSource(ctx, client, mutation); err != nil {
		return WorkspaceEntry{}, err
	}
	if err := client.moveResourceWithToken(ctx, mutation.source, destination, mutation.kind == "directory", mutation.lockToken); err != nil {
		return WorkspaceEntry{}, err
	}
	client.rebaseDocumentSessions(mutation.source, destination)
	for documentID, document := range capability.documents {
		if rebased, ok := rebaseWebDAVDescendantPath(document.path, mutation.source, destination); ok {
			document.path = rebased
			capability.documents[documentID] = document
		}
	}
	entry := WorkspaceEntry{Name: path.Base(destination), Path: destination, Kind: mutation.kind}
	if mutation.kind != "directory" {
		entry.Revision = webDAVMetadataRevision(destination, mutation.kind, mutation.metadata)
	}
	return entry, nil
}

func (a *App) CommitWebDAVDelete(workspaceID string, mutationID string, recursive bool) (err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, mutation, err := a.takePreparedWebDAVMutation(workspaceID, mutationID, "delete")
	if err != nil {
		return err
	}
	defer capability.mu.Unlock()
	client := capability.client
	defer client.bestEffortUnlockTarget(mutation.lockPath, mutation.directory, mutation.lockToken)
	if (mutation.kind == "directory") != recursive {
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "delete", Path: mutation.source, Err: errors.New("recursive confirmation does not match the locked resource type")}
	}
	ctx := appWebDAVContext(a)
	if err := client.refreshResourceLock(ctx, mutation.lockPath, mutation.directory, mutation.lockToken); err != nil {
		return &WebDAVError{Kind: WebDAVErrorConflict, Operation: "delete", Path: mutation.source, Err: errors.New("prepared WebDAV lock expired; retry the deletion")}
	}
	if err := validatePreparedWebDAVSource(ctx, client, mutation); err != nil {
		return err
	}
	if err := client.deleteResourceWithToken(ctx, mutation.source, mutation.kind == "directory", mutation.lockToken); err != nil {
		return err
	}
	client.closeDocumentSessionsAtPath(mutation.source)
	for documentID, document := range capability.documents {
		if webDAVPathAtOrBelow(document.path, mutation.source) {
			delete(capability.documents, documentID)
		}
	}
	return nil
}

func (a *App) CancelWebDAVMutation(workspaceID string, mutationID string) (err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return err
	}
	capability.mu.Lock()
	mutation, ok := capability.mutations[strings.TrimSpace(mutationID)]
	if ok {
		delete(capability.mutations, mutation.id)
	}
	client := capability.client
	closed := capability.closed || client == nil
	capability.mu.Unlock()
	if closed {
		return closedWebDAVCapabilityError("cancel mutation")
	}
	if !ok {
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "cancel mutation", Err: errors.New("prepared mutation capability is invalid")}
	}
	client.bestEffortUnlockTarget(mutation.lockPath, mutation.directory, mutation.lockToken)
	return nil
}

func (a *App) takePreparedWebDAVRename(workspaceID string, mutationID string, destinationPath string) (*webDAVCapability, preparedWebDAVMutation, string, error) {
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return nil, preparedWebDAVMutation{}, "", err
	}
	capability.mu.Lock()
	fail := func(err error) (*webDAVCapability, preparedWebDAVMutation, string, error) {
		capability.mu.Unlock()
		return nil, preparedWebDAVMutation{}, "", err
	}
	if capability.closed || capability.client == nil {
		return fail(closedWebDAVCapabilityError("move"))
	}
	mutation, ok := capability.mutations[strings.TrimSpace(mutationID)]
	if !ok || mutation.operation != "rename" {
		return fail(&WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "move", Err: errors.New("prepared rename capability is invalid")})
	}
	// Commit is a one-shot operation. The frontend intentionally discards the
	// mutation ID after any commit attempt, so destination validation failures
	// must consume the capability and release its DAV lock instead of leaving an
	// unreachable parent lock behind until timeout.
	delete(capability.mutations, mutation.id)
	client := capability.client
	consumeAndFail := func(err error) (*webDAVCapability, preparedWebDAVMutation, string, error) {
		capability.mu.Unlock()
		client.bestEffortUnlockTarget(mutation.lockPath, mutation.directory, mutation.lockToken)
		return nil, preparedWebDAVMutation{}, "", err
	}
	destination, err := normalizeNonRootWebDAVPath(destinationPath)
	if err != nil {
		return consumeAndFail(invalidWebDAVPathError("move", destinationPath, err))
	}
	if destination == mutation.source || parentWebDAVPath(destination) != parentWebDAVPath(mutation.source) {
		return consumeAndFail(&WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "move", Path: mutation.source, Err: errors.New("rename must choose a different name in the same directory")})
	}
	if err := validateWebDAVRenameKind(mutation.kind, mutation.source, destination); err != nil {
		return consumeAndFail(err)
	}
	return capability, mutation, destination, nil
}

func (a *App) takePreparedWebDAVMutation(workspaceID string, mutationID string, operation string) (*webDAVCapability, preparedWebDAVMutation, error) {
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return nil, preparedWebDAVMutation{}, err
	}
	capability.mu.Lock()
	if capability.closed || capability.client == nil {
		capability.mu.Unlock()
		return nil, preparedWebDAVMutation{}, closedWebDAVCapabilityError(operation)
	}
	mutation, ok := capability.mutations[strings.TrimSpace(mutationID)]
	if !ok || mutation.operation != operation {
		capability.mu.Unlock()
		return nil, preparedWebDAVMutation{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: operation, Err: errors.New("prepared mutation capability is invalid")}
	}
	delete(capability.mutations, mutation.id)
	return capability, mutation, nil
}

func validatePreparedWebDAVSource(ctx context.Context, client *WebDAVClient, mutation preparedWebDAVMutation) error {
	if mutation.operation == "rename" {
		parentMetadata, exists, err := client.resourceMetadata(ctx, mutation.lockPath, true)
		if err != nil {
			return err
		}
		if !exists || !parentMetadata.Directory {
			return &WebDAVError{Kind: WebDAVErrorConflict, Operation: "move", Path: mutation.source, Err: errors.New("locked parent collection changed")}
		}
	}
	metadata, exists, err := client.resourceMetadata(ctx, mutation.source, mutation.kind == "directory")
	if err != nil {
		return err
	}
	if !exists {
		return &WebDAVError{Kind: WebDAVErrorNotFound, Operation: mutation.operation, Path: mutation.source}
	}
	if metadata.Directory != (mutation.kind == "directory") {
		return &WebDAVError{Kind: WebDAVErrorConflict, Operation: mutation.operation, Path: mutation.source, Err: errors.New("locked resource changed type")}
	}
	if mutation.kind != "directory" {
		revision := webDAVMetadataRevision(mutation.source, mutation.kind, metadata)
		if revision == "" {
			return &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: mutation.operation, Path: mutation.source, Err: errors.New("server omitted a strong ETag while the file was locked")}
		}
		if revision != mutation.revision {
			return &WebDAVError{Kind: WebDAVErrorConflict, Operation: mutation.operation, Path: mutation.source, Err: errors.New("locked resource changed after confirmation")}
		}
	}
	return nil
}

func (client *WebDAVClient) refreshResourceLock(ctx context.Context, relativePath string, directory bool, lockToken string) error {
	headers := make(http.Header)
	headers.Set("If", "("+lockToken+")")
	headers.Set("Timeout", "Second-30")
	response, err := client.request(ctx, "LOCK", relativePath, directory, nil, headers)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "refresh lock", relativePath, http.StatusOK); err != nil {
		drainWebDAVResponse(response.Body)
		return err
	}
	drainWebDAVResponse(response.Body)
	return nil
}

func (client *WebDAVClient) moveResourceWithToken(ctx context.Context, source string, destination string, directory bool, lockToken string) error {
	destinationURL, err := client.resourceURL(destination, directory)
	if err != nil {
		return invalidWebDAVPathError("move", destination, err)
	}
	headers := make(http.Header)
	headers.Set("Destination", destinationURL.String())
	headers.Set("Overwrite", "F")
	headers.Set("If", "("+lockToken+")")
	response, err := client.request(ctx, "MOVE", source, directory, nil, headers)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "move", source, http.StatusCreated, http.StatusNoContent); err != nil {
		drainWebDAVResponse(response.Body)
		return err
	}
	drainWebDAVResponse(response.Body)
	return nil
}

func (client *WebDAVClient) deleteResourceWithToken(ctx context.Context, relativePath string, directory bool, lockToken string) error {
	headers := make(http.Header)
	headers.Set("If", "("+lockToken+")")
	response, err := client.request(ctx, http.MethodDelete, relativePath, directory, nil, headers)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "delete", relativePath, http.StatusOK, http.StatusAccepted, http.StatusNoContent); err != nil {
		drainWebDAVResponse(response.Body)
		return err
	}
	drainWebDAVResponse(response.Body)
	return nil
}

func (a *App) ReadWebDAVWorkspaceImage(workspaceID string, relativePath string) (data ImageAssetData, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()
	capability, err := a.webDAVCapabilityByID(workspaceID)
	if err != nil {
		return ImageAssetData{}, err
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	if capability.closed || capability.client == nil {
		return ImageAssetData{}, closedWebDAVCapabilityError("read image")
	}
	normalized, err := normalizeNonRootWebDAVPath(relativePath)
	if err != nil || !isImageFilename(normalized) {
		if err == nil {
			err = errors.New("only supported raster images can be read")
		}
		return ImageAssetData{}, invalidWebDAVPathError("read image", relativePath, err)
	}
	imageData, err := capability.client.ReadImage(appWebDAVContext(a), normalized)
	if err != nil {
		return ImageAssetData{}, err
	}
	return imageData.bridgeData(), nil
}

func webDAVBridgeDocument(workspaceID string, remote WebDAVDocument) Document {
	return Document{
		Path:             remote.Path,
		Name:             remote.Name,
		Content:          remote.Content,
		StorageKind:      "webdav",
		DisplayLocation:  remote.DisplayLocation,
		WorkspaceID:      workspaceID,
		WorkspacePath:    remote.Path,
		RemoteDocumentID: remote.RemoteDocumentID,
		ETag:             remote.ETag,
	}
}

func webDAVWorkspaceEntryAndSiblings(client *WebDAVClient, ctx context.Context, relativePath string) (WorkspaceEntry, []WorkspaceEntry, error) {
	directory, err := client.ListDirectory(ctx, parentWebDAVPath(relativePath))
	if err != nil {
		return WorkspaceEntry{}, nil, err
	}
	bridgeDirectory := webDAVWorkspaceDirectory(directory)
	for _, entry := range bridgeDirectory.Entries {
		if entry.Path == relativePath {
			return entry, bridgeDirectory.Entries, nil
		}
	}
	return WorkspaceEntry{}, bridgeDirectory.Entries, &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "stat", Path: relativePath}
}

func validateWebDAVRenameKind(kind string, source string, destination string) error {
	switch kind {
	case "directory":
		return nil
	case "markdown":
		if isMarkdownFilename(destination) {
			return nil
		}
	case "image":
		if sameImageExtensionFamily(source, destination) {
			return nil
		}
	default:
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "move", Path: source, Err: errors.New("unsupported workspace entry")}
	}
	return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "move", Path: source, Err: errors.New("destination extension does not match the resource type")}
}

func sameImageExtensionFamily(source string, destination string) bool {
	sourceExtension := strings.ToLower(path.Ext(source))
	destinationExtension := strings.ToLower(path.Ext(destination))
	if sourceExtension == ".jpg" || sourceExtension == ".jpeg" {
		return destinationExtension == ".jpg" || destinationExtension == ".jpeg"
	}
	return sourceExtension != "" && sourceExtension == destinationExtension && isImageFilename(destination)
}
