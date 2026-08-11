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
    'help.thirdPartyLicenses',
    'help.thirdPartyLicensesDescription',
    'help.thirdPartyLicensesOpen',
    'help.thirdPartyLicensesLoading',
    'help.thirdPartyLicensesUnavailable',
    'help.thirdPartyLicensesContent',
    'help.thirdPartyLicensesBack',
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
  assert.match(translate('zh-CN', 'help.thirdPartyLicensesDescription'), /仅适用于各自的第三方组件/)
  assert.match(translate('en', 'help.thirdPartyLicensesDescription'), /apply only to their respective third-party components/)
})

test('WebDAV connection, remote save, conflict, and disconnect messages are complete', () => {
  const keys = [
    'app.localMode',
    'app.webdavConnected',
    'menu.connectWebDAV',
    'menu.openRecent',
    'menu.clearRecent',
    'workspace.providerWebDAV',
    'webdav.title',
    'webdav.serverURL',
    'webdav.username',
    'webdav.password',
    'webdav.passwordNotStored',
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
  assert.match(translate('zh-CN', 'webdav.passwordNotStored'), /完整服务器地址.*包括路径.*认证用户名和密码.*当前连接/)
  assert.match(translate('en', 'webdav.passwordNotStored'), /complete server address.*including its path.*authentication usernames and passwords.*only.*(?:this|the current) connection/i)
  assert.match(
    translate('en', 'webdav.conflictMessage', { name: 'README.md' }),
    /README\.md.*modified by another client/,
  )
})

test('image import, rendering, and export messages are complete in both languages', () => {
  const keys = [
    'image.insert',
    'image.title',
    'image.modeLocal',
    'image.modeData',
    'image.modeWebDAV',
    'image.modePublic',
    'image.publicURLHint',
    'image.fileHint',
    'image.localHint',
    'image.webDAVHint',
    'image.dataHint',
    'image.publicHint',
    'image.processing',
    'image.insertFailed',
    'image.loadFailed',
    'image.unsupportedSource',
  ]
  for (const locale of ['zh-CN', 'en']) {
    for (const key of keys) assert.ok(translations[locale][key]?.trim(), `${locale}.${key} must exist`)
  }
  assert.match(translate('zh-CN', 'image.fileHint'), /PNG.*JPEG.*GIF.*WebP.*16 MiB/)
  assert.match(translate('en', 'image.publicURLHint'), /HTTPS.*HTML exports preserve/i)
})

test('saved WebDAV connection management explains system credential storage in both languages', () => {
  const keys = [
    'webdav.savedConnections',
    'webdav.newSavedConnection',
    'webdav.noSavedConnections',
    'webdav.savedUsername',
    'webdav.savedCredentialsUnavailable',
    'webdav.deleteConnectionTitle',
    'webdav.deleteConnectionMessage',
    'webdav.temporaryConnection',
    'webdav.newConnectionTitle',
    'webdav.editConnectionTitle',
    'webdav.systemCredentialStore',
    'webdav.passwordKeepPlaceholder',
    'webdav.editPasswordHint',
    'webdav.saveCredentialsSecurely',
    'webdav.removeSavedCredentials',
    'webdav.savedConnectFailed',
    'webdav.endpointChangeNeedsPassword',
    'webdav.credentialStoreUnavailable',
    'webdav.localConnectionStorageUnavailable',
  ]
  for (const locale of ['zh-CN', 'en']) {
    for (const key of keys) assert.ok(translations[locale][key]?.trim(), `${locale}.${key} must exist`)
  }
  assert.match(translate('zh-CN', 'webdav.systemCredentialStore'), /macOS 钥匙串.*Windows 凭据管理器.*不写入设置文件/)
  assert.match(translate('en', 'webdav.systemCredentialStore'), /macOS Keychain.*Windows Credential Manager.*never.*settings file/i)
  assert.match(translate('zh-CN', 'webdav.credentialStoreUnavailable'), /系统凭据库.*macOS 钥匙串.*Windows 凭据管理器/)
  assert.match(translate('en', 'webdav.credentialStoreUnavailable'), /system credential store.*macOS Keychain.*Windows Credential Manager/i)
})

test('about dialog wires application metadata and native update menu actions', async () => {
  const appSource = await readFile(new URL('../frontend/src/App.vue', import.meta.url), 'utf8')
  for (const bridgeMethod of ['GetAppInfo', 'GetThirdPartyNotices', 'CheckForUpdates', 'OpenUpdatePage', 'OpenSourceRepository']) {
    assert.ok(appSource.includes(bridgeMethod), `${bridgeMethod} must be wired into App.vue`)
  }
  assert.ok(appSource.includes("action === 'check-update'"))
  assert.ok(!appSource.includes("action === 'upgrade'"))
  assert.ok(appSource.includes('aria-live="polite"'))
  assert.match(appSource, /class="third-party-notices"[\s\S]*readonly/)
  assert.ok(appSource.includes("aboutView === 'third-party'"))
  assert.ok(appSource.includes('@click="showAboutOverview"'))
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
