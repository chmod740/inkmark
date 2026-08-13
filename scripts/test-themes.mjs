import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import {
  hexColorChannels,
  isDarkTheme,
  normalizeTheme,
  themeBackground,
  themeDefinitions,
  themeIds,
  themePalette,
} from '../frontend/src/themes.ts'
import { buildStandaloneHTML, externalExportStyles } from '../frontend/src/export-document.ts'

const expectedNewThemes = ['mist', 'paper', 'pine', 'sakura', 'ocean', 'indigo', 'nord', 'obsidian']

test('theme registry contains the four existing and eight new unique themes', () => {
  assert.equal(themeIds.length, 12)
  assert.equal(new Set(themeIds).size, 12)
  assert.deepEqual(themeIds.slice(4), expectedNewThemes)
  assert.deepEqual(themeDefinitions.map((theme) => theme.id), [...themeIds])
  for (const definition of themeDefinitions) {
    assert.match(definition.labelKey, /^theme\./)
    for (const color of Object.values(definition.palette)) assert.match(color, /^#[0-9a-f]{6}$/i)
  }
})

test('persisted theme values are allow-listed and dark themes are explicit', () => {
  assert.equal(normalizeTheme('paper'), 'paper')
  assert.equal(normalizeTheme('obsidian'), 'obsidian')
  assert.equal(normalizeTheme('theme-dark; color:red'), 'github')
  assert.equal(normalizeTheme(null), 'github')
  assert.deepEqual(themeIds.filter(isDarkTheme), ['dark', 'indigo', 'nord', 'obsidian'])
})

test('export backgrounds and chart palettes are available for every theme', () => {
  for (const theme of themeIds) {
    assert.equal(themeBackground(theme), themePalette(theme).background)
    assert.deepEqual(hexColorChannels(themeBackground(theme)).length, 3)
    const html = buildStandaloneHTML({
      title: theme,
      theme,
      articleHTML: '<h1>Theme</h1>',
      embeddedStyles: '',
    })
    assert.match(html, new RegExp(`body class="export-document theme-${theme}"`))
    assert.match(html, new RegExp(`data-color-scheme="${isDarkTheme(theme) ? 'dark' : 'light'}"`))
    assert.ok(html.includes(isDarkTheme(theme) ? externalExportStyles.highlightDark : externalExportStyles.highlightLight))
  }
})

test('all eight palettes are wired into the app, menu, translations, and CSS', async () => {
  const [app, styles, menu, chineseAndEnglish] = await Promise.all([
    readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8'),
    readFile(new URL('../frontend/src/styles.css', import.meta.url), 'utf8'),
    readFile(new URL('../menu.go', import.meta.url), 'utf8'),
    readFile(new URL('../frontend/src/i18n.ts', import.meta.url), 'utf8'),
  ])
  assert.match(app, /themeDefinitions\.map/)
  assert.match(app, /isDarkTheme\(renderTheme\)/)
  assert.match(app, /palette:\s*themePalette\(renderTheme\)/)
  assert.match(app, /<select[^>]*:value="theme"/)
  assert.match(app, /<select[^>]*:aria-label="t\('toolbar\.previewStyle'\)"/)
  assert.doesNotMatch(app, /type Theme = 'github'/)
  for (const theme of expectedNewThemes) {
    assert.match(styles, new RegExp(`\\.theme-${theme}\\s*\\{`))
    assert.match(menu, new RegExp(`theme-${theme}`))
    assert.match(chineseAndEnglish, new RegExp(`'theme\\.${theme}'`, 'g'))
  }
})

test('candidate themes style application chrome and rendered Markdown surfaces', async () => {
  const styles = await readFile(new URL('../frontend/src/styles.css', import.meta.url), 'utf8')
  for (const theme of themeDefinitions.slice(4)) {
    const block = styles.match(new RegExp(`\\.theme-${theme.id}\\s*\\{([\\s\\S]*?)\\n\\}`))?.[1] || ''
    assert.ok(block.includes(`--theme-paper: ${theme.palette.background}`), `${theme.id} paper color drifted`)
    assert.ok(block.includes(`--theme-ink: ${theme.palette.ink}`), `${theme.id} ink color drifted`)
    assert.ok(block.includes(`--theme-muted: ${theme.palette.muted}`), `${theme.id} muted color drifted`)
    assert.ok(block.includes(`--theme-accent: ${theme.palette.accent}`), `${theme.id} accent color drifted`)
    assert.ok(block.includes(`--theme-accent-2: ${theme.palette.secondary}`), `${theme.id} secondary color drifted`)
    assert.ok(block.includes(`--theme-border: ${theme.palette.border}`), `${theme.id} border color drifted`)
    assert.ok(block.includes(`--theme-code: ${theme.palette.code}`), `${theme.id} code color drifted`)
  }
  for (const token of [
    '.document-header', '.command-bar', '.workspace-sidebar', '.source-editor', '.preview-viewport', '.status-bar',
    '.markdown-body h2', '.markdown-body blockquote', '.markdown-body pre', '.markdown-alert',
    '.markdown-body th', '.markdown-body pre.mermaid', '.markdown-body pre.extended-diagram-rendered',
  ]) assert.ok(styles.includes(token), `missing themed surface ${token}`)
  assert.match(styles, /body\.export-document:is\([^}]+background:\s*var\(--theme-preview\)/s)
})

test('native select popups remain readable in Windows light and dark color schemes', async () => {
  const styles = await readFile(new URL('../frontend/src/styles.css', import.meta.url), 'utf8')
  assert.match(styles, /:root\s*\{[^}]*color-scheme:\s*light;/s)
  assert.match(styles, /button,\s*textarea,\s*input,\s*select\s*\{\s*font:\s*inherit;/)
  assert.match(styles, /select option\s*\{[^}]*color:\s*#212826;[^}]*background-color:\s*#fff;/s)
  assert.match(styles, /\[data-color-scheme="dark"\]\s*\{\s*color-scheme:\s*dark;\s*\}/)
  assert.match(styles, /\[data-color-scheme="dark"\] select option\s*\{[^}]*color:\s*#e6efec;[^}]*background-color:\s*#111718;/s)
})
