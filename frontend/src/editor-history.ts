export interface EditorSelection {
  start: number
  end: number
}

export interface EditorHistoryEdit {
  start: number
  deleted: string
  inserted: string
  selectionBefore: EditorSelection
  selectionAfter: EditorSelection
}

export interface EditorHistory {
  entries: EditorHistoryEdit[]
  index: number
  bytes: number
}

export type EditorHistoryDirection = 'undo' | 'redo'

export interface EditorHistoryMove {
  edit: EditorHistoryEdit
  direction: EditorHistoryDirection
  history: EditorHistory
}

const maximumEditorHistoryEntries = 256
const maximumEditorHistoryBytes = 4 * 1024 * 1024

export function createEditorHistory(): EditorHistory {
  return { entries: [], index: 0, bytes: 0 }
}

export function appendEditorHistory(history: EditorHistory, edit: EditorHistoryEdit): EditorHistory {
  if (!edit.deleted && !edit.inserted) return history
  const entries = history.entries.slice(0, history.index)
  entries.push(edit)
  let bytes = entries.reduce((total, entry) => total + historyEntryBytes(entry), 0)
  while (entries.length > 1 && (entries.length > maximumEditorHistoryEntries || bytes > maximumEditorHistoryBytes)) {
    const removed = entries.shift()
    if (removed) bytes -= historyEntryBytes(removed)
  }
  return { entries, index: entries.length, bytes }
}

export function moveEditorHistory(history: EditorHistory, direction: EditorHistoryDirection): EditorHistoryMove | null {
  if (direction === 'undo') {
    if (history.index <= 0) return null
    return {
      edit: history.entries[history.index - 1],
      direction,
      history: { ...history, index: history.index - 1 },
    }
  }
  if (history.index >= history.entries.length) return null
  return {
    edit: history.entries[history.index],
    direction,
    history: { ...history, index: history.index + 1 },
  }
}

export function replaceEditorHistoryRange(
  source: string,
  edit: EditorHistoryEdit,
  direction: EditorHistoryDirection,
): { value: string; selection: EditorSelection } | null {
  const expected = direction === 'undo' ? edit.inserted : edit.deleted
  const replacement = direction === 'undo' ? edit.deleted : edit.inserted
  if (edit.start < 0 || source.slice(edit.start, edit.start + expected.length) !== expected) return null
  return {
    value: source.slice(0, edit.start) + replacement + source.slice(edit.start + expected.length),
    selection: direction === 'undo' ? edit.selectionBefore : edit.selectionAfter,
  }
}

function historyEntryBytes(entry: EditorHistoryEdit) {
  return entry.deleted.length + entry.inserted.length
}
