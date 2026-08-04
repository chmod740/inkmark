import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import { guardUnsavedChanges, runGuardedDocumentTransition } from '../frontend/src/document-guard.ts'
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
  assert.match(app, /@input="beginScroll\('editor'\)"/)
  assert.match(app, /function runFormat\(action: string\) \{[\s\S]*?beginScroll\('editor'\)[\s\S]*?target\.setRangeText/)
  assert.match(app, /async function runEditAction\(action: string\) \{[\s\S]*?beginScroll\('editor'\)[\s\S]*?target\.setRangeText/)
  assert.match(app, /await renderDiagrams[\s\S]*await nextTick\(\)[\s\S]*refreshScrollAnchors\(\)[\s\S]*reconcileActiveScroll\(\)/)
  assert.match(styles, /\.preview-first \.preview-panel \{ order: -1; \}/)
  assert.match(styles, /\.document-title-row \{[^}]*width: fit-content;[^}]*max-width: 100%;/s)
  assert.match(styles, /\.document-identity h1 \{[^}]*flex: 0 1 auto;[^}]*min-width: 0;/s)
  assert.match(styles, /\.document-state \{[^}]*flex: 0 0 auto;[^}]*white-space: nowrap;/s)
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
