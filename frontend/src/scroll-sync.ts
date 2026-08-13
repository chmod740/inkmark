export type ScrollPane = 'editor' | 'preview'

export interface ScrollViewport {
  scrollTop: number
  scrollHeight: number
  clientHeight: number
}

export interface ScrollAnchor {
  line: number
  top: number
}

export interface ScrollMapping {
  sourceAnchors: readonly ScrollAnchor[]
  targetAnchors: readonly ScrollAnchor[]
  maxLine: number
}

const scrollTolerance = 0.5

export const maximumScrollAnchors = 8192

export function sampleAnchorIndices(length: number, limit = maximumScrollAnchors) {
  const count = Math.max(0, Math.floor(length))
  const maximum = Math.max(0, Math.floor(limit))
  if (!count || !maximum) return []
  if (count <= maximum) return Array.from({ length: count }, (_, index) => index)
  if (maximum === 1) return [0]

  const indices: number[] = []
  for (let index = 0; index < maximum; index += 1) {
    indices.push(Math.round(index * (count - 1) / (maximum - 1)))
  }
  return indices
}

export function editorLineAnchors(lines: readonly number[], lineHeight: number, paddingTop: number) {
  if (!Number.isFinite(lineHeight) || lineHeight <= 0 || !Number.isFinite(paddingTop)) return []
  return [...new Set(lines)]
    .filter((line) => Number.isFinite(line) && line >= 0)
    .sort((left, right) => left - right)
    .slice(0, maximumScrollAnchors)
    .map((line) => ({ line, top: Math.max(0, paddingTop) + line * lineHeight }))
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.min(maximum, Math.max(minimum, value))
}

function interpolate(
  value: number,
  points: readonly ScrollAnchor[],
  input: 'top' | 'line',
  output: 'top' | 'line',
) {
  const sorted = [...points].sort((left, right) => left[input] - right[input] || left[output] - right[output])
  if (!sorted.length) return 0
  if (value <= sorted[0][input]) return sorted[0][output]

  for (let index = 1; index < sorted.length; index += 1) {
    const right = sorted[index]
    if (value > right[input]) continue
    const left = sorted[index - 1]
    const span = right[input] - left[input]
    if (span <= 0) return right[output]
    const ratio = (value - left[input]) / span
    return left[output] + (right[output] - left[output]) * ratio
  }
  return sorted[sorted.length - 1][output]
}

function anchoredPoints(anchors: readonly ScrollAnchor[], range: number, maxLine: number) {
  const points = anchors
    .filter((anchor) => Number.isFinite(anchor.line) && Number.isFinite(anchor.top))
    .map((anchor) => ({
      line: clamp(anchor.line, 0, maxLine),
      top: clamp(anchor.top, 0, range),
    }))
  points.push({ line: 0, top: 0 }, { line: maxLine, top: range })
  return points
}

export function mapAnchoredScrollTop(
  sourceTop: number,
  sourceRange: number,
  targetRange: number,
  mapping: ScrollMapping,
) {
  if (sourceRange <= 0 || targetRange <= 0 || mapping.maxLine <= 0) return 0
  const sourcePoints = anchoredPoints(mapping.sourceAnchors, sourceRange, mapping.maxLine)
  const targetPoints = anchoredPoints(mapping.targetAnchors, targetRange, mapping.maxLine)
  const line = interpolate(clamp(sourceTop, 0, sourceRange), sourcePoints, 'top', 'line')
  return clamp(interpolate(line, targetPoints, 'line', 'top'), 0, targetRange)
}

export class ScrollSyncController {
  private activePane: ScrollPane | null = null

  begin(pane: ScrollPane) {
    this.activePane = pane
  }

  active() {
    return this.activePane
  }

  reset() {
    this.activePane = null
  }

  sync(pane: ScrollPane, source: ScrollViewport, target: ScrollViewport, mapping?: ScrollMapping) {
    // Scroll events caused by writing target.scrollTop must never become a
    // second source event. Only the pane with current user intent may drive.
    if (this.activePane !== pane) return false

    const sourceRange = Math.max(0, source.scrollHeight - source.clientHeight)
    const targetRange = Math.max(0, target.scrollHeight - target.clientHeight)
    if (sourceRange === 0 || targetRange === 0) return false

    const nextTop = mapping
      ? mapAnchoredScrollTop(source.scrollTop, sourceRange, targetRange, mapping)
      : clamp(source.scrollTop / sourceRange, 0, 1) * targetRange
    if (Math.abs(target.scrollTop - nextTop) <= scrollTolerance) return false

    target.scrollTop = nextTop
    return true
  }
}
