package main

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type localizedMenuText struct {
	zh string
	en string
}

var nativeMenuText = map[string]localizedMenuText{
	"app":                {zh: "墨笺", en: "InkMark"},
	"file":               {zh: "文件", en: "File"},
	"edit":               {zh: "编辑", en: "Edit"},
	"view":               {zh: "视图", en: "View"},
	"format":             {zh: "格式", en: "Format"},
	"window":             {zh: "窗口", en: "Window"},
	"help":               {zh: "帮助", en: "Help"},
	"about":              {zh: "关于墨笺", en: "About InkMark"},
	"settings":           {zh: "设置…", en: "Settings…"},
	"hide":               {zh: "隐藏墨笺", en: "Hide InkMark"},
	"quit-app":           {zh: "退出墨笺", en: "Quit InkMark"},
	"new":                {zh: "新建", en: "New"},
	"open":               {zh: "打开文件…", en: "Open File…"},
	"open-folder":        {zh: "打开文件夹…", en: "Open Folder…"},
	"connect-webdav":     {zh: "连接 WebDAV…", en: "Connect to WebDAV…"},
	"recent":             {zh: "最近", en: "Recent"},
	"no-recent":          {zh: "无最近项目", en: "No Recent Items"},
	"clear-recent":       {zh: "清除最近项目", en: "Clear Recent Items"},
	"save":               {zh: "保存", en: "Save"},
	"save-as":            {zh: "另存为…", en: "Save As…"},
	"export":             {zh: "导出", en: "Export"},
	"export-pdf":         {zh: "PDF 文档…", en: "PDF Document…"},
	"export-html":        {zh: "HTML 网页…", en: "HTML Web Page…"},
	"export-png":         {zh: "PNG 长图…", en: "PNG Long Image…"},
	"export-txt":         {zh: "纯文本…", en: "Plain Text…"},
	"export-doc":         {zh: "Word 兼容文档…", en: "Word-compatible Document…"},
	"exit":               {zh: "退出", en: "Exit"},
	"undo":               {zh: "撤销", en: "Undo"},
	"redo":               {zh: "重做", en: "Redo"},
	"cut":                {zh: "剪切", en: "Cut"},
	"copy":               {zh: "复制", en: "Copy"},
	"paste":              {zh: "粘贴", en: "Paste"},
	"copy-html":          {zh: "复制渲染后的 HTML", en: "Copy Rendered HTML"},
	"select-all":         {zh: "全选", en: "Select All"},
	"find":               {zh: "查找…", en: "Find…"},
	"view-edit":          {zh: "仅编辑", en: "Editor Only"},
	"view-split":         {zh: "分栏编辑", en: "Split View"},
	"view-preview":       {zh: "仅预览", en: "Preview Only"},
	"sync-scroll":        {zh: "同步滚动", en: "Synchronized Scrolling"},
	"preview-first":      {zh: "预览区在左侧", en: "Preview Pane on Left"},
	"preview-style":      {zh: "预览样式", en: "Preview Style"},
	"theme-github":       {zh: "GitHub", en: "GitHub"},
	"theme-clean":        {zh: "清爽", en: "Clean"},
	"theme-wechat":       {zh: "公众号", en: "WeChat"},
	"theme-dark":         {zh: "深色", en: "Dark"},
	"theme-mist":         {zh: "晨雾", en: "Mist"},
	"theme-paper":        {zh: "米纸", en: "Rice Paper"},
	"theme-pine":         {zh: "松墨", en: "Pine Ink"},
	"theme-sakura":       {zh: "樱灰", en: "Sakura Gray"},
	"theme-ocean":        {zh: "海盐", en: "Ocean Salt"},
	"theme-indigo":       {zh: "暮蓝", en: "Midnight Indigo"},
	"theme-nord":         {zh: "极夜", en: "Polar Night"},
	"theme-obsidian":     {zh: "黑曜", en: "Obsidian"},
	"fullscreen":         {zh: "全屏", en: "Full Screen"},
	"bold":               {zh: "粗体", en: "Bold"},
	"italic":             {zh: "斜体", en: "Italic"},
	"strike":             {zh: "删除线", en: "Strikethrough"},
	"heading":            {zh: "二级标题", en: "Heading 2"},
	"quote":              {zh: "引用", en: "Block Quote"},
	"unordered-list":     {zh: "无序列表", en: "Bulleted List"},
	"ordered-list":       {zh: "有序列表", en: "Numbered List"},
	"task-list":          {zh: "任务列表", en: "Task List"},
	"link":               {zh: "链接", en: "Link"},
	"insert-image":       {zh: "插入图片…", en: "Insert Image…"},
	"inline-code":        {zh: "行内代码", en: "Inline Code"},
	"code-block":         {zh: "代码块", en: "Code Block"},
	"table":              {zh: "表格", en: "Table"},
	"minimise":           {zh: "最小化", en: "Minimize"},
	"zoom-window":        {zh: "缩放窗口", en: "Zoom"},
	"welcome":            {zh: "欢迎页", en: "Welcome"},
	"render-test":        {zh: "综合渲染测试页", en: "Rendering Test Page"},
	"keyboard-shortcuts": {zh: "键盘快捷键", en: "Keyboard Shortcuts"},
	"check-update":       {zh: "检查更新…", en: "Check for Updates…"},
}

type currentLocaleProvider interface {
	currentLocale() string
}

func (a *App) applicationMenu() *menu.Menu {
	locale := "zh-CN"
	if provider, ok := any(a).(currentLocaleProvider); ok {
		locale = provider.currentLocale()
	}
	return a.applicationMenuFor(runtime.GOOS, locale)
}

// applicationMenuFor accepts a variadic locale only to keep older internal
// callers source-compatible. New callers should always pass platform and
// locale explicitly.
func (a *App) applicationMenuFor(platform string, locales ...string) *menu.Menu {
	locale := "zh-CN"
	if len(locales) > 0 {
		locale = locales[0]
	}
	locale = normalizeMenuLocale(locale)
	label := func(key string) string { return nativeMenuLabel(locale, key) }
	state := a.currentMenuState()

	applicationMenu := menu.NewMenu()
	if platform == "darwin" {
		appMenu := applicationMenu.AddSubmenu(label("app"))
		appMenu.AddText(label("about"), nil, a.menuAction("about"))
		appMenu.AddText(label("settings"), keys.CmdOrCtrl(","), a.menuAction("settings"))
		appMenu.AddSeparator()
		appMenu.AddText(label("hide"), keys.CmdOrCtrl("h"), a.menuAction("hide"))
		appMenu.AddSeparator()
		appMenu.AddText(label("quit-app"), keys.CmdOrCtrl("q"), a.menuAction("quit"))
	}

	fileMenu := applicationMenu.AddSubmenu(label("file"))
	fileMenu.AddText(label("new"), keys.CmdOrCtrl("n"), a.menuAction("new"))
	fileMenu.AddText(label("open"), keys.CmdOrCtrl("o"), a.menuAction("open"))
	fileMenu.AddText(label("open-folder"), keys.Combo("o", keys.CmdOrCtrlKey, keys.ShiftKey), a.menuAction("open-folder"))
	fileMenu.AddText(label("connect-webdav"), nil, a.menuAction("connect-webdav"))
	recentMenu := fileMenu.AddSubmenu(label("recent"))
	recentItems := a.recentItemsSnapshot()
	if len(recentItems) == 0 {
		recentMenu.AddText(label("no-recent"), nil, a.menuAction("recent-empty")).Disable()
	} else {
		for _, recentItem := range recentItems {
			recentMenu.AddText(recentMenuLabel(recentItem), nil, a.recentMenuAction(recentItem))
		}
	}
	recentMenu.AddSeparator()
	clearRecent := recentMenu.AddText(label("clear-recent"), nil, a.menuAction("clear-recent"))
	if len(recentItems) == 0 {
		clearRecent.Disable()
	}
	fileMenu.AddSeparator()
	fileMenu.AddText(label("save"), keys.CmdOrCtrl("s"), a.menuAction("save"))
	fileMenu.AddText(label("save-as"), keys.Combo("s", keys.CmdOrCtrlKey, keys.ShiftKey), a.menuAction("save-as"))
	fileMenu.AddSeparator()
	exportMenu := fileMenu.AddSubmenu(label("export"))
	exportMenu.AddText(label("export-pdf"), nil, a.menuAction("export-pdf"))
	exportMenu.AddText(label("export-html"), nil, a.menuAction("export-html"))
	exportMenu.AddSeparator()
	exportMenu.AddText(label("export-png"), nil, a.menuAction("export-png"))
	exportMenu.AddText(label("export-txt"), nil, a.menuAction("export-txt"))
	exportMenu.AddText(label("export-doc"), nil, a.menuAction("export-doc"))
	if platform != "darwin" {
		fileMenu.AddSeparator()
		fileMenu.AddText(label("settings"), keys.CmdOrCtrl(","), a.menuAction("settings"))
		fileMenu.AddSeparator()
		fileMenu.AddText(label("exit"), keys.OptionOrAlt("f4"), a.menuAction("quit"))
	}

	editMenu := applicationMenu.AddSubmenu(label("edit"))
	addEditItem := func(labelKey string, accelerator *keys.Accelerator, action string) {
		itemLabel := label(labelKey)
		if platform == "windows" {
			// WebView2 always handles text-editing shortcuts such as Ctrl+V.
			// Registering the same keys as native Wails menu accelerators makes
			// Windows execute both paths. Keep the shortcut visible in the menu,
			// but leave the accelerator unregistered so the focused WebView
			// control owns the keyboard action exactly once.
			itemLabel += "\t" + keys.Stringify(accelerator, platform)
			accelerator = nil
		}
		editMenu.AddText(itemLabel, accelerator, a.menuAction(action))
	}
	addEditItem("undo", keys.CmdOrCtrl("z"), "undo")
	if platform == "darwin" {
		addEditItem("redo", keys.Combo("z", keys.CmdOrCtrlKey, keys.ShiftKey), "redo")
	} else {
		addEditItem("redo", keys.CmdOrCtrl("y"), "redo")
	}
	editMenu.AddSeparator()
	addEditItem("cut", keys.CmdOrCtrl("x"), "cut")
	addEditItem("copy", keys.CmdOrCtrl("c"), "copy")
	addEditItem("paste", keys.CmdOrCtrl("v"), "paste")
	editMenu.AddText(label("copy-html"), nil, a.menuAction("copy-html"))
	editMenu.AddSeparator()
	addEditItem("select-all", keys.CmdOrCtrl("a"), "select-all")
	editMenu.AddSeparator()
	addEditItem("find", keys.CmdOrCtrl("f"), "find")

	viewMenu := applicationMenu.AddSubmenu(label("view"))
	viewMenu.AddRadio(label("view-edit"), state.ViewMode == "edit", keys.CmdOrCtrl("1"), a.menuAction("view-edit"))
	viewMenu.AddRadio(label("view-split"), state.ViewMode == "split", keys.CmdOrCtrl("2"), a.menuAction("view-split"))
	viewMenu.AddRadio(label("view-preview"), state.ViewMode == "preview", keys.CmdOrCtrl("3"), a.menuAction("view-preview"))
	viewMenu.AddSeparator()
	viewMenu.AddCheckbox(label("sync-scroll"), state.SyncScroll, nil, a.menuAction("toggle-sync-scroll"))
	viewMenu.AddCheckbox(label("preview-first"), state.PreviewFirst, nil, a.menuAction("toggle-pane-order"))
	styleMenu := viewMenu.AddSubmenu(label("preview-style"))
	styleMenu.AddRadio(label("theme-github"), state.Theme == "github", nil, a.menuAction("theme-github"))
	styleMenu.AddRadio(label("theme-clean"), state.Theme == "clean", nil, a.menuAction("theme-clean"))
	styleMenu.AddRadio(label("theme-wechat"), state.Theme == "wechat", nil, a.menuAction("theme-wechat"))
	styleMenu.AddRadio(label("theme-dark"), state.Theme == "dark", nil, a.menuAction("theme-dark"))
	styleMenu.AddRadio(label("theme-mist"), state.Theme == "mist", nil, a.menuAction("theme-mist"))
	styleMenu.AddRadio(label("theme-paper"), state.Theme == "paper", nil, a.menuAction("theme-paper"))
	styleMenu.AddRadio(label("theme-pine"), state.Theme == "pine", nil, a.menuAction("theme-pine"))
	styleMenu.AddRadio(label("theme-sakura"), state.Theme == "sakura", nil, a.menuAction("theme-sakura"))
	styleMenu.AddRadio(label("theme-ocean"), state.Theme == "ocean", nil, a.menuAction("theme-ocean"))
	styleMenu.AddRadio(label("theme-indigo"), state.Theme == "indigo", nil, a.menuAction("theme-indigo"))
	styleMenu.AddRadio(label("theme-nord"), state.Theme == "nord", nil, a.menuAction("theme-nord"))
	styleMenu.AddRadio(label("theme-obsidian"), state.Theme == "obsidian", nil, a.menuAction("theme-obsidian"))
	viewMenu.AddSeparator()
	fullscreenAccelerator := keys.Key("f11")
	if platform == "darwin" {
		fullscreenAccelerator = keys.Combo("f", keys.CmdOrCtrlKey, keys.ControlKey)
	}
	viewMenu.AddText(label("fullscreen"), fullscreenAccelerator, a.menuAction("toggle-fullscreen"))

	formatMenu := applicationMenu.AddSubmenu(label("format"))
	formatMenu.AddText(label("bold"), keys.CmdOrCtrl("b"), a.menuAction("format-bold"))
	formatMenu.AddText(label("italic"), keys.CmdOrCtrl("i"), a.menuAction("format-italic"))
	formatMenu.AddText(label("strike"), nil, a.menuAction("format-strike"))
	formatMenu.AddSeparator()
	formatMenu.AddText(label("heading"), nil, a.menuAction("format-heading"))
	formatMenu.AddText(label("quote"), nil, a.menuAction("format-quote"))
	formatMenu.AddText(label("unordered-list"), nil, a.menuAction("format-ul"))
	formatMenu.AddText(label("ordered-list"), nil, a.menuAction("format-ol"))
	formatMenu.AddText(label("task-list"), nil, a.menuAction("format-task"))
	formatMenu.AddSeparator()
	formatMenu.AddText(label("link"), keys.CmdOrCtrl("k"), a.menuAction("format-link"))
	formatMenu.AddText(label("insert-image"), nil, a.menuAction("insert-image"))
	formatMenu.AddText(label("inline-code"), nil, a.menuAction("format-code"))
	formatMenu.AddText(label("code-block"), nil, a.menuAction("format-codeblock"))
	formatMenu.AddText(label("table"), nil, a.menuAction("format-table"))

	if platform == "darwin" {
		windowMenu := applicationMenu.AddSubmenu(label("window"))
		windowMenu.AddText(label("minimise"), keys.CmdOrCtrl("m"), a.menuAction("window-minimise"))
		windowMenu.AddText(label("zoom-window"), nil, a.menuAction("window-toggle-maximise"))
	}

	helpMenu := applicationMenu.AddSubmenu(label("help"))
	helpMenu.AddText(label("welcome"), nil, a.menuAction("show-welcome"))
	helpMenu.AddText(label("render-test"), nil, a.menuAction("show-render-test"))
	helpMenu.AddText(label("keyboard-shortcuts"), nil, a.menuAction("show-shortcuts"))
	helpMenu.AddSeparator()
	helpMenu.AddText(label("check-update"), nil, a.menuAction("check-update"))
	if platform != "darwin" {
		helpMenu.AddSeparator()
		helpMenu.AddText(label("about"), nil, a.menuAction("about"))
	}

	return applicationMenu
}

func normalizeMenuLocale(locale string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(locale), "_", "-"))
	if strings.HasPrefix(normalized, "zh") {
		return "zh-CN"
	}
	return "en"
}

func nativeMenuLabel(locale string, key string) string {
	text, ok := nativeMenuText[key]
	if !ok {
		return key
	}
	if normalizeMenuLocale(locale) == "zh-CN" {
		return text.zh
	}
	return text.en
}

func (a *App) menuAction(action string) menu.Callback {
	return func(_ *menu.CallbackData) {
		ctx := a.currentContext()
		if ctx == nil {
			return
		}
		wailsruntime.EventsEmit(ctx, menuActionEvent, action)
	}
}

func recentMenuLabel(item RecentItem) string {
	cleanLabel := func(value string) string {
		value = strings.Map(func(character rune) rune {
			if character < 0x20 || character == 0x7f {
				return ' '
			}
			return character
		}, value)
		return strings.Join(strings.Fields(value), " ")
	}
	name := cleanLabel(item.Name)
	if item.Kind == "webdav" {
		if name != "" {
			return "WebDAV — " + name
		}
		return "WebDAV"
	}
	if name == "" {
		name = cleanLabel(filepath.Base(item.Path))
	}
	parent := cleanLabel(filepath.Base(filepath.Dir(item.Path)))
	if parent == "." || parent == string(filepath.Separator) || parent == "" || parent == name {
		return name
	}
	return name + " — " + parent
}

func (a *App) recentMenuAction(item RecentItem) menu.Callback {
	return func(_ *menu.CallbackData) {
		ctx := a.currentContext()
		if ctx == nil {
			return
		}
		wailsruntime.EventsEmit(ctx, openRecentEvent, RecentMenuEvent{ID: item.ID, Kind: item.Kind, Name: item.Name})
	}
}
