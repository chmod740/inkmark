import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import {
  appendEditorHistory,
  createEditorHistory,
  moveEditorHistory,
  replaceEditorHistoryRange,
} from '../frontend/src/editor-history.ts'

function edit(start, deleted, inserted, before = start, after = start + inserted.length) {
  return {
    start,
    deleted,
    inserted,
    selectionBefore: { start: before, end: before },
    selectionAfter: { start: after, end: after },
  }
}

test('editor history replays Tab and ordinary typing without copying document snapshots', () => {
  let history = createEditorHistory()
  history = appendEditorHistory(history, edit(0, '', 'A'))
  history = appendEditorHistory(history, edit(1, '', '\t'))
  history = appendEditorHistory(history, edit(2, '', 'B'))

  let source = 'A\tB'
  for (const expected of ['A\t', 'A', '']) {
    const move = moveEditorHistory(history, 'undo')
    assert.ok(move)
    const replacement = replaceEditorHistoryRange(source, move.edit, move.direction)
    assert.ok(replacement)
    history = move.history
    source = replacement.value
    assert.equal(source, expected)
  }
  assert.equal(moveEditorHistory(history, 'undo'), null)

  for (const expected of ['A', 'A\t', 'A\tB']) {
    const move = moveEditorHistory(history, 'redo')
    assert.ok(move)
    const replacement = replaceEditorHistoryRange(source, move.edit, move.direction)
    assert.ok(replacement)
    history = move.history
    source = replacement.value
    assert.equal(source, expected)
  }
})

test('a new edit discards stale redo and malformed replay fails closed', () => {
  let history = appendEditorHistory(createEditorHistory(), edit(0, '', 'A'))
  history = appendEditorHistory(history, edit(1, '', 'B'))
  const undo = moveEditorHistory(history, 'undo')
  assert.ok(undo)
  history = undo.history
  history = appendEditorHistory(history, edit(1, '', 'C'))
  assert.equal(moveEditorHistory(history, 'redo'), null)
  assert.equal(replaceEditorHistoryRange('wrong', history.entries[0], 'undo'), null)
})

test('the source editor owns Tab and history shortcuts while modal inputs retain their native path', async () => {
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  assert.match(app, /@beforeinput="handleEditorBeforeInput"[\s\S]*@input="handleEditorInput"[\s\S]*@keydown="handleEditorKeydown"/)
  assert.match(app, /function handleEditorKeydown\(event: KeyboardEvent\)[\s\S]*event\.key === 'Tab'[\s\S]*replaceEditorRange\(target, start, end, '\\t'\)[\s\S]*key === 'z'[\s\S]*applyEditorHistory\(direction\)/)
  assert.match(app, /action === 'undo' \|\| action === 'redo'[\s\S]*sourceEditorTarget\) applyEditorHistory\(action as EditorHistoryDirection\)[\s\S]*document\.execCommand\(action\)/)
  assert.match(app, /action === 'cut'[\s\S]*sourceEditorTarget\) replaceEditorRange\(target, start, end, ''\)/)
  assert.match(app, /action === 'paste'[\s\S]*sourceEditorTarget\) replaceEditorRange\(target, start, end, text\)/)
  assert.match(app, /const editorHistories = new Map<string, EditorHistory>\(\)/)
})
