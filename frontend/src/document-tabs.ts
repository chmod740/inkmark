import {
  isActiveWorkspaceFile,
  sameWorkspaceFile,
  workspaceDirectoryKey,
  type WorkspaceProvider,
  type WorkspaceChildren,
  type WorkspaceData,
} from './workspace-tree.ts'
import type { BuiltInDocumentKind } from './ui-state.ts'
import type { SourceDerivedData } from './editor-features.ts'

export type TabStorageKind = 'local' | 'webdav' | 'builtin'

export interface TabDocumentData {
  path?: string
  name?: string
  content?: string
  welcome?: boolean
  builtIn?: string
  activationId?: string
  storageKind?: TabStorageKind
  displayLocation?: string
  workspacePath?: string
  workspaceId?: string
  localDocumentId?: string
  remoteDocumentId?: string
  etag?: string
}

export interface TabWorkspaceState {
  workspace: WorkspaceData
  children: WorkspaceChildren
  expandedDirectories: ReadonlySet<string>
  loadingDirectories: ReadonlySet<string>
  truncatedDirectories: ReadonlySet<string>
}

export interface DocumentTabState {
  id: string
  name: string
  source: string
  savedSource: string
  storageKind: TabStorageKind
  localPath: string
  location: string
  workspaceId: string
  workspacePath: string
  localDocumentId: string
  remoteDocumentId: string
  remoteWorkspaceId: string
  etag: string
  builtIn: BuiltInDocumentKind | null
  activationId: string
  workspaceState: TabWorkspaceState | null
  renderState: string
  editorScrollTop: number
  previewScrollTop: number
  selectionStart: number
  selectionEnd: number
  findOpen: boolean
  findQuery: string
  findCaseSensitive: boolean
  findCurrentStart: number
  findCurrentEnd: number
  findMatchOrdinal: number
  stickySourceLine: number
  sourceRevision: number
  derivedSource: SourceDerivedData | null
}

export type TabCloseMode = 'self' | 'left' | 'right' | 'others'

export function createDocumentTab(
  id: string,
  document: TabDocumentData,
  workspaceState: TabWorkspaceState | null = null,
  renderState = '',
): DocumentTabState {
  const builtIn: BuiltInDocumentKind | null = document.builtIn === 'render-test'
    ? 'render-test'
    : document.builtIn === 'welcome' || document.welcome
      ? 'welcome'
      : null
  const storageKind: TabStorageKind = document.storageKind === 'webdav'
    ? 'webdav'
    : builtIn
      ? 'builtin'
      : 'local'
  const source = document.content || ''
  const localPath = storageKind === 'local' ? document.path || '' : ''
  const workspaceId = document.workspaceId || ''
  return {
    id,
    name: document.name || 'Untitled.md',
    source,
    savedSource: source,
    storageKind,
    localPath,
    location: document.displayLocation || localPath,
    workspaceId,
    workspacePath: document.workspacePath || '',
    localDocumentId: storageKind === 'local' ? document.localDocumentId || '' : '',
    remoteDocumentId: storageKind === 'webdav' ? document.remoteDocumentId || '' : '',
    remoteWorkspaceId: storageKind === 'webdav' ? workspaceId : '',
    etag: storageKind === 'webdav' ? document.etag || '' : '',
    builtIn,
    activationId: document.activationId || '',
    workspaceState,
    renderState,
    editorScrollTop: 0,
    previewScrollTop: 0,
    selectionStart: 0,
    selectionEnd: 0,
    findOpen: false,
    findQuery: '',
    findCaseSensitive: false,
    findCurrentStart: -1,
    findCurrentEnd: -1,
    findMatchOrdinal: 0,
    stickySourceLine: 0,
    sourceRevision: 1,
    derivedSource: null,
  }
}

export function documentTabsMatch(left: DocumentTabState, right: TabDocumentData): boolean {
  const rightBuiltIn: BuiltInDocumentKind | null = right.builtIn === 'render-test'
    ? 'render-test'
    : right.builtIn === 'welcome' || right.welcome
      ? 'welcome'
      : null
  if (left.builtIn || rightBuiltIn) {
    return Boolean(left.builtIn && rightBuiltIn && left.builtIn === rightBuiltIn)
  }
  if (left.storageKind === 'webdav' || right.storageKind === 'webdav') {
    return left.storageKind === 'webdav'
      && right.storageKind === 'webdav'
      && left.remoteWorkspaceId === (right.workspaceId || '')
      && workspaceDirectoryKey(left.workspacePath) === workspaceDirectoryKey(right.workspacePath || '')
  }
  return Boolean(left.localPath && right.path && sameWorkspaceFile(left.localPath, right.path))
}

export function findMatchingDocumentTab(tabs: readonly DocumentTabState[], document: TabDocumentData) {
  return tabs.find((tab) => documentTabsMatch(tab, document)) || null
}

export function findWorkspaceDocumentTab(
  tabs: readonly DocumentTabState[],
  provider: WorkspaceProvider,
  workspaceId: string,
  workspacePath: string,
) {
  return tabs.find((tab) => isActiveWorkspaceFile(
    provider,
    workspaceId,
    workspacePath,
    tab.storageKind === 'webdav' ? 'webdav' : tab.storageKind === 'local' ? 'local' : null,
    tab.workspaceId,
    tab.workspacePath,
  )) || null
}

export function tabCloseTargetIDs(
  tabs: readonly DocumentTabState[],
  tabId: string,
  mode: TabCloseMode,
): string[] {
  const index = tabs.findIndex((tab) => tab.id === tabId)
  if (index < 0) return []
  if (mode === 'self') return [tabId]
  if (mode === 'left') return tabs.slice(0, index).map((tab) => tab.id)
  if (mode === 'right') return tabs.slice(index + 1).map((tab) => tab.id)
  return tabs.filter((tab) => tab.id !== tabId).map((tab) => tab.id)
}

export function nextActiveTabID(tabs: readonly DocumentTabState[], closedIndex: number): string {
	if (tabs.length <= 1) return ''
	return tabs[closedIndex + 1]?.id || tabs[closedIndex - 1]?.id || ''
}

export function rebaseDocumentTabs(
  tabs: readonly DocumentTabState[],
  workspaceId: string,
  previousPath: string,
  nextPath: string,
): DocumentTabState[] {
  const previous = workspaceDirectoryKey(previousPath)
  const replacement = workspaceDirectoryKey(nextPath)
  return tabs.map((tab) => {
    if (tab.workspaceId !== workspaceId) return tab
    const current = workspaceDirectoryKey(tab.workspacePath)
    if (current !== previous && !current.startsWith(`${previous}/`)) return tab
    const suffix = current === previous ? '' : current.slice(previous.length + 1)
    const workspacePath = suffix ? `${replacement}/${suffix}` : replacement
    return { ...tab, workspacePath }
  })
}
