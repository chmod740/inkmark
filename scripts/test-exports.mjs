import assert from 'node:assert/strict'
import test from 'node:test'

import {
  buildStandaloneHTML,
  externalExportStyles,
  normalizeProtocolRelativeResources,
  utf8ToBase64,
} from '../frontend/src/export-document.ts'

test('standalone HTML preserves external links and resources', () => {
  const articleHTML = '<h1>测试</h1><p><a href="https://example.com/docs">外链</a></p><img src="https://images.example.com/demo.png" alt="demo">'
  const html = buildStandaloneHTML({
    title: '报告 <测试>',
    theme: 'github',
    articleHTML,
    embeddedStyles: '.markdown-body { color: #123; }',
  })

  assert.match(html, /^<!doctype html>/)
  assert.match(html, /<title>报告 &lt;测试&gt;<\/title>/)
  assert.ok(html.includes('href="https://example.com/docs"'))
  assert.ok(html.includes('src="https://images.example.com/demo.png"'))
  assert.ok(html.includes(`href="${externalExportStyles.katex}"`))
  assert.ok(html.includes(`href="${externalExportStyles.highlightLight}"`))
  assert.match(html, /style-src 'unsafe-inline' https:/)
  assert.match(html, /img-src [^";]*http: https:/)
  assert.match(html, /body class="export-document theme-github"/)
})

test('protocol-relative resources are normalised to HTTPS for local HTML files', () => {
  const articleHTML = '<a href="//docs.example.com/guide">文档</a><img src="//cdn.example.com/demo.png" srcset="//cdn.example.com/1x.png 1x, //cdn.example.com/2x.png 2x"><video poster="//cdn.example.com/poster.jpg"></video><code>src="//cdn.example.com/example.png"</code>'
  const html = buildStandaloneHTML({
    title: 'Resources',
    theme: 'clean',
    articleHTML,
    embeddedStyles: '',
  })

  assert.ok(html.includes('href="https://docs.example.com/guide"'))
  assert.ok(html.includes('src="https://cdn.example.com/demo.png"'))
  assert.ok(html.includes('srcset="https://cdn.example.com/1x.png 1x, https://cdn.example.com/2x.png 2x"'))
  assert.ok(html.includes('poster="https://cdn.example.com/poster.jpg"'))
  assert.ok(html.includes('<code>src="//cdn.example.com/example.png"</code>'))
  assert.equal(normalizeProtocolRelativeResources('<img src="/local/image.png">'), '<img src="/local/image.png">')
})

test('dark HTML selects the matching external highlight theme', () => {
  const html = buildStandaloneHTML({
    title: 'Dark',
    theme: 'dark',
    articleHTML: '<pre><code class="hljs">const answer = 42</code></pre>',
    embeddedStyles: '',
  })
  assert.ok(html.includes(`href="${externalExportStyles.highlightDark}"`))
  assert.match(html, /data-color-scheme="dark"/)
})

test('standalone HTML declares the selected interface language', () => {
  const chinese = buildStandaloneHTML({
    title: '中文',
    theme: 'github',
    articleHTML: '<p>内容</p>',
    embeddedStyles: '',
    language: 'zh-CN',
  })
  const english = buildStandaloneHTML({
    title: 'English',
    theme: 'github',
    articleHTML: '<p>Content</p>',
    embeddedStyles: '',
    language: 'en',
  })
  assert.match(chinese, /<html[^>]* lang="zh-CN"/)
  assert.match(english, /<html[^>]* lang="en"/)
  assert.match(english, /name="generator" content="InkMark Markdown"/)
})

test('Word-compatible export declares Office namespaces and UTF-8 BOM', () => {
  const html = buildStandaloneHTML({
    title: 'Word',
    theme: 'wechat',
    articleHTML: '<p>中文内容</p>',
    embeddedStyles: '',
    wordCompatible: true,
  })
  assert.ok(html.startsWith('\uFEFF<!doctype html>'))
  assert.match(html, /xmlns:w="urn:schemas-microsoft-com:office:word"/)
  assert.match(html, /name="ProgId" content="Word.Document"/)
})

test('embedded style closing tags cannot escape the style element', () => {
  const html = buildStandaloneHTML({
    title: 'Safe styles',
    theme: 'clean',
    articleHTML: '<p>content</p>',
    embeddedStyles: 'p::after { content: "</style><script>bad()</script>"; }',
  })
  assert.ok(!html.includes('</style><script>bad()</script>'))
  assert.ok(html.includes('<\\/style><script>bad()</script>'))
})

test('UTF-8 text is encoded without corrupting Chinese', () => {
  const text = '墨笺 Markdown：你好'
  const encoded = utf8ToBase64(text)
  assert.equal(Buffer.from(encoded, 'base64').toString('utf8'), text)
})
