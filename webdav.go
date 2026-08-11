package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	defaultWebDAVTimeout     = 20 * time.Second
	maxWebDAVMultiStatusSize = 8 << 20
	maxWebDAVResponses       = 4096
	maxWebDAVDocumentSize    = 16 << 20
	maxWebDAVPathLength      = 4096
	maxWebDAVLockResponse    = 256 << 10
)

// WebDAVConfig contains the connection settings supplied at runtime. The
// client deliberately keeps credentials out of URLs and error messages.
type WebDAVConfig struct {
	Endpoint string        `json:"endpoint"`
	Username string        `json:"username"`
	Password string        `json:"password"`
	Timeout  time.Duration `json:"-"`
}

func (config WebDAVConfig) String() string {
	return fmt.Sprintf("{Endpoint:%q Username:<redacted> Password:<redacted> Timeout:%s}", safeWebDAVEndpoint(config.Endpoint), config.Timeout)
}

func (config WebDAVConfig) GoString() string {
	return config.String()
}

// MarshalJSON deliberately excludes authentication material. WebDAVConfig is
// accepted as an input payload, but must never become a credential-bearing
// response, diagnostic snapshot, or persisted settings value.
func (config WebDAVConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Endpoint string `json:"endpoint"`
	}{Endpoint: safeWebDAVEndpoint(config.Endpoint)})
}

type WebDAVErrorKind string

const (
	WebDAVErrorInvalidInput    WebDAVErrorKind = "invalid_input"
	WebDAVErrorAuthentication  WebDAVErrorKind = "authentication"
	WebDAVErrorPermission      WebDAVErrorKind = "permission"
	WebDAVErrorNotFound        WebDAVErrorKind = "not_found"
	WebDAVErrorConflict        WebDAVErrorKind = "conflict"
	WebDAVErrorLocked          WebDAVErrorKind = "locked"
	WebDAVErrorUnsupported     WebDAVErrorKind = "unsupported"
	WebDAVErrorTooLarge        WebDAVErrorKind = "too_large"
	WebDAVErrorRateLimited     WebDAVErrorKind = "rate_limited"
	WebDAVErrorTimeout         WebDAVErrorKind = "timeout"
	WebDAVErrorCanceled        WebDAVErrorKind = "canceled"
	WebDAVErrorNetwork         WebDAVErrorKind = "network"
	WebDAVErrorServer          WebDAVErrorKind = "server"
	WebDAVErrorProtocol        WebDAVErrorKind = "protocol"
	WebDAVErrorCredentialStore WebDAVErrorKind = "credential_store"
	WebDAVErrorLocalStorage    WebDAVErrorKind = "local_storage"
)

type WebDAVError struct {
	Kind       WebDAVErrorKind
	Operation  string
	Path       string
	StatusCode int
	Err        error
}

func (err *WebDAVError) Error() string {
	if err == nil {
		return "<nil>"
	}
	message := "WebDAV " + err.Operation
	if err.Path != "" {
		message += " " + strconv.Quote(err.Path)
	}
	message += " failed"
	if err.StatusCode != 0 {
		message += fmt.Sprintf(": HTTP %d", err.StatusCode)
		if status := http.StatusText(err.StatusCode); status != "" {
			message += " " + status
		}
	} else if err.Err != nil {
		message += ": " + err.Err.Error()
	}
	return message
}

func (err *WebDAVError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func IsWebDAVErrorKind(err error, kind WebDAVErrorKind) bool {
	var webDAVError *WebDAVError
	return errors.As(err, &webDAVError) && webDAVError.Kind == kind
}

type WebDAVClient struct {
	mu                            sync.RWMutex
	baseURL                       *url.URL
	advertisedRootPath            string
	advertisedRootKnown           bool
	username                      string
	password                      string
	client                        *http.Client
	sessions                      map[string]webDAVSession
	testAfterMutationLockAcquired func()
}

type WebDAVEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Directory   bool   `json:"directory"`
	Size        int64  `json:"size"`
	Modified    string `json:"modified,omitempty"`
	ETag        string `json:"etag,omitempty"`
	ContentType string `json:"contentType,omitempty"`
}

type WebDAVDirectory struct {
	Path    string        `json:"path"`
	Entries []WebDAVEntry `json:"entries"`
}

type WebDAVDocument struct {
	Path             string `json:"path"`
	Name             string `json:"name"`
	Content          string `json:"content"`
	ETag             string `json:"etag,omitempty"`
	Modified         string `json:"modified,omitempty"`
	RemoteDocumentID string `json:"remoteDocumentId"`
	DisplayLocation  string `json:"displayLocation"`
}

type WebDAVWriteOptions struct {
	IfMatch    string
	CreateOnly bool
}

type WebDAVWriteResult struct {
	Path            string `json:"path"`
	ETag            string `json:"etag,omitempty"`
	Created         bool   `json:"created"`
	ConcurrencyMode string `json:"concurrencyMode,omitempty"`
}

type webDAVResourceMetadata struct {
	Path        string
	ETag        string
	Modified    string
	ContentType string
	Size        int64
	Directory   bool
}

type webDAVSession struct {
	Path string
	ETag string
}

func NewWebDAVClient(config WebDAVConfig) (*WebDAVClient, error) {
	baseURL, err := normalizeWebDAVBaseURL(config.Endpoint)
	if err != nil {
		return nil, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "configure", Err: err}
	}
	if strings.ContainsAny(config.Username, "\r\n") || strings.ContainsAny(config.Password, "\r\n") {
		return nil, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "configure", Err: errors.New("credentials contain invalid control characters")}
	}
	if config.Username == "" && config.Password != "" {
		return nil, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "configure", Err: errors.New("a password requires a username")}
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultWebDAVTimeout
	}
	client := &WebDAVClient{
		baseURL:  baseURL,
		username: config.Username,
		password: config.Password,
		sessions: make(map[string]webDAVSession),
	}
	client.client = &http.Client{
		Timeout:   timeout,
		Transport: http.DefaultTransport.(*http.Transport).Clone(),
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many WebDAV redirects")
			}
			if !safeWebDAVRedirectURL(baseURL, request.URL) {
				return errors.New("WebDAV redirect left the configured server path")
			}
			if len(via) != 0 && request.Method != via[0].Method {
				return errors.New("WebDAV redirect changed the request method")
			}
			return nil
		},
	}
	return client, nil
}

// CheckConnection verifies authentication and WebDAV support without listing
// child resources.
func (client *WebDAVClient) CheckConnection(ctx context.Context) error {
	return client.ensureAdvertisedRoot(ctx)
}

func (client *WebDAVClient) ListDirectory(ctx context.Context, relativePath string) (WebDAVDirectory, error) {
	normalized, err := normalizeWebDAVPath(relativePath)
	if err != nil {
		return WebDAVDirectory{}, invalidWebDAVPathError("list", relativePath, err)
	}
	if err := client.ensureAdvertisedRoot(ctx); err != nil {
		return WebDAVDirectory{}, err
	}
	multiStatus, err := client.propfind(ctx, normalized, "1")
	if err != nil {
		return WebDAVDirectory{}, err
	}
	entriesByPath := make(map[string]WebDAVEntry)
	for _, response := range multiStatus.Responses {
		entryPath, hrefErr := client.relativePathFromHref(response.Href, normalized)
		if hrefErr != nil || entryPath == normalized || parentWebDAVPath(entryPath) != normalized {
			continue
		}
		properties, ok := response.successfulProperties()
		if !ok || properties.ResourceType == nil {
			continue
		}
		entry := WebDAVEntry{
			Name:        path.Base(entryPath),
			Path:        entryPath,
			Directory:   properties.ResourceType.Collection != nil,
			Modified:    strings.TrimSpace(properties.LastModified),
			ETag:        strings.TrimSpace(properties.ETag),
			ContentType: strings.TrimSpace(properties.ContentType),
		}
		if size, parseErr := strconv.ParseInt(strings.TrimSpace(properties.ContentLength), 10, 64); parseErr == nil && size >= 0 {
			entry.Size = size
		}
		if !entry.Directory && !isMarkdownFilename(entry.Name) && !isImageFilename(entry.Name) {
			continue
		}
		entriesByPath[entryPath] = entry
	}

	entries := make([]WebDAVEntry, 0, len(entriesByPath))
	for _, entry := range entriesByPath {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Directory != entries[right].Directory {
			return entries[left].Directory
		}
		leftName := strings.ToLower(entries[left].Name)
		rightName := strings.ToLower(entries[right].Name)
		if leftName == rightName {
			return entries[left].Name < entries[right].Name
		}
		return leftName < rightName
	})
	return WebDAVDirectory{Path: normalized, Entries: entries}, nil
}

func (client *WebDAVClient) ReadMarkdown(ctx context.Context, relativePath string) (WebDAVDocument, error) {
	normalized, err := normalizeWebDAVMarkdownPath(relativePath)
	if err != nil {
		return WebDAVDocument{}, invalidWebDAVPathError("read", relativePath, err)
	}
	if err := client.ensureAdvertisedRoot(ctx); err != nil {
		return WebDAVDocument{}, err
	}
	response, err := client.request(ctx, http.MethodGet, normalized, false, nil, nil)
	if err != nil {
		return WebDAVDocument{}, err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "read", normalized, http.StatusOK); err != nil {
		drainWebDAVResponse(response.Body)
		return WebDAVDocument{}, err
	}
	if response.ContentLength > maxWebDAVDocumentSize {
		return WebDAVDocument{}, &WebDAVError{Kind: WebDAVErrorTooLarge, Operation: "read", Path: normalized, Err: errors.New("document exceeds the size limit")}
	}
	data, err := readLimitedWebDAVBody(response.Body, maxWebDAVDocumentSize)
	if err != nil {
		return WebDAVDocument{}, bodyWebDAVError("read", normalized, err)
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if !utf8.Valid(data) {
		return WebDAVDocument{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "read", Path: normalized, Err: errors.New("document is not valid UTF-8")}
	}
	documentID, err := newOpaqueID()
	if err != nil {
		return WebDAVDocument{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "read", Path: normalized, Err: errors.New("could not create a document session")}
	}
	etag := strings.TrimSpace(response.Header.Get("ETag"))
	client.mu.Lock()
	client.sessions[documentID] = webDAVSession{Path: normalized, ETag: etag}
	client.mu.Unlock()
	return WebDAVDocument{
		Path:             normalized,
		Name:             path.Base(normalized),
		Content:          string(data),
		ETag:             etag,
		Modified:         strings.TrimSpace(response.Header.Get("Last-Modified")),
		RemoteDocumentID: documentID,
		DisplayLocation:  client.displayLocation(normalized),
	}, nil
}

// registerCreatedMarkdownSession turns a successfully verified create-only
// PUT into an editable document capability without issuing another GET. The
// PUT path already compared the complete representation and returned its
// canonical ETag, so an additional network read would only introduce a
// commit-then-read-failure window.
func (client *WebDAVClient) registerCreatedMarkdownSession(relativePath string, etag string) (WebDAVDocument, error) {
	normalized, err := normalizeWebDAVMarkdownPath(relativePath)
	if err != nil {
		return WebDAVDocument{}, invalidWebDAVPathError("create document", relativePath, err)
	}
	etag = strings.TrimSpace(etag)
	if etag == "" {
		return WebDAVDocument{}, &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: "create document", Path: normalized, Err: errors.New("verified create did not provide a canonical ETag")}
	}
	documentID, err := newOpaqueID()
	if err != nil {
		return WebDAVDocument{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "create document", Path: normalized, Err: errors.New("could not create a document session")}
	}
	client.mu.Lock()
	if client.sessions == nil {
		client.mu.Unlock()
		return WebDAVDocument{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "create document", Path: normalized, Err: errors.New("WebDAV client is closed")}
	}
	client.sessions[documentID] = webDAVSession{Path: normalized, ETag: etag}
	client.mu.Unlock()
	return WebDAVDocument{
		Path:             normalized,
		Name:             path.Base(normalized),
		Content:          "",
		ETag:             etag,
		RemoteDocumentID: documentID,
		DisplayLocation:  client.displayLocation(normalized),
	}, nil
}

// SaveMarkdownSession saves a previously opened document. It prefers an
// exclusive DAV write lock, compares the canonical Depth-0 ETag while holding
// that lock, writes with the lock token, and then refreshes the canonical ETag.
// Servers without LOCK are rejected so a normal save can never silently use a
// racy read-then-write fallback.
func (client *WebDAVClient) SaveMarkdownSession(ctx context.Context, documentID string, content string, etag string) (WebDAVWriteResult, error) {
	documentID = strings.TrimSpace(documentID)
	if client == nil {
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "write", Err: errors.New("WebDAV client is not configured")}
	}
	client.mu.RLock()
	session, ok := client.sessions[documentID]
	client.mu.RUnlock()
	if !ok || documentID == "" {
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "write", Err: errors.New("remote document session is closed or invalid")}
	}
	if etag != session.ETag {
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "write", Path: session.Path, Err: errors.New("document ETag is stale")}
	}
	if session.ETag == "" {
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: "write", Path: session.Path, Err: errors.New("server did not provide an ETag for concurrency-safe saving")}
	}
	if err := validateWebDAVMarkdownContent(session.Path, content); err != nil {
		return WebDAVWriteResult{}, err
	}
	lockToken, lockNullCreated, locked, err := client.tryExclusiveWriteLock(ctx, session.Path)
	if err != nil {
		return WebDAVWriteResult{}, err
	}
	if locked {
		defer client.bestEffortUnlock(session.Path, lockToken)
	} else {
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: "write", Path: session.Path, Err: errors.New("server does not support exclusive DAV write locks")}
	}
	if lockNullCreated {
		client.bestEffortDeleteLocked(session.Path, lockToken)
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "write", Path: session.Path, Err: errors.New("remote document no longer exists")}
	}
	canonicalETag, exists, err := client.resourceState(ctx, session.Path)
	if err != nil {
		return WebDAVWriteResult{}, err
	}
	if !exists {
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "write", Path: session.Path}
	}
	if canonicalETag == "" {
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: "write", Path: session.Path, Err: errors.New("server did not advertise a canonical ETag")}
	}
	if canonicalETag != session.ETag {
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "write", Path: session.Path, Err: errors.New("remote document changed")}
	}
	headers := make(http.Header)
	headers.Set("If", "("+lockToken+")")
	result, _, err := client.putMarkdownValidated(ctx, session.Path, content, headers)
	if err != nil {
		return WebDAVWriteResult{}, err
	}
	result.ConcurrencyMode = "dav-lock"
	client.mu.Lock()
	current, stillOpen := client.sessions[documentID]
	if stillOpen && current.Path == session.Path && current.ETag == session.ETag {
		current.ETag = result.ETag
		client.sessions[documentID] = current
	}
	client.mu.Unlock()
	return result, nil
}

// OverwriteMarkdownSession is the explicit conflict-resolution path. It still
// uses an exclusive DAV lock when available, but deliberately skips the ETag
// comparison requested by normal saves.
func (client *WebDAVClient) OverwriteMarkdownSession(ctx context.Context, documentID string, content string) (WebDAVWriteResult, error) {
	documentID = strings.TrimSpace(documentID)
	if client == nil {
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "write", Err: errors.New("WebDAV client is not configured")}
	}
	client.mu.RLock()
	session, ok := client.sessions[documentID]
	client.mu.RUnlock()
	if !ok || documentID == "" {
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "write", Err: errors.New("remote document session is closed or invalid")}
	}
	if err := validateWebDAVMarkdownContent(session.Path, content); err != nil {
		return WebDAVWriteResult{}, err
	}
	lockToken, lockNullCreated, locked, err := client.tryExclusiveWriteLock(ctx, session.Path)
	if err != nil {
		return WebDAVWriteResult{}, err
	}
	if locked {
		defer client.bestEffortUnlock(session.Path, lockToken)
	}
	if lockNullCreated {
		client.bestEffortDeleteLocked(session.Path, lockToken)
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "write", Path: session.Path, Err: errors.New("remote document no longer exists")}
	}
	headers := make(http.Header)
	mode := "explicit-overwrite"
	if locked {
		headers.Set("If", "("+lockToken+")")
		mode = "dav-lock-overwrite"
	}
	result, _, err := client.putMarkdownValidated(ctx, session.Path, content, headers)
	if err != nil {
		return WebDAVWriteResult{}, err
	}
	result.ConcurrencyMode = mode
	client.mu.Lock()
	current, stillOpen := client.sessions[documentID]
	if stillOpen && current.Path == session.Path {
		current.ETag = result.ETag
		client.sessions[documentID] = current
	}
	client.mu.Unlock()
	return result, nil
}

func (client *WebDAVClient) CloseDocument(documentID string) {
	if client == nil {
		return
	}
	client.mu.Lock()
	delete(client.sessions, strings.TrimSpace(documentID))
	client.mu.Unlock()
}

// rebaseDocumentSessions updates opaque document capabilities after a
// successful WebDAV MOVE. Validators are deliberately preserved: if the
// server changed an ETag while moving the resource, the next ordinary save
// must surface a conflict rather than silently trusting a validator which was
// not compared with the editor's current content.
func (client *WebDAVClient) rebaseDocumentSessions(sourcePath string, destinationPath string) {
	if client == nil {
		return
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	for documentID, session := range client.sessions {
		if rebased, ok := rebaseWebDAVDescendantPath(session.Path, sourcePath, destinationPath); ok {
			session.Path = rebased
			client.sessions[documentID] = session
		}
	}
}

func (client *WebDAVClient) closeDocumentSessionsAtPath(targetPath string) {
	if client == nil {
		return
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	for documentID, session := range client.sessions {
		if webDAVPathAtOrBelow(session.Path, targetPath) {
			delete(client.sessions, documentID)
		}
	}
}

func (client *WebDAVClient) Close() {
	if client == nil {
		return
	}
	client.mu.Lock()
	client.username = ""
	client.password = ""
	client.sessions = make(map[string]webDAVSession)
	httpClient := client.client
	client.mu.Unlock()
	if httpClient != nil {
		httpClient.CloseIdleConnections()
	}
}

func (client *WebDAVClient) PutMarkdown(ctx context.Context, relativePath string, content string, options WebDAVWriteOptions) (WebDAVWriteResult, error) {
	normalized, err := normalizeWebDAVMarkdownPath(relativePath)
	if err != nil {
		return WebDAVWriteResult{}, invalidWebDAVPathError("write", relativePath, err)
	}
	if err := validateWebDAVMarkdownContent(normalized, content); err != nil {
		return WebDAVWriteResult{}, err
	}
	if err := validateWebDAVWriteOptions(options); err != nil {
		return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "write", Path: normalized, Err: err}
	}
	if err := client.ensureAdvertisedRoot(ctx); err != nil {
		return WebDAVWriteResult{}, err
	}
	headers := make(http.Header)
	if options.CreateOnly {
		lockToken, lockNullCreated, locked, lockErr := client.tryExclusiveWriteLock(ctx, normalized)
		if lockErr != nil {
			return WebDAVWriteResult{}, lockErr
		}
		if !locked {
			return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: "write", Path: normalized, Err: errors.New("server does not support safe lock-null creation")}
		}
		if !lockNullCreated {
			defer client.bestEffortUnlock(normalized, lockToken)
			return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "write", Path: normalized, Err: errors.New("remote document already exists")}
		}
		putAccepted := false
		defer func() {
			if !putAccepted {
				client.bestEffortDeleteLocked(normalized, lockToken)
			}
			client.bestEffortUnlock(normalized, lockToken)
		}()
		headers.Set("If", "("+lockToken+")")
		result, accepted, writeErr := client.putMarkdownValidated(ctx, normalized, content, headers)
		putAccepted = accepted
		if writeErr != nil {
			return WebDAVWriteResult{}, writeErr
		}
		result.Created = true
		result.ConcurrencyMode = "dav-lock-create"
		return result, nil
	} else if options.IfMatch != "" {
		lockToken, lockNullCreated, locked, lockErr := client.tryExclusiveWriteLock(ctx, normalized)
		if lockErr != nil {
			return WebDAVWriteResult{}, lockErr
		}
		if !locked {
			return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: "write", Path: normalized, Err: errors.New("server does not support exclusive DAV write locks")}
		}
		defer client.bestEffortUnlock(normalized, lockToken)
		if lockNullCreated {
			client.bestEffortDeleteLocked(normalized, lockToken)
			return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "write", Path: normalized, Err: errors.New("remote document no longer exists")}
		}
		canonicalETag, exists, stateErr := client.resourceState(ctx, normalized)
		if stateErr != nil {
			return WebDAVWriteResult{}, stateErr
		}
		if !exists {
			return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "write", Path: normalized}
		}
		if canonicalETag == "" || canonicalETag != options.IfMatch {
			return WebDAVWriteResult{}, &WebDAVError{Kind: WebDAVErrorConflict, Operation: "write", Path: normalized, Err: errors.New("remote document changed")}
		}
		headers.Set("If", "("+lockToken+")")
	}
	result, _, err := client.putMarkdownValidated(ctx, normalized, content, headers)
	if err != nil {
		return WebDAVWriteResult{}, err
	}
	if options.IfMatch != "" {
		result.ConcurrencyMode = "dav-lock"
	} else {
		result.ConcurrencyMode = "unconditional"
	}
	return result, nil
}

func (client *WebDAVClient) putMarkdownValidated(ctx context.Context, normalized string, content string, headers http.Header) (WebDAVWriteResult, bool, error) {
	headers = headers.Clone()
	headers.Set("Content-Type", "text/markdown; charset=utf-8")
	response, err := client.request(ctx, http.MethodPut, normalized, false, strings.NewReader(content), headers)
	if err != nil {
		return client.reconcileUncertainWrite(ctx, normalized, content, err)
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "write", normalized, http.StatusOK, http.StatusCreated, http.StatusNoContent); err != nil {
		drainWebDAVResponse(response.Body)
		_ = response.Body.Close()
		return client.reconcileUncertainWrite(ctx, normalized, content, err)
	}
	created := response.StatusCode == http.StatusCreated
	drainWebDAVResponse(response.Body)
	etag, err := client.verifyWrittenMarkdown(ctx, normalized, content)
	// Always request the canonical Depth-0 state after a successful PUT. The
	// value is diagnostic only: the session validator must come from the GET
	// representation whose bytes were compared with the submitted content.
	_, _, _ = client.resourceState(ctx, normalized)
	if err != nil {
		return WebDAVWriteResult{}, true, err
	}
	return WebDAVWriteResult{
		Path:    normalized,
		ETag:    etag,
		Created: created,
	}, true, nil
}

// reconcileUncertainWrite handles the case where PUT may have reached the
// server even though the client did not receive a success response. A matching
// GET representation proves success. Any existing but different or
// unverifiable representation must be preserved; only a confirmed 404 permits
// the lock-null cleanup path to run.
func (client *WebDAVClient) reconcileUncertainWrite(ctx context.Context, normalized string, content string, originalErr error) (WebDAVWriteResult, bool, error) {
	etag, verifyErr := client.verifyWrittenMarkdown(ctx, normalized, content)
	if verifyErr == nil {
		_, _, _ = client.resourceState(ctx, normalized)
		return WebDAVWriteResult{Path: normalized, ETag: etag}, true, nil
	}
	if IsWebDAVErrorKind(verifyErr, WebDAVErrorNotFound) {
		return WebDAVWriteResult{}, false, originalErr
	}
	if IsWebDAVErrorKind(verifyErr, WebDAVErrorConflict) || IsWebDAVErrorKind(verifyErr, WebDAVErrorUnsupported) {
		return WebDAVWriteResult{}, true, verifyErr
	}
	return WebDAVWriteResult{}, true, originalErr
}

func (client *WebDAVClient) verifyWrittenMarkdown(ctx context.Context, normalized string, expectedContent string) (string, error) {
	response, err := client.request(ctx, http.MethodGet, normalized, false, nil, nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "verify write", normalized, http.StatusOK); err != nil {
		drainWebDAVResponse(response.Body)
		return "", err
	}
	if response.ContentLength > maxWebDAVDocumentSize {
		return "", &WebDAVError{Kind: WebDAVErrorTooLarge, Operation: "verify write", Path: normalized, Err: errors.New("written document exceeds the size limit")}
	}
	data, err := readLimitedWebDAVBody(response.Body, maxWebDAVDocumentSize)
	if err != nil {
		return "", bodyWebDAVError("verify write", normalized, err)
	}
	if string(data) != expectedContent {
		return "", &WebDAVError{Kind: WebDAVErrorConflict, Operation: "verify write", Path: normalized, Err: errors.New("remote content changed before save verification")}
	}
	etag := strings.TrimSpace(response.Header.Get("ETag"))
	if etag == "" {
		return "", &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: "verify write", Path: normalized, Err: errors.New("GET verification did not provide an ETag")}
	}
	return etag, nil
}

func (client *WebDAVClient) CreateDirectory(ctx context.Context, relativePath string) error {
	normalized, err := normalizeNonRootWebDAVPath(relativePath)
	if err != nil {
		return invalidWebDAVPathError("create directory", relativePath, err)
	}
	response, err := client.request(ctx, "MKCOL", normalized, true, nil, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "create directory", normalized, http.StatusOK, http.StatusCreated, http.StatusNoContent); err != nil {
		drainWebDAVResponse(response.Body)
		return err
	}
	drainWebDAVResponse(response.Body)
	return nil
}

func (client *WebDAVClient) Delete(ctx context.Context, relativePath string) error {
	return client.DeleteResource(ctx, relativePath, false)
}

// DeleteResource keeps collection deletion explicit. WebDAV DELETE on a
// collection is recursive by protocol, so callers must supply the resource
// type and the app bridge only invokes this with a separately confirmed UI
// recursion flag.
func (client *WebDAVClient) DeleteResource(ctx context.Context, relativePath string, directory bool) error {
	normalized, err := normalizeNonRootWebDAVPath(relativePath)
	if err != nil {
		return invalidWebDAVPathError("delete", relativePath, err)
	}
	response, err := client.request(ctx, http.MethodDelete, normalized, directory, nil, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "delete", normalized, http.StatusOK, http.StatusAccepted, http.StatusNoContent); err != nil {
		drainWebDAVResponse(response.Body)
		return err
	}
	drainWebDAVResponse(response.Body)
	return nil
}

func (client *WebDAVClient) DeleteResourceLocked(ctx context.Context, relativePath string, expectedDirectory bool, expectedRevision string) error {
	normalized, err := normalizeNonRootWebDAVPath(relativePath)
	if err != nil {
		return invalidWebDAVPathError("delete", relativePath, err)
	}
	lockDepth := "0"
	if expectedDirectory {
		// Recursive DELETE must freeze the whole confirmed collection tree.
		lockDepth = "infinity"
	}
	lockToken, lockNullCreated, err := client.lockResourceTargetWithDepth(ctx, normalized, expectedDirectory, lockDepth)
	if err != nil {
		return err
	}
	if client.testAfterMutationLockAcquired != nil {
		client.testAfterMutationLockAcquired()
	}
	var metadata webDAVResourceMetadata
	metadataKnown := false
	if lockNullCreated {
		confirmed, exists, confirmErr := client.resourceMetadataAfterLock(normalized, expectedDirectory)
		if confirmErr != nil {
			client.bestEffortUnlockTarget(normalized, expectedDirectory, lockToken)
			return confirmErr
		}
		if exists {
			// Some servers/proxies incorrectly return 201 for an existing lock
			// target. Never interpret that status alone as authority to delete it.
			metadata = confirmed
			metadataKnown = true
		} else {
			client.bestEffortUnlockTarget(normalized, expectedDirectory, lockToken)
			return &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "delete", Path: normalized, Err: errors.New("resource no longer exists")}
		}
	}
	defer client.bestEffortUnlockTarget(normalized, expectedDirectory, lockToken)
	if !metadataKnown {
		var exists bool
		metadata, exists, err = client.resourceMetadata(ctx, normalized, expectedDirectory)
		if err != nil {
			return err
		}
		if !exists {
			return &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "delete", Path: normalized}
		}
	}
	if metadata.Directory != expectedDirectory {
		return &WebDAVError{Kind: WebDAVErrorConflict, Operation: "delete", Path: normalized, Err: errors.New("resource type changed while awaiting confirmation")}
	}
	if err := validateLockedWebDAVRevision("delete", normalized, metadata, expectedDirectory, expectedRevision); err != nil {
		return err
	}
	headers := make(http.Header)
	headers.Set("If", "("+lockToken+")")
	response, err := client.request(ctx, http.MethodDelete, normalized, expectedDirectory, nil, headers)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "delete", normalized, http.StatusOK, http.StatusAccepted, http.StatusNoContent); err != nil {
		drainWebDAVResponse(response.Body)
		return err
	}
	drainWebDAVResponse(response.Body)
	return nil
}

func (client *WebDAVClient) Move(ctx context.Context, sourcePath string, destinationPath string, overwrite bool) error {
	return client.MoveResource(ctx, sourcePath, destinationPath, false, overwrite)
}

// MoveResource emits collection URI trailing slashes when moving a directory
// and always lets the caller control the WebDAV Overwrite header.
func (client *WebDAVClient) MoveResource(ctx context.Context, sourcePath string, destinationPath string, directory bool, overwrite bool) error {
	source, err := normalizeNonRootWebDAVPath(sourcePath)
	if err != nil {
		return invalidWebDAVPathError("move", sourcePath, err)
	}
	destination, err := normalizeNonRootWebDAVPath(destinationPath)
	if err != nil {
		return invalidWebDAVPathError("move", destinationPath, err)
	}
	if source == destination {
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "move", Path: source, Err: errors.New("source and destination are identical")}
	}
	if directory && strings.HasPrefix(destination, source+"/") {
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "move", Path: source, Err: errors.New("a collection cannot be moved into itself")}
	}
	destinationURL, err := client.resourceURL(destination, directory)
	if err != nil {
		return invalidWebDAVPathError("move", destinationPath, err)
	}
	headers := make(http.Header)
	headers.Set("Destination", destinationURL.String())
	if overwrite {
		headers.Set("Overwrite", "T")
	} else {
		headers.Set("Overwrite", "F")
	}
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

func (client *WebDAVClient) MoveResourceLocked(ctx context.Context, sourcePath string, destinationPath string, expectedDirectory bool, expectedRevision string) error {
	source, err := normalizeNonRootWebDAVPath(sourcePath)
	if err != nil {
		return invalidWebDAVPathError("move", sourcePath, err)
	}
	destination, err := normalizeNonRootWebDAVPath(destinationPath)
	if err != nil {
		return invalidWebDAVPathError("move", destinationPath, err)
	}
	if source == destination {
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "move", Path: source, Err: errors.New("source and destination are identical")}
	}
	if parentWebDAVPath(source) != parentWebDAVPath(destination) {
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "move", Path: source, Err: errors.New("locked move is limited to a same-parent rename")}
	}
	if expectedDirectory && strings.HasPrefix(destination, source+"/") {
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "move", Path: source, Err: errors.New("a collection cannot be moved into itself")}
	}
	// x/net/webdav and other strict DAV implementations confirm both MOVE
	// operands against one If-list. Because rename is limited to siblings, an
	// exclusive infinite-depth lock on their common parent safely covers both
	// source and destination while avoiding a lock-null destination resource.
	lockPath := parentWebDAVPath(source)
	lockToken, lockNullCreated, err := client.lockResourceTargetWithDepth(ctx, lockPath, true, "infinity")
	if err != nil {
		return err
	}
	if client.testAfterMutationLockAcquired != nil {
		client.testAfterMutationLockAcquired()
	}
	var parentMetadata webDAVResourceMetadata
	parentMetadataKnown := false
	if lockNullCreated {
		if lockPath == "" {
			client.bestEffortUnlockTarget(lockPath, true, lockToken)
			return &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "move", Path: source, Err: errors.New("server reported the WebDAV root as a lock-null resource")}
		}
		confirmed, exists, confirmErr := client.resourceMetadataAfterLock(lockPath, true)
		if confirmErr != nil {
			client.bestEffortUnlockTarget(lockPath, true, lockToken)
			return confirmErr
		}
		if exists {
			parentMetadata = confirmed
			parentMetadataKnown = true
		} else {
			client.bestEffortUnlockTarget(lockPath, true, lockToken)
			return &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "move", Path: source, Err: errors.New("source parent no longer exists")}
		}
	}
	// The locked collection is never moved, so its unlock path is invariant
	// across success, rejected responses, and transport-unknown outcomes.
	defer client.bestEffortUnlockTarget(lockPath, true, lockToken)
	if !parentMetadataKnown {
		var parentExists bool
		parentMetadata, parentExists, err = client.resourceMetadata(ctx, lockPath, true)
		if err != nil {
			return err
		}
		if !parentExists {
			return &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "move", Path: source, Err: errors.New("source parent no longer exists")}
		}
	}
	if !parentMetadata.Directory {
		return &WebDAVError{Kind: WebDAVErrorConflict, Operation: "move", Path: source, Err: errors.New("source parent changed type while locked")}
	}
	metadata, exists, err := client.resourceMetadata(ctx, source, expectedDirectory)
	if err != nil {
		return err
	}
	if !exists {
		return &WebDAVError{Kind: WebDAVErrorNotFound, Operation: "move", Path: source}
	}
	if metadata.Directory != expectedDirectory {
		return &WebDAVError{Kind: WebDAVErrorConflict, Operation: "move", Path: source, Err: errors.New("resource type changed while awaiting confirmation")}
	}
	if err := validateLockedWebDAVRevision("move", source, metadata, expectedDirectory, expectedRevision); err != nil {
		return err
	}
	destinationURL, err := client.resourceURL(destination, expectedDirectory)
	if err != nil {
		return invalidWebDAVPathError("move", destinationPath, err)
	}
	headers := make(http.Header)
	headers.Set("Destination", destinationURL.String())
	headers.Set("Overwrite", "F")
	headers.Set("If", "("+lockToken+")")
	response, err := client.request(ctx, "MOVE", source, expectedDirectory, nil, headers)
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

func validateLockedWebDAVRevision(operation string, relativePath string, metadata webDAVResourceMetadata, expectedDirectory bool, expectedRevision string) error {
	if expectedDirectory {
		return &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: operation, Path: relativePath, Err: errors.New("collection mutations require a prepared lock capability")}
	}
	kind := ""
	if isMarkdownFilename(relativePath) {
		kind = "markdown"
	} else if isImageFilename(relativePath) {
		kind = "image"
	}
	if kind == "" {
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: operation, Path: relativePath, Err: errors.New("unsupported file type")}
	}
	actualRevision := webDAVMetadataRevision(relativePath, kind, metadata)
	if strings.TrimSpace(expectedRevision) == "" || actualRevision == "" {
		return &WebDAVError{Kind: WebDAVErrorUnsupported, Operation: operation, Path: relativePath, Err: errors.New("destructive file mutations require a strong ETag revision")}
	}
	if actualRevision != expectedRevision {
		return &WebDAVError{Kind: WebDAVErrorConflict, Operation: operation, Path: relativePath, Err: errors.New("remote resource changed")}
	}
	return nil
}

func (client *WebDAVClient) propfind(ctx context.Context, relativePath string, depth string) (webDAVMultiStatus, error) {
	directoryTarget := relativePath == "" || depth == "1"
	return client.propfindTarget(ctx, relativePath, depth, directoryTarget)
}

func (client *WebDAVClient) propfindTarget(ctx context.Context, relativePath string, depth string, directoryTarget bool) (webDAVMultiStatus, error) {
	body := strings.NewReader(`<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/><d:getcontentlength/><d:getlastmodified/><d:getetag/><d:getcontenttype/></d:prop></d:propfind>`)
	headers := make(http.Header)
	headers.Set("Depth", depth)
	headers.Set("Content-Type", "application/xml; charset=utf-8")
	response, err := client.request(ctx, "PROPFIND", relativePath, directoryTarget, body, headers)
	if err != nil {
		return webDAVMultiStatus{}, err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "list", relativePath, http.StatusMultiStatus); err != nil {
		drainWebDAVResponse(response.Body)
		return webDAVMultiStatus{}, err
	}
	data, err := readLimitedWebDAVBody(response.Body, maxWebDAVMultiStatusSize)
	if err != nil {
		return webDAVMultiStatus{}, bodyWebDAVError("list", relativePath, err)
	}
	var multiStatus webDAVMultiStatus
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	if err := decoder.Decode(&multiStatus); err != nil {
		if errors.Is(err, errWebDAVTooManyResponses) {
			return webDAVMultiStatus{}, &WebDAVError{Kind: WebDAVErrorTooLarge, Operation: "list", Path: relativePath, Err: err}
		}
		return webDAVMultiStatus{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "list", Path: relativePath, Err: errors.New("invalid multistatus XML")}
	}
	return multiStatus, nil
}

func (client *WebDAVClient) ensureAdvertisedRoot(ctx context.Context) error {
	if client == nil {
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "connect", Err: errors.New("WebDAV client is not configured")}
	}
	client.mu.RLock()
	known := client.advertisedRootKnown
	client.mu.RUnlock()
	if known {
		return nil
	}
	multiStatus, err := client.propfind(ctx, "", "0")
	if err != nil {
		return err
	}
	advertisedRoot, err := client.discoverAdvertisedRoot(multiStatus)
	if err != nil {
		return &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "connect", Err: err}
	}
	client.mu.Lock()
	if !client.advertisedRootKnown {
		client.advertisedRootPath = advertisedRoot
		client.advertisedRootKnown = true
	}
	client.mu.Unlock()
	return nil
}

func (client *WebDAVClient) discoverAdvertisedRoot(multiStatus webDAVMultiStatus) (string, error) {
	for _, response := range multiStatus.Responses {
		if strings.TrimSpace(response.Href) == "" {
			continue
		}
		resolved, err := client.resolveWebDAVHref(response.Href, client.baseURL)
		if err != nil {
			continue
		}
		rootPath := strings.TrimSuffix(resolved.Path, "/") + "/"
		if rootPath == "//" {
			rootPath = "/"
		}
		return rootPath, nil
	}
	return "", errors.New("Depth-0 response did not advertise a safe collection href")
}

func (client *WebDAVClient) resourceState(ctx context.Context, relativePath string) (string, bool, error) {
	if err := client.ensureAdvertisedRoot(ctx); err != nil {
		return "", false, err
	}
	multiStatus, err := client.propfind(ctx, relativePath, "0")
	if err != nil {
		if IsWebDAVErrorKind(err, WebDAVErrorNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	for _, response := range multiStatus.Responses {
		mappedPath, hrefErr := client.relativePathFromHrefForRequest(response.Href, relativePath, false)
		if hrefErr != nil || mappedPath != relativePath {
			continue
		}
		status := webDAVStatusLineCode(response.Status)
		if status == http.StatusNotFound || status == http.StatusGone {
			return "", false, nil
		}
		if status >= 400 {
			return "", false, &WebDAVError{Kind: webDAVStatusErrorKind(status), Operation: "stat", Path: relativePath, StatusCode: status}
		}
		properties, ok := response.successfulProperties()
		if ok {
			return strings.TrimSpace(properties.ETag), true, nil
		}
		return "", true, nil
	}
	return "", false, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "stat", Path: relativePath, Err: errors.New("Depth-0 response did not describe the resource")}
}

func (client *WebDAVClient) resourceMetadata(ctx context.Context, relativePath string, directory bool) (webDAVResourceMetadata, bool, error) {
	if err := client.ensureAdvertisedRoot(ctx); err != nil {
		return webDAVResourceMetadata{}, false, err
	}
	multiStatus, err := client.propfindTarget(ctx, relativePath, "0", directory)
	if err != nil {
		if IsWebDAVErrorKind(err, WebDAVErrorNotFound) {
			return webDAVResourceMetadata{}, false, nil
		}
		return webDAVResourceMetadata{}, false, err
	}
	for _, response := range multiStatus.Responses {
		mappedPath, hrefErr := client.relativePathFromHrefForRequest(response.Href, relativePath, directory)
		if hrefErr != nil || mappedPath != relativePath {
			continue
		}
		status := webDAVStatusLineCode(response.Status)
		if status == http.StatusNotFound || status == http.StatusGone {
			return webDAVResourceMetadata{}, false, nil
		}
		if status >= 400 {
			return webDAVResourceMetadata{}, false, &WebDAVError{Kind: webDAVStatusErrorKind(status), Operation: "stat", Path: relativePath, StatusCode: status}
		}
		properties, ok := response.successfulProperties()
		if !ok {
			return webDAVResourceMetadata{}, true, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "stat", Path: relativePath, Err: errors.New("Depth-0 response omitted resource properties")}
		}
		if properties.ResourceType == nil {
			return webDAVResourceMetadata{}, true, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "stat", Path: relativePath, Err: errors.New("Depth-0 response omitted the resourcetype property")}
		}
		metadata := webDAVResourceMetadata{
			Path:        mappedPath,
			ETag:        strings.TrimSpace(properties.ETag),
			Modified:    strings.TrimSpace(properties.LastModified),
			ContentType: strings.TrimSpace(properties.ContentType),
			Directory:   properties.ResourceType.Collection != nil,
		}
		if size, parseErr := strconv.ParseInt(strings.TrimSpace(properties.ContentLength), 10, 64); parseErr == nil && size >= 0 {
			metadata.Size = size
		}
		return metadata, true, nil
	}
	return webDAVResourceMetadata{}, false, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "stat", Path: relativePath, Err: errors.New("Depth-0 response did not describe the resource")}
}

func validateWebDAVMarkdownContent(normalized string, content string) error {
	if len(content) > maxWebDAVDocumentSize {
		return &WebDAVError{Kind: WebDAVErrorTooLarge, Operation: "write", Path: normalized, Err: errors.New("document exceeds the size limit")}
	}
	if !utf8.ValidString(content) {
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "write", Path: normalized, Err: errors.New("document is not valid UTF-8")}
	}
	return nil
}

func (client *WebDAVClient) tryExclusiveWriteLock(ctx context.Context, relativePath string) (string, bool, bool, error) {
	token, lockNullCreated, err := client.lockResource(ctx, relativePath)
	if err == nil {
		return token, lockNullCreated, true, nil
	}
	if IsWebDAVErrorKind(err, WebDAVErrorUnsupported) {
		return "", false, false, nil
	}
	return "", false, false, err
}

func (client *WebDAVClient) lockResource(ctx context.Context, relativePath string) (string, bool, error) {
	return client.lockResourceTarget(ctx, relativePath, false)
}

func (client *WebDAVClient) lockResourceTarget(ctx context.Context, relativePath string, directory bool) (string, bool, error) {
	return client.lockResourceTargetWithDepth(ctx, relativePath, directory, "0")
}

func (client *WebDAVClient) lockResourceTargetWithDepth(ctx context.Context, relativePath string, directory bool, depth string) (string, bool, error) {
	if depth != "0" && depth != "infinity" {
		return "", false, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "lock", Path: relativePath, Err: errors.New("invalid LOCK depth")}
	}
	body := strings.NewReader(`<?xml version="1.0" encoding="utf-8"?>
<d:lockinfo xmlns:d="DAV:"><d:lockscope><d:exclusive/></d:lockscope><d:locktype><d:write/></d:locktype><d:owner><d:href>urn:inkmark:markdown</d:href></d:owner></d:lockinfo>`)
	headers := make(http.Header)
	headers.Set("Content-Type", "application/xml; charset=utf-8")
	headers.Set("Depth", depth)
	headers.Set("Timeout", "Second-30")
	response, err := client.request(ctx, "LOCK", relativePath, directory, body, headers)
	if err != nil {
		return "", false, err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "lock", relativePath, http.StatusOK, http.StatusCreated); err != nil {
		drainWebDAVResponse(response.Body)
		return "", false, err
	}
	token := strings.TrimSpace(response.Header.Get("Lock-Token"))
	if token != "" {
		drainWebDAVResponse(response.Body)
	} else {
		payload, readErr := readLimitedWebDAVBody(response.Body, maxWebDAVLockResponse)
		if readErr != nil {
			return "", false, bodyWebDAVError("lock", relativePath, readErr)
		}
		var lockResponse webDAVLockResponse
		decoder := xml.NewDecoder(bytes.NewReader(payload))
		decoder.Strict = true
		if decoder.Decode(&lockResponse) == nil {
			token = strings.TrimSpace(lockResponse.LockDiscovery.ActiveLock.LockToken.Href)
			if token == "" {
				token = strings.TrimSpace(lockResponse.Property.LockDiscovery.ActiveLock.LockToken.Href)
			}
		}
	}
	token, err = normalizeWebDAVLockToken(token)
	if err != nil {
		return "", false, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "lock", Path: relativePath, Err: err}
	}
	return token, response.StatusCode == http.StatusCreated, nil
}

func (client *WebDAVClient) unlockResource(ctx context.Context, relativePath string, lockToken string) error {
	return client.unlockResourceTarget(ctx, relativePath, false, lockToken)
}

func (client *WebDAVClient) unlockResourceTarget(ctx context.Context, relativePath string, directory bool, lockToken string) error {
	headers := make(http.Header)
	headers.Set("Lock-Token", lockToken)
	response, err := client.request(ctx, "UNLOCK", relativePath, directory, nil, headers)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, "unlock", relativePath, http.StatusOK, http.StatusNoContent); err != nil {
		drainWebDAVResponse(response.Body)
		return err
	}
	drainWebDAVResponse(response.Body)
	return nil
}

func (client *WebDAVClient) deleteLockedResource(ctx context.Context, relativePath string, lockToken string) error {
	return client.deleteLockedResourceTarget(ctx, relativePath, false, lockToken, "delete lock-null")
}

func (client *WebDAVClient) deleteLockedResourceTarget(ctx context.Context, relativePath string, directory bool, lockToken string, operation string) error {
	headers := make(http.Header)
	headers.Set("If", "("+lockToken+")")
	response, err := client.request(ctx, http.MethodDelete, relativePath, directory, nil, headers)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := requireWebDAVStatus(response, operation, relativePath, http.StatusOK, http.StatusAccepted, http.StatusNoContent, http.StatusNotFound); err != nil {
		drainWebDAVResponse(response.Body)
		return err
	}
	drainWebDAVResponse(response.Body)
	return nil
}

func (client *WebDAVClient) bestEffortUnlock(relativePath string, lockToken string) {
	client.bestEffortUnlockTarget(relativePath, false, lockToken)
}

func (client *WebDAVClient) bestEffortUnlockTarget(relativePath string, directory bool, lockToken string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = client.unlockResourceTarget(ctx, relativePath, directory, lockToken)
}

// resourceMetadataAfterLock uses an independent bounded context so a caller
// cancellation immediately after HTTP 201 cannot turn an ambiguous response
// into destructive lock-null cleanup. Mutation callers use absence only to
// return NotFound and unlock; unlike create-only writes, they never DELETE a
// target solely because LOCK returned 201.
func (client *WebDAVClient) resourceMetadataAfterLock(relativePath string, directory bool) (webDAVResourceMetadata, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return client.resourceMetadata(ctx, relativePath, directory)
}

func (client *WebDAVClient) bestEffortDeleteLocked(relativePath string, lockToken string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = client.deleteLockedResource(ctx, relativePath, lockToken)
}

func normalizeWebDAVLockToken(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", errors.New("LOCK response did not include a lock token")
	}
	if !strings.HasPrefix(token, "<") {
		token = "<" + token
	}
	if !strings.HasSuffix(token, ">") {
		token += ">"
	}
	if len(token) > 2048 || len(token) < 3 || strings.ContainsAny(token, "\r\n\t ()") {
		return "", errors.New("LOCK response included an invalid lock token")
	}
	return token, nil
}

func (client *WebDAVClient) request(ctx context.Context, method string, relativePath string, directory bool, body io.Reader, headers http.Header) (*http.Response, error) {
	if client == nil || client.baseURL == nil || client.client == nil {
		return nil, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: strings.ToLower(method), Path: relativePath, Err: errors.New("WebDAV client is not configured")}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	resourceURL, err := client.resourceURL(relativePath, directory)
	if err != nil {
		return nil, invalidWebDAVPathError(strings.ToLower(method), relativePath, err)
	}
	request, err := http.NewRequestWithContext(ctx, method, resourceURL.String(), body)
	if err != nil {
		return nil, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: strings.ToLower(method), Path: relativePath, Err: errors.New("could not create request")}
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	request.Header.Set("Accept", "application/xml, text/plain, */*")
	request.Header.Set("User-Agent", "InkMark/"+appVersion)
	client.mu.RLock()
	username := client.username
	password := client.password
	client.mu.RUnlock()
	if username != "" {
		request.SetBasicAuth(username, password)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return nil, transportWebDAVError(strings.ToLower(method), relativePath, ctx, err)
	}
	return response, nil
}

func (client *WebDAVClient) resourceURL(relativePath string, directory bool) (*url.URL, error) {
	normalized, err := normalizeWebDAVPath(relativePath)
	if err != nil {
		return nil, err
	}
	resource := *client.baseURL
	resource.RawPath = ""
	resource.Path = client.baseURL.Path + normalized
	if directory && !strings.HasSuffix(resource.Path, "/") {
		resource.Path += "/"
	}
	return &resource, nil
}

func (client *WebDAVClient) relativePathFromHref(rawHref string, requestedPath string) (string, error) {
	return client.relativePathFromHrefForRequest(rawHref, requestedPath, true)
}

func (client *WebDAVClient) relativePathFromHrefForRequest(rawHref string, requestedPath string, directory bool) (string, error) {
	client.mu.RLock()
	advertisedRoot := client.advertisedRootPath
	known := client.advertisedRootKnown
	client.mu.RUnlock()
	if !known || advertisedRoot == "" {
		return "", errors.New("WebDAV collection href is not known")
	}
	reference := *client.baseURL
	reference.RawPath = ""
	reference.Path = advertisedRoot + requestedPath
	if directory && !strings.HasSuffix(reference.Path, "/") {
		reference.Path += "/"
	}
	resolved, err := client.resolveWebDAVHref(rawHref, &reference)
	if err != nil {
		return "", err
	}
	rootWithoutSlash := strings.TrimSuffix(advertisedRoot, "/")
	if resolved.Path == rootWithoutSlash || resolved.Path == advertisedRoot {
		return "", nil
	}
	if !strings.HasPrefix(resolved.Path, advertisedRoot) {
		return "", errors.New("WebDAV href left the advertised collection")
	}
	relative := strings.TrimPrefix(resolved.Path, advertisedRoot)
	relative = strings.TrimSuffix(relative, "/")
	return normalizeWebDAVPath(relative)
}

func (client *WebDAVClient) resolveWebDAVHref(rawHref string, reference *url.URL) (*url.URL, error) {
	href, err := url.Parse(strings.TrimSpace(rawHref))
	if err != nil || href.User != nil || href.RawQuery != "" || href.Fragment != "" {
		return nil, errors.New("invalid WebDAV href")
	}
	if hasEncodedWebDAVSeparator(href.EscapedPath()) {
		return nil, errors.New("ambiguous encoded WebDAV href")
	}
	resolved := reference.ResolveReference(href)
	if !sameWebDAVOrigin(client.baseURL, resolved) {
		return nil, errors.New("WebDAV href left the configured server origin")
	}
	if err := validateDecodedWebDAVURLPath(resolved.Path); err != nil {
		return nil, err
	}
	return resolved, nil
}

func (client *WebDAVClient) displayLocation(relativePath string) string {
	if client == nil || client.baseURL == nil {
		return relativePath
	}
	return client.baseURL.Host + "/" + relativePath
}

func normalizeWebDAVBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return nil, errors.New("base URL must be an absolute HTTP or HTTPS URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackWebDAVHost(parsed.Hostname())) {
		return nil, errors.New("base URL must use HTTPS; HTTP is allowed only for loopback testing")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, errors.New("base URL must not contain credentials, a query, or a fragment")
	}
	if hasEncodedWebDAVSeparator(parsed.EscapedPath()) {
		return nil, errors.New("base URL contains an ambiguous encoded path separator")
	}
	if err := validateDecodedWebDAVURLPath(parsed.Path); err != nil {
		return nil, err
	}
	parsed.RawPath = ""
	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/"
	if parsed.Path == "//" {
		parsed.Path = "/"
	}
	return parsed, nil
}

func safeWebDAVEndpoint(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "<invalid>"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

func normalizeWebDAVPath(raw string) (string, error) {
	if raw == "" || raw == "." || raw == "/" {
		return "", nil
	}
	if len(raw) > maxWebDAVPathLength || !utf8.ValidString(raw) {
		return "", errors.New("path is too long or is not valid UTF-8")
	}
	if strings.HasPrefix(raw, "/") || strings.Contains(raw, "\\") || strings.Contains(raw, "//") {
		return "", errors.New("path must be a relative slash-separated path")
	}
	raw = strings.TrimSuffix(raw, "/")
	for _, character := range raw {
		if character == 0 || character < 0x20 || character == 0x7f {
			return "", errors.New("path contains control characters")
		}
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("path contains an empty or traversal segment")
		}
	}
	return raw, nil
}

func normalizeNonRootWebDAVPath(raw string) (string, error) {
	normalized, err := normalizeWebDAVPath(raw)
	if err != nil {
		return "", err
	}
	if normalized == "" {
		return "", errors.New("the WebDAV root cannot be changed")
	}
	return normalized, nil
}

func normalizeWebDAVMarkdownPath(raw string) (string, error) {
	normalized, err := normalizeNonRootWebDAVPath(raw)
	if err != nil {
		return "", err
	}
	extension := strings.ToLower(path.Ext(normalized))
	if extension != ".md" && extension != ".markdown" {
		return "", errors.New("only Markdown files can be opened or saved")
	}
	return normalized, nil
}

func validateDecodedWebDAVURLPath(value string) error {
	if len(value) > maxWebDAVPathLength || !utf8.ValidString(value) || strings.Contains(value, "\\") || (value != "/" && strings.Contains(value, "//")) {
		return errors.New("URL contains an invalid path")
	}
	for _, character := range value {
		if character == 0 || character < 0x20 || character == 0x7f {
			return errors.New("URL path contains control characters")
		}
	}
	for _, segment := range strings.Split(strings.Trim(value, "/"), "/") {
		if segment == "." || segment == ".." {
			return errors.New("URL path contains traversal segments")
		}
	}
	return nil
}

func validateWebDAVWriteOptions(options WebDAVWriteOptions) error {
	if options.CreateOnly && options.IfMatch != "" {
		return errors.New("create-only and If-Match cannot be combined")
	}
	if strings.ContainsAny(options.IfMatch, "\r\n") {
		return errors.New("If-Match contains invalid control characters")
	}
	return nil
}

func isLoopbackWebDAVHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func hasEncodedWebDAVSeparator(escapedPath string) bool {
	lower := strings.ToLower(escapedPath)
	return strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") || strings.Contains(lower, "%00")
}

func sameWebDAVOrigin(left *url.URL, right *url.URL) bool {
	if left == nil || right == nil || !strings.EqualFold(left.Scheme, right.Scheme) || !strings.EqualFold(left.Hostname(), right.Hostname()) {
		return false
	}
	return effectiveWebDAVPort(left) == effectiveWebDAVPort(right)
}

func effectiveWebDAVPort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	return "80"
}

func webDAVURLWithinBase(base *url.URL, candidate *url.URL) bool {
	if !sameWebDAVOrigin(base, candidate) {
		return false
	}
	root := strings.TrimSuffix(base.Path, "/")
	return candidate.Path == root || candidate.Path == base.Path || strings.HasPrefix(candidate.Path, base.Path)
}

func safeWebDAVRedirectURL(base *url.URL, candidate *url.URL) bool {
	if candidate == nil || candidate.User != nil || candidate.RawQuery != "" || candidate.ForceQuery || candidate.Fragment != "" {
		return false
	}
	if hasEncodedWebDAVSeparator(candidate.EscapedPath()) || validateDecodedWebDAVURLPath(candidate.Path) != nil {
		return false
	}
	return webDAVURLWithinBase(base, candidate)
}

func parentWebDAVPath(value string) string {
	parent := path.Dir(value)
	if parent == "." {
		return ""
	}
	return parent
}

func invalidWebDAVPathError(operation string, _ string, err error) error {
	return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: operation, Err: err}
}

func requireWebDAVStatus(response *http.Response, operation string, relativePath string, expected ...int) error {
	for _, status := range expected {
		if response.StatusCode == status {
			return nil
		}
	}
	return &WebDAVError{
		Kind:       webDAVStatusErrorKind(response.StatusCode),
		Operation:  operation,
		Path:       relativePath,
		StatusCode: response.StatusCode,
	}
}

func webDAVStatusErrorKind(status int) WebDAVErrorKind {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return WebDAVErrorProtocol
	case http.StatusUnauthorized, http.StatusProxyAuthRequired:
		return WebDAVErrorAuthentication
	case http.StatusForbidden:
		return WebDAVErrorPermission
	case http.StatusNotFound, http.StatusGone:
		return WebDAVErrorNotFound
	case http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return WebDAVErrorUnsupported
	case http.StatusConflict, http.StatusPreconditionFailed:
		return WebDAVErrorConflict
	case http.StatusRequestEntityTooLarge:
		return WebDAVErrorTooLarge
	case http.StatusLocked:
		return WebDAVErrorLocked
	case http.StatusTooManyRequests:
		return WebDAVErrorRateLimited
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return WebDAVErrorTimeout
	default:
		if status >= 500 {
			return WebDAVErrorServer
		}
		return WebDAVErrorProtocol
	}
}

func transportWebDAVError(operation string, relativePath string, ctx context.Context, err error) error {
	kind := WebDAVErrorNetwork
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		kind = WebDAVErrorCanceled
	} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		kind = WebDAVErrorTimeout
	} else {
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			kind = WebDAVErrorTimeout
		}
	}
	return &WebDAVError{Kind: kind, Operation: operation, Path: relativePath, Err: err}
}

func bodyWebDAVError(operation string, relativePath string, err error) error {
	kind := WebDAVErrorProtocol
	if errors.Is(err, errWebDAVBodyTooLarge) {
		kind = WebDAVErrorTooLarge
	}
	return &WebDAVError{Kind: kind, Operation: operation, Path: relativePath, Err: err}
}

var errWebDAVBodyTooLarge = errors.New("WebDAV response exceeds the size limit")
var errWebDAVTooManyResponses = errors.New("WebDAV multistatus contains too many responses")

func readLimitedWebDAVBody(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errWebDAVBodyTooLarge
	}
	return data, nil
}

func drainWebDAVResponse(reader io.Reader) {
	_, _ = io.Copy(io.Discard, io.LimitReader(reader, 4096))
}

type webDAVMultiStatus struct {
	Responses []webDAVResponse `xml:"response"`
}

func (multiStatus *webDAVMultiStatus) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	if start.Name.Local != "multistatus" {
		return errors.New("WebDAV response root is not multistatus")
	}
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch element := token.(type) {
		case xml.StartElement:
			if element.Name.Local != "response" {
				if err := decoder.Skip(); err != nil {
					return err
				}
				continue
			}
			if len(multiStatus.Responses) >= maxWebDAVResponses {
				return errWebDAVTooManyResponses
			}
			var response webDAVResponse
			if err := decoder.DecodeElement(&response, &element); err != nil {
				return err
			}
			multiStatus.Responses = append(multiStatus.Responses, response)
		case xml.EndElement:
			if element.Name == start.Name {
				return nil
			}
		}
	}
}

type webDAVResponse struct {
	Href      string               `xml:"href"`
	Status    string               `xml:"status"`
	PropStats []webDAVPropertyStat `xml:"propstat"`
}

func (response webDAVResponse) successfulProperties() (webDAVProperties, bool) {
	var merged webDAVProperties
	found := false
	for _, propertyStat := range response.PropStats {
		status := webDAVStatusLineCode(propertyStat.Status)
		if status < 200 || status >= 300 {
			continue
		}
		found = true
		properties := propertyStat.Properties
		if properties.ResourceType != nil {
			resourceType := *properties.ResourceType
			merged.ResourceType = &resourceType
		}
		if properties.ContentLength != "" {
			merged.ContentLength = properties.ContentLength
		}
		if properties.LastModified != "" {
			merged.LastModified = properties.LastModified
		}
		if properties.ETag != "" {
			merged.ETag = properties.ETag
		}
		if properties.ContentType != "" {
			merged.ContentType = properties.ContentType
		}
	}
	return merged, found
}

type webDAVPropertyStat struct {
	Properties webDAVProperties `xml:"prop"`
	Status     string           `xml:"status"`
}

type webDAVProperties struct {
	ResourceType  *webDAVResourceType `xml:"resourcetype"`
	ContentLength string              `xml:"getcontentlength"`
	LastModified  string              `xml:"getlastmodified"`
	ETag          string              `xml:"getetag"`
	ContentType   string              `xml:"getcontenttype"`
}

type webDAVResourceType struct {
	Collection *struct{} `xml:"collection"`
}

type webDAVLockResponse struct {
	LockDiscovery webDAVLockDiscovery `xml:"lockdiscovery"`
	Property      struct {
		LockDiscovery webDAVLockDiscovery `xml:"lockdiscovery"`
	} `xml:"prop"`
}

type webDAVLockDiscovery struct {
	ActiveLock struct {
		LockToken struct {
			Href string `xml:"href"`
		} `xml:"locktoken"`
	} `xml:"activelock"`
}

func webDAVStatusLineCode(status string) int {
	fields := strings.Fields(status)
	if len(fields) < 2 {
		return 0
	}
	code, _ := strconv.Atoi(fields[1])
	return code
}
