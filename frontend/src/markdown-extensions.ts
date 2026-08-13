import MarkdownIt, {
  type MarkdownItOptions,
  type Renderer,
  type StateBlock,
  type StateCore,
  type StateInline,
  type Token,
} from 'markdown-it'
import abbreviation from 'markdown-it-abbr'
import definitionList from 'markdown-it-deflist'
import { full as fullEmoji } from 'markdown-it-emoji'
import footnote from 'markdown-it-footnote'
import mark from 'markdown-it-mark'
import subscript from 'markdown-it-sub'
import superscript from 'markdown-it-sup'

export type CalloutType = 'note' | 'tip' | 'important' | 'warning' | 'caution'

export interface CalloutMarker {
  type: CalloutType
  marker: string
  content: string
}

export type MentionSegment =
  | { kind: 'text'; value: string }
  | { kind: 'mention'; value: string; username: string }

export type ExtendedFenceKind = 'mermaid' | 'mindmap' | 'echarts' | 'abc' | 'graphviz'

export type ExtendedFenceClassification =
  | { status: 'supported'; kind: ExtendedFenceKind; source: string }
  | { status: 'unsupported' }
  | { status: 'too-large'; kind: ExtendedFenceKind; maximumCharacters: number }

export const maximumExtendedFenceCharacters = 128 * 1024
export const maximumMindmapCharacters = 64 * 1024
export const maximumMindmapNodes = 256
export const maximumMindmapDepth = 32
export const maximumAbbreviations = 256
export const maximumAbbreviationLabelCharacters = 64
export const maximumAbbreviationTitleCharacters = 512
export const maximumAbbreviationLabelsCharacters = 8 * 1024
export const maximumFrontMatterCharacters = 64 * 1024
export const maximumFrontMatterLines = 256
export const maximumFrontMatterEntries = 256
export const maximumTableOfContents = 4
export const maximumTableOfContentsHeadings = 256
export const maximumHeadingLabelCharacters = 512
export const maximumWikiLinks = 256
export const maximumWikiLinkCharacters = 256
export const maximumWikiLinkCandidates = maximumWikiLinks * 2
export const maximumCitationDefinitions = 256
export const maximumCitations = 256
export const maximumCitationCandidates = maximumCitations * 2
export const maximumCitationDefinitionCharacters = 2 * 1024
export const maximumCitationDefinitionsCharacters = 64 * 1024

export interface InkMarkFrontMatterValue {
  raw: string
  type: 'boolean' | 'null' | 'number' | 'string'
  value: boolean | null | number | string
}

export type InkMarkFrontMatter = Record<string, InkMarkFrontMatterValue>

export interface InkMarkMarkdownEnvironment {
  inkmarkFrontMatter?: InkMarkFrontMatter
  inkmarkTrustedMarker?: string
}

export interface InkMarkHeadingIdAllocator {
  allocateExplicit(id: string): string | null
  allocateGenerated(label: string): string
  reserve(id: string): boolean
}

const calloutTypes = new Set<CalloutType>(['note', 'tip', 'important', 'warning', 'caution'])
const calloutMarkerPattern = /^[ \t]*(?:\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]|(NOTE|TIP|IMPORTANT|WARNING|CAUTION)[ \t]*:)(?:[ \t]+|\r?\n|$)/i
const mentionPattern = /@[A-Za-z0-9](?:[A-Za-z0-9_-]{0,37}[A-Za-z0-9])?/g
const invalidMentionPrefixPattern = /[A-Za-z0-9.!#$%&'*+/=?^_`{|}~@/-]/
const invalidMentionSuffixPattern = /[A-Za-z0-9_@-]/
const headingAttributeIdentifierPattern = /^[A-Za-z][A-Za-z0-9_-]{0,63}$/
const reservedHeadingIdPattern = /^(?:inkmark-|fn|wiki-|cite-|ref-)/i
const frontMatterKeyPattern = /^[A-Za-z_][A-Za-z0-9_.-]{0,63}$/
const forbiddenFrontMatterKeyPattern = /^(?:__proto__|prototype|constructor)$/i
const citationKeyPattern = /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$/
const frontMatterDangerousScalarPattern = /^(?:[!&*]|[|>](?:[+-]?\d*)?$)/

interface HeadingEntry {
  id: string
  level: number
  label: string
}

interface CitationDefinition {
  key: string
  text: string
  line: number
  number?: number
  citations: string[]
}

interface MarkdownDialectEnvironment extends InkMarkMarkdownEnvironment {
  inkmarkTrustedMarker?: string
  abbreviations?: Record<string, string>
  inkmarkCitationDefinitions?: Map<string, CitationDefinition>
  inkmarkCitationDefinitionCharacters?: number
  inkmarkWikiLinkCandidates?: number
  inkmarkCitationCandidates?: number
  inkmarkParsingImageAlt?: boolean
  inkmarkImageAltParseDepth?: number
}

const extendedFenceAliases = new Map<string, ExtendedFenceKind>([
  ['abc', 'abc'],
  ['dot', 'graphviz'],
  ['echarts', 'echarts'],
  ['graphviz', 'graphviz'],
  ['mermaid', 'mermaid'],
  ['mindmap', 'mindmap'],
  ['mmd', 'mermaid'],
])

/** Install syntax extensions which operate entirely inside markdown-it. */
export function installMarkdownExtensions(markdown: InstanceType<typeof MarkdownIt>): InstanceType<typeof MarkdownIt> {
  markdown.use(fullEmoji)
  markdown.use(footnote)
  installFootnoteProvenance(markdown)
  markdown.use(definitionList)
  markdown.use(abbreviation)
  markdown.use(mark)
  markdown.use(subscript)
  markdown.use(superscript)
  installSafeDialectExtensions(markdown)
  return markdown
}

/**
 * Authenticate the two footnote elements that own generated fragment IDs.
 * Raw HTML may contain the same `fn*`/`fnref*` names, so App.vue needs the
 * per-render marker to make the generated targets win during DOM de-duplication.
 */
function installFootnoteProvenance(markdown: InstanceType<typeof MarkdownIt>) {
  const renderReference = markdown.renderer.rules.footnote_ref
  const renderDefinition = markdown.renderer.rules.footnote_open
  if (renderReference) {
    markdown.renderer.rules.footnote_ref = (tokens, index, options, environment, renderer) => {
      const rendered = renderReference(tokens, index, options, environment, renderer)
      const attribute = renderTrustedAttribute(environment)
      return attribute ? rendered.replace('<a ', `<a${attribute} `) : rendered
    }
  }
  if (renderDefinition) {
    markdown.renderer.rules.footnote_open = (tokens, index, options, environment, renderer) => {
      const rendered = renderDefinition(tokens, index, options, environment, renderer)
      const attribute = renderTrustedAttribute(environment)
      return attribute ? rendered.replace('<li ', `<li${attribute} `) : rendered
    }
  }
}

/**
 * Allocate stable heading IDs for both rendered headings and the generated
 * table of contents. Explicit IDs share the same collision set, but may not
 * enter namespaces reserved for InkMark's generated anchors.
 */
export function createHeadingIdAllocator(initialIds: Iterable<string> = []): InkMarkHeadingIdAllocator {
  const used = new Set<string>()
  const nextSuffixByBase = new Map<string, number>()
  for (const id of initialIds) {
    const normalized = id.trim()
    if (normalized) used.add(normalized)
  }

  const unique = (base: string) => {
    if (!used.has(base)) {
      used.add(base)
      nextSuffixByBase.set(base, 2)
      return base
    }
    let suffix = nextSuffixByBase.get(base) || 2
    let candidate = `${base}-${suffix}`
    while (used.has(candidate)) candidate = `${base}-${++suffix}`
    used.add(candidate)
    nextSuffixByBase.set(base, suffix + 1)
    return candidate
  }

  return {
    allocateExplicit(id: string) {
      if (!headingAttributeIdentifierPattern.test(id) || reservedHeadingIdPattern.test(id)) return null
      return unique(id)
    },
    allocateGenerated(label: string) {
      const boundedLabel = Array.from(label).slice(0, maximumHeadingLabelCharacters).join('')
      const normalized = boundedLabel.normalize('NFKC').toLocaleLowerCase('en-US')
      const slug = Array.from(normalized)
        .map((character) => /[\p{Letter}\p{Number}]/u.test(character) ? character : '-')
        .join('')
        .replace(/-+/g, '-')
        .replace(/^-|-$/g, '')
      return unique(`inkmark-heading-${Array.from(slug || 'section').slice(0, 64).join('')}`)
    },
    reserve(id: string) {
      const normalized = id.trim()
      if (!normalized || used.has(normalized)) return false
      used.add(normalized)
      return true
    },
  }
}

function installSafeDialectExtensions(markdown: InstanceType<typeof MarkdownIt>) {
  // markdown-it parses image alt labels through a fresh inline parser using
  // the same environment. Mark that nested parse so inert alt text cannot
  // consume the document-wide Wiki/citation candidate budget.
  const parseInline = markdown.inline.parse.bind(markdown.inline)
  markdown.inline.parse = (source, instance, environment, output) => {
    const dialectEnvironment = environment as MarkdownDialectEnvironment
    const imageAltParse = dialectEnvironment.inkmarkParsingImageAlt === true
    if (imageAltParse) dialectEnvironment.inkmarkImageAltParseDepth = (dialectEnvironment.inkmarkImageAltParseDepth || 0) + 1
    try {
      parseInline(source, instance, environment, output)
    } finally {
      if (imageAltParse) {
        const depth = dialectEnvironment.inkmarkImageAltParseDepth || 1
        if (depth <= 1) delete dialectEnvironment.inkmarkImageAltParseDepth
        else dialectEnvironment.inkmarkImageAltParseDepth = depth - 1
      }
    }
  }
  const imageRule = markdown.inline.ruler.__rules__.find((rule) => rule.name === 'image')
  if (imageRule) {
    const parseImage = imageRule.fn
    markdown.inline.ruler.at('image', (state, silent) => {
      if (silent) return parseImage(state, silent)
      const environment = state.env as MarkdownDialectEnvironment
      environment.inkmarkParsingImageAlt = true
      try {
        return parseImage(state, silent)
      } finally {
        delete environment.inkmarkParsingImageAlt
      }
    })
  }

  markdown.core.ruler.after('normalize', 'inkmark_dialect_environment', resetDialectEnvironment)
  markdown.block.ruler.before('table', 'inkmark_front_matter', parseFrontMatterBlock, {
    alt: ['paragraph', 'reference'],
  })
  markdown.block.ruler.before('reference', 'inkmark_citation_definition', parseCitationDefinitionBlock, {
    alt: ['paragraph', 'reference'],
  })
  markdown.inline.ruler.before('link', 'inkmark_wiki_link', parseWikiLink)
  markdown.inline.ruler.before('link', 'inkmark_citation', parseCitation)
  markdown.core.ruler.after('block', 'inkmark_abbreviation_budget', enforceAbbreviationBudget)
  markdown.core.ruler.after('inline', 'inkmark_document_dialects', finalizeDocumentDialects)
  markdown.core.ruler.after('emoji', 'inkmark_toc_display_labels', refreshTableOfContentsLabels)

  markdown.renderer.rules.inkmark_front_matter = renderFrontMatter
  markdown.renderer.rules.inkmark_citation_definition_literal = (tokens, index, _options, environment) => {
    const token = tokens[index]
    const sourceLine = token.map?.[0]
    const lineAttribute = sourceLine === undefined ? '' : ` data-source-line="${sourceLine}"`
    return `<p class="inkmark-citation-definition-invalid"${lineAttribute}${renderTrustedAttribute(environment)}>${markdown.utils.escapeHtml(token.content)}</p>\n`
  }
  markdown.renderer.rules.inkmark_wiki_link = renderWikiLink
  markdown.renderer.rules.inkmark_citation = renderCitation
  markdown.renderer.rules.inkmark_toc = renderTableOfContents
}

function resetDialectEnvironment(state: StateCore) {
  const environment = state.env as MarkdownDialectEnvironment
  delete environment.abbreviations
  delete environment.inkmarkFrontMatter
  delete environment.inkmarkCitationDefinitions
  delete environment.inkmarkCitationDefinitionCharacters
  delete environment.inkmarkWikiLinkCandidates
  delete environment.inkmarkCitationCandidates
}

function parseFrontMatterBlock(
  state: StateBlock,
  startLine: number,
  endLine: number,
  silent: boolean,
): boolean {
  if (startLine !== 0 || state.parentType !== 'root' || state.blkIndent !== 0) return false
  const opening = state.src.slice(
    state.bMarks[startLine] + state.tShift[startLine],
    state.eMarks[startLine],
  ).replace(/^\uFEFF/, '')
  if (opening !== '---') return false

  const searchEnd = Math.min(endLine, startLine + maximumFrontMatterLines)
  let closingLine = -1
  for (let line = startLine + 1; line < searchEnd; line++) {
    const text = state.src.slice(state.bMarks[line] + state.tShift[line], state.eMarks[line])
    if (text === '---' || text === '...') {
      closingLine = line
      break
    }
    if (state.eMarks[line] - state.bMarks[startLine] > maximumFrontMatterCharacters) return false
  }
  if (closingLine < 0) return false

  const characters = state.eMarks[closingLine] - state.bMarks[startLine]
  if (characters > maximumFrontMatterCharacters) return false
  const parsed = parseFrontMatterEntries(state, startLine + 1, closingLine)
  if (!parsed) return false
  if (silent) return true

  const token = state.push('inkmark_front_matter', 'section', 0)
  token.block = true
  token.map = [startLine, closingLine + 1]
  token.meta = { entries: Object.entries(parsed) }
  const environment = state.env as MarkdownDialectEnvironment
  environment.inkmarkFrontMatter = parsed
  state.line = closingLine + 1
  return true
}

function parseFrontMatterEntries(
  state: StateBlock,
  firstLine: number,
  closingLine: number,
): InkMarkFrontMatter | null {
  const result = Object.create(null) as InkMarkFrontMatter
  let count = 0
  for (let line = firstLine; line < closingLine; line++) {
    const text = state.src.slice(state.bMarks[line], state.eMarks[line])
    if (!text.trim() || /^[ \t]*#/.test(text)) continue
    if (/^[ \t]/.test(text) || /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/.test(text)) return null
    const separator = text.indexOf(':')
    if (separator <= 0) return null
    const key = text.slice(0, separator).trim()
    const raw = text.slice(separator + 1).trim()
    if (
      !frontMatterKeyPattern.test(key)
      || key.split('.').some((part) => forbiddenFrontMatterKeyPattern.test(part))
      || Object.prototype.hasOwnProperty.call(result, key)
      || raw.length > 1024
      || ++count > maximumFrontMatterEntries
    ) return null
    const value = parseFrontMatterScalar(raw)
    if (!value) return null
    result[key] = value
  }
  return result
}

function parseFrontMatterScalar(raw: string): InkMarkFrontMatterValue | null {
  if (!raw) return { raw, type: 'string', value: '' }
  if (frontMatterDangerousScalarPattern.test(raw) || /^[{[]/.test(raw)) return null
  if (/^(?:true|false)$/i.test(raw)) {
    return { raw, type: 'boolean', value: raw.toLowerCase() === 'true' }
  }
  if (/^(?:null|~)$/i.test(raw)) return { raw, type: 'null', value: null }
  if (/^[+-]?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$/.test(raw)) {
    const number = Number(raw)
    if (Number.isFinite(number)) return { raw, type: 'number', value: number }
    return null
  }
  if (raw.startsWith('"') || raw.startsWith("'")) {
    const quote = raw[0]
    if (!raw.endsWith(quote) || raw.length < 2) return null
    if (quote === '"') {
      try {
        const value = JSON.parse(raw)
        return typeof value === 'string' && !/[\u0000-\u001f\u007f]/.test(value)
          ? { raw, type: 'string', value }
          : null
      } catch {
        return null
      }
    }
    const inner = raw.slice(1, -1)
    if (/(^|[^'])'(?!')/.test(inner)) return null
    const value = inner.replace(/''/g, "'")
    return /[\u0000-\u001f\u007f]/.test(value) ? null : { raw, type: 'string', value }
  }
  return { raw, type: 'string', value: raw }
}

function renderFrontMatter(
  tokens: Token[],
  index: number,
  _options: MarkdownItOptions,
  rawEnvironment: unknown,
  _renderer: Renderer,
): string {
  const token = tokens[index]
  const entries = (token.meta?.entries || []) as Array<[string, InkMarkFrontMatterValue]>
  const sourceLine = token.map?.[0]
  const lineAttribute = sourceLine === undefined ? '' : ` data-source-line="${sourceLine}"`
  const trustedAttribute = renderTrustedAttribute(rawEnvironment)
  const rows = entries.map(([key, item]) => (
    `<dt>${escapeHTML(key)}</dt><dd>${escapeHTML(String(item.value ?? item.raw))}</dd>`
  )).join('')
  return `<section class="inkmark-front-matter" aria-label="Front Matter"${lineAttribute}${trustedAttribute}>`
    + '<h2>Front Matter / 文档元数据</h2>'
    + `<dl>${rows}</dl></section>\n`
}

function enforceAbbreviationBudget(state: StateCore) {
  const environment = state.env as MarkdownDialectEnvironment
  const definitions = environment.abbreviations
  if (!definitions) return
  const entries = Object.entries(definitions)
  let labelsCharacters = 0
  const invalid = entries.length > maximumAbbreviations || entries.some(([storedLabel, title]) => {
    const label = storedLabel.startsWith(':') ? storedLabel.slice(1) : storedLabel
    labelsCharacters += Array.from(label).length
    return !label
      || Array.from(label).length > maximumAbbreviationLabelCharacters
      || Array.from(String(title)).length > maximumAbbreviationTitleCharacters
      || labelsCharacters > maximumAbbreviationLabelsCharacters
  })
  if (invalid) delete environment.abbreviations
}

function parseWikiLink(state: StateInline, silent: boolean): boolean {
  if (state.src.charCodeAt(state.pos) !== 0x5b || state.src.charCodeAt(state.pos + 1) !== 0x5b) return false
  // Returning true during markdown-it's silent label scan would consume the
  // nested brackets and prevent the surrounding standard link/image from
  // being recognised. Actual rendering calls the rule again with silent=false.
  if (silent) return false
  const environment = state.env as MarkdownDialectEnvironment
  if (environment.inkmarkImageAltParseDepth) return false
  const inlineState = state as StateInline & { linkLevel: number }
  if (inlineState.linkLevel > 0) return false
  const scanEnd = Math.min(state.posMax, state.pos + maximumWikiLinkCharacters * 2 + 8)
  const close = state.src.slice(state.pos + 2, scanEnd).indexOf(']]')
  if (close < 0) return false
  const end = state.pos + 2 + close
  const raw = state.src.slice(state.pos + 2, end)
  if (!raw || /[\r\n]/.test(raw)) return false
  const separator = raw.indexOf('|')
  const target = (separator < 0 ? raw : raw.slice(0, separator)).trim().normalize('NFC')
  const label = (separator < 0 ? target : raw.slice(separator + 1).trim()).normalize('NFC')
  if (!isSafeWikiText(target) || !label || Array.from(label).length > maximumWikiLinkCharacters) return false

  const candidateCount = environment.inkmarkWikiLinkCandidates || 0
  if (candidateCount >= maximumWikiLinkCandidates) return false
  environment.inkmarkWikiLinkCandidates = candidateCount + 1

  const token = state.push('inkmark_wiki_link', 'span', 0)
  token.content = label
  token.meta = { target, literal: state.src.slice(state.pos, end + 2) }
  state.pos = end + 2
  return true
}

function isSafeWikiText(value: string): boolean {
  return Boolean(value)
    && Array.from(value).length <= maximumWikiLinkCharacters
    && !/[\u0000-\u001f\u007f]/.test(value)
    && !/^[A-Za-z][A-Za-z0-9+.-]*:/.test(value)
    && !/^[/\\]|(?:^|[/\\])\.\.(?:[/\\]|$)/.test(value)
}

function renderWikiLink(
  tokens: Token[],
  index: number,
  _options: MarkdownItOptions,
  rawEnvironment: unknown,
): string {
  const token = tokens[index]
  const target = token.meta?.target || ''
  return `<span class="inkmark-wiki-link inkmark-wiki-link-unresolved" data-wiki-target="${escapeAttribute(target)}" aria-disabled="true"${renderTrustedAttribute(rawEnvironment)}>${escapeHTML(token.content)}</span>`
}

function parseCitationDefinitionBlock(
  state: StateBlock,
  startLine: number,
  _endLine: number,
  silent: boolean,
): boolean {
  const atTopLevel = state.parentType === 'root' || (silent && state.parentType === 'paragraph')
  if (!atTopLevel || state.blkIndent !== 0 || state.sCount[startLine] !== 0) return false
  const text = state.src.slice(state.bMarks[startLine], state.eMarks[startLine])
  const candidate = /^\[@([^\]\r\n]+)\]:[ \t]*(.*)$/.exec(text)
  if (!candidate) return false
  if (silent) return true

  const environment = state.env as MarkdownDialectEnvironment
  const definitions = environment.inkmarkCitationDefinitions
    || (environment.inkmarkCitationDefinitions = new Map<string, CitationDefinition>())
  const key = candidate[1]
  const definitionText = candidate[2].trim()
  const totalCharacters = (environment.inkmarkCitationDefinitionCharacters || 0) + definitionText.length
  const valid = citationKeyPattern.test(key)
    && Boolean(definitionText)
    && definitionText.length <= maximumCitationDefinitionCharacters
    && totalCharacters <= maximumCitationDefinitionsCharacters
    && definitions.size < maximumCitationDefinitions
    && !definitions.has(key)

  if (valid) {
    definitions.set(key, { key, text: definitionText, line: startLine, citations: [] })
    environment.inkmarkCitationDefinitionCharacters = totalCharacters
  } else {
    const token = state.push('inkmark_citation_definition_literal', 'p', 0)
    token.block = true
    token.map = [startLine, startLine + 1]
    token.content = text
  }
  state.line = startLine + 1
  return true
}

function parseCitation(state: StateInline, silent: boolean): boolean {
  if (state.src.charCodeAt(state.pos) !== 0x5b || state.src.charCodeAt(state.pos + 1) !== 0x40) return false
  if (silent) return false
  const environment = state.env as MarkdownDialectEnvironment
  if (environment.inkmarkImageAltParseDepth) return false
  const inlineState = state as StateInline & { linkLevel: number }
  if (inlineState.linkLevel > 0) return false
  // Bound the search window. Repeated, unclosed `[@` prefixes in a large
  // document must not rescan the complete remaining suffix for every byte.
  const scanEnd = Math.min(state.posMax, state.pos + 68)
  const relativeClose = state.src.slice(state.pos + 2, scanEnd).indexOf(']')
  if (relativeClose < 0) return false
  const close = state.pos + 2 + relativeClose
  const key = state.src.slice(state.pos + 2, close)
  if (!citationKeyPattern.test(key)) return false

  const candidateCount = environment.inkmarkCitationCandidates || 0
  if (candidateCount >= maximumCitationCandidates) return false
  environment.inkmarkCitationCandidates = candidateCount + 1
  const token = state.push('inkmark_citation', 'span', 0)
  token.content = `[@${key}]`
  token.meta = { key }
  state.pos = close + 1
  return true
}

function renderCitation(
  tokens: Token[],
  index: number,
  _options: MarkdownItOptions,
  rawEnvironment: unknown,
): string {
  const token = tokens[index]
  const key = String(token.meta?.key || '')
  const number = Number(token.meta?.number)
  const citationId = String(token.meta?.citationId || '')
  if (!Number.isSafeInteger(number) || number < 1 || !citationId) {
    return `<span class="inkmark-citation inkmark-citation-unresolved"${renderTrustedAttribute(rawEnvironment)}>${escapeHTML(token.content)}</span>`
  }
  return `<a class="inkmark-citation" id="${escapeAttribute(citationId)}" href="#ref-${escapeAttribute(key)}" aria-label="Citation ${number}"${renderTrustedAttribute(rawEnvironment)}>[${number}]</a>`
}

function finalizeDocumentDialects(state: StateCore) {
  restoreImageLabelDialectLiterals(state.tokens)
  enforceVisibleWikiBudget(state.tokens)
  enforceVisibleCitationBudget(state.tokens)
  const allocator = createHeadingIdAllocator()
  const headings = finalizeHeadingAttributes(state.tokens, allocator, state.env as MarkdownDialectEnvironment)
  finalizeCitations(state)
  replaceTableOfContentsMarkers(state, headings)
}

function enforceVisibleCitationBudget(tokens: Token[]) {
  let count = 0
  const visit = (items: Token[]) => {
    for (const token of items) {
      if (token.type === 'inkmark_citation') {
        if (count >= maximumCitations) {
          token.type = 'text'
          token.tag = ''
          token.nesting = 0
          token.meta = null
        } else {
          count += 1
        }
      }
      if (token.children) visit(token.children)
    }
  }
  visit(tokens)
}

function enforceVisibleWikiBudget(tokens: Token[]) {
  let count = 0
  const visit = (items: Token[]) => {
    for (const token of items) {
      if (token.type === 'inkmark_wiki_link') {
        if (count >= maximumWikiLinks) {
          token.content = String(token.meta?.literal || token.content)
          token.type = 'text'
          token.tag = ''
          token.nesting = 0
          token.meta = null
        } else {
          count += 1
        }
      }
      if (token.children) visit(token.children)
    }
  }
  visit(tokens)
}

function restoreImageLabelDialectLiterals(tokens: Token[]) {
  for (const token of tokens) {
    if (token.type === 'image' && token.children) {
      for (const child of token.children) {
        if (child.type !== 'inkmark_wiki_link' && child.type !== 'inkmark_citation') continue
        if (child.type === 'inkmark_wiki_link') child.content = String(child.meta?.literal || child.content)
        child.type = 'text'
        child.tag = ''
        child.nesting = 0
        child.meta = null
      }
      continue
    }
    if (token.children) restoreImageLabelDialectLiterals(token.children)
  }
}

function finalizeHeadingAttributes(
  tokens: Token[],
  allocator: InkMarkHeadingIdAllocator,
  environment: MarkdownDialectEnvironment,
): HeadingEntry[] {
  const entries: HeadingEntry[] = []
  for (let index = 0; index < tokens.length - 1; index++) {
    const opening = tokens[index]
    if (opening.type !== 'heading_open') continue
    const inline = tokens[index + 1]
    if (inline?.type !== 'inline') continue

    const attributes = takeHeadingAttributes(inline)
    let id: string | null = null
    if (attributes) {
      id = attributes.id ? allocator.allocateExplicit(attributes.id) : null
      // A rejected explicit ID leaves the complete attribute expression
      // visible and cannot smuggle its otherwise-valid classes into the DOM.
      if (attributes.id && !id) attributes.restore()
      else attributes.classes.forEach((name) => opening.attrJoin('class', `user-heading-${name}`))
    }
    const label = Array.from(inlineTextContent(inline.children || []).trim() || 'Section')
      .slice(0, maximumHeadingLabelCharacters)
      .join('')
    id ||= allocator.allocateGenerated(label)
    opening.attrSet('id', id)
    setTrustedMarker(opening, environment)
    if (entries.length < maximumTableOfContentsHeadings) {
      entries.push({ id, level: Number(opening.tag.slice(1)) || 1, label })
    }
  }
  return entries
}

interface ParsedHeadingAttributes {
  id: string | null
  classes: string[]
  restore: () => void
}

function takeHeadingAttributes(inline: Token): ParsedHeadingAttributes | null {
  const children = inline.children || []
  const last = children[children.length - 1]
  if (!last || last.type !== 'text') return null
  const match = /(^|\s)\{([^{}\r\n]{1,256})\}\s*$/.exec(last.content)
  if (!match) return null
  const parts = match[2].trim().split(/\s+/)
  if (!parts.length || parts.length > 16) return null
  let id: string | null = null
  const classes: string[] = []
  for (const part of parts) {
    if (part.startsWith('#') && !id && headingAttributeIdentifierPattern.test(part.slice(1))) {
      id = part.slice(1)
    } else if (part.startsWith('.') && headingAttributeIdentifierPattern.test(part.slice(1))) {
      if (!classes.includes(part.slice(1))) classes.push(part.slice(1))
    } else {
      return null
    }
  }
  const original = last.content
  const stripped = original.slice(0, match.index) + (match[1] ? '' : original.slice(match.index, match.index + match[1].length))
  last.content = stripped.replace(/\s+$/, '')
  inline.content = inline.content.replace(/(^|\s)\{[^{}\r\n]{1,256}\}\s*$/, '').replace(/\s+$/, '')
  return {
    id,
    classes,
    restore() {
      last.content = original
    },
  }
}

function inlineTextContent(tokens: Token[]): string {
  let result = ''
  for (const token of tokens) {
    if (token.type === 'image') result += token.content
    else if (token.type === 'softbreak' || token.type === 'hardbreak') result += ' '
    else if (token.type === 'text' || token.type === 'code_inline' || token.type === 'emoji') result += token.content
    else if (token.children) result += inlineTextContent(token.children)
  }
  return result.replace(/\s+/g, ' ')
}

function replaceTableOfContentsMarkers(state: StateCore, headings: HeadingEntry[]) {
  let rendered = 0
  for (let index = 0; index < state.tokens.length - 2; index++) {
    const opening = state.tokens[index]
    const inline = state.tokens[index + 1]
    const closing = state.tokens[index + 2]
    const isMarker = opening.type === 'paragraph_open'
      && opening.level === 0
      && inline?.type === 'inline'
      && /^\[toc\]$/i.test(inline.content.trim())
      && inline.children?.length === 1
      && inline.children[0].type === 'text'
      && closing?.type === 'paragraph_close'
    if (!isMarker || rendered >= maximumTableOfContents) continue
    const token = new state.Token('inkmark_toc', 'nav', 0)
    token.block = true
    token.map = opening.map
    token.meta = { entries: headings }
    state.tokens.splice(index, 3, token)
    rendered += 1
  }
}

function refreshTableOfContentsLabels(state: StateCore) {
  const labels: string[] = []
  for (let index = 0; index < state.tokens.length - 1 && labels.length < maximumTableOfContentsHeadings; index++) {
    if (state.tokens[index].type !== 'heading_open') continue
    const inline = state.tokens[index + 1]
    if (inline?.type === 'inline') labels.push(inlineTextContent(inline.children || []).trim() || 'Section')
  }
  state.tokens.forEach((token) => {
    if (token.type !== 'inkmark_toc') return
    const entries = (token.meta?.entries || []) as HeadingEntry[]
    entries.forEach((entry, index) => {
      if (labels[index]) entry.label = labels[index]
    })
  })
}

function renderTableOfContents(
  tokens: Token[],
  index: number,
  _options: MarkdownItOptions,
  rawEnvironment: unknown,
): string {
  const token = tokens[index]
  const entries = (token.meta?.entries || []) as HeadingEntry[]
  const sourceLine = token.map?.[0]
  const lineAttribute = sourceLine === undefined ? '' : ` data-source-line="${sourceLine}"`
  const items = entries.map((entry) => (
    `<li class="inkmark-toc-level-${Math.min(6, Math.max(1, entry.level))}">`
    + `<a href="#${escapeAttribute(entry.id)}">${escapeHTML(entry.label)}</a></li>`
  )).join('')
  return `<nav class="inkmark-toc" aria-label="Table of contents"${lineAttribute}${renderTrustedAttribute(rawEnvironment)}>`
    + '<h2>目录 / Contents</h2>'
    + `<ol>${items}</ol></nav>\n`
}

function finalizeCitations(state: StateCore) {
  const environment = state.env as MarkdownDialectEnvironment
  const definitions = environment.inkmarkCitationDefinitions
  if (!definitions?.size) return
  let nextNumber = 1
  const used: CitationDefinition[] = []

  const visit = (tokens: Token[]) => {
    for (const token of tokens) {
      if (token.type === 'inkmark_citation') {
        const key = String(token.meta?.key || '')
        const definition = definitions.get(key)
        if (!definition) continue
        if (!definition.number) {
          definition.number = nextNumber++
          used.push(definition)
        }
        const citationId = `cite-${key}-${definition.citations.length + 1}`
        definition.citations.push(citationId)
        token.meta = { ...token.meta, number: definition.number, citationId }
      }
      if (token.children) visit(token.children)
    }
  }
  visit(state.tokens)
  if (!used.length) return

  const make = (type: string, tag: string, nesting: 1 | 0 | -1) => new state.Token(type, tag, nesting)
  const sectionOpen = make('section_open', 'section', 1)
  sectionOpen.block = true
  sectionOpen.map = [used[0].line, used[0].line + 1]
  sectionOpen.attrSet('id', 'inkmark-references')
  sectionOpen.attrSet('class', 'inkmark-references')
  sectionOpen.attrSet('aria-label', 'References')
  sectionOpen.attrSet('data-source-line', String(used[0].line))
  setTrustedMarker(sectionOpen, environment)
  const headingOpen = make('heading_open', 'h2', 1)
  headingOpen.block = true
  headingOpen.attrSet('class', 'inkmark-references-title')
  const headingInline = make('inline', '', 0)
  headingInline.content = '参考文献 / References'
  headingInline.children = []
  state.md.inline.parse(headingInline.content, state.md, state.env, headingInline.children)
  const headingClose = make('heading_close', 'h2', -1)
  headingClose.block = true
  const listOpen = make('ordered_list_open', 'ol', 1)
  listOpen.block = true
  const appended = [sectionOpen, headingOpen, headingInline, headingClose, listOpen]

  for (const definition of used) {
    const itemOpen = make('list_item_open', 'li', 1)
    itemOpen.block = true
    itemOpen.map = [definition.line, definition.line + 1]
    itemOpen.attrSet('id', `ref-${definition.key}`)
    itemOpen.attrSet('data-source-line', String(definition.line))
    setTrustedMarker(itemOpen, environment)
    const paragraphOpen = make('paragraph_open', 'p', 1)
    paragraphOpen.block = true
    const inline = make('inline', '', 0)
    inline.content = definition.text
    inline.children = []
    state.md.inline.parse(definition.text, state.md, state.env, inline.children)
    inline.children.push(make('text', '', 0))
    inline.children[inline.children.length - 1].content = ' '
    definition.citations.forEach((citationId, citationIndex) => {
      const backlinkOpen = make('link_open', 'a', 1)
      backlinkOpen.attrSet('href', `#${citationId}`)
      backlinkOpen.attrSet('class', 'inkmark-citation-backref')
      backlinkOpen.attrSet('aria-label', `Back to citation ${citationIndex + 1}`)
      setTrustedMarker(backlinkOpen, environment)
      const backlinkText = make('text', '', 0)
      backlinkText.content = citationIndex === 0 ? '↩' : `↩${citationIndex + 1}`
      const backlinkClose = make('link_close', 'a', -1)
      inline.children!.push(backlinkOpen, backlinkText, backlinkClose)
      if (citationIndex < definition.citations.length - 1) {
        const separator = make('text', '', 0)
        separator.content = ' '
        inline.children!.push(separator)
      }
    })
    const paragraphClose = make('paragraph_close', 'p', -1)
    paragraphClose.block = true
    const itemClose = make('list_item_close', 'li', -1)
    itemClose.block = true
    appended.push(itemOpen, paragraphOpen, inline, paragraphClose, itemClose)
  }
  const listClose = make('ordered_list_close', 'ol', -1)
  listClose.block = true
  const sectionClose = make('section_close', 'section', -1)
  sectionClose.block = true
  appended.push(listClose, sectionClose)
  state.tokens.push(...appended)
}

function escapeHTML(value: unknown): string {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function escapeAttribute(value: unknown): string {
  return escapeHTML(value).replace(/\r|\n/g, ' ')
}

function trustedMarker(rawEnvironment: unknown): string {
  if (!rawEnvironment || typeof rawEnvironment !== 'object') return ''
  const marker = (rawEnvironment as MarkdownDialectEnvironment).inkmarkTrustedMarker
  return typeof marker === 'string' && /^[A-Za-z0-9_-]{16,128}$/.test(marker) ? marker : ''
}

function renderTrustedAttribute(rawEnvironment: unknown): string {
  const marker = trustedMarker(rawEnvironment)
  return marker ? ` data-inkmark-trusted="${escapeAttribute(marker)}"` : ''
}

function setTrustedMarker(token: Token, rawEnvironment: unknown) {
  const marker = trustedMarker(rawEnvironment)
  if (marker) token.attrSet('data-inkmark-trusted', marker)
}

/**
 * Parse both GitHub callout markers (`[!NOTE]`) and the legacy form used by
 * existing InkMark documents (`NOTE:`). The caller can safely replace exactly
 * `marker` with `content` without running another regular expression.
 */
export function parseCalloutMarker(text: string): CalloutMarker | null {
  const match = calloutMarkerPattern.exec(text)
  if (!match) return null
  const normalized = (match[1] || match[2] || '').toLowerCase() as CalloutType
  if (!calloutTypes.has(normalized)) return null
  return {
    type: normalized,
    marker: match[0],
    content: text.slice(match[0].length),
  }
}

/**
 * Split plain text into inert text and @mention segments. This intentionally
 * recognises a conservative ASCII username form and rejects email/URL-like
 * prefixes so decorating mentions cannot corrupt links or addresses.
 */
export function segmentMentions(text: string): MentionSegment[] {
  const segments: MentionSegment[] = []
  let cursor = 0
  mentionPattern.lastIndex = 0

  let match: RegExpExecArray | null
  while ((match = mentionPattern.exec(text))) {
    const start = match.index
    const end = start + match[0].length
    const prefix = start > 0 ? text[start - 1] : ''
    const suffix = end < text.length ? text[end] : ''
    if ((prefix && invalidMentionPrefixPattern.test(prefix)) || (suffix && invalidMentionSuffixPattern.test(suffix))) {
      continue
    }

    appendTextSegment(segments, text.slice(cursor, start))
    segments.push({ kind: 'mention', value: match[0], username: match[0].slice(1) })
    cursor = end
  }

  appendTextSegment(segments, text.slice(cursor))
  return segments
}

/** Alias used by preview decorators which treat the output as render tokens. */
export const tokenizeMentions = segmentMentions

function appendTextSegment(segments: MentionSegment[], value: string) {
  if (!value) return
  const previous = segments[segments.length - 1]
  if (previous?.kind === 'text') previous.value += value
  else segments.push({ kind: 'text', value })
}

/** Return a canonical renderer kind for the first word of a fence info string. */
export function extendedFenceKind(info: string): ExtendedFenceKind | null {
  const language = info.trim().split(/\s+/, 1)[0]?.toLowerCase() || ''
  return extendedFenceAliases.get(language) || null
}

/** Classify a supported visual fence and reject unbounded renderer input. */
export function classifyExtendedFence(
  info: string,
  source: string,
  maximumCharacters = maximumExtendedFenceCharacters,
): ExtendedFenceClassification {
  const kind = extendedFenceKind(info)
  if (!kind) return { status: 'unsupported' }
  const boundedMaximum = Number.isSafeInteger(maximumCharacters) && maximumCharacters >= 0
    ? maximumCharacters
    : maximumExtendedFenceCharacters
  if (source.length > boundedMaximum) {
    return { status: 'too-large', kind, maximumCharacters: boundedMaximum }
  }
  return { status: 'supported', kind, source }
}

interface MindmapItem {
  depth: number
  label: string
}

/**
 * Convert a two-space-indented Markdown bullet list into Mermaid mindmap
 * source. A fixed virtual root permits multiple top-level siblings while
 * generated node IDs and entity-escaped labels keep user text inert.
 */
export function convertIndentedBulletMindmap(source: string, rootLabel = 'Mindmap'): string | null {
  if (!source.trim() || source.length > maximumMindmapCharacters) return null
  const items: MindmapItem[] = []

  for (const rawLine of source.replace(/\r\n?/g, '\n').split('\n')) {
    if (!rawLine.trim()) continue
    if (rawLine.includes('\t')) return null
    const match = /^( *)(?:[-+*])\s+(.+?)\s*$/.exec(rawLine)
    if (!match || match[1].length % 2 !== 0) return null
    const depth = match[1].length / 2
    const label = match[2].trim()
    if (!label || depth > maximumMindmapDepth) return null
    if (!items.length && depth !== 0) return null
    if (items.length && depth > items[items.length - 1].depth + 1) return null
    items.push({ depth, label })
    if (items.length > maximumMindmapNodes) return null
  }

  if (!items.length) return null
  const output = [`mindmap`, `  root(("${escapeMermaidLabel(rootLabel || 'Mindmap')}"))`]
  items.forEach((item, index) => {
    const indentation = '  '.repeat(item.depth + 2)
    output.push(`${indentation}item${index + 1}["${escapeMermaidLabel(item.label)}"]`)
  })
  return output.join('\n')
}

/** Backwards-compatible, shorter name for renderer integration. */
export const convertBulletMindmap = convertIndentedBulletMindmap

function escapeMermaidLabel(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/`/g, '&#96;')
    .replace(/\[/g, '&#91;')
    .replace(/\]/g, '&#93;')
    .replace(/[\u0000-\u001f\u007f]/g, ' ')
}
