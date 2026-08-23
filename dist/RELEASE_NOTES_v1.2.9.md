## 墨笺 Markdown v1.2.9

- 重构桌面工作区：侧边栏、文件树、底栏状态和编辑工具栏采用更紧凑一致的布局；支持平台窗口控件风格切换。
- 新增浏览器式文档标签页，支持溢出滚动、右键批量关闭、未保存内容保护，以及打开目录/WebDAV 后在多个 Markdown 文件间快速切换。
- 优化大型 Markdown 文档的预览：增加渲染性能埋点、预览与派生数据缓存、分阶段渲染和 Worker 处理；切换标签优先显示核心内容。
- 修复批量关闭标签时被大文档渲染阻塞的问题：后台标签不再逐个激活渲染，仅为最终保留的标签恢复预览。
- 完善 WebDAV 与本地目录文件管理、图片预览、Markdown 扩展渲染、搜索定位和编辑器辅助功能。
- 已完成 macOS 与 Windows 生产构建，以及完整自动化回归和实际界面验收。

## InkMark Markdown v1.2.9

- Refactors the desktop workspace with a more cohesive sidebar, file tree, status bar, and editor toolbar, including switchable platform window-control styles.
- Adds browser-like document tabs with overflow scrolling, context-menu bulk closing, unsaved-change protection, and fast switching across Markdown files opened from folders or WebDAV.
- Improves large-document previews with performance instrumentation, preview and derived-data caches, staged rendering, and Worker processing; tab switching prioritizes core content.
- Fixes batch tab closing being blocked by long-running document rendering: background tabs are no longer activated and rendered one by one, and preview resumes only for the final surviving tab.
- Expands WebDAV and local file handling, image preview, Markdown dialect rendering, search navigation, and editor assistance.
- Verified with macOS and Windows production builds, the complete automated regression suite, and hands-on UI checks.

### 安装 / Installation

- macOS：推荐 Universal PKG；同时提供 DMG 和 ZIP，支持 Intel 与 Apple Silicon。
- Windows：提供 amd64 安装版和便携版。
- 使用 `SHA256SUMS` 校验下载文件。
- 安装包未使用商业签名证书，Gatekeeper 或 SmartScreen 可能显示提示。
- macOS: the Universal PKG is recommended; DMG and ZIP are also available for Intel and Apple Silicon.
- Windows: amd64 setup and portable builds are included.
- Verify downloads with `SHA256SUMS`.
- The packages are not commercially signed, so Gatekeeper or SmartScreen may show a warning.
