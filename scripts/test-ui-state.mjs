import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import { guardUnsavedChanges, runGuardedDocumentTransition } from '../frontend/src/document-guard.ts'
import {
  createLineNumberModel,
  defaultEditorPreferences,
  editorPreferencesStorageKey,
  normalizeEditorPreferences,
  parseEditorPreferences,
  parseSourceHeadings,
  readEditorPreferences,
  sourceLineFromScroll,
  stickyHeadingTrail,
  updateEditorPreference,
  writeEditorPreferences,
} from '../frontend/src/editor-features.ts'
import {
  normalizePreviewFirst,
  previewFirstStorageKey,
  resolveDocumentHeaderState,
  togglePreviewFirst,
} from '../frontend/src/ui-state.ts'

test('unsaved document guard covers save, discard, cancel, and clean transitions', async () => {
  let decisions = 0
  let saves = 0
  const run = (dirty, decision, saveResult = true) => guardUnsavedChanges({
    dirty,
    requestDecision: async () => { decisions += 1; return decision },
    save: async () => { saves += 1; return saveResult },
  })

  assert.equal(await run(false, 'cancel'), true)
  assert.equal(decisions, 0)
  assert.equal(await run(true, 'discard'), true)
  assert.equal(saves, 0)
  assert.equal(await run(true, 'cancel'), false)
  assert.equal(await run(true, 'save', false), false)
  assert.equal(await run(true, 'save', true), true)
  assert.equal(decisions, 4)
  assert.equal(saves, 2)
})

test('dirty built-in document only transitions to a new page after an approved branch', async () => {
  const run = async (decision, saveResult = true) => {
    let page = 'built-in'
    let transitions = 0
    const completed = await runGuardedDocumentTransition({
      dirty: true,
      requestDecision: async () => decision,
      save: async () => saveResult,
      transition: async () => {
        transitions += 1
        page = 'new'
      },
    })
    return { completed, page, transitions }
  }

  assert.deepEqual(await run('save', true), { completed: true, page: 'new', transitions: 1 })
  assert.deepEqual(await run('save', false), { completed: false, page: 'built-in', transitions: 0 })
  assert.deepEqual(await run('discard'), { completed: true, page: 'new', transitions: 1 })
  assert.deepEqual(await run('cancel'), { completed: false, page: 'built-in', transitions: 0 })
})

test('document header states never describe a pathless document as saved', () => {
  assert.deepEqual(resolveDocumentHeaderState(false, '', 'welcome'), {
    status: 'built-in',
    location: 'welcome',
  })
  assert.deepEqual(resolveDocumentHeaderState(false, '', 'render-test'), {
    status: 'built-in',
    location: 'render-test',
  })
  assert.deepEqual(resolveDocumentHeaderState(false, '', null), {
    status: 'unsaved',
    location: 'unsaved',
  })
  assert.deepEqual(resolveDocumentHeaderState(true, '', null), {
    status: 'modified',
    location: 'unsaved',
  })
  assert.deepEqual(resolveDocumentHeaderState(false, '/tmp/document.md', null), {
    status: 'saved',
    location: 'path',
  })
  assert.deepEqual(resolveDocumentHeaderState(true, '/tmp/document.md', null), {
    status: 'modified',
    location: 'path',
  })
})

test('preview-first preference is strict, persistent, and reversible', () => {
  assert.equal(previewFirstStorageKey, 'inkmark-preview-first')
  assert.equal(normalizePreviewFirst(null), false)
  assert.equal(normalizePreviewFirst('false'), false)
  assert.equal(normalizePreviewFirst('invalid'), false)
  assert.equal(normalizePreviewFirst('true'), true)
  assert.equal(normalizePreviewFirst(true), true)
  assert.equal(togglePreviewFirst(false), true)
  assert.equal(togglePreviewFirst(togglePreviewFirst(false)), false)
})

test('editor display preferences are strict, local, and independently switchable', () => {
  assert.equal(editorPreferencesStorageKey, 'inkmark-editor-preferences-v1')
  assert.deepEqual(parseEditorPreferences(null), defaultEditorPreferences)
  assert.deepEqual(parseEditorPreferences('{"version":1,"lineNumbers":false,"stickyHeadings":true}'), {
    version: 1,
    lineNumbers: false,
    stickyHeadings: true,
  })
  assert.deepEqual(normalizeEditorPreferences({
    version: 1,
    lineNumbers: true,
    stickyHeadings: true,
    arbitraryCSS: 'url(https://example.test)',
  }), defaultEditorPreferences)
  assert.deepEqual(updateEditorPreference(defaultEditorPreferences, 'lineNumbers', false), {
    version: 1,
    lineNumbers: false,
    stickyHeadings: true,
  })

  const values = new Map()
  const storage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  }
  const next = updateEditorPreference(defaultEditorPreferences, 'stickyHeadings', false)
  writeEditorPreferences(storage, next)
  assert.deepEqual(readEditorPreferences(storage), next)
})

test('line numbers and sticky Markdown heading trails stay bounded and deterministic', () => {
  assert.deepEqual(createLineNumberModel('alpha\nbeta\n'), {
    count: 3,
    digits: 1,
    text: '1\n2\n3',
    available: true,
  })
  assert.deepEqual(createLineNumberModel('1\n2\n3', 2), {
    count: 3,
    digits: 1,
    text: '',
    available: false,
  })

  const source = [
    '# Root',
    'text',
    '## Section',
    '```md',
    '# Not a heading',
    '```',
    '### Detail',
    'body',
    '## Sibling',
  ].join('\n')
  const model = parseSourceHeadings(source)
  assert.equal(model.available, true)
  assert.deepEqual(model.headings.map(({ line, level, raw, parent }) => ({ line, level, raw, parent })), [
    { line: 0, level: 1, raw: '# Root', parent: -1 },
    { line: 2, level: 2, raw: '## Section', parent: 0 },
    { line: 6, level: 3, raw: '### Detail', parent: 1 },
    { line: 8, level: 2, raw: '## Sibling', parent: 0 },
  ])
  assert.deepEqual(stickyHeadingTrail(model.headings, 7).map(({ raw }) => raw), ['# Root', '## Section', '### Detail'])
  assert.deepEqual(stickyHeadingTrail(model.headings, 8).map(({ raw }) => raw), ['# Root', '## Sibling'])
  assert.equal(sourceLineFromScroll(0, 24, 24), 0)
  assert.equal(sourceLineFromScroll(72, 24, 24), 2)
})

test('app wires pane swapping, built-in navigation, and collision-safe header CSS', async () => {
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  const styles = await readFile(new URL('../frontend/src/styles.css', import.meta.url), 'utf8')

  assert.match(app, /RenderingTestDocument as LoadRenderingTestDocument/)
  assert.match(app, /action === 'show-render-test'/)
  assert.match(app, /href === '#inkmark-render-test'/)
  assert.match(app, /href === '#inkmark-welcome'/)
  assert.match(app, /class="swap-panes-button"/)
  assert.match(app, /'preview-first': previewFirst/)
  assert.match(app, /runGuardedDocumentTransition\(/)
  assert.match(app, /pendingSystemDocuments\.push\(document\)[\s\S]*await drainSystemDocuments\(\)/)
  assert.match(app, /EventsOn\('inkmark:close-request',[\s\S]*handleCloseRequest\(\)/)
  assert.match(app, /handleCloseRequest[\s\S]*ConfirmQuit\(\)[\s\S]*CancelQuitRequest\(\)/)
  assert.match(app, /answerUnsavedPrompt\('save'\)/)
  assert.match(app, /answerUnsavedPrompt\('discard'\)/)
  assert.match(app, /answerUnsavedPrompt\('cancel'\)/)
  assert.doesNotMatch(app, /window\.confirm/)
  assert.match(app, /@beforeinput="handleEditorBeforeInput"[\s\S]*@input="handleEditorInput"[\s\S]*@keydown="handleEditorKeydown"/)
  assert.match(app, /class="source-line-numbers"/)
  assert.match(app, /class="source-sticky-headings"[\s\S]*scrollToSourceHeading/)
  assert.match(app, /handleEditorPreferenceChange\('lineNumbers'/)
  assert.match(app, /handleEditorPreferenceChange\('stickyHeadings'/)
  assert.match(app, /wrap="off"/)
  assert.match(app, /function runFormat\(action: string\) \{[\s\S]*?beginScroll\('editor'\)[\s\S]*?replaceEditorRange\(/)
  assert.match(app, /async function runEditAction\(action: string\) \{[\s\S]*?beginScroll\('editor'\)[\s\S]*?applyEditorHistory/)
  assert.match(app, /target\.replaceChildren[\s\S]*refreshScrollAnchors\(\)[\s\S]*reconcileActiveScroll\(\)/)
  assert.match(app, /function enhancePreview[\s\S]*await Promise\.all\(\[[\s\S]*renderDiagrams\(root[\s\S]*preparePreviewImages\(root/)
  assert.match(styles, /\.preview-first \.preview-panel \{ order: -1; \}/)
  assert.match(styles, /\.document-title-row \{[^}]*width: fit-content;[^}]*max-width: 100%;/s)
  assert.match(styles, /\.document-identity h1 \{[^}]*flex: 0 1 auto;[^}]*min-width: 0;/s)
  assert.match(styles, /\.document-state \{[^}]*flex: 0 0 auto;[^}]*white-space: nowrap;/s)
  assert.match(styles, /\.source-line-numbers \{[^}]*position: absolute;[^}]*text-align: right;/s)
  assert.match(styles, /\.source-sticky-headings \{[^}]*position: absolute;[^}]*z-index: 4;/s)
})

test('embedded rendering sample covers bilingual Markdown and ten Mermaid diagrams', async () => {
  const sample = await readFile(new URL('../samples/markdown-rendering-test.md', import.meta.url), 'utf8')
  assert.match(sample, /^# Markdown 综合渲染测试 \/ Comprehensive Rendering Test/m)
  assert.match(sample, /中文与 English 混排/)
  assert.match(sample, /\| 左对齐功能 \/ Left \|/)
  assert.match(sample, /\$E=mc\^2\$/)
  assert.match(sample, /> \[!NOTE\]/)
  assert.match(sample, /<details>/)
  assert.match(sample, /<script>window\.markdownUnsafeScriptExecuted/)
  assert.equal((sample.match(/~~~mermaid/g) || []).length, 10)
  assert.ok(!sample.includes('/Users/'))
})
