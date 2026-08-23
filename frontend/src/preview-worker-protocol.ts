import { deriveSourceData, type SourceDerivedData } from './editor-features.ts'

export const largePreviewChunkThreshold = 768 * 1024
export const previewChunkTargetCharacters = 96 * 1024

export interface PreviewMarkdownChunk {
  readonly source: string
  readonly lineOffset: number
}

export interface PreviewWorkerRequest {
  readonly id: number
  readonly source: string
  readonly sourceRevision: number
  readonly trustedMarker: string
}

export type PreviewWorkerResponse =
  | { readonly type: 'chunk', readonly id: number, readonly html: string, readonly index: number }
  | {
    readonly type: 'complete'
    readonly id: number
    readonly html: string
    readonly chunked: boolean
    readonly derived: SourceDerivedData
  }
  | { readonly type: 'error', readonly id: number, readonly message: string }

/**
 * Chunk only documents whose semantics are independent at blank lines. This
 * conservative gate preserves correctness for front matter, references,
 * footnotes, tables, lists, quotes, HTML blocks and fenced code; those still
 * use the single worker render path.
 */
export function splitMarkdownIntoSafeChunks(source: string): readonly PreviewMarkdownChunk[] | null {
  if (source.length < largePreviewChunkThreshold) return null
  if (/^(?:---|\.\.\.)\s*$|^ {0,3}(?:[`~]{3,}|[-+*] |\d+[.)] |> |<)|^\[\^|^\[[^\]]+\]:|\[\[|\|/mu.test(source)) return null

  const lines = source.split(/(?<=\n)/u)
  const chunks: PreviewMarkdownChunk[] = []
  let lineOffset = 0
  let current = ''
  let currentOffset = 0
  for (const line of lines) {
    current += line
    lineOffset += 1
    const boundary = /^\s*$/u.test(line) || /^ {0,3}#{1,6}(?:[ \t]|$)/u.test(line)
    if (current.length < previewChunkTargetCharacters || !boundary) continue
    chunks.push({ source: current, lineOffset: currentOffset })
    current = ''
    currentOffset = lineOffset
  }
  if (current) chunks.push({ source: current, lineOffset: currentOffset })
  return chunks.length > 1 ? chunks : null
}

export function derivePreviewSource(source: string, sourceRevision: number) {
  return deriveSourceData(source, sourceRevision)
}
