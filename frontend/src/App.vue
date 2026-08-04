<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'
import katex from 'katex'
import MarkdownIt from 'markdown-it'
import taskLists from 'markdown-it-task-lists'
import { katex as markdownKatex } from '@mdit/plugin-katex'
import mermaid from 'mermaid'
import inkmarkIcon from './assets/inkmark-icon.svg'
import {
  buildStandaloneHTML,
  bytesToBase64,
  collectEmbeddedStyles,
  type ExportFormat,
  utf8ToBase64,
} from './export-document'
import { ScrollSyncController, type ScrollPane } from './scroll-sync'
import {
  getSystemLanguages,
  languageModeStorageKey,
  normalizeLanguageMode,
  resolveLocale,
  translate,
  type LanguageMode,
  type Locale,
  type TranslationKey,
} from './i18n'
import {
  CheckForUpdates,
  GetAppInfo,
  GetLanguageSettings,
  LoadInitialDocument,
  OpenExternal,
  OpenFile,
  OpenSourceRepository,
  OpenUpdatePage,
  SaveExportFile,
  SaveFile,
  SaveFileAs,
  SetLanguage,
  SetWindowTitle,
  UpdateMenuState,
  WelcomeDocument as LoadWelcomeDocument,
} from '../wailsjs/go/main/App'
import {
  EventsOn,
	  Hide,
  Quit,
  WindowFullscreen,
  WindowIsFullscreen,
  WindowMinimise,
  WindowToggleMaximise,
  WindowUnfullscreen,
} from '../wailsjs/runtime/runtime'

type Theme = 'github' | 'clean' | 'wechat' | 'dark'
type ViewMode = 'edit' | 'split' | 'preview'

interface DocumentData {
  path: string
  name: string
  content: string
  welcome?: boolean
}

interface ApplicationInfoData {
  version: string
  author: string
  repositoryURL: string
}

interface UpdateInfoData {
  currentVersion: string
  latestVersion: string
  updateAvailable: boolean
  releaseURL: string
  downloadURL: string
  publishedAt: string
}

type UpdateState = 'idle' | 'checking' | 'current' | 'available' | 'unavailable' | 'error'

interface PreviewCapture {
  canvas: HTMLCanvasElement
  breakpoints: number[]
}

interface ExportSnapshot {
  source: string
  theme: Theme
  path: string
  name: string
  title: string
}

const source = ref('')
const savedSource = ref('')
const currentPath = ref('')
const fileName = ref('README.md')
const theme = ref<Theme>(readPreference<Theme>('inkmark-theme', 'github'))
const viewMode = ref<ViewMode>(readPreference<ViewMode>('inkmark-view', 'split'))
const languageMode = ref<LanguageMode>('auto')
const locale = ref<Locale>('en')
const renderState = ref('')
const busy = ref(false)
const editor = ref<HTMLTextAreaElement | null>(null)
const preview = ref<HTMLElement | null>(null)
const previewPane = ref<HTMLElement | null>(null)
const syncScroll = ref(true)
const isWelcomeDocument = ref(false)
const activeDialog = ref<'settings' | 'shortcuts' | 'about' | null>(null)
const applicationInfo = ref<ApplicationInfoData>({ version: '', author: '', repositoryURL: '' })
const updateInfo = ref<UpdateInfoData | null>(null)
const updateState = ref<UpdateState>('idle')

let renderTimer: number | undefined
let renderSequence = 0
let quitting = false
const removeMenuListeners: Array<() => void> = []
const scrollSync = new ScrollSyncController()

const markdown = new MarkdownIt({
  html: true,
  breaks: true,
  linkify: true,
  typographer: true,
})
markdown.use(taskLists, { enabled: false, label: true, labelAfter: true })
markdown.use(markdownKatex, { delimiters: 'all', throwOnError: false })
markdown.renderer.rules.math_inline = (tokens, index) =>
  `<span class="math-source" data-display-mode="inline">${markdown.utils.escapeHtml(tokens[index].content)}</span>`
markdown.renderer.rules.math_block = (tokens, index) =>
  `<div class="math-source" data-display-mode="block">${markdown.utils.escapeHtml(tokens[index].content.trim())}</div>`
const defaultFence = markdown.renderer.rules.fence || ((tokens, index, options, _env, self) => self.renderToken(tokens, index, options))
markdown.renderer.rules.fence = (tokens, index, options, env, self) => {
  const language = tokens[index].info.trim().split(/\s+/)[0].toLowerCase()
  if (language === 'mermaid' || language === 'mmd') {
    return `<pre class="mermaid">${markdown.utils.escapeHtml(tokens[index].content)}</pre>`
  }
  return defaultFence(tokens, index, options, env, self)
}

const dirty = computed(() => source.value !== savedSource.value)
const lineCount = computed(() => source.value ? source.value.split(/\r?\n/).length : 1)
const characterCount = computed(() => Array.from(source.value).length)
const locationLabel = computed(() => currentPath.value || t('document.unsavedLocation'))
const aboutVersion = computed(() => applicationInfo.value.version || '—')
const aboutAuthor = computed(() => applicationInfo.value.author || '—')
const updateStatusText = computed(() => {
  const version = updateInfo.value?.latestVersion || updateInfo.value?.currentVersion || aboutVersion.value
  if (updateState.value === 'checking') return t('help.checkingUpdate')
  if (updateState.value === 'current') return t('help.updateCurrent', { version })
  if (updateState.value === 'available') return t('help.updateAvailable', { version })
  if (updateState.value === 'unavailable') return t('help.updateUnavailable')
  if (updateState.value === 'error') return t('help.updateFailed')
  return t('help.updateNotChecked')
})
const updatePublishedAt = computed(() => {
  const value = updateInfo.value?.publishedAt?.trim()
  if (!value) return ''
  const timestamp = Date.parse(value)
  if (!Number.isFinite(timestamp)) return value
  return new Intl.DateTimeFormat(locale.value, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(timestamp)
})

const themes = computed<Array<{ value: Theme; label: string }>>(() => [
  { value: 'github', label: t('theme.github') },
  { value: 'clean', label: t('theme.clean') },
  { value: 'wechat', label: t('theme.wechat') },
  { value: 'dark', label: t('theme.dark') },
])

const formatActions = computed(() => [
  { action: 'bold', label: 'B', title: `${t('format.bold')} (⌘/Ctrl+B)` },
  { action: 'italic', label: 'I', title: `${t('format.italic')} (⌘/Ctrl+I)` },
  { action: 'strike', label: 'S', title: t('format.strikethrough') },
  { action: 'heading', label: 'H₂', title: t('format.heading2') },
  { action: 'quote', label: '❞', title: t('format.quote') },
  { action: 'ul', label: '•', title: t('format.unorderedList') },
  { action: 'ol', label: '1.', title: t('format.orderedList') },
  { action: 'task', label: '☑', title: t('format.taskList') },
  { action: 'link', label: '↗', title: `${t('format.link')} (⌘/Ctrl+K)` },
  { action: 'code', label: '</>', title: t('format.inlineCode') },
  { action: 'codeblock', label: '```', title: t('format.codeBlock') },
  { action: 'table', label: '▦', title: t('format.table') },
])

const exportLabels = computed<Record<ExportFormat, string>>(() => ({
  pdf: t('export.pdf'),
  html: t('export.html'),
  png: t('export.png'),
  txt: t('export.txt'),
  doc: t('export.doc'),
}))

const shortcutRows = computed(() => [
  ['⌘/Ctrl+N', t('help.shortcut.new')],
  ['⌘/Ctrl+O', t('help.shortcut.open')],
  ['⌘/Ctrl+S', t('help.shortcut.save')],
  ['⌘/Ctrl+Shift+S', t('help.shortcut.saveAs')],
  ['⌘/Ctrl+B', t('help.shortcut.bold')],
  ['⌘/Ctrl+I', t('help.shortcut.italic')],
  ['⌘/Ctrl+K', t('help.shortcut.link')],
  ['⌘/Ctrl+1', t('help.shortcut.viewEdit')],
  ['⌘/Ctrl+2', t('help.shortcut.viewSplit')],
  ['⌘/Ctrl+3', t('help.shortcut.viewPreview')],
])

function t(key: TranslationKey, parameters?: Record<string, string | number>) {
  return translate(locale.value, key, parameters)
}

function readPreference<T extends string>(key: string, fallback: T): T {
  const value = localStorage.getItem(key)
  return (value || fallback) as T
}

function setDocument(document: DocumentData, status: TranslationKey = 'document.opened') {
  currentPath.value = document.path || ''
  fileName.value = document.name || t('document.untitledFilename')
  source.value = document.content || ''
  savedSource.value = source.value
  isWelcomeDocument.value = Boolean(document.welcome)
  renderState.value = t(status)
  void nextTick(renderNow)
}

function newDocument() {
  if (dirty.value && !window.confirm(t('confirm.newUnsaved'))) return
  setDocument({ path: '', name: t('document.untitledFilename'), content: '', welcome: false }, 'document.created')
  nextTick(() => editor.value?.focus())
}

async function openDocument() {
  if (dirty.value && !window.confirm(t('confirm.openUnsaved'))) return
  try {
    const document = await OpenFile() as DocumentData
    if (document?.name) setDocument(document)
  } catch (error) {
    showError(error)
  }
}

async function saveDocument() {
  try {
    busy.value = true
    const result = await SaveFile(currentPath.value, source.value)
    if (result?.path) {
      currentPath.value = result.path
      fileName.value = result.name
      savedSource.value = source.value
      isWelcomeDocument.value = false
      renderState.value = t('document.savedLocally')
    }
  } catch (error) {
    showError(error)
  } finally {
    busy.value = false
  }
}

async function saveDocumentAs() {
  try {
    busy.value = true
    const result = await SaveFileAs(currentPath.value, source.value)
    if (result?.path) {
      currentPath.value = result.path
      fileName.value = result.name
      savedSource.value = source.value
      isWelcomeDocument.value = false
      renderState.value = t('document.savedAsLocally')
    }
  } catch (error) {
    showError(error)
  } finally {
    busy.value = false
  }
}

async function exportDocument(format: ExportFormat) {
  if (busy.value) return
  const snapshot: ExportSnapshot = {
    source: source.value,
    theme: theme.value,
    path: currentPath.value,
    name: fileName.value,
    title: exportDocumentTitle(fileName.value),
  }
  try {
    busy.value = true
    renderState.value = t('export.preparing', { format: exportLabels.value[format] })
    window.clearTimeout(renderTimer)
    const rendered = await renderNow(snapshot.source, snapshot.theme)
    if (!rendered) throw new Error(t('error.previewInterrupted'))
    await nextTick()
    assertExportSnapshot(snapshot)

    const target = preview.value
    if (!target) throw new Error(t('error.previewUnavailable'))

    let payloadBase64: string
    if (format === 'html' || format === 'doc') {
      const documentHTML = buildStandaloneHTML({
        title: snapshot.title,
        theme: snapshot.theme,
        articleHTML: target.innerHTML,
        embeddedStyles: collectEmbeddedStyles(),
        language: locale.value,
        wordCompatible: format === 'doc',
      })
      payloadBase64 = utf8ToBase64(documentHTML)
    } else if (format === 'txt') {
      // Plain text intentionally preserves the complete Markdown source,
      // including formulas and diagram definitions that visual textContent
      // would duplicate or omit.
      payloadBase64 = utf8ToBase64(snapshot.source)
    } else {
      const capture = await capturePreviewCanvas(target, snapshot.theme, format === 'pdf')
      payloadBase64 = format === 'png'
        ? capture.canvas.toDataURL('image/png').split(',', 2)[1]
        : await buildPDFBase64(capture, snapshot.theme)
    }

    assertExportSnapshot(snapshot)
    renderState.value = t('export.chooseLocation', { format: exportLabels.value[format] })
    const result = await SaveExportFile(format, snapshot.path, snapshot.name, payloadBase64)
    renderState.value = result?.path
      ? t('export.completed', { format: exportLabels.value[format], name: result.name })
      : t('export.cancelled')
  } catch (error) {
    showError(error)
  } finally {
    busy.value = false
  }
}

function exportDocumentTitle(name: string) {
  const title = name.replace(/\.[^.]+$/, '').trim()
  return title || t('document.untitled')
}

function assertExportSnapshot(snapshot: ExportSnapshot) {
  if (
    source.value !== snapshot.source
    || theme.value !== snapshot.theme
    || currentPath.value !== snapshot.path
    || fileName.value !== snapshot.name
  ) {
    throw new Error(t('error.documentChangedDuringExport'))
  }
}

async function capturePreviewCanvas(
  target: HTMLElement,
  exportTheme: Theme,
  fitPDFPages = false,
): Promise<PreviewCapture> {
  const captureWidth = 920
  const stage = document.createElement('div')
  stage.className = `export-capture theme-${exportTheme}`
  stage.dataset.colorScheme = exportTheme === 'dark' ? 'dark' : 'light'
  stage.style.cssText = [
    'position:fixed',
    'left:-12000px',
    'top:0',
    `width:${captureWidth}px`,
    'height:auto',
    'overflow:visible',
    'pointer-events:none',
    `background:${exportTheme === 'dark' ? '#0d1117' : '#ffffff'}`,
  ].join(';')

  const clone = target.cloneNode(true) as HTMLElement
  clone.removeAttribute('id')
  clone.style.width = '100%'
  clone.style.minHeight = '0'
  clone.style.margin = '0 auto'
  clone.style.boxShadow = 'none'
  clone.querySelectorAll<HTMLDetailsElement>('details').forEach((details) => {
    details.open = true
    const summary = details.querySelector<HTMLElement>('summary')
    if (summary) summary.style.display = 'block'
  })
  replaceRemoteImagesForOfflineCapture(clone)
  stage.append(clone)
  document.body.append(stage)

  try {
    await document.fonts?.ready
    await waitForLocalImages(clone)
    if (fitPDFPages) fitTallMermaidForPDF(clone, captureWidth)
    const width = Math.max(1, clone.scrollWidth)
    const height = Math.max(1, clone.scrollHeight)
    // Keep the raw RGBA canvas below roughly 72 MB. The Base64 bridge and PDF
    // page slices need additional memory, especially in WebView2 on Windows.
    const maxCanvasSide = 24_000
    const maxCanvasPixels = 18_000_000
    const scale = Math.min(
      2,
      maxCanvasSide / Math.max(width, height),
      Math.sqrt(maxCanvasPixels / (width * height)),
    )
    if (!Number.isFinite(scale) || scale < 0.5) {
      throw new Error(t('error.documentTooLong'))
    }

    const cloneTop = clone.getBoundingClientRect().top
    const cssBreakpoints = Array.from(clone.children)
      // Do not break immediately after a heading. Keeping the heading with its
      // following block avoids orphaned Mermaid/section titles at page bottoms.
      .filter((element) => !element.previousElementSibling?.matches('h1, h2, h3, h4, h5, h6'))
      .map((element) => element.getBoundingClientRect().top - cloneTop)
      .filter((point) => point > 0 && point < height)

    const { default: html2canvas } = await import('html2canvas')
    const canvas = await html2canvas(clone, {
      allowTaint: false,
      backgroundColor: exportTheme === 'dark' ? '#0d1117' : '#ffffff',
      height,
      imageTimeout: 0,
      logging: false,
      onclone: normalizeModernColorsForCanvas,
      scale,
      scrollX: 0,
      scrollY: 0,
      useCORS: false,
      width,
      windowHeight: height,
      windowWidth: captureWidth,
    })
    const pixelRatio = canvas.height / height
    return {
      canvas,
      breakpoints: cssBreakpoints.map((point) => Math.round(point * pixelRatio)),
    }
  } finally {
    stage.remove()
  }
}

function fitTallMermaidForPDF(root: HTMLElement, captureWidth: number) {
  // The A4 content box is 190 × 277 mm. Reserve space for the section
  // heading/margins, then scale only diagrams that cannot fit on one page.
  const maximumDiagramHeight = Math.floor(captureWidth * 277 / 190 - 150)
  root.querySelectorAll<SVGSVGElement>('.mermaid-rendered svg').forEach((svg) => {
    const bounds = svg.getBoundingClientRect()
    if (bounds.width <= 0 || bounds.height <= maximumDiagramHeight) return
    const scale = maximumDiagramHeight / bounds.height
    svg.style.width = `${Math.floor(bounds.width * scale)}px`
    svg.style.height = `${maximumDiagramHeight}px`
    svg.style.maxWidth = '100%'
    svg.style.display = 'block'
    svg.style.margin = '0 auto'
  })
}

function normalizeModernColorsForCanvas(clonedDocument: Document) {
  // WebKit can serialise SVG and system colours as color(srgb ...) or
  // color(display-p3 ...). html2canvas 1.4 does not parse CSS Color 4, so use
  // the browser's own canvas colour parser to convert those values to rgba().
  const colorCanvas = clonedDocument.createElement('canvas')
  colorCanvas.width = 1
  colorCanvas.height = 1
  const colorContext = colorCanvas.getContext('2d')
  const colorCache = new Map<string, string>()
  if (!colorContext) return

  const toRGBA = (colorFunction: string) => {
    const cached = colorCache.get(colorFunction)
    if (cached) return cached
    colorContext.clearRect(0, 0, 1, 1)
    colorContext.fillStyle = '#000000'
    colorContext.fillStyle = colorFunction
    colorContext.fillRect(0, 0, 1, 1)
    const [red, green, blue, alpha] = colorContext.getImageData(0, 0, 1, 1).data
    const converted = `rgba(${red}, ${green}, ${blue}, ${Math.round(alpha / 255 * 1000) / 1000})`
    colorCache.set(colorFunction, converted)
    return converted
  }
  const normalize = (value: string) => value.replace(/color\([^)]*\)/gi, toRGBA)
  const colorProperties = [
    'background-color',
    'background-image',
    'border-top-color',
    'border-right-color',
    'border-bottom-color',
    'border-left-color',
    'box-shadow',
    'color',
    'fill',
    'flood-color',
    'lighting-color',
    'outline-color',
    'stop-color',
    'stroke',
    'text-decoration-color',
    'text-shadow',
    '-webkit-text-stroke-color',
  ]
  const elements = [clonedDocument.documentElement, clonedDocument.body, ...clonedDocument.querySelectorAll('*')]
  const clonedWindow = clonedDocument.defaultView
  if (!clonedWindow) return

  elements.forEach((element) => {
    if (!element || !('style' in element)) return
    const styledElement = element as HTMLElement | SVGElement
    const computed = clonedWindow.getComputedStyle(element)
    colorProperties.forEach((property) => {
      const value = computed.getPropertyValue(property)
      if (/color\(/i.test(value)) styledElement.style.setProperty(property, normalize(value))
    })
    for (const attribute of ['fill', 'stroke', 'stop-color', 'flood-color', 'lighting-color']) {
      const value = element.getAttribute(attribute)
      if (value && /color\(/i.test(value)) element.setAttribute(attribute, normalize(value))
    }
  })
}

function replaceRemoteImagesForOfflineCapture(root: HTMLElement) {
  root.querySelectorAll<HTMLImageElement>('img[src]').forEach((image) => {
    const src = image.getAttribute('src') || ''
    if (!/^(?:https?:)?\/\//i.test(src)) return
    const placeholder = document.createElement('div')
    placeholder.className = 'external-image-placeholder'
    placeholder.textContent = t('export.externalImagePlaceholder', { source: image.alt || src })
    placeholder.style.cssText = 'margin:16px 0;padding:18px;border:1px dashed #9aa4a0;border-radius:6px;color:#68716d;background:#f5f7f5;text-align:center'
    image.replaceWith(placeholder)
  })
}

async function waitForLocalImages(root: HTMLElement) {
  const pending = Array.from(root.querySelectorAll<HTMLImageElement>('img')).filter((image) => !image.complete)
  await Promise.all(pending.map((image) => new Promise<void>((resolve) => {
    const finish = () => resolve()
    image.addEventListener('load', finish, { once: true })
    image.addEventListener('error', finish, { once: true })
    window.setTimeout(finish, 3_000)
  })))
}

async function buildPDFBase64(capture: PreviewCapture, exportTheme: Theme): Promise<string> {
  const { jsPDF } = await import('jspdf')
  const { canvas, breakpoints } = capture
  const pageWidth = 210
  const pageHeight = 297
  const margin = 10
  const contentWidth = pageWidth - margin * 2
  const contentHeight = pageHeight - margin * 2
  const slicePixelHeight = Math.max(1, Math.floor(canvas.width * contentHeight / contentWidth))
  const slices = planPDFSlices(canvas.height, slicePixelHeight, breakpoints)
  const pageCount = slices.length
  const pdf = new jsPDF({ unit: 'mm', format: 'a4', orientation: 'portrait', compress: true })

  for (let pageIndex = 0; pageIndex < slices.length; pageIndex += 1) {
    if (pageIndex > 0) pdf.addPage('a4', 'portrait')
    if (exportTheme === 'dark') {
      pdf.setFillColor(13, 17, 23)
      pdf.rect(0, 0, pageWidth, pageHeight, 'F')
    }

    const { start: sourceY, height: sourceHeight } = slices[pageIndex]
    const pageCanvas = document.createElement('canvas')
    pageCanvas.width = canvas.width
    pageCanvas.height = sourceHeight
    const context = pageCanvas.getContext('2d')
    if (!context) throw new Error(t('error.pdfCanvasUnavailable'))
    context.fillStyle = exportTheme === 'dark' ? '#0d1117' : '#ffffff'
    context.fillRect(0, 0, pageCanvas.width, pageCanvas.height)
    context.drawImage(
      canvas,
      0, sourceY, canvas.width, sourceHeight,
      0, 0, pageCanvas.width, pageCanvas.height,
    )

    const renderedHeight = sourceHeight * contentWidth / canvas.width
    pdf.addImage(pageCanvas, 'PNG', margin, margin, contentWidth, renderedHeight, undefined, 'FAST')
    pdf.setFontSize(8)
    pdf.setTextColor(exportTheme === 'dark' ? 150 : 120)
    pdf.text(`${pageIndex + 1} / ${pageCount}`, pageWidth / 2, pageHeight - 4, { align: 'center' })
  }

  return bytesToBase64(new Uint8Array(pdf.output('arraybuffer')))
}

function planPDFSlices(canvasHeight: number, maximumHeight: number, breakpoints: number[]) {
  const slices: Array<{ start: number; height: number }> = []
  const sortedBreakpoints = [...new Set(breakpoints)].sort((left, right) => left - right)
  let start = 0
  while (start < canvasHeight) {
    const naturalEnd = Math.min(canvasHeight, start + maximumHeight)
    let end = naturalEnd
    if (naturalEnd < canvasHeight) {
      // A moderately short page is preferable to splitting a large diagram
      // that would fit intact on the following page.
      const minimumUsefulEnd = start + maximumHeight * 0.35
      const candidates = sortedBreakpoints.filter((point) => point >= minimumUsefulEnd && point <= naturalEnd)
      if (candidates.length) end = candidates[candidates.length - 1]
    }
    if (end <= start) end = naturalEnd
    slices.push({ start, height: end - start })
    start = end
  }
  return slices
}

function quitApplication() {
  if (dirty.value && !window.confirm(t('confirm.quitUnsaved'))) return
  quitting = true
  Quit()
}

async function showWelcome() {
  if (dirty.value && !window.confirm(t('confirm.openUnsaved'))) return
  try {
    const document = await LoadWelcomeDocument(locale.value) as DocumentData
    setDocument(document, 'status.ready')
  } catch (error) {
    showError(error)
  }
}

async function loadApplicationInfo() {
  try {
    const info = await GetAppInfo() as ApplicationInfoData
    applicationInfo.value = {
      version: info?.version?.trim() || '',
      author: info?.author?.trim() || '',
      repositoryURL: info?.repositoryURL?.trim() || '',
    }
  } catch (error) {
    console.error('Unable to load application information', error)
  }
}

function showAboutDialog() {
  activeDialog.value = 'about'
  if (!applicationInfo.value.version) void loadApplicationInfo()
  if (updateState.value === 'idle') void checkForUpdates(false)
}

async function checkForUpdates(showDialog = true) {
  if (updateState.value === 'checking') return
  if (showDialog) {
    activeDialog.value = 'about'
    if (!applicationInfo.value.version) void loadApplicationInfo()
  }
  updateState.value = 'checking'
  try {
    const result = await CheckForUpdates() as UpdateInfoData
    updateInfo.value = {
      currentVersion: result?.currentVersion?.trim() || applicationInfo.value.version,
      latestVersion: result?.latestVersion?.trim() || '',
      updateAvailable: Boolean(result?.updateAvailable),
      releaseURL: result?.releaseURL?.trim() || '',
      downloadURL: result?.downloadURL?.trim() || '',
      publishedAt: result?.publishedAt?.trim() || '',
    }
    if (!applicationInfo.value.version && updateInfo.value.currentVersion) {
      applicationInfo.value = { ...applicationInfo.value, version: updateInfo.value.currentVersion }
    }
    updateState.value = updateInfo.value.updateAvailable
      ? 'available'
      : updateInfo.value.currentVersion || updateInfo.value.latestVersion
        ? 'current'
        : 'unavailable'
  } catch (error) {
    console.error('Unable to check for updates', error)
    updateInfo.value = null
    updateState.value = 'error'
  }
}

async function openSourceRepository() {
  if (!applicationInfo.value.repositoryURL) return
  try {
    await OpenSourceRepository()
  } catch (error) {
    showError(error)
  }
}

async function openUpdatePage() {
  try {
    await OpenUpdatePage()
  } catch (error) {
    showError(error)
  }
}

async function upgradeApplication() {
  if (updateState.value !== 'available') await checkForUpdates()
  if (updateState.value === 'available') await openUpdatePage()
}

async function toggleFullscreen() {
  if (await WindowIsFullscreen()) WindowUnfullscreen()
  else WindowFullscreen()
}

async function runEditAction(action: string) {
  const target = editor.value
  if (!target) return
  target.focus()
  try {
    if (action === 'select-all') {
      target.setSelectionRange(0, target.value.length)
    } else if (action === 'copy' || action === 'cut') {
      const start = target.selectionStart
      const end = target.selectionEnd
      await navigator.clipboard.writeText(target.value.slice(start, end))
      if (action === 'cut' && end > start) {
        target.setRangeText('', start, end, 'end')
        source.value = target.value
      }
    } else if (action === 'paste') {
      const text = await navigator.clipboard.readText()
      target.setRangeText(text, target.selectionStart, target.selectionEnd, 'end')
      source.value = target.value
    } else if (action === 'undo' || action === 'redo') {
      document.execCommand(action)
      source.value = target.value
    }
  } catch {
    renderState.value = t('edit.clipboardFailed')
  }
}

function updateNativeMenu() {
  void UpdateMenuState(locale.value, viewMode.value, theme.value, syncScroll.value)
}

async function applyLanguageMode(mode: LanguageMode, replaceWelcome = true) {
  const normalized = normalizeLanguageMode(mode)
  const nextLocale = resolveLocale(normalized, getSystemLanguages())
  languageMode.value = normalized
  locale.value = nextLocale
  localStorage.setItem(languageModeStorageKey, normalized)
  document.documentElement.lang = nextLocale
	  document.title = t('app.fullName')
  await SetLanguage(normalized, nextLocale)
  updateNativeMenu()
  if (replaceWelcome && isWelcomeDocument.value && !dirty.value) {
    const welcome = await LoadWelcomeDocument(nextLocale) as DocumentData
    setDocument(welcome, 'status.ready')
  } else {
    await renderNow()
  }
	  await SetWindowTitle(fileName.value, dirty.value)
}

function handleLanguageChange(event: Event) {
  const mode = (event.target as HTMLSelectElement).value as LanguageMode
  void applyLanguageMode(mode).catch(showError)
}

function handleSystemLanguageChange() {
  if (languageMode.value === 'auto') void applyLanguageMode('auto').catch(showError)
}

function handleSystemDocument(document: DocumentData) {
  if (!document?.name) return
  if (dirty.value && !window.confirm(t('confirm.openUnsaved'))) return
  setDocument(document)
}

function handleMenuAction(action: string) {
  if (busy.value) return
  if (action === 'new') newDocument()
  else if (action === 'open') void openDocument()
  else if (action === 'save') void saveDocument()
  else if (action === 'save-as') void saveDocumentAs()
  else if (action === 'export-pdf') void exportDocument('pdf')
  else if (action === 'export-html') void exportDocument('html')
  else if (action === 'export-png') void exportDocument('png')
  else if (action === 'export-txt') void exportDocument('txt')
  else if (action === 'export-doc') void exportDocument('doc')
  else if (action === 'settings') activeDialog.value = 'settings'
  else if (action === 'show-shortcuts') activeDialog.value = 'shortcuts'
  else if (action === 'about') showAboutDialog()
  else if (action === 'check-update') void checkForUpdates()
  else if (action === 'upgrade') void upgradeApplication()
  else if (action === 'source-code') void openSourceRepository()
  else if (action === 'show-welcome') void showWelcome()
  else if (action === 'hide') Hide()
  else if (action === 'window-minimise') WindowMinimise()
  else if (action === 'window-toggle-maximise') WindowToggleMaximise()
  else if (action === 'toggle-fullscreen') void toggleFullscreen()
  else if (action === 'toggle-sync-scroll') syncScroll.value = !syncScroll.value
  else if (action.startsWith('view-')) chooseView(action.slice(5) as ViewMode)
  else if (action.startsWith('theme-')) chooseTheme(action.slice(6) as Theme)
  else if (action.startsWith('format-')) runFormat(action.slice(7))
  else if (['undo', 'redo', 'cut', 'copy', 'paste', 'select-all'].includes(action)) void runEditAction(action)
  else if (action === 'copy-html') void copyHTML()
  else if (action === 'quit') quitApplication()
}

function scheduleRender() {
  window.clearTimeout(renderTimer)
  // Invalidate any Mermaid work still running for the previous document.
  // Otherwise an old asynchronous render may keep changing preview height.
  renderSequence += 1
  renderState.value = t('status.waiting')
  renderTimer = window.setTimeout(renderNow, 120)
}

async function renderNow(sourceText = source.value, renderTheme = theme.value): Promise<boolean> {
  const target = preview.value
  if (!target) return false
  const sequence = ++renderSequence
  renderState.value = t('status.rendering')
  try {
    const renderedHTML = markdown.render(sourceText)
    const cleanHTML = DOMPurify.sanitize(renderedHTML, {
      USE_PROFILES: { html: true },
      ADD_TAGS: ['details', 'summary', 'mark'],
      ADD_ATTR: ['target', 'rel', 'checked', 'disabled', 'data-alert', 'data-display-mode'],
      FORBID_TAGS: ['script', 'style', 'iframe', 'object', 'embed', 'svg'],
    })
    target.innerHTML = cleanHTML
    decoratePreview(target)
    renderMath(target)
    highlightCode(target)
    await renderDiagrams(target, sequence, renderTheme)
    if (sequence !== renderSequence) return false
    renderState.value = t('status.rendered')
    return true
  } catch (error) {
    if (sequence !== renderSequence) return false
    target.textContent = t('error.markdownRenderFailed', { message: errorMessage(error) })
    target.classList.add('render-error')
    renderState.value = t('status.renderFailed')
    return false
  }
}

function decoratePreview(root: HTMLElement) {
  root.classList.remove('render-error')
  linkifyTextNodes(root)

  root.querySelectorAll<HTMLQuoteElement>('blockquote').forEach((blockquote) => {
    if (blockquote.classList.contains('markdown-alert')) return
    const first = blockquote.firstElementChild
    const match = first?.textContent?.match(/^\s*\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*/i)
    if (!first || !match) return
    const type = match[1].toLowerCase()
    const labels: Record<string, string> = {
      note: t('alert.note'),
      tip: t('alert.tip'),
      important: t('alert.important'),
      warning: t('alert.warning'),
      caution: t('alert.caution'),
    }
    first.textContent = (first.textContent || '').replace(match[0], '')
    const title = document.createElement('div')
    title.className = 'markdown-alert-title'
    title.textContent = labels[type]
    blockquote.classList.add('markdown-alert', `markdown-alert-${type}`)
    blockquote.prepend(title)
  })

  root.querySelectorAll('table').forEach((table) => {
    if (table.parentElement?.classList.contains('table-scroll')) return
    const wrapper = document.createElement('div')
    wrapper.className = 'table-scroll'
    table.before(wrapper)
    wrapper.append(table)
  })

  root.querySelectorAll<HTMLInputElement>('input[type="checkbox"]').forEach((checkbox) => {
    checkbox.disabled = true
  })
  root.querySelectorAll<HTMLAnchorElement>('a[href]').forEach((anchor) => {
    anchor.rel = 'noopener noreferrer'
    anchor.addEventListener('click', (event) => {
      const href = anchor.getAttribute('href') || ''
      if (/^(https?:|mailto:)/i.test(href)) {
        event.preventDefault()
        void OpenExternal(href)
      }
    })
  })
  root.querySelectorAll<HTMLElement>('h1,h2,h3,h4,h5,h6').forEach((heading, index) => {
    heading.id = `heading-${index + 1}`
  })
}

function linkifyTextNodes(root: HTMLElement) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  const nodes: Text[] = []
  while (walker.nextNode()) {
    const node = walker.currentNode as Text
    if (!node.parentElement?.closest('a, code, pre, script, style') && /https?:\/\//.test(node.data)) {
      nodes.push(node)
    }
  }
  nodes.forEach((node) => {
    const expression = /https?:\/\/[^\s<>]+/g
    const fragment = document.createDocumentFragment()
    let cursor = 0
    let match: RegExpExecArray | null
    while ((match = expression.exec(node.data))) {
      fragment.append(node.data.slice(cursor, match.index))
      let href = match[0]
      let trailing = ''
      while (/[),.;!?，。；！？：]$/.test(href)) {
        trailing = href.slice(-1) + trailing
        href = href.slice(0, -1)
      }
      const anchor = document.createElement('a')
      anchor.href = href
      anchor.textContent = href
      fragment.append(anchor, trailing)
      cursor = match.index + match[0].length
    }
    fragment.append(node.data.slice(cursor))
    node.replaceWith(fragment)
  })
}

function renderMath(root: HTMLElement) {
  root.querySelectorAll<HTMLElement>('.math-source').forEach((node) => {
    const formula = node.textContent || ''
    const displayMode = node.dataset.displayMode === 'block'
    try {
      node.innerHTML = katex.renderToString(formula, {
        displayMode,
        output: 'htmlAndMathml',
        throwOnError: false,
        strict: 'warn',
        trust: false,
      })
      node.classList.add('math-rendered')
    } catch {
      node.textContent = formula
      node.classList.add('math-error')
    }
  })
}

function highlightCode(root: HTMLElement) {
  root.querySelectorAll<HTMLElement>('pre code').forEach((block) => {
    if (block.closest('.mermaid')) return
    try {
      hljs.highlightElement(block)
    } catch {
      // Unknown languages intentionally remain readable as plain code.
    }
  })
}

async function renderDiagrams(root: HTMLElement, sequence: number, renderTheme: Theme) {
  const diagrams = Array.from(root.querySelectorAll<HTMLElement>('pre.mermaid'))
  if (!diagrams.length) return
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: 'strict',
    theme: renderTheme === 'dark' ? 'dark' : 'base',
    htmlLabels: false,
    fontFamily: '-apple-system, BlinkMacSystemFont, PingFang SC, Microsoft YaHei, sans-serif',
    flowchart: { useMaxWidth: true, htmlLabels: false },
  })
  for (let index = 0; index < diagrams.length; index += 1) {
    if (sequence !== renderSequence) return
    const diagram = diagrams[index]
    const definition = diagram.textContent || ''
    try {
      const id = `inkmark-diagram-${sequence}-${index}`
      const rendered = await mermaid.render(id, definition)
      if (sequence !== renderSequence) return
      diagram.innerHTML = rendered.svg
      diagram.classList.add('mermaid-rendered')
      rendered.bindFunctions?.(diagram)
    } catch {
      diagram.textContent = definition
      diagram.classList.add('mermaid-error')
    }
  }
}

function chooseTheme(value: Theme) {
  if (busy.value) return
  theme.value = value
  localStorage.setItem('inkmark-theme', value)
  document.documentElement.dataset.colorScheme = value === 'dark' ? 'dark' : 'light'
  updateNativeMenu()
  void renderNow()
}

function chooseView(value: ViewMode) {
  if (busy.value) return
  scrollSync.reset()
  viewMode.value = value
  localStorage.setItem('inkmark-view', value)
  updateNativeMenu()
}

function runFormat(action: string) {
  if (busy.value) return
  const target = editor.value
  if (!target) return
  const start = target.selectionStart
  const end = target.selectionEnd
  const selected = target.value.slice(start, end)
  let replacement = selected
  let selectionStart = start
  let selectionEnd = end

  const wrap = (before: string, after: string, placeholder: string) => {
    const text = selected || placeholder
    replacement = before + text + after
    selectionStart = start + before.length
    selectionEnd = selectionStart + text.length
  }
  const prefixLines = (prefix: string) => {
    const lineStart = target.value.lastIndexOf('\n', start - 1) + 1
    const nextBreak = target.value.indexOf('\n', end)
    const lineEnd = nextBreak === -1 ? target.value.length : nextBreak
    const block = target.value.slice(lineStart, lineEnd)
    replacement = block.split('\n').map((line) => prefix + line).join('\n')
    target.setRangeText(replacement, lineStart, lineEnd, 'select')
    source.value = target.value
    target.focus()
    return
  }

  if (action === 'bold') wrap('**', '**', t('format.bold'))
  else if (action === 'italic') wrap('*', '*', t('format.italic'))
  else if (action === 'strike') wrap('~~', '~~', t('format.strikethrough'))
  else if (action === 'link') wrap('[', '](https://)', t('format.linkText'))
  else if (action === 'code') wrap('`', '`', 'code')
  else if (action === 'codeblock') wrap('\n```\n', '\n```\n', t('format.codePlaceholder'))
  else if (action === 'heading') return prefixLines('## ')
  else if (action === 'quote') return prefixLines('> ')
  else if (action === 'ul') return prefixLines('- ')
  else if (action === 'ol') return prefixLines('1. ')
  else if (action === 'task') return prefixLines('- [ ] ')
  else if (action === 'table') {
    replacement = t('format.tableTemplate')
    selectionStart = selectionEnd = start + replacement.length
  }
  target.setRangeText(replacement, start, end, 'end')
  source.value = target.value
  nextTick(() => {
    target.focus()
    target.setSelectionRange(selectionStart, selectionEnd)
  })
}

function beginScroll(pane: ScrollPane) {
  if (syncScroll.value) scrollSync.begin(pane)
}

function syncFromEditor() {
  if (!syncScroll.value || !editor.value || !previewPane.value) return
  scrollSync.sync('editor', editor.value, previewPane.value)
}

function syncFromPreview() {
  if (!syncScroll.value || !editor.value || !previewPane.value) return
  scrollSync.sync('preview', previewPane.value, editor.value)
}

async function copyHTML() {
  if (busy.value) return
  try {
    await navigator.clipboard.writeText(preview.value?.innerHTML || '')
    renderState.value = t('status.htmlCopied')
  } catch {
    renderState.value = t('status.copyFailed')
  }
}

function onKeydown(event: KeyboardEvent) {
  if (busy.value) return
  if (event.key === 'Escape' && activeDialog.value) {
    activeDialog.value = null
    return
  }
  if (!(event.metaKey || event.ctrlKey)) return
  const key = event.key.toLowerCase()
  if (key === 'b') {
    event.preventDefault()
    runFormat('bold')
  } else if (key === 'i') {
    event.preventDefault()
    runFormat('italic')
  } else if (key === 'k') {
    event.preventDefault()
    runFormat('link')
  }
}

function onBeforeUnload(event: BeforeUnloadEvent) {
  if (quitting || !dirty.value) return
  event.preventDefault()
  event.returnValue = ''
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

function showError(error: unknown) {
  renderState.value = errorMessage(error)
}

watch(source, () => scheduleRender())
watch(syncScroll, () => {
  scrollSync.reset()
  updateNativeMenu()
})
watch([fileName, dirty], () => { void SetWindowTitle(fileName.value, dirty.value) })

onMounted(async () => {
  document.documentElement.dataset.colorScheme = theme.value === 'dark' ? 'dark' : 'light'
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('beforeunload', onBeforeUnload)
  window.addEventListener('languagechange', handleSystemLanguageChange)
  removeMenuListeners.push(
    EventsOn('inkmark:menu-action', handleMenuAction),
    EventsOn('inkmark:open-document', handleSystemDocument),
    EventsOn('inkmark:open-error', showError),
  )
  void loadApplicationInfo()
  try {
    const persisted = await GetLanguageSettings()
    const locallyStoredMode = localStorage.getItem(languageModeStorageKey)
    const initialMode = normalizeLanguageMode(locallyStoredMode ?? persisted?.mode)
    await applyLanguageMode(initialMode, false)
    const initial = await LoadInitialDocument(locale.value) as DocumentData
    setDocument(initial, 'status.ready')
  } catch (error) {
    showError(error)
  }
})

onBeforeUnmount(() => {
  window.clearTimeout(renderTimer)
  removeMenuListeners.splice(0).forEach((removeListener) => removeListener())
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('beforeunload', onBeforeUnload)
  window.removeEventListener('languagechange', handleSystemLanguageChange)
})
</script>

<template>
  <div class="app-shell" :class="[`theme-${theme}`, `view-${viewMode}`]">
    <header class="document-header">
      <div class="brand" :aria-label="t('app.fullName')">
        <img class="brand-mark" :src="inkmarkIcon" alt="" />
        <div>
          <strong>{{ t('app.name') }}</strong>
          <span>MARKDOWN</span>
        </div>
      </div>

      <div class="document-identity">
        <div class="document-title-row">
          <span v-if="dirty" class="dirty-dot" :title="t('document.dirtyTitle')"></span>
          <h1>{{ fileName }}</h1>
          <span class="document-state">{{ dirty ? t('document.modified') : t('document.saved') }}</span>
        </div>
        <p :title="locationLabel">{{ locationLabel }}</p>
      </div>

      <div class="header-actions">
        <span class="offline-badge"><i></i> {{ t('app.offline') }}</span>
        <button type="button" class="settings-button" :title="t('settings.title')" @click="activeDialog = 'settings'" aria-haspopup="dialog">⚙</button>
      </div>
    </header>

    <nav class="command-bar" :aria-label="t('toolbar.ariaLabel')">
      <div class="format-toolbar" :aria-label="t('toolbar.markdownFormat')">
        <button
          v-for="item in formatActions"
          :key="item.action"
          type="button"
          :title="item.title"
          :class="{ italic: item.action === 'italic', strike: item.action === 'strike' }"
          :disabled="busy"
          @click="runFormat(item.action)"
        >{{ item.label }}</button>
      </div>

      <div class="toolbar-spacer"></div>

      <label class="sync-option">
        <input v-model="syncScroll" type="checkbox" :disabled="busy" />
        <span>{{ t('toolbar.syncScroll') }}</span>
      </label>

      <div class="segmented" :aria-label="t('toolbar.viewMode')">
        <button type="button" :class="{ active: viewMode === 'edit' }" :disabled="busy" @click="chooseView('edit')">{{ t('toolbar.edit') }}</button>
        <button type="button" :class="{ active: viewMode === 'split' }" :disabled="busy" @click="chooseView('split')">{{ t('toolbar.split') }}</button>
        <button type="button" :class="{ active: viewMode === 'preview' }" :disabled="busy" @click="chooseView('preview')">{{ t('toolbar.preview') }}</button>
      </div>

      <div class="theme-picker" :aria-label="t('toolbar.previewStyle')">
        <span>{{ t('toolbar.layout') }}</span>
        <button
          v-for="item in themes"
          :key="item.value"
          type="button"
          :class="{ active: theme === item.value }"
          :disabled="busy"
          @click="chooseTheme(item.value)"
        >{{ item.label }}</button>
      </div>

      <button type="button" class="copy-button" :disabled="busy" @click="copyHTML">{{ t('toolbar.copyHTML') }}</button>
    </nav>

    <main class="editor-layout">
      <section class="source-panel" :aria-label="t('panel.sourceAriaLabel')">
        <div class="panel-caption">
          <span>{{ t('panel.source') }}</span>
          <small>UTF-8</small>
        </div>
        <textarea
          ref="editor"
          v-model="source"
          class="source-editor"
          :aria-label="t('panel.editorAriaLabel')"
          :disabled="busy"
          spellcheck="false"
          @keydown="beginScroll('editor')"
          @pointerdown="beginScroll('editor')"
          @scroll="syncFromEditor"
          @touchstart.passive="beginScroll('editor')"
          @wheel.passive="beginScroll('editor')"
        ></textarea>
      </section>

      <section class="preview-panel" :aria-label="t('panel.previewAriaLabel')">
        <div class="panel-caption preview-caption">
          <span>{{ t('panel.preview') }}</span>
          <small>{{ theme === 'wechat' ? 'WECHAT' : theme.toUpperCase() }}</small>
        </div>
        <div
          ref="previewPane"
          class="preview-viewport"
          tabindex="0"
          @keydown="beginScroll('preview')"
          @pointerdown="beginScroll('preview')"
          @scroll="syncFromPreview"
          @touchstart.passive="beginScroll('preview')"
          @wheel.passive="beginScroll('preview')"
        >
          <article ref="preview" class="markdown-body"></article>
        </div>
      </section>
    </main>

    <footer class="status-bar">
      <span class="ready-light"></span>
      <span>{{ renderState }}</span>
      <span class="status-spacer"></span>
      <span>{{ t('status.characters', { count: characterCount.toLocaleString(locale) }) }}</span>
      <span>{{ t('status.lines', { count: lineCount.toLocaleString(locale) }) }}</span>
    </footer>

    <div v-if="activeDialog" class="modal-backdrop" @click.self="activeDialog = null">
      <section class="app-dialog" role="dialog" aria-modal="true" :aria-labelledby="`dialog-${activeDialog}`">
        <template v-if="activeDialog === 'settings'">
          <h2 id="dialog-settings">{{ t('settings.title') }}</h2>
          <div class="settings-row">
            <label for="language-select">{{ t('settings.language') }}</label>
            <div>
              <select id="language-select" :value="languageMode" @change="handleLanguageChange">
                <option value="auto">{{ t('settings.languageAuto') }}</option>
                <option value="zh-CN">{{ t('settings.languageChinese') }}</option>
                <option value="en">{{ t('settings.languageEnglish') }}</option>
              </select>
              <p>{{ t('settings.languageDescription') }}</p>
              <p v-if="languageMode === 'auto'" class="detected-language">
                {{ t('settings.autoDetected', { language: locale === 'zh-CN' ? t('settings.languageChinese') : t('settings.languageEnglish') }) }}
              </p>
            </div>
          </div>
        </template>
        <template v-else-if="activeDialog === 'shortcuts'">
          <h2 id="dialog-shortcuts">{{ t('help.shortcutsTitle') }}</h2>
          <p>{{ t('help.shortcutsIntro') }}</p>
          <dl class="shortcut-list">
            <template v-for="row in shortcutRows" :key="row[0]">
              <dt><kbd>{{ row[0] }}</kbd></dt>
              <dd>{{ row[1] }}</dd>
            </template>
          </dl>
        </template>
        <template v-else>
          <div class="about-heading">
            <img :src="inkmarkIcon" alt="" />
            <div>
              <h2 id="dialog-about">{{ t('help.aboutTitle') }}</h2>
              <strong>{{ t('app.fullName') }}</strong>
            </div>
          </div>
          <p>{{ t('help.aboutBody') }}</p>
          <dl class="shortcut-list">
            <dt>{{ t('help.version') }}</dt>
            <dd>{{ aboutVersion }}</dd>
            <dt>{{ t('help.author') }}</dt>
            <dd>{{ aboutAuthor }}</dd>
            <dt>{{ t('help.sourceRepository') }}</dt>
            <dd>
              <a
                v-if="applicationInfo.repositoryURL"
                href="#"
                :title="t('help.openRepository')"
                @click.prevent="openSourceRepository"
              >{{ applicationInfo.repositoryURL }}</a>
              <span v-else>{{ t('help.repositoryUnavailable') }}</span>
            </dd>
            <dt>{{ t('help.updateStatus') }}</dt>
            <dd role="status" aria-live="polite">{{ updateStatusText }}</dd>
            <template v-if="updateInfo">
              <dt>{{ t('help.currentVersion') }}</dt>
              <dd>{{ updateInfo.currentVersion || aboutVersion }}</dd>
              <dt>{{ t('help.latestVersion') }}</dt>
              <dd>{{ updateInfo.latestVersion || '—' }}</dd>
              <template v-if="updatePublishedAt">
                <dt>{{ t('help.publishedAt') }}</dt>
                <dd>{{ updatePublishedAt }}</dd>
              </template>
            </template>
          </dl>
        </template>
        <footer class="dialog-actions">
          <button
            v-if="activeDialog === 'about' && updateState !== 'available'"
            type="button"
            class="button secondary"
            :disabled="updateState === 'checking'"
            @click="checkForUpdates()"
          >{{ updateState === 'checking' ? t('help.checkingUpdate') : t('help.checkUpdate') }}</button>
          <button
            v-else-if="activeDialog === 'about'"
            type="button"
            class="button secondary"
            @click="openUpdatePage"
          >{{ t('help.downloadUpdate') }}</button>
          <span v-if="activeDialog === 'about'" class="status-spacer"></span>
          <button type="button" class="button primary" @click="activeDialog = null">{{ t('common.close') }}</button>
        </footer>
      </section>
    </div>
  </div>
</template>
