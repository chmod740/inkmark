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
    'markdown-it-emoji@3.1.0',
    'markdown-it-footnote@4.0.0',
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
  ]
  for (const marker of required) assert.ok(notice.includes(marker), `${marker} must be covered`)
  assert.ok((notice.match(/^License text \d+/gm) || []).length >= 50, 'full component license texts must be present')
  assert.doesNotMatch(notice, /\/Users\/|[A-Z]:\\Users\\/, 'notice must not expose build-machine paths')
})

test('manually pinned WASM license texts match their audited upstream snapshots', async () => {
  for (const [name, expected] of Object.entries(pinnedLicenseFiles)) {
    const bytes = await readFile(new URL(name, root))
    assert.equal(sha256(bytes), expected, `${name} must stay identical to its audited repository snapshot`)
  }
})
