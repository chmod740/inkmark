import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const root = new URL('../', import.meta.url)
const noticeURL = new URL('../THIRD_PARTY_NOTICES.txt', import.meta.url)
const manifestNames = ['go.mod', 'go.sum', 'frontend/package.json', 'frontend/pnpm-lock.yaml']

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
    'markdown-it-task-lists@2.1.1',
    'mermaid@11.16.1',
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
