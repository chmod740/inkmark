export type DocumentImageStorage = 'local' | 'webdav' | 'builtin'
export type ImageSourceKind = 'data' | 'local-relative' | 'webdav-relative' | 'public-https' | 'unsupported'

export interface ImageAssetData {
  name: string
  mimeType: string
  dataBase64: string
  size: number
  width: number
  height: number
  sha256: string
}

export interface ImageAsset {
  markdownURL: string
  name: string
  mimeType: string
  size: number
  width: number
  height: number
  sha256: string
}

export interface PreparedImageResource {
  id: string
  cacheKey: string
  kind: Exclude<ImageSourceKind, 'unsupported'>
  originalSource: string
  previewSource: string
  dataURI: string
  mimeType: string
  size: number
  width: number
  height: number
  sha256: string
}

export interface PendingImageAssetRequest {
  promise: Promise<ImageAssetData>
  revision: number
}

interface ObjectURLAPI {
  createObjectURL(value: Blob): string
  revokeObjectURL(value: string): void
}

const supportedImageMIMETypes = new Set([
  'image/gif',
  'image/jpeg',
  'image/png',
  'image/webp',
])
const maximumImageBytes = 16 << 20
export const maximumPreviewImages = 64
export const maximumPreviewUniqueImages = 32
export const maximumPreviewImageBytes = 64 << 20
export const maximumPreviewDecodedBytes = 256 << 20
export const maximumPendingImageResolvers = 32
export const maximumConcurrentImageResolvers = 4
export const maximumConcurrentImageDecodes = 2

const dataImagePattern = /^data:(image\/(?:gif|jpeg|png|webp));base64,([a-z\d+/=\s]+)$/i
const explicitSchemePattern = /^[a-z][a-z\d+.-]*:/i

export async function forEachWithConcurrency<Value>(
  values: readonly Value[],
  limit: number,
  callback: (value: Value, index: number) => Promise<void>,
) {
  if (!Number.isSafeInteger(limit) || limit < 1) throw new Error('invalid concurrency limit')
  let cursor = 0
  const workers = Array.from({ length: Math.min(limit, values.length) }, async () => {
    while (cursor < values.length) {
      const index = cursor
      cursor += 1
      await callback(values[index], index)
    }
  })
  await Promise.all(workers)
}

export function classifyImageSource(source: string, storage: DocumentImageStorage): ImageSourceKind {
  const trimmed = source.trim()
  if (!trimmed) return 'unsupported'
  if (dataImagePattern.test(trimmed)) return 'data'

  try {
    const parsed = new URL(trimmed)
    const hostname = parsed.hostname.toLowerCase().replace(/\.$/, '')
    if (
      parsed.protocol === 'https:'
      && !parsed.username
      && !parsed.password
      && parsed.hostname
      && (!parsed.port || parsed.port === '443')
      && !parsed.hash
      && !trimmed.endsWith('?')
      && hostname !== 'localhost'
      && !hostname.endsWith('.localhost')
      && !hostname.endsWith('.local')
      && !hostname.endsWith('.internal')
    ) return 'public-https'
    return 'unsupported'
  } catch {
    // Relative Markdown destinations are resolved through the active document
    // capability below. An explicit or root-relative URL must never be treated
    // as a local file path.
  }

  if (
    trimmed.startsWith('/')
    || trimmed.startsWith('\\')
    || trimmed.startsWith('//')
    || explicitSchemePattern.test(trimmed)
    || trimmed.includes('\0')
  ) return 'unsupported'
  if (storage === 'local') return 'local-relative'
  if (storage === 'webdav') return 'webdav-relative'
  return 'unsupported'
}

export function imageDataURI(mimeType: string, dataBase64: string): string {
  const normalizedMIME = mimeType.trim().toLowerCase()
  if (!supportedImageMIMETypes.has(normalizedMIME)) throw new Error('unsupported image type')
  const normalizedData = dataBase64.replace(/\s+/g, '')
  if (!normalizedData || normalizedData.length % 4 !== 0 || !/^[a-z\d+/]+={0,2}$/i.test(normalizedData)) {
    throw new Error('invalid image data')
  }
  const padding = normalizedData.endsWith('==') ? 2 : normalizedData.endsWith('=') ? 1 : 0
  const decodedSize = normalizedData.length / 4 * 3 - padding
  if (decodedSize > maximumImageBytes) throw new Error('image data is too large')
  return `data:${normalizedMIME};base64,${normalizedData}`
}

export function dataURIImageAsset(source: string): ImageAssetData {
  const match = source.trim().match(dataImagePattern)
  if (!match) throw new Error('invalid image data URI')
  const dataBase64 = match[2].replace(/\s+/g, '')
  imageDataURI(match[1], dataBase64)
  const padding = dataBase64.endsWith('==') ? 2 : dataBase64.endsWith('=') ? 1 : 0
  return {
    name: '',
    mimeType: match[1].toLowerCase(),
    dataBase64,
    size: dataBase64.length / 4 * 3 - padding,
    width: 0,
    height: 0,
    sha256: '',
  }
}

export function buildMarkdownImage(altText: string, source: string): string {
  const safeAlt = altText.replaceAll('\\', '\\\\').replaceAll('[', '\\[').replaceAll(']', '\\]')
  const safeSource = source.trim().replaceAll('<', '%3C').replaceAll('>', '%3E')
  return `![${safeAlt}](<${safeSource}>)`
}

export function defaultImageAlt(name: string): string {
  return name.trim().replace(/\.[^.]+$/, '')
}

export class PreviewImageResourceSet {
  readonly records = new Map<string, PreparedImageResource>()
  private readonly objectURLs = new Set<string>()
  private readonly resourcesByCacheKey = new Map<string, PreparedImageResource>()
  private readonly urlAPI: ObjectURLAPI

  constructor(urlAPI: ObjectURLAPI = URL) {
    this.urlAPI = urlAPI
  }

  add(
    id: string,
    cacheKey: string,
    kind: Exclude<ImageSourceKind, 'unsupported'>,
    originalSource: string,
    asset: ImageAssetData,
  ): PreparedImageResource {
    const reusable = cacheKey ? this.resourcesByCacheKey.get(cacheKey) : undefined
    if (reusable) {
      const record = { ...reusable, id, kind, originalSource }
      this.records.set(id, record)
      return record
    }
    const dataURI = imageDataURI(asset.mimeType, asset.dataBase64)
    const bytes = base64ToBytes(asset.dataBase64)
    const previewSource = this.urlAPI.createObjectURL(new Blob([bytes], { type: asset.mimeType }))
    this.objectURLs.add(previewSource)
    const record: PreparedImageResource = {
      id,
      cacheKey,
      kind,
      originalSource,
      previewSource,
      dataURI,
      mimeType: asset.mimeType,
      size: asset.size,
      width: asset.width,
      height: asset.height,
      sha256: asset.sha256,
    }
    this.records.set(id, record)
    if (cacheKey) this.resourcesByCacheKey.set(cacheKey, record)
    return record
  }

  addDataURI(id: string, originalSource: string): PreparedImageResource {
    const asset = dataURIImageAsset(originalSource)
    const dataURI = imageDataURI(asset.mimeType, asset.dataBase64)
    const record: PreparedImageResource = {
      id,
      cacheKey: '',
      kind: 'data',
      originalSource,
      previewSource: dataURI,
      dataURI,
      mimeType: asset.mimeType,
      size: asset.size,
      width: asset.width,
      height: asset.height,
      sha256: asset.sha256,
    }
    this.records.set(id, record)
    return record
  }

  release() {
    this.objectURLs.forEach((value) => this.urlAPI.revokeObjectURL(value))
    this.objectURLs.clear()
    this.resourcesByCacheKey.clear()
    this.records.clear()
  }

  discard(id: string) {
    const record = this.records.get(id)
    if (!record) return
    this.records.delete(id)
    const replacement = Array.from(this.records.values()).find((candidate) => candidate.cacheKey === record.cacheKey)
    if (record.cacheKey && this.resourcesByCacheKey.get(record.cacheKey)?.id === id) {
      if (replacement) this.resourcesByCacheKey.set(record.cacheKey, replacement)
      else this.resourcesByCacheKey.delete(record.cacheKey)
    }
    const stillUsed = Array.from(this.records.values()).some((candidate) => candidate.previewSource === record.previewSource)
    if (!stillUsed && record.previewSource.startsWith('blob:') && this.objectURLs.delete(record.previewSource)) {
      this.urlAPI.revokeObjectURL(record.previewSource)
    }
  }

  assetForCacheKey(cacheKey: string): ImageAssetData | null {
    if (!cacheKey) return null
    for (const record of this.records.values()) {
      if (record.cacheKey !== cacheKey) continue
      const asset = dataURIImageAsset(record.dataURI)
      return {
        ...asset,
        mimeType: record.mimeType,
        size: record.size,
        width: record.width,
        height: record.height,
        sha256: record.sha256,
      }
    }
    return null
  }
}

export class PreviewImageBudget {
  private count = 0
  private totalBytes = 0
  private totalDecodedBytes = 0
  private reservedBytes = 0
  private readonly uniqueKeys = new Set<string>()
  private readonly byteKeys = new Set<string>()
  private readonly decodedKeys = new Set<string>()
  private readonly resolvingKeys = new Set<string>()

  claimImage(cacheKey: string) {
    this.count += 1
    if (this.count > maximumPreviewImages) throw new Error('too many images in the document')
    if (this.uniqueKeys.has(cacheKey)) return
    if (this.uniqueKeys.size >= maximumPreviewUniqueImages) throw new Error('too many unique images in the document')
    this.uniqueKeys.add(cacheKey)
  }

  beginResolve(cacheKey: string) {
    if (this.byteKeys.has(cacheKey) || this.resolvingKeys.has(cacheKey)) return false
    if (this.totalBytes + this.reservedBytes + maximumImageBytes > maximumPreviewImageBytes) {
      throw new Error('image byte budget exceeded')
    }
    this.resolvingKeys.add(cacheKey)
    this.reservedBytes += maximumImageBytes
    return true
  }

  cancelResolve(cacheKey: string) {
    if (!this.resolvingKeys.delete(cacheKey)) return
    this.reservedBytes -= maximumImageBytes
  }

  finishResolve(cacheKey: string, asset: ImageAssetData) {
    this.cancelResolve(cacheKey)
    this.reserveAsset(cacheKey, asset)
  }

  reserveAsset(cacheKey: string, asset: ImageAssetData) {
    const size = asset.size
    if (!Number.isSafeInteger(size) || size < 0 || size > maximumImageBytes) throw new Error('invalid image size')
    const newBytes = !this.byteKeys.has(cacheKey)
    const newDecodedBytes = !this.decodedKeys.has(cacheKey)
    if (newBytes && this.totalBytes + size > maximumPreviewImageBytes) throw new Error('image byte budget exceeded')
    if (!newDecodedBytes) {
      if (newBytes) {
        this.totalBytes += size
        this.byteKeys.add(cacheKey)
      }
      return
    }
    if (!Number.isSafeInteger(asset.width) || !Number.isSafeInteger(asset.height) || asset.width < 1 || asset.height < 1) {
      throw new Error('invalid image dimensions')
    }
    const decodedBytes = asset.width * asset.height * 4
    if (!Number.isSafeInteger(decodedBytes) || decodedBytes > maximumPreviewDecodedBytes) {
      throw new Error('image decoded size is too large')
    }
    if (this.totalDecodedBytes + decodedBytes > maximumPreviewDecodedBytes) {
      throw new Error('image decoded byte budget exceeded')
    }
    if (newBytes) {
      this.totalBytes += size
      this.byteKeys.add(cacheKey)
    }
    this.totalDecodedBytes += decodedBytes
    this.decodedKeys.add(cacheKey)
  }
}

class BoundedImageWorkGate<Result> {
  private active = 0
  private readonly limit: number
  private readonly queue: Array<{
    task: () => Promise<Result>
    shouldRun: () => boolean
    resolve: (result: Result) => void
    reject: (error: unknown) => void
  }> = []

  constructor(limit = maximumConcurrentImageResolvers) {
    if (!Number.isSafeInteger(limit) || limit < 1) throw new Error('invalid image resolver limit')
    this.limit = limit
  }

  run(task: () => Promise<Result>, shouldRun: () => boolean): Promise<Result> {
    this.pruneSuperseded()
    if (this.queue.length + this.active >= maximumPendingImageResolvers) {
      return Promise.reject(new Error('too many pending image requests'))
    }
    return new Promise<Result>((resolve, reject) => {
      this.queue.push({ task, shouldRun, resolve, reject })
      this.drain()
    })
  }

  private pruneSuperseded() {
    for (let index = this.queue.length - 1; index >= 0; index -= 1) {
      if (this.queue[index].shouldRun()) continue
      const [entry] = this.queue.splice(index, 1)
      entry.reject(new Error('image request was superseded'))
    }
  }

  private drain() {
    while (this.active < this.limit && this.queue.length) {
      const entry = this.queue.shift()!
      if (!entry.shouldRun()) {
        entry.reject(new Error('image request was superseded'))
        continue
      }
      this.active += 1
      Promise.resolve()
        .then(entry.task)
        .then(entry.resolve, entry.reject)
        .finally(() => {
          this.active -= 1
          this.drain()
        })
    }
  }
}

export class ImageResolverGate extends BoundedImageWorkGate<ImageAssetData> {
  constructor(limit = maximumConcurrentImageResolvers) {
    super(limit)
  }
}

export class ImageDecodeGate extends BoundedImageWorkGate<void> {
  constructor(limit = maximumConcurrentImageDecodes) {
    super(limit)
  }
}

export function imageResourceCacheKey(kind: ImageSourceKind, contextKey: string, source: string) {
  return JSON.stringify([kind, contextKey, source.trim()])
}

export function resolvePreparedImageAsset(
  active: PreviewImageResourceSet,
  pending: Map<string, PendingImageAssetRequest>,
  cacheKey: string,
  revision: number,
  isCurrent: (revision: number) => boolean,
  loader: (shouldRun: () => boolean) => Promise<ImageAssetData>,
): Promise<ImageAssetData> {
  const reusable = active.assetForCacheKey(cacheKey)
  if (reusable) return Promise.resolve(reusable)
  const existing = pending.get(cacheKey)
  if (existing) {
    // A later render that needs the same resource adopts the in-flight work.
    // The queued resolver then remains current instead of rejecting solely
    // because the render which created it has been invalidated.
    existing.revision = revision
    return existing.promise
  }
  const entry: PendingImageAssetRequest = { revision, promise: Promise.resolve({} as ImageAssetData) }
  const request = Promise.resolve().then(() => loader(() => isCurrent(entry.revision))).finally(() => {
    if (pending.get(cacheKey) === entry) pending.delete(cacheKey)
  })
  entry.promise = request
  pending.set(cacheKey, entry)
  return request
}

export function exportImageSource(
  record: PreparedImageResource,
  format: 'html' | 'doc' | 'capture',
): string {
  if (format === 'html' && record.kind === 'public-https') return record.originalSource
  return record.dataURI
}

function base64ToBytes(dataBase64: string): Uint8Array {
  const binary = atob(dataBase64.replace(/\s+/g, ''))
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index)
  return bytes
}
