export type SearchDirection = 1 | -1

export interface TextSearchMatch {
  readonly start: number
  readonly end: number
  readonly ordinal: number
  readonly total: number
}

export const maximumSearchQueryLength = 1024

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
