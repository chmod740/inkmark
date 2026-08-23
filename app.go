package main

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	appTitle          = "墨笺 Markdown"
	appTitleEnglish   = "InkMark Markdown"
	maxDocumentSize   = 16 << 20
	maxExportSize     = 96 << 20
	menuActionEvent   = "inkmark:menu-action"
	openDocumentEvent = "inkmark:open-document"
	openRecentEvent   = "inkmark:open-recent"
	openErrorEvent    = "inkmark:open-error"
	closeRequestEvent = "inkmark:close-request"
)

type exportFormatConfig struct {
	Extension   string
	DisplayName string
	DialogTitle string
}

var exportFormatsChinese = map[string]exportFormatConfig{
	"pdf":  {Extension: "pdf", DisplayName: "PDF 文档 (*.pdf)", DialogTitle: "导出 PDF 文档"},
	"html": {Extension: "html", DisplayName: "HTML 网页 (*.html)", DialogTitle: "导出 HTML 网页"},
	"png":  {Extension: "png", DisplayName: "PNG 图片 (*.png)", DialogTitle: "导出 PNG 长图"},
	"txt":  {Extension: "txt", DisplayName: "纯文本文件 (*.txt)", DialogTitle: "导出纯文本"},
	"doc":  {Extension: "doc", DisplayName: "Word 兼容文档 (*.doc)", DialogTitle: "导出 Word 兼容文档"},
}

var exportFormatsEnglish = map[string]exportFormatConfig{
	"pdf":  {Extension: "pdf", DisplayName: "PDF document (*.pdf)", DialogTitle: "Export PDF"},
	"html": {Extension: "html", DisplayName: "HTML page (*.html)", DialogTitle: "Export HTML"},
	"png":  {Extension: "png", DisplayName: "PNG image (*.png)", DialogTitle: "Export PNG"},
	"txt":  {Extension: "txt", DisplayName: "Plain text (*.txt)", DialogTitle: "Export plain text"},
	"doc":  {Extension: "doc", DisplayName: "Word-compatible document (*.doc)", DialogTitle: "Export Word-compatible document"},
}

type Document struct {
	Path             string     `json:"path"`
	Name             string     `json:"name"`
	Content          string     `json:"content"`
	Welcome          bool       `json:"welcome"`
	BuiltIn          string     `json:"builtIn,omitempty"`
	ActivationID     string     `json:"activationId,omitempty"`
	StorageKind      string     `json:"storageKind,omitempty"`
	DisplayLocation  string     `json:"displayLocation,omitempty"`
	WorkspaceID      string     `json:"workspaceId,omitempty"`
	WorkspacePath    string     `json:"workspacePath,omitempty"`
	LocalDocumentID  string     `json:"localDocumentId,omitempty"`
	RemoteDocumentID string     `json:"remoteDocumentId,omitempty"`
	ETag             string     `json:"etag,omitempty"`
	Workspace        *Workspace `json:"workspace,omitempty"`
}

//go:embed samples/markdown-rendering-test.md
var renderingTestMarkdown string

type SaveResult struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type LanguageState struct {
	Mode   string `json:"mode"`
	Locale string `json:"locale"`
}

type MenuState struct {
	ViewMode     string
	Theme        string
	SyncScroll   bool
	PreviewFirst bool
}

type App struct {
	mu                      sync.RWMutex
	savedWebDAVMu           sync.Mutex
	settingsWriteMu         sync.Mutex
	menuRefreshMu           sync.Mutex
	ctx                     context.Context
	initPath                string
	initialLoaded           bool
	language                LanguageState
	lastPage                LastPageState
	recentItems             []RecentItem
	savedWebDAVConnections  []savedWebDAVConnectionState
	webDAVCredentialStore   webDAVCredentialStore
	pendingRecentDocuments  map[string]string
	activeWorkspace         *workspaceCapability // most recently activated; retained for internal compatibility
	localWorkspaces         map[string]*workspaceCapability
	webDAVWorkspaces        map[string]*webDAVCapability
	menuState               MenuState
	settingsPath            string
	updateEndpoint          string
	updateClient            httpDoer
	updateDownloadClient    httpDoer
	latestUpdate            UpdateInfo
	updateAsset             githubReleaseAsset
	checksumAsset           githubReleaseAsset
	downloadedUpdate        downloadedUpdate
	updateChecking          bool
	updateDownloading       bool
	updateLaunching         bool
	updateCancel            context.CancelFunc
	updateCacheDir          string
	allowTestUpdateURLs     bool
	updateInstallerLauncher func(updateLaunchRequest) error
	closeGuard              closeGuardState
}

func NewApp() *App {
	workingDirectory, _ := os.Getwd()
	settingsPath := defaultSettingsPath()
	settings := loadSettingsState(settingsPath)
	return &App{
		initPath:               resolveDocumentArgument(os.Args[1:], workingDirectory),
		language:               settings.LanguageState,
		lastPage:               settings.LastPage,
		recentItems:            settings.RecentItems,
		savedWebDAVConnections: settings.SavedWebDAVConnections,
		webDAVCredentialStore:  systemWebDAVCredentialStore{},
		pendingRecentDocuments: make(map[string]string),
		localWorkspaces:        make(map[string]*workspaceCapability),
		webDAVWorkspaces:       make(map[string]*webDAVCapability),
		menuState:              MenuState{ViewMode: "split", Theme: "github", SyncScroll: true},
		settingsPath:           settingsPath,
		updateEndpoint:         latestReleaseAPIURL,
		updateClient:           newUpdateHTTPClient(),
		updateDownloadClient:   newUpdateDownloadHTTPClient(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
}

func (a *App) shutdown(_ context.Context) {
	a.mu.Lock()
	localWorkspaces := make([]*workspaceCapability, 0, len(a.localWorkspaces))
	for _, capability := range a.localWorkspaces {
		localWorkspaces = append(localWorkspaces, capability)
	}
	a.activeWorkspace = nil
	a.localWorkspaces = nil
	webDAVWorkspaces := make([]*webDAVCapability, 0, len(a.webDAVWorkspaces))
	for _, capability := range a.webDAVWorkspaces {
		webDAVWorkspaces = append(webDAVWorkspaces, capability)
	}
	a.webDAVWorkspaces = nil
	a.pendingRecentDocuments = nil
	a.ctx = nil
	cancel := a.updateCancel
	a.updateCancel = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	for _, capability := range localWorkspaces {
		_ = capability.root.Close()
	}
	for _, capability := range webDAVWorkspaces {
		capability.close()
	}
}

func (a *App) LoadInitialDocument(locale string) (Document, error) {
	locale = normalizeLocale(locale)
	a.mu.Lock()
	path := a.initPath
	a.initPath = ""
	lastPage := a.lastPage
	a.initialLoaded = true
	a.mu.Unlock()
	if path != "" {
		return a.openLocalDocumentWithWorkspace(path, true)
	}
	if lastPage.Kind == "local" {
		if document, err := a.openLocalDocumentWithWorkspace(lastPage.Path, false); err == nil {
			return document, nil
		}
		a.mu.Lock()
		a.lastPage = LastPageState{}
		a.mu.Unlock()
		_ = a.persistSettings()
	}
	if lastPage.Kind == "builtin" && lastPage.BuiltIn == "render-test" {
		return renderingTestDocument(), nil
	}
	if lastPage.Kind == "webdav" {
		if workspace, err := a.ConnectSavedWebDAV(lastPage.SavedConnectionID); err == nil {
			if document, openErr := a.OpenWebDAVFile(workspace.ID, lastPage.WorkspacePath); openErr == nil {
				document.Workspace = &workspace
				return document, nil
			}
			_ = a.CloseWebDAVWorkspace(workspace.ID)
		}
		a.mu.Lock()
		a.lastPage = LastPageState{}
		a.mu.Unlock()
		_ = a.persistSettings()
	}
	return welcomeDocument(locale), nil
}

// RememberLastPage persists only a backend-verified locator. Markdown source,
// credentials, validators and opaque runtime IDs are never written to disk.
func (a *App) RememberLastPage(storageKind string, workspaceID string, localDocumentID string, builtIn string) error {
	next := LastPageState{}
	switch strings.TrimSpace(storageKind) {
	case "builtin":
		if builtIn != "welcome" && builtIn != "render-test" {
			return errors.New("内置页面类型无效")
		}
		next = LastPageState{Version: 1, Kind: "builtin", BuiltIn: builtIn}
	case "local":
		a.mu.RLock()
		capability, ok := a.localWorkspaces[strings.TrimSpace(workspaceID)]
		document, documentOK := workspaceDocumentCapability{}, false
		if ok && capability != nil {
			document, documentOK = capability.documents[strings.TrimSpace(localDocumentID)]
		}
		if documentOK {
			next = LastPageState{
				Version: 1,
				Kind:    "local",
				Path:    filepath.Join(capability.path, filepath.FromSlash(document.path)),
			}
		}
		a.mu.RUnlock()
		if next.Kind == "" {
			return errors.New("本地文档会话已关闭或失效")
		}
	case "webdav":
		a.mu.RLock()
		capability, ok := a.webDAVWorkspaces[strings.TrimSpace(workspaceID)]
		if ok && capability != nil && capability.savedConnectionID != "" {
			capability.mu.RLock()
			document, documentOK := capability.documents[strings.TrimSpace(localDocumentID)]
			if documentOK {
				next = LastPageState{
					Version:           1,
					Kind:              "webdav",
					SavedConnectionID: capability.savedConnectionID,
					WorkspacePath:     document.path,
				}
			}
			capability.mu.RUnlock()
		}
		a.mu.RUnlock()
		// One-time WebDAV sessions intentionally fall back to welcome because
		// there is no saved connection metadata with which to reconnect.
	case "":
	default:
		return errors.New("页面存储类型无效")
	}
	a.mu.Lock()
	a.lastPage = next
	a.mu.Unlock()
	return a.persistSettings()
}

func (a *App) WelcomeDocument(locale string) Document {
	return welcomeDocument(normalizeLocale(locale))
}

func (a *App) RenderingTestDocument() Document {
	return renderingTestDocument()
}

func welcomeDocument(locale string) Document {
	if normalizeLocale(locale) == "en" {
		return Document{
			Name:    "README.md",
			Welcome: true,
			BuiltIn: "welcome",
			Content: "# InkMark Markdown\n\nA local Markdown editor focused on writing.\n\n## Get started\n\n- Write Markdown on the left and see the live preview on the right.\n- Use the File menu to open, save, or export documents.\n- Use the View and Format menus to adjust the workspace and preview style.\n- [Open the bilingual comprehensive rendering test](#inkmark-render-test) to inspect GFM, KaTeX, Mermaid, code highlighting, and safe HTML.\n\n## Features\n\n- GFM, KaTeX, and Mermaid\n- GitHub, Clean, WeChat, and Dark themes\n- PDF, HTML, PNG, TXT, and Word-compatible exports\n",
		}
	}
	return Document{
		Name:    "README.md",
		Welcome: true,
		BuiltIn: "welcome",
		Content: "# 墨笺 Markdown\n\n一款专注于本地写作的 Markdown 编辑器。\n\n## 开始使用\n\n- 在左侧编写 Markdown，右侧查看实时预览。\n- 使用“文件”菜单打开、保存或导出文档。\n- 使用“视图”和“格式”菜单调整编辑方式与排版风格。\n- [打开中英双语综合渲染测试页](#inkmark-render-test)，检查 GFM、KaTeX、Mermaid、代码高亮和安全 HTML。\n\n## 支持\n\n- GFM、KaTeX 和 Mermaid\n- GitHub、清爽、公众号和深色主题\n- PDF、HTML、PNG、TXT 和 Word 兼容文档\n",
	}
}

func renderingTestDocument() Document {
	return Document{
		Name:    "markdown-rendering-test.md",
		Content: strings.TrimSpace(renderingTestMarkdown) + "\n",
		BuiltIn: "render-test",
	}
}

func (a *App) OpenFile() (Document, error) {
	english := a.currentLocale() == "en"
	title := "打开 Markdown 文件"
	markdownFilter := "Markdown 文件 (*.md;*.markdown)"
	textFilter := "文本文件 (*.txt)"
	allFilter := "所有文件"
	if english {
		title = "Open Markdown File"
		markdownFilter = "Markdown files (*.md;*.markdown)"
		textFilter = "Text files (*.txt)"
		allFilter = "All files"
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: title,
		Filters: []runtime.FileFilter{
			{DisplayName: markdownFilter, Pattern: "*.md;*.markdown"},
			{DisplayName: textFilter, Pattern: "*.txt"},
			{DisplayName: allFilter, Pattern: "*"},
		},
	})
	if err != nil {
		if english {
			return Document{}, fmt.Errorf("open dialog failed: %w", err)
		}
		return Document{}, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if path == "" {
		return Document{}, nil
	}
	return a.openLocalDocumentWithWorkspace(path, true)
}

func (a *App) SaveFile(path string, content string) (SaveResult, error) {
	if strings.TrimSpace(path) == "" {
		return a.SaveFileAs("", content)
	}
	return writeDocument(path, content)
}

func (a *App) SaveFileAs(currentPath string, content string) (SaveResult, error) {
	english := a.currentLocale() == "en"
	defaultName := "未命名.md"
	dialogTitle := "保存 Markdown 文件"
	markdownFilter := "Markdown 文件 (*.md)"
	allFilter := "所有文件"
	if english {
		defaultName = "Untitled.md"
		dialogTitle = "Save Markdown File"
		markdownFilter = "Markdown files (*.md)"
		allFilter = "All files"
	}
	defaultDirectory := ""
	if currentPath != "" {
		defaultName = filepath.Base(currentPath)
		if directory := filepath.Dir(currentPath); directory != "." && directory != "" {
			defaultDirectory = directory
		}
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:            dialogTitle,
		DefaultDirectory: defaultDirectory,
		DefaultFilename:  defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: markdownFilter, Pattern: "*.md"},
			{DisplayName: allFilter, Pattern: "*"},
		},
	})
	if err != nil {
		if english {
			return SaveResult{}, fmt.Errorf("save dialog failed: %w", err)
		}
		return SaveResult{}, fmt.Errorf("保存文件对话框失败: %w", err)
	}
	if path == "" {
		return SaveResult{}, nil
	}
	if filepath.Ext(path) == "" {
		path += ".md"
	}
	return writeDocument(path, content)
}

// SaveExportFile writes a frontend-generated export after the user chooses its
// destination. Binary data crosses the Wails bridge as base64 so PDF and PNG
// use the same native, cross-platform save flow as text-based exports.
func (a *App) SaveExportFile(format string, currentPath string, suggestedName string, payloadBase64 string) (SaveResult, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	formats := exportFormatsChinese
	allFilter := "所有文件"
	if a.currentLocale() == "en" {
		formats = exportFormatsEnglish
		allFilter = "All files"
	}
	config, ok := formats[format]
	if !ok {
		return SaveResult{}, fmt.Errorf("不支持的导出格式: %s", format)
	}

	payload, err := decodeExportPayload(format, payloadBase64)
	if err != nil {
		return SaveResult{}, err
	}

	defaultDirectory := ""
	if strings.TrimSpace(currentPath) != "" {
		defaultDirectory = filepath.Dir(currentPath)
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:            config.DialogTitle,
		DefaultDirectory: defaultDirectory,
		DefaultFilename:  exportDefaultFilename(currentPath, suggestedName, config.Extension),
		Filters: []runtime.FileFilter{
			{DisplayName: config.DisplayName, Pattern: "*." + config.Extension},
			{DisplayName: allFilter, Pattern: "*"},
		},
	})
	if err != nil {
		return SaveResult{}, fmt.Errorf("打开导出对话框失败: %w", err)
	}
	if path == "" {
		return SaveResult{}, nil
	}
	path = ensureExportExtension(path, config.Extension)
	return writeExportFile(path, format, payload)
}

func (a *App) SetWindowTitle(name string, dirty bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		if a.currentLocale() == "en" {
			name = "Untitled.md"
		} else {
			name = "未命名.md"
		}
	}
	marker := ""
	if dirty {
		marker = " •"
	}
	title := appTitle
	if a.currentLocale() == "en" {
		title = appTitleEnglish
	}
	runtime.WindowSetTitle(a.ctx, name+marker+" — "+title)
}

func (a *App) OpenExternal(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "mailto") {
		if a.currentLocale() == "en" {
			return errors.New("only HTTP, HTTPS, and mail links are allowed")
		}
		return errors.New("仅允许打开 HTTP、HTTPS 或邮件链接")
	}
	runtime.BrowserOpenURL(a.ctx, rawURL)
	return nil
}

func readDocument(path string) (Document, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Document{}, fmt.Errorf("解析文件路径失败: %w", err)
	}
	file, err := openReadOnlyNonBlocking(absolute)
	if err != nil {
		return Document{}, fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()
	return readDocumentFromFile(file, absolute)
}

func readDocumentFromFile(file *os.File, path string) (Document, error) {
	info, err := file.Stat()
	if err != nil {
		return Document{}, fmt.Errorf("读取文件信息失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		if info.IsDir() {
			return Document{}, errors.New("所选路径是文件夹")
		}
		return Document{}, errors.New("所选路径不是普通文件")
	}
	if info.Size() > maxDocumentSize {
		return Document{}, fmt.Errorf("文档超过 %d MB 限制", maxDocumentSize>>20)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxDocumentSize+1))
	if err != nil {
		return Document{}, fmt.Errorf("读取文件失败: %w", err)
	}
	if len(data) > maxDocumentSize {
		return Document{}, fmt.Errorf("文档超过 %d MB 限制", maxDocumentSize>>20)
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if !utf8.Valid(data) {
		return Document{}, errors.New("文件不是有效的 UTF-8 文本")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	return Document{
		Path:            absolute,
		Name:            filepath.Base(absolute),
		Content:         string(data),
		StorageKind:     "local",
		DisplayLocation: absolute,
	}, nil
}

func newOpaqueID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func writeDocument(path string, content string) (SaveResult, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return SaveResult{}, fmt.Errorf("解析保存路径失败: %w", err)
	}
	if len(content) > maxDocumentSize {
		return SaveResult{}, fmt.Errorf("文档超过 %d MB 限制", maxDocumentSize>>20)
	}
	if err := os.WriteFile(absolute, []byte(content), 0o644); err != nil {
		return SaveResult{}, fmt.Errorf("保存文件失败: %w", err)
	}
	return SaveResult{Path: absolute, Name: filepath.Base(absolute)}, nil
}

func exportDefaultFilename(currentPath string, suggestedName string, extension string) string {
	base := strings.TrimSpace(suggestedName)
	if strings.TrimSpace(currentPath) != "" {
		base = filepath.Base(currentPath)
	} else {
		base = filepath.Base(base)
	}
	stem := strings.TrimSpace(strings.TrimSuffix(base, filepath.Ext(base)))
	if stem == "" || stem == "." {
		stem = "未命名"
	}
	return stem + "." + extension
}

func ensureExportExtension(path string, extension string) string {
	extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
	if extension == "" {
		return path
	}

	suffix := "." + extension
	// Some Windows save dialogs append the selected filter extension even when
	// it was already typed. Chinese IMEs may also turn the typed dot into a
	// full-width variant, so collapse both forms before writing.
	typedSuffixes := []string{suffix, "。" + extension, "．" + extension}
	for {
		collapsed := false
		for _, typedSuffix := range typedSuffixes {
			duplicate := typedSuffix + suffix
			if strings.HasSuffix(strings.ToLower(path), strings.ToLower(duplicate)) {
				path = path[:len(path)-len(duplicate)] + suffix
				collapsed = true
				break
			}
		}
		if !collapsed {
			break
		}
	}
	if !strings.EqualFold(filepath.Ext(path), suffix) {
		path += suffix
	}
	return path
}

func decodeExportPayload(format string, payloadBase64 string) ([]byte, error) {
	maxEncodedSize := ((maxExportSize + 2) / 3) * 4
	if len(payloadBase64) > maxEncodedSize {
		return nil, fmt.Errorf("导出文件超过 %d MB 限制", maxExportSize>>20)
	}
	payload, err := base64.StdEncoding.DecodeString(payloadBase64)
	if err != nil {
		return nil, errors.New("导出数据不是有效的 Base64")
	}
	if len(payload) > maxExportSize {
		return nil, fmt.Errorf("导出文件超过 %d MB 限制", maxExportSize>>20)
	}
	if err := validateExportPayload(format, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func validateExportPayload(format string, payload []byte) error {
	if len(payload) == 0 && format != "txt" {
		return errors.New("导出数据为空")
	}
	switch format {
	case "pdf":
		if !bytes.HasPrefix(payload, []byte("%PDF-")) {
			return errors.New("PDF 导出数据无效")
		}
	case "png":
		if !bytes.HasPrefix(payload, []byte{'\x89', 'P', 'N', 'G', '\r', '\n', '\x1a', '\n'}) {
			return errors.New("PNG 导出数据无效")
		}
	case "html", "doc":
		if !utf8.Valid(payload) || !bytes.Contains(bytes.ToLower(payload), []byte("<html")) {
			return errors.New("HTML 导出数据无效")
		}
	case "txt":
		if !utf8.Valid(payload) {
			return errors.New("纯文本导出数据不是有效的 UTF-8")
		}
	default:
		return fmt.Errorf("不支持的导出格式: %s", format)
	}
	return nil
}

func writeExportFile(path string, format string, payload []byte) (SaveResult, error) {
	if err := validateExportPayload(format, payload); err != nil {
		return SaveResult{}, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return SaveResult{}, fmt.Errorf("解析导出路径失败: %w", err)
	}
	if len(payload) > maxExportSize {
		return SaveResult{}, fmt.Errorf("导出文件超过 %d MB 限制", maxExportSize>>20)
	}
	if err := os.WriteFile(absolute, payload, 0o644); err != nil {
		return SaveResult{}, fmt.Errorf("导出文件失败: %w", err)
	}
	return SaveResult{Path: absolute, Name: filepath.Base(absolute)}, nil
}
