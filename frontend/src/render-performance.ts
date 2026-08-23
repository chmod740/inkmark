export type PreviewRenderPhase =
  | 'queued'
  | 'worker'
  | 'sanitize'
  | 'core'
  | 'commit'
  | 'enhance'
  | 'total'

export interface PreviewRenderMeasurement {
  readonly id: number
  readonly tabID: string
  readonly sourceBytes: number
  readonly cacheHit: boolean
  readonly chunked: boolean
  readonly phases: Readonly<Record<PreviewRenderPhase, number>>
  readonly timestamp: number
}

const maximumMeasurements = 120

function now() {
  return typeof performance !== 'undefined' ? performance.now() : Date.now()
}

export class PreviewRenderMeasurementSession {
  private readonly startedAt: number
  private readonly marks: Map<PreviewRenderPhase, number>
  private finished = false
  private readonly owner: PreviewPerformanceMetrics
  private readonly id: number
  private readonly tabID: string
  private readonly sourceBytes: number
  private readonly cacheHit: boolean
  private chunked: boolean

  constructor(
    owner: PreviewPerformanceMetrics,
    id: number,
    tabID: string,
    sourceBytes: number,
    cacheHit: boolean,
    chunked: boolean,
    startedAt = now(),
  ) {
    this.owner = owner
    this.id = id
    this.tabID = tabID
    this.sourceBytes = sourceBytes
    this.cacheHit = cacheHit
    this.chunked = chunked
    this.startedAt = startedAt
    this.marks = new Map([['queued', this.startedAt]])
  }

  mark(phase: PreviewRenderPhase) {
    if (!this.finished) this.marks.set(phase, now())
  }

  setChunked(chunked: boolean) {
    if (!this.finished) this.chunked = Boolean(chunked)
  }

  finish() {
    if (this.finished) return
    this.finished = true
    const endedAt = now()
    const at = (phase: PreviewRenderPhase) => this.marks.get(phase) ?? endedAt
    const phases: Record<PreviewRenderPhase, number> = {
      queued: Math.max(0, at('worker') - this.startedAt),
      worker: Math.max(0, at('sanitize') - at('worker')),
      sanitize: Math.max(0, at('core') - at('sanitize')),
      core: Math.max(0, at('commit') - at('core')),
      commit: Math.max(0, at('enhance') - at('commit')),
      enhance: Math.max(0, endedAt - at('enhance')),
      total: Math.max(0, endedAt - this.startedAt),
    }
    this.owner.record({
      id: this.id,
      tabID: this.tabID,
      sourceBytes: this.sourceBytes,
      cacheHit: this.cacheHit,
      chunked: this.chunked,
      phases,
      timestamp: Date.now(),
    })
  }
}

export class PreviewPerformanceMetrics {
  private measurements: PreviewRenderMeasurement[] = []
  private nextID = 1

  begin(
    tabID: string,
    source: string,
    options: { cacheHit?: boolean, chunked?: boolean, startedAt?: number } = {},
  ) {
    return new PreviewRenderMeasurementSession(
      this,
      this.nextID++,
      tabID,
      new TextEncoder().encode(source).byteLength,
      Boolean(options.cacheHit),
      Boolean(options.chunked),
      Number.isFinite(options.startedAt) ? options.startedAt : undefined,
    )
  }

  record(measurement: PreviewRenderMeasurement) {
    this.measurements.push(measurement)
    if (this.measurements.length > maximumMeasurements) this.measurements.splice(0, this.measurements.length - maximumMeasurements)
  }

  snapshot() {
    return this.measurements.map((measurement) => ({ ...measurement, phases: { ...measurement.phases } }))
  }

  clear() {
    this.measurements = []
  }
}
