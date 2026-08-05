import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import {
  BoundedCache,
  LatestPreviewCommit,
  maximumMermaidCacheEntries,
  mermaidCacheKey,
} from '../frontend/src/preview-render.ts'

function deferred() {
  let resolve
  const promise = new Promise((complete) => { resolve = complete })
  return { promise, resolve }
}

test('staging leaves the live preview untouched and only the latest render commits', async () => {
  const gate = new LatestPreviewCommit()
  const firstReady = deferred()
  const secondReady = deferred()
  const commits = []
  let livePreview = 'previous complete preview'

  const firstRevision = gate.begin()
  const first = gate.stageAndCommit(firstRevision, async () => {
    await firstReady.promise
    return 'obsolete staged preview'
  }, (staged) => {
    commits.push(staged)
    livePreview = staged
  })

  const secondRevision = gate.begin()
  const second = gate.stageAndCommit(secondRevision, async () => {
    await secondReady.promise
    return 'latest complete preview'
  }, (staged) => {
    commits.push(staged)
    livePreview = staged
  })

  assert.equal(livePreview, 'previous complete preview')
  assert.deepEqual(commits, [])

  secondReady.resolve()
  assert.equal(await second, true)
  assert.equal(livePreview, 'latest complete preview')
  assert.deepEqual(commits, ['latest complete preview'])

  firstReady.resolve()
  assert.equal(await first, false)
  assert.equal(livePreview, 'latest complete preview')
  assert.deepEqual(commits, ['latest complete preview'])
})

test('explicit invalidation prevents an in-flight render from committing', async () => {
  const gate = new LatestPreviewCommit()
  const ready = deferred()
  let commits = 0
  const revision = gate.begin()
  const render = gate.stageAndCommit(revision, async () => {
    await ready.promise
    return 'staged'
  }, () => { commits += 1 })

  gate.invalidate()
  ready.resolve()
  assert.equal(await render, false)
  assert.equal(commits, 0)
})

test('Mermaid cache is theme-aware, definition-aware, bounded, and LRU', () => {
  assert.equal(mermaidCacheKey('github', 'flowchart LR; A-->B'), mermaidCacheKey('github', 'flowchart LR; A-->B'))
  assert.notEqual(mermaidCacheKey('dark', 'flowchart LR; A-->B'), mermaidCacheKey('github', 'flowchart LR; A-->B'))
  assert.notEqual(mermaidCacheKey('github', 'flowchart LR; A-->C'), mermaidCacheKey('github', 'flowchart LR; A-->B'))
  assert.equal(maximumMermaidCacheEntries, 64)

  const cache = new BoundedCache(2)
  cache.set('first', '<svg>first</svg>')
  cache.set('second', '<svg>second</svg>')
  assert.equal(cache.get('first'), '<svg>first</svg>')
  cache.set('third', '<svg>third</svg>')

  assert.equal(cache.size, 2)
  assert.equal(cache.get('second'), undefined)
  assert.equal(cache.get('first'), '<svg>first</svg>')
  assert.equal(cache.get('third'), '<svg>third</svg>')
  cache.clear()
  assert.equal(cache.size, 0)
})

test('App renders into a detached root and atomically swaps only completed content', async () => {
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')

  assert.match(app, /previewCommit\.stageAndCommit\(sequence, async \(\) => \{[\s\S]*target\.cloneNode\(false\)[\s\S]*staging\.innerHTML = cleanHTML/)
  assert.match(app, /decoratePreview\(staging\)[\s\S]*renderMath\(staging\)[\s\S]*highlightCode\(staging\)[\s\S]*await renderDiagrams\(staging/)
  assert.match(app, /target\.replaceChildren\([\s\S]*refreshScrollAnchors\(\)[\s\S]*reconcileActiveScroll\(\)/)
  assert.doesNotMatch(app, /target\.innerHTML\s*=\s*cleanHTML/)
  assert.match(app, /mermaidRenderCache\.get\(cacheKey\)/)
  assert.match(app, /mermaidRenderCache\.set\(cacheKey, rendered\)/)
})
