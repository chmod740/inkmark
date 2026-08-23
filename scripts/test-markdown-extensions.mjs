import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import test from 'node:test'

const frontendRequire = createRequire(new URL('../frontend/package.json', import.meta.url))
const MarkdownIt = frontendRequire('markdown-it')

import {
  classifyExtendedFence,
  convertIndentedBulletMindmap,
  extendedFenceKind,
  installMarkdownExtensions,
  maximumExtendedFenceCharacters,
  maximumMindmapCharacters,
  maximumMindmapDepth,
  maximumMindmapNodes,
  parseCalloutMarker,
  segmentMentions,
} from '../frontend/src/markdown-extensions.ts'

test('full emoji and footnotes render representative syntax from 未命名.md', () => {
  const markdown = installMarkdownExtensions(new MarkdownIt())
  const html = markdown.render([
    '你好 :smile: :+1: :not_an_inkmark_emoji:',
    '',
    '代码中的短码保持原样：`:smile:`。',
    '',
    '正文脚注[^1]和长脚注[^bignote]。',
    '',
    '[^1]: 简短说明',
    '[^bignote]: 第一段',
    '',
    '    第二段',
    '',
    '    ```',
    '    可以使用代码块。',
    '    ```',
    '',
    '    还支持**加粗**和[链接](https://example.com)。',
  ].join('\n'))

  assert.match(html, /你好 😄 👍 :not_an_inkmark_emoji:/)
  assert.match(html, /class="footnote-ref"/)
  assert.match(html, /class="footnotes"/)
  assert.match(html, /简短说明/)
  assert.match(html, /第一段/)
  assert.match(html, /第二段/)
  assert.match(html, /<code>:smile:<\/code>/)
  assert.match(html, /<pre><code>可以使用代码块。\n<\/code><\/pre>/)
  assert.match(html, /<strong>加粗<\/strong>/)
  assert.match(html, /href="https:\/\/example\.com"/)
})

test('generated footnote fragment owners carry the per-render trusted marker', () => {
  const markdown = installMarkdownExtensions(new MarkdownIt())
  const marker = 'Trusted_Footnote-1234567890'
  const html = markdown.render('First[^one], repeated[^one].\n\n[^one]: Footnote.', {
    inkmarkTrustedMarker: marker,
  })
  assert.match(html, new RegExp(`<a data-inkmark-trusted="${marker}" href="#fn1" id="fnref1">`))
  assert.match(html, new RegExp(`<a data-inkmark-trusted="${marker}" href="#fn1" id="fnref1:1">`))
  assert.match(html, new RegExp(`<li data-inkmark-trusted="${marker}" id="fn1" class="footnote-item">`))
  const invalid = markdown.render('Text[^one].\n\n[^one]: Footnote.', {
    inkmarkTrustedMarker: 'bad" onclick="attack',
  })
  assert.doesNotMatch(invalid, /data-inkmark-trusted|onclick=/)
})

test('callout markers accept GitHub and legacy forms without prefix confusion', () => {
  assert.deepEqual(parseCalloutMarker('[!NOTE] 后续内容'), {
    type: 'note',
    marker: '[!NOTE] ',
    content: '后续内容',
  })
  assert.deepEqual(parseCalloutMarker('  Tip: useful'), {
    type: 'tip',
    marker: '  Tip: ',
    content: 'useful',
  })
  assert.deepEqual(parseCalloutMarker('NOTE:'), {
    type: 'note',
    marker: 'NOTE:',
    content: '',
  })
  assert.deepEqual(parseCalloutMarker('[!WARNING]\n下一行'), {
    type: 'warning',
    marker: '[!WARNING]\n',
    content: '下一行',
  })
  assert.deepEqual(parseCalloutMarker('Tip:\r\nNext line'), {
    type: 'tip',
    marker: 'Tip:\r\n',
    content: 'Next line',
  })
  assert.equal(parseCalloutMarker('NOTEBOOK: not a callout'), null)
  assert.equal(parseCalloutMarker('[!NOTEBOOK] not a callout'), null)
  assert.equal(parseCalloutMarker('text NOTE: not at the start'), null)
  assert.equal(parseCalloutMarker('[!NOTE]<script>'), null)
})

test('mentions are segmented as inert data and avoid email or illegal boundaries', () => {
  assert.deepEqual(segmentMentions('Hi @Vanessa and @user_2.'), [
    { kind: 'text', value: 'Hi ' },
    { kind: 'mention', value: '@Vanessa', username: 'Vanessa' },
    { kind: 'text', value: ' and ' },
    { kind: 'mention', value: '@user_2', username: 'user_2' },
    { kind: 'text', value: '.' },
  ])
  assert.deepEqual(segmentMentions('mail@example.com @@admin foo@bar /@route @bad-'), [
    { kind: 'text', value: 'mail@example.com @@admin foo@bar /@route @bad-' },
  ])
  assert.deepEqual(segmentMentions('@a <script>@owner</script>'), [
    { kind: 'mention', value: '@a', username: 'a' },
    { kind: 'text', value: ' <script>' },
    { kind: 'mention', value: '@owner', username: 'owner' },
    { kind: 'text', value: '</script>' },
  ])
})

test('two-space bullet mindmap conversion preserves hierarchy and escapes labels', () => {
  const source = [
    '- 教程',
    '- 语法指导',
    '  - 普通内容',
    '  - 提及用户 @Vanessa',
    '    - 表情符号 Emoji',
    '      - Heading 4',
    '- <script> & "quoted" [node] `tick`',
  ].join('\n')
  const converted = convertIndentedBulletMindmap(source)

  assert.equal(converted, [
    'mindmap',
    '  root(("Mindmap"))',
    '    item1["教程"]',
    '    item2["语法指导"]',
    '      item3["普通内容"]',
    '      item4["提及用户 @Vanessa"]',
    '        item5["表情符号 Emoji"]',
    '          item6["Heading 4"]',
    '    item7["&lt;script&gt; &amp; &quot;quoted&quot; &#91;node&#93; &#96;tick&#96;"]',
  ].join('\n'))
  assert.ok(!converted?.includes('<script>'))
})

test('malformed or unbounded bullet mindmaps fail closed', () => {
  assert.equal(convertIndentedBulletMindmap(' - odd indentation'), null)
  assert.equal(convertIndentedBulletMindmap('- root\n    - skipped level'), null)
  assert.equal(convertIndentedBulletMindmap('- root\n\t- tabbed'), null)
  assert.equal(convertIndentedBulletMindmap('- root\nclick attacker "javascript:bad"'), null)
  assert.equal(convertIndentedBulletMindmap(' '.repeat(maximumMindmapCharacters + 1)), null)
  assert.equal(convertIndentedBulletMindmap(Array.from({ length: maximumMindmapNodes + 1 }, (_, index) => `- n${index}`).join('\n')), null)
  assert.equal(convertIndentedBulletMindmap(`${'  '.repeat(maximumMindmapDepth + 1)}- deep`), null)
})

test('extended visual fences use canonical kinds and enforce a hard size bound', () => {
  assert.equal(extendedFenceKind('MERMAID'), 'mermaid')
  assert.equal(extendedFenceKind('mmd title=test'), 'mermaid')
  assert.equal(extendedFenceKind('mindmap'), 'mindmap')
  assert.equal(extendedFenceKind('echarts'), 'echarts')
  assert.equal(extendedFenceKind('abc'), 'abc')
  assert.equal(extendedFenceKind('graphviz'), 'graphviz')
  assert.equal(extendedFenceKind('dot strict'), 'graphviz')
  assert.equal(extendedFenceKind('mermaid<script>'), null)
  assert.equal(extendedFenceKind('javascript'), null)
  assert.equal(extendedFenceKind('constructor'), null)
  assert.equal(extendedFenceKind('__proto__'), null)

  assert.deepEqual(classifyExtendedFence('graphviz', 'digraph { a -> b }'), {
    status: 'supported',
    kind: 'graphviz',
    source: 'digraph { a -> b }',
  })
  assert.deepEqual(classifyExtendedFence('unknown', 'x'), { status: 'unsupported' })
  assert.deepEqual(classifyExtendedFence('echarts', 'x'.repeat(maximumExtendedFenceCharacters + 1)), {
    status: 'too-large',
    kind: 'echarts',
    maximumCharacters: maximumExtendedFenceCharacters,
  })
  assert.equal(classifyExtendedFence('abc', 'abc', -1).status, 'supported')
})

test('App integrates extensions without turning mentions or visual fences into active links', async () => {
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')

  assert.match(app, /installMarkdownExtensions\(markdown\)/)
  assert.match(app, /classifyExtendedFence\(token\.info, token\.content\)/)
  assert.match(app, /convertBulletMindmap\(classification\.source\)/)
  assert.match(app, /class="mermaid" data-kind="mindmap"/)
  assert.match(app, /class="extended-diagram" data-kind="\$\{classification\.kind\}" data-extended-diagram="\$\{classification\.kind\}"/)
  assert.match(app, /'data-source-line', 'data-kind'/)
  assert.match(app, /parseCalloutMarker\(first\.textContent \|\| ''\)/)
  assert.match(app, /removeLeadingText\(first, marker\.marker\.length, \/\\r\?\\n\/u\.test\(marker\.marker\)\)/)
  assert.match(app, /root\.firstChild instanceof HTMLBRElement[\s\S]*root\.firstChild\.remove\(\)/)
  assert.doesNotMatch(app, /first\.textContent\s*=/)
  assert.match(app, /closest\('a, code, pre, style, script, textarea, \.markdown-mention'\)/)
  assert.match(app, /mention\.textContent = segment\.value/)
  assert.doesNotMatch(app, /mention\.href/)
  assert.match(app, /href\.startsWith\('#'\)[\s\S]*scrollToPreviewAnchor/)
  assert.match(app, /destination\.scrollIntoView\(\{ block: 'start' \}\)/)
  assert.match(app, /OpenExternal\(href\)/)
  assert.match(app, /renderExtendedDiagrams\(root, \{[\s\S]*isCurrent: \(\) => previewCommit\.isCurrent\(sequence\)/)
  assert.match(app, /activeExtendedDiagramDispose\?\.\(\)/)
  assert.match(app, /fitTallDiagramsForPDF\(clone, captureWidth\)/)
  assert.match(app, /\.mermaid-rendered svg, \.extended-diagram-rendered svg/)
})

test('styles and bilingual sample cover every non-media extension', async () => {
  const [styles, sample, packageJSON, macBuild, windowsBuild] = await Promise.all([
    readFile(new URL('../frontend/src/styles.css', import.meta.url), 'utf8'),
    readFile(new URL('../samples/markdown-rendering-test.md', import.meta.url), 'utf8'),
    readFile(new URL('../frontend/package.json', import.meta.url), 'utf8'),
    readFile(new URL('./build-macos.sh', import.meta.url), 'utf8'),
    readFile(new URL('./build-windows.ps1', import.meta.url), 'utf8'),
  ])

  assert.match(styles, /\.markdown-mention\s*\{/)
  assert.match(styles, /\.markdown-body \.footnotes\s*\{/)
  assert.match(styles, /pre\.extended-diagram\s*\{/)
  assert.match(styles, /\.theme-dark \.markdown-mention/)
  assert.match(styles, /\.theme-dark \.markdown-body \.footnotes/)
  assert.match(styles, /\.theme-dark \.markdown-body pre\.extended-diagram/)
  assert.match(styles, /\.theme-dark \.markdown-body pre\.extended-diagram-rendered[^}]*background:\s*#f6f8fa/)

  assert.match(sample, /:smile: :\+1: :rocket: :tada:/)
  assert.match(sample, /> NOTE: .*\*\*粗体\*\*.*`inline code`.*\[外部链接\]/)
  assert.match(sample, /> Tip: .*\*\*strong text\*\*.*`code`.*\[external link\]/)
  assert.match(sample, /脚注支持简短说明\[\^short-note\]/)
  assert.match(sample, /~~~mindmap\n- 教程 \/ Tutorials[\s\S]*- 快捷键 \/ Shortcuts\n/)
  assert.match(sample, /~~~echarts\n\{[\s\S]*"series"/)
  assert.match(sample, /~~~abc\nX:1[\s\S]*K:C/)
  assert.match(sample, /~~~graphviz\ndigraph InkMark/)
  assert.doesNotMatch(sample, /!\[[^\]]*\]\([^)]*\)/)

  const scripts = JSON.parse(packageJSON).scripts
  assert.equal(scripts['test:markdown'], 'node --experimental-strip-types --test ../scripts/test-markdown-extensions.mjs')
  assert.equal(scripts['test:diagrams'], 'node --experimental-strip-types --test ../scripts/test-extended-diagrams.mjs')
  assert.match(macBuild, /pnpm --dir frontend test:markdown/)
  assert.match(macBuild, /pnpm --dir frontend test:diagrams/)
  assert.match(windowsBuild, /pnpm --dir frontend test:markdown[\s\S]*Run Markdown extension tests/)
  assert.match(windowsBuild, /pnpm --dir frontend test:diagrams[\s\S]*Run extended diagram safety tests/)
})
