import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import {
  flattenWorkspaceTree,
  isActiveWorkspaceFile,
  loadedWorkspaceDirectoryKeys,
  localWorkspaceRelativePath,
  normalizeWorkspace,
  normalizeWorkspaceDirectory,
  normalizeWorkspaceEntries,
  retainExistingWorkspaceDirectories,
  sameWorkspaceFile,
  workspaceBackendDirectoryPath,
  workspaceDirectoryExists,
  workspaceDirectoryKey,
  workspaceParentDirectoryKey,
  workspaceTreeIndent,
} from '../frontend/src/workspace-tree.ts'

const directory = (name, path) => ({
  name,
  path,
  absolutePath: `/workspace/${path}`,
  kind: 'directory',
})
const markdown = (name, path) => ({
  name,
  path,
  absolutePath: `/workspace/${path}`,
  kind: 'markdown',
})

test('workspace entries retain only valid directories and Markdown files and sort directories first', () => {
  assert.deepEqual(
    normalizeWorkspaceEntries([
      markdown('z10.md', 'z10.md'),
      { name: 'ignored.txt', path: 'ignored.txt', kind: 'file' },
      directory('Guides', 'Guides'),
      markdown('z2.md', 'z2.md'),
      null,
    ]).map(({ name, kind }) => ({ name, kind })),
    [
      { name: 'Guides', kind: 'directory' },
      { name: 'z2.md', kind: 'markdown' },
      { name: 'z10.md', kind: 'markdown' },
    ],
  )
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
  assert.match(app, /const inferredLocalWorkspacePath = \([\s\S]*localWorkspaceRelativePath\(workspace\.value\.path, localDocumentPath\.value\)/)
  assert.match(app, /documentWorkspaceId\.value = document\.workspaceId[\s\S]*inferredLocalWorkspacePath/)
  assert.ok((app.match(/localWorkspaceRelativePath\(workspace\.value\.path, result\.path\)/g) || []).length >= 2)
  assert.match(app, /:current-workspace-id="documentWorkspaceId"/)
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
