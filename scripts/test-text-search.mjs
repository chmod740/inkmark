import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import {
  countTextMatches,
  findTextMatch,
  maximumSearchQueryLength,
  normalizeSearchQuery,
  segmentTextMatches,
} from '../frontend/src/text-search.ts'

test('literal text search counts and navigates with deterministic wrapping', () => {
  const source = 'Alpha beta ALPHA beta alpha'
  assert.equal(countTextMatches(source, 'alpha'), 3)
  assert.equal(countTextMatches(source, 'alpha', true), 1)
  assert.deepEqual(findTextMatch(source, 'alpha', 0, 1), { start: 0, end: 5, ordinal: 1, total: 3 })
  assert.deepEqual(findTextMatch(source, 'alpha', 5, 1), { start: 11, end: 16, ordinal: 2, total: 3 })
  assert.deepEqual(findTextMatch(source, 'alpha', source.length, 1), { start: 0, end: 5, ordinal: 1, total: 3 })
  assert.deepEqual(findTextMatch(source, 'alpha', 11, -1), { start: 0, end: 5, ordinal: 1, total: 3 })
  assert.deepEqual(findTextMatch(source, 'alpha', 0, -1), { start: 22, end: 27, ordinal: 3, total: 3 })
})

test('queries remain literal, bounded, Unicode-safe, and empty-safe', () => {
  assert.equal(countTextMatches('a.b a?b a.b', 'a.b'), 2)
  assert.deepEqual(findTextMatch('中文🙂中文', '中文', 1, 1), { start: 4, end: 6, ordinal: 2, total: 2 })
  assert.equal(findTextMatch('abc', '', 0, 1), null)
  assert.equal(normalizeSearchQuery('x'.repeat(maximumSearchQueryLength + 20)).length, maximumSearchQueryLength)
  assert.equal(normalizeSearchQuery({ toString: () => 'hostile' }), '')
})

test('highlight segments preserve source text, bound the DOM, and retain the current match', () => {
  const source = '<tag> Match & match\n' + 'match '.repeat(2_050)
  const currentStart = source.lastIndexOf('match')
  const segments = segmentTextMatches(source, 'match', currentStart)
  assert.equal(segments.map(({ text }) => text).join(''), source)
  assert.equal(segments.filter(({ highlighted }) => highlighted).length, 2_000)
  assert.equal(segments.filter(({ current }) => current).length, 1)
  assert.equal(segments.find(({ current }) => current)?.text, 'match')
  assert.equal(segments[0].text.startsWith('<tag> '), true)
})

test('application wires native and keyboard find into an accessible editor search bar', async () => {
  const app = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  const menu = await readFile(new URL('../menu.go', import.meta.url), 'utf8')
  assert.match(menu, /addEditItem\("find", keys\.CmdOrCtrl\("f"\), "find"\)/)
  assert.match(app, /key === 'f'[\s\S]*event\.preventDefault\(\)[\s\S]*showFindBar\(\)/)
  assert.match(app, /action === 'find'[\s\S]*showFindBar\(\)/)
  assert.match(app, /class="find-bar"[\s\S]*role="search"/)
  assert.match(app, /@keydown\.enter\.prevent="findNext\(\$event\.shiftKey \? -1 : 1\)"/)
  assert.match(app, /@keydown\.escape\.prevent="closeFindBar"/)
  assert.match(app, /target\.setSelectionRange\(match\.start, match\.end\)/)
  assert.match(app, /class="source-find-highlights"[\s\S]*find-highlight-current/)
  assert.match(app, /currentHighlight\.offsetTop[\s\S]*beginScroll\('editor'\)[\s\S]*target\.scrollTop[\s\S]*syncFromEditor\(\)/)
})
