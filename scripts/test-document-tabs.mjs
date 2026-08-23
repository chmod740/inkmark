import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import {
  createDocumentTab,
  documentTabsMatch,
  findMatchingDocumentTab,
  findWorkspaceDocumentTab,
  nextActiveTabID,
  rebaseDocumentTabs,
  tabCloseTargetIDs,
} from '../frontend/src/document-tabs.ts'

const workspaceState = {
  workspace: { id: 'local-1', provider: 'local', path: '/docs', name: 'docs', entries: [], truncated: false },
  children: { '': [] },
  expandedDirectories: new Set(),
  loadingDirectories: new Set(),
  truncatedDirectories: new Set(),
}

test('document tabs preserve independent editor and capability state', () => {
  const first = createDocumentTab('one', {
    name: 'a.md', path: '/docs/a.md', content: '# A', storageKind: 'local',
    workspaceId: 'local-1', workspacePath: 'a.md', localDocumentId: 'doc-a',
  }, workspaceState)
  const second = createDocumentTab('two', {
    name: 'b.md', path: '/docs/b.md', content: '# B', storageKind: 'local',
    workspaceId: 'local-1', workspacePath: 'b.md', localDocumentId: 'doc-b',
  }, workspaceState)
  first.source = '# changed'
  first.editorScrollTop = 420
  assert.equal(second.source, '# B')
  assert.equal(second.editorScrollTop, 0)
  assert.notEqual(first.localDocumentId, second.localDocumentId)
})

test('opening the same local, remote, or built-in document focuses the existing tab', () => {
  const local = createDocumentTab('local', { name: 'a.md', path: '/docs/a.md', content: '' })
  const remote = createDocumentTab('remote', {
    name: 'r.md', content: '', storageKind: 'webdav', workspaceId: 'dav', workspacePath: 'folder/r.md',
  })
  const welcome = createDocumentTab('welcome', { name: 'README.md', content: '', builtIn: 'welcome' })
  assert.ok(documentTabsMatch(local, { path: '/docs/a.md' }))
  assert.equal(findMatchingDocumentTab([local, remote, welcome], {
    storageKind: 'webdav', workspaceId: 'dav', workspacePath: 'folder/r.md',
  })?.id, 'remote')
  assert.equal(findMatchingDocumentTab([local, remote, welcome], { builtIn: 'welcome' })?.id, 'welcome')
  assert.equal(findMatchingDocumentTab([welcome], {
    name: 'README.md', path: '/docs/README.md', storageKind: 'local',
  }), null, 'a local file with a built-in filename must open in its own tab')
})

test('closing a tab selects the nearest surviving neighbour', () => {
  const tabs = ['a', 'b', 'c'].map((id) => createDocumentTab(id, { name: `${id}.md`, path: `/${id}.md` }))
  assert.equal(nextActiveTabID(tabs, 0), 'b')
  assert.equal(nextActiveTabID(tabs, 1), 'c')
  assert.equal(nextActiveTabID(tabs, 2), 'b')
  assert.equal(nextActiveTabID([tabs[0]], 0), '')
})

test('sidebar activation finds an existing workspace tab without allocating a duplicate capability', () => {
  const local = createDocumentTab('local', {
    name: 'a.md', path: '/docs/a.md', workspaceId: 'local-1', workspacePath: 'a.md', localDocumentId: 'doc-a',
  })
  const remote = createDocumentTab('remote', {
    name: 'a.md', storageKind: 'webdav', workspaceId: 'dav-1', workspacePath: 'a.md', remoteDocumentId: 'remote-a',
  })
  assert.equal(findWorkspaceDocumentTab([local, remote], 'local', 'local-1', 'a.md')?.id, 'local')
  assert.equal(findWorkspaceDocumentTab([local, remote], 'webdav', 'dav-1', 'a.md')?.id, 'remote')
  assert.equal(findWorkspaceDocumentTab([local, remote], 'local', 'local-2', 'a.md'), null)
})

test('browser-style tab close commands select only the requested side', () => {
  const tabs = ['a', 'b', 'c', 'd'].map((id) => createDocumentTab(id, { name: `${id}.md`, path: `/${id}.md` }))
  assert.deepEqual(tabCloseTargetIDs(tabs, 'c', 'self'), ['c'])
  assert.deepEqual(tabCloseTargetIDs(tabs, 'c', 'left'), ['a', 'b'])
  assert.deepEqual(tabCloseTargetIDs(tabs, 'c', 'right'), ['d'])
  assert.deepEqual(tabCloseTargetIDs(tabs, 'c', 'others'), ['a', 'b', 'd'])
  assert.deepEqual(tabCloseTargetIDs(tabs, 'missing', 'others'), [])
})

test('directory rename rebases every affected tab in that workspace only', () => {
  const child = createDocumentTab('child', {
    name: 'child.md', path: '/docs/old/child.md', workspaceId: 'local-1', workspacePath: 'old/child.md',
  })
  const sibling = createDocumentTab('sibling', {
    name: 'sibling.md', path: '/docs/sibling.md', workspaceId: 'local-1', workspacePath: 'sibling.md',
  })
  const other = createDocumentTab('other', {
    name: 'other.md', path: '/other/old/other.md', workspaceId: 'local-2', workspacePath: 'old/other.md',
  })
  const rebased = rebaseDocumentTabs([child, sibling, other], 'local-1', 'old', 'new')
  assert.equal(rebased[0].workspacePath, 'new/child.md')
  assert.equal(rebased[1].workspacePath, 'sibling.md')
  assert.equal(rebased[2].workspacePath, 'old/other.md')
})

test('application shell exposes accessible tabs, one compact toolbar, sidebar footer settings, and bottom status', async () => {
  const [app, styles] = await Promise.all([
    readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8'),
    readFile(new URL('../frontend/src/styles.css', import.meta.url), 'utf8'),
  ])
  assert.match(app, /class="app-sidebar-shell"[\s\S]*class="sidebar-brand"[\s\S]*class="sidebar-footer"/)
  assert.match(app, /class="document-tabs" role="tablist"/)
	assert.match(app, /ref="documentTabsElement" class="document-tabs" role="tablist"/)
  assert.match(app, /class="document-tab-main"[\s\S]*role="tab"[\s\S]*:aria-selected=[\s\S]*:tabindex=/)
  assert.match(app, /@keydown="handleTabKeydown\(\$event, index, tab\.id\)"/)
	assert.match(app, /@contextmenu="openTabContextMenu\(\$event, tab\.id\)"/)
	assert.match(app, /class="tab-context-menu"[\s\S]*runTabContextAction\('self'\)[\s\S]*runTabContextAction\('left'\)[\s\S]*runTabContextAction\('right'\)[\s\S]*runTabContextAction\('others'\)/)
  assert.match(app, /class="document-topbar"[\s\S]*class="command-bar" role="toolbar"/)
  assert.match(app, /class="status-bar"[\s\S]*status-document-state[\s\S]*connection-badge[\s\S]*renderState/)
  assert.doesNotMatch(app, /<header class="document-header"/)
  assert.match(styles, /\.app-shell \{[\s\S]*grid-template-columns: clamp\(220px, 19vw, 278px\) minmax\(0, 1fr\)/)
	assert.match(styles, /\.document-tabs \{[\s\S]*overflow-x: auto/)
	assert.match(styles, /\.document-tabs \{[\s\S]*width: 0;[\s\S]*flex: 1 1 0;/)
	assert.match(styles, /\.document-tab \{[\s\S]*min-width: 56px;[\s\S]*flex: 1 1 180px;[\s\S]*container: document-tab \/ inline-size;/)
	assert.match(styles, /@container document-tab \(max-width: 88px\)[\s\S]*\.document-tab-close \{ display: none; \}/)
	assert.match(styles, /\.command-bar \{[\s\S]*flex: 0 0 auto;/)
	assert.match(styles, /\.tab-context-menu \{[\s\S]*position: fixed;[\s\S]*z-index: 1500;/)
	assert.doesNotMatch(app, /class="document-tab"[\s\S]{0,240}:title="tabLocation\(tab\)"/)
	assert.match(styles, /\.sidebar-footer \{[\s\S]*border-top: 0;/)
	assert.match(app, /class="settings-button"[\s\S]*class="settings-button-icon"[\s\S]*<circle cx="12" cy="12" r="3"/)
	assert.doesNotMatch(app, /<span aria-hidden="true">⚙<\/span>/)
	assert.match(styles, /\.app-shell \.sidebar-footer \.settings-button \{[\s\S]*width: 100%;[\s\S]*border: 0;[\s\S]*background: transparent;/)
	assert.match(styles, /\.app-shell\.theme-autumn \.app-sidebar-shell \{[\s\S]*background-image: radial-gradient[\s\S]*background-size: 4px 4px;/)
	assert.match(app, /runtimePlatform === 'windows'[\s\S]*window-controls-\$\{windowControlStyle\}/)
	assert.match(app, /class="window-control minimise"[\s\S]*@click="WindowMinimise"/)
	assert.match(app, /class="window-control maximise"[\s\S]*@click="WindowToggleMaximise"/)
	assert.match(app, /class="window-control close"[\s\S]*@click="requestApplicationQuit"/)
	assert.match(app, /id="window-control-style-select"[\s\S]*handleWindowControlStyleChange/)
	assert.match(app, /<nav class="command-bar"[\s\S]*v-if="runtimePlatform === 'windows'"/)
	assert.doesNotMatch(app, /<div class="sidebar-brand"[\s\S]{0,240}class="window-controls"/)
	assert.match(app, /class="window-control minimise"[\s\S]*class="window-control maximise"[\s\S]*class="window-control close"/)
	assert.match(styles, /\.window-controls \{[\s\S]*height: 50px;[\s\S]*gap: 8px;[\s\S]*--wails-draggable: no-drag;/)
	assert.match(styles, /\.window-control \{[\s\S]*width: 14px;[\s\S]*height: 14px;[\s\S]*border-radius: 999px;/)
	assert.match(styles, /\.window-control\.close \{ background: #ff5f57; \}/)
	assert.match(styles, /\.window-control\.minimise \{ background: #febc2e; \}/)
	assert.match(styles, /\.window-control\.maximise \{ background: #28c840; \}/)
	assert.match(styles, /\.window-controls-system \{[\s\S]*gap: 0;[\s\S]*padding: 0;/)
	assert.match(styles, /\.window-controls-system \.window-control \{[\s\S]*width: 46px;[\s\S]*height: 50px;[\s\S]*border-radius: 0;/)
	assert.match(styles, /\.window-controls-system \.window-control\.close:hover \{[\s\S]*background: #c42b1c;/)
	assert.match(styles, /\.segmented button\.active \{ font-weight: 700; \}/)
	assert.match(styles, /\.theme-picker select,[\s\S]*background: transparent;/)
})

test('the overflowing tab strip uses a non-passive native wheel listener and turns vertical wheel motion into horizontal scrolling', async () => {
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  assert.match(app, /document\.addEventListener\('wheel', handleDocumentTabsWheel, \{ capture: true, passive: false \}\)/)
  assert.match(app, /function handleDocumentTabsWheel\(event: WheelEvent\)[\s\S]*documentTabsElement\.value[\s\S]*tabList\.contains\(target\)[\s\S]*scrollWidth <= tabList\.clientWidth \+ 1[\s\S]*DOM_DELTA_LINE[\s\S]*tabList\.scrollLeft \+= rawDelta \* deltaMultiplier[\s\S]*event\.preventDefault\(\)/)
})

test('tab lifecycle keeps capabilities until close and guards every dirty tab on quit', async () => {
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  assert.match(app, /async function releaseDocumentCapability\(tab:[\s\S]*CloseWorkspaceDocument[\s\S]*CloseWebDAVDocument/)
  assert.match(app, /interface TabActivationOptions[\s\S]*awaitPreview[\s\S]*renderPreview/)
  assert.match(app, /function interruptActivePreviewWork\([\s\S]*renderScheduler\.cancel\(\)[\s\S]*previewCommit\.invalidate\(\)[\s\S]*cancelPreviewWorkerRequests/)
  assert.match(app, /async function closeDocumentTab\(tabId:[\s\S]*closing\.source !== closing\.savedSource[\s\S]*activateTab\(tabId, true, \{ renderPreview: false \}\)[\s\S]*performDocumentTransition\('close'/)
  assert.match(app, /async function closeDocumentTab[\s\S]*renderPreview: !options\.deferPreview/)
  assert.match(app, /async function runTabContextAction[\s\S]*const deferPreview = targets\.length > 1[\s\S]*closeDocumentTab\(tabId, \{ deferPreview \}\)[\s\S]*activateTab\(preferredTabId, true, \{ awaitPreview: false \}\)[\s\S]*requestActivePreviewRender\('fallback'\)/)
  assert.match(app, /async function resolveAllDirtyTabsBeforeQuit\(\)[\s\S]*for \(const tabId of dirtyTabIds\)[\s\S]*requestUnsavedDecision\('quit'\)/)
	assert.match(app, /async function openDocumentTab\(document:[\s\S]*findMatchingDocumentTab[\s\S]*discardDuplicateDocument/)
	assert.match(app, /async function openWorkspaceDocument\(entry:[\s\S]*findWorkspaceDocumentTab[\s\S]*await activateTab\(existingTab\.id\)/)
	assert.match(app, /async function activateTab\([\s\S]*allowWhileBusy = false[\s\S]*busy\.value && !allowWhileBusy/)
	assert.match(app, /await activateTab\(matching\.id, true\)/)
  assert.match(app, /function preserveDocumentsAfterWorkspaceDelete[\s\S]*for \(const tab of tabs\.value\)/)
})

test('last-page persistence uses only verified native capabilities and explicit startup files retain priority', async () => {
  const [app, backend, settings] = await Promise.all([
    readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8'),
    readFile(new URL('../app.go', import.meta.url), 'utf8'),
    readFile(new URL('../app_state.go', import.meta.url), 'utf8'),
  ])
  assert.match(app, /RememberLastPage\(kind, tab\.workspaceId, documentCapabilityId, tab\.builtIn \|\| ''\)/)
  assert.match(backend, /func \(a \*App\) RememberLastPage\([\s\S]*capability\.documents\[strings\.TrimSpace\(localDocumentID\)\]/)
  assert.match(backend, /path := a\.initPath[\s\S]*if path != ""[\s\S]*openLocalDocumentWithWorkspace\(path, true\)[\s\S]*lastPage\.Kind/)
  assert.match(settings, /normalizeLastPageState[\s\S]*filepath\.IsAbs\(path\)[\s\S]*isMarkdownFilename\(path\)/)
  assert.doesNotMatch(settings, /LastPageState[\s\S]{0,300}(?:Content|Password|Token|ETag)/)
})
