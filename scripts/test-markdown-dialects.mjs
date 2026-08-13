import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { performance } from 'node:perf_hooks'
import test from 'node:test'

const frontendRequire = createRequire(new URL('../frontend/package.json', import.meta.url))
const MarkdownIt = frontendRequire('markdown-it')
const { katex: markdownKatex } = frontendRequire('@mdit/plugin-katex')

import {
  createHeadingIdAllocator,
  installMarkdownExtensions,
  maximumAbbreviationLabelCharacters,
  maximumAbbreviations,
  maximumCitationCandidates,
  maximumCitations,
  maximumCitationDefinitions,
  maximumFrontMatterCharacters,
  maximumFrontMatterLines,
  maximumWikiLinks,
  maximumWikiLinkCandidates,
} from '../frontend/src/markdown-extensions.ts'

function dialectMarkdown(options = {}) {
  return installMarkdownExtensions(new MarkdownIt({ html: true, ...options }))
}

test('definition lists, abbreviations, mark, subscript, and superscript use established markdown-it syntax', () => {
  const html = dialectMarkdown().render([
    '*[HTML]: Hyper Text Markup Language',
    '',
    'HTML and ==highlighted== H~2~O x^2^. Code: `==not mark==`.',
    '',
    'InkMark',
    ': A Markdown editor.',
    ': A second definition with **inline markup**.',
  ].join('\n'))

  assert.match(html, /<abbr title="Hyper Text Markup Language">HTML<\/abbr>/)
  assert.match(html, /<mark>highlighted<\/mark>/)
  assert.match(html, /H<sub>2<\/sub>O x<sup>2<\/sup>/)
  assert.match(html, /<dl>[\s\S]*<dt>InkMark<\/dt>[\s\S]*<dd>A Markdown editor\.<\/dd>/)
  assert.match(html, /<strong>inline markup<\/strong>/)
  assert.match(html, /<code>==not mark==<\/code>/)
})

test('dialect syntax stays inert in code, URLs, raw attributes, and KaTeX', () => {
  const markdown = dialectMarkdown().use(markdownKatex, { delimiters: 'all', throwOnError: false })
  const html = markdown.render([
    '*[HTML]: Hyper Text Markup Language',
    '',
    '`HTML ==code== H~2~O x^2^ [[Wiki]] [@ref]`',
    '',
    '```text',
    'HTML ==fence== H~2~O x^2^ [[Wiki]] [@ref]',
    '```',
    '',
    '[safe](https://example.test/==path==?q=H~2~O)',
    '',
    '<span title="HTML ==raw== H~2~ [[Wiki]] [@ref]">raw</span>',
    '',
    'Math $x^{2} + H_{2} + \\text{[[Wiki]] [@ref]}$.',
    '',
    '[@ref]: Reference.',
  ].join('\n'))
  assert.match(html, /<code>HTML ==code== H~2~O x\^2\^ \[\[Wiki\]\] \[@ref\]<\/code>/)
  assert.match(html, /<pre><code class="language-text">HTML ==fence== H~2~O x\^2\^ \[\[Wiki\]\] \[@ref\]/)
  assert.match(html, /href="https:\/\/example\.test\/==path==\?q=H~2~O"/)
  assert.match(html, /title="HTML ==raw== H~2~ \[\[Wiki\]\] \[@ref\]"/)
  assert.match(html, /<span class="katex"/)
  assert.doesNotMatch(html, /katex-error/)
  assert.doesNotMatch(html, /inkmark-wiki-link|inkmark-citation-unresolved/)
})

test('abbreviation definitions fail closed before the replacement regular expression exceeds its budget', () => {
  const tooMany = Array.from({ length: maximumAbbreviations + 1 }, (_, index) => `*[A${index}]: title ${index}`)
  const html = dialectMarkdown().render([...tooMany, '', 'A0 A256'].join('\n'))
  assert.doesNotMatch(html, /<abbr/)
  assert.match(html, /<p>A0 A256<\/p>/)

  const oversizedLabel = 'A'.repeat(maximumAbbreviationLabelCharacters + 1)
  const oversized = dialectMarkdown().render(`*[${oversizedLabel}]: title\n\n${oversizedLabel}`)
  assert.doesNotMatch(oversized, /<abbr/)
})

test('heading attributes and TOC share deterministic, collision-safe IDs', () => {
  const html = dialectMarkdown().render([
    '[TOC]',
    '',
    '# Introduction {#intro .lead .wide}',
    '## Duplicate {#intro .secondary}',
    '### Reserved {#fn1 .forbidden}',
    '#### Generated heading',
    '#### Emoji :smile:',
    '##### Invalid {#safe onclick=bad}',
  ].join('\n'))

  assert.match(html, /<nav class="inkmark-toc"[^>]*data-source-line="0">/)
  assert.match(html, /<h2>目录 \/ Contents<\/h2>/)
  assert.match(html, /href="#intro">Introduction<\/a>/)
  assert.match(html, /href="#intro-2">Duplicate<\/a>/)
  assert.match(html, /<h1 class="user-heading-lead user-heading-wide" id="intro">Introduction<\/h1>/)
  assert.match(html, /<h2 class="user-heading-secondary" id="intro-2">Duplicate<\/h2>/)
  assert.match(html, /<h3 id="inkmark-heading-reserved-fn1-forbidden">Reserved \{#fn1 \.forbidden\}<\/h3>/)
  assert.doesNotMatch(html, /class="[^"]*forbidden/)
  assert.match(html, /<h4 id="inkmark-heading-generated-heading">Generated heading<\/h4>/)
  assert.match(html, /href="#inkmark-heading-emoji-smile">Emoji 😄<\/a>/)
  assert.match(html, /<h4 id="inkmark-heading-emoji-smile">Emoji 😄<\/h4>/)
  assert.match(html, /Invalid \{#safe onclick=bad\}/)
  assert.doesNotMatch(html, /<h5[^>]*\sonclick=/i)

  const allocator = createHeadingIdAllocator(['existing'])
  assert.equal(allocator.allocateExplicit('existing'), 'existing-2')
  assert.equal(allocator.allocateExplicit('wiki-target'), null)
  assert.equal(allocator.allocateGenerated('你好，世界'), 'inkmark-heading-你好-世界')
})

test('TOC only replaces a standalone marker and bounds the generated entry count', () => {
  const headings = Array.from({ length: 300 }, (_, index) => `## Heading ${index + 1}`).join('\n')
  const html = dialectMarkdown().render(`[toc]\n\nText [TOC] stays.\n\n${headings}`)
  assert.equal((html.match(/<nav class="inkmark-toc"/g) || []).length, 1)
  assert.match(html, /Text \[TOC\] stays\./)
  assert.equal((html.match(/class="inkmark-toc-level-/g) || []).length, 256)
})

test('Wiki syntax renders an inert unresolved marker and never creates a resource link', () => {
  const html = dialectMarkdown().render([
    '[[Knowledge Base|知识库]] [[Simple]]',
    '',
    'Unsafe forms stay literal: [[https://example.com|remote]] [[../secret]]',
    '',
    '[outer [[Nested]]](https://example.com)',
  ].join('\n'))

  assert.match(html, /<span class="inkmark-wiki-link inkmark-wiki-link-unresolved" data-wiki-target="Knowledge Base" aria-disabled="true">知识库<\/span>/)
  assert.match(html, /data-wiki-target="Simple"[^>]*>Simple<\/span>/)
  assert.doesNotMatch(html, /href="#wiki-/)
  assert.match(html, /\[\[https:\/\/example\.com\|remote\]\]/)
  assert.match(html, /\[\[\.\.\/secret\]\]/)
  assert.match(html, /<a href="https:\/\/example\.com">outer \[\[Nested\]\]<\/a>/)

  const many = Array.from({ length: maximumWikiLinks + 1 }, (_, index) => `[[Page ${index}]]`).join(' ')
  const bounded = dialectMarkdown().render(many)
  assert.equal((bounded.match(/class="inkmark-wiki-link /g) || []).length, maximumWikiLinks)
  assert.match(bounded, /\[\[Page 256\]\]/)
})

test('front matter is a bounded, inert top-of-document flat map', () => {
  const environment = {}
  const html = dialectMarkdown().render([
    '\uFEFF---',
    'title: "InkMark <script>alert(1)</script>"',
    'draft: false',
    'priority: 2.5',
    'empty: null',
    '---',
    '# Document',
  ].join('\n'), environment)

  assert.match(html, /<section class="inkmark-front-matter"[^>]*data-source-line="0">/)
  assert.match(html, /<h2>Front Matter \/ 文档元数据<\/h2>/)
  assert.match(html, /InkMark &lt;script&gt;alert\(1\)&lt;\/script&gt;/)
  assert.doesNotMatch(html, /<script>/)
  assert.equal(Object.getPrototypeOf(environment.inkmarkFrontMatter), null)
  assert.deepEqual(environment.inkmarkFrontMatter.draft, { raw: 'false', type: 'boolean', value: false })
  assert.deepEqual(environment.inkmarkFrontMatter.priority, { raw: '2.5', type: 'number', value: 2.5 })
  assert.deepEqual(environment.inkmarkFrontMatter.empty, { raw: 'null', type: 'null', value: null })

  dialectMarkdown().render('# no metadata', environment)
  assert.equal(environment.inkmarkFrontMatter, undefined)
})

test('unclosed, nested, executable, duplicate, or oversized front matter stays ordinary Markdown', () => {
  const invalidDocuments = [
    '---\ntitle: unclosed',
    '---\n__proto__: bad\n---',
    '---\ntitle: one\ntitle: two\n---',
    '---\ntags: [one, two]\n---',
    '---\nload: !include remote.yml\n---',
    '---\ntitle: "unsafe\\u0000value"\n---',
    `---\nvalue: ${'x'.repeat(maximumFrontMatterCharacters)}\n---`,
    `---\n${Array.from({ length: maximumFrontMatterLines }, (_, index) => `key${index}: value`).join('\n')}\n---`,
    '> ---\n> title: nested\n> ---',
  ]
  invalidDocuments.forEach((source) => {
    const environment = {}
    const html = dialectMarkdown().render(source, environment)
    assert.doesNotMatch(html, /class="inkmark-front-matter"/)
    assert.equal(environment.inkmarkFrontMatter, undefined)
  })
})

test('citation-lite numbers cited definitions and creates a backlink for every citation', () => {
  const html = dialectMarkdown().render([
    'First [@doe], repeat [@doe], unresolved [@missing], and unsupported [-@doe].',
    '',
    '[@doe]: Jane Doe, **A Book**, 2026.',
    '[@unused]: This uncited definition is not listed.',
  ].join('\n'))

  assert.match(html, /id="cite-doe-1" href="#ref-doe"[^>]*>\[1\]<\/a>/)
  assert.match(html, /id="cite-doe-2" href="#ref-doe"[^>]*>\[1\]<\/a>/)
  assert.match(html, /<span class="inkmark-citation inkmark-citation-unresolved">\[@missing\]<\/span>/)
  assert.match(html, /\[-@doe\]/)
  assert.match(html, /<section id="inkmark-references" class="inkmark-references"/)
  assert.match(html, /<h2 class="inkmark-references-title">参考文献 \/ References<\/h2>/)
  assert.match(html, /<li id="ref-doe" data-source-line="2">[\s\S]*Jane Doe, <strong>A Book<\/strong>, 2026\./)
  assert.match(html, /href="#cite-doe-1" class="inkmark-citation-backref"[^>]*>↩<\/a>/)
  assert.match(html, /href="#cite-doe-2" class="inkmark-citation-backref"[^>]*>↩2<\/a>/)
  assert.doesNotMatch(html, /id="ref-unused"/)
})

test('citation-lite only consumes bounded top-level definitions and escapes invalid definitions', () => {
  const source = [
    '> [@quoted]: A quoted pseudo-definition.',
    '',
    '[@dup]: First definition.',
    '[@dup]: <script>duplicate()</script>',
    '',
    'Quoted [@quoted], known [@dup].',
  ].join('\n')
  const html = dialectMarkdown().render(source)
  assert.match(html, /inkmark-citation-unresolved">\[@quoted\]/)
  assert.match(html, /id="ref-dup"/)
  assert.match(html, /inkmark-citation-definition-invalid[^>]*>\[@dup\]: &lt;script&gt;duplicate\(\)&lt;\/script&gt;/)
  assert.doesNotMatch(html, /<script>duplicate/)

  const tooManyDefinitions = Array.from(
    { length: maximumCitationDefinitions + 1 },
    (_, index) => `[@key${index}]: Reference ${index}`,
  ).join('\n')
  const bounded = dialectMarkdown().render(`${tooManyDefinitions}\n\n[@key256]`)
  assert.match(bounded, /inkmark-citation-definition-invalid/)
  assert.match(bounded, /inkmark-citation-unresolved">\[@key256\]/)

  const tooManyUnresolved = Array.from(
    { length: maximumCitations + 1 },
    (_, index) => `[@missing${index}]`,
  ).join(' ')
  const boundedUnresolved = dialectMarkdown().render(tooManyUnresolved)
  assert.equal(
    (boundedUnresolved.match(/class="inkmark-citation inkmark-citation-unresolved"/g) || []).length,
    maximumCitations,
  )
  assert.match(boundedUnresolved, /\[@missing256\]/)
})

test('citations remain literal inside link labels and image alt text', () => {
  const html = dialectMarkdown().render([
    '[outer [@ref]](https://example.test)',
    '![alt [@ref]](image.png)',
    '',
    'Visible citation [@ref].',
    '',
    '[@ref]: Reference.',
  ].join('\n'))
  assert.match(html, /<a href="https:\/\/example\.test">outer \[@ref\]<\/a>/)
  assert.match(html, /<img src="image\.png" alt="alt \[@ref\]">/)
  assert.equal((html.match(/class="inkmark-citation"/g) || []).length, 1)
  assert.equal((html.match(/id="cite-ref-/g) || []).length, 1)

  const imageAltNoise = Array.from(
    { length: Math.max(maximumWikiLinkCandidates, maximumCitationCandidates) },
    (_, index) => `![alt [[Wiki ${index}]] [@ref]](image-${index}.png)`,
  ).join('\n')
  const afterNoise = dialectMarkdown().render([
    imageAltNoise,
    '',
    'Visible [[Destination]] and [@ref].',
    '',
    '[@ref]: Reference.',
  ].join('\n'))
  assert.match(afterNoise, /data-wiki-target="Destination"/)
  assert.match(afterNoise, /id="cite-ref-1"/)
  assert.equal((afterNoise.match(/class="inkmark-citation"/g) || []).length, 1)
})

test('unclosed citation candidates stay linear and candidate tokens are capped before core finalization', () => {
  const timedRender = (source) => {
    const started = performance.now()
    const html = dialectMarkdown().render(source)
    return { html, elapsed: performance.now() - started }
  }
  timedRender('[@missing')
  const small = timedRender('[@missing '.repeat(100_000))
  const large = timedRender('[@missing '.repeat(200_000))
  assert.ok(large.elapsed < Math.max(1_500, small.elapsed * 5), `unclosed citations took ${large.elapsed.toFixed(1)}ms`)

  const citations = Array.from({ length: maximumCitationCandidates + 100 }, (_, index) => `[@key${index}]`).join(' ')
  const boundedCitations = timedRender(citations).html
  assert.equal(
    (boundedCitations.match(/class="inkmark-citation inkmark-citation-unresolved"/g) || []).length,
    maximumCitations,
  )
  assert.match(boundedCitations, new RegExp(`\\[@key${maximumCitationCandidates}\\]`))

  const wikis = Array.from({ length: maximumWikiLinkCandidates + 100 }, (_, index) => `[[Wiki ${index}]]`).join(' ')
  const boundedWikis = timedRender(wikis).html
  assert.equal((boundedWikis.match(/class="inkmark-wiki-link /g) || []).length, maximumWikiLinks)
  assert.match(boundedWikis, new RegExp(`\\[\\[Wiki ${maximumWikiLinkCandidates}\\]\\]`))
})

test('large repeated Wiki and heading inputs stay within linear rendering budgets', () => {
  const timedRender = (source) => {
    const started = performance.now()
    const html = dialectMarkdown().render(source)
    return { html, elapsed: performance.now() - started }
  }
  // Warm up parser/JIT paths before comparing growth.
  timedRender('# Same\n[[Wiki]]')
  const small = timedRender(Array.from({ length: 1_000 }, () => '# Same\n[[Wiki]]').join('\n'))
  const large = timedRender(Array.from({ length: 4_000 }, () => '# Same\n[[Wiki]]').join('\n'))
  assert.equal((large.html.match(/class="inkmark-wiki-link /g) || []).length, maximumWikiLinks)
  assert.match(large.html, /id="inkmark-heading-same-4000"/)
  assert.ok(large.elapsed < Math.max(1_500, small.elapsed * 8), `render took ${large.elapsed.toFixed(1)}ms`)
})

test('a top-level citation definition can directly interrupt a preceding paragraph', () => {
  const html = dialectMarkdown().render('Citation [@direct].\n[@direct]: Direct reference.')
  assert.match(html, /Citation <a class="inkmark-citation"[^>]*>\[1\]<\/a>\.<\/p>/)
  assert.match(html, /<li id="ref-direct"[^>]*>[\s\S]*Direct reference\./)
  assert.doesNotMatch(html, /\[@direct\]: Direct reference/)
})

test('generated dialect capability nodes carry an optional trusted render marker', () => {
  const marker = 'Trusted_Marker-1234567890'
  const html = dialectMarkdown().render([
    '---',
    'title: Demo',
    '---',
    '[TOC]',
    '# Heading',
    '[[Wiki]] and [@ref].',
    '',
    '[@ref]: Reference.',
  ].join('\n'), { inkmarkTrustedMarker: marker })
  assert.match(html, new RegExp(`<section class="inkmark-front-matter"[^>]*data-inkmark-trusted="${marker}"`))
  assert.match(html, new RegExp(`<nav class="inkmark-toc"[^>]*data-inkmark-trusted="${marker}"`))
  assert.match(html, new RegExp(`<h1 id="inkmark-heading-heading" data-inkmark-trusted="${marker}"`))
  assert.match(html, new RegExp(`class="inkmark-wiki-link[^>]*data-inkmark-trusted="${marker}"`))
  assert.match(html, new RegExp(`class="inkmark-citation"[^>]*data-inkmark-trusted="${marker}"`))
  assert.match(html, new RegExp(`<section id="inkmark-references"[^>]*data-inkmark-trusted="${marker}"`))

  const invalid = dialectMarkdown().render('# Heading\n\n[[Wiki]]', { inkmarkTrustedMarker: 'bad" onmouseover="x' })
  assert.doesNotMatch(invalid, /data-inkmark-trusted/)
  assert.doesNotMatch(invalid, /onmouseover=/)
})

test('App authenticates generated nodes before privileged preview decorators run', async () => {
  const [app, indexHTML, exportDocument] = await Promise.all([
    readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8'),
    readFile(new URL('../frontend/index.html', import.meta.url), 'utf8'),
    readFile(new URL('../frontend/src/export-document.ts', import.meta.url), 'utf8'),
  ])
  assert.match(app, /markdown\.render\(sourceText, \{ inkmarkTrustedMarker: trustedMarker \}\)/)
  assert.match(app, /ADD_ATTR:[\s\S]*'data-inkmark-trusted'[\s\S]*'data-wiki-target'/)
  const provenance = app.indexOf('enforceGeneratedPreviewProvenance(staging, trustedMarker)')
  const decoration = app.indexOf('decoratePreview(staging)', provenance)
  const math = app.indexOf('renderMath(staging)', provenance)
  const diagrams = app.indexOf('renderDiagrams(staging', provenance)
  assert.ok(provenance >= 0 && provenance < decoration && decoration < math && math < diagrams)
  assert.match(app, /\.math-source'[\s\S]*if \(trusted\(element\)\) return[\s\S]*classList\.remove\('math-source'\)/)
  assert.match(app, /'pre\.mermaid'[\s\S]*classList\.remove\('mermaid'\)/)
  assert.match(app, /'\[data-extended-diagram\]'[\s\S]*delete element\.dataset\.extendedDiagram/)
  assert.match(app, /'\.inkmark-image-placeholder'[\s\S]*delete element\.dataset\.inkmarkImageSource/)
  assert.match(app, /protectedIDs[\s\S]*candidate\.removeAttribute\('id'\)/)
  assert.doesNotMatch(app, /protectedIDs\.forEach\([\s\S]{0,500}querySelectorAll<HTMLElement>\('\[id\]'\)/)
  assert.match(app, /'\[data-source-line\]'[\s\S]*if \(!trusted\(element\)\) element\.removeAttribute\('data-source-line'\)/)
  assert.match(app, /root\.querySelectorAll<HTMLElement>\('\[data-inkmark-trusted\]'\)[\s\S]*removeAttribute\('data-inkmark-trusted'\)/)
  assert.match(app, /FORBID_TAGS:[\s\S]{0,350}'form'[\s\S]{0,120}'button'[\s\S]{0,120}'textarea'/)
  assert.match(app, /input\.type !== 'checkbox' \|\| !input\.classList\.contains\('task-list-item-checkbox'\)/)
  assert.match(app, /anchor\.addEventListener\('click',[\s\S]*event\.preventDefault\(\)[\s\S]*\^\(https\?:\|mailto:\)/)
  assert.match(app, /href === '#inkmark-render-test' && builtInDocument\.value === 'welcome'/)
  assert.match(app, /href === '#inkmark-welcome' && builtInDocument\.value === 'render-test'/)
  assert.doesNotMatch(app, /heading\.id = `heading-/)
  assert.match(indexHTML, /form-action 'none'/)
  assert.match(exportDocument, /form-action 'none'/)
})
