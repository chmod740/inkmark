<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'
import katex from 'katex'
import MarkdownIt from 'markdown-it'
import taskLists from 'markdown-it-task-lists'
import { katex as markdownKatex } from '@mdit/plugin-katex'
import mermaid from 'mermaid'
import DirectorySidebar from './DirectorySidebar.vue'
import inkmarkIcon from './assets/inkmark-icon.svg?no-inline'
import {
  buildStandaloneHTML,
  bytesToBase64,
  collectEmbeddedStyles,
  type ExportFormat,
  utf8ToBase64,
} from './export-document'
import { runGuardedDocumentTransition, type DocumentTransition, type UnsavedDecision } from './document-guard'
import {
  ScrollSyncController,
  sampleAnchorIndices,
  shouldMeasureEditorAnchors,
  type ScrollAnchor,
  type ScrollMapping,
  type ScrollPane,
} from './scroll-sync'
import {
  BoundedCache,
  LatestPreviewCommit,
  maximumMermaidCacheEntries,
  mermaidCacheKey,
} from './preview-render'
import {
  normalizePreviewFirst,
  previewFirstStorageKey,
  resolveDocumentHeaderState,
  togglePreviewFirst,
  type BuiltInDocumentKind,
} from './ui-state'
import {
  dispatchTextEditInput,
  isTextEditControl,
  resolveTextEditControl,
} from './edit-actions'
import {
  ImageResolverGate,
  ImageDecodeGate,
  PreviewImageBudget,
  PreviewImageResourceSet,
  buildMarkdownImage,
  classifyImageSource,
  defaultImageAlt,
  dataURIImageAsset,
  exportImageSource,
  forEachWithConcurrency,
  imageDataURI,
  imageResourceCacheKey,
  maximumPreviewImages,
  maximumConcurrentImageResolvers,
  resolvePreparedImageAsset,
  type ImageAsset,
  type ImageAssetData,
  type PendingImageAssetRequest,
  type PreparedImageResource,
} from './image-resources'
import {
  SavedWebDAVFormError,
  buildSavedWebDAVConnectionInput,
  clearedWebDAVConnectionForm,
  normalizeSavedWebDAVConnections,
  resolveRecentWebDAVOpen,
  webDAVOriginChanged,
  type SavedWebDAVConnection,
} from './saved-webdav'
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
import { UpdateDownloadSessionGate } from './update-session'
import {
  flattenWorkspaceTree,
  loadedWorkspaceDirectoryKeys,
  normalizeWorkspace,
  normalizeWorkspaceDirectory,
  normalizeWorkspaceEntries,
  retainExistingWorkspaceDirectories,
  isActiveWorkspaceFile,
  isWorkspaceBackedLocalDocument,
  localWorkspaceRelativePath,
  localWorkspaceAbsolutePath,
  rebaseWorkspacePath,
  sameWorkspaceFile,
  workspaceBackendDirectoryPath,
  workspaceDirectoryExists,
  workspaceDirectoryKey,
  workspaceJoinPath,
  workspaceParentPath,
  workspacePathIsWithin,
  workspaceRootKey,
  type WorkspaceChildren,
  type WorkspaceData,
  type WorkspaceEntryData,
  type WorkspaceProvider,
} from './workspace-tree'
import {
  ActivateRecentDocument,
  BeginWebDAVMutation,
  CancelUpdateDownload,
  CancelQuitRequest,
  CancelWebDAVMutation,
  CheckForUpdates,
  ClearRecentItems,
  CloseWorkspaceDocument,
  CloseWebDAVDocument,
  CloseWorkspace,
  CloseWebDAVWorkspace,
  ConnectSavedWebDAV,
  ConnectWebDAV,
  ConfirmQuit,
  CommitWebDAVDelete,
  CommitWebDAVRename,
  CreateWebDAVDirectory,
  CreateWebDAVMarkdownFile,
  CreateWorkspaceDirectory,
  CreateWorkspaceMarkdownFile,
  DownloadUpdate,
  DeleteSavedWebDAVConnection,
  DeleteWorkspaceEntry,
  FetchPublicImage,
  GetAppInfo,
  GetLanguageSettings,
  GetThirdPartyNotices,
  LoadInitialDocument,
  OpenDirectory,
  OpenExternal,
  OpenFile,
  OverwriteWebDAVFile,
  OpenRecentDirectory,
  OpenRecentFile,
  OpenRecentWebDAV,
  OpenUpdatePage,
  OpenWebDAVFile,
  OpenWorkspaceFile,
  ImportLocalImageData,
  ImportWebDAVImageData,
  ListWebDAVDirectory,
  ReadWorkspaceDirectory,
  ReadWebDAVWorkspaceImage,
  ReadWorkspaceImage,
  ResolveLocalImage,
  ResolveWebDAVImage,
  RenameWorkspaceEntry,
  ValidateImageData,
  LaunchUpdateInstaller,
  ListSavedWebDAVConnections,
  RenderingTestDocument as LoadRenderingTestDocument,
  SaveExportFile,
  SaveFile,
  SaveFileAs,
  SaveWorkspaceMarkdownFile,
  SaveWebDAVFile,
  SaveWebDAVConnection,
  SelectImageFile,
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
type ImageInsertMode = 'local' | 'data' | 'webdav' | 'public'
type WebDAVConnectionFormMode = 'connect' | 'new' | 'edit'
type WebDAVDialogView = 'saved' | 'temporary' | 'new' | 'edit'
type WebDAVConnectionOperation = 'idle' | 'connecting-saved' | 'saving' | 'deleting'
type AboutView = 'overview' | 'third-party'
type WorkspaceDialogView = 'create-markdown' | 'create-directory' | 'rename' | 'delete' | 'image'
type WorkspaceMutationOperation = 'rename' | 'delete'

interface DocumentData {
  path: string
  name: string
  content: string
  welcome?: boolean
  builtIn?: string
  activationId?: string
  storageKind?: 'local' | 'webdav' | 'builtin'
  displayLocation?: string
  workspacePath?: string
  workspaceId?: string
  localDocumentId?: string
  remoteDocumentId?: string
  etag?: string
}

interface WebDAVSaveResultData {
  path: string
  name: string
  etag: string
  conflict: boolean
}

interface WebDAVMutationData {
  mutationId: string
  entry: WorkspaceEntryData
  expiresAt: string
}

interface ActiveWorkspaceMutation {
  workspaceId: string
  mutationId: string
  operation: WorkspaceMutationOperation
  expiresAt: string
}

interface RecentOpenEventData {
  id: string
  kind: 'file' | 'directory' | 'folder' | 'webdav'
  name: string
}

interface RecentWebDAVConnectionData {
  endpoint: string
  name: string
  savedConnectionId?: string
  hasSavedCredentials?: boolean
}

type PendingWorkspaceOpenRequest =
  | { kind: 'picker' }
  | { kind: 'recent-directory'; id: string }

interface ApplicationInfoData {
  version: string
  author: string
}

interface UpdateInfoData {
  currentVersion: string
  latestVersion: string
  updateAvailable: boolean
  releaseURL: string
  downloadURL: string
  publishedAt: string
  assetName: string
  assetSize: number
  installable: boolean
  checksumAvailable: boolean
  installerKind: string
}

interface UpdateDownloadData {
  sessionID: string
  assetName: string
  version: string
  bytesDownloaded: number
  totalBytes: number
  progress: number
  ready: boolean
}

type UpdateState = 'idle' | 'checking' | 'current' | 'available' | 'downloading' | 'cancelling' | 'ready' | 'installing' | 'unavailable' | 'error'

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

interface ImageRenderContext {
  storageKind: 'local' | 'webdav' | 'builtin'
  localDocumentPath: string
  remoteWorkspaceId: string
  remoteDocumentId: string
}

const source = ref('')
const savedSource = ref('')
const localDocumentPath = ref('')
const documentLocation = ref('')
const documentStorageKind = ref<'local' | 'webdav' | 'builtin'>('builtin')
const documentWorkspaceId = ref('')
const currentWorkspacePath = ref('')
const localDocumentId = ref('')
const remoteDocumentId = ref('')
const remoteWorkspaceId = ref('')
const remoteDocumentETag = ref('')
const fileName = ref('README.md')
const theme = ref<Theme>(readPreference<Theme>('inkmark-theme', 'github'))
const viewMode = ref<ViewMode>(readPreference<ViewMode>('inkmark-view', 'split'))
const previewFirst = ref(normalizePreviewFirst(localStorage.getItem(previewFirstStorageKey)))
const languageMode = ref<LanguageMode>('auto')
const locale = ref<Locale>('en')
const renderState = ref('')
const busy = ref(false)
const editor = ref<HTMLTextAreaElement | null>(null)
const preview = ref<HTMLElement | null>(null)
const previewPane = ref<HTMLElement | null>(null)
const appDialog = ref<HTMLElement | null>(null)
const syncScroll = ref(true)
const builtInDocument = ref<BuiltInDocumentKind | null>(null)
const activeDialog = ref<'settings' | 'shortcuts' | 'about' | 'webdav' | 'image' | 'workspace' | null>(null)
const aboutView = ref<AboutView>('overview')
const thirdPartyNotices = ref('')
const thirdPartyNoticesState = ref<'idle' | 'loading' | 'ready' | 'error'>('idle')
const unsavedTransition = ref<DocumentTransition | null>(null)
const resolvingUnsavedPrompt = ref(false)
const applicationInfo = ref<ApplicationInfoData>({ version: '', author: '' })
const updateInfo = ref<UpdateInfoData | null>(null)
const updateState = ref<UpdateState>('idle')
const updateDownload = ref<UpdateDownloadData | null>(null)
const updateError = ref('')
const workspace = ref<WorkspaceData | null>(null)
const workspaceChildren = ref<WorkspaceChildren>({})
const expandedWorkspaceDirectories = ref<ReadonlySet<string>>(new Set())
const loadingWorkspaceDirectories = ref<ReadonlySet<string>>(new Set())
const truncatedWorkspaceDirectories = ref<ReadonlySet<string>>(new Set())
const workspaceRefreshing = ref(false)
const webDAVBaseURL = ref('')
const webDAVUsername = ref('')
const webDAVPassword = ref('')
const showWebDAVPassword = ref(false)
const webDAVConnecting = ref(false)
const webDAVConnectionError = ref('')
const webDAVConflictOpen = ref(false)
const webDAVConflictBusy = ref(false)
const savedWebDAVConnections = ref<SavedWebDAVConnection[]>([])
const savedWebDAVConnectionsLoading = ref(false)
const savedWebDAVConnectionsError = ref('')
const webDAVConnectionFormMode = ref<WebDAVConnectionFormMode>('connect')
const webDAVDialogView = ref<WebDAVDialogView>('saved')
const webDAVEditingConnectionID = ref('')
const webDAVConnectionName = ref('')
const webDAVStoreCredentials = ref(false)
const webDAVRemoveCredentials = ref(false)
const webDAVConnectionManagerBusy = ref(false)
const webDAVConnectionOperation = ref<WebDAVConnectionOperation>('idle')
const webDAVConnectingConnectionID = ref('')
const webDAVDeleteCandidate = ref<SavedWebDAVConnection | null>(null)
const imageInsertMode = ref<ImageInsertMode>('data')
const selectedImage = ref<ImageAssetData | null>(null)
const imageAltText = ref('')
const publicImageURL = ref('')
const imageInsertBusy = ref(false)
const imageInsertError = ref('')
const workspaceDialogView = ref<WorkspaceDialogView>('create-markdown')
const workspaceDialogEntry = ref<WorkspaceEntryData | null>(null)
const workspaceDialogParentPath = ref('')
const workspaceEntryName = ref('')
const workspaceMutationBusy = ref(false)
const workspaceMutationError = ref('')
const workspaceMutationNeedsRestart = ref(false)
const workspacePreviewImage = ref<ImageAssetData | null>(null)
const workspacePreviewSource = ref('')
const workspaceDeleteBufferPreserved = ref(false)
const activeWorkspaceMutation = ref<ActiveWorkspaceMutation | null>(null)
const workspaceMutationPreparing = ref(false)
const workspaceMutationCancelling = ref(false)
let workspacePreviewGeneration = 0
let workspaceMutationBeginGeneration = 0
let workspaceMutationCancelGeneration = 0
let pendingWorkspaceMutationBegin: Promise<WorkspaceEntryData | null> | null = null
let pendingWorkspaceMutationCancel: Promise<boolean> | null = null

let renderTimer: number | undefined
let quitting = false
let pendingUpdateInstall = false
let updateDownloadCancelled = false
let documentTransitionInProgress = false
let pendingWorkspaceOpenRequest: PendingWorkspaceOpenRequest | null = null
let workspaceRefreshQueued = false
let savedConnectionsLoadGeneration = 0
let drainingSystemDocuments = false
const pendingSystemDocuments: DocumentData[] = []
let resolveUnsavedDecision: ((decision: UnsavedDecision) => void) | null = null
let layoutResizeObserver: ResizeObserver | null = null
let layoutReconcileFrame: number | null = null
let editorScrollAnchors: ScrollAnchor[] = []
let previewScrollAnchors: ScrollAnchor[] = []
let lastFocusedElement: Element | null = null
let dialogReturnFocus: HTMLElement | null = null
let webDAVDeleteReturnFocus: HTMLElement | null = null
const removeMenuListeners: Array<() => void> = []
const scrollSync = new ScrollSyncController()
const previewCommit = new LatestPreviewCommit()
const imageResolverGate = new ImageResolverGate()
const imageDecodeGate = new ImageDecodeGate()
const pendingImageAssets = new Map<string, PendingImageAssetRequest>()
let activePreviewImages = new PreviewImageResourceSet()
const workspacePreviewImages = new PreviewImageResourceSet()
type MermaidRenderResult = Awaited<ReturnType<typeof mermaid.render>>
const mermaidRenderCache = new BoundedCache<string, MermaidRenderResult>(maximumMermaidCacheEntries)
const updateDownloadSessions = new UpdateDownloadSessionGate()

const workspaceRows = computed(() => flattenWorkspaceTree(
  workspaceChildren.value,
  expandedWorkspaceDirectories.value,
  loadingWorkspaceDirectories.value,
))
const currentWorkspaceProvider = computed<WorkspaceProvider | null>(() => {
  if (!documentWorkspaceId.value || !currentWorkspacePath.value) return null
  if (documentStorageKind.value === 'webdav') return 'webdav'
  if (documentStorageKind.value === 'local') return 'local'
  return null
})
const workspaceLabels = computed(() => ({
  title: t('workspace.title'),
  close: t('workspace.close'),
  refresh: t('workspace.refresh'),
  refreshing: t('workspace.refreshing'),
  empty: t('workspace.empty'),
  loading: t('workspace.loading'),
  truncatedRoot: t('workspace.truncatedRoot'),
  truncatedDirectory: t('workspace.truncatedDirectory'),
  expandDirectory: t('workspace.expandDirectory'),
  collapseDirectory: t('workspace.collapseDirectory'),
  openFile: t('workspace.openFile'),
  previewImage: t('workspace.previewImage'),
  contextMenu: t('workspace.contextMenu'),
  rootActions: t('workspace.rootActions'),
  newMarkdown: t('workspace.newMarkdown'),
  newDirectory: t('workspace.newDirectory'),
  rename: t('workspace.rename'),
  delete: t('workspace.delete'),
  providerWebDAV: t('workspace.providerWebDAV'),
}))
const workspaceDialogTitle = computed(() => {
  if (workspaceDialogView.value === 'create-markdown') return t('workspace.newMarkdownTitle')
  if (workspaceDialogView.value === 'create-directory') return t('workspace.newDirectoryTitle')
  if (workspaceDialogView.value === 'rename') return t('workspace.renameTitle')
  if (workspaceDialogView.value === 'delete') return t('workspace.deleteTitle')
  return t('workspace.imagePreviewTitle')
})
const workspaceDeleteAffectsCurrentDocument = computed(() => Boolean(
  workspace.value
  && workspaceDialogEntry.value
  && documentWorkspaceId.value === workspace.value.id
  && workspacePathIsWithin(currentWorkspacePath.value, workspaceDialogEntry.value.path),
))
const workspaceDeleteDescription = computed(() => {
  const entry = workspaceDialogEntry.value
  if (!entry) return ''
  const key = entry.kind === 'directory'
    ? 'workspace.deleteDirectoryMessage'
    : 'workspace.deleteFileMessage'
  return t(key, { name: entry.name })
})
const workspaceDialogParentLabel = computed(() => {
  const activeWorkspace = workspace.value
  if (!activeWorkspace) return workspaceDialogParentPath.value || '/'
  return workspaceDialogParentPath.value
    ? `${activeWorkspace.name}/${workspaceDialogParentPath.value}`
    : activeWorkspace.name
})
const workspaceMutationLockLabel = computed(() => {
  const mutation = activeWorkspaceMutation.value
  if (!mutation) return ''
  const timestamp = Date.parse(mutation.expiresAt)
  if (!Number.isFinite(timestamp)) return t('workspace.webDAVMutationLocked')
  const expiresAt = new Intl.DateTimeFormat(locale.value, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(timestamp)
  return t('workspace.webDAVMutationLockedUntil', { time: expiresAt })
})

const markdown = new MarkdownIt({
  html: true,
  breaks: true,
  linkify: true,
  typographer: true,
})
markdown.use(taskLists, { enabled: false, label: true, labelAfter: true })
markdown.use(markdownKatex, { delimiters: 'all', throwOnError: false })
markdown.core.ruler.push('inkmark-source-lines', (state) => {
  state.tokens.forEach((token) => {
    if (!token.map) return
    if (token.nesting === 1 || ['fence', 'code_block', 'hr', 'html_block', 'math_block'].includes(token.type)) {
      token.attrSet('data-source-line', String(token.map[0]))
    }
  })
})
// Markdown images are emitted without a src attribute. A browser may start
// decoding or loading an image as soon as detached HTML is parsed, so the
// original destination remains inert until preparePreviewImages validates it
// through the local, WebDAV, Data URI, or public-HTTPS resolver.
markdown.renderer.rules.image = (tokens, index) => {
  const token = tokens[index]
  const source = markdown.utils.escapeHtml(String(token.attrGet('src') || ''))
  const alt = markdown.utils.escapeHtml(token.content || '')
  const title = markdown.utils.escapeHtml(String(token.attrGet('title') || ''))
  const titleAttribute = title ? ` data-inkmark-image-title="${title}"` : ''
  return `<span class="inkmark-image-placeholder" data-inkmark-image-source="${source}" data-inkmark-image-alt="${alt}"${titleAttribute}></span>`
}
markdown.renderer.rules.math_inline = (tokens, index) =>
  `<span class="math-source" data-display-mode="inline">${markdown.utils.escapeHtml(tokens[index].content)}</span>`
markdown.renderer.rules.math_block = (tokens, index) => {
  const sourceLine = tokens[index].map?.[0]
  const sourceAttribute = sourceLine === undefined ? '' : ` data-source-line="${sourceLine}"`
  return `<div class="math-source" data-display-mode="block"${sourceAttribute}>${markdown.utils.escapeHtml(tokens[index].content.trim())}</div>`
}
const defaultFence = markdown.renderer.rules.fence || ((tokens, index, options, _env, self) => self.renderToken(tokens, index, options))
markdown.renderer.rules.fence = (tokens, index, options, env, self) => {
  const language = tokens[index].info.trim().split(/\s+/)[0].toLowerCase()
  if (language === 'mermaid' || language === 'mmd') {
    const sourceLine = tokens[index].map?.[0]
    const sourceAttribute = sourceLine === undefined ? '' : ` data-source-line="${sourceLine}"`
    return `<pre class="mermaid"${sourceAttribute}>${markdown.utils.escapeHtml(tokens[index].content)}</pre>`
  }
  return defaultFence(tokens, index, options, env, self)
}

const dirty = computed(() => source.value !== savedSource.value)
const lineCount = computed(() => source.value ? source.value.split(/\r?\n/).length : 1)
const characterCount = computed(() => Array.from(source.value).length)
const documentHeaderState = computed(() => resolveDocumentHeaderState(
  dirty.value,
  documentLocation.value,
  builtInDocument.value,
))
const documentStateLabel = computed(() => {
  if (documentHeaderState.value.status === 'modified') return t('document.modified')
  if (documentHeaderState.value.status === 'saved') return t('document.saved')
  if (documentHeaderState.value.status === 'built-in') return t('document.builtIn')
  return t('document.unsaved')
})
const locationLabel = computed(() => {
  if (documentHeaderState.value.location === 'path') return documentLocation.value
  if (documentHeaderState.value.location === 'welcome') return t('document.welcomeLocation')
  if (documentHeaderState.value.location === 'render-test') return t('document.renderTestLocation')
  return t('document.unsavedLocation')
})
const connectionStatusLabel = computed(() => {
  if (webDAVConnecting.value) return t('webdav.connecting')
  if (workspace.value?.provider === 'webdav' || documentStorageKind.value === 'webdav') {
    return t('webdav.connectedBadge')
  }
  return t('app.localMode')
})
const unsavedPromptMessage = computed(() => {
  if (unsavedTransition.value === 'new') {
    return t('unsaved.messageNew', { name: fileName.value })
  }
  if (unsavedTransition.value === 'quit') {
    return t('unsaved.messageQuit', { name: fileName.value })
  }
  return t('unsaved.messageOpen', { name: fileName.value })
})
const aboutVersion = computed(() => applicationInfo.value.version || '—')
const aboutAuthor = computed(() => applicationInfo.value.author || '—')
const updateStatusText = computed(() => {
  const currentVersion = updateInfo.value?.currentVersion || aboutVersion.value
  const latestVersion = updateInfo.value?.latestVersion || currentVersion
  if (updateState.value === 'checking') return t('help.checkingUpdate')
  if (updateState.value === 'current') return t('help.updateCurrent', { version: currentVersion })
  if (updateState.value === 'available') return t('help.updateAvailable', { version: latestVersion })
  if (updateState.value === 'downloading') {
    return t('help.updateDownloading', { progress: Math.round((updateDownload.value?.progress || 0) * 100) })
  }
  if (updateState.value === 'cancelling') return t('help.updateCancelling')
  if (updateState.value === 'ready') return t('help.updateReady', { version: latestVersion })
  if (updateState.value === 'installing') return t('help.updateInstalling', { version: latestVersion })
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
  { action: 'image', label: '▧', title: t('image.insert') },
])

const imageInsertModes = computed<Array<{ value: ImageInsertMode; label: string; disabled: boolean }>>(() => [
  {
    value: 'local',
    label: t('image.modeLocal'),
    disabled: documentStorageKind.value !== 'local' || !localDocumentPath.value,
  },
  { value: 'data', label: t('image.modeData'), disabled: false },
  {
    value: 'webdav',
    label: t('image.modeWebDAV'),
    disabled: documentStorageKind.value !== 'webdav' || !remoteWorkspaceId.value || !remoteDocumentId.value,
  },
  { value: 'public', label: t('image.modePublic'), disabled: false },
])

const canInsertImage = computed(() => {
  if (imageInsertBusy.value) return false
  if (imageInsertMode.value === 'public') return Boolean(publicImageURL.value.trim())
  return Boolean(selectedImage.value?.dataBase64)
})

const editingSavedWebDAVConnection = computed(() => savedWebDAVConnections.value.find(
  (connection) => connection.id === webDAVEditingConnectionID.value,
) || null)
const editingChangesWebDAVOrigin = computed(() => Boolean(
  editingSavedWebDAVConnection.value?.hasCredentials
  && webDAVOriginChanged(editingSavedWebDAVConnection.value.endpoint, webDAVBaseURL.value),
))
const webDAVDialogBusy = computed(() => webDAVConnecting.value
  || webDAVConnectionManagerBusy.value)
const webDAVDialogBusyMessage = computed(() => {
  if (webDAVConnecting.value || webDAVConnectionOperation.value === 'connecting-saved') {
    return t('webdav.connecting')
  }
  if (webDAVConnectionOperation.value === 'saving') return t('webdav.savingConnection')
  if (webDAVConnectionOperation.value === 'deleting') return t('webdav.deletingConnection')
  return t('webdav.processingConnection')
})
const webDAVEndpointInvalid = computed(() => webDAVConnectionError.value === t('webdav.endpointRequired')
  || webDAVConnectionError.value === t('webdav.invalidURL'))
const webDAVConnectingConnectionName = computed(() => savedWebDAVConnections.value.find(
  (connection) => connection.id === webDAVConnectingConnectionID.value,
)?.name || 'WebDAV')
const canSubmitWebDAVForm = computed(() => {
  if (webDAVDialogView.value === 'saved' || webDAVDialogBusy.value || !webDAVBaseURL.value.trim()) return false
  if (webDAVConnectionFormMode.value !== 'connect' && !webDAVConnectionName.value.trim()) return false
  if (
    webDAVConnectionFormMode.value === 'edit'
    && editingChangesWebDAVOrigin.value
    && !webDAVPassword.value
    && !webDAVRemoveCredentials.value
  ) return false
  if (webDAVPassword.value && !webDAVUsername.value.trim()) return false
  return true
})

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
  ['⌘/Ctrl+Shift+O', t('help.shortcut.openFolder')],
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
  const previousLocalWorkspaceId = documentStorageKind.value === 'local' ? documentWorkspaceId.value : ''
  const previousLocalDocumentId = documentStorageKind.value === 'local' ? localDocumentId.value : ''
  const previousRemoteWorkspaceId = remoteWorkspaceId.value
  const previousRemoteDocumentId = remoteDocumentId.value
  webDAVConflictOpen.value = false
  scrollSync.reset()
  editorScrollAnchors = []
  previewScrollAnchors = []
  fileName.value = document.name || t('document.untitledFilename')
  source.value = document.content || ''
  savedSource.value = source.value
  builtInDocument.value = document.builtIn === 'render-test'
    ? 'render-test'
    : document.builtIn === 'welcome' || document.welcome
      ? 'welcome'
      : null
  documentStorageKind.value = document.storageKind === 'webdav'
    ? 'webdav'
    : builtInDocument.value
      ? 'builtin'
      : 'local'
  localDocumentPath.value = documentStorageKind.value === 'local' ? document.path || '' : ''
  documentLocation.value = document.displayLocation
    || (documentStorageKind.value === 'local' ? localDocumentPath.value : '')
  const inferredLocalWorkspacePath = (
    documentStorageKind.value === 'local'
      && Boolean(document.localDocumentId)
      && workspace.value?.provider === 'local'
      ? localWorkspaceRelativePath(workspace.value.path, localDocumentPath.value)
      : ''
  )
  currentWorkspacePath.value = document.workspacePath || inferredLocalWorkspacePath
  localDocumentId.value = documentStorageKind.value === 'local' ? document.localDocumentId || '' : ''
  remoteDocumentId.value = documentStorageKind.value === 'webdav' ? document.remoteDocumentId || '' : ''
  remoteWorkspaceId.value = documentStorageKind.value === 'webdav'
    ? document.workspaceId || (workspace.value?.provider === 'webdav' ? workspace.value.id : '')
    : ''
  documentWorkspaceId.value = document.workspaceId
    || (inferredLocalWorkspacePath && workspace.value?.provider === 'local' ? workspace.value.id : '')
    || remoteWorkspaceId.value
  remoteDocumentETag.value = documentStorageKind.value === 'webdav' ? document.etag || '' : ''
  renderState.value = t(status)
  if (
    previousLocalWorkspaceId
    && previousLocalDocumentId
    && (
      previousLocalWorkspaceId !== documentWorkspaceId.value
      || previousLocalDocumentId !== localDocumentId.value
    )
  ) {
    void CloseWorkspaceDocument(previousLocalWorkspaceId, previousLocalDocumentId).catch(() => {})
  }
  if (
    previousRemoteWorkspaceId
    && previousRemoteDocumentId
    && previousRemoteDocumentId !== remoteDocumentId.value
  ) {
    void CloseWebDAVDocument(previousRemoteWorkspaceId, previousRemoteDocumentId).catch(() => {})
  }
  if (
    previousRemoteWorkspaceId
    && previousRemoteWorkspaceId !== remoteWorkspaceId.value
    && workspace.value?.id !== previousRemoteWorkspaceId
  ) {
    void CloseWebDAVWorkspace(previousRemoteWorkspaceId).catch(() => {})
  }
  void nextTick(async () => {
    if (editor.value) editor.value.scrollTop = 0
    if (previewPane.value) previewPane.value.scrollTop = 0
    await renderNow()
  })
}

function detachCurrentLocalWorkspaceDocument(workspaceId: string) {
  if (
    documentStorageKind.value !== 'local'
    || documentWorkspaceId.value !== workspaceId
    || !currentWorkspacePath.value
  ) return false
  const retainedSource = source.value
  setDocument({
    path: '',
    name: t('document.untitledFilename'),
    content: retainedSource,
    welcome: false,
  }, 'workspace.closedCurrentPreserved')
  // Force Save As even when the retained editor happened to match the last
  // saved bytes. The capability root is about to close, so its old absolute
  // path must never become an implicit fallback save authority.
  savedSource.value = `${retainedSource}\u0000`
  return true
}

function setWorkspace(value: unknown) {
  const nextWorkspace = normalizeWorkspace(value)
  if (!nextWorkspace) return false
  const previousWorkspace = workspace.value
  const workspaceChanged = Boolean(previousWorkspace && previousWorkspace.id !== nextWorkspace.id)
  const detachedCurrentDocument = Boolean(
    workspaceChanged
    && previousWorkspace?.provider === 'local'
    && detachCurrentLocalWorkspaceDocument(previousWorkspace.id),
  )
  const mutationCleanup = workspaceChanged
    ? cancelActiveWorkspaceMutation()
    : Promise.resolve(true)
  if (workspaceChanged && activeDialog.value === 'workspace') activeDialog.value = null
  workspace.value = nextWorkspace
  if (nextWorkspace.provider === 'local' && documentStorageKind.value === 'local') {
    const relativeDocumentPath = localWorkspaceRelativePath(nextWorkspace.path, localDocumentPath.value)
    if (relativeDocumentPath) {
      documentWorkspaceId.value = nextWorkspace.id
      currentWorkspacePath.value = relativeDocumentPath
    }
  }
  workspaceChildren.value = { [workspaceRootKey]: nextWorkspace.entries }
  expandedWorkspaceDirectories.value = new Set()
  loadingWorkspaceDirectories.value = new Set()
  truncatedWorkspaceDirectories.value = new Set()
  workspaceRefreshQueued = false
  renderState.value = detachedCurrentDocument
    ? t('workspace.closedCurrentPreserved')
    : t('workspace.opened', { name: nextWorkspace.name })
  if (previousWorkspace && workspaceChanged) {
    if (previousWorkspace.provider === 'webdav') {
      if (remoteWorkspaceId.value !== previousWorkspace.id) {
        void mutationCleanup.finally(() => CloseWebDAVWorkspace(previousWorkspace.id).catch(() => {}))
      }
    } else {
      void mutationCleanup.finally(() => CloseWorkspace(previousWorkspace.id).catch(() => {}))
    }
  }
  void nextTick(scheduleLayoutReconciliation)
  return true
}

function closeWorkspace() {
  const activeWorkspace = workspace.value
  const detachedCurrentDocument = Boolean(
    activeWorkspace?.provider === 'local'
    && detachCurrentLocalWorkspaceDocument(activeWorkspace.id),
  )
  const mutationCleanup = cancelActiveWorkspaceMutation()
  if (activeDialog.value === 'workspace') activeDialog.value = null
  workspace.value = null
  workspaceChildren.value = {}
  expandedWorkspaceDirectories.value = new Set()
  loadingWorkspaceDirectories.value = new Set()
  truncatedWorkspaceDirectories.value = new Set()
  workspaceRefreshQueued = false
  if (activeWorkspace?.provider === 'webdav') {
    if (remoteWorkspaceId.value !== activeWorkspace.id) {
      void mutationCleanup.finally(() => CloseWebDAVWorkspace(activeWorkspace.id).catch(() => {}))
    }
  } else if (activeWorkspace?.id) {
    void mutationCleanup.finally(() => CloseWorkspace(activeWorkspace.id).catch(() => {}))
  }
  if (detachedCurrentDocument) renderState.value = t('workspace.closedCurrentPreserved')
  void nextTick(scheduleLayoutReconciliation)
}

async function readActiveWorkspaceDirectory(activeWorkspace: WorkspaceData, relativePath: string) {
  if (activeWorkspace.provider === 'webdav') {
    return ListWebDAVDirectory(activeWorkspace.id, relativePath)
  }
  return ReadWorkspaceDirectory(activeWorkspace.id, relativePath)
}

function workspaceOpenMustWait() {
  return documentTransitionInProgress
    || Boolean(unsavedTransition.value)
    || workspaceRefreshing.value
    || workspaceMutationPreparing.value
    || workspaceMutationCancelling.value
    || Boolean(activeWorkspaceMutation.value)
}

function deferWorkspaceOpen(request: PendingWorkspaceOpenRequest) {
  if (!pendingWorkspaceOpenRequest) pendingWorkspaceOpenRequest = request
  renderState.value = t('workspace.openDeferred')
}

async function drainPendingWorkspaceOpen() {
  if (workspaceOpenMustWait() || busy.value || !pendingWorkspaceOpenRequest) return
  const request = pendingWorkspaceOpenRequest
  pendingWorkspaceOpenRequest = null
  if (request.kind === 'picker') await openDirectory()
  else await openRecentDirectory(request.id)
}

function drainWorkspaceRefreshQueue() {
  if (
    !workspaceRefreshQueued
    || workspaceRefreshing.value
    || loadingWorkspaceDirectories.value.size
    || !workspace.value
  ) return
  workspaceRefreshQueued = false
  void refreshWorkspace({ silent: true })
}

async function refreshWorkspace({ silent = false }: { silent?: boolean } = {}) {
  const activeWorkspace = workspace.value
  if (!activeWorkspace) return false
  if (workspaceRefreshing.value || loadingWorkspaceDirectories.value.size) {
    workspaceRefreshQueued = true
    if (!silent) renderState.value = t('workspace.refreshQueued')
    return false
  }

  workspaceRefreshing.value = true
  if (!silent) renderState.value = t('workspace.refreshing')
  const previouslyLoaded = loadedWorkspaceDirectoryKeys(workspaceChildren.value)
  const nextChildren: Record<string, readonly WorkspaceEntryData[]> = {}
  const nextTruncated = new Set<string>()
  const failedDirectories = new Set<string>()
  let partial = false
  try {
    const rootResult = normalizeWorkspaceDirectory(
      await readActiveWorkspaceDirectory(activeWorkspace, workspaceBackendDirectoryPath(workspaceRootKey)),
    )
    if (workspace.value?.id !== activeWorkspace.id) return false
    nextChildren[workspaceRootKey] = rootResult.entries

    for (const directoryPath of previouslyLoaded) {
      if (workspace.value?.id !== activeWorkspace.id) return false
      if (!workspaceDirectoryExists(nextChildren, directoryPath)) continue
      try {
        const directoryResult = normalizeWorkspaceDirectory(
          await readActiveWorkspaceDirectory(
            activeWorkspace,
            workspaceBackendDirectoryPath(directoryPath),
          ),
        )
        if (workspace.value?.id !== activeWorkspace.id) return false
        nextChildren[directoryPath] = directoryResult.entries
        if (directoryResult.truncated) nextTruncated.add(directoryPath)
      } catch {
        // A folder may disappear between refreshing its parent and reading it.
        // Keep the refreshed parent, collapse the stale branch, and let a later
        // refresh retry if the directory still exists.
        partial = true
        failedDirectories.add(directoryPath)
      }
    }

    if (workspace.value?.id !== activeWorkspace.id) return false
    const nextExpanded = new Set(retainExistingWorkspaceDirectories(
      expandedWorkspaceDirectories.value,
      nextChildren,
    ))
    failedDirectories.forEach((path) => nextExpanded.delete(path))
    workspace.value = {
      ...activeWorkspace,
      entries: rootResult.entries,
      truncated: rootResult.truncated,
    }
    workspaceChildren.value = nextChildren
    expandedWorkspaceDirectories.value = nextExpanded
    truncatedWorkspaceDirectories.value = nextTruncated
    if (!silent) {
      renderState.value = partial ? t('workspace.refreshPartial') : t('workspace.refreshed')
    }
    void nextTick(scheduleLayoutReconciliation)
    return true
  } catch (error) {
    if (!silent && workspace.value?.id === activeWorkspace.id) showError(error)
    return false
  } finally {
    workspaceRefreshing.value = false
    drainWorkspaceRefreshQueue()
    void drainPendingWorkspaceOpen()
  }
}

function requestUnsavedDecision(transition: DocumentTransition): Promise<UnsavedDecision> {
  if (resolveUnsavedDecision) return Promise.resolve('cancel')
  activeDialog.value = null
  unsavedTransition.value = transition
  resolvingUnsavedPrompt.value = false
  return new Promise((resolve) => {
    resolveUnsavedDecision = resolve
  })
}

function answerUnsavedPrompt(decision: UnsavedDecision) {
  if (resolvingUnsavedPrompt.value || !resolveUnsavedDecision) return
  resolvingUnsavedPrompt.value = true
  const resolve = resolveUnsavedDecision
  resolveUnsavedDecision = null
  unsavedTransition.value = null
  resolve(decision)
  void nextTick(() => { resolvingUnsavedPrompt.value = false })
}

async function performDocumentTransition(
  transition: DocumentTransition,
  action: () => Promise<void> | void,
) {
  if (documentTransitionInProgress) return false
  documentTransitionInProgress = true
  try {
    return await runGuardedDocumentTransition({
      dirty: dirty.value,
      requestDecision: () => requestUnsavedDecision(transition),
      save: saveDocument,
      transition: action,
    })
  } finally {
    documentTransitionInProgress = false
    void drainSystemDocuments()
    void drainPendingWorkspaceOpen()
  }
}

async function newDocument() {
  await performDocumentTransition('new', () => {
    setDocument({ path: '', name: t('document.untitledFilename'), content: '', welcome: false }, 'document.created')
    void nextTick(() => editor.value?.focus())
  })
}

async function bindLocalDocumentToActiveWorkspace(document: DocumentData) {
  const activeWorkspace = workspace.value
  if (
    !document?.name
    || document.storageKind === 'webdav'
    || document.builtIn
    || !document.path
    || activeWorkspace?.provider !== 'local'
  ) return document
  const relativePath = localWorkspaceRelativePath(activeWorkspace.path, document.path)
  if (!relativePath) return document
  const bound = await OpenWorkspaceFile(activeWorkspace.id, relativePath) as DocumentData
  if (
    !bound?.localDocumentId
    || bound.workspaceId !== activeWorkspace.id
    || workspaceDirectoryKey(bound.workspacePath || '') !== workspaceDirectoryKey(relativePath)
  ) throw new Error(t('workspace.invalidResponse'))
  return bound
}

async function openDocument() {
  await performDocumentTransition('open', async () => {
    try {
      const selected = await OpenFile() as DocumentData
      const document = await bindLocalDocumentToActiveWorkspace(selected)
      if (document?.name) setDocument(document)
    } catch (error) {
      showError(error)
    }
  })
}

function clearWebDAVConnectionForm(clearEndpoint = true) {
  const cleared = clearedWebDAVConnectionForm(clearEndpoint ? '' : webDAVBaseURL.value)
  webDAVBaseURL.value = cleared.endpoint
  webDAVConnectionName.value = cleared.name
  webDAVUsername.value = cleared.username
  webDAVPassword.value = cleared.password
  showWebDAVPassword.value = cleared.showPassword
  webDAVConnectionFormMode.value = cleared.mode
  webDAVEditingConnectionID.value = cleared.editingConnectionID
  webDAVStoreCredentials.value = cleared.storeCredentials
  webDAVRemoveCredentials.value = cleared.removeCredentials
  webDAVConnectionError.value = cleared.error
}

function clearWebDAVConnectionManager() {
  savedConnectionsLoadGeneration += 1
  clearWebDAVConnectionForm()
  webDAVDialogView.value = 'saved'
  savedWebDAVConnections.value = []
  savedWebDAVConnectionsLoading.value = false
  savedWebDAVConnectionsError.value = ''
  webDAVConnectionManagerBusy.value = false
  webDAVConnectionOperation.value = 'idle'
  webDAVConnectingConnectionID.value = ''
  webDAVDeleteCandidate.value = null
  webDAVDeleteReturnFocus = null
}

function clearImageInsertForm() {
  selectedImage.value = null
  imageAltText.value = ''
  publicImageURL.value = ''
  imageInsertError.value = ''
}

function cancelActiveWorkspaceMutation({ reportFailure = true } = {}) {
  workspaceMutationBeginGeneration += 1
  if (pendingWorkspaceMutationCancel) return pendingWorkspaceMutationCancel
  const task = (async () => {
    const pendingBegin = pendingWorkspaceMutationBegin
    let mutation = activeWorkspaceMutation.value
    activeWorkspaceMutation.value = null
    if (pendingBegin) await pendingBegin.catch(() => null)
    if (!mutation && activeWorkspaceMutation.value) {
      mutation = activeWorkspaceMutation.value
      activeWorkspaceMutation.value = null
    }
    if (!mutation) {
      void drainPendingWorkspaceOpen()
      return true
    }

    const cancellation = ++workspaceMutationCancelGeneration
    workspaceMutationCancelling.value = true
    try {
      await CancelWebDAVMutation(mutation.workspaceId, mutation.mutationId)
      return true
    } catch {
      if (reportFailure && !quitting) renderState.value = t('workspace.webDAVMutationCancelFailed')
      return false
    } finally {
      if (cancellation === workspaceMutationCancelGeneration) workspaceMutationCancelling.value = false
      void drainPendingWorkspaceOpen()
    }
  })()
  pendingWorkspaceMutationCancel = task
  void task.finally(() => {
    if (pendingWorkspaceMutationCancel === task) pendingWorkspaceMutationCancel = null
  })
  return task
}

function clearWorkspaceDialog() {
  workspacePreviewGeneration += 1
  workspacePreviewImages.release()
  workspaceDialogEntry.value = null
  workspaceDialogParentPath.value = ''
  workspaceEntryName.value = ''
  workspaceMutationError.value = ''
  workspaceMutationNeedsRestart.value = false
  workspaceMutationBusy.value = false
  workspacePreviewImage.value = null
  workspacePreviewSource.value = ''
  workspaceDeleteBufferPreserved.value = false
}

function dismissActiveDialog() {
  if (activeDialog.value === 'webdav' && webDAVDialogBusy.value) return
  if (activeDialog.value === 'webdav' && webDAVDeleteCandidate.value) {
    cancelDeleteSavedWebDAVConnection()
    return
  }
  if (activeDialog.value === 'image' && imageInsertBusy.value) return
  if (
    activeDialog.value === 'workspace'
    && workspaceMutationBusy.value
    && workspaceDialogView.value !== 'image'
  ) return
  activeDialog.value = null
}

function activeDialogFocusScope() {
  if (activeDialog.value === 'webdav' && webDAVDeleteCandidate.value) {
    return appDialog.value?.querySelector<HTMLElement>('.saved-connection-delete') || appDialog.value
  }
  return appDialog.value
}

function activeDialogFocusableElements() {
  const scope = activeDialogFocusScope()
  if (!scope) return []
  return Array.from(scope.querySelectorAll<HTMLElement>([
    'button:not([disabled])',
    'input:not([disabled])',
    'select:not([disabled])',
    'textarea:not([disabled])',
    'a[href]',
    '[contenteditable="true"]',
    '[tabindex]:not([tabindex="-1"])',
  ].join(','))).filter((element) => element.getAttribute('aria-hidden') !== 'true'
    && !element.matches(':disabled')
    && !element.closest('[inert]'))
}

function focusActiveDialog() {
  const scope = activeDialogFocusScope()
  if (!scope) return
  const preferred = scope.querySelector<HTMLElement>('[data-dialog-initial]:not([disabled])')
  const target = preferred || activeDialogFocusableElements()[0] || scope
  if (target === scope && !scope.hasAttribute('tabindex')) scope.tabIndex = -1
  target.focus({ preventScroll: true })
}

function trapActiveDialogFocus(event: KeyboardEvent) {
  if (event.key !== 'Tab' || !activeDialog.value) return false
  const focusable = activeDialogFocusableElements()
  if (!focusable.length) {
    event.preventDefault()
    focusActiveDialog()
    return true
  }
  const currentIndex = focusable.indexOf(document.activeElement as HTMLElement)
  const nextIndex = event.shiftKey
    ? (currentIndex <= 0 ? focusable.length - 1 : currentIndex - 1)
    : (currentIndex < 0 || currentIndex === focusable.length - 1 ? 0 : currentIndex + 1)
  if (currentIndex < 0
    || (!event.shiftKey && currentIndex === focusable.length - 1)
    || (event.shiftKey && currentIndex === 0)) {
    event.preventDefault()
    focusable[nextIndex].focus()
    return true
  }
  return false
}

function handleActiveDialogBackdrop() {
  if (activeDialog.value === 'webdav' && webDAVDeleteCandidate.value) {
    cancelDeleteSavedWebDAVConnection()
    return
  }
  dismissActiveDialog()
}

function dismissWebDAVConflict() {
  if (webDAVConflictBusy.value) return
  webDAVConflictOpen.value = false
}

function showWebDAVConnectionDialog(endpoint = '', errorMessage = '') {
  if (busy.value || webDAVDialogBusy.value) return
  clearWebDAVConnectionForm()
  webDAVDeleteCandidate.value = null
  webDAVBaseURL.value = endpoint.trim()
  webDAVConnectionError.value = errorMessage
  webDAVDialogView.value = endpoint.trim() || errorMessage ? 'temporary' : 'saved'
  activeDialog.value = 'webdav'
  void loadSavedWebDAVConnections()
}

async function loadSavedWebDAVConnections() {
  const generation = ++savedConnectionsLoadGeneration
  savedWebDAVConnectionsLoading.value = true
  savedWebDAVConnectionsError.value = ''
  try {
    const connections = normalizeSavedWebDAVConnections(await ListSavedWebDAVConnections())
    if (generation !== savedConnectionsLoadGeneration || activeDialog.value !== 'webdav') return
    savedWebDAVConnections.value = connections
  } catch (error) {
    if (generation !== savedConnectionsLoadGeneration || activeDialog.value !== 'webdav') return
    savedWebDAVConnectionsError.value = localizedErrorMessage(error)
  } finally {
    if (generation === savedConnectionsLoadGeneration) savedWebDAVConnectionsLoading.value = false
  }
}

function showSavedWebDAVConnections() {
  clearWebDAVConnectionForm()
  webDAVDeleteCandidate.value = null
  webDAVDialogView.value = 'saved'
}

function showTemporaryWebDAVForm(endpoint = '', username = '', errorMessage = '') {
  clearWebDAVConnectionForm()
  webDAVDeleteCandidate.value = null
  webDAVConnectionFormMode.value = 'connect'
  webDAVDialogView.value = 'temporary'
  webDAVBaseURL.value = endpoint.trim()
  webDAVUsername.value = username.trim()
  webDAVConnectionError.value = errorMessage
}

function showNewSavedWebDAVForm() {
  if (webDAVDialogBusy.value) return
  clearWebDAVConnectionForm()
  webDAVDeleteCandidate.value = null
  webDAVConnectionFormMode.value = 'new'
  webDAVDialogView.value = 'new'
}

function showEditSavedWebDAVForm(connection: SavedWebDAVConnection) {
  if (webDAVDialogBusy.value) return
  clearWebDAVConnectionForm()
  webDAVDeleteCandidate.value = null
  webDAVConnectionFormMode.value = 'edit'
  webDAVDialogView.value = 'edit'
  webDAVEditingConnectionID.value = connection.id
  webDAVConnectionName.value = connection.name
  webDAVBaseURL.value = connection.endpoint
  webDAVUsername.value = connection.username
}

function cancelSavedWebDAVForm() {
  if (webDAVDialogBusy.value) return
  showSavedWebDAVConnections()
}

function showImageDialog() {
  if (busy.value || imageInsertBusy.value) return
  clearImageInsertForm()
  if (documentStorageKind.value === 'webdav' && remoteWorkspaceId.value && remoteDocumentId.value) {
    imageInsertMode.value = 'webdav'
  } else if (documentStorageKind.value === 'local' && localDocumentPath.value) {
    imageInsertMode.value = 'local'
  } else {
    imageInsertMode.value = 'data'
  }
  activeDialog.value = 'image'
}

function currentImageRenderContext(): ImageRenderContext {
  return {
    storageKind: documentStorageKind.value,
    localDocumentPath: localDocumentPath.value,
    remoteWorkspaceId: remoteWorkspaceId.value,
    remoteDocumentId: remoteDocumentId.value,
  }
}

function sameImageRenderContext(left: ImageRenderContext, right: ImageRenderContext) {
  return left.storageKind === right.storageKind
    && left.localDocumentPath === right.localDocumentPath
    && left.remoteWorkspaceId === right.remoteWorkspaceId
    && left.remoteDocumentId === right.remoteDocumentId
}

async function selectImageForInsertion() {
  if (imageInsertBusy.value || imageInsertMode.value === 'public') return
  try {
    imageInsertBusy.value = true
    imageInsertError.value = ''
    const asset = await SelectImageFile() as ImageAssetData
    if (!asset?.dataBase64) return
    imageDataURI(asset.mimeType, asset.dataBase64)
    selectedImage.value = asset
    if (!imageAltText.value.trim()) imageAltText.value = defaultImageAlt(asset.name)
  } catch (error) {
    imageInsertError.value = localizedErrorMessage(error)
  } finally {
    imageInsertBusy.value = false
  }
}

function insertMarkdownImage(markdownURL: string, fallbackName = '') {
  const target = editor.value
  if (!target) throw new Error(t('error.previewUnavailable'))
  const start = target.selectionStart
  const end = target.selectionEnd
  const alt = imageAltText.value.trim() || defaultImageAlt(fallbackName) || t('image.defaultAlt')
  const markdownImage = buildMarkdownImage(alt, markdownURL)
  target.setRangeText(markdownImage, start, end, 'end')
  source.value = target.value
  nextTick(() => {
    target.focus()
    target.setSelectionRange(start + markdownImage.length, start + markdownImage.length)
  })
}

async function insertSelectedImage() {
  if (!canInsertImage.value) return
  const context = currentImageRenderContext()
  try {
    imageInsertBusy.value = true
    busy.value = true
    imageInsertError.value = ''
    let markdownURL = ''
    let fallbackName = ''
    if (imageInsertMode.value === 'public') {
      const parsed = new URL(publicImageURL.value.trim())
      if (classifyImageSource(parsed.href, 'builtin') !== 'public-https') {
        throw new Error(t('image.publicHTTPSRequired'))
      }
      markdownURL = parsed.href
      fallbackName = parsed.pathname.split('/').pop() || ''
    } else {
      const asset = selectedImage.value
      if (!asset?.dataBase64) return
      fallbackName = asset.name
      if (imageInsertMode.value === 'data') {
        markdownURL = imageDataURI(asset.mimeType, asset.dataBase64)
      } else {
        let imported: ImageAsset
        if (imageInsertMode.value === 'local') {
          if (context.storageKind !== 'local' || !context.localDocumentPath) {
            throw new Error(t('image.saveDocumentFirst'))
          }
          imported = await ImportLocalImageData(
            context.localDocumentPath,
            asset.name,
            asset.mimeType,
            asset.dataBase64,
          ) as ImageAsset
        } else {
          if (context.storageKind !== 'webdav' || !context.remoteWorkspaceId || !context.remoteDocumentId) {
            throw new Error(t('image.openWebDAVDocumentFirst'))
          }
          imported = await ImportWebDAVImageData(
            context.remoteWorkspaceId,
            context.remoteDocumentId,
            asset.name,
            asset.mimeType,
            asset.dataBase64,
          ) as ImageAsset
        }
        markdownURL = imported?.markdownURL?.trim() || ''
        fallbackName = imported?.name || fallbackName
        if (!markdownURL) throw new Error(t('image.invalidResponse'))
      }
    }
    if (!sameImageRenderContext(context, currentImageRenderContext())) {
      throw new Error(t('image.documentChanged'))
    }
    insertMarkdownImage(markdownURL, fallbackName)
    activeDialog.value = null
    renderState.value = t('image.inserted')
  } catch (error) {
    imageInsertError.value = localizedErrorMessage(error)
    renderState.value = t('image.insertFailed', { message: imageInsertError.value })
  } finally {
    busy.value = false
    imageInsertBusy.value = false
  }
}

async function openRecentWebDAV(id: string) {
  if (busy.value || webDAVConnecting.value) return
  let endpoint = ''
  let fallbackError = ''
  try {
    busy.value = true
    const connection = await OpenRecentWebDAV(id) as RecentWebDAVConnectionData
    endpoint = connection?.endpoint?.trim() || ''
    if (!endpoint) throw new Error(t('webdav.invalidResponse'))
    const recentResult = await resolveRecentWebDAVOpen(connection, async (connectionID) => {
      const openedWorkspace = await ConnectSavedWebDAV(connectionID)
      if (!setWorkspace(openedWorkspace)) throw new Error(t('webdav.invalidResponse'))
    })
    if (recentResult.kind === 'connected') {
      renderState.value = t('webdav.connected', { name: workspace.value?.name || connection.name || 'WebDAV' })
      return
    }
    if (recentResult.error) {
      fallbackError = t('webdav.savedConnectFailed', { message: localizedErrorMessage(recentResult.error) })
    }
  } catch (error) {
    showError(error)
  } finally {
    busy.value = false
  }
  if (endpoint) showWebDAVConnectionDialog(endpoint, fallbackError)
}

async function connectSavedWebDAV(connection: SavedWebDAVConnection) {
  if (busy.value || webDAVDialogBusy.value) return
  try {
    busy.value = true
    webDAVConnectionManagerBusy.value = true
    webDAVConnectionOperation.value = 'connecting-saved'
    webDAVConnectingConnectionID.value = connection.id
    webDAVConnectionError.value = ''
    savedWebDAVConnectionsError.value = ''
    renderState.value = t('webdav.connectingSaved', { name: connection.name })
    const openedWorkspace = await ConnectSavedWebDAV(connection.id)
    if (!setWorkspace(openedWorkspace)) throw new Error(t('webdav.invalidResponse'))
    renderState.value = t('webdav.connected', { name: workspace.value?.name || connection.name })
    activeDialog.value = null
  } catch (error) {
    const message = localizedErrorMessage(error)
    showTemporaryWebDAVForm(connection.endpoint, connection.username, t('webdav.savedConnectFailed', { message }))
    renderState.value = t('webdav.connectionFailed', { message })
  } finally {
    webDAVPassword.value = ''
    webDAVConnectingConnectionID.value = ''
    webDAVConnectionOperation.value = 'idle'
    webDAVConnectionManagerBusy.value = false
    busy.value = false
  }
}

function savedWebDAVFormErrorMessage(error: unknown) {
  if (error instanceof SavedWebDAVFormError) {
    if (error.code === 'name_required') return t('webdav.connectionNameRequired')
    if (error.code === 'endpoint_required') return t('webdav.endpointRequired')
    if (error.code === 'origin_requires_password') return t('webdav.endpointChangeNeedsPassword')
    return t('webdav.usernameChangeNeedsPassword')
  }
  return localizedErrorMessage(error)
}

async function saveWebDAVConnection() {
  if (busy.value || webDAVDialogBusy.value || webDAVConnectionFormMode.value === 'connect') return
  const existing = editingSavedWebDAVConnection.value
  try {
    const input = buildSavedWebDAVConnectionInput({
      mode: webDAVConnectionFormMode.value,
      id: webDAVEditingConnectionID.value,
      name: webDAVConnectionName.value,
      endpoint: webDAVBaseURL.value,
      username: webDAVUsername.value,
      password: webDAVPassword.value,
      storeCredentials: webDAVStoreCredentials.value,
      removeCredentials: webDAVRemoveCredentials.value,
      existing,
    })
    busy.value = true
    webDAVConnectionManagerBusy.value = true
    webDAVConnectionOperation.value = 'saving'
    webDAVConnectionError.value = ''
    await SaveWebDAVConnection(input)
    webDAVPassword.value = ''
    renderState.value = t(webDAVConnectionFormMode.value === 'new'
      ? 'webdav.connectionSaved'
      : 'webdav.connectionUpdated')
    showSavedWebDAVConnections()
    await loadSavedWebDAVConnections()
  } catch (error) {
    webDAVConnectionError.value = savedWebDAVFormErrorMessage(error)
  } finally {
    webDAVPassword.value = ''
    webDAVConnectionOperation.value = 'idle'
    webDAVConnectionManagerBusy.value = false
    busy.value = false
  }
}

function requestDeleteSavedWebDAVConnection(connection: SavedWebDAVConnection) {
  if (webDAVDialogBusy.value) return
  savedWebDAVConnectionsError.value = ''
  webDAVDeleteReturnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
  webDAVDeleteCandidate.value = connection
  void nextTick(() => focusActiveDialog())
}

function cancelDeleteSavedWebDAVConnection() {
  if (webDAVConnectionManagerBusy.value) return
  const returnFocus = webDAVDeleteReturnFocus
  webDAVDeleteReturnFocus = null
  webDAVDeleteCandidate.value = null
  savedWebDAVConnectionsError.value = ''
  void nextTick(() => {
    if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true })
    else focusActiveDialog()
  })
}

async function confirmDeleteSavedWebDAVConnection() {
  const candidate = webDAVDeleteCandidate.value
  if (!candidate || busy.value || webDAVDialogBusy.value) return
  try {
    busy.value = true
    webDAVConnectionManagerBusy.value = true
    webDAVConnectionOperation.value = 'deleting'
    savedWebDAVConnectionsError.value = ''
    await DeleteSavedWebDAVConnection(candidate.id)
    webDAVDeleteCandidate.value = null
    webDAVDeleteReturnFocus = null
    if (webDAVEditingConnectionID.value === candidate.id) showSavedWebDAVConnections()
    renderState.value = t('webdav.connectionDeleted', { name: candidate.name })
    await loadSavedWebDAVConnections()
  } catch (error) {
    savedWebDAVConnectionsError.value = localizedErrorMessage(error)
  } finally {
    webDAVConnectionOperation.value = 'idle'
    webDAVConnectionManagerBusy.value = false
    busy.value = false
    if (!webDAVDeleteCandidate.value) void nextTick(() => focusActiveDialog())
  }
}

function submitWebDAVDialog() {
  if (webDAVDialogView.value === 'saved') return
  if (webDAVConnectionFormMode.value === 'connect') void connectWebDAV()
  else void saveWebDAVConnection()
}

async function connectWebDAV() {
  if (busy.value || webDAVConnecting.value) return
  const endpoint = webDAVBaseURL.value.trim()
  const username = webDAVUsername.value.trim()
  const password = webDAVPassword.value
  if (!endpoint) {
    webDAVConnectionError.value = t('webdav.endpointRequired')
    return
  }

  webDAVConnecting.value = true
  busy.value = true
  webDAVConnectionError.value = ''
  renderState.value = t('webdav.connecting')
  try {
    const openedWorkspace = await ConnectWebDAV({ endpoint, username, password })
    if (!setWorkspace(openedWorkspace)) throw new Error(t('webdav.invalidResponse'))
    renderState.value = t('webdav.connected', { name: workspace.value?.name || 'WebDAV' })
    activeDialog.value = null
  } catch (error) {
    const message = localizedErrorMessage(error)
    webDAVConnectionError.value = message
    renderState.value = t('webdav.connectionFailed', { message })
  } finally {
    webDAVPassword.value = ''
    webDAVConnecting.value = false
    busy.value = false
  }
}

async function openDirectory() {
  if (workspaceOpenMustWait() || busy.value) {
    deferWorkspaceOpen({ kind: 'picker' })
    return
  }
  try {
    busy.value = true
    const openedWorkspace = await OpenDirectory()
    setWorkspace(openedWorkspace)
  } catch (error) {
    showError(error)
  } finally {
    busy.value = false
  }
}

async function openRecentDirectory(id: string) {
  if (workspaceOpenMustWait() || busy.value) {
    deferWorkspaceOpen({ kind: 'recent-directory', id })
    return
  }
  try {
    busy.value = true
    setWorkspace(await OpenRecentDirectory(id))
  } catch (error) {
    showError(error)
  } finally {
    busy.value = false
    void drainPendingWorkspaceOpen()
  }
}

async function toggleWorkspaceDirectory(entry: WorkspaceEntryData) {
  const activeWorkspace = workspace.value
  if (!activeWorkspace || entry.kind !== 'directory') return
  const directoryKey = workspaceDirectoryKey(entry.path)
  if (loadingWorkspaceDirectories.value.has(directoryKey)) return
  const expanded = new Set(expandedWorkspaceDirectories.value)
  if (expanded.has(directoryKey)) {
    expanded.delete(directoryKey)
    expandedWorkspaceDirectories.value = expanded
    return
  }

  expanded.add(directoryKey)
  expandedWorkspaceDirectories.value = expanded
  if (Object.hasOwn(workspaceChildren.value, directoryKey)) return

  const loading = new Set(loadingWorkspaceDirectories.value)
  loading.add(directoryKey)
  loadingWorkspaceDirectories.value = loading
  try {
    const result = normalizeWorkspaceDirectory(
      await readActiveWorkspaceDirectory(activeWorkspace, entry.path),
    )
    if (workspace.value?.id !== activeWorkspace.id) return
    workspaceChildren.value = {
      ...workspaceChildren.value,
      [directoryKey]: result.entries,
    }
    if (result.truncated) {
      const truncated = new Set(truncatedWorkspaceDirectories.value)
      truncated.add(directoryKey)
      truncatedWorkspaceDirectories.value = truncated
    }
  } catch (error) {
    if (workspace.value?.id === activeWorkspace.id) {
      const nextExpanded = new Set(expandedWorkspaceDirectories.value)
      nextExpanded.delete(directoryKey)
      expandedWorkspaceDirectories.value = nextExpanded
      showError(error)
    }
  } finally {
    const nextLoading = new Set(loadingWorkspaceDirectories.value)
    nextLoading.delete(directoryKey)
    loadingWorkspaceDirectories.value = nextLoading
    drainWorkspaceRefreshQueue()
  }
}

async function openWorkspaceDocument(entry: WorkspaceEntryData) {
  const activeWorkspace = workspace.value
  if (!activeWorkspace || entry.kind !== 'markdown') return
  const currentProvider = documentStorageKind.value === 'webdav' ? 'webdav' : 'local'
  if (isActiveWorkspaceFile(
    activeWorkspace.provider,
    activeWorkspace.id,
    entry.path,
    currentProvider,
    documentWorkspaceId.value,
    currentWorkspacePath.value,
  )) return
  await performDocumentTransition('open', async () => {
    try {
      busy.value = true
      const document = activeWorkspace.provider === 'webdav'
        ? await OpenWebDAVFile(activeWorkspace.id, entry.path) as DocumentData
        : await OpenWorkspaceFile(activeWorkspace.id, entry.path) as DocumentData
      if (document?.name) setDocument(document)
    } catch (error) {
      showError(error)
    } finally {
      busy.value = false
    }
  })
}

function showWorkspaceCreateMarkdown(parentPath: string) {
  if (busy.value || workspaceRefreshing.value || workspaceMutationCancelling.value) return
  workspaceDialogView.value = 'create-markdown'
  workspaceDialogEntry.value = null
  workspaceDialogParentPath.value = workspaceDirectoryKey(parentPath)
  workspaceEntryName.value = t('workspace.defaultMarkdownName')
  workspaceMutationError.value = ''
  activeDialog.value = 'workspace'
}

function showWorkspaceCreateDirectory(parentPath: string) {
  if (busy.value || workspaceRefreshing.value || workspaceMutationCancelling.value) return
  workspaceDialogView.value = 'create-directory'
  workspaceDialogEntry.value = null
  workspaceDialogParentPath.value = workspaceDirectoryKey(parentPath)
  workspaceEntryName.value = t('workspace.defaultDirectoryName')
  workspaceMutationError.value = ''
  activeDialog.value = 'workspace'
}

function populateWorkspaceMutationDialog(view: 'rename' | 'delete', entry: WorkspaceEntryData) {
  workspaceDialogView.value = view
  workspaceDialogEntry.value = entry
  workspaceDialogParentPath.value = workspaceParentPath(entry.path)
  workspaceEntryName.value = entry.name
  workspaceMutationError.value = ''
  workspaceMutationNeedsRestart.value = false
  workspaceDeleteBufferPreserved.value = false
  activeDialog.value = 'workspace'
}

function beginWebDAVWorkspaceMutation(
  activeWorkspace: WorkspaceData,
  entry: WorkspaceEntryData,
  operation: WorkspaceMutationOperation,
) {
  const generation = ++workspaceMutationBeginGeneration
  workspaceMutationPreparing.value = true
  busy.value = true
  renderState.value = t('workspace.webDAVMutationPreparing')

  const task = (async (): Promise<WorkspaceEntryData | null> => {
    let mutationId = ''
    let installed = false
    try {
      const result = await BeginWebDAVMutation(
        activeWorkspace.id,
        entry.path,
        entry.kind,
        entry.revision,
        operation,
      ) as WebDAVMutationData
      mutationId = typeof result?.mutationId === 'string' ? result.mutationId.trim() : ''
      const lockedEntry = normalizeWorkspaceEntries([result?.entry])[0]
      const expiresAt = typeof result?.expiresAt === 'string' ? result.expiresAt.trim() : ''
      if (
        !mutationId
        || !expiresAt
        || !lockedEntry
        || workspaceDirectoryKey(lockedEntry.path) !== workspaceDirectoryKey(entry.path)
        || lockedEntry.kind !== entry.kind
        || (lockedEntry.kind !== 'directory' && !lockedEntry.revision)
      ) throw new Error(t('workspace.invalidResponse'))

      if (
        generation !== workspaceMutationBeginGeneration
        || workspace.value?.id !== activeWorkspace.id
      ) {
        return null
      }
      activeWorkspaceMutation.value = {
        workspaceId: activeWorkspace.id,
        mutationId,
        operation,
        expiresAt,
      }
      installed = true
      return lockedEntry
    } catch (error) {
      if (generation === workspaceMutationBeginGeneration) {
        renderState.value = t('workspace.operationFailed', {
          message: localizedWebDAVMutationError(error),
        })
      }
      return null
    } finally {
      if (mutationId && !installed) {
        await CancelWebDAVMutation(activeWorkspace.id, mutationId).catch(() => undefined)
      }
      workspaceMutationPreparing.value = false
      busy.value = false
    }
  })()
  pendingWorkspaceMutationBegin = task
  void task.finally(() => {
    if (pendingWorkspaceMutationBegin === task) pendingWorkspaceMutationBegin = null
  })
  return task
}

async function showWorkspaceRename(entry: WorkspaceEntryData) {
  if (busy.value || workspaceRefreshing.value || workspaceMutationCancelling.value) return
  const activeWorkspace = workspace.value
  if (!activeWorkspace) return
  let confirmedEntry = entry
  if (activeWorkspace.provider === 'webdav') {
    const lockedEntry = await beginWebDAVWorkspaceMutation(activeWorkspace, entry, 'rename')
    if (!lockedEntry) return
    confirmedEntry = lockedEntry
  } else if (!entry.revision) {
    renderState.value = t('workspace.revisionExpired')
    return
  }
  populateWorkspaceMutationDialog('rename', confirmedEntry)
}

async function showWorkspaceDelete(entry: WorkspaceEntryData) {
  if (busy.value || workspaceRefreshing.value || workspaceMutationCancelling.value) return
  const activeWorkspace = workspace.value
  if (!activeWorkspace) return
  let confirmedEntry = entry
  if (activeWorkspace.provider === 'webdav') {
    const lockedEntry = await beginWebDAVWorkspaceMutation(activeWorkspace, entry, 'delete')
    if (!lockedEntry) return
    confirmedEntry = lockedEntry
  } else if (!entry.revision) {
    renderState.value = t('workspace.revisionExpired')
    return
  }
  populateWorkspaceMutationDialog('delete', confirmedEntry)
}

async function showWorkspaceImage(entry: WorkspaceEntryData) {
  const activeWorkspace = workspace.value
  if (
    !activeWorkspace
    || entry.kind !== 'image'
    || busy.value
    || workspaceRefreshing.value
    || workspaceMutationCancelling.value
  ) return
  const generation = ++workspacePreviewGeneration
  workspaceDialogView.value = 'image'
  workspaceDialogEntry.value = entry
  workspaceDialogParentPath.value = workspaceParentPath(entry.path)
  workspaceEntryName.value = entry.name
  workspacePreviewImage.value = null
  workspacePreviewSource.value = ''
  workspaceMutationError.value = ''
  workspaceMutationBusy.value = true
  activeDialog.value = 'workspace'
  try {
    const image = activeWorkspace.provider === 'webdav'
      ? await ReadWebDAVWorkspaceImage(activeWorkspace.id, entry.path) as ImageAssetData
      : await ReadWorkspaceImage(activeWorkspace.id, entry.path) as ImageAssetData
    if (
      generation !== workspacePreviewGeneration
      || activeDialog.value !== 'workspace'
      || workspaceDialogView.value !== 'image'
      || workspace.value?.id !== activeWorkspace.id
    ) return
    workspacePreviewImages.release()
    const prepared = workspacePreviewImages.add(
      'workspace-preview',
      '',
      activeWorkspace.provider === 'webdav' ? 'webdav-relative' : 'local-relative',
      entry.path,
      image,
    )
    workspacePreviewImage.value = image
    workspacePreviewSource.value = prepared.previewSource
  } catch (error) {
    if (generation === workspacePreviewGeneration && activeDialog.value === 'workspace') {
      workspaceMutationError.value = localizedWorkspaceErrorMessage(error)
    }
  } finally {
    if (generation === workspacePreviewGeneration) workspaceMutationBusy.value = false
  }
}

function normalizeWorkspaceRequestedName(view: WorkspaceDialogView, entry: WorkspaceEntryData | null) {
  let name = workspaceEntryName.value.trim()
  if (!name || name === '.' || name === '..' || /[\\/\u0000-\u001f\u007f]/u.test(name)) {
    throw new Error(t('workspace.invalidName'))
  }
  const markdownName = view === 'create-markdown' || (view === 'rename' && entry?.kind === 'markdown')
  if (markdownName) {
    if (!name.includes('.')) name += '.md'
    if (!/\.(?:md|markdown)$/iu.test(name)) throw new Error(t('workspace.markdownExtensionRequired'))
  }
  return name
}

async function createWorkspaceMarkdown(activeWorkspace: WorkspaceData, relativePath: string) {
  return activeWorkspace.provider === 'webdav'
    ? await CreateWebDAVMarkdownFile(activeWorkspace.id, relativePath) as DocumentData
    : await CreateWorkspaceMarkdownFile(activeWorkspace.id, relativePath) as DocumentData
}

async function createWorkspaceDirectory(activeWorkspace: WorkspaceData, relativePath: string) {
  if (activeWorkspace.provider === 'webdav') {
    await CreateWebDAVDirectory(activeWorkspace.id, relativePath)
  } else {
    await CreateWorkspaceDirectory(activeWorkspace.id, relativePath)
  }
}

async function renameWorkspaceEntry(
  activeWorkspace: WorkspaceData,
  entry: WorkspaceEntryData,
  destinationPath: string,
) {
  if (activeWorkspace.provider === 'webdav') {
    const mutation = activeWorkspaceMutation.value
    if (
      !mutation
      || mutation.workspaceId !== activeWorkspace.id
      || mutation.operation !== 'rename'
    ) throw new Error(t('workspace.webDAVMutationExpired'))
    // Commit consumes the opaque capability even when the server rejects the
    // MOVE, so a failed attempt must be prepared again instead of reusing it.
    activeWorkspaceMutation.value = null
    const result = await CommitWebDAVRename(
      activeWorkspace.id,
      mutation.mutationId,
      destinationPath,
    )
    const renamedEntry = normalizeWorkspaceEntries([result])[0]
    if (
      !renamedEntry
      || renamedEntry.kind !== entry.kind
      || workspaceDirectoryKey(renamedEntry.path) !== workspaceDirectoryKey(destinationPath)
    ) throw new Error(t('workspace.invalidResponse'))
    return renamedEntry
  } else {
    const result = await RenameWorkspaceEntry(
      activeWorkspace.id,
      entry.path,
      destinationPath,
      entry.kind,
      entry.revision,
    )
    const renamedEntry = normalizeWorkspaceEntries([result])[0]
    if (
      !renamedEntry
      || !renamedEntry.revision
      || renamedEntry.kind !== entry.kind
      || workspaceDirectoryKey(renamedEntry.path) !== workspaceDirectoryKey(destinationPath)
    ) throw new Error(t('workspace.invalidResponse'))
    return renamedEntry
  }
}

async function deleteWorkspaceEntry(activeWorkspace: WorkspaceData, entry: WorkspaceEntryData) {
  const recursive = entry.kind === 'directory'
  if (activeWorkspace.provider === 'webdav') {
    const mutation = activeWorkspaceMutation.value
    if (
      !mutation
      || mutation.workspaceId !== activeWorkspace.id
      || mutation.operation !== 'delete'
    ) throw new Error(t('workspace.webDAVMutationExpired'))
    // The backend removes the prepared mutation before attempting DELETE and
    // always releases its lock afterward. Never expose a consumed token again.
    activeWorkspaceMutation.value = null
    await CommitWebDAVDelete(activeWorkspace.id, mutation.mutationId, recursive)
  } else {
    await DeleteWorkspaceEntry(activeWorkspace.id, entry.path, recursive, entry.revision)
  }
}

function workspaceDocumentDisplayLocation(activeWorkspace: WorkspaceData, relativePath: string) {
  if (activeWorkspace.provider === 'local') {
    return localWorkspaceAbsolutePath(activeWorkspace.path, relativePath)
  }
  try {
    const base = activeWorkspace.path.endsWith('/') ? activeWorkspace.path : `${activeWorkspace.path}/`
    const encodedPath = workspaceDirectoryKey(relativePath)
      .split('/')
      .map((segment) => encodeURIComponent(segment))
      .join('/')
    return new URL(encodedPath, base).toString()
  } catch {
    return `${activeWorkspace.path.replace(/\/+$/u, '')}/${workspaceDirectoryKey(relativePath)}`
  }
}

function updateCurrentDocumentAfterWorkspaceRename(
  activeWorkspace: WorkspaceData,
  sourcePath: string,
  destinationPath: string,
) {
  if (
    documentWorkspaceId.value !== activeWorkspace.id
    || !workspacePathIsWithin(currentWorkspacePath.value, sourcePath)
  ) return
  const nextPath = rebaseWorkspacePath(currentWorkspacePath.value, sourcePath, destinationPath)
  currentWorkspacePath.value = nextPath
  fileName.value = nextPath.split('/').pop() || fileName.value
  documentLocation.value = workspaceDocumentDisplayLocation(activeWorkspace, nextPath)
  if (activeWorkspace.provider === 'local') {
    localDocumentPath.value = localWorkspaceAbsolutePath(activeWorkspace.path, nextPath)
  }
}

function preserveCurrentDocumentAfterWorkspaceDelete(activeWorkspace: WorkspaceData, deletedPath: string) {
  if (
    documentWorkspaceId.value !== activeWorkspace.id
    || !workspacePathIsWithin(currentWorkspacePath.value, deletedPath)
  ) return
  const retainedSource = source.value
  setDocument({
    path: '',
    name: t('document.untitledFilename'),
    content: retainedSource,
    welcome: false,
  }, 'workspace.deletedCurrentPreserved')
  // The deleted backing file can never be treated as the saved version. Keep
  // even an empty retained buffer dirty so quit/open still invokes the guard.
  savedSource.value = `${retainedSource}\u0000`
}

async function submitWorkspaceNameDialog() {
  const activeWorkspace = workspace.value
  const view = workspaceDialogView.value
  const entry = workspaceDialogEntry.value
  if (
    !activeWorkspace
    || workspaceMutationBusy.value
    || workspaceMutationNeedsRestart.value
    || !['create-markdown', 'create-directory', 'rename'].includes(view)
  ) return
  try {
    const name = normalizeWorkspaceRequestedName(view, entry)
    const destinationPath = workspaceJoinPath(workspaceDialogParentPath.value, name)
    workspaceMutationError.value = ''

    if (view === 'create-markdown') {
      activeDialog.value = null
      await performDocumentTransition('open', async () => {
        workspaceMutationBusy.value = true
        busy.value = true
        try {
          const document = await createWorkspaceMarkdown(activeWorkspace, destinationPath)
          if (!document?.name) throw new Error(t('workspace.invalidResponse'))
          setDocument(document, 'workspace.markdownCreated')
          await refreshWorkspace({ silent: true })
        } catch (error) {
          await refreshWorkspace({ silent: true })
          renderState.value = t('workspace.operationFailed', {
            message: localizedWorkspaceErrorMessage(error),
          })
        } finally {
          busy.value = false
          workspaceMutationBusy.value = false
        }
      })
      return
    }

    workspaceMutationBusy.value = true
    busy.value = true
    if (view === 'create-directory') {
      await createWorkspaceDirectory(activeWorkspace, destinationPath)
      renderState.value = t('workspace.directoryCreated', { name })
    } else if (entry) {
      if (destinationPath === workspaceDirectoryKey(entry.path)) {
        activeDialog.value = null
        return
      }
      const renamedEntry = await renameWorkspaceEntry(activeWorkspace, entry, destinationPath)
      updateCurrentDocumentAfterWorkspaceRename(activeWorkspace, entry.path, renamedEntry.path)
      renderState.value = t('workspace.renamed', { name })
    }
    activeDialog.value = null
    await refreshWorkspace({ silent: true })
  } catch (error) {
    if (
      activeWorkspace.provider === 'webdav'
      && view === 'rename'
      && !activeWorkspaceMutation.value
    ) {
      workspaceMutationNeedsRestart.value = true
      workspaceMutationError.value = localizedWebDAVMutationError(error)
      await refreshWorkspace({ silent: true })
    } else {
      workspaceMutationError.value = localizedWorkspaceErrorMessage(error)
    }
  } finally {
    busy.value = false
    workspaceMutationBusy.value = false
  }
}

async function confirmDeleteWorkspaceEntry() {
  const activeWorkspace = workspace.value
  const entry = workspaceDialogEntry.value
  if (
    !activeWorkspace
    || !entry
    || workspaceMutationBusy.value
    || workspaceMutationNeedsRestart.value
    || workspaceDialogView.value !== 'delete'
  ) return
  const deletionCouldAffectCurrentDocument = documentWorkspaceId.value === activeWorkspace.id
    && workspacePathIsWithin(currentWorkspacePath.value, entry.path)
  workspaceMutationBusy.value = true
  busy.value = true
  workspaceMutationError.value = ''
  if (deletionCouldAffectCurrentDocument) {
    workspaceDeleteBufferPreserved.value = true
    preserveCurrentDocumentAfterWorkspaceDelete(activeWorkspace, entry.path)
  }
  try {
    await deleteWorkspaceEntry(activeWorkspace, entry)
    renderState.value = deletionCouldAffectCurrentDocument
      ? t('workspace.deletedCurrentPreserved')
      : t('workspace.deleted', { name: entry.name })
    activeDialog.value = null
    await refreshWorkspace({ silent: true })
  } catch (error) {
    // Recursive deletion can fail after removing only part of a tree. Once the
    // target covered the open document, never assume its backing file survived:
    // preserve the buffer as an explicitly dirty untitled document.
    if (activeWorkspace.provider === 'webdav') {
      workspaceMutationNeedsRestart.value = true
      workspaceMutationError.value = localizedWebDAVMutationError(error)
    } else {
      workspaceMutationError.value = localizedWorkspaceErrorMessage(error)
    }
    await refreshWorkspace({ silent: true })
  } finally {
    busy.value = false
    workspaceMutationBusy.value = false
  }
}

async function openRecentItem(value: unknown) {
  if (!value || typeof value !== 'object') return
  const candidate = value as Partial<RecentOpenEventData>
  if (!candidate.id || !candidate.kind) return

  if (candidate.kind === 'webdav') {
    await openRecentWebDAV(candidate.id)
    return
  }
  if (candidate.kind === 'directory' || candidate.kind === 'folder') {
    await openRecentDirectory(candidate.id)
    return
  }
  if (busy.value) return
  if (candidate.kind !== 'file') return
  const recentID = candidate.id

  await performDocumentTransition('open', async () => {
    try {
      busy.value = true
      const selected = await OpenRecentFile(recentID) as DocumentData
      const document = await bindLocalDocumentToActiveWorkspace(selected)
      if (document?.name) setDocument(document)
    } catch (error) {
      showError(error)
    } finally {
      busy.value = false
    }
  })
}

async function clearRecentItems() {
  try {
    await ClearRecentItems()
    renderState.value = t('workspace.recentCleared')
  } catch (error) {
    showError(error)
  }
}

async function saveDocument(): Promise<boolean> {
  try {
    busy.value = true
    if (documentStorageKind.value === 'webdav') {
      if (!remoteWorkspaceId.value || !remoteDocumentId.value) {
        throw new Error(t('webdav.notFound'))
      }
      renderState.value = t('webdav.savingRemotely')
      const remoteResult = await SaveWebDAVFile(
        remoteWorkspaceId.value,
        remoteDocumentId.value,
        source.value,
        remoteDocumentETag.value,
      ) as WebDAVSaveResultData
      if (remoteResult?.conflict) {
        renderState.value = t('webdav.conflictTitle')
        webDAVConflictOpen.value = true
        return false
      }
      remoteDocumentETag.value = remoteResult?.etag || remoteDocumentETag.value
      savedSource.value = source.value
      renderState.value = t('webdav.savedRemotely', { name: fileName.value })
      if (workspace.value?.id === remoteWorkspaceId.value) void refreshWorkspace({ silent: true })
      return true
    }

    const localWorkspaceCandidate = workspace.value
    const activeLocalWorkspace = isWorkspaceBackedLocalDocument(
      localWorkspaceCandidate,
      documentStorageKind.value,
      documentWorkspaceId.value,
      currentWorkspacePath.value,
    )
      ? localWorkspaceCandidate
      : null
    if (activeLocalWorkspace) {
      if (!localDocumentId.value) {
        renderState.value = t('workspace.revisionExpired')
        return false
      }
      try {
        const result = await SaveWorkspaceMarkdownFile(
          activeLocalWorkspace.id,
          localDocumentId.value,
          source.value,
        )
        if (!result?.path) throw new Error(t('workspace.invalidResponse'))
        localDocumentPath.value = result.path
        documentLocation.value = result.path
        fileName.value = result.name || fileName.value
        savedSource.value = source.value
        builtInDocument.value = null
        renderState.value = t('document.savedLocally')
        void refreshWorkspace({ silent: true })
        return true
      } catch (error) {
        renderState.value = t('workspace.operationFailed', {
          message: localizedWorkspaceErrorMessage(error),
        })
        return false
      }
    }

    const previousLocalPath = localDocumentPath.value
    const expectedSource = source.value
    const result = await SaveFile(previousLocalPath, expectedSource)
    if (result?.path) {
      const activeLocalWorkspace = workspace.value?.provider === 'local' ? workspace.value : null
      const activeWorkspacePath = activeLocalWorkspace
        ? localWorkspaceRelativePath(activeLocalWorkspace.path, result.path)
        : ''
      if (activeLocalWorkspace && activeWorkspacePath) {
        const adopted = await OpenWorkspaceFile(activeLocalWorkspace.id, activeWorkspacePath) as DocumentData
        if (
          !adopted?.localDocumentId
          || adopted.workspaceId !== activeLocalWorkspace.id
          || workspaceDirectoryKey(adopted.workspacePath || '') !== workspaceDirectoryKey(activeWorkspacePath)
          || adopted.content !== expectedSource
        ) throw new Error(t('workspace.invalidResponse'))
        localDocumentPath.value = adopted.path
        documentLocation.value = adopted.displayLocation || adopted.path
        documentWorkspaceId.value = activeLocalWorkspace.id
        currentWorkspacePath.value = activeWorkspacePath
        localDocumentId.value = adopted.localDocumentId
        fileName.value = adopted.name || result.name
      } else {
        localDocumentPath.value = result.path
        documentLocation.value = result.path
        documentWorkspaceId.value = ''
        currentWorkspacePath.value = ''
        localDocumentId.value = ''
        fileName.value = result.name
      }
      documentStorageKind.value = 'local'
      savedSource.value = expectedSource
      builtInDocument.value = null
      renderState.value = t('document.savedLocally')
      if (workspace.value) void refreshWorkspace({ silent: true })
      return true
    }
    return false
  } catch (error) {
    showError(error)
    return false
  } finally {
    busy.value = false
  }
}

async function overwriteConflictingWebDAVDocument() {
  if (webDAVConflictBusy.value || !remoteWorkspaceId.value || !remoteDocumentId.value) return
  webDAVConflictBusy.value = true
  busy.value = true
  try {
    const result = await OverwriteWebDAVFile(
      remoteWorkspaceId.value,
      remoteDocumentId.value,
      source.value,
    ) as WebDAVSaveResultData
    if (result?.conflict) throw new Error(t('webdav.conflictMessage', { name: fileName.value }))
    remoteDocumentETag.value = result?.etag || ''
    savedSource.value = source.value
    webDAVConflictOpen.value = false
    renderState.value = t('webdav.savedRemotely', { name: fileName.value })
    if (workspace.value?.id === remoteWorkspaceId.value) void refreshWorkspace({ silent: true })
  } catch (error) {
    showError(error)
  } finally {
    busy.value = false
    webDAVConflictBusy.value = false
  }
}

async function reloadConflictingWebDAVDocument() {
  if (webDAVConflictBusy.value || !remoteWorkspaceId.value || !currentWorkspacePath.value) return
  webDAVConflictBusy.value = true
  busy.value = true
  try {
    const document = await OpenWebDAVFile(remoteWorkspaceId.value, currentWorkspacePath.value) as DocumentData
    if (!document?.name) throw new Error(t('webdav.invalidResponse'))
    webDAVConflictOpen.value = false
    setDocument(document)
  } catch (error) {
    showError(error)
  } finally {
    busy.value = false
    webDAVConflictBusy.value = false
  }
}

async function saveDocumentAs() {
  try {
    busy.value = true
    const expectedSource = source.value
    const previousLocalWorkspaceId = documentStorageKind.value === 'local' ? documentWorkspaceId.value : ''
    const previousLocalDocumentId = documentStorageKind.value === 'local' ? localDocumentId.value : ''
    const previousRemoteWorkspaceId = remoteWorkspaceId.value
    const previousRemoteDocumentId = remoteDocumentId.value
    const saveAsHint = documentStorageKind.value === 'webdav' ? fileName.value : localDocumentPath.value
    const result = await SaveFileAs(saveAsHint, expectedSource)
    if (result?.path) {
      const activeLocalWorkspace = workspace.value?.provider === 'local' ? workspace.value : null
      const activeWorkspacePath = activeLocalWorkspace
        ? localWorkspaceRelativePath(activeLocalWorkspace.path, result.path)
        : ''
      if (activeLocalWorkspace && activeWorkspacePath) {
        const adopted = await OpenWorkspaceFile(activeLocalWorkspace.id, activeWorkspacePath) as DocumentData
        if (
          !adopted?.localDocumentId
          || adopted.workspaceId !== activeLocalWorkspace.id
          || workspaceDirectoryKey(adopted.workspacePath || '') !== workspaceDirectoryKey(activeWorkspacePath)
          || adopted.content !== expectedSource
        ) throw new Error(t('workspace.invalidResponse'))
        setDocument(adopted, 'document.savedAsLocally')
        void refreshWorkspace({ silent: true })
        return
      }
      localDocumentPath.value = result.path
      documentLocation.value = result.path
      documentWorkspaceId.value = ''
      currentWorkspacePath.value = ''
      localDocumentId.value = ''
      documentStorageKind.value = 'local'
      remoteDocumentId.value = ''
      remoteWorkspaceId.value = ''
      remoteDocumentETag.value = ''
      fileName.value = result.name
      savedSource.value = source.value
      builtInDocument.value = null
      renderState.value = t('document.savedAsLocally')
      if (previousLocalWorkspaceId && previousLocalDocumentId) {
        void CloseWorkspaceDocument(previousLocalWorkspaceId, previousLocalDocumentId).catch(() => {})
      }
      if (previousRemoteWorkspaceId && previousRemoteDocumentId) {
        void CloseWebDAVDocument(previousRemoteWorkspaceId, previousRemoteDocumentId).catch(() => {})
      }
      if (previousRemoteWorkspaceId && workspace.value?.id !== previousRemoteWorkspaceId) {
        void CloseWebDAVWorkspace(previousRemoteWorkspaceId).catch(() => {})
      }
      if (workspace.value) void refreshWorkspace({ silent: true })
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
    path: documentStorageKind.value === 'local' ? localDocumentPath.value : '',
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
        articleHTML: exportArticleHTML(target, format),
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
    || (documentStorageKind.value === 'local' ? localDocumentPath.value : '') !== snapshot.path
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
  materializeExportImages(clone, 'capture')
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

function requestApplicationQuit() {
  if (!quitting) Quit()
}

async function handleCloseRequest() {
  try {
    await cancelActiveWorkspaceMutation({ reportFailure: false })
    if (activeDialog.value === 'workspace') activeDialog.value = null
    const completed = await performDocumentTransition('quit', async () => {
      if (pendingUpdateInstall) {
        updateState.value = 'installing'
        await LaunchUpdateInstaller()
      }
      quitting = true
      await ConfirmQuit()
    })
    if (!completed) {
      await CancelQuitRequest()
      if (pendingUpdateInstall) {
        pendingUpdateInstall = false
        updateState.value = 'ready'
        activeDialog.value = 'about'
      }
    }
  } catch (error) {
    quitting = false
    pendingUpdateInstall = false
    if (updateDownload.value?.ready) updateState.value = 'ready'
    await CancelQuitRequest().catch(() => undefined)
    showError(error)
    activeDialog.value = 'about'
  }
}

async function showWelcome() {
  await performDocumentTransition('open', async () => {
    try {
      const document = await LoadWelcomeDocument(locale.value) as DocumentData
      setDocument(document, 'status.ready')
    } catch (error) {
      showError(error)
    }
  })
}

async function showRenderingTest() {
  await performDocumentTransition('open', async () => {
    try {
      const document = await LoadRenderingTestDocument() as DocumentData
      setDocument(document, 'status.ready')
    } catch (error) {
      showError(error)
    }
  })
}

async function loadApplicationInfo() {
  try {
    const info = await GetAppInfo() as ApplicationInfoData
    applicationInfo.value = {
      version: info?.version?.trim() || '',
      author: info?.author?.trim() || '',
    }
  } catch (error) {
    console.error('Unable to load application information', error)
  }
}

function showAboutDialog() {
  aboutView.value = 'overview'
  activeDialog.value = 'about'
  if (!applicationInfo.value.version) void loadApplicationInfo()
  if (updateState.value === 'idle') void checkForUpdates(false)
}

async function showThirdPartyLicenses() {
  aboutView.value = 'third-party'
  await nextTick(() => focusActiveDialog())
  if (thirdPartyNoticesState.value === 'ready' || thirdPartyNoticesState.value === 'loading') return
  thirdPartyNoticesState.value = 'loading'
  try {
    const notices = await GetThirdPartyNotices()
    if (!notices.trim()) throw new Error('The embedded third-party notice is empty')
    thirdPartyNotices.value = notices
    thirdPartyNoticesState.value = 'ready'
  } catch (error) {
    console.error('Unable to load third-party licenses', error)
    thirdPartyNotices.value = ''
    thirdPartyNoticesState.value = 'error'
  }
}

function showAboutOverview() {
  aboutView.value = 'overview'
  void nextTick(() => focusActiveDialog())
}

async function checkForUpdates(showDialog = true) {
  if (['checking', 'downloading', 'cancelling', 'installing'].includes(updateState.value)) return
  if (showDialog) {
    aboutView.value = 'overview'
    activeDialog.value = 'about'
    if (!applicationInfo.value.version) void loadApplicationInfo()
  }
  updateState.value = 'checking'
  updateError.value = ''
  try {
    const result = await CheckForUpdates() as UpdateInfoData
    updateInfo.value = {
      currentVersion: result?.currentVersion?.trim() || applicationInfo.value.version,
      latestVersion: result?.latestVersion?.trim() || '',
      updateAvailable: Boolean(result?.updateAvailable),
      releaseURL: result?.releaseURL?.trim() || '',
      downloadURL: result?.downloadURL?.trim() || '',
      publishedAt: result?.publishedAt?.trim() || '',
      assetName: result?.assetName?.trim() || '',
      assetSize: Number(result?.assetSize) || 0,
      installable: Boolean(result?.installable),
      checksumAvailable: Boolean(result?.checksumAvailable),
      installerKind: result?.installerKind?.trim() || '',
    }
    updateDownload.value = null
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
    updateError.value = errorMessage(error)
    updateState.value = 'error'
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
  if (updateState.value === 'downloading' || updateState.value === 'cancelling' || updateState.value === 'installing') return
  if (updateState.value === 'ready') {
    pendingUpdateInstall = true
    activeDialog.value = null
    requestApplicationQuit()
    return
  }
  if (updateState.value !== 'available') await checkForUpdates()
  if (updateState.value !== 'available' || !updateInfo.value) return
  if (!updateInfo.value.installable || !updateInfo.value.checksumAvailable) {
    await openUpdatePage()
    return
  }
  updateState.value = 'downloading'
  updateDownload.value = null
  updateError.value = ''
  updateDownloadCancelled = false
  const downloadSession = updateDownloadSessions.begin()
  try {
    const result = await DownloadUpdate(downloadSession) as UpdateDownloadData
    if (updateDownloadCancelled || !updateDownloadSessions.isActive(downloadSession)) {
      updateDownloadSessions.finish(downloadSession)
      updateState.value = 'available'
      return
    }
    updateDownload.value = normalizeUpdateDownload(result)
    updateDownloadSessions.finish(downloadSession)
    updateState.value = 'ready'
    pendingUpdateInstall = true
    activeDialog.value = null
    requestApplicationQuit()
  } catch (error) {
    if (updateDownloadCancelled || !updateDownloadSessions.isActive(downloadSession)) {
      updateDownloadSessions.finish(downloadSession)
      updateState.value = 'available'
      return
    }
    updateDownloadSessions.finish(downloadSession)
    updateError.value = errorMessage(error)
    updateState.value = 'error'
  }
}

async function cancelUpdateDownload() {
  if (updateState.value !== 'downloading') return
  updateDownloadCancelled = true
  updateState.value = 'cancelling'
  await CancelUpdateDownload()
}

function handleUpdateProgress(payload: UpdateDownloadData) {
  if (updateState.value !== 'downloading' || !updateDownloadSessions.isActive(payload?.sessionID || '')) return
  updateDownload.value = normalizeUpdateDownload(payload)
}

function normalizeUpdateDownload(payload: UpdateDownloadData): UpdateDownloadData {
  const progress = Math.max(0, Math.min(1, Number(payload?.progress) || 0))
  return {
    sessionID: payload?.sessionID || '',
    assetName: payload?.assetName?.trim() || updateInfo.value?.assetName || '',
    version: payload?.version?.trim() || updateInfo.value?.latestVersion || '',
    bytesDownloaded: Math.max(0, Number(payload?.bytesDownloaded) || 0),
    totalBytes: Math.max(0, Number(payload?.totalBytes) || 0),
    progress,
    ready: Boolean(payload?.ready),
  }
}

async function toggleFullscreen() {
  if (await WindowIsFullscreen()) WindowUnfullscreen()
  else WindowFullscreen()
}

async function runEditAction(action: string) {
  const activeElement = document.activeElement
  const focusLeftDocument = !activeElement
    || activeElement === document.body
    || activeElement === document.documentElement
  const modalRoot = document.querySelector<HTMLElement>('[aria-modal="true"]')
  const target = resolveTextEditControl(
    activeElement,
    focusLeftDocument ? lastFocusedElement : null,
    modalRoot ? null : editor.value,
  )
  if (!target) return
  if (modalRoot && !modalRoot.contains(target)) return
  const sourceEditorTarget = target === editor.value
  if (sourceEditorTarget) beginScroll('editor')
  target.focus()
  const targetIsAvailable = () => isTextEditControl(target)
    && (!modalRoot || (modalRoot.isConnected && modalRoot.contains(target)))
  const targetStillOwnsEditIntent = () => {
    const current = document.activeElement
    const focusAtDocumentBoundary = !current
      || current === document.body
      || current === document.documentElement
    return targetIsAvailable()
      && (current === target || (focusAtDocumentBoundary && lastFocusedElement === target))
  }
  try {
    if (action === 'select-all') {
      target.setSelectionRange(0, target.value.length)
    } else if (action === 'copy' || action === 'cut') {
      const start = target.selectionStart ?? 0
      const end = target.selectionEnd ?? start
      if (end <= start) return
      await navigator.clipboard.writeText(target.value.slice(start, end))
      if (action === 'cut' && targetStillOwnsEditIntent()) {
        target.setRangeText('', start, end, 'end')
        dispatchTextEditInput(target, 'deleteByCut')
      }
    } else if (action === 'paste') {
      const text = await navigator.clipboard.readText()
      if (!targetStillOwnsEditIntent()) return
      const start = target.selectionStart ?? 0
      const end = target.selectionEnd ?? start
      target.setRangeText(text, start, end, 'end')
      const inputData = target instanceof HTMLInputElement && target.type === 'password' ? null : text
      dispatchTextEditInput(target, 'insertFromPaste', inputData)
    } else if (action === 'undo' || action === 'redo') {
      document.execCommand(action)
      dispatchTextEditInput(target, action === 'undo' ? 'historyUndo' : 'historyRedo')
    }
  } catch {
    renderState.value = t('edit.clipboardFailed')
  }
}

function rememberFocusedElement(event: FocusEvent) {
  if (event.target instanceof Element) lastFocusedElement = event.target
}

function updateNativeMenu() {
  void UpdateMenuState(locale.value, viewMode.value, theme.value, syncScroll.value, previewFirst.value)
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
  if (replaceWelcome && builtInDocument.value === 'welcome' && !dirty.value) {
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

async function handleSystemDocument(document: DocumentData) {
  if (!document?.name) return
  pendingSystemDocuments.push(document)
  await drainSystemDocuments()
}

async function drainSystemDocuments() {
  if (drainingSystemDocuments || documentTransitionInProgress || !pendingSystemDocuments.length) return
  drainingSystemDocuments = true
  try {
    while (pendingSystemDocuments.length) {
      const document = pendingSystemDocuments.shift()
      if (!document) continue
      await performDocumentTransition('open', async () => {
        const boundDocument = await bindLocalDocumentToActiveWorkspace(document)
        setDocument(boundDocument)
        if (!document.activationId) return
        try {
          await ActivateRecentDocument(document.activationId)
        } catch (error) {
          showError(error)
        }
      })
    }
  } finally {
    drainingSystemDocuments = false
  }
}

function handleMenuAction(action: string) {
  if (activeDialog.value) {
    if (['undo', 'redo', 'cut', 'copy', 'paste', 'select-all'].includes(action)) void runEditAction(action)
    return
  }
  if (action === 'open-folder') {
    void openDirectory()
    return
  }
  if (busy.value) return
  if (action === 'new') void newDocument()
  else if (action === 'open') void openDocument()
  else if (action === 'connect-webdav') showWebDAVConnectionDialog()
  else if (action === 'insert-image') showImageDialog()
  else if (action === 'clear-recent') void clearRecentItems()
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
  else if (action === 'show-welcome') void showWelcome()
  else if (action === 'show-render-test') void showRenderingTest()
  else if (action === 'hide') Hide()
  else if (action === 'window-minimise') WindowMinimise()
  else if (action === 'window-toggle-maximise') WindowToggleMaximise()
  else if (action === 'toggle-fullscreen') void toggleFullscreen()
  else if (action === 'toggle-sync-scroll') syncScroll.value = !syncScroll.value
  else if (action === 'toggle-pane-order') swapPaneOrder()
  else if (action.startsWith('view-')) chooseView(action.slice(5) as ViewMode)
  else if (action.startsWith('theme-')) chooseTheme(action.slice(6) as Theme)
  else if (action.startsWith('format-')) runFormat(action.slice(7))
  else if (['undo', 'redo', 'cut', 'copy', 'paste', 'select-all'].includes(action)) void runEditAction(action)
  else if (action === 'copy-html') void copyHTML()
  else if (action === 'quit') requestApplicationQuit()
}

function scheduleRender() {
  window.clearTimeout(renderTimer)
  // Invalidate any Mermaid work still running for the previous document.
  // Otherwise an old asynchronous render may keep changing preview height.
  previewCommit.invalidate()
  renderState.value = t('status.waiting')
  renderTimer = window.setTimeout(renderNow, 120)
}

async function preparePreviewImages(
  root: HTMLElement,
  sequence: number,
  context: ImageRenderContext,
  resources: PreviewImageResourceSet,
) {
  const allPlaceholders = Array.from(root.querySelectorAll<HTMLElement>(
    '.inkmark-image-placeholder[data-inkmark-image-source]',
  ))
  const placeholders = allPlaceholders.slice(0, maximumPreviewImages)
  allPlaceholders.slice(maximumPreviewImages).forEach((placeholder) => {
    const source = placeholder.dataset.inkmarkImageAlt || placeholder.dataset.inkmarkImageSource || ''
    placeholder.className = 'image-resource-error'
    placeholder.setAttribute('role', 'img')
    placeholder.removeAttribute('data-inkmark-image-source')
    placeholder.textContent = t('image.loadFailed', { source })
  })
  const budget = new PreviewImageBudget()
  await forEachWithConcurrency(placeholders, maximumConcurrentImageResolvers, async (placeholder, index) => {
    if (!previewCommit.isCurrent(sequence)) return
    const originalSource = placeholder.dataset.inkmarkImageSource?.trim() || ''
    const alt = placeholder.dataset.inkmarkImageAlt || ''
    const title = placeholder.dataset.inkmarkImageTitle || ''
    const kind = classifyImageSource(originalSource, context.storageKind)
    const resourceID = `inkmark-image-${sequence}-${index}`
    try {
      let cacheKey = ''
      let loadImageAsset: () => Promise<ImageAssetData>
      if (kind === 'data') {
        const embedded = dataURIImageAsset(originalSource)
        // The complete Data URI is already the resource identity. Keeping it
        // as the Map key avoids a second large concatenated copy while making
        // identical embedded images reusable across renders.
        cacheKey = originalSource
        loadImageAsset = () => ValidateImageData(
          '',
          embedded.mimeType,
          embedded.dataBase64,
        ) as Promise<ImageAssetData>
      } else if (kind === 'local-relative' || kind === 'webdav-relative' || kind === 'public-https') {
        const contextKey = kind === 'local-relative'
          ? context.localDocumentPath
          : kind === 'webdav-relative'
            ? `${context.remoteWorkspaceId}\0${context.remoteDocumentId}`
            : ''
        cacheKey = imageResourceCacheKey(kind, contextKey, originalSource)
        loadImageAsset = async () => {
          if (kind === 'local-relative') {
            if (!context.localDocumentPath) throw new Error(t('image.relativeUnavailable'))
            return ResolveLocalImage(context.localDocumentPath, originalSource) as Promise<ImageAssetData>
          } else if (kind === 'webdav-relative') {
            if (!context.remoteWorkspaceId || !context.remoteDocumentId) {
              throw new Error(t('image.relativeUnavailable'))
            }
            return ResolveWebDAVImage(
              context.remoteWorkspaceId,
              context.remoteDocumentId,
              originalSource,
            ) as Promise<ImageAssetData>
          }
          return FetchPublicImage(originalSource) as Promise<ImageAssetData>
        }
      } else {
        throw new Error(t('image.unsupportedSource'))
      }
      budget.claimImage(cacheKey)
      // Adopted work belongs to an earlier render's budget. Pessimistically
      // reserve its maximum size in this render until its validated metadata
      // arrives, so new downloads cannot run ahead of the 64 MiB limit.
      const adoptedReservation = pendingImageAssets.has(cacheKey) && budget.beginResolve(cacheKey)
      let resolved: ImageAssetData
      try {
        resolved = await resolvePreparedImageAsset(
          activePreviewImages,
          pendingImageAssets,
          cacheKey,
          sequence,
          (revision) => previewCommit.isCurrent(revision),
          (shouldRun) => imageResolverGate.run(async () => {
            const reserved = budget.beginResolve(cacheKey)
            try {
              const asset = await loadImageAsset()
              budget.finishResolve(cacheKey, asset)
              return asset
            } catch (error) {
              if (reserved) budget.cancelResolve(cacheKey)
              throw error
            }
          }, shouldRun),
        )
      } catch (error) {
        if (adoptedReservation) budget.cancelResolve(cacheKey)
        throw error
      }
      // Active or cross-generation pending resources did not necessarily use
      // this generation's budget while loading, so account for them again in
      // the generation that is about to commit.
      if (adoptedReservation) budget.finishResolve(cacheKey, resolved)
      else budget.reserveAsset(cacheKey, resolved)
      const record: PreparedImageResource = resources.add(resourceID, cacheKey, kind, originalSource, resolved)
      if (!previewCommit.isCurrent(sequence)) return
      const image = document.createElement('img')
      image.alt = alt
      if (title) image.title = title
      image.dataset.inkmarkImageId = resourceID
      image.src = record.previewSource
      await imageDecodeGate.run(
        () => waitForImageDecode(image),
        () => previewCommit.isCurrent(sequence),
      )
      if (!previewCommit.isCurrent(sequence)) return
      placeholder.replaceWith(image)
    } catch {
      if (!previewCommit.isCurrent(sequence)) return
      resources.discard(resourceID)
      placeholder.className = 'image-resource-error'
      placeholder.setAttribute('role', 'img')
      placeholder.removeAttribute('data-inkmark-image-source')
      if (kind === 'public-https') {
        placeholder.dataset.inkmarkPublicImage = originalSource
        placeholder.dataset.inkmarkImageAlt = alt
      }
      placeholder.textContent = t('image.loadFailed', { source: alt || originalSource })
    }
  })
}

async function waitForImageDecode(image: HTMLImageElement) {
  if (image.complete && image.naturalWidth > 0) return
  const completion = typeof image.decode === 'function'
    ? image.decode()
    : new Promise<void>((resolve, reject) => {
        image.addEventListener('load', () => resolve(), { once: true })
        image.addEventListener('error', () => reject(new Error('image decode failed')), { once: true })
      })
  let timeoutID = 0
  try {
    await Promise.race([
      completion,
      new Promise<never>((_resolve, reject) => {
        timeoutID = window.setTimeout(() => reject(new Error('image decode timed out')), 8_000)
      }),
    ])
  } finally {
    window.clearTimeout(timeoutID)
  }
}

function materializeExportImages(root: HTMLElement, format: 'html' | 'doc' | 'capture') {
  root.querySelectorAll<HTMLImageElement>('img[data-inkmark-image-id]').forEach((image) => {
    const resourceID = image.dataset.inkmarkImageId || ''
    const record = activePreviewImages.records.get(resourceID)
    if (record) image.src = exportImageSource(record, format)
    delete image.dataset.inkmarkImageId
  })
  if (format === 'html') {
    root.querySelectorAll<HTMLElement>('[data-inkmark-public-image]').forEach((placeholder) => {
      const source = placeholder.dataset.inkmarkPublicImage || ''
      if (classifyImageSource(source, 'builtin') !== 'public-https') return
      const image = document.createElement('img')
      image.src = source
      image.alt = placeholder.dataset.inkmarkImageAlt || ''
      placeholder.replaceWith(image)
    })
  }
}

function exportArticleHTML(target: HTMLElement, format: 'html' | 'doc') {
  const clone = target.cloneNode(true) as HTMLElement
  materializeExportImages(clone, format)
  return clone.innerHTML
}

async function renderNow(sourceText = source.value, renderTheme = theme.value): Promise<boolean> {
  const target = preview.value
  if (!target) return false
  const sequence = previewCommit.begin()
  const nextPreviewImages = new PreviewImageResourceSet()
  const imageContext = currentImageRenderContext()
  renderState.value = t('status.rendering')
  try {
    const renderedHTML = markdown.render(sourceText)
    const cleanHTML = DOMPurify.sanitize(renderedHTML, {
      USE_PROFILES: { html: true },
      ADD_TAGS: ['details', 'summary', 'mark'],
      ADD_ATTR: [
        'target', 'rel', 'checked', 'disabled', 'data-alert', 'data-display-mode', 'data-source-line',
        'data-inkmark-image-source', 'data-inkmark-image-alt', 'data-inkmark-image-title',
      ],
      // No resource-bearing HTML reaches the detached staging DOM. Markdown
      // image tokens use inert spans above and receive a validated Blob URL
      // only after the native resolver accepts their bytes.
      FORBID_TAGS: ['script', 'style', 'iframe', 'object', 'embed', 'svg', 'img', 'picture', 'source', 'video', 'audio', 'track'],
      FORBID_ATTR: [
        'src', 'srcset', 'poster', 'background', 'style',
        'data-inkmark-public-image', 'data-inkmark-image-id',
      ],
    })
    const committed = await previewCommit.stageAndCommit(sequence, async () => {
      const staging = target.cloneNode(false) as HTMLElement
      staging.innerHTML = cleanHTML
      decoratePreview(staging)
      renderMath(staging)
      highlightCode(staging)
      await Promise.all([
        renderDiagrams(staging, sequence, renderTheme),
        preparePreviewImages(staging, sequence, imageContext, nextPreviewImages),
      ])
      return staging
    }, (staging) => {
      // All expensive and asynchronous work happens off-screen. Replacing the
      // children once keeps the old preview stable until the new one is ready.
      target.classList.remove('render-error')
      target.replaceChildren(...Array.from(staging.childNodes))
      activePreviewImages.release()
      activePreviewImages = nextPreviewImages
      refreshScrollAnchors()
      reconcileActiveScroll()
    })
    if (!committed) {
      nextPreviewImages.release()
      return false
    }
    renderState.value = t('status.rendered')
    return true
  } catch (error) {
    nextPreviewImages.release()
    if (!previewCommit.isCurrent(sequence)) return false
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
      if (href === '#inkmark-render-test') {
        event.preventDefault()
        void showRenderingTest()
        return
      }
      if (href === '#inkmark-welcome') {
        event.preventDefault()
        void showWelcome()
        return
      }
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
  const usedCacheKeys = new Set<string>()
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: 'strict',
    theme: renderTheme === 'dark' ? 'dark' : 'base',
    htmlLabels: false,
    fontFamily: '-apple-system, BlinkMacSystemFont, PingFang SC, Microsoft YaHei, sans-serif',
    flowchart: { useMaxWidth: true, htmlLabels: false },
  })
  for (let index = 0; index < diagrams.length; index += 1) {
    if (!previewCommit.isCurrent(sequence)) return
    const diagram = diagrams[index]
    const definition = diagram.textContent || ''
    const cacheKey = mermaidCacheKey(renderTheme, definition)
    try {
      // Do not insert the same cached SVG twice into one document because its
      // internal IDs may collide. Repeated renders can still reuse it safely.
      let rendered = usedCacheKeys.has(cacheKey) ? undefined : mermaidRenderCache.get(cacheKey)
      if (!rendered) {
        const id = `inkmark-diagram-${sequence}-${index}`
        rendered = await mermaid.render(id, definition)
        if (!previewCommit.isCurrent(sequence)) return
        if (!usedCacheKeys.has(cacheKey)) mermaidRenderCache.set(cacheKey, rendered)
      }
      if (!previewCommit.isCurrent(sequence)) return
      diagram.innerHTML = rendered.svg
      diagram.classList.add('mermaid-rendered')
      rendered.bindFunctions?.(diagram)
      usedCacheKeys.add(cacheKey)
    } catch {
      if (!previewCommit.isCurrent(sequence)) return
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

function swapPaneOrder() {
  if (busy.value) return
  scrollSync.reset()
  previewFirst.value = togglePreviewFirst(previewFirst.value)
  localStorage.setItem(previewFirstStorageKey, String(previewFirst.value))
  updateNativeMenu()
}

function runFormat(action: string) {
  if (busy.value) return
  if (action === 'image') {
    showImageDialog()
    return
  }
  const target = editor.value
  if (!target) return
  beginScroll('editor')
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

function measureEditorScrollAnchors(lines: readonly number[]): ScrollAnchor[] {
  const target = editor.value
  const uniqueLines = [...new Set(lines)]
    .filter((line) => Number.isFinite(line) && line >= 0)
    .sort((left, right) => left - right)
  if (!target || !uniqueLines.length || !shouldMeasureEditorAnchors(source.value.length, uniqueLines.length)) return []

  const computed = window.getComputedStyle(target)
  const mirror = document.createElement('div')
  Object.assign(mirror.style, {
    position: 'fixed',
    left: '-100000px',
    top: '0',
    visibility: 'hidden',
    pointerEvents: 'none',
    boxSizing: computed.boxSizing,
    width: `${target.clientWidth}px`,
    paddingTop: computed.paddingTop,
    paddingRight: computed.paddingRight,
    paddingBottom: computed.paddingBottom,
    paddingLeft: computed.paddingLeft,
    border: '0',
    font: computed.font,
    fontFamily: computed.fontFamily,
    fontSize: computed.fontSize,
    fontWeight: computed.fontWeight,
    lineHeight: computed.lineHeight,
    letterSpacing: computed.letterSpacing,
    tabSize: computed.tabSize,
    whiteSpace: 'pre-wrap',
    overflowWrap: 'break-word',
    wordBreak: computed.wordBreak,
    contain: 'layout style paint',
  })

  const offsets = new Map<number, number>()
  let currentLine = 0
  let currentOffset = 0
  let lineIndex = 0
  while (lineIndex < uniqueLines.length) {
    const requestedLine = uniqueLines[lineIndex]
    while (currentLine < requestedLine) {
      const nextBreak = source.value.indexOf('\n', currentOffset)
      if (nextBreak === -1) break
      currentOffset = nextBreak + 1
      currentLine += 1
    }
    if (currentLine !== requestedLine) break
    offsets.set(requestedLine, currentOffset)
    lineIndex += 1
  }

  const markers: Array<{ line: number; element: HTMLSpanElement }> = []
  let textOffset = 0
  offsets.forEach((offset, line) => {
    mirror.append(document.createTextNode(source.value.slice(textOffset, offset)))
    const marker = document.createElement('span')
    Object.assign(marker.style, {
      display: 'inline-block',
      width: '0',
      height: '1px',
      margin: '0',
      padding: '0',
      overflow: 'hidden',
      verticalAlign: 'top',
    })
    mirror.append(marker)
    markers.push({ line, element: marker })
    textOffset = offset
  })
  mirror.append(document.createTextNode(source.value.slice(textOffset) || '\u200b'))
  document.body.append(mirror)
  const anchors = markers.map(({ line, element }) => ({ line, top: element.offsetTop }))
  mirror.remove()
  return anchors
}

function measurePreviewScrollAnchors(): ScrollAnchor[] {
  const pane = previewPane.value
  const root = preview.value
  if (!pane || !root) return []
  const paneTop = pane.getBoundingClientRect().top
  const elements = root.querySelectorAll<HTMLElement>('[data-source-line]')
  return sampleAnchorIndices(elements.length)
    .map((index) => elements[index])
    .map((element) => ({
      line: Number(element.dataset.sourceLine),
      top: element.getBoundingClientRect().top - paneTop + pane.scrollTop,
    }))
    .filter((anchor) => Number.isFinite(anchor.line) && Number.isFinite(anchor.top))
}

function refreshScrollAnchors() {
  previewScrollAnchors = measurePreviewScrollAnchors()
  editorScrollAnchors = measureEditorScrollAnchors(previewScrollAnchors.map((anchor) => anchor.line))
}

function scheduleLayoutReconciliation() {
  if (layoutReconcileFrame !== null) return
  layoutReconcileFrame = window.requestAnimationFrame(() => {
    layoutReconcileFrame = null
    refreshScrollAnchors()
    reconcileActiveScroll()
  })
}

function scrollMapping(pane: ScrollPane): ScrollMapping {
  const maxLine = Math.max(0, lineCount.value - 1)
  return pane === 'editor'
    ? { sourceAnchors: editorScrollAnchors, targetAnchors: previewScrollAnchors, maxLine }
    : { sourceAnchors: previewScrollAnchors, targetAnchors: editorScrollAnchors, maxLine }
}

function reconcileActiveScroll() {
  const activePane = scrollSync.active()
  if (activePane === 'editor') syncFromEditor()
  else if (activePane === 'preview') syncFromPreview()
}

function beginScroll(pane: ScrollPane) {
  if (syncScroll.value) scrollSync.begin(pane)
}

function syncFromEditor() {
  if (!syncScroll.value || !editor.value || !previewPane.value) return
  scrollSync.sync('editor', editor.value, previewPane.value, scrollMapping('editor'))
}

function syncFromPreview() {
  if (!syncScroll.value || !editor.value || !previewPane.value) return
  scrollSync.sync('preview', previewPane.value, editor.value, scrollMapping('preview'))
}

async function copyHTML() {
  if (busy.value) return
  try {
    const target = preview.value
    await navigator.clipboard.writeText(target ? exportArticleHTML(target, 'html') : '')
    renderState.value = t('status.htmlCopied')
  } catch {
    renderState.value = t('status.copyFailed')
  }
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && unsavedTransition.value) {
    event.preventDefault()
    answerUnsavedPrompt('cancel')
    return
  }
  if (event.key === 'Escape' && webDAVConflictOpen.value) {
    event.preventDefault()
    dismissWebDAVConflict()
    return
  }
  if (event.key === 'Escape' && activeDialog.value) {
    event.preventDefault()
    if (activeDialog.value === 'webdav' && webDAVDeleteCandidate.value) {
      cancelDeleteSavedWebDAVConnection()
      return
    }
    dismissActiveDialog()
    return
  }
  if (trapActiveDialogFocus(event)) return
  if (busy.value) return
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

function localizedErrorMessage(error: unknown) {
  const message = errorMessage(error)
  const match = message.match(/^\[INKMARK_WEBDAV:([a-z_]+)\]/)
  if (!match) return message
  const translations: Partial<Record<string, TranslationKey>> = {
    authentication: 'webdav.authenticationFailed',
    permission: 'webdav.permissionDenied',
    not_found: 'webdav.notFound',
    conflict: 'webdav.conflictTitle',
    locked: 'webdav.locked',
    unsupported: 'webdav.unsupported',
    too_large: 'webdav.tooLarge',
    rate_limited: 'webdav.rateLimited',
    timeout: 'webdav.timeout',
    canceled: 'webdav.canceled',
    network: 'webdav.networkError',
    server: 'webdav.serverError',
    protocol: 'webdav.invalidResponse',
    invalid_input: 'webdav.invalidRequest',
    credential_store: 'webdav.credentialStoreUnavailable',
    local_storage: 'webdav.localConnectionStorageUnavailable',
  }
  return t(translations[match[1]] || 'webdav.operationFailed')
}

function localizedWorkspaceErrorMessage(error: unknown) {
  const message = errorMessage(error)
  if (/^\[INKMARK_WEBDAV:/u.test(message)) return localizedErrorMessage(error)
  if (/already exists|file exists|已存在|冲突/iu.test(message)) return t('workspace.entryExists')
  if (/not found|does not exist|不存在|找不到/iu.test(message)) return t('workspace.entryNotFound')
  if (/permission|access denied|权限|拒绝访问/iu.test(message)) return t('workspace.permissionDenied')
  if (/too large|exceeds|超过|过大/iu.test(message)) return t('workspace.resourceTooLarge')
  if (/invalid|not allowed|无效|不允许|只能|路径|名称/iu.test(message)) return t('workspace.invalidOperation')
  return t('workspace.operationUnavailable')
}

function localizedWebDAVMutationError(error: unknown) {
  const message = errorMessage(error)
  if (
    message === t('workspace.webDAVMutationExpired')
    || message === t('workspace.webDAVMutationUnsupported')
  ) return message
  const match = message.match(/^\[INKMARK_WEBDAV:([a-z_]+)\]/u)
  if (!match) return localizedWorkspaceErrorMessage(error)
  if (['conflict', 'locked', 'canceled', 'invalid_input'].includes(match[1])) {
    return t('workspace.webDAVMutationExpired')
  }
  if (match[1] === 'unsupported') return t('workspace.webDAVMutationUnsupported')
  if (match[1] === 'not_found') return t('workspace.entryNotFound')
  return localizedErrorMessage(error)
}

function showError(error: unknown) {
  renderState.value = localizedErrorMessage(error)
}

watch(source, () => scheduleRender())
watch(syncScroll, () => {
  scrollSync.reset()
  updateNativeMenu()
})
watch(busy, (isBusy) => {
  if (!isBusy) void drainPendingWorkspaceOpen()
})
watch(activeDialog, (nextDialog, previousDialog) => {
  if (nextDialog && !previousDialog) {
    const activeElement = document.activeElement
    dialogReturnFocus = activeElement instanceof HTMLElement
      && activeElement !== document.body
      && activeElement !== document.documentElement
      ? activeElement
      : lastFocusedElement instanceof HTMLElement
        ? lastFocusedElement
        : null
    void nextTick(() => focusActiveDialog())
  }
  if (previousDialog === 'webdav' && nextDialog !== 'webdav') {
    clearWebDAVConnectionManager()
    lastFocusedElement = null
  }
  if (previousDialog === 'image' && nextDialog !== 'image') clearImageInsertForm()
  if (previousDialog === 'workspace' && nextDialog !== 'workspace') {
    void cancelActiveWorkspaceMutation()
    clearWorkspaceDialog()
  }
  if (previousDialog === 'about' && nextDialog !== 'about') aboutView.value = 'overview'
  if (!nextDialog && previousDialog) {
    const returnFocus = dialogReturnFocus
    dialogReturnFocus = null
    void nextTick(() => {
      if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true })
      else editor.value?.focus({ preventScroll: true })
    })
  }
})
watch(webDAVDialogView, () => {
  if (activeDialog.value === 'webdav') void nextTick(() => focusActiveDialog())
})
watch(webDAVDialogBusy, (isBusy, wasBusy) => {
  if (isBusy || !wasBusy || activeDialog.value !== 'webdav') return
  void nextTick(() => {
    const focused = document.activeElement
    if (focused === appDialog.value || !appDialog.value?.contains(focused)) focusActiveDialog()
  })
})
watch(webDAVRemoveCredentials, (removeCredentials) => {
  if (!removeCredentials) return
  webDAVPassword.value = ''
  showWebDAVPassword.value = false
})
watch(webDAVStoreCredentials, (storeCredentials) => {
  if (storeCredentials || webDAVConnectionFormMode.value !== 'new') return
  webDAVUsername.value = ''
  webDAVPassword.value = ''
  showWebDAVPassword.value = false
})
watch([imageInsertMode, publicImageURL, imageAltText], () => {
  if (activeDialog.value === 'image' && !imageInsertBusy.value) imageInsertError.value = ''
})
watch([fileName, dirty], () => { void SetWindowTitle(fileName.value, dirty.value) })

onMounted(async () => {
  document.documentElement.dataset.colorScheme = theme.value === 'dark' ? 'dark' : 'light'
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('beforeunload', onBeforeUnload)
  window.addEventListener('languagechange', handleSystemLanguageChange)
  document.addEventListener('focusin', rememberFocusedElement, true)
  removeMenuListeners.push(
    EventsOn('inkmark:menu-action', handleMenuAction),
    EventsOn('inkmark:open-document', handleSystemDocument),
    EventsOn('inkmark:open-recent', (item: unknown) => { void openRecentItem(item) }),
    EventsOn('inkmark:open-error', showError),
    EventsOn('inkmark:close-request', () => { void handleCloseRequest() }),
    EventsOn('inkmark:update-progress', handleUpdateProgress),
  )
  layoutResizeObserver = new ResizeObserver(scheduleLayoutReconciliation)
  if (editor.value) layoutResizeObserver.observe(editor.value)
  if (preview.value) layoutResizeObserver.observe(preview.value)
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
  void cancelActiveWorkspaceMutation({ reportFailure: false })
  clearWebDAVConnectionManager()
  clearImageInsertForm()
  window.clearTimeout(renderTimer)
  previewCommit.invalidate()
  if (updateState.value === 'downloading') void CancelUpdateDownload()
  mermaidRenderCache.clear()
  pendingImageAssets.clear()
  activePreviewImages.release()
  workspacePreviewImages.release()
  if (layoutReconcileFrame !== null) window.cancelAnimationFrame(layoutReconcileFrame)
  layoutReconcileFrame = null
  layoutResizeObserver?.disconnect()
  layoutResizeObserver = null
  if (resolveUnsavedDecision) {
    resolveUnsavedDecision('cancel')
    resolveUnsavedDecision = null
  }
  removeMenuListeners.splice(0).forEach((removeListener) => removeListener())
  lastFocusedElement = null
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('beforeunload', onBeforeUnload)
  window.removeEventListener('languagechange', handleSystemLanguageChange)
  document.removeEventListener('focusin', rememberFocusedElement, true)
})
</script>

<template>
  <div class="app-shell" :class="[`theme-${theme}`, `view-${viewMode}`, { 'preview-first': previewFirst }]">
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
          <span
            class="document-state"
            :class="{ 'remote-document-state': documentStorageKind === 'webdav' }"
          >{{ documentStateLabel }}</span>
        </div>
        <p :title="locationLabel">{{ locationLabel }}</p>
      </div>

      <div class="header-actions">
        <span
          class="connection-badge"
          :class="{
            'webdav-connected': workspace?.provider === 'webdav' || documentStorageKind === 'webdav',
            'is-connecting': webDAVConnecting,
          }"
        >{{ connectionStatusLabel }}</span>
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

      <button
        type="button"
        class="swap-panes-button"
        :class="{ active: previewFirst }"
        :aria-pressed="previewFirst"
        :title="t('toolbar.swapPanes')"
        :disabled="busy"
        @click="swapPaneOrder"
      ><span aria-hidden="true">⇄</span> <span class="swap-panes-label">{{ t('toolbar.swapPanes') }}</span></button>

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

    <div class="workspace-layout" :class="{ 'has-workspace': workspace }">
      <DirectorySidebar
        v-if="workspace"
        :workspace="workspace"
        :rows="workspaceRows"
        :current-provider="currentWorkspaceProvider"
        :current-workspace-id="documentWorkspaceId"
        :current-workspace-path="currentWorkspacePath"
        :labels="workspaceLabels"
        :disabled="busy || workspaceRefreshing || workspaceMutationCancelling"
        :modal-open="Boolean(activeDialog || unsavedTransition || webDAVConflictOpen)"
        :refreshing="workspaceRefreshing"
        :truncated-directories="truncatedWorkspaceDirectories"
        @close="closeWorkspace"
        @refresh="refreshWorkspace()"
        @toggle="toggleWorkspaceDirectory"
        @open="openWorkspaceDocument"
        @preview="showWorkspaceImage"
        @create-markdown="showWorkspaceCreateMarkdown"
        @create-directory="showWorkspaceCreateDirectory"
        @rename="showWorkspaceRename"
        @delete="showWorkspaceDelete"
      />

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
          @input="beginScroll('editor')"
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
    </div>

    <footer class="status-bar">
      <span class="ready-light"></span>
      <span>{{ renderState }}</span>
      <span class="status-spacer"></span>
      <span>{{ t('status.characters', { count: characterCount.toLocaleString(locale) }) }}</span>
      <span>{{ t('status.lines', { count: lineCount.toLocaleString(locale) }) }}</span>
    </footer>

    <div v-if="unsavedTransition" class="modal-backdrop" @click.self="answerUnsavedPrompt('cancel')">
      <section
        class="app-dialog unsaved-dialog"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="dialog-unsaved-title"
        aria-describedby="dialog-unsaved-message"
      >
        <h2 id="dialog-unsaved-title">{{ t('unsaved.title') }}</h2>
        <p id="dialog-unsaved-message">{{ unsavedPromptMessage }}</p>
        <footer class="dialog-actions unsaved-actions">
          <button
            type="button"
            class="button secondary"
            :disabled="resolvingUnsavedPrompt"
            @click="answerUnsavedPrompt('discard')"
          >{{ t('unsaved.discard') }}</button>
          <span class="status-spacer"></span>
          <button
            type="button"
            class="button secondary"
            :disabled="resolvingUnsavedPrompt"
            @click="answerUnsavedPrompt('cancel')"
          >{{ t('unsaved.cancel') }}</button>
          <button
            type="button"
            class="button primary"
            :disabled="resolvingUnsavedPrompt"
            autofocus
            @click="answerUnsavedPrompt('save')"
          >{{ t('unsaved.save') }}</button>
        </footer>
      </section>
    </div>

    <div v-if="webDAVConflictOpen" class="modal-backdrop" @click.self="dismissWebDAVConflict">
      <section
        class="app-dialog"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="dialog-webdav-conflict"
        aria-describedby="dialog-webdav-conflict-message"
      >
        <h2 id="dialog-webdav-conflict">{{ t('webdav.conflictTitle') }}</h2>
        <p id="dialog-webdav-conflict-message" class="webdav-conflict-message">
          {{ t('webdav.conflictMessage', { name: fileName }) }}
        </p>
        <footer class="dialog-actions">
          <button
            type="button"
            class="button secondary"
            :disabled="webDAVConflictBusy"
            @click="dismissWebDAVConflict"
          >{{ t('webdav.conflictCancel') }}</button>
          <span class="status-spacer"></span>
          <button
            type="button"
            class="button secondary"
            :disabled="webDAVConflictBusy"
            @click="reloadConflictingWebDAVDocument"
          >{{ t('webdav.conflictReload') }}</button>
          <button
            type="button"
            class="button primary"
            :disabled="webDAVConflictBusy"
            @click="overwriteConflictingWebDAVDocument"
          >{{ t('webdav.conflictOverwrite') }}</button>
        </footer>
      </section>
    </div>

    <div v-if="activeDialog" class="modal-backdrop" @click.self="handleActiveDialogBackdrop">
      <section
        ref="appDialog"
        class="app-dialog"
        :class="{
          'webdav-dialog': activeDialog === 'webdav',
          'has-webdav-delete-confirmation': activeDialog === 'webdav' && Boolean(webDAVDeleteCandidate),
          'image-dialog': activeDialog === 'image',
          'workspace-dialog': activeDialog === 'workspace',
          'workspace-image-preview-dialog': activeDialog === 'workspace' && workspaceDialogView === 'image',
          'about-licenses-dialog': activeDialog === 'about' && aboutView === 'third-party',
        }"
        :role="activeDialog === 'workspace' && workspaceDialogView === 'delete' ? 'alertdialog' : 'dialog'"
        aria-modal="true"
        :aria-labelledby="`dialog-${activeDialog}`"
        :aria-describedby="activeDialog === 'webdav'
          ? 'dialog-webdav-description'
          : activeDialog === 'workspace' && workspaceDialogView === 'delete'
            ? 'dialog-workspace-description'
            : undefined"
      >
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
        <template v-else-if="activeDialog === 'webdav'">
          <header class="webdav-dialog-header" :inert="Boolean(webDAVDeleteCandidate)">
            <div>
              <h2 id="dialog-webdav">{{ t('webdav.title') }}</h2>
              <p id="dialog-webdav-description">{{ t('webdav.description') }}</p>
            </div>
            <button
              type="button"
              class="dialog-close-button"
              :aria-label="t('webdav.closeDialog')"
              :disabled="webDAVDialogBusy"
              @click="dismissActiveDialog"
            >×</button>
          </header>

          <nav
            class="webdav-dialog-tabs"
            :aria-label="t('webdav.connectionMethod')"
            :inert="Boolean(webDAVDeleteCandidate)"
          >
            <button
              id="webdav-saved-tab"
              type="button"
              :aria-pressed="webDAVDialogView !== 'temporary'"
              :class="{ active: webDAVDialogView !== 'temporary' }"
              :disabled="webDAVDialogBusy || webDAVDialogView === 'new' || webDAVDialogView === 'edit'"
              :data-dialog-initial="webDAVDialogView === 'saved' ? '' : null"
              @click="showSavedWebDAVConnections"
            >{{ t('webdav.savedTab') }}</button>
            <button
              id="webdav-temporary-tab"
              type="button"
              :aria-pressed="webDAVDialogView === 'temporary'"
              :class="{ active: webDAVDialogView === 'temporary' }"
              :disabled="webDAVDialogBusy || webDAVDialogView === 'new' || webDAVDialogView === 'edit'"
              @click="showTemporaryWebDAVForm()"
            >{{ t('webdav.temporaryTab') }}</button>
          </nav>

          <section
            v-if="webDAVDialogView === 'saved'"
            id="webdav-saved-panel"
            class="saved-connections"
            :class="{ 'has-delete-confirmation': webDAVDeleteCandidate }"
            :aria-busy="savedWebDAVConnectionsLoading"
          >
            <div
              class="saved-connections-content"
              :inert="Boolean(webDAVDeleteCandidate)"
              :aria-hidden="webDAVDeleteCandidate ? 'true' : undefined"
            >
              <header>
              <div>
                <strong>{{ t('webdav.savedConnections') }}</strong>
                <span>{{ t('webdav.savedConnectionsHint') }}</span>
              </div>
              <button
                type="button"
                class="button primary"
                :disabled="webDAVDialogBusy"
                @click="showNewSavedWebDAVForm"
              >{{ t('webdav.newSavedConnection') }}</button>
            </header>
            <div v-if="savedWebDAVConnectionsLoading" class="saved-connections-empty" role="status">
              <strong>{{ t('webdav.loadingSavedConnections') }}</strong>
            </div>
            <div
              v-else-if="!savedWebDAVConnections.length && !savedWebDAVConnectionsError"
              class="saved-connections-empty"
            >
              <strong>{{ t('webdav.noSavedConnections') }}</strong>
              <span>{{ t('webdav.noSavedConnectionsHint') }}</span>
              <div class="saved-connections-empty-actions">
                <button type="button" class="button primary" @click="showNewSavedWebDAVForm">
                  {{ t('webdav.newSavedConnection') }}
                </button>
                <button type="button" class="button secondary" @click="showTemporaryWebDAVForm()">
                  {{ t('webdav.temporaryTab') }}
                </button>
              </div>
            </div>
            <ul v-else-if="savedWebDAVConnections.length" class="saved-connection-list">
              <li v-for="connection in savedWebDAVConnections" :key="connection.id">
                <div class="saved-connection-details">
                  <div class="saved-connection-title-row">
                    <strong>{{ connection.name }}</strong>
                    <span
                      class="saved-connection-badge"
                      :class="{
                        available: connection.hasCredentials && connection.credentialsAvailable,
                        warning: connection.hasCredentials && !connection.credentialsAvailable,
                      }"
                    >{{ connection.hasCredentials && connection.credentialsAvailable
                      ? t('webdav.credentialsSavedBadge')
                      : connection.hasCredentials
                        ? t('webdav.credentialsRequiredBadge')
                        : t('webdav.noCredentialsBadge') }}</span>
                  </div>
                  <span :title="connection.endpoint">{{ connection.endpoint }}</span>
                  <small v-if="connection.username">{{ t('webdav.savedUsername', { username: connection.username }) }}</small>
                  <small v-else-if="connection.usernamePresent">{{ t('webdav.savedUsernameUnavailable') }}</small>
                  <small v-else>{{ t('webdav.savedAnonymous') }}</small>
                  <small v-if="connection.hasCredentials && !connection.credentialsAvailable" class="saved-credential-warning">
                    {{ t('webdav.savedCredentialsUnavailable') }}
                  </small>
                </div>
                <div class="saved-connection-actions" :aria-label="t('webdav.connectionActions', { name: connection.name })">
                  <button
                    type="button"
                    class="button primary"
                    :disabled="webDAVDialogBusy"
                    :aria-label="t('webdav.connectNamed', { name: connection.name })"
                    @click="connectSavedWebDAV(connection)"
                  >{{ webDAVConnectingConnectionID === connection.id
                    ? t('webdav.connecting')
                    : t('webdav.connectSaved') }}</button>
                  <button
                    type="button"
                    class="button secondary"
                    :disabled="webDAVDialogBusy"
                    :aria-label="t('webdav.editNamed', { name: connection.name })"
                    @click="showEditSavedWebDAVForm(connection)"
                  >{{ t('common.edit') }}</button>
                  <button
                    type="button"
                    class="button danger subtle"
                    :disabled="webDAVDialogBusy"
                    :aria-label="t('webdav.deleteNamed', { name: connection.name })"
                    @click="requestDeleteSavedWebDAVConnection(connection)"
                  >{{ t('common.delete') }}</button>
                </div>
              </li>
            </ul>

            <p
              v-if="webDAVConnectionOperation === 'connecting-saved'"
              class="connection-status is-busy"
              role="status"
              aria-live="polite"
            >{{ t('webdav.connectingSaved', { name: webDAVConnectingConnectionName }) }}</p>

            <div
              v-if="!savedWebDAVConnectionsLoading && savedWebDAVConnectionsError && !webDAVDeleteCandidate"
              class="connection-status is-error saved-connections-error"
              role="alert"
            >
              <span>{{ savedWebDAVConnectionsError }}</span>
              <button type="button" class="button secondary" @click="loadSavedWebDAVConnections">
                {{ t('webdav.retrySavedConnections') }}
              </button>
              </div>
            </div>

            <section
              v-if="webDAVDeleteCandidate"
              class="saved-connection-delete"
              role="alertdialog"
              aria-modal="true"
              :aria-label="t('webdav.deleteConnectionTitle')"
              :aria-describedby="savedWebDAVConnectionsError
                ? 'webdav-delete-message webdav-delete-error'
                : 'webdav-delete-message'"
            >
              <strong>{{ t('webdav.deleteConnectionTitle') }}</strong>
              <p id="webdav-delete-message">{{ t('webdav.deleteConnectionMessage', { name: webDAVDeleteCandidate.name }) }}</p>
              <code>{{ webDAVDeleteCandidate.endpoint }}</code>
              <p
                v-if="savedWebDAVConnectionsError"
                id="webdav-delete-error"
                class="connection-status is-error"
                role="alert"
              >{{ savedWebDAVConnectionsError }}</p>
              <div>
                <button
                  type="button"
                  class="button secondary"
                  :disabled="webDAVConnectionManagerBusy"
                  data-dialog-initial
                  @click="cancelDeleteSavedWebDAVConnection"
                >{{ t('webdav.keepSavedConnection') }}</button>
                <button
                  type="button"
                  class="button danger"
                  :disabled="webDAVConnectionManagerBusy"
                  @click="confirmDeleteSavedWebDAVConnection"
                >{{ webDAVConnectionOperation === 'deleting'
                  ? t('webdav.deletingConnection')
                  : t('webdav.deleteConnectionConfirm') }}</button>
              </div>
            </section>
          </section>

          <form
            v-else
            id="webdav-connection-form"
            class="connection-form"
            autocomplete="off"
            @submit.prevent="submitWebDAVDialog"
          >
            <header class="connection-form-heading">
              <button
                v-if="webDAVDialogView === 'new' || webDAVDialogView === 'edit'"
                type="button"
                class="connection-back-button"
                :disabled="webDAVDialogBusy"
                @click="cancelSavedWebDAVForm"
              >← {{ t('webdav.backToSaved') }}</button>
              <div>
                <strong>{{ webDAVConnectionFormMode === 'connect'
                  ? t('webdav.temporaryConnection')
                  : webDAVConnectionFormMode === 'new'
                    ? t('webdav.newConnectionTitle')
                    : t('webdav.editConnectionTitle') }}</strong>
                <span>{{ webDAVConnectionFormMode === 'connect'
                  ? t('webdav.temporaryConnectionHint')
                  : t('webdav.systemCredentialStore') }}</span>
              </div>
            </header>
            <div v-if="webDAVConnectionFormMode !== 'connect'" class="connection-field">
              <label for="webdav-connection-name">{{ t('webdav.connectionName') }}</label>
              <input
                id="webdav-connection-name"
                v-model="webDAVConnectionName"
                type="text"
                autocomplete="off"
                :placeholder="t('webdav.connectionNamePlaceholder')"
                :disabled="webDAVDialogBusy"
                :autofocus="webDAVDialogView === 'new' || webDAVDialogView === 'edit'"
                required
                data-dialog-initial
                @input="webDAVConnectionError = ''"
              />
            </div>
            <div class="connection-field">
              <label for="webdav-server-url">{{ t('webdav.serverURL') }}</label>
              <input
                id="webdav-server-url"
                v-model="webDAVBaseURL"
                type="url"
                inputmode="url"
                spellcheck="false"
                autocomplete="off"
                :placeholder="t('webdav.serverURLPlaceholder')"
                :aria-invalid="webDAVEndpointInvalid"
                :disabled="webDAVDialogBusy"
                :autofocus="webDAVDialogView === 'temporary'"
                required
                :data-dialog-initial="webDAVDialogView === 'temporary' ? '' : null"
                @input="webDAVConnectionError = ''"
              />
              <p
                v-if="webDAVConnectionFormMode === 'edit'
                  && editingChangesWebDAVOrigin
                  && !webDAVPassword
                  && !webDAVRemoveCredentials"
                class="connection-field-hint is-warning"
              >{{ t('webdav.endpointChangeNeedsPassword') }}</p>
            </div>
            <label v-if="webDAVConnectionFormMode === 'new'" class="connection-checkbox credential-choice">
              <input v-model="webDAVStoreCredentials" type="checkbox" :disabled="webDAVDialogBusy" @change="webDAVConnectionError = ''" />
              <span>
                <strong>{{ t('webdav.saveCredentialsSecurely') }}</strong>
                <small>{{ t('webdav.systemCredentialStore') }}</small>
              </span>
            </label>
            <div class="connection-credentials-grid">
              <div class="connection-field">
                <label for="webdav-username">{{ t('webdav.username') }}</label>
                <input
                  id="webdav-username"
                  v-model="webDAVUsername"
                  type="text"
                  autocomplete="off"
                  autocapitalize="none"
                  spellcheck="false"
                  :placeholder="t('webdav.usernamePlaceholder')"
                  :disabled="webDAVDialogBusy
                    || (webDAVConnectionFormMode === 'edit' && webDAVRemoveCredentials)
                    || (webDAVConnectionFormMode === 'new' && !webDAVStoreCredentials)"
                  @input="webDAVConnectionError = ''"
                />
              </div>
              <div class="connection-field">
                <label for="webdav-password">{{ t('webdav.password') }}</label>
                <div class="connection-password-control">
                  <input
                    id="webdav-password"
                    v-model="webDAVPassword"
                    :type="showWebDAVPassword ? 'text' : 'password'"
                    autocomplete="off"
                    :placeholder="webDAVConnectionFormMode === 'edit'
                      ? t('webdav.passwordKeepPlaceholder')
                      : t('webdav.passwordPlaceholder')"
                    :disabled="webDAVDialogBusy
                      || (webDAVConnectionFormMode === 'edit' && webDAVRemoveCredentials)
                      || (webDAVConnectionFormMode === 'new' && !webDAVStoreCredentials)"
                    @input="webDAVConnectionError = ''"
                  />
                  <button
                    type="button"
                    class="connection-password-toggle"
                    :disabled="webDAVDialogBusy
                      || (webDAVConnectionFormMode === 'edit' && webDAVRemoveCredentials)
                      || (webDAVConnectionFormMode === 'new' && !webDAVStoreCredentials)"
                    :aria-label="showWebDAVPassword ? t('webdav.hidePassword') : t('webdav.showPassword')"
                    :aria-pressed="showWebDAVPassword"
                    aria-controls="webdav-password"
                    @click="showWebDAVPassword = !showWebDAVPassword"
                  >{{ showWebDAVPassword ? t('webdav.hidePassword') : t('webdav.showPassword') }}</button>
                </div>
              </div>
            </div>
            <p class="connection-field-hint">{{ webDAVConnectionFormMode === 'connect'
              ? t('webdav.passwordNotStored')
              : webDAVConnectionFormMode === 'edit'
                ? t('webdav.editPasswordHint')
                : webDAVStoreCredentials
                  ? t('webdav.credentialsWillBeStored')
                  : t('webdav.connectionWithoutCredentials') }}</p>
            <label
              v-if="webDAVConnectionFormMode === 'edit' && editingSavedWebDAVConnection?.hasCredentials"
              class="connection-checkbox danger-option"
            >
              <input v-model="webDAVRemoveCredentials" type="checkbox" :disabled="webDAVDialogBusy" @change="webDAVConnectionError = ''" />
              <span>{{ t('webdav.removeSavedCredentials') }}</span>
            </label>
            <p
              v-if="webDAVDialogBusy || webDAVConnectionError"
              class="connection-status"
              :class="{ 'is-busy': webDAVDialogBusy, 'is-error': Boolean(webDAVConnectionError) }"
              :role="webDAVConnectionError ? 'alert' : 'status'"
              aria-live="polite"
            >{{ webDAVDialogBusy ? webDAVDialogBusyMessage : webDAVConnectionError }}</p>
          </form>
        </template>
        <template v-else-if="activeDialog === 'image'">
          <h2 id="dialog-image">{{ t('image.title') }}</h2>
          <p>{{ t('image.description') }}</p>
          <form
            id="image-insert-form"
            class="image-insert-form"
            @submit.prevent="insertSelectedImage"
          >
            <fieldset class="image-mode-options" :disabled="imageInsertBusy">
              <legend>{{ t('image.storageMode') }}</legend>
              <label v-for="mode in imageInsertModes" :key="mode.value">
                <input
                  v-model="imageInsertMode"
                  type="radio"
                  name="image-storage-mode"
                  :value="mode.value"
                  :disabled="mode.disabled"
                />
                <span>{{ mode.label }}</span>
              </label>
            </fieldset>

            <div class="connection-field">
              <label for="image-alt-text">{{ t('image.altText') }}</label>
              <input
                id="image-alt-text"
                v-model="imageAltText"
                type="text"
                :disabled="imageInsertBusy"
                :placeholder="t('image.altPlaceholder')"
              />
            </div>

            <div v-if="imageInsertMode === 'public'" class="connection-field">
              <label for="public-image-url">{{ t('image.publicURL') }}</label>
              <input
                id="public-image-url"
                v-model="publicImageURL"
                type="url"
                inputmode="url"
                spellcheck="false"
                autocomplete="off"
                :disabled="imageInsertBusy"
                :placeholder="t('image.publicURLPlaceholder')"
                autofocus
              />
              <p class="connection-field-hint">{{ t('image.publicURLHint') }}</p>
            </div>

            <div v-else class="image-file-picker">
              <button
                type="button"
                class="button secondary"
                :disabled="imageInsertBusy"
                @click="selectImageForInsertion"
              >{{ selectedImage ? t('image.chooseAnother') : t('image.chooseFile') }}</button>
              <div v-if="selectedImage" class="selected-image-summary">
                <strong>{{ selectedImage.name }}</strong>
                <span>{{ selectedImage.width }} × {{ selectedImage.height }}</span>
                <span>{{ t('image.fileSize', { size: Math.max(1, Math.ceil(selectedImage.size / 1024)) }) }}</span>
              </div>
              <p class="connection-field-hint">{{ t('image.fileHint') }}</p>
            </div>

            <p class="image-mode-hint">
              {{ imageInsertMode === 'local'
                ? t('image.localHint')
                : imageInsertMode === 'webdav'
                  ? t('image.webDAVHint')
                  : imageInsertMode === 'data'
                    ? t('image.dataHint')
                    : t('image.publicHint') }}
            </p>
            <p
              v-if="imageInsertError"
              class="connection-status is-error"
              role="status"
              aria-live="polite"
            >{{ imageInsertError }}</p>
          </form>
        </template>
        <template v-else-if="activeDialog === 'workspace'">
          <h2 id="dialog-workspace">{{ workspaceDialogTitle }}</h2>

          <p
            v-if="activeWorkspaceMutation && ['rename', 'delete'].includes(workspaceDialogView)"
            class="workspace-lock-status"
            role="status"
            aria-live="polite"
          >{{ workspaceMutationLockLabel }}</p>

          <template v-if="workspaceDialogView === 'image'">
            <p class="workspace-preview-location" :title="workspaceDialogEntry?.path">
              {{ workspaceDialogEntry?.path }}
            </p>
            <div
              v-if="workspaceMutationBusy"
              class="workspace-image-loading"
              role="status"
              aria-live="polite"
            >{{ t('workspace.loadingImage') }}</div>
            <figure v-else-if="workspacePreviewImage && workspacePreviewSource" class="workspace-image-preview">
              <img
                :src="workspacePreviewSource"
                :alt="workspaceDialogEntry?.name || t('workspace.imagePreviewTitle')"
              />
              <figcaption>
                <strong>{{ workspacePreviewImage.name }}</strong>
                <span>{{ workspacePreviewImage.width }} × {{ workspacePreviewImage.height }}</span>
                <span>{{ t('image.fileSize', { size: Math.max(1, Math.ceil(workspacePreviewImage.size / 1024)) }) }}</span>
              </figcaption>
            </figure>
          </template>

          <template v-else-if="workspaceDialogView === 'delete'">
            <p id="dialog-workspace-description" class="workspace-delete-message">
              {{ workspaceDeleteDescription }}
            </p>
            <p
              v-if="workspaceDeleteAffectsCurrentDocument || workspaceDeleteBufferPreserved"
              class="workspace-preserve-message"
            >
              {{ t('workspace.deleteCurrentPreserved') }}
            </p>
          </template>

          <form
            v-else
            id="workspace-entry-form"
            class="workspace-entry-form"
            @submit.prevent="submitWorkspaceNameDialog"
          >
            <label for="workspace-entry-name">{{ t('workspace.nameLabel') }}</label>
            <input
              id="workspace-entry-name"
              v-model="workspaceEntryName"
              type="text"
              autocomplete="off"
              spellcheck="false"
              maxlength="255"
              data-dialog-initial
              :disabled="workspaceMutationBusy || workspaceMutationNeedsRestart"
              @focus="($event.target as HTMLInputElement).select()"
            />
            <p class="workspace-entry-parent">
              {{ t('workspace.parentLocation', { path: workspaceDialogParentLabel }) }}
            </p>
          </form>

          <p
            v-if="workspaceMutationError"
            class="connection-status is-error workspace-operation-error"
            role="alert"
            aria-live="assertive"
          >{{ t('workspace.operationFailed', { message: workspaceMutationError }) }}</p>
          <p
            v-if="workspaceMutationNeedsRestart"
            class="workspace-mutation-retry"
            role="status"
          >{{ t('workspace.webDAVMutationRetry') }}</p>
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
        <template v-else-if="activeDialog === 'about' && aboutView === 'third-party'">
          <header class="about-licenses-heading">
            <h2 id="dialog-about">{{ t('help.thirdPartyLicenses') }}</h2>
            <p>{{ t('help.thirdPartyLicensesDescription') }}</p>
          </header>
          <p
            v-if="thirdPartyNoticesState === 'loading'"
            class="about-licenses-status"
            role="status"
            aria-live="polite"
          >{{ t('help.thirdPartyLicensesLoading') }}</p>
          <p
            v-else-if="thirdPartyNoticesState === 'error'"
            class="about-licenses-status update-error"
            role="alert"
          >{{ t('help.thirdPartyLicensesUnavailable') }}</p>
          <textarea
            v-else
            class="third-party-notices"
            :aria-label="t('help.thirdPartyLicensesContent')"
            :value="thirdPartyNotices"
            readonly
            spellcheck="false"
            wrap="off"
          ></textarea>
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
            <dt>{{ t('help.thirdPartyLicenses') }}</dt>
            <dd class="about-license-entry">
              <button
                type="button"
                class="about-license-link"
                @click="showThirdPartyLicenses"
              >{{ t('help.thirdPartyLicensesOpen') }}</button>
              <small>{{ t('help.thirdPartyLicensesDescription') }}</small>
            </dd>
            <dt>{{ t('help.updateStatus') }}</dt>
            <dd role="status" aria-live="polite">{{ updateStatusText }}</dd>
            <template v-if="updateState === 'downloading'">
              <dt>{{ t('help.downloadProgress') }}</dt>
              <dd>
                <progress :value="updateDownload?.progress || 0" max="1"></progress>
                {{ Math.round((updateDownload?.progress || 0) * 100) }}%
              </dd>
            </template>
            <template v-if="updateError">
              <dt>{{ t('help.updateError') }}</dt>
              <dd class="update-error">{{ updateError }}</dd>
            </template>
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
        <footer
          class="dialog-actions"
          :inert="activeDialog === 'webdav' && Boolean(webDAVDeleteCandidate)"
        >
          <template v-if="activeDialog === 'webdav'">
            <button
              v-if="webDAVDialogView === 'saved'"
              type="button"
              class="button primary"
              :disabled="webDAVDialogBusy"
              @click="dismissActiveDialog"
            >{{ t('common.close') }}</button>
            <button
              v-else
              type="button"
              class="button secondary"
              :disabled="webDAVDialogBusy"
              @click="webDAVDialogView === 'temporary' ? dismissActiveDialog() : cancelSavedWebDAVForm()"
            >{{ t('common.cancel') }}</button>
            <button
              v-if="webDAVDialogView !== 'saved'"
              type="submit"
              form="webdav-connection-form"
              class="button primary"
              :disabled="!canSubmitWebDAVForm"
            >{{ webDAVDialogBusy
                ? webDAVDialogBusyMessage
                : webDAVConnectionFormMode === 'connect'
                  ? t('webdav.connect')
                  : t('webdav.saveConnection') }}</button>
          </template>
          <template v-else-if="activeDialog === 'image'">
            <button
              type="button"
              class="button secondary"
              :disabled="imageInsertBusy"
              @click="dismissActiveDialog"
            >{{ t('common.cancel') }}</button>
            <button
              type="submit"
              form="image-insert-form"
              class="button primary"
              :disabled="!canInsertImage"
            >{{ imageInsertBusy ? t('image.processing') : t('image.insert') }}</button>
          </template>
          <template v-else-if="activeDialog === 'workspace'">
            <button
              type="button"
              class="button secondary"
              :data-dialog-initial="['delete', 'image'].includes(workspaceDialogView) ? '' : undefined"
              :disabled="workspaceMutationBusy && workspaceDialogView !== 'image'"
              @click="dismissActiveDialog"
            >{{ workspaceDialogView === 'image' ? t('common.close') : t('common.cancel') }}</button>
            <button
              v-if="workspaceDialogView === 'delete'"
              type="button"
              class="button danger"
              :disabled="workspaceMutationBusy || workspaceMutationNeedsRestart"
              @click="confirmDeleteWorkspaceEntry"
            >{{ workspaceMutationBusy ? t('workspace.deleting') : t('workspace.deleteConfirm') }}</button>
            <button
              v-else-if="workspaceDialogView !== 'image'"
              type="submit"
              form="workspace-entry-form"
              class="button primary"
              :disabled="workspaceMutationBusy || workspaceMutationNeedsRestart || !workspaceEntryName.trim()"
            >{{ workspaceMutationBusy ? t('workspace.processing') : t('common.confirm') }}</button>
          </template>
          <template v-else>
            <button
              v-if="activeDialog === 'about' && aboutView === 'third-party'"
              type="button"
              class="button secondary"
              data-dialog-initial
              @click="showAboutOverview"
            >{{ t('help.thirdPartyLicensesBack') }}</button>
            <button
              v-else-if="activeDialog === 'about' && !['available', 'downloading', 'cancelling', 'ready', 'installing'].includes(updateState)"
              type="button"
              class="button secondary"
              :disabled="updateState === 'checking'"
              @click="checkForUpdates()"
            >{{ updateState === 'checking' ? t('help.checkingUpdate') : t('help.checkUpdate') }}</button>
            <button
              v-else-if="activeDialog === 'about' && updateState === 'downloading'"
              type="button"
              class="button secondary"
              @click="cancelUpdateDownload"
            >{{ t('help.cancelDownload') }}</button>
            <button
              v-else-if="activeDialog === 'about' && updateState === 'cancelling'"
              type="button"
              class="button secondary"
              disabled
            >{{ t('help.updateCancellingButton') }}</button>
            <button
              v-else-if="activeDialog === 'about'
                && updateInfo?.updateAvailable
                && ['available', 'ready', 'installing'].includes(updateState)"
              type="button"
              class="button secondary"
              :disabled="updateState === 'installing'"
              @click="upgradeApplication"
            >{{ updateState === 'ready'
                ? t('help.installUpdate')
                : updateState === 'installing'
                  ? t('help.updateInstallingButton')
                  : updateInfo?.installable && updateInfo?.checksumAvailable
                    ? t('help.downloadAndInstall')
                    : t('help.openDownloadPage') }}</button>
            <span v-if="activeDialog === 'about'" class="status-spacer"></span>
            <button type="button" class="button primary" @click="activeDialog = null">{{ t('common.close') }}</button>
          </template>
        </footer>
      </section>
    </div>
  </div>
</template>
