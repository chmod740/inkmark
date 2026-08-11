export interface SavedWebDAVConnection {
  id: string
  name: string
  endpoint: string
  username: string
  hasCredentials: boolean
  credentialsAvailable: boolean
  usernamePresent: boolean
}

export interface WebDAVConnectionInput {
  id: string
  name: string
  endpoint: string
  username: string
  password: string
  replaceCredentials: boolean
  removeCredentials: boolean
}

export interface RecentWebDAVMetadata {
  endpoint: string
  name: string
  savedConnectionId?: string
  hasSavedCredentials?: boolean
}

export type RecentWebDAVAction =
  | { kind: 'connect-saved'; connectionID: string; endpoint: string }
  | { kind: 'prompt'; endpoint: string }

export type RecentWebDAVOpenResult =
  | { kind: 'connected'; endpoint: string }
  | { kind: 'prompt'; endpoint: string; error?: unknown }

export interface ClearedWebDAVConnectionForm {
  endpoint: string
  name: string
  username: string
  password: string
  mode: 'connect'
  editingConnectionID: string
  storeCredentials: false
  removeCredentials: false
  showPassword: false
  error: string
}

interface SavedConnectionForm {
  mode: 'new' | 'edit'
  id?: string
  name: string
  endpoint: string
  username: string
  password: string
  storeCredentials: boolean
  removeCredentials: boolean
  existing?: SavedWebDAVConnection | null
}

export class SavedWebDAVFormError extends Error {
  readonly code: 'name_required' | 'endpoint_required' | 'username_requires_password' | 'origin_requires_password'

  constructor(code: SavedWebDAVFormError['code']) {
    super(code)
    this.code = code
  }
}

export function recentWebDAVAction(metadata: RecentWebDAVMetadata): RecentWebDAVAction {
  const endpoint = metadata.endpoint?.trim() || ''
  const connectionID = metadata.savedConnectionId?.trim() || ''
  if (metadata.hasSavedCredentials && connectionID) {
    return { kind: 'connect-saved', connectionID, endpoint }
  }
  return { kind: 'prompt', endpoint }
}

export async function resolveRecentWebDAVOpen(
  metadata: RecentWebDAVMetadata,
  connectSaved: (connectionID: string) => Promise<void>,
): Promise<RecentWebDAVOpenResult> {
  const action = recentWebDAVAction(metadata)
  if (action.kind === 'prompt') return action
  try {
    await connectSaved(action.connectionID)
    return { kind: 'connected', endpoint: action.endpoint }
  } catch (error) {
    return { kind: 'prompt', endpoint: action.endpoint, error }
  }
}

export function clearedWebDAVConnectionForm(endpoint = ''): ClearedWebDAVConnectionForm {
  return {
    endpoint: endpoint.trim(),
    name: '',
    username: '',
    password: '',
    mode: 'connect',
    editingConnectionID: '',
    storeCredentials: false,
    removeCredentials: false,
    showPassword: false,
    error: '',
  }
}

export function webDAVOriginChanged(previousEndpoint: string, nextEndpoint: string): boolean {
  const previousOrigin = webDAVOrigin(previousEndpoint)
  const nextOrigin = webDAVOrigin(nextEndpoint)
  return Boolean(previousOrigin && nextOrigin && previousOrigin !== nextOrigin)
}

export function buildSavedWebDAVConnectionInput(form: SavedConnectionForm): WebDAVConnectionInput {
  const name = form.name.trim()
  const endpoint = form.endpoint.trim()
  const username = form.username.trim()
  const password = form.password
  if (!name) throw new SavedWebDAVFormError('name_required')
  if (!endpoint) throw new SavedWebDAVFormError('endpoint_required')

  if (form.mode === 'new') {
    return {
      id: '',
      name,
      endpoint,
      username: form.storeCredentials ? username : '',
      password: form.storeCredentials ? password : '',
      replaceCredentials: form.storeCredentials,
      removeCredentials: false,
    }
  }

  const existing = form.existing
  if (
    existing?.hasCredentials
    && !form.removeCredentials
    && !password
    && webDAVOriginChanged(existing.endpoint, endpoint)
  ) {
    throw new SavedWebDAVFormError('origin_requires_password')
  }
  if (!form.removeCredentials && !password && username !== (existing?.username || '')) {
    throw new SavedWebDAVFormError('username_requires_password')
  }
  const replaceCredentials = !form.removeCredentials && Boolean(password)
  return {
    id: form.id?.trim() || '',
    name,
    endpoint,
    username: replaceCredentials ? username : '',
    password: replaceCredentials ? password : '',
    replaceCredentials,
    removeCredentials: form.removeCredentials,
  }
}

function webDAVOrigin(endpoint: string): string {
  try {
    const parsed = new URL(endpoint.trim())
    if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') return ''
    if (parsed.username || parsed.password || !parsed.hostname) return ''
    return parsed.origin
  } catch {
    return ''
  }
}

export function normalizeSavedWebDAVConnections(value: unknown): SavedWebDAVConnection[] {
  if (!Array.isArray(value)) return []
  return value.flatMap((candidate) => {
    if (!candidate || typeof candidate !== 'object') return []
    const entry = candidate as Partial<SavedWebDAVConnection>
    const id = entry.id?.trim() || ''
    const endpoint = entry.endpoint?.trim() || ''
    if (!id || !endpoint) return []
    return [{
      id,
      name: entry.name?.trim() || endpoint,
      endpoint,
      username: entry.username?.trim() || '',
      hasCredentials: Boolean(entry.hasCredentials),
      credentialsAvailable: Boolean(entry.credentialsAvailable),
      usernamePresent: Boolean(entry.usernamePresent),
    }]
  })
}
