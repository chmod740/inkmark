import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import {
  getSystemLanguages,
  getWelcomeDocument,
  languageModeStorageKey,
  normalizeLanguageMode,
  resolveLocale,
  translate,
  translations,
  welcomeDocument,
} from '../frontend/src/i18n.ts'

test('language modes are normalised without trusting persisted values', () => {
  assert.equal(normalizeLanguageMode('auto'), 'auto')
  assert.equal(normalizeLanguageMode('system'), 'auto')
  assert.equal(normalizeLanguageMode(' zh_CN '), 'zh-CN')
  assert.equal(normalizeLanguageMode('zh-Hans-CN'), 'zh-CN')
  assert.equal(normalizeLanguageMode('en-US'), 'en')
  assert.equal(normalizeLanguageMode('fr-FR'), 'auto')
  assert.equal(normalizeLanguageMode(null), 'auto')
  assert.equal(languageModeStorageKey, 'inkmark-language')
})

test('automatic locale detection follows the primary system language', () => {
  assert.equal(resolveLocale('auto', ['zh-CN', 'en-US']), 'zh-CN')
  assert.equal(resolveLocale('auto', 'zh_Hant_TW'), 'zh-CN')
  assert.equal(resolveLocale('auto', ['en-GB', 'zh-CN']), 'en')
  assert.equal(resolveLocale('auto', ['fr-FR']), 'en')
  assert.equal(resolveLocale('auto', []), 'en')
})

test('manual language modes override the detected system language', () => {
  assert.equal(resolveLocale('zh-CN', ['en-US']), 'zh-CN')
  assert.equal(resolveLocale('en', ['zh-CN']), 'en')
  assert.equal(resolveLocale('unexpected', ['zh-CN']), 'zh-CN')
})

test('system language lookup is safe in browser-like and headless runtimes', () => {
  const languages = getSystemLanguages()
  assert.ok(Array.isArray(languages))
  assert.ok(languages.every((language) => typeof language === 'string' && language.length > 0))
  assert.ok(resolveLocale('auto', languages) === 'zh-CN' || resolveLocale('auto', languages) === 'en')
})

test('Chinese and English dictionaries expose exactly the same keys', () => {
  const chineseKeys = Object.keys(translations['zh-CN']).sort()
  const englishKeys = Object.keys(translations.en).sort()
  assert.deepEqual(englishKeys, chineseKeys)
  assert.ok(chineseKeys.length >= 100)
  for (const locale of ['zh-CN', 'en']) {
    for (const key of chineseKeys) {
      assert.notEqual(translations[locale][key].trim(), '', `${locale}.${key} must not be empty`)
    }
  }
})

test('translation parameters are interpolated and unknown parameters remain visible', () => {
  assert.equal(
    translate('zh-CN', 'export.completed', { format: 'PDF 文档', name: '报告.pdf' }),
    'PDF 文档已导出：报告.pdf',
  )
  assert.equal(
    translate('en', 'error.markdownRenderFailed', { message: 'bad token' }),
    'Markdown rendering failed: bad token',
  )
  assert.equal(translate('en', 'status.characters'), '{count} characters')
})

test('about and update messages are complete in both languages', () => {
  const keys = [
    'help.version',
    'help.author',
    'help.sourceRepository',
    'help.repositoryUnavailable',
    'help.updateStatus',
    'help.checkUpdate',
    'help.downloadUpdate',
    'help.downloadAndInstall',
    'help.updateNotChecked',
    'help.updateCurrent',
    'help.updateAvailable',
    'help.updateUnavailable',
    'help.updateFailed',
    'menu.checkUpdate',
  ]
  for (const locale of ['zh-CN', 'en']) {
    for (const key of keys) assert.ok(translations[locale][key]?.trim(), `${locale}.${key} must exist`)
  }
  assert.equal(
    translate('zh-CN', 'help.updateAvailable', { version: '1.2.0' }),
    '发现新版本 1.2.0',
  )
  assert.equal(
    translate('en', 'help.updateCurrent', { version: '1.0.0' }),
    'You are using the latest version (1.0.0)',
  )
  assert.equal(translate('zh-CN', 'help.downloadAndInstall'), '下载并安装新版本')
  assert.equal(translate('en', 'help.downloadAndInstall'), 'Download and Install New Version')
})

test('WebDAV connection, remote save, conflict, and disconnect messages are complete', () => {
  const keys = [
    'app.localMode',
    'app.webdavConnected',
    'menu.connectWebDAV',
    'workspace.providerWebDAV',
    'webdav.title',
    'webdav.serverURL',
    'webdav.username',
    'webdav.password',
    'webdav.connect',
    'webdav.connecting',
    'webdav.connectedBadge',
    'webdav.endpointRequired',
    'webdav.invalidResponse',
    'webdav.authenticationFailed',
    'webdav.permissionDenied',
    'webdav.networkError',
    'webdav.timeout',
    'webdav.unsupported',
    'webdav.locked',
    'webdav.canceled',
    'webdav.invalidRequest',
    'webdav.tooLarge',
    'webdav.rateLimited',
    'webdav.serverError',
    'webdav.operationFailed',
    'webdav.connectionFailed',
    'webdav.remoteSave',
    'webdav.savedRemotely',
    'webdav.remoteSaveFailed',
    'webdav.conflictTitle',
    'webdav.conflictMessage',
    'webdav.conflictReload',
    'webdav.conflictOverwrite',
    'webdav.disconnectTitle',
    'webdav.disconnectMessage',
    'webdav.disconnectConfirm',
  ]
  for (const locale of ['zh-CN', 'en']) {
    for (const key of keys) assert.ok(translations[locale][key]?.trim(), `${locale}.${key} must exist`)
  }
  assert.equal(translate('zh-CN', 'app.localMode'), '本地模式')
  assert.equal(translate('en', 'app.webdavConnected'), 'WebDAV Connected')
  assert.match(
    translate('en', 'webdav.conflictMessage', { name: 'README.md' }),
    /README\.md.*modified by another client/,
  )
})

test('about dialog wires application metadata and native update menu actions', async () => {
  const appSource = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  for (const bridgeMethod of ['GetAppInfo', 'CheckForUpdates', 'OpenUpdatePage', 'OpenSourceRepository']) {
    assert.ok(appSource.includes(bridgeMethod), `${bridgeMethod} must be wired into App.vue`)
  }
  assert.ok(appSource.includes("action === 'check-update'"))
  assert.ok(!appSource.includes("action === 'upgrade'"))
  assert.ok(appSource.includes('aria-live="polite"'))
})

test('welcome documents are complete, localised README files', () => {
  assert.equal(welcomeDocument['zh-CN'].name, 'README.md')
  assert.equal(welcomeDocument.en.name, 'README.md')
  assert.match(welcomeDocument['zh-CN'].content, /^# 墨笺 Markdown/m)
  assert.match(welcomeDocument['zh-CN'].content, /## 开始使用/)
  assert.match(welcomeDocument['zh-CN'].content, /#inkmark-render-test/)
  assert.match(welcomeDocument.en.content, /^# InkMark Markdown/m)
  assert.match(welcomeDocument.en.content, /## Get started/)
  assert.match(welcomeDocument.en.content, /#inkmark-render-test/)
  assert.ok(!welcomeDocument['zh-CN'].content.includes('文件操作已收纳'))
  assert.ok(!welcomeDocument.en.content.includes('powered by'))

  const copy = getWelcomeDocument('zh-CN')
  copy.content = 'changed'
  assert.notEqual(copy.content, welcomeDocument['zh-CN'].content)
})
