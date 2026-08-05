# 墨笺 InkMark Markdown

[English](README_EN.md) · [下载最新版](https://github.com/chmod740/inkmark/releases/latest) · [问题反馈](https://github.com/chmod740/inkmark/issues)

墨笺是一款面向 macOS 与 Windows 的本地 Markdown 编辑器，提供实时预览、原生菜单、多种排版主题和完整的文档导出能力。编辑、渲染和导出所需资源全部随应用提供；只有用户主动检查更新或访问外部链接时才需要网络。

![墨笺分栏编辑与实时预览](docs/images/inkmark-editor.jpg)

## 主要功能

- 打开、编辑、保存、另存为 `.md` 与 `.markdown` 文件
- 120ms 防抖实时渲染；Markdown、公式、代码高亮和图表在后台完整生成后一次更新预览，连续输入时页面不闪烁、不抖动
- 编辑区与预览区按源码段落双向同步滚动，公式和图表重排后自动校准，并避免静止时漂移
- 新建、打开或退出前如有未保存更改，可选择保存、不保存或取消
- 编辑区与预览区可一键调换左右位置，选择会在重启后保留
- GitHub、清爽阅读、微信公众号和深色四种预览主题
- macOS 与 Windows 原生菜单，集中提供文件、编辑、视图、格式、帮助和更新功能
- 自动检测、简体中文、English 三种语言模式；默认自动跟随系统语言
- 普通启动显示对应语言的本地欢迎页；从 Finder 或资源管理器打开 Markdown 文件时直接显示目标内容
- 欢迎页和“帮助”菜单提供中英双语综合渲染测试页，覆盖表格、公式、代码、提示块、安全 HTML 与十类 Mermaid 图
- 应用、文件关联、桌面快捷方式和应用内使用统一图标
- “关于墨笺”显示版本、作者、源码仓库和更新状态
- “帮助”菜单可以检查 GitHub Releases，在应用内下载并校验当前平台安装包；确认未保存文档后自动关闭旧版本并打开系统安装界面

## Markdown 与预览

- CommonMark 与 GitHub Flavored Markdown
- 表格、任务列表、删除线、自动链接和提示块
- KaTeX 行内公式与块级公式
- Mermaid 流程图、时序图等图表
- JavaScript、TypeScript、Python、Go、Rust、JSON、Shell 等常用代码高亮
- 原始 HTML 安全过滤，阻止脚本、iframe、object、embed 和原始 SVG 注入

## 导出格式

| 格式 | 说明 |
| --- | --- |
| PDF | 按 A4 页面智能分页，保留当前主题 |
| HTML | 生成完整 UTF-8 网页，可引用外部样式、字体、图片和媒体资源 |
| PNG | 将完整预览导出为长图 |
| TXT | 导出 Markdown 纯文本 |
| Word 兼容文档 | 生成带 Office 元数据的 UTF-8 HTML `.doc`，可由 Microsoft Word 打开并继续编辑 |

## 下载与安装

从 [GitHub Releases](https://github.com/chmod740/inkmark/releases/latest) 下载对应安装包：

- macOS 11 或更高版本：优先使用 `macos-universal.pkg` 原位升级，也提供同时支持 Apple Silicon 与 Intel 的 DMG 和 ZIP
- Windows 10/11 x64：`windows-amd64-setup.exe`

macOS 与 Windows 安装包目前未使用商业代码签名证书。首次运行时系统可能显示 Gatekeeper 或 SmartScreen 提示，请确认文件来自本仓库 Release，并使用同一 Release 中的 `SHA256SUMS` 校验完整性。

## 快速使用

1. 直接启动墨笺，或在 Finder/资源管理器中使用墨笺打开 Markdown 文件。
2. 在左侧编辑，右侧查看实时预览。
3. 使用“视图”菜单切换工作区和主题，使用“格式”菜单插入常用 Markdown 标记。
4. 使用“文件”菜单保存文档，或导出 PDF、HTML、PNG、TXT 与 Word 兼容文档。
5. 在“设置”中选择自动、简体中文或 English。
6. 在“帮助 → 检查更新”查看新版本；可直接下载、校验并启动系统安装器，“关于墨笺”也会显示下载和安装状态。

## 离线与联网说明

编辑器运行时不会从 CDN 加载 JavaScript、CSS、字体、KaTeX、Mermaid 或高亮资源。打开、编辑、预览、保存和本地导出均可离线完成。

以下操作会按用户请求联网：

- 检查新版本时访问 GitHub Releases API
- 下载更新或打开源码仓库时访问 GitHub
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
pnpm --dir frontend test:update
pnpm --dir frontend test:ui
pnpm --dir frontend test:installer
node scripts/verify-offline.mjs
```

测试覆盖文件读写、未保存文档切换与原生关闭守卫、语言设置、原生菜单、安全升级下载与安装编排、原子预览提交、版本比较与 Release 响应、导出结构、外部资源策略、同步滚动、大文档锚点上限和离线资源完整性。

## 项目结构

- `app.go`：原生文件对话框、读写与导出保存
- `close_guard.go`：macOS 与 Windows 原生关闭前的保存守卫
- `app_state.go`：语言设置、单实例和系统文件打开
- `update.go`、`update_download.go`、`update_launch.go`：版本检测、安装包选择、安全下载校验和平台安装器编排
- `menu.go`：macOS 与 Windows 原生菜单
- `frontend/src/App.vue`：编辑器、实时预览、设置和关于页面
- `frontend/src/i18n.ts`：中英文界面与欢迎页文案
- `frontend/src/ui-state.ts`：文档状态与分栏顺序偏好
- `frontend/src/document-guard.ts`：未保存文档的安全切换守卫
- `frontend/src/export-document.ts`：HTML、PDF、PNG、TXT 和 Word 兼容导出
- `frontend/src/scroll-sync.ts`：稳定的双向滚动同步
- `frontend/src/preview-render.ts`：预览原子提交与 Mermaid 缓存
- `samples/markdown-rendering-test.md`：应用内置的中英双语综合渲染样例
- `scripts/`：构建、打包和回归测试脚本

## 版本与更新

发布版本使用 `v主版本.次版本.修订版本` 标签。墨笺只在用户打开“关于”或主动选择“检查更新”时查询最新 Release；发现更高版本后，会选择与当前操作系统和架构匹配的安装包。

作者：PengHu · 源码仓库：[github.com/chmod740/inkmark](https://github.com/chmod740/inkmark)
