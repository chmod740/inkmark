import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import {
  SavedWebDAVFormError,
  buildSavedWebDAVConnectionInput,
  clearedWebDAVConnectionForm,
  normalizeSavedWebDAVConnections,
  recentWebDAVAction,
  resolveRecentWebDAVOpen,
  webDAVOriginChanged,
} from '../frontend/src/saved-webdav.ts'

const appURL = new URL('../frontend/src/App.vue', import.meta.url)

const saved = {
  id: 'connection-opaque-id',
  name: 'Team Documents',
  endpoint: 'https://dav.example.test/team/',
  username: 'editor',
  hasCredentials: true,
  credentialsAvailable: true,
  usernamePresent: true,
}

test('recent WebDAV entries auto-connect only through an opaque saved-connection ID', () => {
  assert.deepEqual(recentWebDAVAction({
    endpoint: saved.endpoint,
    name: saved.name,
    savedConnectionId: saved.id,
    hasSavedCredentials: true,
  }), { kind: 'connect-saved', connectionID: saved.id, endpoint: saved.endpoint })
  assert.deepEqual(recentWebDAVAction({
    endpoint: saved.endpoint,
    name: saved.name,
    savedConnectionId: saved.id,
    hasSavedCredentials: false,
  }), { kind: 'prompt', endpoint: saved.endpoint })
})

test('recent automatic connection falls back without exposing credentials', async () => {
  const calls = []
  const connected = await resolveRecentWebDAVOpen({
    endpoint: saved.endpoint,
    name: saved.name,
    savedConnectionId: saved.id,
    hasSavedCredentials: true,
  }, async (connectionID) => { calls.push(connectionID) })
  assert.deepEqual(calls, [saved.id])
  assert.deepEqual(connected, { kind: 'connected', endpoint: saved.endpoint })

  const failure = new Error('credential unavailable')
  const fallback = await resolveRecentWebDAVOpen({
    endpoint: saved.endpoint,
    name: saved.name,
    savedConnectionId: saved.id,
    hasSavedCredentials: true,
  }, async () => { throw failure })
  assert.deepEqual(fallback, { kind: 'prompt', endpoint: saved.endpoint, error: failure })
  assert.equal(Object.hasOwn(fallback, 'username'), false)
  assert.equal(Object.hasOwn(fallback, 'password'), false)
})

test('closing the manager creates a credential-free connection form', () => {
  const cleared = clearedWebDAVConnectionForm(`  ${saved.endpoint}  `)
  assert.equal(cleared.endpoint, saved.endpoint)
  assert.equal(cleared.username, '')
  assert.equal(cleared.password, '')
  assert.equal(cleared.showPassword, false)
  assert.equal(cleared.storeCredentials, false)
  assert.equal(cleared.removeCredentials, false)
})

test('new saved connections send credentials only after explicit opt-in', () => {
  const base = {
    mode: 'new',
    name: ' Team ',
    endpoint: ` ${saved.endpoint} `,
    username: 'editor',
    password: 'not-a-real-secret',
    removeCredentials: false,
  }
  assert.deepEqual(buildSavedWebDAVConnectionInput({ ...base, storeCredentials: false }), {
    id: '',
    name: 'Team',
    endpoint: saved.endpoint,
    username: '',
    password: '',
    replaceCredentials: false,
    removeCredentials: false,
  })
  const optedIn = buildSavedWebDAVConnectionInput({ ...base, storeCredentials: true })
  assert.equal(optedIn.username, 'editor')
  assert.equal(optedIn.password, 'not-a-real-secret')
  assert.equal(optedIn.replaceCredentials, true)
})

test('editing preserves a blank password and requires explicit replacement or removal', () => {
  const base = {
    mode: 'edit',
    id: saved.id,
    name: saved.name,
    endpoint: saved.endpoint,
    username: saved.username,
    storeCredentials: false,
    existing: saved,
  }
  const preserved = buildSavedWebDAVConnectionInput({ ...base, password: '', removeCredentials: false })
  assert.equal(preserved.replaceCredentials, false)
  assert.equal(preserved.removeCredentials, false)
  assert.equal(preserved.username, '')
  assert.equal(preserved.password, '')

  const replaced = buildSavedWebDAVConnectionInput({ ...base, password: 'replacement', removeCredentials: false })
  assert.equal(replaced.replaceCredentials, true)
  assert.equal(replaced.username, saved.username)
  assert.equal(replaced.password, 'replacement')

  const removed = buildSavedWebDAVConnectionInput({ ...base, password: '', removeCredentials: true })
  assert.equal(removed.removeCredentials, true)
  assert.equal(removed.replaceCredentials, false)
  assert.equal(removed.password, '')

  assert.throws(
    () => buildSavedWebDAVConnectionInput({ ...base, username: 'changed', password: '', removeCredentials: false }),
    (error) => error instanceof SavedWebDAVFormError && error.code === 'username_requires_password',
  )

  assert.equal(webDAVOriginChanged(saved.endpoint, 'https://dav.example.test/other/'), false)
  assert.equal(webDAVOriginChanged(saved.endpoint, 'https://other.example.test/team/'), true)
  assert.throws(
    () => buildSavedWebDAVConnectionInput({
      ...base,
      endpoint: 'https://other.example.test/team/',
      password: '',
      removeCredentials: false,
    }),
    (error) => error instanceof SavedWebDAVFormError && error.code === 'origin_requires_password',
  )
  assert.doesNotThrow(() => buildSavedWebDAVConnectionInput({
    ...base,
    endpoint: 'https://other.example.test/team/',
    password: '',
    removeCredentials: true,
  }))
})

test('saved connection normalization exposes metadata and username but ignores password-shaped input', () => {
  const [normalized] = normalizeSavedWebDAVConnections([{ ...saved, password: 'must-not-survive' }])
  assert.deepEqual(normalized, saved)
  assert.equal(Object.hasOwn(normalized, 'password'), false)
  assert.deepEqual(normalizeSavedWebDAVConnections([{ endpoint: saved.endpoint }]), [])
})

test('App manages saved connections without receiving a password from list or recent APIs', async () => {
  const app = await readFile(appURL, 'utf8')
  const recent = app.match(/async function openRecentWebDAV\([^)]*\) \{[\s\S]*?\n\}/)?.[0] || ''
  const connectSaved = app.match(/async function connectSavedWebDAV\([^)]*\) \{[\s\S]*?\n\}/)?.[0] || ''
  const save = app.match(/async function saveWebDAVConnection\([^)]*\) \{[\s\S]*?\n\}/)?.[0] || ''
  const remove = app.match(/async function confirmDeleteSavedWebDAVConnection\([^)]*\) \{[\s\S]*?\n\}/)?.[0] || ''
  const savedList = app.match(/<ul[^>]*class="saved-connection-list">[\s\S]*?<\/ul>/)?.[0] || ''

  for (const bridge of [
    'ListSavedWebDAVConnections',
    'SaveWebDAVConnection',
    'DeleteSavedWebDAVConnection',
    'ConnectSavedWebDAV',
  ]) assert.match(app, new RegExp(`\\b${bridge}\\b`))
  assert.match(recent, /resolveRecentWebDAVOpen\(connection/)
  assert.match(recent, /await ConnectSavedWebDAV\(connectionID\)/)
  assert.match(recent, /fallbackError = t\('webdav\.savedConnectFailed'/)
  assert.match(recent, /showWebDAVConnectionDialog\(endpoint, fallbackError\)/)
  assert.doesNotMatch(recent, /connection\.password|connection\.username/)
  assert.match(connectSaved, /await ConnectSavedWebDAV\(connection\.id\)/)
  assert.match(connectSaved, /showTemporaryWebDAVForm\(connection\.endpoint, connection\.username/)
  assert.match(save, /buildSavedWebDAVConnectionInput/)
  assert.match(save, /await SaveWebDAVConnection\(input\)/)
  assert.match(remove, /await DeleteSavedWebDAVConnection\(candidate\.id\)/)
  assert.match(app, /webDAVDeleteCandidate/)
  assert.match(app, /deleteConnectionMessage/)
  assert.match(app, /editingChangesWebDAVOrigin/)
  assert.match(app, /endpointChangeNeedsPassword/)
  assert.match(app, /credential_store: 'webdav\.credentialStoreUnavailable'/)
  assert.match(app, /local_storage: 'webdav\.localConnectionStorageUnavailable'/)
  assert.match(savedList, /connection\.name/)
  assert.match(savedList, /connection\.endpoint/)
  assert.match(savedList, /connection\.username/)
  assert.doesNotMatch(savedList, /password|webDAVPassword/i)
  assert.match(app, /previousDialog === 'webdav'[\s\S]*clearWebDAVConnectionManager\(\)/)
  assert.match(app, /function clearWebDAVConnectionForm[\s\S]*webDAVPassword\.value = ''/)
  assert.doesNotMatch(app, /localStorage\.setItem\([^\n]*(webdav|username|password|credential)/i)
  assert.doesNotMatch(app, /console\.(?:debug|info|log|warn|error)\([^\n]*(webdav|username|password|credential)/i)
})

test('WebDAV dialog separates saved connections from one-time and edit flows', async () => {
  const app = await readFile(appURL, 'utf8')

  assert.match(app, /type WebDAVDialogView = 'saved' \| 'temporary' \| 'new' \| 'edit'/)
  assert.match(app, /const webDAVDialogView = ref<WebDAVDialogView>\('saved'\)/)
  assert.match(app, /webDAVDialogView\.value = endpoint\.trim\(\) \|\| errorMessage \? 'temporary' : 'saved'/)
  assert.match(app, /class="webdav-dialog-tabs"[\s\S]*webdav\.savedTab[\s\S]*webdav\.temporaryTab/)
  assert.match(app, /:aria-pressed="webDAVDialogView !== 'temporary'"/)
  assert.doesNotMatch(app, /class="webdav-dialog-tabs"\s+role="tablist"/)
  assert.doesNotMatch(app, /aria-controls="webdav-(?:saved-panel|connection-form)"/)
  assert.match(app, /:disabled="webDAVDialogBusy \|\| webDAVDialogView === 'new' \|\| webDAVDialogView === 'edit'"/)
  assert.match(app, /v-if="webDAVDialogView === 'saved'"[\s\S]*class="saved-connections"/)
  assert.match(app, /<form\s+v-else\s+id="webdav-connection-form"/)
  assert.match(app, /function cancelSavedWebDAVForm\(\) \{[\s\S]*showSavedWebDAVConnections\(\)/)
  assert.match(app, /showSavedWebDAVConnections\(\)[\s\S]*await loadSavedWebDAVConnections\(\)/)

  const dialogBusy = app.match(/const webDAVDialogBusy = computed\(\(\) =>[\s\S]*?\)\nconst webDAVDialogBusyMessage/)?.[0] || ''
  assert.match(dialogBusy, /webDAVConnecting\.value/)
  assert.match(dialogBusy, /webDAVConnectionManagerBusy\.value/)
  assert.doesNotMatch(dialogBusy, /savedWebDAVConnectionsLoading/)
  assert.match(app, /savedWebDAVConnectionsError/)
  assert.match(app, /webdav\.retrySavedConnections/)
  assert.match(app, /webDAVConnectingConnectionID === connection\.id[\s\S]*webdav\.connecting/)
})

test('WebDAV dialog traps and restores focus while isolating delete confirmation', async () => {
  const app = await readFile(appURL, 'utf8')

  assert.match(app, /ref="appDialog"/)
  assert.match(app, /function trapActiveDialogFocus\(event: KeyboardEvent\) \{[\s\S]*event\.key !== 'Tab'[\s\S]*event\.preventDefault\(\)/)
  assert.match(app, /function activeDialogFocusScope\(\) \{[\s\S]*webDAVDeleteCandidate\.value[\s\S]*\.saved-connection-delete/)
  assert.match(app, /watch\(activeDialog,[\s\S]*dialogReturnFocus[\s\S]*returnFocus\?\.isConnected[\s\S]*returnFocus\.focus/)
  assert.match(app, /webDAVDeleteReturnFocus = document\.activeElement[\s\S]*returnFocus\?\.isConnected[\s\S]*returnFocus\.focus/)
  assert.match(app, /role="alertdialog"[\s\S]*data-dialog-initial[\s\S]*keepSavedConnection/)
  assert.match(app, /:inert="Boolean\(webDAVDeleteCandidate\)"/)
  assert.match(app, /class="saved-connections-content"[\s\S]*:aria-hidden="webDAVDeleteCandidate \? 'true' : undefined"/)
  assert.match(app, /id="webdav-delete-error"[\s\S]*savedWebDAVConnectionsError/)
  assert.match(app, /event\.key === 'Escape'[\s\S]*webDAVDeleteCandidate\.value[\s\S]*cancelDeleteSavedWebDAVConnection\(\)/)
  assert.match(app, /watch\(webDAVDialogBusy,[\s\S]*focusActiveDialog\(\)/)
  assert.match(app, /function handleMenuAction\(action: string\) \{[\s\S]*if \(activeDialog\.value\)[\s\S]*runEditAction\(action\)[\s\S]*return/)
  assert.match(app, /!element\.matches\(':disabled'\)[\s\S]*!element\.closest\('\[inert\]'\)/)
})

test('settings persist saved connection metadata but not plaintext credential fields', async () => {
  const appState = await readFile(new URL('../app_state.go', import.meta.url), 'utf8')
  const connections = await readFile(new URL('../webdav_connections.go', import.meta.url), 'utf8')
  const settings = appState.match(/type settingsState struct \{[\s\S]*?\n\}/)?.[0] || ''
  const persisted = connections.match(/type savedWebDAVConnectionState struct \{[\s\S]*?\n\}/)?.[0] || ''
  assert.match(settings, /SavedWebDAVConnections/)
  assert.doesNotMatch(settings, /Username|Password|WebDAVConfig/)
  assert.match(persisted, /Endpoint/)
  assert.doesNotMatch(persisted, /Username\s+string|Password\s+string/)
  assert.match(connections, /systemWebDAVCredentialStore/)
  assert.match(connections, /keyring\.Set/)
  assert.match(connections, /keyring\.Delete/)
})
