import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import {
  ImageResolverGate,
  PreviewImageBudget,
  PreviewImageResourceSet,
  buildMarkdownImage,
  classifyImageSource,
  exportImageSource,
  forEachWithConcurrency,
  imageDataURI,
  imageResourceCacheKey,
  maximumPreviewImages,
  maximumPendingImageResolvers,
  resolvePreparedImageAsset,
} from '../frontend/src/image-resources.ts'
import { buildStandaloneHTML } from '../frontend/src/export-document.ts'

const appURL = new URL('../frontend/src/App.vue', import.meta.url)

const imageAsset = {
  name: 'sample.png',
  mimeType: 'image/png',
  dataBase64: 'iVBORw0KGgo=',
  size: 8,
  width: 1,
  height: 1,
  sha256: 'fixture',
}

test('image sources are divided into the four supported rendering paths', () => {
  assert.equal(classifyImageSource('assets/photo.png', 'local'), 'local-relative')
  assert.equal(classifyImageSource('../assets/photo.webp', 'webdav'), 'webdav-relative')
  assert.equal(classifyImageSource('data:image/png;base64,iVBORw0KGgo=', 'builtin'), 'data')
  assert.equal(classifyImageSource('https://images.example.test/photo.jpg', 'builtin'), 'public-https')

  assert.equal(classifyImageSource('assets/photo.png', 'builtin'), 'unsupported')
  assert.equal(classifyImageSource('http://images.example.test/photo.jpg', 'local'), 'unsupported')
  assert.equal(classifyImageSource('https://user:secret@example.test/photo.jpg', 'local'), 'unsupported')
  assert.equal(classifyImageSource('https://example.test:8443/photo.jpg', 'local'), 'unsupported')
  assert.equal(classifyImageSource('https://example.test/photo.jpg#fragment', 'local'), 'unsupported')
  assert.equal(classifyImageSource('https://localhost/photo.jpg', 'local'), 'unsupported')
  assert.equal(classifyImageSource('https://files.internal/photo.jpg', 'local'), 'unsupported')
  assert.equal(classifyImageSource('//images.example.test/photo.jpg', 'local'), 'unsupported')
  assert.equal(classifyImageSource('/etc/passwd', 'local'), 'unsupported')
  assert.equal(classifyImageSource('file:///tmp/photo.png', 'local'), 'unsupported')
})

test('Markdown image insertion escapes alternative text and uses a delimited destination', () => {
  assert.equal(
    buildMarkdownImage('diagram [draft]', 'assets/design (1).png'),
    '![diagram \\[draft\\]](<assets/design (1).png>)',
  )
  assert.equal(imageDataURI('IMAGE/PNG', imageAsset.dataBase64), `data:image/png;base64,${imageAsset.dataBase64}`)
  assert.throws(() => imageDataURI('image/svg+xml', 'PHN2Zz4='), /unsupported/)
  assert.throws(() => imageDataURI('image/png', '***'), /invalid/)
})

test('preview image generations revoke every Blob URL together', () => {
  const created = []
  const revoked = []
  const fakeURL = {
    createObjectURL(blob) {
      assert.equal(blob.type, 'image/png')
      const value = `blob:inkmark-test-${created.length + 1}`
      created.push(value)
      return value
    },
    revokeObjectURL(value) { revoked.push(value) },
  }
  const resources = new PreviewImageResourceSet(fakeURL)
  const localKey = imageResourceCacheKey('local-relative', '/documents/readme.md', 'assets/sample.png')
  const publicKey = imageResourceCacheKey('public-https', '', 'https://example.test/sample.png')
  const local = resources.add('local-1', localKey, 'local-relative', 'assets/sample.png', imageAsset)
  const duplicateLocal = resources.add('local-2', localKey, 'local-relative', 'assets/sample.png', imageAsset)
  const publicImage = resources.add('public-1', publicKey, 'public-https', 'https://example.test/sample.png', imageAsset)
  const embedded = resources.addDataURI('data-1', imageDataURI('image/png', imageAsset.dataBase64))

  assert.deepEqual(created, ['blob:inkmark-test-1', 'blob:inkmark-test-2'])
  assert.equal(resources.records.size, 4)
  assert.equal(duplicateLocal.previewSource, local.previewSource)
  assert.equal(exportImageSource(local, 'html'), local.dataURI)
  assert.equal(exportImageSource(publicImage, 'html'), 'https://example.test/sample.png')
  assert.equal(exportImageSource(publicImage, 'doc'), publicImage.dataURI)
  assert.equal(exportImageSource(publicImage, 'capture'), publicImage.dataURI)
  assert.equal(exportImageSource(embedded, 'html'), embedded.dataURI)

  resources.discard('local-1')
  assert.deepEqual(revoked, [], 'a shared Blob must survive while another image still references it')
  assert.equal(resources.records.has('local-1'), false)
  resources.discard('local-2')
  assert.deepEqual(revoked, ['blob:inkmark-test-1'])
  resources.release()
  assert.deepEqual(revoked, created)
  assert.equal(resources.records.size, 0)
})

test('preview budgets bound count, unique bytes, pending work, and resolver concurrency', async () => {
  const budget = new PreviewImageBudget()
  for (let index = 0; index < maximumPreviewImages; index += 1) budget.claimImage('shared')
  assert.throws(() => budget.claimImage('shared'), /too many images/)

  const byteBudget = new PreviewImageBudget()
  for (let index = 0; index < 4; index += 1) {
    const key = `asset-${index}`
    byteBudget.claimImage(key)
    assert.equal(byteBudget.beginResolve(key), true)
    byteBudget.finishResolve(key, { ...imageAsset, size: 16 << 20 })
  }
  byteBudget.claimImage('overflow')
  assert.throws(() => byteBudget.beginResolve('overflow'), /byte budget/)

  const decodedBudget = new PreviewImageBudget()
  decodedBudget.claimImage('large-decoded')
  decodedBudget.reserveAsset('large-decoded', { ...imageAsset, width: 8192, height: 8192 })
  decodedBudget.claimImage('decoded-overflow')
  assert.throws(
    () => decodedBudget.reserveAsset('decoded-overflow', { ...imageAsset, width: 1, height: 1 }),
    /decoded byte budget/,
  )

  const gate = new ImageResolverGate(2)
  let active = 0
  let peak = 0
  let release
  const blocked = new Promise((resolve) => { release = resolve })
  const tasks = Array.from({ length: 5 }, () => gate.run(async () => {
    active += 1
    peak = Math.max(peak, active)
    await blocked
    active -= 1
    return imageAsset
  }, () => true))
  await new Promise((resolve) => setTimeout(resolve, 0))
  assert.equal(peak, 2)
  release()
  await Promise.all(tasks)
  assert.equal(peak, 2)

  const superseded = new ImageResolverGate(1)
  let firstRelease
  const first = superseded.run(() => new Promise((resolve) => { firstRelease = () => resolve(imageAsset) }), () => true)
  const skipped = superseded.run(async () => imageAsset, () => false)
  await new Promise((resolve) => setTimeout(resolve, 0))
  firstRelease()
  await first
  await assert.rejects(skipped, /superseded/)

  const boundedBudget = new PreviewImageBudget()
  let resolverCalls = 0
  await forEachWithConcurrency(Array.from({ length: 5 }, (_, index) => index), 4, async (index) => {
    const key = `bounded-${index}`
    try {
      boundedBudget.claimImage(key)
      boundedBudget.beginResolve(key)
      resolverCalls += 1
      boundedBudget.finishResolve(key, { ...imageAsset, size: 16 << 20 })
    } catch {
      // The fifth maximum-sized resource is represented as an error
      // placeholder without invoking its resolver.
    }
  })
  assert.equal(resolverCalls, 4)
})

test('successive renders reuse active image bytes, deduplicate pending work, and isolate document contexts', async () => {
  const active = new PreviewImageResourceSet({
    createObjectURL: () => 'blob:active-image',
    revokeObjectURL: () => {},
  })
  const pending = new Map()
  const firstContext = imageResourceCacheKey('webdav-relative', 'workspace-1\0document-1', 'assets/sample.png')
  const secondContext = imageResourceCacheKey('webdav-relative', 'workspace-2\0document-2', 'assets/sample.png')
  let loads = 0
  const loader = async () => {
    loads += 1
    await Promise.resolve()
    return imageAsset
  }
  const [first, second] = await Promise.all([
    resolvePreparedImageAsset(active, pending, firstContext, 1, (revision) => revision === 1, () => loader()),
    resolvePreparedImageAsset(active, pending, firstContext, 1, (revision) => revision === 1, () => loader()),
  ])
  assert.equal(first, second)
  assert.equal(loads, 1)
  assert.equal(pending.size, 0)

  active.add('active-1', firstContext, 'webdav-relative', 'assets/sample.png', first)
  await resolvePreparedImageAsset(active, pending, firstContext, 1, (revision) => revision === 1, () => loader())
  assert.equal(loads, 1, 'the next committed generation must reuse verified active bytes')
  await resolvePreparedImageAsset(active, pending, secondContext, 1, (revision) => revision === 1, () => loader())
  assert.equal(loads, 2, 'a different workspace/document context must resolve again')

  let failures = 0
  const failing = () => {
    failures += 1
    return Promise.reject(new Error('offline'))
  }
  await assert.rejects(resolvePreparedImageAsset(active, pending, 'failure', 1, () => true, () => failing()), /offline/)
  await assert.rejects(resolvePreparedImageAsset(active, pending, 'failure', 1, () => true, () => failing()), /offline/)
  assert.equal(failures, 2, 'failed pending work must be removed so the user can retry')
  active.release()
})

test('a newer preview generation adopts matching queued image work', async () => {
  const active = new PreviewImageResourceSet({
    createObjectURL: () => 'blob:unused',
    revokeObjectURL: () => {},
  })
  const pending = new Map()
  const gate = new ImageResolverGate(1)
  let currentRevision = 1
  let releaseBlocker
  const blocker = resolvePreparedImageAsset(
    active,
    pending,
    'blocker',
    1,
    (revision) => revision === currentRevision,
    (shouldRun) => gate.run(() => new Promise((resolve) => {
      releaseBlocker = () => resolve(imageAsset)
    }), shouldRun),
  )
  const oldShared = resolvePreparedImageAsset(
    active,
    pending,
    'shared',
    1,
    (revision) => revision === currentRevision,
    (shouldRun) => gate.run(async () => imageAsset, shouldRun),
  )
  await new Promise((resolve) => setTimeout(resolve, 0))
  currentRevision = 2
  const newShared = resolvePreparedImageAsset(
    active,
    pending,
    'shared',
    2,
    (revision) => revision === currentRevision,
    (shouldRun) => gate.run(async () => imageAsset, shouldRun),
  )
  releaseBlocker()
  await blocker
  assert.equal(await oldShared, imageAsset)
  assert.equal(await newShared, imageAsset)
  active.release()
})

test('a saturated stale generation is pruned before admitting new image keys', async () => {
  const active = new PreviewImageResourceSet({
    createObjectURL: () => 'blob:unused',
    revokeObjectURL: () => {},
  })
  const pending = new Map()
  const gate = new ImageResolverGate(1)
  let currentRevision = 1
  let releaseActive
  const oldRequests = []
  oldRequests.push(resolvePreparedImageAsset(
    active,
    pending,
    'old-active',
    1,
    (revision) => revision === currentRevision,
    (shouldRun) => gate.run(() => new Promise((resolve) => {
      releaseActive = () => resolve(imageAsset)
    }), shouldRun),
  ))
  for (let index = 1; index < maximumPendingImageResolvers; index += 1) {
    oldRequests.push(resolvePreparedImageAsset(
      active,
      pending,
      `old-${index}`,
      1,
      (revision) => revision === currentRevision,
      (shouldRun) => gate.run(async () => imageAsset, shouldRun),
    ))
  }
  const settledOldRequests = Promise.allSettled(oldRequests)
  await new Promise((resolve) => setTimeout(resolve, 0))
  assert.equal(pending.size, maximumPendingImageResolvers)

  currentRevision = 2
  let newLoaderCalls = 0
  const next = resolvePreparedImageAsset(
    active,
    pending,
    'new-key',
    2,
    (revision) => revision === currentRevision,
    (shouldRun) => gate.run(async () => {
      newLoaderCalls += 1
      return imageAsset
    }, shouldRun),
  )
  await new Promise((resolve) => setTimeout(resolve, 0))
  releaseActive()
  assert.equal(await next, imageAsset)
  await settledOldRequests
  assert.equal(newLoaderCalls, 1)
  assert.equal(pending.size, 0)
  active.release()
})

test('standalone HTML permits embedded images and retained public HTTPS images without scripts', () => {
  const html = buildStandaloneHTML({
    title: 'Images',
    theme: 'github',
    articleHTML: '<img src="data:image/png;base64,abc" /><img src="https://example.test/image.png" />',
    embeddedStyles: '',
  })
  assert.match(html, /img-src data: blob: file: http: https:/)
  assert.match(html, /src="data:image\/png;base64,abc"/)
  assert.match(html, /src="https:\/\/example\.test\/image\.png"/)
  assert.match(html, /script-src 'none'/)
})

test('App resolves images off-screen and atomically commits one prepared generation', async () => {
  const app = await readFile(appURL, 'utf8')
  const prepare = app.match(/async function preparePreviewImages\([\s\S]*?\n\}/)?.[0] || ''
  const render = app.match(/async function renderNow\([\s\S]*?\n\}/)?.[0] || ''
  const exportDocument = app.match(/async function exportDocument\([\s\S]*?\n\}/)?.[0] || ''
  const capture = app.match(/async function capturePreviewCanvas\([\s\S]*?\n\}/)?.[0] || ''

  for (const bridge of [
    'SelectImageFile',
    'ImportLocalImageData',
    'ImportWebDAVImageData',
    'ResolveLocalImage',
    'ResolveWebDAVImage',
    'FetchPublicImage',
    'ValidateImageData',
  ]) assert.match(app, new RegExp(`\\b${bridge}\\b`))

  assert.match(app, /action === 'insert-image'\) showImageDialog\(\)/)
  assert.match(app, /id="image-insert-form"/)
  assert.match(app, /imageInsertMode === 'public'/)
  assert.match(app, /ImportLocalImageData\([\s\S]*context\.localDocumentPath/)
  assert.match(app, /ImportWebDAVImageData\([\s\S]*context\.remoteWorkspaceId[\s\S]*context\.remoteDocumentId/)
  assert.match(prepare, /classifyImageSource/)
  assert.match(prepare, /maximumPreviewImages/)
  assert.match(prepare, /PreviewImageBudget/)
  assert.match(prepare, /imageResolverGate\.run/)
  assert.match(prepare, /ValidateImageData/)
  assert.match(prepare, /cacheKey = originalSource/)
  assert.match(prepare, /budget\.beginResolve\(cacheKey\)/)
  assert.match(prepare, /budget\.reserveAsset\(cacheKey, resolved\)/)
  assert.match(prepare, /imageDecodeGate\.run/)
  assert.match(prepare, /ResolveLocalImage/)
  assert.match(prepare, /ResolveWebDAVImage/)
  assert.match(prepare, /FetchPublicImage/)
  assert.match(prepare, /imageResourceCacheKey\(kind, contextKey, originalSource\)/)
  assert.match(prepare, /resolvePreparedImageAsset\([\s\S]*activePreviewImages,[\s\S]*pendingImageAssets,[\s\S]*cacheKey/)
  assert.match(prepare, /imageDecodeGate\.run\([\s\S]*waitForImageDecode\(image\)/)
  assert.match(prepare, /placeholder\.dataset\.inkmarkPublicImage = originalSource/)
  assert.match(render, /const staging = target\.cloneNode\(false\)/)
  assert.match(render, /FORBID_TAGS:[^\n]*'img'[^\n]*'picture'[^\n]*'source'/)
  assert.match(render, /FORBID_ATTR:[\s\S]*?'src'[\s\S]*?'srcset'[\s\S]*?'style'[\s\S]*?'data-inkmark-public-image'/)
  assert.match(render, /await Promise\.all\(\[[\s\S]*renderDiagrams[\s\S]*preparePreviewImages/)
  assert.match(render, /target\.replaceChildren/)
  assert.match(render, /activePreviewImages\.release\(\)[\s\S]*activePreviewImages = nextPreviewImages/)
  assert.match(exportDocument, /articleHTML: exportArticleHTML\(target, format\)/)
  assert.match(capture, /materializeExportImages\(clone, 'capture'\)/)
  assert.match(app, /format === 'html'[\s\S]*data-inkmark-public-image[\s\S]*image\.src = source/)
  assert.doesNotMatch(app, /replaceRemoteImagesForOfflineCapture/)
})
