export type WorkspaceEntryKind = 'directory' | 'markdown'

export interface WorkspaceEntryData {
  name: string
  path: string
  absolutePath: string
  kind: WorkspaceEntryKind
}

export interface WorkspaceData {
  id: string
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
    if ((kind !== 'directory' && kind !== 'markdown') || !name || !path) return []
    return [{ name, path, absolutePath, kind }]
  })

  return entries.sort((left, right) => {
    if (left.kind !== right.kind) return left.kind === 'directory' ? -1 : 1
    return left.name.localeCompare(right.name, undefined, { numeric: true, sensitivity: 'base' })
  })
}

export function normalizeWorkspace(value: unknown): WorkspaceData | null {
  if (!value || typeof value !== 'object') return null
  const record = value as Record<string, unknown>
  const id = typeof record.id === 'string' ? record.id.trim() : ''
  const path = typeof record.path === 'string' ? record.path : ''
  const name = typeof record.name === 'string' ? record.name.trim() : ''
  if (!id || !path || !name) return null
  return {
    id,
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
