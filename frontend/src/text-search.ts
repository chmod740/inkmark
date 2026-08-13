export type SearchDirection = 1 | -1

export interface TextSearchMatch {
  readonly start: number
  readonly end: number
  readonly ordinal: number
  readonly total: number
}

export interface TextSearchSegment {
  readonly text: string
  readonly highlighted: boolean
  readonly current: boolean
}

export const maximumSearchQueryLength = 1024
export const maximumHighlightedTextMatches = 2000

function searchPattern(query: string, caseSensitive: boolean): RegExp {
  const literal = query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return new RegExp(literal, caseSensitive ? 'gu' : 'giu')
}

export function normalizeSearchQuery(value: unknown): string {
  if (typeof value !== 'string') return ''
  return value.slice(0, maximumSearchQueryLength)
}

export function countTextMatches(source: string, rawQuery: unknown, caseSensitive = false): number {
  const query = normalizeSearchQuery(rawQuery)
  if (!query) return 0
  let count = 0
  const pattern = searchPattern(query, caseSensitive)
  while (pattern.exec(source)) count += 1
  return count
}

export function findTextMatch(
  source: string,
  rawQuery: unknown,
  from: number,
  direction: SearchDirection,
  caseSensitive = false,
): TextSearchMatch | null {
  const query = normalizeSearchQuery(rawQuery)
  if (!query) return null
  const startFrom = Math.max(0, Math.min(source.length, Number.isFinite(from) ? Math.trunc(from) : 0))
  const pattern = searchPattern(query, caseSensitive)
  let total = 0
  let firstStart = -1
  let lastStart = -1
  let selectedStart = -1
  let match: RegExpExecArray | null
  while ((match = pattern.exec(source))) {
    total += 1
    if (firstStart < 0) firstStart = match.index
    lastStart = match.index
    if (direction === 1 && selectedStart < 0 && match.index >= startFrom) selectedStart = match.index
    if (direction === -1 && match.index < startFrom) selectedStart = match.index
  }
  if (total === 0) return null
  if (selectedStart < 0) selectedStart = direction === 1 ? firstStart : lastStart

  const ordinalPattern = searchPattern(query, caseSensitive)
  let ordinal = 0
  while ((match = ordinalPattern.exec(source))) {
    ordinal += 1
    if (match.index === selectedStart) break
  }
  return {
    start: selectedStart,
    end: selectedStart + query.length,
    ordinal,
    total,
  }
}

export function segmentTextMatches(
  source: string,
  rawQuery: unknown,
  currentStart = -1,
  caseSensitive = false,
): readonly TextSearchSegment[] {
  const query = normalizeSearchQuery(rawQuery)
  if (!query) return [{ text: source, highlighted: false, current: false }]
  const pattern = searchPattern(query, caseSensitive)
  const positions: Array<{ start: number, end: number, current: boolean }> = []
  let currentPosition: { start: number, end: number, current: boolean } | null = null
  let match: RegExpExecArray | null
  while ((match = pattern.exec(source))) {
    const position = { start: match.index, end: match.index + match[0].length, current: match.index === currentStart }
    if (position.current) currentPosition = position
    if (positions.length < maximumHighlightedTextMatches) positions.push(position)
  }
  if (currentPosition && !positions.some((position) => position.current)) {
    positions[positions.length - 1] = currentPosition
    positions.sort((left, right) => left.start - right.start)
  }
  if (positions.length === 0) return [{ text: source, highlighted: false, current: false }]

  const segments: TextSearchSegment[] = []
  let offset = 0
  for (const position of positions) {
    if (position.start > offset) {
      segments.push({ text: source.slice(offset, position.start), highlighted: false, current: false })
    }
    segments.push({ text: source.slice(position.start, position.end), highlighted: true, current: position.current })
    offset = position.end
  }
  if (offset < source.length) segments.push({ text: source.slice(offset), highlighted: false, current: false })
  return segments
}
