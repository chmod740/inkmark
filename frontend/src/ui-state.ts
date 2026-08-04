export type BuiltInDocumentKind = 'welcome' | 'render-test'
export type DocumentStatus = 'modified' | 'saved' | 'unsaved' | 'built-in'
export type DocumentLocation = 'path' | 'unsaved' | BuiltInDocumentKind

export const previewFirstStorageKey = 'inkmark-preview-first'

export function normalizePreviewFirst(value: unknown): boolean {
  return value === true || value === 'true'
}

export function togglePreviewFirst(value: boolean): boolean {
  return !value
}

export function resolveDocumentHeaderState(
  dirty: boolean,
  path: string,
  builtIn: BuiltInDocumentKind | null,
): { status: DocumentStatus; location: DocumentLocation } {
  const normalizedPath = path.trim()
  const status: DocumentStatus = dirty
    ? 'modified'
    : normalizedPath
      ? 'saved'
      : builtIn
        ? 'built-in'
        : 'unsaved'
  return {
    status,
    location: normalizedPath ? 'path' : builtIn || 'unsaved',
  }
}
