import { instance } from '@viz-js/viz'

interface GraphvizWorkerRequest {
  id: number
  source: string
}

interface GraphvizWorkerResponse {
  id: number
  svg?: string
  error?: string
}

const vizRuntime = instance()
const maximumRenderedSVGCharacters = 2 * 1024 * 1024

self.addEventListener('message', async (event: MessageEvent<GraphvizWorkerRequest>) => {
  const { id, source } = event.data || {}
  if (!Number.isSafeInteger(id) || typeof source !== 'string') return
  let response: GraphvizWorkerResponse
  try {
    const viz = await vizRuntime
    const svg = viz.renderString(source, { format: 'svg', engine: 'dot' })
    if (svg.length > maximumRenderedSVGCharacters) throw new Error('Rendered SVG exceeds the size limit')
    response = { id, svg }
  } catch (error) {
    response = { id, error: error instanceof Error ? error.message.slice(0, 300) : 'Graphviz rendering failed' }
  }
  self.postMessage(response)
})
