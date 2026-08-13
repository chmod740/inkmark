import DOMPurify from 'dompurify'
import type { EChartsOption } from 'echarts'

export type ExtendedDiagramKind = 'echarts' | 'abc' | 'graphviz'
export type EChartsRenderer = 'svg' | 'canvas'

export const maximumExtendedDiagrams = 16
export const maximumEChartsCharacters = 64 * 1024
export const maximumEChartsSeries = 8
export const maximumEChartsPointsPerSeries = 2048
export const maximumEChartsTotalPoints = 8192
export const maximumABCCharacters = 32 * 1024
export const maximumABCLines = 512
export const maximumGraphvizCharacters = 64 * 1024
export const maximumGraphvizEdges = 1024
export const maximumGraphvizStatements = 2048
export const maximumGraphvizTokens = 2048
export const maximumGraphvizIdentifiers = 1024
export const maximumGraphvizDiagrams = 2
export const maximumRenderedSVGCharacters = 2 * 1024 * 1024
export const maximumGraphvizRenderMilliseconds = 5_000
export const maximumExtendedDiagramGenerationMilliseconds = 12_000

const minimumChartWidth = 320
const maximumChartWidth = 1600
const minimumChartHeight = 240
const maximumChartHeight = 900
const maximumSVGDimension = 10_000
const maximumSVGArea = 16_000_000
const unsafeSchemePattern = /(?:^|[\s"'(])(?:https?|ftp|file|data|javascript):/i
const graphvizTokenPattern = /"(?:\\.|[^"\\])*"|(?:->|--)|[\p{L}_][\p{L}\p{N}_]*|[+-]?(?:\d+(?:\.\d*)?|\.\d+)|[{}[\];=,:]/gu
const graphvizIdentifierPattern = /^(?:[\p{L}_][\p{L}\p{N}_]*|[+-]?(?:\d+(?:\.\d*)?|\.\d+))$/u
const graphvizWhitespaceOrCommentPattern = /^(?:\s+|\/\*[\s\S]*?\*\/|\/\/[^\r\n]*(?:\r?\n|$))+$/u

type PlainRecord = Record<string, unknown>

export interface SafeEChartsLineOption {
  animation: false
  title?: { text: string }
  tooltip?: { trigger: 'axis'; axisPointer?: { type?: 'line'; lineStyle?: { width?: number } } }
  legend?: { data: string[] }
  xAxis: { type: 'category'; boundaryGap?: boolean; data: string[]; axisTick?: { show?: boolean }; axisLine?: { show?: boolean } }
  yAxis: { type: 'value'; axisTick?: { show?: boolean }; axisLine?: { show?: boolean }; splitLine?: { show?: boolean; lineStyle?: { color?: string; type?: 'solid' | 'dashed' | 'dotted' } } }
  series: Array<{
    type: 'line'
    name?: string
    smooth?: boolean
    itemStyle?: { color: string }
    areaStyle?: { opacity?: number }
    z?: number
    data: number[]
  }>
}

export interface ExtendedDiagramRenderOptions {
  signal?: AbortSignal
  isCurrent?: () => boolean
  echartsRenderer?: EChartsRenderer
  chartWidth?: number
  chartHeight?: number
}

export interface ExtendedDiagramRenderResult {
  rendered: number
  failed: number
  dispose: () => void
}

function fail(message: string): never {
  throw new Error(message)
}

function isPlainRecord(value: unknown): value is PlainRecord {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function assertKeys(value: PlainRecord, allowed: readonly string[], context: string): void {
  const allowedKeys = new Set(allowed)
  const unexpected = Object.keys(value).find((key) => !allowedKeys.has(key))
  if (unexpected) fail(`${context} contains unsupported field "${unexpected}"`)
}

function boundedText(value: unknown, context: string, maximum = 200): string {
  if (typeof value !== 'string' || !value.trim() || value.length > maximum || /[\u0000-\u001f\u007f]/u.test(value)) {
    fail(`${context} must be non-empty plain text`)
  }
  return value
}

function stringList(value: unknown, context: string, maximumItems: number): string[] {
  if (!Array.isArray(value) || !value.length || value.length > maximumItems) fail(`${context} has an invalid item count`)
  return value.map((item, index) => boundedText(item, `${context}[${index}]`, 100))
}

function finiteNumber(value: unknown, context: string): number {
  const parsed = typeof value === 'number'
    ? value
    : typeof value === 'string' && value.trim() && /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?$/i.test(value.trim())
      ? Number(value)
      : Number.NaN
  if (!Number.isFinite(parsed) || Math.abs(parsed) > 1e12) fail(`${context} must be a finite bounded number`)
  return parsed
}

function safeColor(value: unknown, context: string): string {
  if (typeof value !== 'string' || !/^(?:#[0-9a-f]{3,8}|rgba?\(\s*\d{1,3}(?:\s*,\s*\d{1,3}){2}(?:\s*,\s*(?:0|1|0?\.\d+|\.\d+))?\s*\)|[a-z]{1,20})$/i.test(value)) {
    fail(`${context} must be a plain color`)
  }
  return value
}

function oneObject(value: unknown, context: string): PlainRecord {
  const candidate = Array.isArray(value) && value.length === 1 ? value[0] : value
  if (!isPlainRecord(candidate)) fail(`${context} must be an object or a one-item array`)
  return candidate
}

function optionalShow(value: unknown, context: string): { show?: boolean } {
  const record = oneObject(value, context)
  assertKeys(record, ['show'], context)
  if (record.show !== undefined && typeof record.show !== 'boolean') fail(`${context}.show must be boolean`)
  return record.show === undefined ? {} : { show: record.show }
}

/** Parse JSON and rebuild only the inert subset used by the line chart in 未命名.md. */
export function parseSafeEChartsOption(source: string): SafeEChartsLineOption {
  if (!source.trim() || source.length > maximumEChartsCharacters) fail('ECharts definition exceeds the size limit')
  let parsed: unknown
  try {
    parsed = JSON.parse(source)
  } catch {
    fail('ECharts definition must be valid JSON')
  }
  if (!isPlainRecord(parsed)) fail('ECharts definition must be a JSON object')
  assertKeys(parsed, ['title', 'tooltip', 'legend', 'xAxis', 'yAxis', 'series'], 'ECharts option')

  const option: SafeEChartsLineOption = {
    animation: false,
    xAxis: { type: 'category', data: [] },
    yAxis: { type: 'value' },
    series: [],
  }

  if (parsed.title !== undefined) {
    if (!isPlainRecord(parsed.title)) fail('ECharts title must be an object')
    assertKeys(parsed.title, ['text'], 'ECharts title')
    option.title = { text: boundedText(parsed.title.text, 'ECharts title') }
  }
  if (parsed.tooltip !== undefined) {
    if (!isPlainRecord(parsed.tooltip)) fail('ECharts tooltip must be an object')
    assertKeys(parsed.tooltip, ['trigger', 'axisPointer'], 'ECharts tooltip')
    if (parsed.tooltip.trigger !== 'axis') fail('Only axis tooltips are supported')
    option.tooltip = { trigger: 'axis' }
    if (parsed.tooltip.axisPointer !== undefined) {
      const pointer = oneObject(parsed.tooltip.axisPointer, 'ECharts axisPointer')
      assertKeys(pointer, ['type', 'lineStyle'], 'ECharts axisPointer')
      const safePointer: NonNullable<SafeEChartsLineOption['tooltip']>['axisPointer'] = {}
      if (pointer.type !== undefined) {
        if (pointer.type !== 'line') fail('Only line ECharts axis pointers are supported')
        safePointer.type = 'line'
      }
      if (pointer.lineStyle !== undefined) {
        const style = oneObject(pointer.lineStyle, 'ECharts axisPointer lineStyle')
        assertKeys(style, ['width'], 'ECharts axisPointer lineStyle')
        if (style.width !== undefined) {
          const width = finiteNumber(style.width, 'ECharts axisPointer lineStyle width')
          if (width < 0 || width > 10) fail('ECharts axisPointer line width exceeds the limit')
          safePointer.lineStyle = { width }
        }
      }
      option.tooltip.axisPointer = safePointer
    }
  }
  if (parsed.legend !== undefined) {
    if (!isPlainRecord(parsed.legend)) fail('ECharts legend must be an object')
    assertKeys(parsed.legend, ['data'], 'ECharts legend')
    option.legend = { data: stringList(parsed.legend.data, 'ECharts legend data', maximumEChartsSeries) }
  }

  const xAxis = oneObject(parsed.xAxis, 'ECharts xAxis')
  assertKeys(xAxis, ['type', 'boundaryGap', 'data', 'axisTick', 'axisLine'], 'ECharts xAxis')
  if (xAxis.type !== 'category') fail('Only a category xAxis is supported')
  option.xAxis.data = stringList(xAxis.data, 'ECharts xAxis data', maximumEChartsPointsPerSeries)
  if (xAxis.boundaryGap !== undefined) {
    if (typeof xAxis.boundaryGap !== 'boolean') fail('ECharts boundaryGap must be boolean')
    option.xAxis.boundaryGap = xAxis.boundaryGap
  }
  if (xAxis.axisTick !== undefined) option.xAxis.axisTick = optionalShow(xAxis.axisTick, 'ECharts xAxis axisTick')
  if (xAxis.axisLine !== undefined) option.xAxis.axisLine = optionalShow(xAxis.axisLine, 'ECharts xAxis axisLine')

  const yAxis = oneObject(parsed.yAxis, 'ECharts yAxis')
  assertKeys(yAxis, ['type', 'axisTick', 'axisLine', 'splitLine'], 'ECharts yAxis')
  if (yAxis.type !== 'value') fail('Only a value yAxis is supported')
  if (yAxis.axisTick !== undefined) option.yAxis.axisTick = optionalShow(yAxis.axisTick, 'ECharts yAxis axisTick')
  if (yAxis.axisLine !== undefined) option.yAxis.axisLine = optionalShow(yAxis.axisLine, 'ECharts yAxis axisLine')
  if (yAxis.splitLine !== undefined) {
    const splitLine = oneObject(yAxis.splitLine, 'ECharts yAxis splitLine')
    assertKeys(splitLine, ['show', 'lineStyle'], 'ECharts yAxis splitLine')
    const safeSplitLine: NonNullable<SafeEChartsLineOption['yAxis']['splitLine']> = {}
    if (splitLine.show !== undefined) {
      if (typeof splitLine.show !== 'boolean') fail('ECharts yAxis splitLine.show must be boolean')
      safeSplitLine.show = splitLine.show
    }
    if (splitLine.lineStyle !== undefined) {
      const lineStyle = oneObject(splitLine.lineStyle, 'ECharts yAxis splitLine lineStyle')
      assertKeys(lineStyle, ['color', 'type'], 'ECharts yAxis splitLine lineStyle')
      const safeLineStyle: NonNullable<NonNullable<SafeEChartsLineOption['yAxis']['splitLine']>['lineStyle']> = {}
      if (lineStyle.color !== undefined) safeLineStyle.color = safeColor(lineStyle.color, 'ECharts split line color')
      if (lineStyle.type !== undefined) {
        if (!['solid', 'dashed', 'dotted'].includes(String(lineStyle.type))) fail('Unsupported ECharts split line type')
        safeLineStyle.type = lineStyle.type as 'solid' | 'dashed' | 'dotted'
      }
      safeSplitLine.lineStyle = safeLineStyle
    }
    option.yAxis.splitLine = safeSplitLine
  }

  if (!Array.isArray(parsed.series) || !parsed.series.length || parsed.series.length > maximumEChartsSeries) {
    fail('ECharts series count exceeds the limit')
  }
  let totalPoints = 0
  option.series = parsed.series.map((rawSeries, seriesIndex) => {
    if (!isPlainRecord(rawSeries)) fail(`ECharts series[${seriesIndex}] must be an object`)
    assertKeys(rawSeries, ['type', 'name', 'smooth', 'itemStyle', 'areaStyle', 'z', 'data'], `ECharts series[${seriesIndex}]`)
    if (rawSeries.type !== 'line') fail('Only line series are supported')
    if (!Array.isArray(rawSeries.data) || !rawSeries.data.length || rawSeries.data.length > maximumEChartsPointsPerSeries) {
      fail(`ECharts series[${seriesIndex}] data exceeds the limit`)
    }
    if (rawSeries.data.length !== option.xAxis.data.length) fail(`ECharts series[${seriesIndex}] data length does not match xAxis`)
    totalPoints += rawSeries.data.length
    if (totalPoints > maximumEChartsTotalPoints) fail('ECharts total data exceeds the limit')

    const series: SafeEChartsLineOption['series'][number] = {
      type: 'line',
      data: rawSeries.data.map((item, dataIndex) => finiteNumber(item, `ECharts series[${seriesIndex}].data[${dataIndex}]`)),
    }
    if (rawSeries.name !== undefined) series.name = boundedText(rawSeries.name, `ECharts series[${seriesIndex}] name`, 100)
    if (rawSeries.smooth !== undefined) {
      if (typeof rawSeries.smooth !== 'boolean') fail('ECharts smooth must be boolean')
      series.smooth = rawSeries.smooth
    }
    if (rawSeries.itemStyle !== undefined) {
      if (!isPlainRecord(rawSeries.itemStyle)) fail('ECharts itemStyle must be an object')
      assertKeys(rawSeries.itemStyle, ['color'], 'ECharts itemStyle')
      series.itemStyle = { color: safeColor(rawSeries.itemStyle.color, 'ECharts itemStyle color') }
    }
    if (rawSeries.areaStyle !== undefined) {
      if (!isPlainRecord(rawSeries.areaStyle)) fail('ECharts areaStyle must be an object')
      assertKeys(rawSeries.areaStyle, ['normal', 'opacity'], 'ECharts areaStyle')
      if (rawSeries.areaStyle.normal !== undefined) {
        if (!isPlainRecord(rawSeries.areaStyle.normal)) fail('ECharts areaStyle.normal must be an object')
        assertKeys(rawSeries.areaStyle.normal, [], 'ECharts areaStyle.normal')
      }
      series.areaStyle = {}
      if (rawSeries.areaStyle.opacity !== undefined) {
        const opacity = finiteNumber(rawSeries.areaStyle.opacity, 'ECharts areaStyle opacity')
        if (opacity < 0 || opacity > 1) fail('ECharts areaStyle opacity must be between 0 and 1')
        series.areaStyle.opacity = opacity
      }
    }
    if (rawSeries.z !== undefined) {
      const z = finiteNumber(rawSeries.z, `ECharts series[${seriesIndex}] z`)
      if (!Number.isSafeInteger(z) || Math.abs(z) > 100) fail('ECharts z must be a bounded integer')
      series.z = z
    }
    return series
  })

  if (option.legend && option.legend.data.some((name) => !option.series.some((series) => series.name === name))) {
    fail('Every ECharts legend item must name a series')
  }
  return option
}

/** Validate a single, visual-only ABC tune. Audio APIs are never imported or invoked. */
export function validateABCSource(source: string): string {
  const normalized = source.replace(/\r\n?/g, '\n')
  const lines = normalized.split('\n')
  if (!normalized.trim() || normalized.length > maximumABCCharacters || lines.length > maximumABCLines) {
    fail('ABC definition exceeds the size limit')
  }
  if (/\u0000|[\u0001-\u0008\u000b\u000c\u000e-\u001f\u007f]/u.test(normalized)) fail('ABC definition contains control characters')
  if (lines.some((line) => line.length > 1024)) fail('ABC line exceeds the size limit')
  if (!/^X:\s*\d+\s*$/m.test(normalized) || !/^K:\s*\S+/m.test(normalized)) fail('ABC definition requires X and K headers')
  if ((normalized.match(/^X:/gm) || []).length !== 1) fail('Only one ABC tune is supported per fence')
  // Disable parser directives, macros and metadata fields which are unrelated
  // to static notation and could request MIDI behavior or carry URL metadata.
  if (/^\s*%%/m.test(normalized) || /^\s*[FIU]:/m.test(normalized) || unsafeSchemePattern.test(normalized)) {
    fail('ABC definition contains an unsupported directive or resource reference')
  }
  return normalized
}

/** Reject DOT features capable of naming external resources before WebAssembly sees the source. */
export function validateGraphvizSource(source: string): string {
  const normalized = source.replace(/\r\n?/g, '\n')
  if (!normalized.trim() || normalized.length > maximumGraphvizCharacters) fail('Graphviz definition exceeds the size limit')
  if (/\u0000|[\u0001-\u0008\u000b\u000c\u000e-\u001f\u007f]/u.test(normalized)) fail('Graphviz definition contains control characters')
  if (!/^\s*(?:strict\s+)?(?:di)?graph\b/i.test(normalized) || !/[{}]/.test(normalized)) fail('Graphviz definition must be a DOT graph')
  if ((normalized.match(/(?:->|--)/g) || []).length > maximumGraphvizEdges) fail('Graphviz edge count exceeds the limit')
  if ((normalized.match(/[;\n]/g) || []).length > maximumGraphvizStatements) fail('Graphviz statement count exceeds the limit')
  if (/^\s*#/m.test(normalized)) fail('Graphviz preprocessor lines are not supported')
  if (unsafeSchemePattern.test(normalized)) fail('Graphviz external resource references are not supported')
  if (/["']?\b(?:image|imagescale|shapefile|href|url|edgehref|headhref|tailhref|labelhref|stylesheet|fontpath)["']?\s*=/i.test(normalized)) {
    fail('Graphviz external resource attributes are not supported')
  }
  if (/<\s*(?:img|image|a)\b/i.test(normalized) || /\b(?:src|href)\s*=/i.test(normalized)) {
    fail('Graphviz HTML resource elements are not supported')
  }
  const tokens = normalized.match(graphvizTokenPattern) || []
  const unmatched = normalized.split(graphvizTokenPattern).join('')
  if (unmatched && !graphvizWhitespaceOrCommentPattern.test(unmatched)) fail('Graphviz definition contains unsupported tokens')
  if (tokens.length > maximumGraphvizTokens) fail('Graphviz token count exceeds the limit')
  if (tokens.filter((token) => graphvizIdentifierPattern.test(token)).length > maximumGraphvizIdentifiers) {
    fail('Graphviz identifier count exceeds the limit')
  }
  const size = /["']?\bsize["']?\s*=\s*["']?\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)/i.exec(normalized)
  if (size && (Number(size[1]) > 32 || Number(size[2]) > 32)) fail('Graphviz requested size exceeds the limit')
  return normalized
}

function ensureActive(options: ExtendedDiagramRenderOptions): void {
  if (options.signal?.aborted || options.isCurrent?.() === false) throw new DOMException('The operation was aborted', 'AbortError')
}

function boundedDimension(value: number | undefined, fallback: number, minimum: number, maximum: number): number {
  return Number.isFinite(value) ? Math.min(maximum, Math.max(minimum, Math.round(value!))) : fallback
}

function createGraphvizWorker(): Worker {
  if (import.meta.env.DEV) {
    return new Worker(new URL('./graphviz-worker.ts', import.meta.url), {
      type: 'module',
      name: 'inkmark-graphviz',
    })
  }
  // Production uses a self-contained classic worker so Graphviz also works
  // with the original WebKit shipped by supported macOS 11 releases.
  return new Worker(new URL('./graphviz-worker.ts', import.meta.url), { name: 'inkmark-graphviz' })
}

/** Sanitize engine-produced SVG and enforce dimensions/resource isolation. */
export function sanitizeExtendedDiagramSVG(markup: string): SVGSVGElement {
  if (!markup || markup.length > maximumRenderedSVGCharacters) fail('Rendered SVG exceeds the size limit')
  const sanitized = DOMPurify.sanitize(markup, {
    USE_PROFILES: { svg: true, svgFilters: true },
    FORBID_TAGS: ['script', 'style', 'foreignObject', 'image', 'use', 'a', 'iframe', 'object', 'embed'],
    FORBID_ATTR: ['href', 'xlink:href', 'src', 'onload', 'onclick', 'onerror'],
  })
  const documentNode = new DOMParser().parseFromString(sanitized, 'image/svg+xml')
  if (documentNode.querySelector('parsererror')) fail('Renderer returned invalid SVG')
  const svg = documentNode.documentElement
  if (svg.namespaceURI !== 'http://www.w3.org/2000/svg' || svg.localName !== 'svg') fail('Renderer did not return SVG')
  for (const element of Array.from(svg.querySelectorAll('*'))) {
    for (const attribute of Array.from(element.attributes)) {
      const name = attribute.name.toLowerCase()
      const value = attribute.value.trim()
      if (name.startsWith('on') || ['href', 'xlink:href', 'src'].includes(name)) element.removeAttribute(attribute.name)
      if (name === 'style' && /(?:@import|expression\s*\(|javascript:|url\s*\()/i.test(value)) element.removeAttribute(attribute.name)
      if (!['xmlns', 'xmlns:xlink'].includes(name) && unsafeSchemePattern.test(value)) element.removeAttribute(attribute.name)
      if (/url\s*\(/i.test(value) && !/^url\(#[A-Za-z_][\w:.-]*\)$/i.test(value)) element.removeAttribute(attribute.name)
    }
  }
  const viewBox = svg.getAttribute('viewBox')?.trim().split(/[\s,]+/).map(Number)
  if (viewBox?.length === 4) {
    const width = Math.abs(viewBox[2])
    const height = Math.abs(viewBox[3])
    if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0 || width > maximumSVGDimension || height > maximumSVGDimension || width * height > maximumSVGArea) {
      fail('Rendered SVG dimensions exceed the limit')
    }
  }
  svg.setAttribute('role', 'img')
  svg.removeAttribute('xmlns:xlink')
  return svg as unknown as SVGSVGElement
}

function renderFailure(node: HTMLElement, kind: string, error: unknown): void {
  const message = error instanceof Error && error.name !== 'AbortError' ? error.message : 'rendering failed'
  node.replaceChildren()
  node.classList.add('extended-diagram-error')
  node.dataset.extendedDiagramState = 'error'
  node.setAttribute('role', 'img')
  node.setAttribute('aria-label', `${kind} diagram could not be rendered`)
  node.textContent = `[${kind} diagram unavailable: ${message.slice(0, 180)}]`
}

async function renderECharts(node: HTMLElement, source: string, options: ExtendedDiagramRenderOptions, disposers: Array<() => void>): Promise<void> {
  const option = parseSafeEChartsOption(source)
  const engineOption = option as EChartsOption
  ensureActive(options)
  const echarts = await import('echarts')
  ensureActive(options)
  const width = boundedDimension(options.chartWidth || node.clientWidth, 800, minimumChartWidth, maximumChartWidth)
  const height = boundedDimension(options.chartHeight, 420, minimumChartHeight, maximumChartHeight)
  node.replaceChildren()
  if (options.echartsRenderer === 'canvas') {
    const chart = echarts.init(node, undefined, { renderer: 'canvas', width, height })
    try {
      chart.setOption(engineOption, { notMerge: true, lazyUpdate: false })
      ensureActive(options)
      disposers.push(() => chart.dispose())
    } catch (error) {
      chart.dispose()
      throw error
    }
  } else {
    const chart = echarts.init(null, undefined, { renderer: 'svg', ssr: true, width, height })
    try {
      chart.setOption(engineOption, { notMerge: true, lazyUpdate: false })
      const svg = sanitizeExtendedDiagramSVG(chart.renderToSVGString())
      ensureActive(options)
      node.append(document.importNode(svg, true))
    } finally {
      chart.dispose()
    }
  }
}

async function renderABC(node: HTMLElement, source: string, options: ExtendedDiagramRenderOptions): Promise<void> {
  const safeSource = validateABCSource(source)
  ensureActive(options)
  const { default: abcjs } = await import('abcjs')
  ensureActive(options)
  const scratch = document.createElement('div')
  const tunes = abcjs.renderAbc(scratch, safeSource, {
    add_classes: false,
    oneSvgPerLine: false,
    responsive: 'resize',
    selectionColor: 'currentColor',
  })
  if (tunes.length !== 1) fail('ABC renderer did not produce exactly one tune')
  const renderedSVG = scratch.querySelector('svg')
  if (!renderedSVG) fail('ABC renderer did not return SVG')
  if (!renderedSVG.hasAttribute('xmlns')) renderedSVG.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
  const svgMarkup = renderedSVG.outerHTML
  const svg = sanitizeExtendedDiagramSVG(svgMarkup)
  ensureActive(options)
  node.replaceChildren(document.importNode(svg, true))
}

async function renderGraphviz(
  node: HTMLElement,
  source: string,
  options: ExtendedDiagramRenderOptions,
  generationDeadline: number,
): Promise<void> {
  const safeSource = validateGraphvizSource(source)
  ensureActive(options)
  const worker = createGraphvizWorker()
  const requestID = Date.now() + Math.floor(Math.random() * 1_000_000)
  const markup = await new Promise<string>((resolve, reject) => {
    let settled = false
    const finish = (callback: () => void) => {
      if (settled) return
      settled = true
      window.clearTimeout(timeoutID)
      window.clearInterval(generationID)
      options.signal?.removeEventListener('abort', onAbort)
      worker.terminate()
      callback()
    }
    const onAbort = () => finish(() => reject(new DOMException('The operation was aborted', 'AbortError')))
    const timeoutID = window.setTimeout(
      () => finish(() => reject(new Error('Graphviz rendering timed out'))),
      Math.max(1, Math.min(maximumGraphvizRenderMilliseconds, generationDeadline - Date.now())),
    )
    const generationID = window.setInterval(() => {
      if (options.isCurrent?.() === false) onAbort()
    }, 50)
    worker.addEventListener('message', (event: MessageEvent<{ id?: number; svg?: string; error?: string }>) => {
      if (event.data?.id !== requestID) return
      if (typeof event.data.svg === 'string') finish(() => resolve(event.data.svg!))
      else finish(() => reject(new Error(event.data.error || 'Graphviz rendering failed')))
    })
    worker.addEventListener('error', () => finish(() => reject(new Error('Graphviz worker failed'))))
    options.signal?.addEventListener('abort', onAbort, { once: true })
    if (options.signal?.aborted || options.isCurrent?.() === false) onAbort()
    else worker.postMessage({ id: requestID, source: safeSource })
  })
  const svg = sanitizeExtendedDiagramSVG(markup)
  ensureActive(options)
  node.replaceChildren(document.importNode(svg, true))
}

/** Render inert fence placeholders inside a detached preview staging root. */
export async function renderExtendedDiagrams(root: HTMLElement, options: ExtendedDiagramRenderOptions = {}): Promise<ExtendedDiagramRenderResult> {
  const nodes = Array.from(root.querySelectorAll<HTMLElement>('[data-extended-diagram]'))
  const disposers: Array<() => void> = []
  const generationDeadline = Date.now() + maximumExtendedDiagramGenerationMilliseconds
  let graphvizDiagrams = 0
  let rendered = 0
  let failed = 0
  nodes.slice(maximumExtendedDiagrams).forEach((node) => {
    renderFailure(node, node.dataset.extendedDiagram || 'diagram', new Error('diagram count exceeds the limit'))
    failed += 1
  })
  for (const node of nodes.slice(0, maximumExtendedDiagrams)) {
    ensureActive(options)
    const kind = node.dataset.extendedDiagram as ExtendedDiagramKind | undefined
    const source = node.textContent || ''
    try {
      if (Date.now() >= generationDeadline) fail('extended diagram generation timed out')
      if (kind === 'echarts') await renderECharts(node, source, options, disposers)
      else if (kind === 'abc') await renderABC(node, source, options)
      else if (kind === 'graphviz') {
        graphvizDiagrams += 1
        if (graphvizDiagrams > maximumGraphvizDiagrams) fail('Graphviz diagram count exceeds the limit')
        await renderGraphviz(node, source, options, generationDeadline)
      }
      else fail('Unsupported extended diagram kind')
      ensureActive(options)
      node.classList.add('extended-diagram-rendered', `${kind}-rendered`)
      node.dataset.extendedDiagramState = 'ready'
      rendered += 1
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        disposers.splice(0).forEach((dispose) => dispose())
        throw error
      }
      renderFailure(node, kind || 'diagram', error)
      failed += 1
    }
  }
  return {
    rendered,
    failed,
    dispose: () => disposers.splice(0).forEach((dispose) => dispose()),
  }
}
