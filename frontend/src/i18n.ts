export type LanguageMode = 'auto' | 'zh-CN' | 'en'
export type Locale = Exclude<LanguageMode, 'auto'>

export const languageModeStorageKey = 'inkmark-language'

const zhCNTranslations = {
  'app.name': '墨笺',
  'app.fullName': '墨笺 Markdown',
  'app.description': '本地 Markdown 编辑器',
  'app.offline': '本地离线',

  'document.untitled': '未命名',
  'document.untitledFilename': '未命名.md',
  'document.unsavedLocation': '尚未保存到磁盘',
  'document.modified': '已修改',
  'document.saved': '已保存',
  'document.opened': '文档已打开',
  'document.created': '已新建空白文档',
  'document.savedLocally': '已保存到本地',
  'document.savedAsLocally': '已另存到本地',
  'document.dirtyTitle': '有未保存的修改',

  'confirm.newUnsaved': '当前文档尚未保存，仍要新建文档吗？',
  'confirm.openUnsaved': '当前文档尚未保存，仍要打开其他文件吗？',
  'confirm.quitUnsaved': '当前文档尚未保存，仍要退出墨笺吗？',

  'status.ready': '准备就绪',
  'status.waiting': '等待输入…',
  'status.rendering': '渲染中…',
  'status.rendered': '实时预览已更新',
  'status.renderFailed': '渲染失败',
  'status.htmlCopied': '渲染 HTML 已复制',
  'status.copyFailed': '复制失败',
  'status.characters': '{count} 字',
  'status.lines': '{count} 行',

  'error.previewInterrupted': '预览更新被中断，请重新导出',
  'error.previewUnavailable': '预览区域尚未就绪',
  'error.documentChangedDuringExport': '文档在导出期间发生变化，请重新导出',
  'error.documentTooLong': '文档过长，无法安全生成画布；请改用 HTML 或纯文本导出',
  'error.pdfCanvasUnavailable': '无法创建 PDF 页面画布',
  'error.markdownRenderFailed': 'Markdown 渲染失败：{message}',

  'theme.github': 'GitHub',
  'theme.clean': '清爽',
  'theme.wechat': '公众号',
  'theme.dark': '深色',

  'format.bold': '粗体',
  'format.italic': '斜体',
  'format.strikethrough': '删除线',
  'format.heading2': '二级标题',
  'format.quote': '引用',
  'format.unorderedList': '无序列表',
  'format.orderedList': '有序列表',
  'format.taskList': '任务列表',
  'format.link': '链接',
  'format.inlineCode': '行内代码',
  'format.codeBlock': '代码块',
  'format.table': '表格',
  'format.linkText': '链接文本',
  'format.codePlaceholder': '代码',
  'format.tableTemplate': '\n| 列一 | 列二 | 列三 |\n| --- | :---: | ---: |\n| 内容 | 内容 | 100 |\n',

  'export.pdf': 'PDF 文档',
  'export.html': 'HTML 网页',
  'export.png': 'PNG 长图',
  'export.txt': '纯文本',
  'export.doc': 'Word 兼容文档',
  'export.preparing': '正在准备{format}…',
  'export.chooseLocation': '请选择{format}的保存位置…',
  'export.completed': '{format}已导出：{name}',
  'export.cancelled': '已取消导出',
  'export.externalImagePlaceholder': '外部图片（HTML 导出后可加载）：{source}',

  'alert.note': '说明',
  'alert.tip': '提示',
  'alert.important': '重要',
  'alert.warning': '警告',
  'alert.caution': '注意',

  'toolbar.ariaLabel': '编辑与预览工具栏',
  'toolbar.markdownFormat': 'Markdown 格式',
  'toolbar.syncScroll': '同步滚动',
  'toolbar.viewMode': '视图模式',
  'toolbar.edit': '编辑',
  'toolbar.split': '分栏',
  'toolbar.preview': '预览',
  'toolbar.previewStyle': '预览排版风格',
  'toolbar.layout': '排版',
  'toolbar.copyHTML': '复制 HTML',
  'panel.source': 'MARKDOWN 源码',
  'panel.sourceAriaLabel': 'Markdown 编辑区',
  'panel.editorAriaLabel': 'Markdown 源码编辑器',
  'panel.preview': '实时预览',
  'panel.previewAriaLabel': 'Markdown 实时预览',

  'settings.title': '设置',
  'settings.language': '语言',
  'settings.languageDescription': '选择界面和菜单使用的语言。',
  'settings.languageAuto': '自动（系统）',
  'settings.languageChinese': '简体中文',
  'settings.languageEnglish': 'English',
  'settings.autoDetected': '已自动检测：{language}',
  'settings.close': '关闭',

  'common.close': '关闭',
  'edit.clipboardFailed': '无法访问剪贴板',

  'menu.file': '文件',
  'menu.new': '新建',
  'menu.open': '打开…',
  'menu.openRecent': '打开最近文件',
  'menu.clearRecent': '清除最近记录',
  'menu.showWelcome': '显示欢迎页',
  'menu.save': '保存',
  'menu.saveAs': '另存为…',
  'menu.export': '导出',
  'menu.exportPDF': 'PDF 文档…',
  'menu.exportHTML': 'HTML 网页…',
  'menu.exportPNG': 'PNG 长图…',
  'menu.exportTXT': '纯文本…',
  'menu.exportDOC': 'Word 兼容文档…',
  'menu.closeWindow': '关闭窗口',
  'menu.quit': '退出',
  'menu.edit': '编辑',
  'menu.undo': '撤销',
  'menu.redo': '重做',
  'menu.cut': '剪切',
  'menu.copy': '复制',
  'menu.paste': '粘贴',
  'menu.selectAll': '全选',
  'menu.find': '查找…',
  'menu.view': '视图',
  'menu.editOnly': '仅编辑',
  'menu.splitView': '分栏视图',
  'menu.previewOnly': '仅预览',
  'menu.syncScroll': '同步滚动',
  'menu.zoomIn': '放大',
  'menu.zoomOut': '缩小',
  'menu.actualSize': '实际大小',
  'menu.toggleFullscreen': '切换全屏',
  'menu.format': '格式',
  'menu.theme': '预览主题',
  'menu.window': '窗口',
  'menu.minimize': '最小化',
  'menu.zoom': '缩放',
  'menu.bringAllToFront': '前置全部窗口',
  'menu.help': '帮助',
  'menu.settings': '设置…',
  'menu.keyboardShortcuts': '键盘快捷键',
  'menu.about': '关于墨笺',
  'menu.checkUpdate': '检查更新…',
  'menu.upgrade': '升级/下载更新…',

  'help.shortcutsTitle': '键盘快捷键',
  'help.shortcutsIntro': '使用以下快捷键更快地编辑和管理文档。',
  'help.aboutTitle': '关于墨笺',
  'help.aboutBody': '墨笺是一款支持实时预览的本地 Markdown 编辑器。',
  'help.version': '版本',
  'help.author': '作者',
  'help.sourceRepository': '源码仓库',
  'help.repositoryUnavailable': '未配置源码仓库',
  'help.openRepository': '打开源码仓库',
  'help.updateStatus': '更新状态',
  'help.currentVersion': '当前版本',
  'help.latestVersion': '最新版本',
  'help.publishedAt': '发布时间',
  'help.checkUpdate': '检查更新',
  'help.checkingUpdate': '正在检查更新…',
  'help.downloadUpdate': '升级/下载更新',
  'help.updateNotChecked': '尚未检查更新',
  'help.updateCurrent': '当前已是最新版本（{version}）',
  'help.updateAvailable': '发现新版本 {version}',
  'help.updateUnavailable': '暂时无法获取更新信息',
  'help.updateFailed': '检查更新失败',
  'help.shortcut.new': '新建文档',
  'help.shortcut.open': '打开文档',
  'help.shortcut.save': '保存文档',
  'help.shortcut.saveAs': '另存为',
  'help.shortcut.bold': '粗体',
  'help.shortcut.italic': '斜体',
  'help.shortcut.link': '插入链接',
  'help.shortcut.viewEdit': '仅编辑视图',
  'help.shortcut.viewSplit': '分栏视图',
  'help.shortcut.viewPreview': '仅预览视图',
} as const

export type TranslationKey = keyof typeof zhCNTranslations
export type TranslationDictionary = Readonly<Record<TranslationKey, string>>

const enTranslations: TranslationDictionary = {
  'app.name': 'InkMark',
  'app.fullName': 'InkMark Markdown',
  'app.description': 'Local Markdown Editor',
  'app.offline': 'Offline',

  'document.untitled': 'Untitled',
  'document.untitledFilename': 'Untitled.md',
  'document.unsavedLocation': 'Not saved to disk',
  'document.modified': 'Modified',
  'document.saved': 'Saved',
  'document.opened': 'Document opened',
  'document.created': 'New blank document created',
  'document.savedLocally': 'Saved locally',
  'document.savedAsLocally': 'Saved as a local file',
  'document.dirtyTitle': 'Unsaved changes',

  'confirm.newUnsaved': 'This document has unsaved changes. Create a new document anyway?',
  'confirm.openUnsaved': 'This document has unsaved changes. Open another file anyway?',
  'confirm.quitUnsaved': 'This document has unsaved changes. Quit InkMark anyway?',

  'status.ready': 'Ready',
  'status.waiting': 'Waiting for input…',
  'status.rendering': 'Rendering…',
  'status.rendered': 'Live preview updated',
  'status.renderFailed': 'Rendering failed',
  'status.htmlCopied': 'Rendered HTML copied',
  'status.copyFailed': 'Copy failed',
  'status.characters': '{count} characters',
  'status.lines': '{count} lines',

  'error.previewInterrupted': 'The preview update was interrupted. Please export again.',
  'error.previewUnavailable': 'The preview is not ready yet.',
  'error.documentChangedDuringExport': 'The document changed during export. Please export again.',
  'error.documentTooLong': 'This document is too long to capture safely. Export it as HTML or plain text instead.',
  'error.pdfCanvasUnavailable': 'Unable to create the PDF page canvas.',
  'error.markdownRenderFailed': 'Markdown rendering failed: {message}',

  'theme.github': 'GitHub',
  'theme.clean': 'Clean',
  'theme.wechat': 'WeChat',
  'theme.dark': 'Dark',

  'format.bold': 'Bold',
  'format.italic': 'Italic',
  'format.strikethrough': 'Strikethrough',
  'format.heading2': 'Heading 2',
  'format.quote': 'Blockquote',
  'format.unorderedList': 'Bulleted List',
  'format.orderedList': 'Numbered List',
  'format.taskList': 'Task List',
  'format.link': 'Link',
  'format.inlineCode': 'Inline Code',
  'format.codeBlock': 'Code Block',
  'format.table': 'Table',
  'format.linkText': 'link text',
  'format.codePlaceholder': 'code',
  'format.tableTemplate': '\n| Column 1 | Column 2 | Column 3 |\n| --- | :---: | ---: |\n| Content | Content | 100 |\n',

  'export.pdf': 'PDF Document',
  'export.html': 'HTML Page',
  'export.png': 'PNG Long Image',
  'export.txt': 'Plain Text',
  'export.doc': 'Word-compatible Document',
  'export.preparing': 'Preparing {format}…',
  'export.chooseLocation': 'Choose where to save the {format}…',
  'export.completed': '{format} exported: {name}',
  'export.cancelled': 'Export cancelled',
  'export.externalImagePlaceholder': 'External image (available in the HTML export): {source}',

  'alert.note': 'Note',
  'alert.tip': 'Tip',
  'alert.important': 'Important',
  'alert.warning': 'Warning',
  'alert.caution': 'Caution',

  'toolbar.ariaLabel': 'Editing and preview toolbar',
  'toolbar.markdownFormat': 'Markdown formatting',
  'toolbar.syncScroll': 'Sync scrolling',
  'toolbar.viewMode': 'View mode',
  'toolbar.edit': 'Edit',
  'toolbar.split': 'Split',
  'toolbar.preview': 'Preview',
  'toolbar.previewStyle': 'Preview style',
  'toolbar.layout': 'Style',
  'toolbar.copyHTML': 'Copy HTML',
  'panel.source': 'MARKDOWN SOURCE',
  'panel.sourceAriaLabel': 'Markdown editor',
  'panel.editorAriaLabel': 'Markdown source editor',
  'panel.preview': 'LIVE PREVIEW',
  'panel.previewAriaLabel': 'Markdown live preview',

  'settings.title': 'Settings',
  'settings.language': 'Language',
  'settings.languageDescription': 'Choose the language used by the interface and menus.',
  'settings.languageAuto': 'Automatic (System)',
  'settings.languageChinese': '简体中文',
  'settings.languageEnglish': 'English',
  'settings.autoDetected': 'Automatically detected: {language}',
  'settings.close': 'Close',

  'common.close': 'Close',
  'edit.clipboardFailed': 'Unable to access the clipboard',

  'menu.file': 'File',
  'menu.new': 'New',
  'menu.open': 'Open…',
  'menu.openRecent': 'Open Recent',
  'menu.clearRecent': 'Clear Recent Files',
  'menu.showWelcome': 'Show Welcome Page',
  'menu.save': 'Save',
  'menu.saveAs': 'Save As…',
  'menu.export': 'Export',
  'menu.exportPDF': 'PDF Document…',
  'menu.exportHTML': 'HTML Page…',
  'menu.exportPNG': 'PNG Long Image…',
  'menu.exportTXT': 'Plain Text…',
  'menu.exportDOC': 'Word-compatible Document…',
  'menu.closeWindow': 'Close Window',
  'menu.quit': 'Quit',
  'menu.edit': 'Edit',
  'menu.undo': 'Undo',
  'menu.redo': 'Redo',
  'menu.cut': 'Cut',
  'menu.copy': 'Copy',
  'menu.paste': 'Paste',
  'menu.selectAll': 'Select All',
  'menu.find': 'Find…',
  'menu.view': 'View',
  'menu.editOnly': 'Editor Only',
  'menu.splitView': 'Split View',
  'menu.previewOnly': 'Preview Only',
  'menu.syncScroll': 'Sync Scrolling',
  'menu.zoomIn': 'Zoom In',
  'menu.zoomOut': 'Zoom Out',
  'menu.actualSize': 'Actual Size',
  'menu.toggleFullscreen': 'Toggle Full Screen',
  'menu.format': 'Format',
  'menu.theme': 'Preview Theme',
  'menu.window': 'Window',
  'menu.minimize': 'Minimize',
  'menu.zoom': 'Zoom',
  'menu.bringAllToFront': 'Bring All to Front',
  'menu.help': 'Help',
  'menu.settings': 'Settings…',
  'menu.keyboardShortcuts': 'Keyboard Shortcuts',
  'menu.about': 'About InkMark',
  'menu.checkUpdate': 'Check for Updates…',
  'menu.upgrade': 'Upgrade / Download Update…',

  'help.shortcutsTitle': 'Keyboard Shortcuts',
  'help.shortcutsIntro': 'Use these shortcuts to edit and manage documents more quickly.',
  'help.aboutTitle': 'About InkMark',
  'help.aboutBody': 'InkMark is a local Markdown editor with live preview.',
  'help.version': 'Version',
  'help.author': 'Author',
  'help.sourceRepository': 'Source Repository',
  'help.repositoryUnavailable': 'Source repository is not configured',
  'help.openRepository': 'Open Source Repository',
  'help.updateStatus': 'Update Status',
  'help.currentVersion': 'Current Version',
  'help.latestVersion': 'Latest Version',
  'help.publishedAt': 'Published',
  'help.checkUpdate': 'Check for Updates',
  'help.checkingUpdate': 'Checking for updates…',
  'help.downloadUpdate': 'Upgrade / Download Update',
  'help.updateNotChecked': 'Updates have not been checked yet',
  'help.updateCurrent': 'You are using the latest version ({version})',
  'help.updateAvailable': 'Version {version} is available',
  'help.updateUnavailable': 'Update information is currently unavailable',
  'help.updateFailed': 'Unable to check for updates',
  'help.shortcut.new': 'New Document',
  'help.shortcut.open': 'Open Document',
  'help.shortcut.save': 'Save Document',
  'help.shortcut.saveAs': 'Save As',
  'help.shortcut.bold': 'Bold',
  'help.shortcut.italic': 'Italic',
  'help.shortcut.link': 'Insert Link',
  'help.shortcut.viewEdit': 'Editor Only',
  'help.shortcut.viewSplit': 'Split View',
  'help.shortcut.viewPreview': 'Preview Only',
}

export const translations: Readonly<Record<Locale, TranslationDictionary>> = {
  'zh-CN': zhCNTranslations,
  en: enTranslations,
}

export interface WelcomeDocument {
  name: string
  content: string
}

export const welcomeDocument: Readonly<Record<Locale, Readonly<WelcomeDocument>>> = {
  'zh-CN': {
    name: 'README.md',
    content: `# 墨笺 Markdown

一款专注于本地写作的 Markdown 编辑器。

## 开始使用

- 在左侧编写 Markdown，右侧查看实时预览。
- 使用“文件”菜单打开、保存或导出文档。
- 使用“视图”和“格式”菜单调整编辑方式与排版风格。

## 支持

- GFM、KaTeX 和 Mermaid
- GitHub、清爽、公众号和深色主题
- PDF、HTML、PNG、TXT 和 Word 兼容文档
`,
  },
  en: {
    name: 'README.md',
    content: `# InkMark Markdown

A local Markdown editor focused on writing.

## Get started

- Write Markdown on the left and see the live preview on the right.
- Use the File menu to open, save, or export documents.
- Use the View and Format menus to adjust the workspace and preview style.

## Features

- GFM, KaTeX, and Mermaid
- GitHub, Clean, WeChat, and Dark themes
- PDF, HTML, PNG, TXT, and Word-compatible exports
`,
  },
}

export function normalizeLanguageMode(value: unknown): LanguageMode {
  if (typeof value !== 'string') return 'auto'
  const normalized = value.trim().toLowerCase().replaceAll('_', '-')
  if (normalized === 'auto' || normalized === 'system') return 'auto'
  if (normalized === 'en' || normalized.startsWith('en-')) return 'en'
  if (normalized === 'zh' || normalized.startsWith('zh-')) return 'zh-CN'
  return 'auto'
}

export function getSystemLanguages(): readonly string[] {
  if (typeof navigator === 'undefined') return []
  if (Array.isArray(navigator.languages) && navigator.languages.length) return navigator.languages
  return navigator.language ? [navigator.language] : []
}

export function resolveLocale(
  mode: unknown,
  systemLanguages: readonly string[] | string | null | undefined = getSystemLanguages(),
): Locale {
  const normalizedMode = normalizeLanguageMode(mode)
  if (normalizedMode !== 'auto') return normalizedMode

  const languages = typeof systemLanguages === 'string'
    ? [systemLanguages]
    : systemLanguages || []
  const primaryLanguage = languages.find((language) => typeof language === 'string' && language.trim()) || ''
  return /^zh(?:[-_]|$)/i.test(primaryLanguage.trim()) ? 'zh-CN' : 'en'
}

export function translate(
  locale: Locale,
  key: TranslationKey,
  parameters: Readonly<Record<string, string | number>> = {},
): string {
  return translations[locale][key].replace(/\{([^{}]+)\}/g, (placeholder, name: string) => {
    const value = parameters[name]
    return value === undefined ? placeholder : String(value)
  })
}

export function getWelcomeDocument(locale: Locale): WelcomeDocument {
  return { ...welcomeDocument[locale] }
}
