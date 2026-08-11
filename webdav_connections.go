package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	keyring "github.com/zalando/go-keyring"
)

const (
	maxSavedWebDAVConnections  = 32
	maxWebDAVConnectionName    = 128
	maxWebDAVCredentialPayload = 2048
	webDAVKeyringService       = "InkMark Markdown WebDAV"
)

var (
	errWebDAVCredentialsNotFound          = errors.New("saved WebDAV credentials were not found")
	errWebDAVCredentialStore              = errors.New("system credential storage is unavailable")
	errWebDAVLocalStorage                 = errors.New("local saved-connection storage is unavailable")
	errWebDAVCredentialRollbackIncomplete = errors.New("credential rollback was incomplete")
)

// Lock order for saved connections is savedWebDAVMu -> settingsWriteMu -> mu.
// Credential-vault I/O happens without mu, but while savedWebDAVMu is held, so
// a reader can never combine metadata and credentials from different updates.

// savedWebDAVConnectionState is the non-secret part of a saved connection.
// It is safe to persist in settings.json. Usernames and passwords are stored
// together in the operating system credential vault under the opaque ID.
type savedWebDAVConnectionState struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Endpoint         string `json:"endpoint"`
	CredentialsSaved bool   `json:"credentialsSaved,omitempty"`
	UsernamePresent  bool   `json:"usernamePresent,omitempty"`
}

// SavedWebDAVConnection is returned to the connection manager. It exposes
// only metadata and credential-presence flags, never authentication material.
type SavedWebDAVConnection struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Endpoint             string `json:"endpoint"`
	Username             string `json:"username"`
	HasCredentials       bool   `json:"hasCredentials"`
	CredentialsAvailable bool   `json:"credentialsAvailable"`
	UsernamePresent      bool   `json:"usernamePresent"`
}

// WebDAVConnectionInput creates a saved connection when ID is empty and
// updates an existing one otherwise. Credentials are unchanged by default.
// ReplaceCredentials and RemoveCredentials are explicit and mutually
// exclusive, so an empty password in an edit form cannot erase a saved one.
type WebDAVConnectionInput struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Endpoint           string `json:"endpoint"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	ReplaceCredentials bool   `json:"replaceCredentials"`
	RemoveCredentials  bool   `json:"removeCredentials"`
}

func (input WebDAVConnectionInput) String() string {
	return fmt.Sprintf("{ID:%q Name:%q Endpoint:%q Username:<redacted> Password:<redacted> ReplaceCredentials:%t RemoveCredentials:%t}",
		input.ID, input.Name, safeWebDAVEndpoint(input.Endpoint), input.ReplaceCredentials, input.RemoveCredentials)
}

func (input WebDAVConnectionInput) GoString() string { return input.String() }

// MarshalJSON keeps accidental diagnostics and snapshots credential-free.
// Wails can still decode Username and Password when this type is used as an
// input argument; this method affects only outbound serialization.
func (input WebDAVConnectionInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID                 string `json:"id"`
		Name               string `json:"name"`
		Endpoint           string `json:"endpoint"`
		ReplaceCredentials bool   `json:"replaceCredentials"`
		RemoveCredentials  bool   `json:"removeCredentials"`
	}{
		ID:                 input.ID,
		Name:               input.Name,
		Endpoint:           safeWebDAVEndpoint(input.Endpoint),
		ReplaceCredentials: input.ReplaceCredentials,
		RemoveCredentials:  input.RemoveCredentials,
	})
}

type webDAVCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Origin   string `json:"origin,omitempty"`
}

func (credentials webDAVCredentials) String() string {
	return "{Username:<redacted> Password:<redacted>}"
}

func (credentials webDAVCredentials) GoString() string { return credentials.String() }

type webDAVCredentialStore interface {
	Set(connectionID string, credentials webDAVCredentials) error
	Get(connectionID string) (webDAVCredentials, error)
	Delete(connectionID string) error
}

// systemWebDAVCredentialStore uses Keychain on macOS, Credential Manager on
// Windows, and Secret Service on supported Linux desktops through go-keyring.
type systemWebDAVCredentialStore struct{}

func (systemWebDAVCredentialStore) Set(connectionID string, credentials webDAVCredentials) error {
	payload, err := json.Marshal(credentials)
	if err != nil || len(payload) > maxWebDAVCredentialPayload {
		return errWebDAVCredentialStore
	}
	if err := keyring.Set(webDAVKeyringService, webDAVCredentialAccount(connectionID), string(payload)); err != nil {
		return errWebDAVCredentialStore
	}
	return nil
}

func (systemWebDAVCredentialStore) Get(connectionID string) (webDAVCredentials, error) {
	payload, err := keyring.Get(webDAVKeyringService, webDAVCredentialAccount(connectionID))
	if errors.Is(err, keyring.ErrNotFound) {
		return webDAVCredentials{}, errWebDAVCredentialsNotFound
	}
	if err != nil || len(payload) > maxWebDAVCredentialPayload {
		return webDAVCredentials{}, errWebDAVCredentialStore
	}
	var credentials webDAVCredentials
	if json.Unmarshal([]byte(payload), &credentials) != nil || validateWebDAVCredentialValues(credentials) != nil {
		return webDAVCredentials{}, errWebDAVCredentialStore
	}
	return credentials, nil
}

func (systemWebDAVCredentialStore) Delete(connectionID string) error {
	err := keyring.Delete(webDAVKeyringService, webDAVCredentialAccount(connectionID))
	if errors.Is(err, keyring.ErrNotFound) {
		return errWebDAVCredentialsNotFound
	}
	if err != nil {
		return errWebDAVCredentialStore
	}
	return nil
}

func webDAVCredentialAccount(connectionID string) string {
	return "connection:" + connectionID
}

func (a *App) ListSavedWebDAVConnections() []SavedWebDAVConnection {
	a.savedWebDAVMu.Lock()
	defer a.savedWebDAVMu.Unlock()

	a.mu.RLock()
	states := append([]savedWebDAVConnectionState(nil), a.savedWebDAVConnections...)
	a.mu.RUnlock()
	connections := make([]SavedWebDAVConnection, 0, len(states))
	var store webDAVCredentialStore
	for _, state := range states {
		username := ""
		available := false
		if state.CredentialsSaved {
			if store == nil {
				store = a.currentWebDAVCredentialStore()
			}
			if credentials, err := store.Get(state.ID); err == nil && webDAVCredentialsMatchEndpoint(credentials, state.Endpoint) {
				username = credentials.Username
				available = true
				credentials.Username = ""
				credentials.Password = ""
				credentials.Origin = ""
			}
		}
		connections = append(connections, publicSavedWebDAVConnection(state, username, available))
	}
	return connections
}

func (a *App) SaveWebDAVConnection(input WebDAVConnectionInput) (connection SavedWebDAVConnection, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()

	endpoint, defaultName, ok := normalizeRecentWebDAVEndpoint(input.Endpoint)
	if !ok {
		return SavedWebDAVConnection{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "save connection", Err: errors.New("the WebDAV endpoint is invalid")}
	}
	credentialOrigin, err := savedWebDAVCredentialOrigin(endpoint)
	if err != nil {
		return SavedWebDAVConnection{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "save connection", Err: errors.New("the WebDAV endpoint origin is invalid")}
	}
	name, err := normalizeWebDAVConnectionName(input.Name, defaultName)
	if err != nil {
		return SavedWebDAVConnection{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "save connection", Err: err}
	}
	if input.ReplaceCredentials && input.RemoveCredentials {
		return SavedWebDAVConnection{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "save connection", Err: errors.New("credential actions conflict")}
	}
	// An explicitly stored anonymous pair carries no authentication material.
	// Treat it as credential removal so recents do not advertise a credential
	// that is indistinguishable from an unauthenticated connection.
	if input.ReplaceCredentials && input.Username == "" && input.Password == "" {
		input.ReplaceCredentials = false
		input.RemoveCredentials = true
	}
	if input.ReplaceCredentials {
		credentials := webDAVCredentials{Username: input.Username, Password: input.Password, Origin: credentialOrigin}
		if err := validateWebDAVCredentialValues(credentials); err != nil {
			return SavedWebDAVConnection{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "save connection", Err: err}
		}
	}

	a.savedWebDAVMu.Lock()
	defer a.savedWebDAVMu.Unlock()
	a.settingsWriteMu.Lock()
	defer a.settingsWriteMu.Unlock()

	a.mu.Lock()
	if strings.TrimSpace(a.settingsPath) == "" {
		a.mu.Unlock()
		return SavedWebDAVConnection{}, savedWebDAVPersistenceError("save connection", errWebDAVLocalStorage, nil)
	}
	previousConnections := append([]savedWebDAVConnectionState(nil), a.savedWebDAVConnections...)
	previousRecent := append([]RecentItem(nil), a.recentItems...)
	connectionID := strings.TrimSpace(input.ID)
	created := connectionID == ""
	index := savedWebDAVConnectionIndex(previousConnections, connectionID)
	if created {
		if len(previousConnections) >= maxSavedWebDAVConnections {
			a.mu.Unlock()
			return SavedWebDAVConnection{}, &WebDAVError{Kind: WebDAVErrorTooLarge, Operation: "save connection", Err: errors.New("too many WebDAV connections are saved")}
		}
		connectionID, err = newOpaqueID()
		if err != nil {
			a.mu.Unlock()
			return SavedWebDAVConnection{}, &WebDAVError{Kind: WebDAVErrorProtocol, Operation: "save connection", Err: errors.New("could not create a connection capability")}
		}
		index = len(previousConnections)
		previousConnections = append(previousConnections, savedWebDAVConnectionState{ID: connectionID})
	} else if !validPersistentOpaqueID(connectionID) || index < 0 {
		a.mu.Unlock()
		return SavedWebDAVConnection{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "save connection", Err: errors.New("saved connection capability is invalid")}
	}
	previous := previousConnections[index]
	if previous.CredentialsSaved && !input.ReplaceCredentials && !input.RemoveCredentials && !sameSavedWebDAVOrigin(previous.Endpoint, endpoint) {
		a.mu.Unlock()
		return SavedWebDAVConnection{}, &WebDAVError{
			Kind:      WebDAVErrorInvalidInput,
			Operation: "save connection",
			Err:       errors.New("changing the WebDAV origin requires replacing or removing saved credentials"),
		}
	}
	next := previous
	next.ID = connectionID
	next.Name = name
	next.Endpoint = endpoint
	if input.ReplaceCredentials {
		next.CredentialsSaved = true
		next.UsernamePresent = input.Username != ""
	} else if input.RemoveCredentials {
		next.CredentialsSaved = false
		next.UsernamePresent = false
	}
	updatedConnections := append([]savedWebDAVConnectionState(nil), previousConnections...)
	updatedConnections[index] = next
	updatedRecent := updateRecentWebDAVConnection(previousRecent, next)
	a.mu.Unlock()

	credentialMutation := input.ReplaceCredentials || (input.RemoveCredentials && previous.CredentialsSaved)
	var store webDAVCredentialStore
	oldCredentials := webDAVCredentials{}
	hadOldCredentials := false
	if credentialMutation {
		store = a.currentWebDAVCredentialStore()
		oldCredentials, hadOldCredentials, err = readCredentialsForRollback(store, previous)
		if err != nil {
			return SavedWebDAVConnection{}, err
		}
		defer func() {
			oldCredentials.Username = ""
			oldCredentials.Password = ""
		}()
	}
	if input.ReplaceCredentials {
		if err := store.Set(connectionID, webDAVCredentials{Username: input.Username, Password: input.Password, Origin: credentialOrigin}); err != nil {
			return SavedWebDAVConnection{}, credentialStoreBridgeError("save", err)
		}
	} else if input.RemoveCredentials && previous.CredentialsSaved {
		if err := store.Delete(connectionID); err != nil && !errors.Is(err, errWebDAVCredentialsNotFound) {
			return SavedWebDAVConnection{}, credentialStoreBridgeError("delete", err)
		}
	}

	a.mu.Lock()
	a.savedWebDAVConnections = updatedConnections
	a.recentItems = updatedRecent
	state, settingsPath := a.settingsSnapshotLocked()
	a.mu.Unlock()
	if err := saveSettingsState(settingsPath, state); err != nil {
		a.mu.Lock()
		a.savedWebDAVConnections = previousConnectionsWithoutNew(previousConnections, created)
		a.recentItems = previousRecent
		a.mu.Unlock()
		var rollbackErr error
		if credentialMutation {
			rollbackErr = restoreWebDAVCredentials(store, connectionID, oldCredentials, hadOldCredentials)
		}
		return SavedWebDAVConnection{}, savedWebDAVPersistenceError("save connection", err, rollbackErr)
	}
	a.refreshApplicationMenu()
	responseUsername := ""
	responseCredentialsAvailable := false
	if input.ReplaceCredentials {
		responseUsername = input.Username
		responseCredentialsAvailable = true
	} else if !input.RemoveCredentials && next.CredentialsSaved {
		if store == nil {
			store = a.currentWebDAVCredentialStore()
		}
		if credentials, getErr := store.Get(connectionID); getErr == nil && webDAVCredentialsMatchEndpoint(credentials, next.Endpoint) {
			responseUsername = credentials.Username
			responseCredentialsAvailable = true
			credentials.Username = ""
			credentials.Password = ""
			credentials.Origin = ""
		}
	}
	return publicSavedWebDAVConnection(next, responseUsername, responseCredentialsAvailable), nil
}

func (a *App) DeleteSavedWebDAVConnection(connectionID string) (err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()

	connectionID = strings.TrimSpace(connectionID)
	if !validPersistentOpaqueID(connectionID) {
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "delete connection", Err: errors.New("saved connection capability is invalid")}
	}
	a.savedWebDAVMu.Lock()
	defer a.savedWebDAVMu.Unlock()
	a.settingsWriteMu.Lock()
	defer a.settingsWriteMu.Unlock()

	a.mu.Lock()
	if strings.TrimSpace(a.settingsPath) == "" {
		a.mu.Unlock()
		return savedWebDAVPersistenceError("delete connection", errWebDAVLocalStorage, nil)
	}
	index := savedWebDAVConnectionIndex(a.savedWebDAVConnections, connectionID)
	if index < 0 {
		a.mu.Unlock()
		return &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "delete connection", Err: errors.New("saved connection capability is invalid")}
	}
	previousConnections := append([]savedWebDAVConnectionState(nil), a.savedWebDAVConnections...)
	previousRecent := append([]RecentItem(nil), a.recentItems...)
	removed := previousConnections[index]
	nextConnections := append([]savedWebDAVConnectionState(nil), previousConnections[:index]...)
	nextConnections = append(nextConnections, previousConnections[index+1:]...)
	nextRecent := detachRecentWebDAVConnection(previousRecent, connectionID)
	a.mu.Unlock()

	store := a.currentWebDAVCredentialStore()
	oldCredentials, hadOldCredentials, err := readCredentialsForRollback(store, removed)
	if err != nil {
		return err
	}
	defer func() {
		oldCredentials.Username = ""
		oldCredentials.Password = ""
	}()
	if removed.CredentialsSaved {
		if err := store.Delete(connectionID); err != nil && !errors.Is(err, errWebDAVCredentialsNotFound) {
			return credentialStoreBridgeError("delete", err)
		}
	}

	a.mu.Lock()
	a.savedWebDAVConnections = nextConnections
	a.recentItems = nextRecent
	state, settingsPath := a.settingsSnapshotLocked()
	a.mu.Unlock()
	if err := saveSettingsState(settingsPath, state); err != nil {
		a.mu.Lock()
		a.savedWebDAVConnections = previousConnections
		a.recentItems = previousRecent
		a.mu.Unlock()
		rollbackErr := restoreWebDAVCredentials(store, connectionID, oldCredentials, hadOldCredentials)
		return savedWebDAVPersistenceError("delete connection", err, rollbackErr)
	}
	a.refreshApplicationMenu()
	return nil
}

func (a *App) ConnectSavedWebDAV(connectionID string) (workspace Workspace, err error) {
	defer func() { err = exposeWebDAVBridgeError(err) }()

	connectionID = strings.TrimSpace(connectionID)
	state, config, err := a.savedWebDAVConnectionConfig(connectionID)
	if err != nil {
		return Workspace{}, err
	}
	workspace, err = a.connectWebDAV(config, state.ID)
	config.Username = ""
	config.Password = ""
	return workspace, err
}

// savedWebDAVConnectionConfig snapshots metadata and credentials under the
// saved-connection transaction lock. The caller performs network I/O only
// after this method returns and releases the lock.
func (a *App) savedWebDAVConnectionConfig(connectionID string) (savedWebDAVConnectionState, WebDAVConfig, error) {
	a.savedWebDAVMu.Lock()
	defer a.savedWebDAVMu.Unlock()

	state, ok := a.savedWebDAVConnectionByID(connectionID)
	if !ok {
		return savedWebDAVConnectionState{}, WebDAVConfig{}, &WebDAVError{Kind: WebDAVErrorInvalidInput, Operation: "connect", Err: errors.New("saved connection capability is invalid")}
	}
	credentials := webDAVCredentials{}
	if state.CredentialsSaved {
		var err error
		credentials, err = a.currentWebDAVCredentialStore().Get(connectionID)
		if errors.Is(err, errWebDAVCredentialsNotFound) {
			a.markSavedWebDAVCredentialsMissingLocked(connectionID)
			return savedWebDAVConnectionState{}, WebDAVConfig{}, &WebDAVError{Kind: WebDAVErrorAuthentication, Operation: "connect", Err: errors.New("saved credentials are unavailable; enter them again")}
		}
		if err != nil {
			return savedWebDAVConnectionState{}, WebDAVConfig{}, credentialStoreBridgeError("read", err)
		}
		if !webDAVCredentialsMatchEndpoint(credentials, state.Endpoint) {
			credentials.Username = ""
			credentials.Password = ""
			credentials.Origin = ""
			return savedWebDAVConnectionState{}, WebDAVConfig{}, &WebDAVError{
				Kind:      WebDAVErrorAuthentication,
				Operation: "connect",
				Err:       errors.New("saved credentials do not match this WebDAV server; enter them again"),
			}
		}
	}
	config := WebDAVConfig{Endpoint: state.Endpoint, Username: credentials.Username, Password: credentials.Password}
	credentials.Username = ""
	credentials.Password = ""
	credentials.Origin = ""
	return state, config, nil
}

func (a *App) savedWebDAVConnectionByID(connectionID string) (savedWebDAVConnectionState, bool) {
	if !validPersistentOpaqueID(connectionID) {
		return savedWebDAVConnectionState{}, false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	index := savedWebDAVConnectionIndex(a.savedWebDAVConnections, connectionID)
	if index < 0 {
		return savedWebDAVConnectionState{}, false
	}
	return a.savedWebDAVConnections[index], true
}

func (a *App) currentWebDAVCredentialStore() webDAVCredentialStore {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.webDAVCredentialStore == nil {
		a.webDAVCredentialStore = systemWebDAVCredentialStore{}
	}
	return a.webDAVCredentialStore
}

// markSavedWebDAVCredentialsMissingLocked requires savedWebDAVMu.
func (a *App) markSavedWebDAVCredentialsMissingLocked(connectionID string) {
	a.settingsWriteMu.Lock()
	a.mu.Lock()
	index := savedWebDAVConnectionIndex(a.savedWebDAVConnections, connectionID)
	if index < 0 || !a.savedWebDAVConnections[index].CredentialsSaved {
		a.mu.Unlock()
		a.settingsWriteMu.Unlock()
		return
	}
	previous := a.savedWebDAVConnections[index]
	a.savedWebDAVConnections[index].CredentialsSaved = false
	a.savedWebDAVConnections[index].UsernamePresent = false
	state, settingsPath := a.settingsSnapshotLocked()
	a.mu.Unlock()
	if saveSettingsState(settingsPath, state) != nil {
		a.mu.Lock()
		if index < len(a.savedWebDAVConnections) && a.savedWebDAVConnections[index].ID == connectionID {
			a.savedWebDAVConnections[index] = previous
		}
		a.mu.Unlock()
	}
	a.settingsWriteMu.Unlock()
}

func normalizeLoadedSavedWebDAVConnections(states []savedWebDAVConnectionState) []savedWebDAVConnectionState {
	result := make([]savedWebDAVConnectionState, 0, min(len(states), maxSavedWebDAVConnections))
	seen := make(map[string]struct{}, len(states))
	for _, state := range states {
		if len(result) == maxSavedWebDAVConnections || !validPersistentOpaqueID(state.ID) {
			continue
		}
		if _, duplicate := seen[state.ID]; duplicate {
			continue
		}
		endpoint, defaultName, ok := normalizeRecentWebDAVEndpoint(state.Endpoint)
		if !ok {
			continue
		}
		name, err := normalizeWebDAVConnectionName(state.Name, defaultName)
		if err != nil {
			continue
		}
		state.Name = name
		state.Endpoint = endpoint
		if !state.CredentialsSaved {
			state.UsernamePresent = false
		}
		seen[state.ID] = struct{}{}
		result = append(result, state)
	}
	return result
}

func normalizeWebDAVConnectionName(raw string, fallback string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		name = strings.TrimSpace(fallback)
	}
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > maxWebDAVConnectionName || strings.ContainsRune(name, 0) {
		return "", errors.New("the saved connection name is invalid")
	}
	for _, character := range name {
		if character < 0x20 || character == 0x7f {
			return "", errors.New("the saved connection name contains control characters")
		}
	}
	return name, nil
}

func validateWebDAVCredentialValues(credentials webDAVCredentials) error {
	if strings.ContainsAny(credentials.Username, "\r\n") || strings.ContainsAny(credentials.Password, "\r\n") || strings.ContainsAny(credentials.Origin, "\r\n") {
		return errors.New("credentials contain invalid control characters")
	}
	if credentials.Username == "" && credentials.Password != "" {
		return errors.New("a password requires a username")
	}
	payload, err := json.Marshal(credentials)
	if err != nil || len(payload) > maxWebDAVCredentialPayload {
		return errors.New("credentials are too large")
	}
	return nil
}

func savedWebDAVCredentialOrigin(endpoint string) (string, error) {
	normalized, err := normalizeWebDAVBaseURL(endpoint)
	if err != nil || normalized == nil || normalized.Scheme == "" || normalized.Host == "" {
		return "", errors.New("invalid WebDAV origin")
	}
	return strings.ToLower(normalized.Scheme) + "://" + strings.ToLower(normalized.Host), nil
}

func webDAVCredentialsMatchEndpoint(credentials webDAVCredentials, endpoint string) bool {
	if credentials.Origin == "" {
		// Legacy vault entries were not scoped to an origin. They stay in the
		// system vault for recovery, but are never used for automatic login.
		return false
	}
	expected, err := savedWebDAVCredentialOrigin(endpoint)
	if err != nil {
		return false
	}
	actual, err := savedWebDAVCredentialOrigin(credentials.Origin)
	return err == nil && actual == expected
}

func validPersistentOpaqueID(value string) bool {
	if len(value) != 32 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16
}

func savedWebDAVConnectionIndex(states []savedWebDAVConnectionState, connectionID string) int {
	for index, state := range states {
		if state.ID == connectionID {
			return index
		}
	}
	return -1
}

func publicSavedWebDAVConnection(state savedWebDAVConnectionState, username string, credentialsAvailable bool) SavedWebDAVConnection {
	return SavedWebDAVConnection{
		ID:                   state.ID,
		Name:                 state.Name,
		Endpoint:             state.Endpoint,
		Username:             username,
		HasCredentials:       state.CredentialsSaved,
		CredentialsAvailable: credentialsAvailable,
		UsernamePresent:      state.UsernamePresent,
	}
}

func updateRecentWebDAVConnection(items []RecentItem, connection savedWebDAVConnectionState) []RecentItem {
	result := append([]RecentItem(nil), items...)
	for index := range result {
		if result[index].Kind != "webdav" || result[index].ConnectionID != connection.ID {
			continue
		}
		_, host, ok := normalizeRecentWebDAVEndpoint(connection.Endpoint)
		if !ok {
			continue
		}
		result[index].Path = connection.Endpoint
		result[index].Name = host
	}
	return result
}

func detachRecentWebDAVConnection(items []RecentItem, connectionID string) []RecentItem {
	result := make([]RecentItem, 0, len(items))
	for _, item := range items {
		if item.Kind == "webdav" && item.ConnectionID == connectionID {
			// A recent item bound to deleted credentials must not be silently
			// rebound to another account that happens to share the endpoint.
			continue
		}
		result = append(result, item)
	}
	return result
}

func readCredentialsForRollback(store webDAVCredentialStore, state savedWebDAVConnectionState) (webDAVCredentials, bool, error) {
	if !state.CredentialsSaved {
		return webDAVCredentials{}, false, nil
	}
	credentials, err := store.Get(state.ID)
	if errors.Is(err, errWebDAVCredentialsNotFound) {
		return webDAVCredentials{}, false, nil
	}
	if err != nil {
		return webDAVCredentials{}, false, credentialStoreBridgeError("read", err)
	}
	return credentials, true, nil
}

func restoreWebDAVCredentials(store webDAVCredentialStore, connectionID string, credentials webDAVCredentials, present bool) error {
	defer func() {
		credentials.Username = ""
		credentials.Password = ""
	}()
	if present {
		if err := store.Set(connectionID, credentials); err != nil {
			return credentialStoreBridgeError("restore", err)
		}
		return nil
	}
	if err := store.Delete(connectionID); err != nil && !errors.Is(err, errWebDAVCredentialsNotFound) {
		return credentialStoreBridgeError("restore", err)
	}
	return nil
}

func previousConnectionsWithoutNew(states []savedWebDAVConnectionState, created bool) []savedWebDAVConnectionState {
	if !created || len(states) == 0 {
		return append([]savedWebDAVConnectionState(nil), states...)
	}
	return append([]savedWebDAVConnectionState(nil), states[:len(states)-1]...)
}

func credentialStoreBridgeError(operation string, _ error) error {
	return &WebDAVError{Kind: WebDAVErrorCredentialStore, Operation: operation + " credentials", Err: errWebDAVCredentialStore}
}

func savedWebDAVPersistenceError(operation string, cause error, rollbackErr error) error {
	if rollbackErr != nil {
		cause = errors.Join(cause, fmt.Errorf("%w: %v", errWebDAVCredentialRollbackIncomplete, rollbackErr))
	}
	return &WebDAVError{Kind: WebDAVErrorLocalStorage, Operation: operation, Err: cause}
}

func sameSavedWebDAVOrigin(left string, right string) bool {
	leftURL, leftErr := normalizeWebDAVBaseURL(left)
	rightURL, rightErr := normalizeWebDAVBaseURL(right)
	return leftErr == nil && rightErr == nil && sameWebDAVOrigin(leftURL, rightURL)
}
