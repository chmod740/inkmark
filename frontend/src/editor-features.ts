export interface EditorPreferences {
  readonly version: 1
  readonly lineNumbers: boolean
  readonly stickyHeadings: boolean
}

export interface LineNumberModel {
  readonly count: number
  readonly digits: number
  readonly text: string
  readonly available: boolean
}

export interface SourceHeading {
  readonly line: number
  readonly level: number
  readonly raw: string
  readonly parent: number
}

export interface SourceHeadingModel {
  readonly headings: readonly SourceHeading[]
  readonly available: boolean
}

export const editorPreferencesStorageKey = 'inkmark-editor-preferences-v1'
export const maximumRenderedLineNumbers = 100_000
export const maximumStickySourceHeadings = 10_000
export const defaultEditorPreferences: EditorPreferences = Object.freeze({
  version: 1,
  lineNumbers: true,
  stickyHeadings: true,
})

function clonedDefaults(): EditorPreferences {
  return { ...defaultEditorPreferences }
}

export function normalizeEditorPreferences(value: unknown): EditorPreferences {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return clonedDefaults()
  const record = value as Record<string, unknown>
  const keys = Object.keys(record).sort()
  if (keys.join(',') !== 'lineNumbers,stickyHeadings,version') return clonedDefaults()
  if (record.version !== 1 || typeof record.lineNumbers !== 'boolean' || typeof record.stickyHeadings !== 'boolean') {
    return clonedDefaults()
  }
  return {
    version: 1,
    lineNumbers: record.lineNumbers,
    stickyHeadings: record.stickyHeadings,
  }
}

export function parseEditorPreferences(raw: unknown): EditorPreferences {
  if (typeof raw !== 'string' || raw.length > 2_048) return clonedDefaults()
  try {
    return normalizeEditorPreferences(JSON.parse(raw))
  } catch {
    return clonedDefaults()
  }
}

export function readEditorPreferences(storage: Pick<Storage, 'getItem'>): EditorPreferences {
  return parseEditorPreferences(storage.getItem(editorPreferencesStorageKey))
}

export function writeEditorPreferences(storage: Pick<Storage, 'setItem'>, preferences: EditorPreferences) {
  storage.setItem(editorPreferencesStorageKey, JSON.stringify(normalizeEditorPreferences(preferences)))
}

export function updateEditorPreference(
  preferences: EditorPreferences,
  key: 'lineNumbers' | 'stickyHeadings',
  enabled: boolean,
): EditorPreferences {
  const normalized = normalizeEditorPreferences(preferences)
  return { ...normalized, [key]: Boolean(enabled) }
}

export function createLineNumberModel(
  source: string,
  maximum = maximumRenderedLineNumbers,
): LineNumberModel {
  const limit = Math.max(1, Number.isFinite(maximum) ? Math.trunc(maximum) : maximumRenderedLineNumbers)
  const numbers = ['1']
  let count = 1
  let offset = 0
  while ((offset = source.indexOf('\n', offset)) >= 0) {
    count += 1
    offset += 1
    if (count <= limit) numbers.push(String(count))
    else if (count === limit + 1) numbers.length = 0
  }
  const available = count <= limit
  return {
    count,
    digits: String(count).length,
    text: available ? numbers.join('\n') : '',
    available,
  }
}

function* sourceLines(source: string): Generator<[number, string]> {
  let line = 0
  let offset = 0
  while (offset <= source.length) {
    const nextBreak = source.indexOf('\n', offset)
    if (nextBreak < 0) {
      yield [line, source.slice(offset)]
      return
    }
    yield [line, source.slice(offset, nextBreak)]
    line += 1
    offset = nextBreak + 1
  }
}

export function parseSourceHeadings(
  source: string,
  maximum = maximumStickySourceHeadings,
): SourceHeadingModel {
  const limit = Math.max(1, Number.isFinite(maximum) ? Math.trunc(maximum) : maximumStickySourceHeadings)
  const headings: SourceHeading[] = []
  const hierarchy: number[] = []
  let fenceCharacter = ''
  let fenceLength = 0

  for (const [line, sourceLine] of sourceLines(source)) {
    const text = sourceLine.endsWith('\r') ? sourceLine.slice(0, -1) : sourceLine
    const fence = text.match(/^ {0,3}(`{3,}|~{3,})(.*)$/u)
    if (fence) {
      const character = fence[1][0]
      if (!fenceCharacter) {
        fenceCharacter = character
        fenceLength = fence[1].length
      } else if (character === fenceCharacter && fence[1].length >= fenceLength && !fence[2].trim()) {
        fenceCharacter = ''
        fenceLength = 0
      }
      continue
    }
    if (fenceCharacter) continue

    const match = text.match(/^ {0,3}(#{1,6})(?:[ \t]+|$)(.*)$/u)
    if (!match) continue
    const level = match[1].length
    const raw = text.trimEnd().slice(0, 1_024)
    while (hierarchy.length && headings[hierarchy[hierarchy.length - 1]].level >= level) hierarchy.pop()
    const parent = hierarchy.length ? hierarchy[hierarchy.length - 1] : -1
    headings.push({ line, level, raw, parent })
    hierarchy.push(headings.length - 1)
    if (headings.length > limit) return { headings: [], available: false }
  }
  return { headings, available: true }
}

export function stickyHeadingTrail(headings: readonly SourceHeading[], line: number): readonly SourceHeading[] {
  const targetLine = Math.max(0, Number.isFinite(line) ? Math.trunc(line) : 0)
  let left = 0
  let right = headings.length - 1
  let current = -1
  while (left <= right) {
    const middle = Math.floor((left + right) / 2)
    if (headings[middle].line <= targetLine) {
      current = middle
      left = middle + 1
    } else {
      right = middle - 1
    }
  }
  const trail: SourceHeading[] = []
  while (current >= 0) {
    trail.unshift(headings[current])
    current = headings[current].parent
  }
  return trail
}

export function sourceLineFromScroll(scrollTop: number, lineHeight: number, paddingTop: number) {
  if (!Number.isFinite(scrollTop) || !Number.isFinite(lineHeight) || lineHeight <= 0) return 0
  return Math.max(0, Math.floor((Math.max(0, scrollTop) - Math.max(0, paddingTop)) / lineHeight))
}
