import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import {
  derivePreviewSource,
  largePreviewChunkThreshold,
  splitMarkdownIntoSafeChunks,
} from '../frontend/src/preview-worker-protocol.ts'
import { PreviewPerformanceMetrics } from '../frontend/src/render-performance.ts'
import { LatestRenderScheduler } from '../frontend/src/render-scheduler.ts'

test('an immediate activation consumes a pending debounced source render', async () => {
  const previousWindow = globalThis.window
  globalThis.window = {
    clearTimeout,
    setTimeout,
  }
  try {
    const runs = []
    const scheduler = new LatestRenderScheduler(async (request) => { runs.push(request) })
    await scheduler.request('typing', 40)
    await scheduler.request('activate')
    assert.deepEqual(runs, ['activate'])
    await new Promise((resolve) => setTimeout(resolve, 55))
    assert.deepEqual(runs, ['activate'])
  } finally {
    globalThis.window = previousWindow
  }
})

test('large plain Markdown splits only at safe document boundaries and preserves source lines', () => {
  const paragraph = '这是一段用于超大文档分块渲染验证的正文。'.repeat(4_000)
  const source = Array.from({ length: 12 }, (_value, index) => `## Section ${index + 1}\n\n${paragraph}\n\n`).join('')
  assert.ok(source.length > largePreviewChunkThreshold)
  const chunks = splitMarkdownIntoSafeChunks(source)
  assert.ok(chunks && chunks.length > 1)
  assert.equal(chunks.map((chunk) => chunk.source).join(''), source)
  assert.equal(chunks[0].lineOffset, 0)
  assert.ok(chunks.at(-1).lineOffset > 0)
})

test('semantic cross-block Markdown remains on the single-render path', () => {
  const source = `${'正文\n\n'.repeat(250_000)}\n\`\`\`js\nconst x = 1\n\`\`\``
  assert.ok(source.length > largePreviewChunkThreshold)
  assert.equal(splitMarkdownIntoSafeChunks(source), null)
})

test('worker-derived editor data is shared by line numbers, headings and character count', () => {
  const derived = derivePreviewSource('# 标题\n\n正文😀', 7)
  assert.equal(derived.revision, 7)
  assert.equal(derived.lineNumbers.count, 3)
  assert.equal(derived.headings.headings.length, 1)
  assert.equal(derived.characterCount, 9)
})

test('performance metrics retain bounded phase measurements without source content', () => {
  const metrics = new PreviewPerformanceMetrics()
  const measurement = metrics.begin('tab-1', '# private source')
  measurement.mark('worker')
  measurement.mark('sanitize')
  measurement.mark('core')
  measurement.mark('commit')
  measurement.mark('enhance')
  measurement.setChunked(true)
  measurement.finish()
  const [record] = metrics.snapshot()
  assert.equal(record.tabID, 'tab-1')
  assert.equal(record.sourceBytes, new TextEncoder().encode('# private source').byteLength)
  assert.ok(record.phases.total >= 0)
  assert.equal(record.chunked, true)
  assert.equal(JSON.stringify(record).includes('private source'), false)
})

test('the application exposes measurement, artifact-cache, progressive and worker paths', async () => {
  const [app, worker] = await Promise.all([
    readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8'),
    readFile(new URL('../frontend/src/preview-worker.ts', import.meta.url), 'utf8'),
  ])
  assert.match(app, /const previewArtifacts = new PreviewArtifactCache\(\)/)
  assert.match(app, /function stashActivePreview[\s\S]*document\.createDocumentFragment/)
  assert.match(app, /function restoreCachedPreview[\s\S]*previewArtifacts\.get/)
  assert.match(app, /__inkmarkPreviewMetrics/)
  assert.match(app, /PreviewMarkdownWorker/)
  assert.match(app, /await firstCoreCommitted[\s\S]*appendCorePreviewChunk[\s\S]*await yieldPreviewWork\(\)/)
  assert.match(worker, /type: 'chunk'[\s\S]*self\.postMessage\(response\)[\s\S]*await new Promise<void>\(\(resolve\) => setTimeout\(resolve, 0\)\)/)
})
