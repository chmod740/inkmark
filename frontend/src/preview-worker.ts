/// <reference lib="webworker" />
import MarkdownIt from 'markdown-it'
import taskLists from 'markdown-it-task-lists'
import { katex as markdownKatex } from '@mdit/plugin-katex'
import {
  classifyExtendedFence,
  convertBulletMindmap,
  installMarkdownExtensions,
} from './markdown-extensions.ts'
import {
  derivePreviewSource,
  splitMarkdownIntoSafeChunks,
  type PreviewWorkerRequest,
  type PreviewWorkerResponse,
} from './preview-worker-protocol.ts'

function lineOffset(environment: unknown) {
  const value = Number((environment as { inkmarkLineOffset?: unknown } | undefined)?.inkmarkLineOffset)
  return Number.isSafeInteger(value) && value >= 0 ? value : 0
}

function createMarkdownRenderer() {
  const markdown = new MarkdownIt({ html: true, breaks: true, linkify: true, typographer: true })
  installMarkdownExtensions(markdown)
  markdown.use(taskLists, { enabled: false, label: true, labelAfter: true })
  markdown.use(markdownKatex, { delimiters: 'all', throwOnError: false })
  markdown.core.ruler.push('inkmark-source-lines', (state) => {
    const offset = lineOffset(state.env)
    state.tokens.forEach((token) => {
      if (!token.map) return
      if (token.nesting === 1 || ['fence', 'code_block', 'hr', 'html_block', 'math_block'].includes(token.type)) {
        token.attrSet('data-source-line', String(token.map[0] + offset))
        const trustedMarker = String(state.env?.inkmarkTrustedMarker || '')
        if (/^[A-Za-z0-9_-]{16,160}$/.test(trustedMarker)) token.attrSet('data-inkmark-trusted', trustedMarker)
      }
    })
  })
  markdown.renderer.rules.image = (tokens, index, _options, env) => {
    const token = tokens[index]
    const trustedMarker = markdown.utils.escapeHtml(String(env?.inkmarkTrustedMarker || ''))
    const trustedAttribute = trustedMarker ? ` data-inkmark-trusted="${trustedMarker}"` : ''
    const source = markdown.utils.escapeHtml(String(token.attrGet('src') || ''))
    const alt = markdown.utils.escapeHtml(token.content || '')
    const title = markdown.utils.escapeHtml(String(token.attrGet('title') || ''))
    const titleAttribute = title ? ` data-inkmark-image-title="${title}"` : ''
    return `<span class="inkmark-image-placeholder" data-inkmark-image-source="${source}" data-inkmark-image-alt="${alt}"${titleAttribute}${trustedAttribute}></span>`
  }
  markdown.renderer.rules.math_inline = (tokens, index, _options, env) => {
    const marker = markdown.utils.escapeHtml(String(env?.inkmarkTrustedMarker || ''))
    const trustedAttribute = marker ? ` data-inkmark-trusted="${marker}"` : ''
    return `<span class="math-source" data-display-mode="inline"${trustedAttribute}>${markdown.utils.escapeHtml(tokens[index].content)}</span>`
  }
  markdown.renderer.rules.math_block = (tokens, index, _options, env) => {
    const sourceLine = tokens[index].map?.[0]
    const sourceAttribute = sourceLine === undefined ? '' : ` data-source-line="${sourceLine + lineOffset(env)}"`
    const marker = markdown.utils.escapeHtml(String(env?.inkmarkTrustedMarker || ''))
    const trustedAttribute = marker ? ` data-inkmark-trusted="${marker}"` : ''
    return `<div class="math-source" data-display-mode="block"${sourceAttribute}${trustedAttribute}>${markdown.utils.escapeHtml(tokens[index].content.trim())}</div>`
  }
  const defaultFence = markdown.renderer.rules.fence || ((tokens, index, options, _env, self) => self.renderToken(tokens, index, options))
  markdown.renderer.rules.fence = (tokens, index, options, env, self) => {
    const token = tokens[index]
    const trustedMarker = markdown.utils.escapeHtml(String(env?.inkmarkTrustedMarker || ''))
    const trustedAttribute = trustedMarker ? ` data-inkmark-trusted="${trustedMarker}"` : ''
    const classification = classifyExtendedFence(token.info, token.content)
    if (classification.status !== 'supported') return defaultFence(tokens, index, options, env, self)
    const sourceLine = token.map?.[0]
    const sourceAttribute = sourceLine === undefined ? '' : ` data-source-line="${sourceLine + lineOffset(env)}"`
    if (classification.kind === 'mermaid') return `<pre class="mermaid" data-kind="mermaid"${sourceAttribute}${trustedAttribute}>${markdown.utils.escapeHtml(classification.source)}</pre>`
    if (classification.kind === 'mindmap') {
      const definition = convertBulletMindmap(classification.source)
      return definition ? `<pre class="mermaid" data-kind="mindmap"${sourceAttribute}${trustedAttribute}>${markdown.utils.escapeHtml(definition)}</pre>` : defaultFence(tokens, index, options, env, self)
    }
    if (classification.kind === 'echarts' || classification.kind === 'abc' || classification.kind === 'graphviz') {
      return `<pre class="extended-diagram" data-kind="${classification.kind}" data-extended-diagram="${classification.kind}"${sourceAttribute}${trustedAttribute}>${markdown.utils.escapeHtml(classification.source)}</pre>`
    }
    return defaultFence(tokens, index, options, env, self)
  }
  return markdown
}

const markdown = createMarkdownRenderer()

self.addEventListener('message', async (event: MessageEvent<PreviewWorkerRequest>) => {
  const request = event.data
  if (!request || !Number.isSafeInteger(request.id) || typeof request.source !== 'string') return
  try {
    const chunks = splitMarkdownIntoSafeChunks(request.source)
    if (chunks) {
      for (let index = 0; index < chunks.length; index += 1) {
        const chunk = chunks[index]
        const response: PreviewWorkerResponse = {
          type: 'chunk', id: request.id, index,
          html: markdown.render(chunk.source, { inkmarkTrustedMarker: request.trustedMarker, inkmarkLineOffset: chunk.lineOffset }),
        }
        self.postMessage(response)
        // Give the UI thread a chance to commit the first readable section
        // before processing the next bounded unit of a multi-megabyte file.
        await new Promise<void>((resolve) => setTimeout(resolve, 0))
      }
    }
    const response: PreviewWorkerResponse = {
      type: 'complete',
      id: request.id,
      html: chunks ? '' : markdown.render(request.source, { inkmarkTrustedMarker: request.trustedMarker }),
      chunked: Boolean(chunks),
      derived: derivePreviewSource(request.source, request.sourceRevision),
    }
    self.postMessage(response)
  } catch (error) {
    const response: PreviewWorkerResponse = {
      type: 'error', id: request.id,
      message: error instanceof Error ? error.message.slice(0, 300) : 'Markdown worker failed',
    }
    self.postMessage(response)
  }
})
