# 墨笺 InkMark Markdown

[English](README_EN.md) · [下载最新版](https://github.com/chmod740/inkmark/releases/latest) · [问题反馈](https://github.com/chmod740/inkmark/issues)

墨笺是一款面向 macOS 与 Windows 的 Markdown 编辑器，提供本地文件与 WebDAV 文档编辑、实时预览、原生菜单、多种排版主题和完整的文档导出能力。编辑、渲染和导出所需资源全部随应用提供；联网功能只会在用户主动连接 WebDAV、使用公共图床、检查更新或访问外部链接时使用。

![墨笺分栏编辑与实时预览](docs/images/inkmark-editor.jpg)

## 主要功能

- 打开、编辑、保存、另存为 `.md` 与 `.markdown` 文件
- 通过“文件 → 连接 WebDAV”连接 HTTPS WebDAV 服务，在云端目录侧边栏中浏览、创建、打开、编辑并保存 Markdown 文件
- 可在 WebDAV 连接窗口新建、编辑、连接或删除常用连接；连接名称与地址保存在应用设置中，选择保存的用户名和密码只进入 macOS 钥匙串或 Windows 凭据管理器，不写入设置、最近项目或日志
- 远端保存使用 WebDAV 写锁与 ETag 并发保护；文件被其他遵循 DAV 锁的客户端修改时，可选择重新加载、取消或明确覆盖，避免静默丢失更改
- 可将本地文件夹或 WebDAV 目录作为工作区打开；侧边栏显示 Markdown 与受支持图片，点击图片可安全预览；右键根目录或文件夹可新建内容，右键现有条目可重命名或删除，删除文件或目录前必须再次确认
- “文件 → 最近”按最近使用顺序保留最多 10 个文件、文件夹或 WebDAV 连接；存在可用系统凭据时会直接尝试连接，否则打开连接窗口重新认证
- 从“格式 → 插入图片”或工具栏插入图片：可复制到本地文档资源目录、嵌入为单文件 Data URI，或上传到当前 WebDAV 文档资源目录；预览还可安全读取公开 HTTPS 图床图片
- 120ms 防抖实时渲染；Markdown、公式、代码高亮和图表在后台完整生成后一次更新预览，连续输入时页面不闪烁、不抖动
- 编辑区与预览区按源码段落双向同步滚动，公式和图表重排后自动校准，并避免静止时漂移
- 新建、打开或退出前如有未保存更改，可选择保存、不保存或取消
- 编辑区与预览区可一键调换左右位置，选择会在重启后保留
- 使用 `⌘/Ctrl+F` 查找 Markdown 源码内容，支持前后跳转、循环查找、匹配计数与区分大小写
- GitHub、清爽阅读、微信公众号、深色及晨雾、米纸、松墨、樱灰、海盐、暮蓝、极夜、黑曜共十二种预览主题
- 界面、正文与代码可分别选择字体；界面和正文新增跨平台黑体预设，并提供系统无衬线、阅读衬线、系统等宽，以及随应用离线提供的 Fusion Pixel 12 简体/繁体像素字体
- macOS 与 Windows 原生菜单，集中提供文件、编辑、视图、格式、帮助和更新功能
- 自动检测、简体中文、English 三种语言模式；默认自动跟随系统语言
- 普通启动显示对应语言的本地欢迎页；从 Finder 或资源管理器打开 Markdown 文件时直接显示目标内容
- 欢迎页和“帮助”菜单提供中英双语综合渲染测试页，覆盖扩展方言、表格、公式、代码、表情、脚注、提示块、安全 HTML、Mermaid、思维导图、ECharts、ABC 乐谱与 Graphviz
- 应用、文件关联、桌面快捷方式和应用内使用统一图标
- “关于墨笺”显示版本、作者和更新状态
- “帮助”菜单可以检查 GitHub Releases，在应用内下载并校验当前平台安装包；确认未保存文档后自动关闭旧版本并打开系统安装界面

## Markdown 与预览

- CommonMark 与 GitHub Flavored Markdown
- 表格、任务列表、删除线、自动链接、Emoji 短码、脚注，以及 GitHub 与 `NOTE:` 风格提示块
- 定义列表、缩写定义、`==高亮==`、`H~2~O` 下标和 `x^2^` 上标
- 独占一行的 `[TOC]` 目录，以及标题末尾的 `{#custom-id .custom-class}` 属性；标题 ID 会确定性去重，用户类名使用隔离前缀
- `[[目标]]` 与 `[[目标|标签]]` Wiki 标记以未解析的本地视觉标记显示，不读取工作区文件、不跳转网络
- 文档开头的受限 Front Matter：仅支持有界的扁平 `key: value` 元数据，不执行 YAML 标签、锚点或外部引用
- 引用文献轻量语法：正文 `[@key]` 配合顶层 `[@key]: 文献内容`；这是 InkMark citation-lite，不是完整 Pandoc/CSL 引擎，也不会读取外部 bibliography
- 普通文本中的 `@用户名` 可作视觉标记；它不会查询用户目录、发送通知或生成外部链接
- KaTeX 行内公式与块级公式
- Mermaid 流程图、时序图等图表，并可把两空格缩进的 `mindmap` 列表转换为思维导图
- 受限 JSON 折线图 ECharts、静态 ABC 五线谱，以及在可超时 Worker 中生成的 Graphviz SVG；三类输出均经过安全清理且不加载外部资源
- JavaScript、TypeScript、Python、Go、Rust、JSON、Shell 等常用代码高亮
- 原始 HTML 安全过滤，阻止脚本、iframe、object、embed 和原始 SVG 注入
- 静态 PNG、JPEG、GIF 与 WebP 图片；支持本地相对地址、Data URI、私有 WebDAV 相对地址和公开 HTTPS 地址四种渲染方式
- 本地与 WebDAV 图片使用内容哈希命名，限制文件大小、尺寸和格式；私有远端图片沿用当前 WebDAV 会话，不把认证信息写入 Markdown

## 导出格式

| 格式 | 说明 |
| --- | --- |
| PDF | 按 A4 页面智能分页，保留当前主题 |
| HTML | 生成完整 UTF-8 网页，可引用外部样式、字体、图片和媒体资源 |
| PNG | 将完整预览导出为长图 |
| TXT | 导出 Markdown 纯文本 |
| Word 兼容文档 | 生成带 Office 元数据的 UTF-8 HTML `.doc`，可由 Microsoft Word 打开并继续编辑 |

选择 Fusion Pixel 时，HTML 与 Word 兼容导出会嵌入离线 WOFF2；部分 Word 版本不支持 Data URI 字体时会回退到系统字体。Fusion Pixel 当前只提供 Regular 400，粗体与斜体由渲染引擎合成；Mermaid 图中文字仍使用其安全的内置字体设置。

## 下载与安装

从 [GitHub Releases](https://github.com/chmod740/inkmark/releases/latest) 下载对应安装包：

- macOS 11 或更高版本（建议安装最新系统更新，以获得完整 Graphviz WebView/WASM 兼容性）：优先使用 `macos-universal.pkg` 原位升级，也提供同时支持 Apple Silicon 与 Intel 的 DMG 和 ZIP
- Windows 10/11 x64：`windows-amd64-setup.exe`

macOS 与 Windows 安装包目前未使用商业代码签名证书。首次运行时系统可能显示 Gatekeeper 或 SmartScreen 提示，请确认文件来自本仓库 Release，并使用同一 Release 中的 `SHA256SUMS` 校验完整性。

## 快速使用

1. 直接启动墨笺，或在 Finder/资源管理器中使用墨笺打开 Markdown 文件。
2. 可使用“文件 → 打开文件夹”显示本地目录侧边栏，或使用“文件 → 连接 WebDAV”连接云端目录。右键侧边栏空白区域或文件夹可新建 Markdown 文件或子目录；右键现有条目可重命名或删除，点击图片条目可打开预览。
3. WebDAV 成功连接后，规范化的完整服务器地址（包括路径）会进入“最近”项目。可在连接窗口将常用连接保存到系统凭据库；再次从“最近”打开时，有可用凭据会直接尝试连接，否则只预填地址并要求重新认证。
   如果同一服务还提供绕过 WebDAV 锁的网页或 API 写入入口，需要服务端让所有写入路径共享原子条件更新或锁，才能获得跨入口并发保护。
4. 在编辑区编写 Markdown，并在预览区查看实时结果。
5. 使用“视图”菜单切换编辑布局和主题，使用“格式”菜单插入常用 Markdown 标记或图片。
6. 使用“文件”菜单保存文档，或导出 PDF、HTML、PNG、TXT 与 Word 兼容文档；WebDAV 文档的“另存为”会生成本地副本。
7. 在“设置”中选择自动、简体中文或 English，并分别设置界面、正文与代码字体；字体偏好仅保存固定预设，不接受任意 CSS 字体值。
8. 在“帮助 → 检查更新”查看新版本；可直接下载、校验并启动系统安装器，“关于墨笺”也会显示下载和安装状态。

## 离线与联网说明

编辑器运行时不会从 CDN 加载 JavaScript、CSS、字体、KaTeX、Mermaid、ECharts、ABC、Graphviz 或高亮资源。Fusion Pixel 简体/繁体字体随应用提供；选择它们时，HTML 与 Word 兼容导出会嵌入相应离线字体资源。本地相对图片、Data URI 图片和内置图表可在应用内完全离线预览与导出；独立 HTML 若含 KaTeX/代码高亮，仍可能引用其现有外部样式地址。

以下操作会按用户请求联网：

- 连接 WebDAV、读取远端目录、打开或保存远端 Markdown 文件，以及上传或读取私有远端图片
- 预览公开 HTTPS 图片，或将其嵌入 PDF、PNG 与 Word 兼容导出；HTML 导出会保留原始公开地址
- 检查新版本时访问 GitHub Releases API
- 下载更新时访问 GitHub
- 点击文档中的外部链接
- 打开导出的 HTML 时加载该文件引用的外部资源

## 从源码构建

项目使用 Go、[Wails v2](https://wails.io/) 和 Vue 3。需要 Go 1.25+、Node.js、pnpm 与对应平台的原生构建工具。

macOS：

```bash
./scripts/build-macos.sh
./scripts/package-macos.sh
```

脚本生成 Universal 应用，并在 `dist/` 中创建 PKG、DMG、ZIP 和校验文件。

Windows：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1
```

Windows 构建需要 NSIS 和 WebView2 Runtime，生成用户级安装包并注册 `.md`、`.markdown` 文件关联。

## 测试

```bash
go test ./...
go test -race ./...
pnpm --dir frontend typecheck
pnpm --dir frontend test:i18n
pnpm --dir frontend test:export
pnpm --dir frontend test:scroll
pnpm --dir frontend test:preview
pnpm --dir frontend test:markdown
pnpm --dir frontend test:dialects
pnpm --dir frontend test:diagrams
pnpm --dir frontend test:update
pnpm --dir frontend test:ui
pnpm --dir frontend test:workspace
pnpm --dir frontend test:webdav
pnpm --dir frontend test:saved-webdav
pnpm --dir frontend test:image
pnpm --dir frontend test:installer
pnpm --dir frontend test:notices
pnpm --dir frontend test:fonts
pnpm --dir frontend test:find
node scripts/verify-offline.mjs
```

测试覆盖本地文件读写、WebDAV 认证与目录解析、系统凭据库连接管理、路径编码、ETag 冲突、远端保存、本地与 WebDAV 工作区的新建/重命名/递归删除、图片条目安全预览、本地与 WebDAV 图片导入、四类图片渲染、Emoji/脚注/提示块/提及、定义列表/缩写/高亮/上下标、TOC/标题属性/Wiki/Front Matter/citation-lite 的边界与恶意输入、源码查找导航、字体预设存储与离线嵌字、深色模式字体预览、主题配色、思维导图与三类静态图形的输入限制和恶意载荷拒绝、公共图片网络边界、受限工作区目录访问、侧边栏懒加载与上下文菜单、最近项目、未保存文档切换与原生关闭守卫、语言设置、原生菜单、安全升级下载与安装编排、原子预览提交、版本比较与 Release 响应、导出结构、外部资源策略、同步滚动、大文档锚点上限和离线资源完整性。真实 WebDAV 回归测试只在运行时注入测试地址与凭据时执行，并会清理随机命名的临时资源。

## 项目结构

- `app.go`：原生文件对话框、读写与导出保存
- `close_guard.go`：macOS 与 Windows 原生关闭前的保存守卫
- `app_state.go`：语言设置、单实例和系统文件打开
- `workspace.go`、`recent.go`：受限本地工作区、Markdown 与图片枚举、安全文件操作和最近项目
- `webdav.go`、`webdav_app.go`：HTTPS WebDAV 协议、远端能力会话、目录与文件操作以及写锁与 ETag 安全保存
- `webdav_connections.go`：常用 WebDAV 连接元数据、系统凭据库与最近项目直连
- `image_assets.go`：本地与 WebDAV 图片导入、相对资源解析和受限公共 HTTPS 图片获取
- `update.go`、`update_download.go`、`update_launch.go`：版本检测、安装包选择、安全下载校验和平台安装器编排
- `menu.go`：macOS 与 Windows 原生菜单
- `frontend/src/App.vue`：编辑器、实时预览、设置和关于页面
- `frontend/src/DirectorySidebar.vue`、`workspace-tree.ts`：目录侧边栏、图片条目、右键文件操作和按需展开状态
- `frontend/src/i18n.ts`：中英文界面与欢迎页文案
- `frontend/src/ui-state.ts`：文档状态与分栏顺序偏好
- `frontend/src/document-guard.ts`：未保存文档的安全切换守卫
- `frontend/src/export-document.ts`：HTML、PDF、PNG、TXT 和 Word 兼容导出
- `frontend/src/scroll-sync.ts`：稳定的双向滚动同步
- `frontend/src/preview-render.ts`：预览原子提交与 Mermaid 缓存
- `frontend/src/markdown-extensions.ts`：Emoji、脚注、提示块、提及、思维导图和扩展 Markdown 方言适配
- `frontend/src/font-preferences.ts`、`font-preferences.css`：三类字体偏好、固定安全字体栈、Fusion Pixel 离线字体与导出嵌入
- `frontend/src/extended-diagrams.ts`、`graphviz-worker.ts`：受限 ECharts、ABC 与隔离 Graphviz 静态渲染
- `frontend/src/image-resources.ts`：四类图片来源识别、预览资源生命周期与导出转换
- `samples/markdown-rendering-test.md`：应用内置的中英双语综合渲染样例
- `scripts/`：构建、打包和回归测试脚本

## 版本与更新

发布版本使用 `v主版本.次版本.修订版本` 标签。墨笺只在用户打开“关于”或主动选择“检查更新”时查询最新 Release；发现更高版本后，会选择与当前操作系统和架构匹配的安装包。

作者：Codex · 源码仓库：[github.com/chmod740/inkmark](https://github.com/chmod740/inkmark)
