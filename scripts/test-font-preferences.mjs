import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import {
  applyFontPreferences,
  buildEmbeddedFontCSS,
  clearFontPreferences,
  defaultFontPreferences,
  fontPreferenceCSSVariables,
  fontPreferencesStorageKey,
  getFontPresetsForScope,
  maximumStoredFontPreferencesLength,
  normalizeFontPreferences,
  parseFontPreferences,
  readFontPreferences,
  resetFontPreferences,
  serializeFontPreferences,
  updateFontPreference,
  usesBundledFont,
  waitForSelectedFonts,
  writeFontPreferences,
} from '../frontend/src/font-preferences.ts'

const validPreferences = {
  version: 1,
  ui: 'modern-sans',
  content: 'reading-serif',
  code: 'fusion-pixel-12-tc',
}

function sha256(bytes) {
  return createHash('sha256').update(bytes).digest('hex')
}

test('font catalog exposes only suitable presets for each independent scope', () => {
  assert.deepEqual(
    getFontPresetsForScope('ui').map(({ id }) => id),
    ['system', 'modern-sans', 'reading-serif', 'fusion-pixel-12-sc', 'fusion-pixel-12-tc'],
  )
  assert.deepEqual(
    getFontPresetsForScope('content').map(({ id }) => id),
    ['system', 'modern-sans', 'reading-serif', 'fusion-pixel-12-sc', 'fusion-pixel-12-tc'],
  )
  assert.deepEqual(
    getFontPresetsForScope('code').map(({ id }) => id),
    ['system-mono', 'fusion-pixel-12-sc', 'fusion-pixel-12-tc'],
  )
  assert.deepEqual(defaultFontPreferences, {
    version: 1,
    ui: 'system',
    content: 'system',
    code: 'system-mono',
  })
})

test('stored preferences accept only the exact versioned preset schema', () => {
  const serialized = serializeFontPreferences(validPreferences)
  assert.equal(
    serialized,
    '{"version":1,"ui":"modern-sans","content":"reading-serif","code":"fusion-pixel-12-tc"}',
  )
  assert.deepEqual(parseFontPreferences(serialized), validPreferences)
  assert.deepEqual(parseFontPreferences(
    '{"version":1,"ui":"fusion-pixel-12","content":"fusion-pixel-12","code":"fusion-pixel-12"}',
  ), {
    version: 1,
    ui: 'fusion-pixel-12-sc',
    content: 'fusion-pixel-12-sc',
    code: 'fusion-pixel-12-sc',
  })

  const invalidValues = [
    null,
    undefined,
    '',
    'null',
    '[]',
    '{broken',
    JSON.stringify({ ...validPreferences, version: 2 }),
    JSON.stringify({ ...validPreferences, extra: 'value' }),
    JSON.stringify({ version: 1, ui: 'system', content: 'system' }),
    JSON.stringify({ ...validPreferences, ui: 'system-mono' }),
    JSON.stringify({ ...validPreferences, code: 'reading-serif' }),
    JSON.stringify({ ...validPreferences, ui: 'serif; color:red' }),
    '{"version":1,"ui":"system","content":"system","code":"system-mono","__proto__":{"polluted":true}}',
    'x'.repeat(maximumStoredFontPreferencesLength + 1),
  ]
  for (const value of invalidValues) {
    assert.deepEqual(parseFontPreferences(value), defaultFontPreferences)
  }

  const inherited = Object.assign(Object.create({ ui: 'modern-sans' }), validPreferences)
  assert.deepEqual(normalizeFontPreferences(inherited), defaultFontPreferences)
  const hostileProxy = new Proxy({}, { getPrototypeOf() { throw new Error('hostile') } })
  assert.deepEqual(normalizeFontPreferences(hostileProxy), defaultFontPreferences)
  const hostileValueProxy = new Proxy({ ...validPreferences }, {
    get(target, key, receiver) {
      if (key === 'ui') throw new Error('hostile')
      return Reflect.get(target, key, receiver)
    },
  })
  assert.deepEqual(normalizeFontPreferences(hostileValueProxy), defaultFontPreferences)
  assert.equal({}.polluted, undefined)
})

test('preference updates cannot place an unsupported or arbitrary CSS value in a scope', () => {
  assert.deepEqual(updateFontPreference(validPreferences, 'ui', 'fusion-pixel-12-sc'), {
    ...validPreferences,
    ui: 'fusion-pixel-12-sc',
  })
  assert.deepEqual(updateFontPreference(validPreferences, 'ui', 'system-mono'), validPreferences)
  assert.deepEqual(updateFontPreference(validPreferences, 'code', 'url(https://example.test/font.woff2)'), validPreferences)
  assert.deepEqual(resetFontPreferences(), defaultFontPreferences)
  assert.notEqual(resetFontPreferences(), defaultFontPreferences)
})

test('font stacks cover macOS and Windows and expose explicit simplified/traditional ordering', () => {
  const variables = fontPreferenceCSSVariables(validPreferences, 'zh-CN')
  assert.deepEqual(Object.keys(variables).sort(), [
    '--inkmark-code-font',
    '--inkmark-content-font',
    '--inkmark-ui-font',
  ])
  assert.match(variables['--inkmark-ui-font'], /Avenir Next/)
  assert.match(variables['--inkmark-ui-font'], /Segoe UI/)
  assert.match(variables['--inkmark-ui-font'], /PingFang SC/)
  assert.match(variables['--inkmark-ui-font'], /Microsoft YaHei/)
  assert.match(variables['--inkmark-content-font'], /Iowan Old Style/)
  assert.match(variables['--inkmark-content-font'], /Songti SC/)
  assert.match(variables['--inkmark-content-font'], /SimSun/)

  const simplified = fontPreferenceCSSVariables({
    ...validPreferences,
    code: 'fusion-pixel-12-sc',
  }, 'en')['--inkmark-code-font']
  const traditional = fontPreferenceCSSVariables({
    ...validPreferences,
    code: 'fusion-pixel-12-tc',
  }, 'zh-CN')['--inkmark-code-font']
  assert.ok(simplified.indexOf('12 SC') < simplified.indexOf('12 TC'))
  assert.ok(traditional.indexOf('12 TC') < traditional.indexOf('12 SC'))
  assert.match(simplified, /SFMono-Regular/)
  assert.match(simplified, /Consolas/)
  assert.doesNotMatch(Object.values(variables).join('\n'), /url\(|[{};]/i)
})

test('applying preferences writes exactly three fixed CSS custom properties', () => {
  const writes = []
  const normalized = applyFontPreferences({
    style: {
      setProperty(name, value) { writes.push([name, value]) },
    },
  }, validPreferences, 'zh-TW')
  assert.deepEqual(normalized, validPreferences)
  assert.deepEqual(writes.map(([name]) => name).sort(), [
    '--inkmark-code-font',
    '--inkmark-content-font',
    '--inkmark-ui-font',
  ])
  assert.equal(writes.length, 3)
  assert.ok(writes.every(([, value]) => !value.includes('url(')))
})

test('font loading waits for each selected bundled face before layout reconciliation', async () => {
  const calls = []
  let readyResolved = false
  const fonts = {
    ready: Promise.resolve().then(() => { readyResolved = true }),
    async load(font, text) { calls.push([font, text]); return [] },
  }
  await waitForSelectedFonts(fonts, {
    version: 1,
    ui: 'fusion-pixel-12-sc',
    content: 'fusion-pixel-12-tc',
    code: 'fusion-pixel-12-sc',
  })
  assert.equal(calls.length, 2)
  assert.match(calls[0][0], /12 SC/)
  assert.match(calls[0][1], /简体中文/)
  assert.match(calls[1][0], /12 TC/)
  assert.match(calls[1][1], /繁體中文/)
  assert.equal(readyResolved, true)

  calls.length = 0
  await waitForSelectedFonts(fonts, defaultFontPreferences)
  assert.equal(calls.length, 0)
  await waitForSelectedFonts(undefined, validPreferences)

  await waitForSelectedFonts({
    ready: Promise.reject(new Error('WebView closed')),
    load() { return Promise.reject(new Error('font unavailable')) },
  }, validPreferences)
})

test('storage helpers fail closed without persisting raw values', () => {
  const records = new Map()
  const storage = {
    getItem(key) { return records.get(key) ?? null },
    setItem(key, value) { records.set(key, value) },
    removeItem(key) { records.delete(key) },
  }
  assert.deepEqual(readFontPreferences(storage), defaultFontPreferences)
  assert.equal(writeFontPreferences(storage, validPreferences), true)
  assert.equal(records.get(fontPreferencesStorageKey), serializeFontPreferences(validPreferences))
  assert.deepEqual(readFontPreferences(storage), validPreferences)
  assert.equal(clearFontPreferences(storage), true)
  assert.equal(records.has(fontPreferencesStorageKey), false)

  const unavailable = {
    getItem() { throw new Error('disabled') },
    setItem() { throw new Error('disabled') },
    removeItem() { throw new Error('disabled') },
  }
  assert.deepEqual(readFontPreferences(unavailable), defaultFontPreferences)
  assert.equal(writeFontPreferences(unavailable, validPreferences), false)
  assert.equal(clearFontPreferences(unavailable), false)
})

test('standalone export embeds fixed offline font assets only when selected', async () => {
  let loads = 0
  const systemCSS = await buildEmbeddedFontCSS(defaultFontPreferences, 'en', async () => {
    loads += 1
    return new Uint8Array([1])
  })
  assert.equal(loads, 0)
  assert.doesNotMatch(systemCSS, /@font-face|base64/)
  assert.match(systemCSS, /--inkmark-code-font:/)
  assert.match(systemCSS, /body\.export-document \{[\s\S]*font-family:/)
  assert.match(systemCSS, /\.export-document \.markdown-body \{[\s\S]*font-family:/)
  assert.match(systemCSS, /\.export-document \.markdown-body code,[\s\S]*font-family:/)

  const loadedNames = []
  const embeddedCSS = await buildEmbeddedFontCSS(validPreferences, 'zh-TW', async (url) => {
    loadedNames.push(url.pathname.split('/').pop())
    return new Uint8Array(loadedNames.length === 1 ? [0, 1, 2] : [3, 4, 5])
  })
  assert.equal(usesBundledFont(validPreferences), true)
  assert.equal(usesBundledFont(defaultFontPreferences), false)
  assert.deepEqual(loadedNames.sort(), [
    'fusion-pixel-12px-monospaced-zh_hans.otf.woff2',
    'fusion-pixel-12px-monospaced-zh_hant.otf.woff2',
  ])
  assert.equal((embeddedCSS.match(/@font-face/g) || []).length, 2)
  assert.equal((embeddedCSS.match(/data:font\/woff2;base64,/g) || []).length, 2)
  assert.match(embeddedCSS, /AAEC/)
  assert.match(embeddedCSS, /AwQF/)
  assert.ok(embeddedCSS.indexOf('12 TC') < embeddedCSS.lastIndexOf('12 SC'))
  assert.doesNotMatch(embeddedCSS, /https?:|file:/)

  const realEmbeddedCSS = await buildEmbeddedFontCSS(validPreferences, 'zh-CN', (url) => readFile(url))
  assert.ok(Buffer.byteLength(realEmbeddedCSS) > 1_700_000, 'both real WOFF2 assets must be embedded')
  assert.ok(Buffer.byteLength(realEmbeddedCSS) < 1_900_000, 'embedded font CSS must stay within its audited budget')
  assert.equal((realEmbeddedCSS.match(/data:font\/woff2;base64,/g) || []).length, 2)
  assert.doesNotMatch(realEmbeddedCSS, /(?:src:\s*url\(")(?!data:)/)

  await assert.rejects(
    buildEmbeddedFontCSS(validPreferences, 'zh-CN', async () => new Uint8Array()),
    /font asset is empty/i,
  )

  const originalFetch = globalThis.fetch
  globalThis.fetch = async () => new Response(new Uint8Array([1]), {
    headers: { 'content-length': '1' },
  })
  try {
    await assert.rejects(buildEmbeddedFontCSS(validPreferences), /unexpected size/i)
  } finally {
    globalThis.fetch = originalFetch
  }

  globalThis.fetch = async (url) => new Response(await readFile(url))
  try {
    const withoutLengthHeader = await buildEmbeddedFontCSS(validPreferences)
    assert.ok(Buffer.byteLength(withoutLengthHeader) > 1_700_000)
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('vendored Fusion Pixel assets and license match the audited release snapshot', async () => {
  const assets = {
    '../frontend/src/assets/fonts/fusion-pixel-12px-monospaced-zh_hans.otf.woff2': {
      size: 659_528,
      digest: '7a08aa2bf8970d10bc0f902e9256b86628f17e119efcd9bbcd6c4a169290d867',
    },
    '../frontend/src/assets/fonts/fusion-pixel-12px-monospaced-zh_hant.otf.woff2': {
      size: 666_996,
      digest: 'b42690df31fa8d0aa7b61f1508ddf3416c36c914e8329bd664a8878d29a7056d',
    },
    './licenses/FusionPixelFont-2026.07.20-OFL-1.1.txt': {
      size: 4_418,
      digest: 'bc518cf64b8032c07690f33cc270c35c179255a6ac8efa7c165ebae7e8f76a63',
    },
    './licenses/FusionPixelFont-2026.07.20-ArkPixel-OFL-1.1.txt': {
      size: 4_412,
      digest: '3ab41567e68e3988ba1ef16dd2644eca95ca5648ea12e7d46e6287fc0bbe5aee',
    },
    './licenses/FusionPixelFont-2026.07.20-Cubic11-OFL-1.1.txt': {
      size: 5_363,
      digest: 'bdd640c94530f5845de621089875aefcaec17585dbd4dab191c97118539bf92f',
    },
    './licenses/FusionPixelFont-2026.07.20-Galmuri-OFL-1.1.txt': {
      size: 4_360,
      digest: '86a3ee9495f942f0243f18c103da9faca27adb88142613edb8bb852e56c892c1',
    },
  }
  for (const [path, expected] of Object.entries(assets)) {
    const bytes = await readFile(new URL(path, import.meta.url))
    assert.equal(bytes.byteLength, expected.size, `${path} size`)
    assert.equal(sha256(bytes), expected.digest, `${path} SHA-256`)
  }
  const license = await readFile(new URL('./licenses/FusionPixelFont-2026.07.20-OFL-1.1.txt', import.meta.url), 'utf8')
  assert.match(license, /Fusion Pixel Font/)
  assert.match(license, /Copyright \(c\) 2022, TakWolf/)
  assert.match(license, /SIL OPEN FONT LICENSE Version 1\.1/)
})

test('the font stylesheet is loaded and contains no network source', async () => {
  const main = await readFile(new URL('../frontend/src/main.ts', import.meta.url), 'utf8')
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  const stylesheet = await readFile(new URL('../frontend/src/font-preferences.css', import.meta.url), 'utf8')
  const packageJSON = JSON.parse(await readFile(new URL('../frontend/package.json', import.meta.url), 'utf8'))
  const macBuild = await readFile(new URL('./build-macos.sh', import.meta.url), 'utf8')
  const windowsBuild = await readFile(new URL('./build-windows.ps1', import.meta.url), 'utf8')
  assert.match(main, /import '\.\/font-preferences\.css'/)
  assert.equal((stylesheet.match(/@font-face/g) || []).length, 2)
  assert.match(stylesheet, /fusion-pixel-12px-monospaced-zh_hans\.otf\.woff2/)
  assert.match(stylesheet, /fusion-pixel-12px-monospaced-zh_hant\.otf\.woff2/)
  assert.doesNotMatch(stylesheet, /https?:\/\//)
  assert.match(stylesheet, /font-synthesis: weight style/)
  assert.match(packageJSON.scripts['test:fonts'], /test-font-preferences\.mjs/)
  assert.match(macBuild, /pnpm --dir frontend test:fonts/)
  assert.match(windowsBuild, /pnpm --dir frontend test:fonts/)
  assert.match(app, /readFontPreferences\(localStorage\)/)
  assert.match(app, /writeFontPreferences\(localStorage, nextPreferences\)/)
  assert.match(app, /waitForSelectedFonts\(document\.fonts, nextPreferences\)/)
  assert.match(app, /nextTick\(async \(\) => \{[\s\S]*waitForSelectedFonts\(document\.fonts, nextPreferences\)[\s\S]*\}\)\.catch\(showError\)/)
  assert.match(app, /onMounted\(async \(\) => \{[\s\S]*waitForSelectedFonts\(document\.fonts, fontPreferences\.value\)/)
  assert.match(app, /buildEmbeddedFontCSS\([\s\S]*parseFontPreferences\(snapshot\.fontPreferences\),[\s\S]*snapshot\.language/s)
  assert.match(app, /embeddedStyles: `\$\{collectEmbeddedStyles\(\)\}\\n\$\{embeddedFontCSS\}`/)
  assert.match(app, /locale\.value !== snapshot\.language/)
})
