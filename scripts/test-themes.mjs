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

const expectedNewThemes = ['mist', 'paper', 'autumn', 'pine', 'sakura', 'ocean', 'indigo', 'nord', 'obsidian']

test('theme registry contains the four original and nine complete palette themes', () => {
  assert.equal(themeIds.length, 13)
  assert.equal(new Set(themeIds).size, 13)
  assert.deepEqual(themeIds.slice(4), expectedNewThemes)
  assert.deepEqual(themeDefinitions.map((theme) => theme.id), [...themeIds])
  for (const definition of themeDefinitions) {
    assert.match(definition.labelKey, /^theme\./)
    for (const color of Object.values(definition.palette)) assert.match(color, /^#[0-9a-f]{6}$/i)
  }
})

test('persisted theme values are allow-listed and dark themes are explicit', () => {
  assert.equal(normalizeTheme('paper'), 'paper')
  assert.equal(normalizeTheme('autumn'), 'autumn')
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

test('all complete palettes are wired into the app, menu, translations, and CSS', async () => {
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

test('autumn paper exposes the audited reading typography without changing font preferences', async () => {
  const [styles, fontPreferences] = await Promise.all([
    readFile(new URL('../frontend/src/styles.css', import.meta.url), 'utf8'),
    readFile(new URL('../frontend/src/font-preferences.ts', import.meta.url), 'utf8'),
  ])
  assert.match(styles, /\.theme-autumn\s*\{[\s\S]*--theme-paper:\s*#f7f5f1;[\s\S]*--theme-accent:\s*#64272d;[\s\S]*--theme-reading-font:[^;]*LXGW WenKai Screen[^;]*Kaiti SC[^;]*var\(--inkmark-content-font\)/)
  const bodyBlock = styles.match(/\.theme-autumn \.markdown-body\s*\{([\s\S]*?)\n\}/)?.[1] || ''
  assert.match(bodyBlock, /max-width:\s*832px/)
  assert.match(bodyBlock, /padding:\s*40px clamp\(28px, 7\.7%, 64px\) 110px/)
  assert.match(bodyBlock, /radial-gradient\([\s\S]*background-size:\s*4px 4px/s)
  assert.match(bodyBlock, /font-family:\s*var\(--theme-reading-font\)/)
  assert.match(bodyBlock, /font-size:\s*16px/)
  assert.match(bodyBlock, /line-height:\s*1\.75/)
  const headingBlock = styles.match(/\.theme-autumn \.markdown-body h1\s*\{([\s\S]*?)\n\}/)?.[1] || ''
  assert.match(headingBlock, /font-size:\s*22px/)
  assert.match(headingBlock, /line-height:\s*32px/)
  assert.match(headingBlock, /border:\s*0/)
  assert.match(styles, /\.theme-autumn \.markdown-body li::marker\s*\{\s*color:\s*var\(--theme-accent\)/)
  assert.match(styles, /\.theme-autumn \.markdown-body blockquote\s*\{[\s\S]*border-left:\s*3px solid var\(--theme-accent\)/s)
  assert.match(styles, /\.theme-autumn \.markdown-body pre[\s\S]*border-radius:\s*10px/s)
  assert.doesNotMatch(fontPreferences, /theme-autumn|autumn/i)
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
