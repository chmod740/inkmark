export type WorkspaceEntryKind = 'directory' | 'markdown' | 'image'
export type WorkspaceProvider = 'local' | 'webdav'

export interface WorkspaceEntryData {
  name: string
  path: string
  absolutePath: string
  kind: WorkspaceEntryKind
  revision: string
}

export interface WorkspaceData {
  id: string
  provider: WorkspaceProvider
  path: string
  name: string
  entries: WorkspaceEntryData[]
  truncated: boolean
}

export interface WorkspaceDirectoryData {
  entries: WorkspaceEntryData[]
  truncated: boolean
}

export interface WorkspaceTreeRow {
  entry: WorkspaceEntryData
  depth: number
  expanded: boolean
  loaded: boolean
  loading: boolean
}

export type WorkspaceChildren = Readonly<Record<string, readonly WorkspaceEntryData[]>>

export const workspaceRootKey = ''
const workspaceEntryKindRank: Readonly<Record<WorkspaceEntryKind, number>> = {
  directory: 0,
  markdown: 1,
  image: 2,
}

export function normalizeWorkspaceProvider(value: unknown): WorkspaceProvider | null {
  if (value === 'webdav') return 'webdav'
  // Workspaces produced before provider support were always local. Preserve
  // that payload contract while rejecting unknown provider names.
  if (value === undefined || value === null || value === '' || value === 'local') return 'local'
  return null
}

export function workspaceDirectoryKey(path: string): string {
  return path
    .replaceAll('\\', '/')
    .replace(/^\.\//, '')
    .replace(/^\/+|\/+$/g, '')
}

export function workspaceBackendDirectoryPath(path: string): string {
  return workspaceDirectoryKey(path) || '.'
}

export function workspaceParentDirectoryKey(path: string): string {
  const key = workspaceDirectoryKey(path)
  const separator = key.lastIndexOf('/')
  return separator < 0 ? workspaceRootKey : key.slice(0, separator)
}

export function loadedWorkspaceDirectoryKeys(children: WorkspaceChildren): string[] {
  return Object.keys(children)
    .map(workspaceDirectoryKey)
    .filter((path) => path !== workspaceRootKey)
    .sort((left, right) => {
      const depthDifference = left.split('/').length - right.split('/').length
      return depthDifference || left.localeCompare(right)
    })
}

export function workspaceDirectoryExists(children: WorkspaceChildren, path: string): boolean {
  const key = workspaceDirectoryKey(path)
  if (key === workspaceRootKey) return Object.hasOwn(children, workspaceRootKey)
  const parentKey = workspaceParentDirectoryKey(key)
  return (children[parentKey] || []).some((entry) => (
    entry.kind === 'directory' && workspaceDirectoryKey(entry.path) === key
  ))
}

export function retainExistingWorkspaceDirectories(
  directories: ReadonlySet<string>,
  children: WorkspaceChildren,
): ReadonlySet<string> {
  return new Set(
    Array.from(directories, workspaceDirectoryKey)
      .filter((path) => workspaceDirectoryExists(children, path)),
  )
}

export function workspaceTreeIndent(depth: number): number {
  const safeDepth = Number.isFinite(depth) ? Math.max(0, Math.floor(depth)) : 0
  return 10 + Math.min(safeDepth, 6) * 14
}

export function normalizeWorkspaceEntries(value: unknown): WorkspaceEntryData[] {
  if (!Array.isArray(value)) return []

  const entries = value.flatMap((candidate): WorkspaceEntryData[] => {
    if (!candidate || typeof candidate !== 'object') return []
    const record = candidate as Record<string, unknown>
    const kind = record.kind
    const name = typeof record.name === 'string' ? record.name.trim() : ''
    const path = typeof record.path === 'string' ? record.path : ''
    const absolutePath = typeof record.absolutePath === 'string' ? record.absolutePath : ''
    const revision = typeof record.revision === 'string' ? record.revision : ''
    if ((kind !== 'directory' && kind !== 'markdown' && kind !== 'image') || !name || !path) return []
    return [{ name, path, absolutePath, kind, revision }]
  })

  return entries.sort((left, right) => {
    if (left.kind !== right.kind) return workspaceEntryKindRank[left.kind] - workspaceEntryKindRank[right.kind]
    return left.name.localeCompare(right.name, undefined, { numeric: true, sensitivity: 'base' })
  })
}

export function normalizeWorkspace(value: unknown): WorkspaceData | null {
  if (!value || typeof value !== 'object') return null
  const record = value as Record<string, unknown>
  const id = typeof record.id === 'string' ? record.id.trim() : ''
  const provider = normalizeWorkspaceProvider(record.provider)
  const path = typeof record.path === 'string' ? record.path : ''
  const name = typeof record.name === 'string' ? record.name.trim() : ''
  if (!id || !provider || !path || !name) return null
  return {
    id,
    provider,
    path,
    name,
    entries: normalizeWorkspaceEntries(record.entries),
    truncated: record.truncated === true,
  }
}

export function normalizeWorkspaceDirectory(value: unknown): WorkspaceDirectoryData {
  if (Array.isArray(value)) {
    return { entries: normalizeWorkspaceEntries(value), truncated: false }
  }
  if (!value || typeof value !== 'object') return { entries: [], truncated: false }
  const record = value as Record<string, unknown>
  return {
    entries: normalizeWorkspaceEntries(record.entries),
    truncated: record.truncated === true,
  }
}

export function flattenWorkspaceTree(
  children: WorkspaceChildren,
  expandedDirectories: ReadonlySet<string>,
  loadingDirectories: ReadonlySet<string> = new Set(),
): WorkspaceTreeRow[] {
  const rows: WorkspaceTreeRow[] = []
  const activeAncestors = new Set<string>()

  const visit = (directoryPath: string, depth: number) => {
    const directoryKey = workspaceDirectoryKey(directoryPath)
    const entries = children[directoryKey] || []
    for (const entry of entries) {
      const entryKey = workspaceDirectoryKey(entry.path)
      const isDirectory = entry.kind === 'directory'
      const expanded = isDirectory && expandedDirectories.has(entryKey)
      const loaded = isDirectory && Object.hasOwn(children, entryKey)
      rows.push({
        entry,
        depth,
        expanded,
        loaded,
        loading: isDirectory && loadingDirectories.has(entryKey),
      })

      // A malicious or malformed provider response must not be able to create
      // an infinite tree by making a directory its own descendant.
      if (!expanded || !loaded || activeAncestors.has(entryKey)) continue
      activeAncestors.add(entryKey)
      visit(entry.path, depth + 1)
      activeAncestors.delete(entryKey)
    }
  }

  visit(workspaceRootKey, 0)
  return rows
}

export function sameWorkspaceFile(left: string, right: string): boolean {
  if (!left || !right) return false
  const normalize = (value: string) => value.replaceAll('\\', '/').replace(/\/+$/g, '')
  const normalizedLeft = normalize(left)
  const normalizedRight = normalize(right)
  if (/^[a-z]:\//i.test(normalizedLeft) || /^[a-z]:\//i.test(normalizedRight)) {
    return normalizedLeft.toLocaleLowerCase('en-US') === normalizedRight.toLocaleLowerCase('en-US')
  }
  return normalizedLeft === normalizedRight
}

export function workspaceJoinPath(parentPath: string, name: string): string {
  const parent = workspaceDirectoryKey(parentPath)
  const child = name.trim()
  return workspaceDirectoryKey(parent ? `${parent}/${child}` : child)
}

export function workspacePathIsWithin(candidatePath: string, ancestorPath: string): boolean {
  const candidate = workspaceDirectoryKey(candidatePath)
  const ancestor = workspaceDirectoryKey(ancestorPath)
  if (!candidate || !ancestor) return false
  return candidate === ancestor || candidate.startsWith(`${ancestor}/`)
}

export function rebaseWorkspacePath(candidatePath: string, oldPath: string, newPath: string): string {
  const candidate = workspaceDirectoryKey(candidatePath)
  const previous = workspaceDirectoryKey(oldPath)
  const replacement = workspaceDirectoryKey(newPath)
  if (!workspacePathIsWithin(candidate, previous) || !replacement) return candidate
  if (candidate === previous) return replacement
  return `${replacement}/${candidate.slice(previous.length + 1)}`
}

export function workspaceParentPath(path: string): string {
  return workspaceParentDirectoryKey(path)
}

export function localWorkspaceAbsolutePath(workspacePath: string, relativePath: string): string {
  const relative = workspaceDirectoryKey(relativePath)
  if (!workspacePath || !relative) return ''
  const windowsStyle = workspacePath.includes('\\') && !workspacePath.includes('/')
  const separator = windowsStyle ? '\\' : '/'
  const root = workspacePath.replace(/[\\/]+$/g, '')
  return `${root}${separator}${relative.replaceAll('/', separator)}`
}

export function isWorkspaceBackedLocalDocument(
  workspace: WorkspaceData | null | undefined,
  storageKind: string,
  documentWorkspaceID: string,
  relativePath: string,
): boolean {
  return Boolean(
    workspace?.provider === 'local'
    && storageKind === 'local'
    && documentWorkspaceID === workspace.id
    && workspaceDirectoryKey(relativePath),
  )
}

/**
 * Restores the relative identity used by the local workspace sidebar when a
 * document was opened through the file picker or recent-items menu.
 */
export function localWorkspaceRelativePath(workspaceRoot: string, documentPath: string): string {
  if (!workspaceRoot || !documentPath) return ''
  const normalizeAbsolute = (value: string) => {
    const normalized = value.replaceAll('\\', '/').replace(/\/+$/g, '')
    return normalized || (value.startsWith('/') ? '/' : '')
  }
  const root = normalizeAbsolute(workspaceRoot)
  const document = normalizeAbsolute(documentPath)
  if (!root || !document) return ''

  const windowsPath = /^[a-z]:(?:\/|$)/i.test(root) || root.startsWith('//')
  const comparableRoot = windowsPath ? root.toLocaleLowerCase('en-US') : root
  const comparableDocument = windowsPath ? document.toLocaleLowerCase('en-US') : document
  const prefix = comparableRoot === '/' ? '/' : `${comparableRoot}/`
  if (!comparableDocument.startsWith(prefix)) return ''

  const relative = document.slice(root === '/' ? 1 : root.length + 1)
  const segments = relative.split('/')
  if (!relative || segments.some((segment) => !segment || segment === '.' || segment === '..')) return ''
  return workspaceDirectoryKey(relative)
}

/**
 * Compares the stable identity of a file shown by a workspace provider.
 * Relative paths from different providers must never select each other, even
 * when their display names and directory layouts happen to be identical.
 */
export function isActiveWorkspaceFile(
  leftProvider: WorkspaceProvider | null | undefined,
  leftWorkspaceId: string,
  leftRelativePath: string,
  rightProvider: WorkspaceProvider | null | undefined,
  rightWorkspaceId: string,
  rightRelativePath: string,
): boolean {
  const normalizedLeftProvider = normalizeWorkspaceProvider(leftProvider)
  const normalizedRightProvider = normalizeWorkspaceProvider(rightProvider)
  if (!normalizedLeftProvider || normalizedLeftProvider !== normalizedRightProvider) return false
  if (!leftWorkspaceId || leftWorkspaceId !== rightWorkspaceId) return false

  const leftPath = workspaceDirectoryKey(leftRelativePath)
  const rightPath = workspaceDirectoryKey(rightRelativePath)
  return Boolean(leftPath && rightPath && leftPath === rightPath)
}

// Kept as a descriptive alias for callers that compare two workspace-backed
// document identities outside active-selection rendering.
export const sameWorkspaceDocument = isActiveWorkspaceFile
