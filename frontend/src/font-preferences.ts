export type FontScope = 'ui' | 'content' | 'code'

export type FontPresetId =
  | 'system'
  | 'modern-sans'
  | 'reading-serif'
  | 'system-mono'
  | 'fusion-pixel-12-sc'
  | 'fusion-pixel-12-tc'

export interface FontPreferences {
  version: 1
  ui: FontPresetId
  content: FontPresetId
  code: FontPresetId
}

export type FontPreferenceCSSVariable =
  | '--inkmark-ui-font'
  | '--inkmark-content-font'
  | '--inkmark-code-font'

export interface FontPresetOption {
  readonly id: FontPresetId
  readonly labelKey:
    | 'settings.fontSystem'
    | 'settings.fontModernSans'
    | 'settings.fontReadingSerif'
    | 'settings.fontSystemMono'
    | 'settings.fontFusionPixelSimplified'
    | 'settings.fontFusionPixelTraditional'
  readonly category: 'system' | 'sans-serif' | 'serif' | 'monospace' | 'pixel'
  readonly family: string
  readonly scopes: readonly FontScope[]
}

interface FontStyleTarget {
  readonly style: {
    setProperty(name: string, value: string): void
  }
}

interface FontFaceSetTarget {
  readonly ready: PromiseLike<unknown>
  load(font: string, text?: string): PromiseLike<unknown>
}

export interface FontPreferenceStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

export type BundledFontAssetLoader = (url: URL) => Promise<ArrayBuffer | Uint8Array>

export const fontPreferencesStorageKey = 'inkmark-font-preferences-v1'
export const maximumStoredFontPreferencesLength = 512

const systemUIStack = 'system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif'
const modernSansStack = '"Avenir Next", Avenir, "Segoe UI Variable", "Segoe UI", "PingFang SC", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif'
const readingSerifStack = '"Iowan Old Style", Baskerville, "Songti SC", STSong, SimSun, "Times New Roman", serif'
const systemMonoStack = 'ui-monospace, "SFMono-Regular", Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", "PingFang SC", "Microsoft YaHei", monospace'

const allScopes = Object.freeze<FontScope[]>(['ui', 'content', 'code'])
const proseScopes = Object.freeze<FontScope[]>(['ui', 'content'])
const codeScopes = Object.freeze<FontScope[]>(['code'])

export const fontPresetOptions: readonly FontPresetOption[] = Object.freeze([
  Object.freeze({
    id: 'system',
    labelKey: 'settings.fontSystem',
    category: 'system',
    family: systemUIStack,
    scopes: proseScopes,
  }),
  Object.freeze({
    id: 'modern-sans',
    labelKey: 'settings.fontModernSans',
    category: 'sans-serif',
    family: modernSansStack,
    scopes: proseScopes,
  }),
  Object.freeze({
    id: 'reading-serif',
    labelKey: 'settings.fontReadingSerif',
    category: 'serif',
    family: readingSerifStack,
    scopes: proseScopes,
  }),
  Object.freeze({
    id: 'system-mono',
    labelKey: 'settings.fontSystemMono',
    category: 'monospace',
    family: systemMonoStack,
    scopes: codeScopes,
  }),
  Object.freeze({
    id: 'fusion-pixel-12-sc',
    labelKey: 'settings.fontFusionPixelSimplified',
    category: 'pixel',
    family: '"InkMark Fusion Pixel 12 SC", "InkMark Fusion Pixel 12 TC", ' + systemMonoStack,
    scopes: allScopes,
  }),
  Object.freeze({
    id: 'fusion-pixel-12-tc',
    labelKey: 'settings.fontFusionPixelTraditional',
    category: 'pixel',
    family: '"InkMark Fusion Pixel 12 TC", "InkMark Fusion Pixel 12 SC", ' + systemMonoStack,
    scopes: allScopes,
  }),
])

export const defaultFontPreferences: Readonly<FontPreferences> = Object.freeze({
  version: 1,
  ui: 'system',
  content: 'system',
  code: 'system-mono',
})

export const fontPreferenceCSSVariableNames: Readonly<Record<FontScope, FontPreferenceCSSVariable>> = Object.freeze({
  ui: '--inkmark-ui-font',
  content: '--inkmark-content-font',
  code: '--inkmark-code-font',
})

export const fusionPixelFontAssetURLs = Object.freeze({
  simplified: new URL('./assets/fonts/fusion-pixel-12px-monospaced-zh_hans.otf.woff2', import.meta.url),
  traditional: new URL('./assets/fonts/fusion-pixel-12px-monospaced-zh_hant.otf.woff2', import.meta.url),
})

const presetById = new Map(fontPresetOptions.map((preset) => [preset.id, preset]))
const preferenceKeys = Object.freeze(['version', 'ui', 'content', 'code'])
const rejectedObjectKeys = new Set(['__proto__', 'constructor', 'prototype'])

function cloneDefaults(): FontPreferences {
  return { ...defaultFontPreferences }
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return false
  try {
    const prototype = Object.getPrototypeOf(value)
    return prototype === Object.prototype || prototype === null
  } catch {
    return false
  }
}

function migrateLegacyPreset(value: unknown): unknown {
  return value === 'fusion-pixel-12' ? 'fusion-pixel-12-sc' : value
}

function presetSupportsScope(value: unknown, scope: FontScope): value is FontPresetId {
  if (typeof value !== 'string') return false
  return presetById.get(value as FontPresetId)?.scopes.includes(scope) === true
}

function normalizedPreferenceRecord(value: unknown): FontPreferences | null {
  if (!isPlainRecord(value)) return null
  let keys: string[]
  try {
    keys = Object.keys(value)
  } catch {
    return null
  }
  if (keys.length !== preferenceKeys.length) return null
  if (keys.some((key) => rejectedObjectKeys.has(key) || !preferenceKeys.includes(key))) return null
  let version: unknown
  let ui: unknown
  let content: unknown
  let code: unknown
  try {
    version = value.version
    ui = migrateLegacyPreset(value.ui)
    content = migrateLegacyPreset(value.content)
    code = migrateLegacyPreset(value.code)
  } catch {
    return null
  }
  if (
    version !== 1
    || !presetSupportsScope(ui, 'ui')
    || !presetSupportsScope(content, 'content')
    || !presetSupportsScope(code, 'code')
  ) return null
  return { version: 1, ui, content, code }
}

export function getFontPresetsForScope(scope: FontScope): readonly FontPresetOption[] {
  return fontPresetOptions.filter((preset) => preset.scopes.includes(scope))
}

export function normalizeFontPreferences(value: unknown): FontPreferences {
  return normalizedPreferenceRecord(value) || cloneDefaults()
}

export function parseFontPreferences(raw: unknown): FontPreferences {
  if (typeof raw !== 'string' || raw.length === 0 || raw.length > maximumStoredFontPreferencesLength) {
    return cloneDefaults()
  }
  try {
    return normalizeFontPreferences(JSON.parse(raw))
  } catch {
    return cloneDefaults()
  }
}

export function serializeFontPreferences(value: unknown): string {
  const preferences = normalizeFontPreferences(value)
  return JSON.stringify({
    version: 1,
    ui: preferences.ui,
    content: preferences.content,
    code: preferences.code,
  })
}

export function updateFontPreference(
  current: unknown,
  scope: FontScope,
  presetId: unknown,
): FontPreferences {
  const preferences = normalizeFontPreferences(current)
  if (!presetSupportsScope(presetId, scope)) return preferences
  return { ...preferences, [scope]: presetId }
}

export function resetFontPreferences(): FontPreferences {
  return cloneDefaults()
}

export function readFontPreferences(storage: FontPreferenceStorage): FontPreferences {
  try {
    return parseFontPreferences(storage.getItem(fontPreferencesStorageKey))
  } catch {
    return cloneDefaults()
  }
}

export function writeFontPreferences(storage: FontPreferenceStorage, value: unknown): boolean {
  try {
    storage.setItem(fontPreferencesStorageKey, serializeFontPreferences(value))
    return true
  } catch {
    return false
  }
}

export function clearFontPreferences(storage: FontPreferenceStorage): boolean {
  try {
    storage.removeItem(fontPreferencesStorageKey)
    return true
  } catch {
    return false
  }
}

function fusionPixelStack(traditional: boolean): string {
  const first = traditional
    ? '"InkMark Fusion Pixel 12 TC"'
    : '"InkMark Fusion Pixel 12 SC"'
  const second = traditional
    ? '"InkMark Fusion Pixel 12 SC"'
    : '"InkMark Fusion Pixel 12 TC"'
  return `${first}, ${second}, ${systemMonoStack}`
}

export function resolveFontFamily(presetId: FontPresetId, _language?: unknown): string {
  switch (presetId) {
    case 'modern-sans': return modernSansStack
    case 'reading-serif': return readingSerifStack
    case 'system-mono': return systemMonoStack
    case 'fusion-pixel-12-sc': return fusionPixelStack(false)
    case 'fusion-pixel-12-tc': return fusionPixelStack(true)
    case 'system':
    default: return systemUIStack
  }
}

export function fontPreferenceCSSVariables(
  value: unknown,
  language?: unknown,
): Readonly<Record<FontPreferenceCSSVariable, string>> {
  const preferences = normalizeFontPreferences(value)
  return Object.freeze({
    '--inkmark-ui-font': resolveFontFamily(preferences.ui, language),
    '--inkmark-content-font': resolveFontFamily(preferences.content, language),
    '--inkmark-code-font': resolveFontFamily(preferences.code, language),
  })
}

export function applyFontPreferences(
  target: FontStyleTarget,
  value: unknown,
  language?: unknown,
): FontPreferences {
  const preferences = normalizeFontPreferences(value)
  const variables = fontPreferenceCSSVariables(preferences, language)
  for (const name of Object.values(fontPreferenceCSSVariableNames)) {
    target.style.setProperty(name, variables[name])
  }
  return preferences
}

export function usesBundledFont(value: unknown): boolean {
  const preferences = normalizeFontPreferences(value)
  return preferences.ui.startsWith('fusion-pixel-12-')
    || preferences.content.startsWith('fusion-pixel-12-')
    || preferences.code.startsWith('fusion-pixel-12-')
}

export async function waitForSelectedFonts(
  fonts: FontFaceSetTarget | null | undefined,
  value: unknown,
): Promise<void> {
  if (!fonts) return
  const ready = Promise.resolve(fonts.ready).catch(() => undefined)
  const preferences = normalizeFontPreferences(value)
  const selected = new Set([preferences.ui, preferences.content, preferences.code])
  const loads: PromiseLike<unknown>[] = []
  if (selected.has('fusion-pixel-12-sc')) {
    loads.push(fonts.load('12px "InkMark Fusion Pixel 12 SC"', '墨笺 Markdown 简体中文'))
  }
  if (selected.has('fusion-pixel-12-tc')) {
    loads.push(fonts.load('12px "InkMark Fusion Pixel 12 TC"', '墨箋 Markdown 繁體中文'))
  }
  // A corrupt or unavailable optional face must not block editing or export;
  // the fixed stacks still provide platform fallbacks on macOS and Windows.
  await Promise.allSettled(loads)
  // FontFaceSet.ready is normally non-rejecting, but embedded WebViews can be
  // torn down while this task is pending. The caller can continue safely.
  await ready
}

function bytesToBase64(bytes: Uint8Array): string {
  const chunkSize = 0x8000
  let binary = ''
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize))
  }
  return btoa(binary)
}

async function defaultBundledFontAssetLoader(url: URL): Promise<Uint8Array> {
  const response = await fetch(url)
  if (!response.ok) throw new Error(`Unable to load bundled font asset (${response.status})`)
  // Compare against the imported URLs themselves: Vite fingerprints their
  // filenames in production, so matching the source filename would fail only
  // in packaged builds.
  const expectedSize = url.href === fusionPixelFontAssetURLs.simplified.href ? 659_528
    : url.href === fusionPixelFontAssetURLs.traditional.href ? 666_996
      : 0
  if (expectedSize === 0) throw new Error('Unexpected bundled font asset')
  const declaredSizeHeader = response.headers.get('content-length')
  // Fetch exposes a decoded body while preserving compressed HTTP headers, so
  // content-length is comparable only when no content encoding was applied.
  if (declaredSizeHeader !== null && !response.headers.get('content-encoding')) {
    const declaredSize = Number(declaredSizeHeader)
    if (!Number.isSafeInteger(declaredSize) || declaredSize !== expectedSize) {
      throw new Error('Bundled font asset has an unexpected size')
    }
  }
  const bytes = await readFixedLengthResponse(response, expectedSize)
  if (bytes.byteLength !== expectedSize) throw new Error('Bundled font asset has an unexpected size')
  return bytes
}

async function readFixedLengthResponse(response: Response, expectedSize: number): Promise<Uint8Array> {
  if (!response.body) return new Uint8Array(await response.arrayBuffer())
  const reader = response.body.getReader()
  const bytes = new Uint8Array(expectedSize)
  let offset = 0
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      if (offset + value.byteLength > expectedSize) {
        await reader.cancel()
        throw new Error('Bundled font asset exceeds its expected size')
      }
      bytes.set(value, offset)
      offset += value.byteLength
    }
  } finally {
    reader.releaseLock()
  }
  if (offset !== expectedSize) throw new Error('Bundled font asset has an unexpected size')
  return bytes
}

function exportVariableRules(value: unknown, language?: unknown): string {
  const variables = fontPreferenceCSSVariables(value, language)
  return `:root {
${Object.entries(variables).map(([name, family]) => `  ${name}: ${family};`).join('\n')}
}
body.export-document {
  font-family: ${variables['--inkmark-ui-font']};
}
.export-document .markdown-body {
  font-family: ${variables['--inkmark-content-font']};
}
.export-document .markdown-body code,
.export-document .markdown-body kbd,
.export-document .markdown-body pre {
  font-family: ${variables['--inkmark-code-font']};
}`
}

export async function buildEmbeddedFontCSS(
  value: unknown,
  language?: unknown,
  loadAsset: BundledFontAssetLoader = defaultBundledFontAssetLoader,
): Promise<string> {
  const preferences = normalizeFontPreferences(value)
  const variableRules = exportVariableRules(preferences, language)
  if (!usesBundledFont(preferences)) return variableRules

  const [simplified, traditional] = await Promise.all([
    loadAsset(fusionPixelFontAssetURLs.simplified),
    loadAsset(fusionPixelFontAssetURLs.traditional),
  ])
  const simplifiedBytes = simplified instanceof Uint8Array ? simplified : new Uint8Array(simplified)
  const traditionalBytes = traditional instanceof Uint8Array ? traditional : new Uint8Array(traditional)
  if (simplifiedBytes.byteLength === 0 || traditionalBytes.byteLength === 0) {
    throw new Error('Bundled font asset is empty')
  }
  if (simplifiedBytes.byteLength + traditionalBytes.byteLength > 2 * 1024 * 1024) {
    throw new Error('Bundled font assets exceed the export size limit')
  }

  return `@font-face {
  font-family: "InkMark Fusion Pixel 12 SC";
  src: url("data:font/woff2;base64,${bytesToBase64(simplifiedBytes)}") format("woff2");
  font-style: normal;
  font-weight: 400;
  font-display: swap;
}
@font-face {
  font-family: "InkMark Fusion Pixel 12 TC";
  src: url("data:font/woff2;base64,${bytesToBase64(traditionalBytes)}") format("woff2");
  font-style: normal;
  font-weight: 400;
  font-display: swap;
}
${variableRules}`
}
