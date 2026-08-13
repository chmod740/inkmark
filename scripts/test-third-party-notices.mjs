import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const root = new URL('../', import.meta.url)
const noticeURL = new URL('../THIRD_PARTY_NOTICES.txt', import.meta.url)
const manifestNames = ['go.mod', 'go.sum', 'frontend/package.json', 'frontend/pnpm-lock.yaml']
const pinnedLicenseFiles = {
  'scripts/licenses/viz-js-3.29.0-MIT.txt': 'f1fb91bf7cbcb42b2af56949e4fa909e1979d5b57c2826cabacdb4b631457720',
  'scripts/licenses/Graphviz-15.1.1-EPL-2.0.txt': '8c349f80764d0648e645f41ef23772a70c995a0924b5235f735f4a3d09df127c',
  'scripts/licenses/Expat-2.8.1-MIT.txt': '31b15de82aa19a845156169a17a5488bf597e561b2c318d159ed583139b25e87',
  'scripts/licenses/FusionPixelFont-2026.07.20-OFL-1.1.txt': 'bc518cf64b8032c07690f33cc270c35c179255a6ac8efa7c165ebae7e8f76a63',
  'scripts/licenses/FusionPixelFont-2026.07.20-ArkPixel-OFL-1.1.txt': '3ab41567e68e3988ba1ef16dd2644eca95ca5648ea12e7d46e6287fc0bbe5aee',
  'scripts/licenses/FusionPixelFont-2026.07.20-Cubic11-OFL-1.1.txt': 'bdd640c94530f5845de621089875aefcaec17585dbd4dab191c97118539bf92f',
  'scripts/licenses/FusionPixelFont-2026.07.20-Galmuri-OFL-1.1.txt': '86a3ee9495f942f0243f18c103da9faca27adb88142613edb8bb852e56c892c1',
}

function sha256(bytes) {
  return createHash('sha256').update(bytes).digest('hex')
}

test('third-party notice matches the release dependency manifests', async () => {
  const notice = await readFile(noticeURL, 'utf8')
  assert.ok(Buffer.byteLength(notice) > 100_000, 'the complete notice must include full license texts')
  assert.match(notice, /does not grant a license to any\s+InkMark-authored source code/i)
  assert.match(notice, /licensing status of the InkMark project itself is separate/i)

  for (const name of manifestNames) {
    const bytes = await readFile(new URL(name, root))
    assert.ok(notice.includes(`- ${name} SHA-256 ${sha256(bytes)}`), `${name} fingerprint must be current`)
  }
})

test('notice covers platform binaries, frontend runtime dependencies, and KaTeX fonts', async () => {
  const notice = await readFile(noticeURL, 'utf8')
  const required = [
    'github.com/wailsapp/wails/v2@v2.13.0',
    'github.com/wailsapp/go-webview2@v1.0.22',
    'webviewloader/LICENSE',
    'github.com/danieljoos/wincred@v1.2.3',
    'github.com/zalando/go-keyring@v0.2.8',
    '@mdit/plugin-katex@1.0.2',
    'dompurify@3.4.13',
    'highlight.js@11.11.1',
    'html2canvas@1.4.1',
    'jspdf@4.2.1',
    'katex@0.18.1',
    'markdown-it@15.0.0',
    'markdown-it-abbr@2.0.0',
    'markdown-it-deflist@4.0.0',
    'markdown-it-emoji@3.1.0',
    'markdown-it-footnote@4.0.0',
    'markdown-it-mark@4.0.0',
    'markdown-it-sub@2.0.0',
    'markdown-it-sup@2.0.0',
    'markdown-it-task-lists@2.1.1',
    'mermaid@11.16.1',
    'echarts@6.1.0',
    'echarts@6.1.0 (licenses/LICENSE-d3)',
    'Copyright 2010-2016 Mike Bostock',
    'abcjs@6.7.0',
    '@viz-js/viz@3.29.0 (MIT; embeds Graphviz 15.1.1 (EPL-2.0) and Expat 2.8.1 (MIT)',
    '@viz-js/viz@3.29.0 / embedded Graphviz@15.1.1',
    '@viz-js/viz@3.29.0 / embedded Expat@2.8.1',
    'graphviz-releases/15.1.1/graphviz-15.1.1.tar.gz',
    'libexpat/releases/download/R_2_8_1/expat-2.8.1.tar.gz',
    'Eclipse Public License - v 2.0',
    'Copyright (c) Michael Daines',
    'Copyright (c) 1998-2000 Thai Open Source Software Center Ltd and Clark Cooper',
    'vue@3.5.40',
    'khroma@2.1.0',
    'pako@2.2.0 (lib/zlib/README)',
    '(C) 1995-2013 Jean-loup Gailly and Mark Adler',
    'SIL OPEN FONT LICENSE Version 1.1 - 26 February 2007',
    'Copyright (c) 2009-2010, Design Science, Inc.',
    'Copyright (c) 2014-2018 Khan Academy',
    'Fusion Pixel Font 12px Monospaced 2026.07.20 (Simplified and Traditional Chinese)',
    'fusion-pixel-font-12px-monospaced-otf.woff2-v2026.07.20.zip SHA-256 83ff3c1fa8dd0ecd1b010c4bb256a7318ca4fb2dcaadbfdfc0ea560da219a565',
    'fusion-pixel-12px-monospaced-zh_hans.otf.woff2 (659528 bytes; SHA-256 7a08aa2bf8970d10bc0f902e9256b86628f17e119efcd9bbcd6c4a169290d867)',
    'fusion-pixel-12px-monospaced-zh_hant.otf.woff2 (666996 bytes; SHA-256 b42690df31fa8d0aa7b61f1508ddf3416c36c914e8329bd664a8878d29a7056d)',
    'Copyright (c) 2022, TakWolf',
    'Copyright (c) 2021, TakWolf',
    '[Cubic 11]',
    'Copyright (c) 2019–2025 Lee Minseo',
  ]
  for (const marker of required) assert.ok(notice.includes(marker), `${marker} must be covered`)
  assert.ok((notice.match(/^License text \d+/gm) || []).length >= 50, 'full component license texts must be present')
  assert.doesNotMatch(notice, /\/Users\/|[A-Z]:\\Users\\/, 'notice must not expose build-machine paths')
})

test('manually pinned WASM and font licenses match their audited upstream snapshots', async () => {
  for (const [name, expected] of Object.entries(pinnedLicenseFiles)) {
    const bytes = await readFile(new URL(name, root))
    assert.equal(sha256(bytes), expected, `${name} must stay identical to its audited repository snapshot`)
  }
})
