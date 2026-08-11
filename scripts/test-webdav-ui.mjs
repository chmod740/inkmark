import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import {
  dispatchTextEditInput,
  isTextEditControl,
  resolveTextEditControl,
} from '../frontend/src/edit-actions.ts'

const appURL = new URL('../frontend/src/App.vue', import.meta.url)

function blockStartingAt(source, marker) {
  const markerIndex = source.indexOf(marker)
  assert.notEqual(markerIndex, -1, `missing source marker: ${marker}`)
  const openingBrace = source.indexOf('{', markerIndex)
  assert.notEqual(openingBrace, -1, `missing block after source marker: ${marker}`)
  let depth = 0
  for (let index = openingBrace; index < source.length; index += 1) {
    if (source[index] === '{') depth += 1
    else if (source[index] === '}') {
      depth -= 1
      if (depth === 0) return source.slice(markerIndex, index + 1)
    }
  }
  assert.fail(`unterminated block after source marker: ${marker}`)
}

function textControl({
  tagName = 'INPUT',
  type = 'text',
  value = '',
  connected = true,
  disabled = false,
  readOnly = false,
} = {}) {
  const events = []
  let focusCalls = 0
  return {
    tagName,
    type,
    value,
    disabled,
    readOnly,
    isConnected: connected,
    selectionStart: 0,
    selectionEnd: 0,
    focus() { focusCalls += 1 },
    setSelectionRange(start, end) {
      this.selectionStart = start
      this.selectionEnd = end
    },
    setRangeText(replacement, start, end, selectionMode = 'preserve') {
      this.value = `${this.value.slice(0, start)}${replacement}${this.value.slice(end)}`
      if (selectionMode === 'end') {
        this.selectionStart = start + replacement.length
        this.selectionEnd = this.selectionStart
      }
    },
    dispatchEvent(event) {
      events.push(event)
      return true
    },
    get events() { return events },
    get focusCalls() { return focusCalls },
  }
}

test('native edit target resolution covers every WebDAV credential field without falling through to the editor', () => {
  const editor = textControl({ tagName: 'TEXTAREA', value: 'editor remains unchanged' })
  const body = { tagName: 'BODY', isConnected: true }
  const nonEditableButton = { tagName: 'BUTTON', isConnected: true }

  for (const type of ['url', 'text', 'password']) {
    const credentialField = textControl({ type })
    assert.equal(isTextEditControl(credentialField), true)
    assert.equal(resolveTextEditControl(credentialField, null, editor), credentialField)
    assert.equal(resolveTextEditControl(body, credentialField, null), credentialField)
  }

  assert.equal(resolveTextEditControl(body, nonEditableButton, null), null)
  assert.equal(resolveTextEditControl(body, null, editor), editor)
  assert.equal(resolveTextEditControl(body, textControl({ connected: false }), null), null)
  assert.equal(resolveTextEditControl(body, null, textControl({ disabled: true })), null)
  assert.equal(resolveTextEditControl(body, null, textControl({ readOnly: true })), null)
  assert.equal(resolveTextEditControl(body, null, { tagName: 'DIV', isConnected: true }), null)
  assert.equal(editor.value, 'editor remains unchanged')
})

test('credential cut and paste notifications bubble so Vue v-model receives the new value', () => {
  for (const inputType of ['url', 'text', 'password']) {
    const credentialField = textControl({ type: inputType })
    dispatchTextEditInput(credentialField, 'insertFromPaste', inputType === 'password' ? null : 'test-value')
    dispatchTextEditInput(credentialField, 'deleteByCut')

    assert.equal(credentialField.events.length, 2)
    assert.deepEqual(credentialField.events.map((event) => event.type), ['input', 'input'])
    assert.equal(credentialField.events.every((event) => event.bubbles), true)
  }
})

test('one modal paste targets the credential field without mutating the editor', () => {
  const editor = textControl({ tagName: 'TEXTAREA', value: 'editor sentinel' })
  editor.setSelectionRange(2, 8)
  const credentialField = textControl({ type: 'url', value: 'https:///' })
  credentialField.setSelectionRange(8, 9)
  const body = { tagName: 'BODY', isConnected: true }
  const modal = { contains: (candidate) => candidate === credentialField }

  const target = resolveTextEditControl(body, credentialField, null)
  assert.equal(target, credentialField)
  assert.equal(modal.contains(target), true)
  target.setRangeText('dav.example/', target.selectionStart, target.selectionEnd, 'end')
  dispatchTextEditInput(target, 'insertFromPaste', 'dav.example/')

  assert.equal(credentialField.value, 'https://dav.example/')
  assert.equal(credentialField.events.length, 1)
  assert.equal(editor.value, 'editor sentinel')
  assert.deepEqual([editor.selectionStart, editor.selectionEnd], [2, 8])

  const outsideTarget = resolveTextEditControl(body, editor, null)
  assert.equal(modal.contains(outsideTarget), false)
  assert.equal(editor.value, 'editor sentinel')
  assert.equal(editor.events.length, 0)
})

test('WebView owns Windows edit shortcuts while a native menu click edits once', async () => {
  const app = await readFile(appURL, 'utf8')
  const onKeydown = blockStartingAt(app, 'function onKeydown')
  const handleMenuAction = blockStartingAt(app, 'function handleMenuAction')
  const modalMenuBranch = handleMenuAction.slice(0, handleMenuAction.indexOf("if (action === 'open-folder')"))
  const runEditAction = blockStartingAt(app, 'async function runEditAction')
  const pasteStart = runEditAction.indexOf("} else if (action === 'paste') {")
  const pasteEnd = runEditAction.indexOf("} else if (action === 'undo' || action === 'redo')", pasteStart)
  const pasteBranch = runEditAction.slice(pasteStart, pasteEnd)

  assert.doesNotMatch(onKeydown, /runEditAction|nativeEditShortcutAction/)
  assert.equal((handleMenuAction.match(/\['undo', 'redo', 'cut', 'copy', 'paste', 'select-all'\]\.includes\(action\)/g) || []).length, 2)
  assert.equal((modalMenuBranch.match(/runEditAction\(action\)/g) || []).length, 1)
  assert.match(modalMenuBranch, /runEditAction\(action\)[\s\S]*\breturn\b/)
  assert.equal((handleMenuAction.match(/runEditAction\(action\)/g) || []).length, 2)
  assert.equal((pasteBranch.match(/navigator\.clipboard\.readText\(\)/g) || []).length, 1)
  assert.equal((pasteBranch.match(/target\.setRangeText\(/g) || []).length, 1)
  assert.equal((pasteBranch.match(/dispatchTextEditInput\(/g) || []).length, 1)
})

test('WebDAV UI connects, browses, opens, saves, and closes through opaque capabilities', async () => {
  const app = await readFile(appURL, 'utf8')

  assert.match(app, /ConnectWebDAV\(\{ endpoint, username, password \}\)/)
  assert.match(app, /ListWebDAVDirectory\(activeWorkspace\.id, relativePath\)/)
  assert.match(app, /OpenWebDAVFile\(activeWorkspace\.id, entry\.path\)/)
  assert.match(app, /SaveWebDAVFile\([\s\S]*remoteWorkspaceId\.value[\s\S]*remoteDocumentId\.value[\s\S]*remoteDocumentETag\.value/)
  assert.match(app, /CloseWebDAVWorkspace\(/)
  assert.match(app, /action === 'connect-webdav'/)
  assert.match(app, /id="webdav-connection-form"/)
  assert.match(app, /id="webdav-password"/)
})

test('remote saves preserve dirty content on conflict and require an explicit resolution', async () => {
  const app = await readFile(appURL, 'utf8')

  assert.match(app, /if \(remoteResult\?\.conflict\) \{[\s\S]*webDAVConflictOpen\.value = true[\s\S]*return false/)
  assert.match(app, /reloadConflictingWebDAVDocument/)
  assert.match(app, /overwriteConflictingWebDAVDocument/)
  assert.match(app, /OverwriteWebDAVFile\([\s\S]*remoteWorkspaceId\.value[\s\S]*remoteDocumentId\.value/)
  assert.match(app, /webdav\.conflictReload/)
  assert.match(app, /webdav\.conflictOverwrite/)
  assert.match(app, /const saveAsHint = documentStorageKind\.value === 'webdav' \? fileName\.value : localDocumentPath\.value/)
  assert.match(app, /documentStorageKind\.value = 'local'[\s\S]*remoteDocumentId\.value = ''[\s\S]*remoteWorkspaceId\.value = ''/)
})

test('WebDAV credentials remain ephemeral and never enter browser persistence', async () => {
  const app = await readFile(appURL, 'utf8')
  const editActions = await readFile(new URL('../frontend/src/edit-actions.ts', import.meta.url), 'utf8')

  assert.match(app, /autocomplete="off"/)
  assert.match(app, /webDAVPassword\.value = ''/)
  assert.match(app, /onBeforeUnmount\(\(\) => \{[\s\S]*clearWebDAVConnectionManager\(\)/)
  assert.match(app, /function clearWebDAVConnectionManager\(\) \{[\s\S]*clearWebDAVConnectionForm\(\)/)
  assert.doesNotMatch(app, /localStorage\.setItem\([^\n]*(webdav|username|password|credential)/i)
  assert.doesNotMatch(app, /sessionStorage\.(setItem|getItem)\([^\n]*(webdav|username|password|credential)/i)
  assert.doesNotMatch(editActions, /(?:local|session)Storage|console\.(?:debug|info|log|warn|error)/)
})

test('recent WebDAV connections use an opaque saved ID or reopen a credential-free form', async () => {
  const app = await readFile(appURL, 'utf8')
  const recentEvent = app.match(/interface RecentOpenEventData \{[\s\S]*?\n\}/)?.[0] || ''
  const clearConnectionForm = app.match(/function clearWebDAVConnectionForm\([^)]*\) \{[\s\S]*?\n\}/)?.[0] || ''
  const showConnectionDialog = app.match(/function showWebDAVConnectionDialog\([^)]*\) \{[\s\S]*?\n\}/)?.[0] || ''
  const openRecentWebDAV = app.match(/async function openRecentWebDAV\([^)]*\) \{[\s\S]*?\n\}/)?.[0] || ''
  const openRecentItem = app.match(/async function openRecentItem\(value: unknown\) \{[\s\S]*?\n\}/)?.[0] || ''
  const webDAVBranch = openRecentItem.match(/if \(candidate\.kind === 'webdav'\) \{[\s\S]*?\n  \}/)?.[0] || ''

  assert.match(app, /OpenRecentWebDAV/)
  assert.match(recentEvent, /kind:[^\n]*'webdav'/)
  assert.match(clearConnectionForm, /clearedWebDAVConnectionForm/)
  assert.match(clearConnectionForm, /webDAVUsername\.value = cleared\.username/)
  assert.match(clearConnectionForm, /webDAVPassword\.value = cleared\.password/)
  assert.match(showConnectionDialog, /clearWebDAVConnectionForm\(\)[\s\S]*webDAVBaseURL\.value = endpoint\.trim\(\)[\s\S]*activeDialog\.value = 'webdav'/)
  assert.match(openRecentWebDAV, /await OpenRecentWebDAV\(id\)/)
  assert.match(openRecentWebDAV, /endpoint = connection\?\.endpoint\?\.trim\(\) \|\| ''/)
  assert.match(openRecentWebDAV, /resolveRecentWebDAVOpen\(connection/)
  assert.match(openRecentWebDAV, /await ConnectSavedWebDAV\(connectionID\)/)
  assert.match(openRecentWebDAV, /fallbackError = t\('webdav\.savedConnectFailed'/)
  assert.match(openRecentWebDAV, /showWebDAVConnectionDialog\(endpoint, fallbackError\)/)
  assert.doesNotMatch(openRecentWebDAV, /ConnectWebDAV|connection\.(?:username|password)/)
  assert.match(webDAVBranch, /await openRecentWebDAV\(candidate\.id\)/)
  assert.match(webDAVBranch, /return/)
  assert.doesNotMatch(webDAVBranch, /ConnectWebDAV/)
})

test('recent persistence and native menu events store no WebDAV credentials', async () => {
  const recent = await readFile(new URL('../recent.go', import.meta.url), 'utf8')
  const appState = await readFile(new URL('../app_state.go', import.meta.url), 'utf8')
  const menu = await readFile(new URL('../menu.go', import.meta.url), 'utf8')
  const webDAVBridge = await readFile(new URL('../webdav_app.go', import.meta.url), 'utf8')
  const recentItem = recent.match(/type RecentItem struct \{[\s\S]*?\n\}/)?.[0] || ''
  const recentWebDAVData = recent.match(/type RecentWebDAVConnection struct \{[\s\S]*?\n\}/)?.[0] || ''
  const recentWebDAV = recent.match(/func \(a \*App\) OpenRecentWebDAV\([^)]*\)[\s\S]*?\n\}/)?.[0] || ''
  const menuAction = menu.match(/func \(a \*App\) recentMenuAction\([^)]*\)[\s\S]*?\n\}/)?.[0] || ''

  assert.match(recentItem, /Kind\s+string\s+`json:"kind"`/)
  assert.doesNotMatch(recentItem, /user(?:name)?|password|credential|authorization/i)
  assert.match(recentWebDAVData, /Endpoint\s+string\s+`json:"endpoint"`/)
  assert.match(recentWebDAVData, /Name\s+string\s+`json:"name"`/)
  assert.match(recentWebDAVData, /SavedConnectionID\s+string/)
  assert.match(recentWebDAVData, /HasSavedCredentials\s+bool/)
  assert.doesNotMatch(recentWebDAVData, /Username\s+string|Password\s+string|Authorization\s+string/i)
  assert.match(recentWebDAV, /recentItemByID\([^,]+,\s*"webdav"\)/)
  assert.match(recentWebDAV, /normalizeRecentWebDAVEndpoint\(item\.Path\)/)
  assert.match(recentWebDAV, /Endpoint:\s*endpoint/)
  assert.match(recentWebDAV, /Name:\s*name/)
  assert.doesNotMatch(recentWebDAV, /ConnectWebDAV|NewWebDAVClient|Username\s*:|Password\s*:|Authorization\s*:/i)
  assert.match(webDAVBridge, /recordRecentWebDAV\(client\.baseURL\.String\(\),\s*savedConnectionID\)/)
  assert.match(appState, /RecentItems\s+\[\]RecentItem\s+`json:"recentItems,omitempty"`/)
  assert.doesNotMatch(appState.match(/type settingsState struct \{[\s\S]*?\n\}/)?.[0] || '', /WebDAVConfig|Username\s+string|Password\s+string|Authorization\s+string/i)
  assert.match(menu, /for _, recentItem := range recentItems \{[\s\S]*recentMenuAction\(recentItem\)/)
  assert.match(menuAction, /RecentMenuEvent\{ID: item\.ID, Kind: item\.Kind, Name: item\.Name\}/)
  assert.doesNotMatch(menuAction, /Path:|Endpoint:|user(?:name)?|password|credential|authorization/i)
})

test('native menu edit actions stay inside the focused WebDAV field and preserve editor content', async () => {
  const app = await readFile(appURL, 'utf8')
  const runEditAction = app.match(/async function runEditAction\(action: string\) \{[\s\S]*?\n\}/)?.[0] || ''

  assert.match(app, /id="webdav-server-url"[\s\S]*?type="url"/)
  assert.match(app, /id="webdav-username"[\s\S]*?type="text"/)
  assert.match(app, /id="webdav-password"[\s\S]*?:type="showWebDAVPassword \? 'text' : 'password'"/)
  assert.match(app, /document\.addEventListener\('focusin', rememberFocusedElement, true\)/)
  assert.match(app, /function rememberFocusedElement\(event: FocusEvent\) \{[\s\S]*event\.target instanceof Element[\s\S]*lastFocusedElement = event\.target/)
  assert.match(runEditAction, /const activeElement = document\.activeElement/)
  assert.match(runEditAction, /activeElement === document\.body[\s\S]*activeElement === document\.documentElement/)
  assert.match(runEditAction, /const modalRoot = document\.querySelector<HTMLElement>\('\[aria-modal="true"\]'\)/)
  assert.match(runEditAction, /resolveTextEditControl\([\s\S]*activeElement[\s\S]*focusLeftDocument \? lastFocusedElement : null[\s\S]*modalRoot \? null : editor\.value/)
  assert.match(runEditAction, /if \(modalRoot && !modalRoot\.contains\(target\)\) return/)
  assert.match(runEditAction, /if \(sourceEditorTarget\) beginScroll\('editor'\)/)
  assert.match(runEditAction, /target\.setRangeText\(text, start, end, 'end'\)[\s\S]*dispatchTextEditInput\(target, 'insertFromPaste', inputData\)/)
  assert.match(runEditAction, /target instanceof HTMLInputElement && target\.type === 'password' \? null : text/)
  assert.doesNotMatch(runEditAction, /source\.value = target\.value/)
  assert.doesNotMatch(runEditAction, /const target = editor\.value/)
})

test('in-flight WebDAV operations keep their modal state visible', async () => {
  const app = await readFile(appURL, 'utf8')

  assert.match(app, /function dismissActiveDialog\(\) \{[\s\S]*activeDialog\.value === 'webdav' && webDAVDialogBusy\.value[\s\S]*activeDialog\.value = null/)
  assert.match(app, /function dismissWebDAVConflict\(\) \{[\s\S]*webDAVConflictBusy\.value[\s\S]*webDAVConflictOpen\.value = false/)
  assert.match(app, /v-if="webDAVConflictOpen"[^>]*@click\.self="dismissWebDAVConflict"/)
  assert.match(app, /v-if="activeDialog"[^>]*@click\.self="handleActiveDialogBackdrop"/)
  assert.match(app, /function handleActiveDialogBackdrop\(\) \{[\s\S]*webDAVDeleteCandidate\.value[\s\S]*cancelDeleteSavedWebDAVConnection\(\)[\s\S]*dismissActiveDialog\(\)/)
})

test('WebDAV backend error kinds are mapped to localized UI messages', async () => {
  const app = await readFile(appURL, 'utf8')
  const bridge = await readFile(new URL('../webdav_app.go', import.meta.url), 'utf8')

  assert.match(bridge, /\[INKMARK_WEBDAV:%s\]/)
  assert.ok(app.includes("message.match(/^\\[INKMARK_WEBDAV:([a-z_]+)\\]/)"))
  for (const [kind, key] of [
    ['authentication', 'webdav.authenticationFailed'],
    ['permission', 'webdav.permissionDenied'],
    ['timeout', 'webdav.timeout'],
    ['locked', 'webdav.locked'],
    ['network', 'webdav.networkError'],
    ['unsupported', 'webdav.unsupported'],
    ['credential_store', 'webdav.credentialStoreUnavailable'],
    ['local_storage', 'webdav.localConnectionStorageUnavailable'],
  ]) {
    assert.match(app, new RegExp(`${kind}: '${key}'`))
  }
})
