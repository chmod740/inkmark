package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryWebDAVCredentialStore struct {
	mu           sync.Mutex
	values       map[string]webDAVCredentials
	setErr       error
	getErr       error
	deleteErr    error
	setErrors    []error
	deleteErrors []error
}

func newMemoryWebDAVCredentialStore() *memoryWebDAVCredentialStore {
	return &memoryWebDAVCredentialStore{values: make(map[string]webDAVCredentials)}
}

func (store *memoryWebDAVCredentialStore) Set(connectionID string, credentials webDAVCredentials) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.setErrors) != 0 {
		err := store.setErrors[0]
		store.setErrors = store.setErrors[1:]
		if err != nil {
			return err
		}
	}
	if store.setErr != nil {
		return store.setErr
	}
	store.values[connectionID] = credentials
	return nil
}

func (store *memoryWebDAVCredentialStore) Get(connectionID string) (webDAVCredentials, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.getErr != nil {
		return webDAVCredentials{}, store.getErr
	}
	credentials, ok := store.values[connectionID]
	if !ok {
		return webDAVCredentials{}, errWebDAVCredentialsNotFound
	}
	return credentials, nil
}

func (store *memoryWebDAVCredentialStore) Delete(connectionID string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deleteErrors) != 0 {
		err := store.deleteErrors[0]
		store.deleteErrors = store.deleteErrors[1:]
		if err != nil {
			return err
		}
	}
	if store.deleteErr != nil {
		return store.deleteErr
	}
	if _, ok := store.values[connectionID]; !ok {
		return errWebDAVCredentialsNotFound
	}
	delete(store.values, connectionID)
	return nil
}

type blockingWebDAVCredentialStore struct {
	mu sync.Mutex

	values map[string]webDAVCredentials

	blockNextSet bool
	setStarted   chan struct{}
	releaseSet   chan struct{}

	blockNextGetAsMissing bool
	getStarted            chan struct{}
	releaseGet            chan struct{}
}

func newBlockingWebDAVCredentialStore() *blockingWebDAVCredentialStore {
	return &blockingWebDAVCredentialStore{values: make(map[string]webDAVCredentials)}
}

func (store *blockingWebDAVCredentialStore) pauseNextSet() (<-chan struct{}, chan<- struct{}) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.blockNextSet = true
	store.setStarted = make(chan struct{})
	store.releaseSet = make(chan struct{})
	return store.setStarted, store.releaseSet
}

func (store *blockingWebDAVCredentialStore) pauseNextGetAsMissing() (<-chan struct{}, chan<- struct{}) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.blockNextGetAsMissing = true
	store.getStarted = make(chan struct{})
	store.releaseGet = make(chan struct{})
	return store.getStarted, store.releaseGet
}

func (store *blockingWebDAVCredentialStore) Set(connectionID string, credentials webDAVCredentials) error {
	store.mu.Lock()
	store.values[connectionID] = credentials
	block := store.blockNextSet
	started := store.setStarted
	release := store.releaseSet
	store.blockNextSet = false
	store.mu.Unlock()
	if block {
		close(started)
		<-release
	}
	return nil
}

func (store *blockingWebDAVCredentialStore) Get(connectionID string) (webDAVCredentials, error) {
	store.mu.Lock()
	blockAsMissing := store.blockNextGetAsMissing
	started := store.getStarted
	release := store.releaseGet
	store.blockNextGetAsMissing = false
	if !blockAsMissing {
		credentials, ok := store.values[connectionID]
		store.mu.Unlock()
		if !ok {
			return webDAVCredentials{}, errWebDAVCredentialsNotFound
		}
		return credentials, nil
	}
	store.mu.Unlock()
	close(started)
	<-release
	return webDAVCredentials{}, errWebDAVCredentialsNotFound
}

func (store *blockingWebDAVCredentialStore) Delete(connectionID string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.values[connectionID]; !ok {
		return errWebDAVCredentialsNotFound
	}
	delete(store.values, connectionID)
	return nil
}

func TestSavedWebDAVConnectionPersistsOnlyMetadataAndVaultsCredentials(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "InkMark", "settings.json")
	store := newMemoryWebDAVCredentialStore()
	app := &App{
		language:              LanguageState{Mode: "auto", Locale: "en"},
		settingsPath:          settingsPath,
		webDAVCredentialStore: store,
	}
	input := WebDAVConnectionInput{
		Name:               "Work server",
		Endpoint:           "https://EXAMPLE.com:443/netdisk/api/webdav",
		Username:           "vault-only-user-83",
		Password:           "vault-only-password-47",
		ReplaceCredentials: true,
	}
	connection, err := app.SaveWebDAVConnection(input)
	if err != nil {
		t.Fatal(err)
	}
	if !validPersistentOpaqueID(connection.ID) || connection.Name != "Work server" || connection.Endpoint != "https://example.com/netdisk/api/webdav/" || connection.Username != input.Username || !connection.HasCredentials || !connection.CredentialsAvailable || !connection.UsernamePresent {
		t.Fatalf("unexpected saved connection metadata: %#v", connection)
	}
	credentials, err := store.Get(connection.ID)
	if err != nil || credentials.Username != input.Username || credentials.Password != input.Password || credentials.Origin != "https://example.com" {
		t.Fatalf("credentials were not sent to the vault: %#v, %v", credentials, err)
	}

	payload, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{input.Username, input.Password, `"username"`, `"password"`} {
		if bytes.Contains(bytes.ToLower(payload), bytes.ToLower([]byte(forbidden))) {
			t.Fatalf("settings leaked credential material %q: %s", forbidden, payload)
		}
	}
	loaded := loadSettingsState(settingsPath)
	if len(loaded.SavedWebDAVConnections) != 1 || loaded.SavedWebDAVConnections[0].ID != connection.ID || !loaded.SavedWebDAVConnections[0].CredentialsSaved {
		t.Fatalf("saved metadata did not reload: %#v", loaded.SavedWebDAVConnections)
	}

	listed := app.ListSavedWebDAVConnections()
	listedPayload, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || !listed[0].HasCredentials || !listed[0].CredentialsAvailable || listed[0].Username != input.Username || bytes.Contains(listedPayload, []byte(input.Password)) || bytes.Contains(bytes.ToLower(listedPayload), []byte(`"password"`)) {
		t.Fatalf("connection list exposed credentials: %s", listedPayload)
	}
	inputPayload, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(inputPayload, []byte(input.Username)) || bytes.Contains(inputPayload, []byte(input.Password)) || bytes.Contains(bytes.ToLower(inputPayload), []byte("username")) || bytes.Contains(bytes.ToLower(inputPayload), []byte("password")) {
		t.Fatalf("input diagnostics exposed credentials: %s", inputPayload)
	}
	var decodedInput WebDAVConnectionInput
	if err := json.Unmarshal([]byte(`{"name":"Decoded","endpoint":"https://example.com/dav/","username":"decoded-user","password":"decoded-password","replaceCredentials":true}`), &decodedInput); err != nil {
		t.Fatal(err)
	}
	if decodedInput.Username != "decoded-user" || decodedInput.Password != "decoded-password" || !decodedInput.ReplaceCredentials {
		t.Fatalf("custom outbound marshaling broke inbound Wails-style JSON decoding: %#v", decodedInput)
	}
}

func TestSavedWebDAVConnectionUpdateKeepsCredentialsUnlessExplicit(t *testing.T) {
	store := newMemoryWebDAVCredentialStore()
	app := &App{settingsPath: filepath.Join(t.TempDir(), "settings.json"), webDAVCredentialStore: store}
	created, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "Original",
		Endpoint:           "https://example.com/dav/",
		Username:           "alice",
		Password:           "original-password",
		ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		ID:       created.ID,
		Name:     "Renamed",
		Endpoint: "https://example.com/renamed-dav/",
		// Blank edit fields plus no explicit action must keep the vault entry.
	})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := store.Get(created.ID)
	if err != nil || credentials.Username != "alice" || credentials.Password != "original-password" {
		t.Fatalf("blank edit unexpectedly erased credentials: %#v, %v", credentials, err)
	}
	if !updated.HasCredentials || !updated.UsernamePresent || updated.Name != "Renamed" {
		t.Fatalf("metadata flags changed unexpectedly: %#v", updated)
	}

	replaced, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		ID:                 created.ID,
		Name:               updated.Name,
		Endpoint:           updated.Endpoint,
		Username:           "bob",
		Password:           "replacement-password",
		ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	credentials, _ = store.Get(created.ID)
	if credentials.Username != "bob" || credentials.Password != "replacement-password" || !replaced.HasCredentials {
		t.Fatalf("explicit credential replacement failed: %#v, %#v", credentials, replaced)
	}

	removed, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		ID:                created.ID,
		Name:              replaced.Name,
		Endpoint:          replaced.Endpoint,
		RemoveCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if removed.HasCredentials || removed.UsernamePresent {
		t.Fatalf("explicit credential removal left presence flags: %#v", removed)
	}
	if _, err := store.Get(created.ID); !errors.Is(err, errWebDAVCredentialsNotFound) {
		t.Fatalf("explicit credential removal left a vault entry: %v", err)
	}
}

func TestSavedWebDAVConnectionDeleteRemovesVaultAndBoundRecent(t *testing.T) {
	server, _ := newWebDAVBridgeServer(t)
	defer server.Close()
	store := newMemoryWebDAVCredentialStore()
	app := &App{settingsPath: filepath.Join(t.TempDir(), "settings.json"), webDAVCredentialStore: store}
	defer app.shutdown(nil)
	connection, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "Remote",
		Endpoint:           server.URL + "/netdisk/api/webdav/",
		Username:           webDAVTestUsername,
		Password:           webDAVTestPassword,
		ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ConnectSavedWebDAV(connection.ID); err != nil {
		t.Fatal(err)
	}
	recent := app.recentItemsSnapshot()
	if len(recent) != 1 || recent[0].ConnectionID != connection.ID {
		t.Fatalf("saved connection was not bound to recent: %#v", recent)
	}
	if err := app.DeleteSavedWebDAVConnection(connection.ID); err != nil {
		t.Fatal(err)
	}
	if len(app.ListSavedWebDAVConnections()) != 0 {
		t.Fatal("deleted connection remained in the manager")
	}
	if _, err := store.Get(connection.ID); !errors.Is(err, errWebDAVCredentialsNotFound) {
		t.Fatalf("deleted connection remained in credential storage: %v", err)
	}
	recent = app.recentItemsSnapshot()
	if len(recent) != 0 {
		t.Fatalf("deleting a saved connection should remove its account-bound recent item: %#v", recent)
	}
}

func TestSavedWebDAVCredentialsAreCryptographicallyScopedToEndpointOrigin(t *testing.T) {
	var requests atomic.Int32
	serverA := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		response.WriteHeader(http.StatusNoContent)
	}))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}))
	defer serverB.Close()

	connectionID, err := newOpaqueID()
	if err != nil {
		t.Fatal(err)
	}
	originB, err := savedWebDAVCredentialOrigin(serverB.URL + "/dav/")
	if err != nil {
		t.Fatal(err)
	}
	store := newMemoryWebDAVCredentialStore()
	if err := store.Set(connectionID, webDAVCredentials{Username: "server-b-user", Password: "server-b-password", Origin: originB}); err != nil {
		t.Fatal(err)
	}
	app := &App{
		savedWebDAVConnections: []savedWebDAVConnectionState{{
			ID:               connectionID,
			Name:             "Server A metadata",
			Endpoint:         serverA.URL + "/dav/",
			CredentialsSaved: true,
			UsernamePresent:  true,
		}},
		webDAVCredentialStore: store,
	}
	if _, err := app.ConnectSavedWebDAV(connectionID); !IsWebDAVErrorKind(err, WebDAVErrorAuthentication) {
		t.Fatalf("origin-mismatched credentials were not rejected before connecting: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("origin-mismatched credentials reached the configured server: %d requests", requests.Load())
	}
	listed := app.ListSavedWebDAVConnections()
	if len(listed) != 1 || listed[0].CredentialsAvailable || listed[0].Username != "" {
		t.Fatalf("origin-mismatched credentials were exposed as available: %#v", listed)
	}

	legacyID, err := newOpaqueID()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(legacyID, webDAVCredentials{Username: "legacy-user", Password: "legacy-password"}); err != nil {
		t.Fatal(err)
	}
	app.savedWebDAVConnections = append(app.savedWebDAVConnections, savedWebDAVConnectionState{
		ID: legacyID, Name: "Legacy", Endpoint: serverA.URL + "/dav/", CredentialsSaved: true, UsernamePresent: true,
	})
	if _, err := app.ConnectSavedWebDAV(legacyID); !IsWebDAVErrorKind(err, WebDAVErrorAuthentication) {
		t.Fatalf("unscoped legacy credentials were allowed to auto-connect: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("legacy credentials reached the configured server: %d requests", requests.Load())
	}
}

func TestOpenRecentWebDAVResolvesSavedCredentialWithoutExposingIt(t *testing.T) {
	server, _ := newWebDAVBridgeServer(t)
	defer server.Close()
	store := newMemoryWebDAVCredentialStore()
	app := &App{settingsPath: filepath.Join(t.TempDir(), "settings.json"), webDAVCredentialStore: store}
	defer app.shutdown(nil)
	connection, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "Remote",
		Endpoint:           server.URL + "/netdisk/api/webdav/",
		Username:           webDAVTestUsername,
		Password:           webDAVTestPassword,
		ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := app.ConnectSavedWebDAV(connection.ID)
	if err != nil || workspace.Provider != "webdav" {
		t.Fatalf("saved connection did not connect: %#v, %v", workspace, err)
	}
	recent := app.recentItemsSnapshot()[0]
	opened, err := app.OpenRecentWebDAV(recent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if opened.SavedConnectionID != connection.ID || !opened.HasSavedCredentials || opened.Endpoint != connection.Endpoint {
		t.Fatalf("recent item did not resolve its saved connection: %#v", opened)
	}
	payload, err := json.Marshal(opened)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{webDAVTestUsername, webDAVTestPassword, "username", "password"} {
		if bytes.Contains(bytes.ToLower(payload), bytes.ToLower([]byte(forbidden))) {
			t.Fatalf("recent response leaked credential material %q: %s", forbidden, payload)
		}
	}
}

func TestSavedWebDAVConnectionsDisambiguateAccountsOnOneEndpoint(t *testing.T) {
	store := newMemoryWebDAVCredentialStore()
	app := &App{settingsPath: filepath.Join(t.TempDir(), "settings.json"), webDAVCredentialStore: store}
	endpoint := "https://example.com/shared/dav/"
	first, err := app.SaveWebDAVConnection(WebDAVConnectionInput{Name: "Account A", Endpoint: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.SaveWebDAVConnection(WebDAVConnectionInput{Name: "Account B", Endpoint: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	firstRecent, _ := makeRecentItem("webdav", endpoint)
	firstRecent.ConnectionID = first.ID
	secondRecent, _ := makeRecentItem("webdav", endpoint)
	secondRecent.ConnectionID = second.ID
	items := prependRecentItem([]RecentItem{firstRecent}, secondRecent)
	if len(items) != 2 || items[0].ConnectionID != second.ID || items[1].ConnectionID != first.ID {
		t.Fatalf("same-endpoint saved accounts were conflated: %#v", items)
	}
	loaded := normalizeLoadedRecentItems(items)
	if len(loaded) != 2 || loaded[0].ConnectionID != second.ID || loaded[1].ConnectionID != first.ID {
		t.Fatalf("same-endpoint associations did not survive loading: %#v", loaded)
	}
}

func TestDeletedSameEndpointAccountNeverRebindsRecentToAnotherCredential(t *testing.T) {
	store := newMemoryWebDAVCredentialStore()
	app := &App{settingsPath: filepath.Join(t.TempDir(), "settings.json"), webDAVCredentialStore: store}
	endpoint := "https://example.com/shared/dav/"
	first, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name: "Account A", Endpoint: endpoint, Username: "account-a", Password: "password-a", ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name: "Account B", Endpoint: endpoint, Username: "account-b", Password: "password-b", ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	app.recordRecentWebDAV(endpoint, first.ID)
	if err := app.DeleteSavedWebDAVConnection(first.ID); err != nil {
		t.Fatal(err)
	}
	if recent := app.recentItemsSnapshot(); len(recent) != 0 {
		t.Fatalf("deleted account left a recent item which could be rebound: %#v", recent)
	}

	// Legacy entries without an account ID remain usable, but must reopen the
	// credential-free connection form even when exactly one matching account
	// is saved.
	app.recordRecentWebDAV(endpoint, "")
	legacy := app.recentItemsSnapshot()[0]
	opened, err := app.OpenRecentWebDAV(legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if opened.SavedConnectionID != "" || opened.HasSavedCredentials {
		t.Fatalf("legacy recent was rebound to account B: %#v (B=%s)", opened, second.ID)
	}

	bound, _ := makeRecentItem("webdav", endpoint)
	bound.ConnectionID = second.ID
	loaded := normalizeLoadedRecentItems([]RecentItem{legacy, bound})
	if len(loaded) != 2 || loaded[0].ConnectionID != "" || loaded[1].ConnectionID != second.ID {
		t.Fatalf("legacy and account-bound recents were conflated on reload: %#v", loaded)
	}
}

func TestSavedWebDAVMetadataAndCredentialOperationsRollbackTogether(t *testing.T) {
	store := newMemoryWebDAVCredentialStore()
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	app := &App{settingsPath: settingsPath, webDAVCredentialStore: store}
	connection, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "Original",
		Endpoint:           "https://example.com/dav/",
		Username:           "alice",
		Password:           "original-password",
		ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	notDirectory := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(notDirectory, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	app.mu.Lock()
	app.settingsPath = filepath.Join(notDirectory, "settings.json")
	app.mu.Unlock()
	if _, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		ID:                 connection.ID,
		Name:               "Should roll back",
		Endpoint:           "https://example.com/changed/",
		Username:           "bob",
		Password:           "changed-password",
		ReplaceCredentials: true,
	}); err == nil {
		t.Fatal("expected metadata persistence failure")
	}
	listed := app.ListSavedWebDAVConnections()
	credentials, credentialErr := store.Get(connection.ID)
	if len(listed) != 1 || listed[0].Name != "Original" || listed[0].Endpoint != connection.Endpoint || credentialErr != nil || credentials.Username != "alice" || credentials.Password != "original-password" {
		t.Fatalf("failed update did not roll back metadata and credentials: %#v, %#v, %v", listed, credentials, credentialErr)
	}
	if err := app.DeleteSavedWebDAVConnection(connection.ID); err == nil {
		t.Fatal("expected delete persistence failure")
	}
	credentials, credentialErr = store.Get(connection.ID)
	if len(app.ListSavedWebDAVConnections()) != 1 || credentialErr != nil || credentials.Password != "original-password" {
		t.Fatalf("failed delete did not restore metadata and credentials: %#v, %#v, %v", app.ListSavedWebDAVConnections(), credentials, credentialErr)
	}
}

func TestSavedWebDAVCredentialStoreFailureDoesNotMutateMetadata(t *testing.T) {
	store := newMemoryWebDAVCredentialStore()
	store.setErr = errors.New("simulated vault failure with no secret data")
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	app := &App{settingsPath: settingsPath, webDAVCredentialStore: store}
	_, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "Remote",
		Endpoint:           "https://example.com/dav/",
		Username:           "alice",
		Password:           "must-never-appear-in-error",
		ReplaceCredentials: true,
	})
	if !IsWebDAVErrorKind(err, WebDAVErrorCredentialStore) || !strings.Contains(err.Error(), "[INKMARK_WEBDAV:credential_store]") || strings.Contains(err.Error(), "must-never-appear") || strings.Contains(err.Error(), "alice") {
		t.Fatalf("credential storage failure was not safely reported: %v", err)
	}
	if len(app.ListSavedWebDAVConnections()) != 0 {
		t.Fatal("credential storage failure still created metadata")
	}
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("credential storage failure unexpectedly wrote settings: %v", err)
	}
}

func TestSavedWebDAVListSurvivesCredentialVaultReadFailure(t *testing.T) {
	store := newMemoryWebDAVCredentialStore()
	connectionID, err := newOpaqueID()
	if err != nil {
		t.Fatal(err)
	}
	store.getErr = errors.New("simulated unavailable vault")
	app := &App{
		savedWebDAVConnections: []savedWebDAVConnectionState{{
			ID:               connectionID,
			Name:             "Remote",
			Endpoint:         "https://example.com/dav/",
			CredentialsSaved: true,
			UsernamePresent:  true,
		}},
		webDAVCredentialStore: store,
	}
	listed := app.ListSavedWebDAVConnections()
	if len(listed) != 1 || listed[0].Username != "" || !listed[0].HasCredentials || listed[0].CredentialsAvailable || !listed[0].UsernamePresent {
		t.Fatalf("vault read failure should retain manageable metadata with unavailable status: %#v", listed)
	}
}

func TestMissingSavedCredentialIsMarkedAndRequiresReentry(t *testing.T) {
	store := newMemoryWebDAVCredentialStore()
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	connectionID, err := newOpaqueID()
	if err != nil {
		t.Fatal(err)
	}
	state := savedWebDAVConnectionState{
		ID:               connectionID,
		Name:             "Remote",
		Endpoint:         "https://example.com/dav/",
		CredentialsSaved: true,
		UsernamePresent:  true,
	}
	app := &App{settingsPath: settingsPath, savedWebDAVConnections: []savedWebDAVConnectionState{state}, webDAVCredentialStore: store}
	if _, err := app.ConnectSavedWebDAV(connectionID); !IsWebDAVErrorKind(err, WebDAVErrorAuthentication) {
		t.Fatalf("missing credential did not request reentry: %v", err)
	}
	listed := app.ListSavedWebDAVConnections()
	if len(listed) != 1 || listed[0].HasCredentials || listed[0].UsernamePresent {
		t.Fatalf("missing vault entry left stale presence flags: %#v", listed)
	}
	loaded := loadSettingsState(settingsPath)
	if len(loaded.SavedWebDAVConnections) != 1 || loaded.SavedWebDAVConnections[0].CredentialsSaved {
		t.Fatalf("missing credential state was not persisted: %#v", loaded.SavedWebDAVConnections)
	}
}

func TestLoadedSavedWebDAVConnectionsRejectForgedMetadata(t *testing.T) {
	validID, err := newOpaqueID()
	if err != nil {
		t.Fatal(err)
	}
	states := normalizeLoadedSavedWebDAVConnections([]savedWebDAVConnectionState{
		{ID: "attacker-controlled", Name: "Forged", Endpoint: "https://example.com/dav/", CredentialsSaved: true},
		{ID: validID, Name: "Safe", Endpoint: "https://EXAMPLE.com:443/dav", CredentialsSaved: true, UsernamePresent: true},
		{ID: validID, Name: "Duplicate", Endpoint: "https://other.example/dav/"},
		{ID: strings.Repeat("a", 32), Name: "Unsafe", Endpoint: "https://user:password@example.com/dav/"},
	})
	if len(states) != 1 || states[0].ID != validID || states[0].Endpoint != "https://example.com/dav/" || states[0].Name != "Safe" || !states[0].CredentialsSaved || !states[0].UsernamePresent {
		t.Fatalf("loaded saved connections trusted forged metadata: %#v", states)
	}
}

func TestSavedWebDAVConnectionUpdateAndConnectUseOneCredentialSnapshot(t *testing.T) {
	store := newBlockingWebDAVCredentialStore()
	app := &App{settingsPath: filepath.Join(t.TempDir(), "settings.json"), webDAVCredentialStore: store}
	created, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "Original",
		Endpoint:           "https://old.example/dav/",
		Username:           "old-user",
		Password:           "old-password",
		ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	setStarted, releaseSet := store.pauseNextSet()
	type saveOutcome struct {
		connection SavedWebDAVConnection
		err        error
	}
	saveDone := make(chan saveOutcome, 1)
	go func() {
		connection, saveErr := app.SaveWebDAVConnection(WebDAVConnectionInput{
			ID:                 created.ID,
			Name:               "Updated",
			Endpoint:           "https://new.example/dav/",
			Username:           "new-user",
			Password:           "new-password",
			ReplaceCredentials: true,
		})
		saveDone <- saveOutcome{connection: connection, err: saveErr}
	}()
	select {
	case <-setStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("credential replacement did not reach the controlled vault")
	}

	type configOutcome struct {
		state  savedWebDAVConnectionState
		config WebDAVConfig
		err    error
	}
	configStarted := make(chan struct{})
	configDone := make(chan configOutcome, 1)
	go func() {
		close(configStarted)
		state, config, configErr := app.savedWebDAVConnectionConfig(created.ID)
		configDone <- configOutcome{state: state, config: config, err: configErr}
	}()
	<-configStarted
	completedBeforeCommit := false
	var configured configOutcome
	select {
	case configured = <-configDone:
		completedBeforeCommit = true
	case <-time.After(75 * time.Millisecond):
	}
	close(releaseSet)

	var saved saveOutcome
	select {
	case saved = <-saveDone:
	case <-time.After(2 * time.Second):
		t.Fatal("credential replacement remained blocked")
	}
	if saved.err != nil {
		t.Fatal(saved.err)
	}
	if !completedBeforeCommit {
		select {
		case configured = <-configDone:
		case <-time.After(2 * time.Second):
			t.Fatal("saved connection snapshot remained blocked")
		}
	}
	if completedBeforeCommit {
		t.Fatal("saved connection snapshot observed the vault before metadata committed")
	}
	if configured.err != nil {
		t.Fatal(configured.err)
	}
	if configured.state.Endpoint != saved.connection.Endpoint || configured.config.Endpoint != saved.connection.Endpoint || configured.config.Username != "new-user" || configured.config.Password != "new-password" {
		t.Fatalf("metadata and credentials came from different transactions: state-endpoint=%q config-endpoint=%q user-match=%t password-match=%t", configured.state.Endpoint, configured.config.Endpoint, configured.config.Username == "new-user", configured.config.Password == "new-password")
	}
}

func TestMissingCredentialCannotOverwriteConcurrentReplacement(t *testing.T) {
	store := newBlockingWebDAVCredentialStore()
	app := &App{settingsPath: filepath.Join(t.TempDir(), "settings.json"), webDAVCredentialStore: store}
	created, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "Remote",
		Endpoint:           "https://example.com/dav/",
		Username:           "old-user",
		Password:           "old-password",
		ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	getStarted, releaseGet := store.pauseNextGetAsMissing()
	configDone := make(chan error, 1)
	go func() {
		_, _, configErr := app.savedWebDAVConnectionConfig(created.ID)
		configDone <- configErr
	}()
	select {
	case <-getStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("missing-credential read did not reach the controlled vault")
	}

	saveDone := make(chan error, 1)
	go func() {
		_, saveErr := app.SaveWebDAVConnection(WebDAVConnectionInput{
			ID:                 created.ID,
			Name:               "Remote",
			Endpoint:           created.Endpoint,
			Username:           "replacement-user",
			Password:           "replacement-password",
			ReplaceCredentials: true,
		})
		saveDone <- saveErr
	}()
	saveCompletedBeforeMissingMark := false
	var earlySaveErr error
	select {
	case earlySaveErr = <-saveDone:
		saveCompletedBeforeMissingMark = true
	case <-time.After(75 * time.Millisecond):
	}
	close(releaseGet)

	select {
	case configErr := <-configDone:
		if !IsWebDAVErrorKind(configErr, WebDAVErrorAuthentication) {
			t.Fatalf("missing credential returned the wrong error kind: %v", configErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("missing-credential snapshot remained blocked")
	}
	if !saveCompletedBeforeMissingMark {
		select {
		case saveErr := <-saveDone:
			if saveErr != nil {
				t.Fatal(saveErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("credential replacement remained blocked")
		}
	} else {
		if earlySaveErr != nil {
			t.Fatal(earlySaveErr)
		}
		t.Fatal("replacement completed before the missing-credential transaction finished")
	}

	listed := app.ListSavedWebDAVConnections()
	if len(listed) != 1 || !listed[0].HasCredentials || !listed[0].CredentialsAvailable || listed[0].Username != "replacement-user" {
		t.Fatalf("a stale missing marker overwrote the replacement: %#v", listed)
	}
}

func TestSavedWebDAVOriginChangeRequiresExplicitCredentialAction(t *testing.T) {
	store := newMemoryWebDAVCredentialStore()
	app := &App{settingsPath: filepath.Join(t.TempDir(), "settings.json"), webDAVCredentialStore: store}
	created, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "Scoped",
		Endpoint:           "https://example.com/dav/",
		Username:           "scoped-user",
		Password:           "scoped-password",
		ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		ID:       created.ID,
		Name:     created.Name,
		Endpoint: "https://other.example/dav/",
	}); !IsWebDAVErrorKind(err, WebDAVErrorInvalidInput) || !strings.Contains(err.Error(), "[INKMARK_WEBDAV:invalid_input]") {
		t.Fatalf("cross-origin credential retention was not rejected with a stable code: %v", err)
	}
	listed := app.ListSavedWebDAVConnections()
	if len(listed) != 1 || listed[0].Endpoint != created.Endpoint || !listed[0].HasCredentials {
		t.Fatalf("rejected origin change mutated the connection: %#v", listed)
	}

	sameOrigin, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		ID:       created.ID,
		Name:     created.Name,
		Endpoint: "https://EXAMPLE.com:443/another-path/",
	})
	if err != nil || sameOrigin.Endpoint != "https://example.com/another-path/" || !sameOrigin.HasCredentials {
		t.Fatalf("same-origin path change did not preserve credentials: %#v, %v", sameOrigin, err)
	}
	replaced, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		ID:                 created.ID,
		Name:               created.Name,
		Endpoint:           "https://other.example/dav/",
		Username:           "other-user",
		Password:           "other-password",
		ReplaceCredentials: true,
	})
	if err != nil || replaced.Endpoint != "https://other.example/dav/" || !replaced.HasCredentials {
		t.Fatalf("explicit cross-origin credential replacement failed: %#v, %v", replaced, err)
	}
	removed, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		ID:                created.ID,
		Name:              created.Name,
		Endpoint:          "https://third.example/dav/",
		RemoveCredentials: true,
	})
	if err != nil || removed.HasCredentials || removed.CredentialsAvailable {
		t.Fatalf("explicit cross-origin credential removal failed: %#v, %v", removed, err)
	}
}

func TestSavedWebDAVRollbackFailureIsStableObservableAndRedacted(t *testing.T) {
	store := newMemoryWebDAVCredentialStore()
	app := &App{settingsPath: filepath.Join(t.TempDir(), "settings.json"), webDAVCredentialStore: store}
	created, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "Original",
		Endpoint:           "https://example.com/dav/",
		Username:           "rollback-old-user",
		Password:           "rollback-old-password",
		ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	notDirectory := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(notDirectory, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	app.mu.Lock()
	app.settingsPath = filepath.Join(notDirectory, "settings.json")
	app.mu.Unlock()
	store.mu.Lock()
	store.setErrors = []error{nil, errors.New("simulated restore failure without secret material")}
	store.mu.Unlock()
	_, err = app.SaveWebDAVConnection(WebDAVConnectionInput{
		ID:                 created.ID,
		Name:               "Changed",
		Endpoint:           "https://example.com/changed/",
		Username:           "rollback-new-user",
		Password:           "rollback-new-password",
		ReplaceCredentials: true,
	})
	if !IsWebDAVErrorKind(err, WebDAVErrorLocalStorage) || !strings.Contains(err.Error(), "[INKMARK_WEBDAV:local_storage]") || !strings.Contains(err.Error(), errWebDAVCredentialRollbackIncomplete.Error()) {
		t.Fatalf("rollback failure was not stable and observable: %v", err)
	}
	for _, secret := range []string{"rollback-old-user", "rollback-old-password", "rollback-new-user", "rollback-new-password"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("rollback error leaked credential material %q", secret)
		}
	}
	app.mu.RLock()
	state := app.savedWebDAVConnections[0]
	app.mu.RUnlock()
	credentials, credentialErr := store.Get(created.ID)
	if state.Endpoint != created.Endpoint || credentialErr != nil || credentials.Username != "rollback-new-user" {
		t.Fatalf("rollback fixture did not exercise the expected incomplete state: endpoint-restored=%t credential-remained-new=%t err=%v", state.Endpoint == created.Endpoint, credentials.Username == "rollback-new-user", credentialErr)
	}

	newStore := newMemoryWebDAVCredentialStore()
	newStore.deleteErrors = []error{errors.New("simulated orphan cleanup failure")}
	newApp := &App{settingsPath: filepath.Join(notDirectory, "another-settings.json"), webDAVCredentialStore: newStore}
	_, err = newApp.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "New",
		Endpoint:           "https://new.example/dav/",
		Username:           "new-orphan-user",
		Password:           "new-orphan-password",
		ReplaceCredentials: true,
	})
	if !IsWebDAVErrorKind(err, WebDAVErrorLocalStorage) || !strings.Contains(err.Error(), errWebDAVCredentialRollbackIncomplete.Error()) {
		t.Fatalf("new-connection cleanup failure was not observable: %v", err)
	}
	if strings.Contains(err.Error(), "new-orphan-user") || strings.Contains(err.Error(), "new-orphan-password") {
		t.Fatal("new-connection cleanup error leaked credential material")
	}
	newStore.mu.Lock()
	orphanCount := len(newStore.values)
	newStore.mu.Unlock()
	if orphanCount != 1 {
		t.Fatalf("rollback fixture did not leave exactly one controlled orphan: %d", orphanCount)
	}
}

func TestSavedWebDAVStorageErrorsAndAnonymousCredentialsUseStableSemantics(t *testing.T) {
	store := newMemoryWebDAVCredentialStore()
	app := &App{webDAVCredentialStore: store}
	_, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "No settings",
		Endpoint:           "https://example.com/dav/",
		Username:           "must-not-be-stored",
		Password:           "must-not-be-stored",
		ReplaceCredentials: true,
	})
	if !IsWebDAVErrorKind(err, WebDAVErrorLocalStorage) || !strings.Contains(err.Error(), "[INKMARK_WEBDAV:local_storage]") {
		t.Fatalf("missing settings path returned the wrong stable error: %v", err)
	}
	store.mu.Lock()
	storedCount := len(store.values)
	store.mu.Unlock()
	if storedCount != 0 || len(app.ListSavedWebDAVConnections()) != 0 {
		t.Fatal("missing settings path still persisted credential state")
	}

	anonymousStore := newMemoryWebDAVCredentialStore()
	anonymousApp := &App{settingsPath: filepath.Join(t.TempDir(), "settings.json"), webDAVCredentialStore: anonymousStore}
	anonymous, err := anonymousApp.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "Public",
		Endpoint:           "https://public.example/dav/",
		ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if anonymous.HasCredentials || anonymous.CredentialsAvailable || anonymous.UsernamePresent {
		t.Fatalf("empty anonymous values were advertised as saved credentials: %#v", anonymous)
	}
	anonymousStore.mu.Lock()
	anonymousCount := len(anonymousStore.values)
	anonymousStore.mu.Unlock()
	if anonymousCount != 0 {
		t.Fatal("empty anonymous values were written to the credential vault")
	}

	redacted := fmt.Sprintf("%v %#v", webDAVCredentials{Username: "format-user", Password: "format-password"}, webDAVCredentials{Username: "format-user", Password: "format-password"})
	if strings.Contains(redacted, "format-user") || strings.Contains(redacted, "format-password") || !strings.Contains(redacted, "<redacted>") {
		t.Fatalf("credential diagnostics were not redacted: %s", redacted)
	}
}

func TestSavedWebDAVMetadataOnlyUpdateSurvivesVaultReadFailure(t *testing.T) {
	store := newMemoryWebDAVCredentialStore()
	app := &App{settingsPath: filepath.Join(t.TempDir(), "settings.json"), webDAVCredentialStore: store}
	created, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		Name:               "Original",
		Endpoint:           "https://example.com/dav/",
		Username:           "vault-user",
		Password:           "vault-password",
		ReplaceCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.getErr = errors.New("simulated unavailable vault")
	store.mu.Unlock()
	updated, err := app.SaveWebDAVConnection(WebDAVConnectionInput{
		ID:       created.ID,
		Name:     "Renamed while unavailable",
		Endpoint: "https://example.com/another-path/",
	})
	if err != nil {
		t.Fatalf("metadata-only update unnecessarily depended on the vault: %v", err)
	}
	if updated.Name != "Renamed while unavailable" || updated.Endpoint != "https://example.com/another-path/" || !updated.HasCredentials || updated.CredentialsAvailable {
		t.Fatalf("metadata-only update returned inconsistent availability: %#v", updated)
	}
}

func TestSystemWebDAVCredentialStoreRoundTrip(t *testing.T) {
	if os.Getenv("INKMARK_TEST_SYSTEM_KEYRING") != "1" {
		t.Skip("set INKMARK_TEST_SYSTEM_KEYRING=1 for an operating-system credential vault integration test")
	}
	connectionID, err := newOpaqueID()
	if err != nil {
		t.Fatal(err)
	}
	store := systemWebDAVCredentialStore{}
	credentials := webDAVCredentials{Username: "inkmark-keyring-test", Password: "temporary-keyring-secret", Origin: "https://example.com"}
	t.Cleanup(func() { _ = store.Delete(connectionID) })
	if err := store.Set(connectionID, credentials); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(connectionID)
	if err != nil || loaded != credentials {
		t.Fatalf("operating-system credential vault round trip failed: user-match=%t password-match=%t origin-match=%t err=%v", loaded.Username == credentials.Username, loaded.Password == credentials.Password, loaded.Origin == credentials.Origin, err)
	}
	if err := store.Delete(connectionID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(connectionID); !errors.Is(err, errWebDAVCredentialsNotFound) {
		t.Fatalf("deleted operating-system credential remained readable: %v", err)
	}
}
