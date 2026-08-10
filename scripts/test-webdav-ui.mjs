import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const appURL = new URL('../frontend/src/App.vue', import.meta.url)

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

  assert.match(app, /autocomplete="off"/)
  assert.match(app, /webDAVPassword\.value = ''/)
  assert.match(app, /onBeforeUnmount\(\(\) => \{[\s\S]*clearWebDAVConnectionForm\(\)/)
  assert.doesNotMatch(app, /localStorage\.setItem\([^\n]*(webdav|username|password|credential)/i)
  assert.doesNotMatch(app, /sessionStorage\.(setItem|getItem)\([^\n]*(webdav|username|password|credential)/i)
})

test('in-flight WebDAV operations keep their modal state visible', async () => {
  const app = await readFile(appURL, 'utf8')

  assert.match(app, /function dismissActiveDialog\(\) \{[\s\S]*activeDialog\.value === 'webdav' && webDAVConnecting\.value[\s\S]*activeDialog\.value = null/)
  assert.match(app, /function dismissWebDAVConflict\(\) \{[\s\S]*webDAVConflictBusy\.value[\s\S]*webDAVConflictOpen\.value = false/)
  assert.match(app, /v-if="webDAVConflictOpen"[^>]*@click\.self="dismissWebDAVConflict"/)
  assert.match(app, /v-if="activeDialog"[^>]*@click\.self="dismissActiveDialog"/)
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
  ]) {
    assert.match(app, new RegExp(`${kind}: '${key}'`))
  }
})
