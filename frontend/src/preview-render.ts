export const maximumMermaidCacheEntries = 64
export const maximumMermaidDiagramsPerPreview = 16

export interface MermaidMathDefinition {
  definition: string
  hasMath: boolean
}

function findSingleDollarEnd(line: string, start: number) {
  for (let index = start + 1; index < line.length; index += 1) {
    if (line[index] === '\\') {
      index += 1
      continue
    }
    if (line[index] !== '$') continue
    if (line[index - 1] === '$' || line[index + 1] === '$') continue
    return index
  }
  return -1
}

/**
 * Mermaid's KaTeX integration recognises `$$...$$`, while InkMark Markdown
 * uses `$...$` for inline formulas. Normalise the inline spelling only inside
 * diagram source and leave escaped dollars, directives, and comments intact.
 */
export function normalizeMermaidMath(definition: string): MermaidMathDefinition {
  let hasMath = false
  const normalized = definition.split('\n').map((line) => {
    if (line.trimStart().startsWith('%%')) return line

    let output = ''
    for (let index = 0; index < line.length;) {
      if (line[index] === '\\' && line[index + 1] === '$') {
        output += line.slice(index, index + 2)
        index += 2
        continue
      }
      if (line.startsWith('$$', index)) {
        const end = line.indexOf('$$', index + 2)
        if (end >= index + 3) {
          hasMath = true
          output += line.slice(index, end + 2)
          index = end + 2
          continue
        }
      }
      if (line[index] === '$') {
        const end = findSingleDollarEnd(line, index)
        if (end >= index + 2) {
          hasMath = true
          output += `$$${line.slice(index + 1, end)}$$`
          index = end + 1
          continue
        }
      }
      output += line[index]
      index += 1
    }
    return output
  }).join('\n')

  return { definition: normalized, hasMath }
}

export class BoundedCache<Key, Value> {
  private readonly entries = new Map<Key, Value>()
  private readonly limit: number

  constructor(limit: number) {
    this.limit = limit
  }

  get size() {
    return this.entries.size
  }

  get(key: Key): Value | undefined {
    const value = this.entries.get(key)
    if (value === undefined) return undefined

    // Refresh the insertion order so frequently used diagrams stay cached.
    this.entries.delete(key)
    this.entries.set(key, value)
    return value
  }

  set(key: Key, value: Value) {
    if (this.limit <= 0) return
    this.entries.delete(key)
    this.entries.set(key, value)
    while (this.entries.size > this.limit) {
      const oldest = this.entries.keys().next().value
      if (oldest === undefined) break
      this.entries.delete(oldest)
    }
  }

  clear() {
    this.entries.clear()
  }
}

export function mermaidCacheKey(theme: string, definition: string, htmlLabels = false) {
  return JSON.stringify([theme, definition, htmlLabels])
}

export class LatestPreviewCommit {
  private revision = 0

  invalidate() {
    this.revision += 1
    return this.revision
  }

  begin() {
    return this.invalidate()
  }

  isCurrent(revision: number) {
    return revision === this.revision
  }

  async stageAndCommit<Value>(
    revision: number,
    stage: () => Promise<Value> | Value,
    commit: (value: Value) => void,
  ) {
    const staged = await stage()
    if (!this.isCurrent(revision)) return false
    commit(staged)
    return true
  }
}
