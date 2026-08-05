export const maximumMermaidCacheEntries = 64

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

export function mermaidCacheKey(theme: string, definition: string) {
  return JSON.stringify([theme, definition])
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
