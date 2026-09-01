import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import {
  BoundedCache,
  LatestPreviewCommit,
  maximumMermaidCacheEntries,
  maximumMermaidDiagramsPerPreview,
  mermaidCacheKey,
  normalizeMermaidMath,
} from '../frontend/src/preview-render.ts'
import { embedMermaidMathLabelHTML, normalizeMermaidSafeHTML } from '../frontend/src/mermaid-math.ts'

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
  assert.notEqual(mermaidCacheKey('github', 'flowchart LR; A-->B', true), mermaidCacheKey('github', 'flowchart LR; A-->B', false))
  assert.equal(maximumMermaidCacheEntries, 64)
  assert.equal(maximumMermaidDiagramsPerPreview, 16)

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

test('Mermaid node and edge labels accept inline and native formula delimiters', () => {
  const normalized = normalizeMermaidMath(String.raw`flowchart LR
  A["速度 $v=\frac{s}{t}$"] -->|误差 $\epsilon$| B["输出 $$y=f(x)$$"]
  C["价格 \$20"]
  %% $comment$ must remain untouched`)

  assert.equal(normalized.hasMath, true)
  assert.ok(normalized.definition.includes(String.raw`速度 $$v=\frac{s}{t}$$`))
  assert.ok(normalized.definition.includes(String.raw`误差 $$\epsilon$$`))
  assert.ok(normalized.definition.includes('输出 $$y=f(x)$$'))
  assert.ok(normalized.definition.includes(String.raw`价格 \$20`))
  assert.ok(normalized.definition.includes('%% $comment$ must remain untouched'))

  assert.deepEqual(normalizeMermaidMath('flowchart LR\n  A["price $20"] --> B'), {
    definition: 'flowchart LR\n  A["price $20"] --> B',
    hasMath: false,
  })
})

test('Mermaid formula labels are converted to stable inline HTML before layout', () => {
  const embedded = embedMermaidMathLabelHTML(String.raw`flowchart LR
  A["输入 $$x_i=\frac{i}{n}$$"] -->|"损失 $$\mathcal{L}=\sum_i e_i$$"| B["输出 $$\hat y=f_\theta(x)$$"]`)
  const unquoted = embedMermaidMathLabelHTML(String.raw`flowchart LR
  A[输入 $$x_i$$] -->|概率 $$p(y\mid x)$$| B`)

  assert.equal(embedded.changed, true)
  assert.equal((embedded.definition.match(/inkmark-mermaid-formula/g) || []).length, 3)
  assert.ok(!embedded.definition.includes('$$'))
  assert.match(embedded.definition, /<i>x<\/i><sub>i<\/sub>=<span class='inkmark-mermaid-fraction'>/)
  assert.match(embedded.definition, /ℒ=∑<sub>i<\/sub>/)
  assert.match(embedded.definition, /inkmark-mermaid-accent/)
  assert.match(embedded.definition, /ŷ/)
  assert.ok(!embedded.definition.includes('<foreignObject'))
  assert.ok(!embedded.definition.includes('<script'))
  assert.equal((unquoted.definition.match(/inkmark-mermaid-formula/g) || []).length, 2)
  assert.ok(!unquoted.definition.includes('$$'))
})

test('Mermaid safe HTML labels accept escaped tags and unquoted edge labels', () => {
  const normalized = normalizeMermaidSafeHTML(String.raw`flowchart LR
  A["输入\<br/>帧"] -->|功率谱 \<i>P\</i>\<sub>t\</sub>[k]| B["输出"]
  %% \<i>comment\</i> must stay unchanged`)

  assert.equal(normalized.hasHTML, true)
  assert.match(normalized.definition, /A\["输入<br\/>帧"\]/)
  assert.match(normalized.definition, /\|"功率谱 <i>P<\/i><sub>t<\/sub>\[k\]"\|/)
  assert.match(normalized.definition, /%% \\<i>comment\\<\/i> must stay unchanged/)
})

test('App uses a worker, commits core Markdown first, then enhances expensive resources', async () => {
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')

  assert.match(app, /import PreviewMarkdownWorker from '\.\/preview-worker\?worker'/)
  assert.match(app, /renderMarkdownInWorker\(sourceText, expectedRevision, trustedMarker, \(chunk\) =>/)
  assert.match(app, /sanitizePreviewHTML\(renderedHTML\)/)
  assert.match(app, /function createCorePreviewStaging[\s\S]*target\.cloneNode\(false\)[\s\S]*staging\.innerHTML = sanitizePreviewHTML\(renderedHTML\)/)
  assert.match(app, /decoratePreview\(staging\)[\s\S]*renderMath\(staging\)[\s\S]*highlightCode\(staging\)/)
  assert.match(app, /previewCommit\.stageAndCommit\(sequence, \(\) => \{[\s\S]*createCorePreviewStaging\(target, renderedHTML, trustedMarker\)[\s\S]*commitCorePreview\(target, staging/)
  assert.match(app, /function enhancePreview[\s\S]*await Promise\.all\(\[[\s\S]*renderDiagrams\(root[\s\S]*preparePreviewImages\(root/)
  assert.match(app, /const enhancement = enhancePreview\(target, sequence/)
  assert.match(app, /diagrams\.slice\(maximumMermaidDiagramsPerPreview\)[\s\S]*diagram count exceeds the limit/)
  assert.match(app, /target\.replaceChildren\([\s\S]*refreshScrollAnchors\(\)[\s\S]*reconcileActiveScroll\(\)/)
  assert.doesNotMatch(app, /target\.innerHTML\s*=\s*cleanHTML/)
  assert.match(app, /mermaidRenderCache\.get\(cacheKey\)/)
  assert.match(app, /mermaidRenderCache\.set\(cacheKey, rendered\)/)
  assert.match(app, /normalizeMermaidMath\(safeHTML\.definition\)/)
  assert.match(app, /normalizeMermaidSafeHTML\(sourceDefinition\)/)
  assert.match(app, /embedMermaidMathLabelHTML\(normalized\.definition\)/)
  assert.match(app, /securityLevel: 'strict'/)
  assert.match(app, /htmlLabels: mathLabels/)
  assert.match(app, /classList\.add\('mermaid-math-labels'\)/)
  assert.match(app, /svg\.setAttribute\('width', String\(measuredWidth\)\)/)
  assert.match(app, /rasterizeMermaidDiagrams\(clone\)/)
  assert.match(app, /querySelectorAll<SVGForeignObjectElement>\('\.mermaid-rendered svg foreignObject'\)/)
  assert.match(app, /replaceMermaidHTMLLabelWithSVGText/)
  assert.match(app, /runSpan\.setAttribute\('baseline-shift', run\.baseline\)/)
  assert.match(app, /new XMLSerializer\(\)\.serializeToString\(svg\)/)
  assert.match(app, /svg\.replaceWith\(replacement\)/)
  assert.match(app, /replacement\.src = URL\.createObjectURL\(blob\)/)
  assert.match(app, /URL\.revokeObjectURL\(url\)/)
})
