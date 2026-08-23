import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import {
  flattenWorkspaceTree,
  isActiveWorkspaceFile,
  isWorkspaceBackedLocalDocument,
  loadedWorkspaceDirectoryKeys,
  localWorkspaceAbsolutePath,
  localWorkspaceRelativePath,
  normalizeWorkspace,
  normalizeWorkspaceDirectory,
  normalizeWorkspaceEntries,
  retainExistingWorkspaceDirectories,
  rebaseWorkspacePath,
  sameWorkspaceFile,
  workspaceBackendDirectoryPath,
  workspaceDirectoryExists,
  workspaceDirectoryKey,
  workspaceJoinPath,
  workspaceParentDirectoryKey,
  workspacePathIsWithin,
  workspaceTreeIndent,
} from '../frontend/src/workspace-tree.ts'

const directory = (name, path, revision = `directory-revision:${path}`) => ({
  name,
  path,
  absolutePath: `/workspace/${path}`,
  kind: 'directory',
  revision,
})
const markdown = (name, path, revision = `markdown-revision:${path}`) => ({
  name,
  path,
  absolutePath: `/workspace/${path}`,
  kind: 'markdown',
  revision,
})
const image = (name, path, revision = `image-revision:${path}`) => ({
  name,
  path,
  absolutePath: `/workspace/${path}`,
  kind: 'image',
  revision,
})

test('workspace entries retain directories, Markdown, and supported images in a stable kind order', () => {
  assert.deepEqual(
    normalizeWorkspaceEntries([
      markdown('z10.md', 'z10.md'),
      { name: 'ignored.txt', path: 'ignored.txt', kind: 'file' },
      image('z2.png', 'z2.png'),
      directory('Guides', 'Guides'),
      markdown('z2.md', 'z2.md'),
      image('z10.png', 'z10.png'),
      null,
    ]).map(({ name, kind }) => ({ name, kind })),
    [
      { name: 'Guides', kind: 'directory' },
      { name: 'z2.md', kind: 'markdown' },
      { name: 'z10.md', kind: 'markdown' },
      { name: 'z2.png', kind: 'image' },
      { name: 'z10.png', kind: 'image' },
    ],
  )
})

test('workspace revisions are retained byte-for-byte and never reconstructed by the client', () => {
  const opaqueRevision = 'opaque::"etag with spaces"::A/B+C=='
  assert.equal(
    normalizeWorkspaceEntries([markdown('README.md', 'README.md', opaqueRevision)])[0].revision,
    opaqueRevision,
  )
  assert.equal(normalizeWorkspaceEntries([{
    name: 'legacy.md',
    path: 'legacy.md',
    absolutePath: '/workspace/legacy.md',
    kind: 'markdown',
  }])[0].revision, '')
})

test('workspace normalization rejects incomplete roots and keeps truncation state', () => {
  assert.equal(normalizeWorkspace({ path: '/tmp', name: 'tmp' }), null)
  assert.deepEqual(normalizeWorkspace({
    id: 'workspace-id',
    path: '/tmp/docs',
    name: 'docs',
    entries: [markdown('README.md', 'README.md')],
    truncated: true,
  }), {
    id: 'workspace-id',
    provider: 'local',
    path: '/tmp/docs',
    name: 'docs',
    entries: [markdown('README.md', 'README.md')],
    truncated: true,
  })
  assert.deepEqual(normalizeWorkspace({
    id: 'webdav-id',
    provider: 'webdav',
    path: 'https://example.test/dav/',
    name: 'Remote Notes',
    entries: [],
  }), {
    id: 'webdav-id',
    provider: 'webdav',
    path: 'https://example.test/dav/',
    name: 'Remote Notes',
    entries: [],
    truncated: false,
  })
  assert.equal(normalizeWorkspace({
    id: 'unknown-id',
    provider: 'ftp',
    path: '/tmp/docs',
    name: 'docs',
  }), null)
  assert.deepEqual(normalizeWorkspaceDirectory([markdown('README.md', 'README.md')]), {
    entries: [markdown('README.md', 'README.md')],
    truncated: false,
  })
})

test('flat tree includes descendants only after their parent is expanded and loaded', () => {
  const children = {
    '': [directory('docs', 'docs'), markdown('README.md', 'README.md')],
    docs: [directory('api', 'docs/api'), markdown('guide.md', 'docs/guide.md')],
    'docs/api': [markdown('index.md', 'docs/api/index.md')],
  }

  assert.deepEqual(
    flattenWorkspaceTree(children, new Set()).map(({ entry, depth }) => [entry.path, depth]),
    [['docs', 0], ['README.md', 0]],
  )
  assert.deepEqual(
    flattenWorkspaceTree(children, new Set(['docs'])).map(({ entry, depth }) => [entry.path, depth]),
    [['docs', 0], ['docs/api', 1], ['docs/guide.md', 1], ['README.md', 0]],
  )
  assert.deepEqual(
    flattenWorkspaceTree(children, new Set(['docs', 'docs/api'])).map(({ entry, depth }) => [entry.path, depth]),
    [['docs', 0], ['docs/api', 1], ['docs/api/index.md', 2], ['docs/guide.md', 1], ['README.md', 0]],
  )
})

test('flat tree exposes lazy loading state and stops cyclic provider responses', () => {
  const cycle = directory('loop', 'loop')
  const rows = flattenWorkspaceTree(
    { '': [cycle], loop: [cycle] },
    new Set(['loop']),
    new Set(['loop']),
  )
  assert.equal(rows.length, 2)
  assert.equal(rows[0].loaded, true)
  assert.equal(rows[0].loading, true)
})

test('workspace path helpers are separator-safe and case-aware on Windows', () => {
  assert.equal(workspaceDirectoryKey('.\\guides\\api\\'), 'guides/api')
  assert.equal(sameWorkspaceFile('C:\\Docs\\README.md', 'c:/docs/readme.md'), true)
  assert.equal(sameWorkspaceFile('/Docs/README.md', '/docs/readme.md'), false)
  assert.equal(sameWorkspaceFile('', ''), false)
  assert.equal(localWorkspaceRelativePath('/Users/test/Notes', '/Users/test/Notes/guides/readme.md'), 'guides/readme.md')
  assert.equal(localWorkspaceRelativePath('/Users/test/Notes', '/Users/test/Other/readme.md'), '')
  assert.equal(localWorkspaceRelativePath('C:\\Users\\Test\\Notes', 'c:\\users\\test\\notes\\Guide.md'), 'Guide.md')
  assert.equal(localWorkspaceRelativePath('/', '/README.md'), 'README.md')
  assert.equal(localWorkspaceAbsolutePath('/Users/test/Notes', 'guides/readme.md'), '/Users/test/Notes/guides/readme.md')
  assert.equal(localWorkspaceAbsolutePath('C:\\Users\\Test\\Notes', 'guides/readme.md'), 'C:\\Users\\Test\\Notes\\guides\\readme.md')
})

test('workspace mutation paths join, contain, and rebase descendants without prefix confusion', () => {
  assert.equal(workspaceJoinPath('', 'README.md'), 'README.md')
  assert.equal(workspaceJoinPath('guides\\api', 'README.md'), 'guides/api/README.md')
  assert.equal(workspacePathIsWithin('docs/current.md', 'docs'), true)
  assert.equal(workspacePathIsWithin('docs-old/current.md', 'docs'), false)
  assert.equal(rebaseWorkspacePath('docs/api/current.md', 'docs', 'handbook'), 'handbook/api/current.md')
  assert.equal(rebaseWorkspacePath('other/current.md', 'docs', 'handbook'), 'other/current.md')
})

test('active workspace files require the same provider, workspace, and relative path', () => {
  assert.equal(isActiveWorkspaceFile('webdav', 'remote-1', 'guides\\README.md', 'webdav', 'remote-1', './guides/README.md'), true)
  assert.equal(isActiveWorkspaceFile('local', 'local-1', 'guides/README.md', 'webdav', 'local-1', 'guides/README.md'), false)
  assert.equal(isActiveWorkspaceFile(undefined, 'local-1', 'README.md', 'local', 'local-1', 'README.md'), true)
  assert.equal(isActiveWorkspaceFile('webdav', 'remote-1', 'README.md', 'webdav', 'remote-2', 'README.md'), false)
  assert.equal(isActiveWorkspaceFile('webdav', 'remote-1', '', 'webdav', 'remote-1', ''), false)
  assert.equal(isActiveWorkspaceFile('webdav', 'remote-1', 'README.md', 'webdav', 'remote-1', 'readme.md'), false)
})

test('app restores local sidebar identity for picker, recent, save, and save-as paths', async () => {
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  assert.ok((app.match(/localWorkspaceRelativePath\(activeLocalWorkspace\.path, result\.path\)/g) || []).length >= 2)
  assert.match(app, /bindLocalDocumentToActiveWorkspace\([\s\S]*localWorkspaceRelativePath\(activeWorkspace\.path, document\.path\)/)
  assert.match(app, /function workspaceStateForDocument\(document:[\s\S]*normalizeWorkspace\(document\.workspace\)/)
  assert.match(app, /const tab = createDocumentTab\(newTabID\(\), document, workspaceStateForDocument\(document\)/)
  assert.match(app, /:current-workspace-id="documentWorkspaceId"/)
})

test('workspace-backed local saves stay inside the active root capability', async () => {
  const localWorkspace = {
    id: 'local-1',
    provider: 'local',
    path: '/notes',
    name: 'notes',
    entries: [],
    truncated: false,
  }
  assert.equal(isWorkspaceBackedLocalDocument(localWorkspace, 'local', 'local-1', 'README.md'), true)
  assert.equal(isWorkspaceBackedLocalDocument(localWorkspace, 'local', 'other', 'README.md'), false)
  assert.equal(isWorkspaceBackedLocalDocument(localWorkspace, 'webdav', 'local-1', 'README.md'), false)
  assert.equal(isWorkspaceBackedLocalDocument({ ...localWorkspace, provider: 'webdav' }, 'local', 'local-1', 'README.md'), false)
  assert.equal(isWorkspaceBackedLocalDocument(localWorkspace, 'local', 'local-1', ''), false)

  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  const saveFunction = app.slice(
    app.indexOf('async function saveDocument()'),
    app.indexOf('async function overwriteConflictingWebDAVDocument()'),
  )
  assert.match(saveFunction, /isWorkspaceBackedLocalDocument\(\s*localWorkspaceCandidate,\s*documentStorageKind\.value,\s*documentWorkspaceId\.value,\s*currentWorkspacePath\.value/s)
  assert.match(saveFunction, /SaveWorkspaceMarkdownFile\(\s*activeLocalWorkspace\.id,\s*localDocumentId\.value,\s*source\.value/s)
  assert.match(saveFunction, /if \(activeLocalWorkspace\) \{\s*if \(!localDocumentId\.value\)[\s\S]*return false[\s\S]*SaveWorkspaceMarkdownFile/s)
  assert.ok(
    saveFunction.indexOf('SaveWorkspaceMarkdownFile(') < saveFunction.indexOf('SaveFile(previousLocalPath'),
    'capability-backed documents must not fall through to the absolute-path save API',
  )
  const capabilitySave = saveFunction.slice(
    saveFunction.indexOf('if (activeLocalWorkspace)'),
    saveFunction.indexOf('const previousLocalPath'),
  )
  assert.match(capabilitySave, /catch \(error\)[\s\S]*localizedWorkspaceErrorMessage\(error\)/)
  assert.doesNotMatch(capabilitySave, /showError\(error\)/)
  const pickerSave = saveFunction.slice(
    saveFunction.indexOf('const previousLocalPath'),
    saveFunction.indexOf('return false', saveFunction.indexOf('const previousLocalPath')),
  )
  assert.match(pickerSave, /OpenWorkspaceFile\(activeLocalWorkspace\.id, activeWorkspacePath\)/)
  assert.match(pickerSave, /adopted\.content !== expectedSource/)
  assert.match(pickerSave, /localDocumentId\.value = adopted\.localDocumentId/)
  assert.doesNotMatch(pickerSave, /previousWorkspacePath|previousDocumentWorkspaceId/)

  const tabState = await readFile(new URL('../frontend/src/document-tabs.ts', import.meta.url), 'utf8')
  assert.match(tabState, /localDocumentId: storageKind === 'local' \? document\.localDocumentId \|\| '' : ''/)
  assert.match(app, /async function releaseDocumentCapability\([\s\S]*CloseWorkspaceDocument\(tab\.workspaceId, tab\.localDocumentId\)/)

  const saveAsFunction = app.slice(
    app.indexOf('async function saveDocumentAs()'),
    app.indexOf('\nasync function exportDocument('),
  )
  assert.match(saveAsFunction, /OpenWorkspaceFile\(activeLocalWorkspace\.id, activeWorkspacePath\)/)
  assert.match(saveAsFunction, /adopted\.content !== expectedSource/)
  assert.match(saveAsFunction, /localDocumentId\.value = ''/)
})

test('refresh helpers order loaded folders parent-first and map the root for the backend', () => {
  const children = {
    'docs/api': [markdown('index.md', 'docs/api/index.md')],
    '': [directory('docs', 'docs')],
    docs: [directory('api', 'docs/api')],
    'docs/api/v2': [],
  }
  assert.deepEqual(loadedWorkspaceDirectoryKeys(children), ['docs', 'docs/api', 'docs/api/v2'])
  assert.equal(workspaceBackendDirectoryPath(''), '.')
  assert.equal(workspaceBackendDirectoryPath('./docs'), 'docs')
  assert.equal(workspaceParentDirectoryKey('docs/api/v2'), 'docs/api')
  assert.equal(workspaceParentDirectoryKey('docs'), '')
})

test('refresh reconciliation drops expansion for folders removed outside the app', () => {
  const refreshedChildren = {
    '': [directory('docs', 'docs'), directory('notes', 'notes')],
    docs: [markdown('guide.md', 'docs/guide.md')],
  }
  assert.equal(workspaceDirectoryExists(refreshedChildren, 'docs'), true)
  assert.equal(workspaceDirectoryExists(refreshedChildren, 'docs/api'), false)
  assert.deepEqual(
    Array.from(retainExistingWorkspaceDirectories(
      new Set(['docs', 'docs/api', 'deleted']),
      refreshedChildren,
    )),
    ['docs'],
  )
})

test('deep tree indentation is bounded without losing shallow hierarchy', () => {
  assert.equal(workspaceTreeIndent(0), 10)
  assert.equal(workspaceTreeIndent(3), 52)
  assert.equal(workspaceTreeIndent(50), 94)
  assert.equal(workspaceTreeIndent(Number.NaN), 10)
})

test('sidebar file actions are keyboard accessible, modal-isolated, and include image preview', async () => {
  const sidebar = await readFile(new URL('../frontend/src/DirectorySidebar.vue', import.meta.url), 'utf8')
  assert.match(sidebar, /event\.key === 'ContextMenu'.*event\.shiftKey.*event\.key === 'F10'/s)
  assert.match(sidebar, /role="menu"/)
  assert.ok((sidebar.match(/role="menuitem"/g) || []).length >= 4)
  assert.match(sidebar, /if \(props\.disabled \|\| props\.modalOpen\)/)
  assert.match(sidebar, /watch\(\(\) => props\.disabled \|\| props\.modalOpen/)
  assert.match(sidebar, /entry\.kind === 'image'.*emit\('preview'/s)
  assert.match(sidebar, /contextMenuTrigger\.value.*trigger\.focus/s)
  assert.match(sidebar, /event\.preventDefault\(\).*event\.stopPropagation\(\)/s)

  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  assert.match(app, /:modal-open="Boolean\(activeDialog \|\| unsavedTransition \|\| webDAVConflictOpen\)"/)
  assert.match(app, /ReadWebDAVWorkspaceImage\(activeWorkspace\.id, entry\.path\)/)
  assert.match(app, /ReadWorkspaceImage\(activeWorkspace\.id, entry\.path\)/)
  assert.match(app, /const workspacePreviewImages = new PreviewImageResourceSet\(\)/)
  assert.match(app, /workspacePreviewImages\.add\([\s\S]*prepared\.previewSource/)
  assert.ok((app.match(/workspacePreviewImages\.release\(\)/g) || []).length >= 3)
  const imagePreviewHandler = app.slice(
    app.indexOf('async function showWorkspaceImagePreview('),
    app.indexOf('function normalizeWorkspaceRequestedName'),
  )
  assert.doesNotMatch(imagePreviewHandler, /workspacePreviewSource\.value = imageDataURI/)
  assert.match(app, /role="alertdialog"|:role="activeDialog === 'workspace'.*'alertdialog'/s)
  assert.match(app, /aria-modal="true"/)
  assert.match(app, /workspaceDialogView === 'delete'.*class="button secondary"/s)
  assert.doesNotMatch(app, /class="button danger"\s+data-dialog-initial/)
})

test('sidebar rows use fixed icon columns and font-independent disclosure chevrons', async () => {
  const [sidebar, styles] = await Promise.all([
    readFile(new URL('../frontend/src/DirectorySidebar.vue', import.meta.url), 'utf8'),
    readFile(new URL('../frontend/src/styles.css', import.meta.url), 'utf8'),
  ])
  assert.match(sidebar, /class="workspace-disclosure"[\s\S]*'is-expanded':[\s\S]*'is-collapsed'/)
  assert.doesNotMatch(sidebar, /\{\{[\s\S]*row\.expanded[\s\S]*[⌄›][\s\S]*\}\}/)
  assert.match(styles, /\.workspace-tree-row \{[\s\S]*?display: grid;[\s\S]*?grid-template-columns: 14px 14px minmax\(0, 1fr\);/)
  assert.match(styles, /\.workspace-disclosure \{[\s\S]*?width: 14px;[\s\S]*?height: 14px;[\s\S]*?place-items: center;/)
  assert.match(styles, /\.workspace-disclosure\.is-collapsed::before[\s\S]*?\.workspace-disclosure\.is-expanded::before[\s\S]*?border-right: 1\.5px solid currentColor;[\s\S]*?border-bottom: 1\.5px solid currentColor;/)
  assert.match(styles, /\.workspace-disclosure\.is-collapsed::before \{ transform: rotate\(-45deg\); \}/)
  assert.match(styles, /\.workspace-disclosure\.is-expanded::before \{ transform: rotate\(45deg\); \}/)
	assert.match(styles, /\.workspace-tree-row\.is-active \{[\s\S]*background: rgba\(100, 39, 45, \.075\);[\s\S]*box-shadow: inset 3px 0 #64272d;[\s\S]*font-weight: 700;/)
	assert.doesNotMatch(styles, /\.workspace-tree-row\.is-active \{[^}]*linear-gradient/)
	assert.match(styles, /\.workspace-entry-icon\.markdown::before \{[\s\S]*box-shadow: 0 4px currentColor, 0 8px currentColor;/)
	assert.match(styles, /\.app-shell\.theme-autumn \.workspace-tree-row\.is-active \{[\s\S]*background-color: rgba\(100, 39, 45, \.075\);[\s\S]*background-image: none;/)
	assert.match(styles, /\.app-shell\.theme-autumn \.workspace-tree-row\.is-active \{[\s\S]*box-shadow: inset 3px 0 var\(--theme-accent\);/)
	assert.match(sidebar, /class="workspace-header-actions"[\s\S]*class="workspace-header-button workspace-refresh-button"[\s\S]*<svg viewBox="0 0 20 20"[\s\S]*class="workspace-header-button workspace-close-button"[\s\S]*<svg viewBox="0 0 20 20"/)
	assert.match(styles, /\.workspace-header-actions \{[^}]*align-items: center;[^}]*gap: 2px;/)
	assert.match(styles, /\.workspace-header-button \{[\s\S]*width: 26px;[\s\S]*height: 26px;/)
	assert.match(styles, /\.workspace-header-button svg \{[^}]*width: 16px;[^}]*height: 16px;/)
})

test('current document identity is rebased on rename and detached before any possibly partial delete', async () => {
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  assert.match(app, /for \(const tab of tabs\.value\)[\s\S]*rebaseWorkspacePath\(tab\.workspacePath, sourcePath, destinationPath\)/)
  assert.match(app, /tab\.localPath = localWorkspaceAbsolutePath\(activeWorkspace\.path, nextPath\)/)
  assert.match(app, /RenameWorkspaceEntry\(\s*activeWorkspace\.id,\s*entry\.path,\s*destinationPath,\s*entry\.kind,\s*entry\.revision/s)
  assert.match(app, /DeleteWorkspaceEntry\(activeWorkspace\.id, entry\.path, recursive, entry\.revision\)/)
  assert.match(app, /renameWorkspaceEntry\(activeWorkspace, entry, destinationPath\)/)

  const deleteFunction = app.slice(
    app.indexOf('async function confirmDeleteWorkspaceEntry()'),
    app.indexOf('async function openRecentItem'),
  )
  const detachIndex = deleteFunction.indexOf('preserveDocumentsAfterWorkspaceDelete(activeWorkspace, entry.path)')
  const deleteIndex = deleteFunction.indexOf('await deleteWorkspaceEntry(activeWorkspace, entry)')
  assert.ok(detachIndex >= 0 && deleteIndex >= 0 && detachIndex < deleteIndex)
  assert.match(deleteFunction, /workspaceDeleteBufferPreserved\.value = true/)
  assert.match(app, /tab\.savedSource = `\$\{tab\.source\}\\u0000`/)
})

test('closing or replacing a local workspace detaches its editor buffer before the root capability closes', async () => {
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  const detachFunction = app.slice(
    app.indexOf('function detachWorkspaceTabs('),
    app.indexOf('\nfunction setWorkspace('),
  )
  assert.match(detachFunction, /for \(const tab of tabs\.value\)/)
  assert.match(detachFunction, /tab\.workspaceState\?\.workspace\.id === workspaceId/)
  assert.match(detachFunction, /tab\.savedSource = `\$\{tab\.source\}\\u0000`/)
  assert.match(detachFunction, /tab\.localDocumentId = ''/)
  assert.match(detachFunction, /tab\.remoteDocumentId = ''/)
  const closeWorkspaceFunction = app.slice(
    app.indexOf('async function closeWorkspace('),
    app.indexOf('\nasync function readActiveWorkspaceDirectory('),
  )
  assert.ok(
    closeWorkspaceFunction.indexOf('detachWorkspaceTabs(activeWorkspace.id)')
      < closeWorkspaceFunction.indexOf('CloseWorkspace(activeWorkspace.id)'),
    'workspace close must detach the buffer before closing the active root',
  )
})

test('WebDAV rename and delete are prepared before confirmation and every exit releases the opaque mutation', async () => {
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  const beginFunction = app.slice(
    app.indexOf('function beginWebDAVWorkspaceMutation('),
    app.indexOf('async function showWorkspaceRename('),
  )
  assert.match(beginFunction, /BeginWebDAVMutation\(\s*activeWorkspace\.id,\s*entry\.path,\s*entry\.kind,\s*entry\.revision,\s*operation/s)
  assert.match(beginFunction, /mutationId.*lockedEntry.*expiresAt/s)
  assert.match(beginFunction, /if \(mutationId && !installed\)[\s\S]*CancelWebDAVMutation\(activeWorkspace\.id, mutationId\)/s)

  for (const functionName of ['showWorkspaceRename', 'showWorkspaceDelete']) {
    const start = app.indexOf(`async function ${functionName}(`)
    const end = app.indexOf('\nasync function ', start + 1)
    const handler = app.slice(start, end)
    const beginIndex = handler.indexOf('await beginWebDAVWorkspaceMutation(')
    const dialogIndex = handler.indexOf('populateWorkspaceMutationDialog(')
    assert.ok(beginIndex >= 0 && dialogIndex > beginIndex, `${functionName} must lock before opening its dialog`)
  }

  assert.match(app, /CommitWebDAVRename\(\s*activeWorkspace\.id,\s*mutation\.mutationId,\s*destinationPath/s)
  assert.match(app, /CommitWebDAVDelete\(activeWorkspace\.id, mutation\.mutationId, recursive\)/)
  const cancelFunction = app.slice(
    app.indexOf('function cancelActiveWorkspaceMutation('),
    app.indexOf('function clearWorkspaceDialog('),
  )
  assert.match(cancelFunction, /CancelWebDAVMutation\(mutation\.workspaceId, mutation\.mutationId\)/)
  assert.match(cancelFunction, /if \(pendingWorkspaceMutationCancel\) return pendingWorkspaceMutationCancel/)
  assert.match(cancelFunction, /pendingWorkspaceMutationCancel = task/)
  assert.match(app, /previousDialog === 'workspace'.*cancelActiveWorkspaceMutation\(\)/s)
  assert.match(app, /async function handleCloseRequest\(\)[\s\S]*await cancelActiveWorkspaceMutation/)
  assert.match(app, /onBeforeUnmount\(\(\) => \{[\s\S]*cancelActiveWorkspaceMutation/)
  assert.match(app, /workspaceMutationLockLabel/)
  assert.match(app, /workspaceMutationNeedsRestart.*workspace\.webDAVMutationRetry/s)
})

test('workspace file operation UI includes dark and narrow-window treatments', async () => {
  const styles = await readFile(new URL('../frontend/src/styles.css', import.meta.url), 'utf8')
  assert.match(styles, /\[data-color-scheme="dark"\] \.workspace-context-menu/)
  assert.match(styles, /\[data-color-scheme="dark"\] \.workspace-image-preview img/)
  assert.match(styles, /@media \(max-width: 720px\)[\s\S]*\.workspace-dialog/)
  const contextMenuZ = Number(styles.match(/\.workspace-context-menu \{[\s\S]*?z-index: (\d+)/)?.[1])
  const modalZ = Number(styles.match(/\.modal-backdrop \{[\s\S]*?z-index: (\d+)/)?.[1])
  assert.ok(contextMenuZ < modalZ, 'modal backdrops must always cover any stale context menu frame')
})
