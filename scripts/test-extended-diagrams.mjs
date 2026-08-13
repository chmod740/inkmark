import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import {
  maximumABCCharacters,
  maximumEChartsCharacters,
  maximumEChartsPointsPerSeries,
  maximumGraphvizCharacters,
  maximumGraphvizEdges,
  maximumGraphvizIdentifiers,
  maximumGraphvizTokens,
  maximumGraphvizRenderMilliseconds,
  parseSafeEChartsOption,
  validateABCSource,
  validateGraphvizSource,
} from '../frontend/src/extended-diagrams.ts'

const dates = Array.from({ length: 31 }, (_, index) => `2026-07-${String(index + 1).padStart(2, '0')}`)
const chartSource = JSON.stringify({
  title: { text: '最近30天' },
  tooltip: { trigger: 'axis', axisPointer: { type: 'line', lineStyle: { width: 1 } } },
  legend: { data: ['访问量', '下载量', '上传量'] },
  xAxis: [{
    type: 'category',
    boundaryGap: false,
    data: dates,
    axisTick: { show: false },
    axisLine: { show: false },
  }],
  yAxis: [{
    type: 'value',
    axisTick: { show: false },
    axisLine: { show: false },
    splitLine: { show: true, lineStyle: { color: 'rgba(127,127,127,.38)', type: 'dashed' } },
  }],
  series: ['访问量', '下载量', '上传量'].map((name, seriesIndex) => ({
    name,
    type: 'line',
    smooth: true,
    itemStyle: { color: ['#5470c6', '#91cc75', '#fac858'][seriesIndex] },
    areaStyle: { normal: {} },
    z: seriesIndex + 1,
    data: dates.map((_, index) => String(index * (seriesIndex + 1))),
  })),
})

const abcSource = [
  'X:24',
  'T:Clouds Thicken',
  'C:Paul Rosen',
  'S:Copyright 2005',
  'M:6/8',
  'L:1/8',
  'Q:3/8=116',
  'R:Creepy Jig',
  'K:Em',
  '|:"Em"EEE E2G|"D"FDD D2F|',
  '|"Em"GFE B2A|GEE E3|',
  '|:B2B Bcd|e2e efg|',
  '|fed edc|BAG F3:|',
].join('\n')

const dotSource = `digraph finite_state_machine {
  rankdir=LR;
  size="8,5";
  node [shape=doublecircle]; S;
  node [shape=point]; qi;
  node [shape=circle];
  qi -> S;
  S -> q1 [label="a"];
  S -> S [label="b"];
  q1 -> S [label="a"];
  q1 -> q2 [label="b"];
  q2 -> q1 [label="a,b"];
}`

test('未命名.md ECharts fence is rebuilt as a bounded line-only option', () => {
  const option = parseSafeEChartsOption(chartSource)
  assert.equal(option.animation, false)
  assert.equal(option.title?.text, '最近30天')
  assert.deepEqual(option.legend?.data, ['访问量', '下载量', '上传量'])
  assert.equal(option.xAxis.data.length, 31)
  assert.equal(option.tooltip?.axisPointer?.lineStyle?.width, 1)
  assert.deepEqual(option.xAxis.axisTick, { show: false })
  assert.equal(option.yAxis.splitLine?.lineStyle?.color, 'rgba(127,127,127,.38)')
  assert.equal(option.series.length, 3)
  assert.ok(option.series.every((series) => series.type === 'line' && series.data.length === 31))
  assert.ok(option.series.every((series) => series.data.every(Number.isFinite)))
})

test('未命名.md ABC fence is accepted for static visual notation', () => {
  assert.equal(validateABCSource(abcSource), abcSource)
})

test('未命名.md Graphviz finite-state DOT fence is accepted', () => {
  assert.equal(validateGraphvizSource(dotSource), dotSource)
})

test('ECharts rejects executable notation, resource fields, invalid JSON and budgets', () => {
  assert.throws(() => parseSafeEChartsOption(`({ formatter: () => fetch('https://evil.test') })`), /valid JSON/)
  assert.throws(() => parseSafeEChartsOption(JSON.stringify({
    xAxis: { type: 'category', data: ['x'] },
    yAxis: { type: 'value' },
    series: [{ type: 'line', symbol: 'image://https://evil.test/a.svg', data: [1] }],
  })), /unsupported field "symbol"/)
  assert.throws(() => parseSafeEChartsOption(JSON.stringify({
    tooltip: { trigger: 'axis', formatter: 'javascript:alert(1)' },
    xAxis: { type: 'category', data: ['x'] },
    yAxis: { type: 'value' },
    series: [{ type: 'line', data: [1] }],
  })), /unsupported field "formatter"/)
  assert.throws(() => parseSafeEChartsOption('x'.repeat(maximumEChartsCharacters + 1)), /size limit/)
  const tooMany = Array.from({ length: maximumEChartsPointsPerSeries + 1 }, (_, index) => String(index))
  assert.throws(() => parseSafeEChartsOption(JSON.stringify({
    xAxis: { type: 'category', data: tooMany },
    yAxis: { type: 'value' },
    series: [{ type: 'line', data: tooMany }],
  })), /invalid item count|size limit/)
})

test('ABC rejects playback/resource directives, multiple tunes and budgets', () => {
  assert.throws(() => validateABCSource(`${abcSource}\n%%MIDI program 10`), /unsupported directive/)
  assert.throws(() => validateABCSource(`${abcSource}\nF:https://evil.test/tune.abc`), /unsupported directive/)
  assert.throws(() => validateABCSource(`${abcSource}\nX:25\nK:C\nCDEF`), /one ABC tune/)
  assert.throws(() => validateABCSource(`X:1\nK:C\n${'C'.repeat(maximumABCCharacters)}`), /size limit/)
  assert.throws(() => validateABCSource('T:Missing tune headers\nCDEF'), /requires X and K/)
})

test('Graphviz rejects external resources, preprocessors, invalid input and budgets', () => {
  for (const malicious of [
    'digraph { a [image="https://evil.test/a.png"] }',
    'digraph { a [href="javascript:alert(1)"] }',
    'digraph { a [label=<<IMG SRC="file:///etc/passwd"/>>] }',
    'digraph { graph [stylesheet="https://evil.test/a.css"] }',
  ]) assert.throws(() => validateGraphvizSource(malicious), /resource/)
  assert.throws(() => validateGraphvizSource('#include "/etc/passwd"\ndigraph { a }'), /DOT graph|preprocessor/)
  assert.throws(() => validateGraphvizSource('not a graph'), /DOT graph/)
  assert.throws(() => validateGraphvizSource('x'.repeat(maximumGraphvizCharacters + 1)), /size limit/)
  assert.throws(() => validateGraphvizSource(`digraph { ${'a -> b;'.repeat(maximumGraphvizEdges + 1)} }`), /edge count/)
  assert.throws(() => validateGraphvizSource(`digraph { ${Array.from({ length: maximumGraphvizIdentifiers + 1 }, (_, index) => `n${index}`).join(' ')} }`), /identifier count|token count/)
  assert.throws(() => validateGraphvizSource(`digraph { ${'node '.repeat(maximumGraphvizTokens + 1)} }`), /token count|size limit/)
  assert.throws(() => validateGraphvizSource(`digraph { ${'😀 '.repeat(2_000)} }`), /unsupported tokens/)
  assert.throws(() => validateGraphvizSource('digraph { graph [size="100,100"] }'), /requested size/)
})

test('Graphviz rendering is isolated in a terminating worker with a hard timeout', async () => {
  const renderer = await readFile(new URL('../frontend/src/extended-diagrams.ts', import.meta.url), 'utf8')
  const worker = await readFile(new URL('../frontend/src/graphviz-worker.ts', import.meta.url), 'utf8')
  assert.equal(maximumGraphvizRenderMilliseconds, 5_000)
  assert.match(renderer, /import\.meta\.env\.DEV[\s\S]*type: 'module'/)
  assert.match(renderer, /return new Worker\(new URL\('\.\/graphviz-worker\.ts', import\.meta\.url\), \{ name: 'inkmark-graphviz' \}\)/)
  assert.match(renderer, /const worker = createGraphvizWorker\(\)/)
  assert.match(renderer, /Graphviz rendering timed out/)
  assert.match(renderer, /worker\.terminate\(\)/)
  assert.match(renderer, /signal\?\.addEventListener\('abort'/)
  assert.match(renderer, /markup\.length > maximumRenderedSVGCharacters/)
  assert.match(renderer, /!renderedSVG\.hasAttribute\('xmlns'\)[\s\S]*http:\/\/www\.w3\.org\/2000\/svg/)
  assert.match(renderer, /graphvizDiagrams > maximumGraphvizDiagrams/)
  assert.match(renderer, /generationDeadline - Date\.now\(\)/)
  assert.match(worker, /svg\.length > maximumRenderedSVGCharacters/)
  assert.match(worker, /viz\.renderString\(source, \{ format: 'svg', engine: 'dot' \}\)/)
})
