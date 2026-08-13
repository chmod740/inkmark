import MarkdownIt from 'markdown-it'
import { full as fullEmoji } from 'markdown-it-emoji'
import footnote from 'markdown-it-footnote'

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

const calloutTypes = new Set<CalloutType>(['note', 'tip', 'important', 'warning', 'caution'])
const calloutMarkerPattern = /^[ \t]*(?:\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]|(NOTE|TIP|IMPORTANT|WARNING|CAUTION)[ \t]*:)(?:[ \t]+|\r?\n|$)/i
const mentionPattern = /@[A-Za-z0-9](?:[A-Za-z0-9_-]{0,37}[A-Za-z0-9])?/g
const invalidMentionPrefixPattern = /[A-Za-z0-9.!#$%&'*+/=?^_`{|}~@/-]/
const invalidMentionSuffixPattern = /[A-Za-z0-9_@-]/

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
  return markdown
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
