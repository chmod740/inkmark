# InkMark Markdown

[简体中文](README.md) · [Download Latest Release](https://github.com/chmod740/inkmark/releases/latest) · [Report an Issue](https://github.com/chmod740/inkmark/issues)

InkMark is a Markdown editor for macOS and Windows with local and WebDAV document editing, live preview, native menus, multiple reading themes, and complete document export. Everything required for editing, rendering, and exporting ships with the app; network features are used only when you explicitly connect to WebDAV, use a public image host, check for updates, or open an external link.

![InkMark split editor and live preview](docs/images/inkmark-editor.jpg)

## Highlights

- Open, edit, save, and save-as for `.md` and `.markdown` files
- Connect to an HTTPS WebDAV service from File → Connect to WebDAV, then browse, create, open, edit, and save Markdown files from the cloud directory sidebar
- Create, edit, connect, or delete reusable entries in the WebDAV connection window. Names and addresses live in app settings; usernames and passwords that you choose to save go only to macOS Keychain or Windows Credential Manager, never settings, Recent, or logs
- WebDAV write-lock and ETag concurrency protection for remote saves, with explicit reload, cancel, and overwrite choices when another DAV-lock-aware client changes a file
- Open a local folder or WebDAV directory as a workspace. The sidebar shows Markdown and supported images and opens images in a safe preview. Right-click the root or a folder to create content, or right-click an existing entry to rename or delete it; every file or folder deletion requires confirmation
- Reopen or clear up to 10 recently used files, folders, or WebDAV connections from File → Recent; an entry connects directly when usable system credentials exist, otherwise it opens the connection window for authentication
- Insert images from Format → Insert Image or the toolbar: copy into a local document assets folder, embed as a single-file Data URI, or upload into the active WebDAV document assets folder. Preview also supports public HTTPS image hosts
- Debounced 120 ms live rendering; Markdown, math, highlighting, and diagrams are completed off-screen and committed once so continuous typing does not flash or jitter
- Source-aware two-way scroll synchronization that reconciles after math and diagram reflow without idle drift
- Save, Don't Save, and Cancel choices before replacing or closing a modified document
- One-click swapping of editor and preview sides, persisted across launches
- Find Markdown source with `⌘/Ctrl+F`, including previous/next navigation, wrapping, match counts, and case-sensitive search
- Twelve preview themes: GitHub, Clean, WeChat, Dark, Mist, Rice Paper, Pine Ink, Sakura Gray, Ocean Salt, Midnight Indigo, Polar Night, and Obsidian
- Independent interface, prose, and code fonts: a cross-platform Heiti preset for interface/prose, system sans, reading serif, system monospace, and offline-bundled Fusion Pixel 12 Simplified/Traditional Chinese presets
- Native macOS and Windows menus for files, editing, views, formatting, help, and updates
- Automatic language detection plus explicit Simplified Chinese and English settings; automatic detection is the default
- A localized welcome page on normal launch; Finder or Explorer file launches open the requested document directly
- A built-in bilingual rendering test, available from Welcome and Help, covering extended dialects, tables, math, code, emoji, footnotes, alerts, safe HTML, Mermaid, mind maps, ECharts, ABC notation, and Graphviz
- One consistent icon for the app, file associations, desktop shortcuts, and in-app branding
- An About page with version, author, and update status
- GitHub Releases update checks with in-app download and checksum verification; after unsaved work is resolved, InkMark closes the old version and opens the system installer

## Markdown and Preview Support

- CommonMark and GitHub Flavored Markdown
- Tables, task lists, strikethrough, autolinks, emoji shortcodes, footnotes, and both GitHub and `NOTE:`-style alerts
- Definition lists, abbreviation definitions, `==mark==`, `H~2~O` subscripts, and `x^2^` superscripts
- A standalone `[TOC]` marker and trailing `{#custom-id .custom-class}` heading attributes; IDs are deterministically de-duplicated and user classes receive an isolated prefix
- `[[target]]` and `[[target|label]]` Wiki markers render as unresolved local visual markers; they never read workspace files or navigate to the network
- Bounded flat `key: value` Front Matter at the start of a document; YAML tags, anchors, nested structures, and external references are not executed
- Lightweight citations with `[@key]` plus a top-level `[@key]: reference text` definition. This is InkMark citation-lite, not a full Pandoc/CSL engine, and it never loads an external bibliography
- Plain-text `@username` tokens may be decorated visually; they do not query a user directory, send notifications, or create external links
- Inline and display KaTeX formulas
- Mermaid flowcharts, sequence diagrams, and other diagrams, plus two-space-indented `mindmap` lists converted into mind maps
- Restricted JSON ECharts line charts, static ABC music notation, and Graphviz SVG generated in a timeout-bounded Worker; all three are sanitized and cannot load external resources
- Highlighting for JavaScript, TypeScript, Python, Go, Rust, JSON, Shell, and other common languages
- Sanitization of raw HTML, blocking scripts, iframes, objects, embeds, and raw SVG injection
- Static PNG, JPEG, GIF, and WebP images rendered from local relative paths, Data URIs, private WebDAV-relative paths, or public HTTPS addresses
- Content-addressed local and WebDAV assets with format, byte-size, and dimension limits; private remote images reuse the active WebDAV session without placing authentication data in Markdown

## Export Formats

| Format | Description |
| --- | --- |
| PDF | Smart A4 pagination using the active theme |
| HTML | A complete UTF-8 page that may reference external styles, fonts, images, and media |
| PNG | A long image of the complete preview |
| TXT | Plain Markdown source text |
| Word-compatible document | UTF-8 HTML `.doc` with Office metadata that Microsoft Word can open and edit |

When Fusion Pixel is selected, HTML and Word-compatible exports embed the offline WOFF2 assets. Word versions without Data URI font support fall back to system fonts. Fusion Pixel currently supplies Regular 400 only, so bold and italic are synthesized; Mermaid text continues to use its restricted built-in font configuration.

## Download and Install

Download the appropriate installer from [GitHub Releases](https://github.com/chmod740/inkmark/releases/latest):

- macOS 11 or later (install current system updates for complete Graphviz WebView/WASM compatibility): use `macos-universal.pkg` for in-place upgrades; Universal DMG and ZIP packages are also provided for Apple Silicon and Intel
- Windows 10/11 x64: `windows-amd64-setup.exe`

The macOS and Windows packages are not currently signed with commercial distribution certificates. Gatekeeper or SmartScreen may therefore show a warning on first launch. Confirm that the file came from this repository's Release page and verify it with the accompanying `SHA256SUMS` file.

## Quick Start

1. Launch InkMark directly, or open a Markdown file with InkMark from Finder or Explorer.
2. Use File → Open Folder for a local directory sidebar, or File → Connect to WebDAV for a cloud directory. Right-click blank sidebar space or a folder to create a Markdown file or subfolder; right-click an existing entry to rename or delete it, and click an image entry to preview it.
3. After a successful WebDAV connection, the normalized complete server address, including its path, is added to Recent. You may save a reusable connection in the operating system credential vault. Selecting it from Recent connects directly when usable credentials exist; otherwise only the address is prefilled and authentication is requested again.
   If the same service also exposes web or API writers that bypass DAV locks, the server must provide atomic conditional updates or a shared lock across every write path for cross-channel concurrency protection.
4. Write Markdown in the editor and inspect the live result in the preview.
5. Use the View menu for editing layouts and themes, and the Format menu for common Markdown markup or images.
6. Save from the File menu or export to PDF, HTML, PNG, TXT, or a Word-compatible document. Save As creates a local copy of a WebDAV document.
7. Choose Automatic, Simplified Chinese, or English in Settings, and configure interface, prose, and code fonts independently. Preferences store only fixed presets, never arbitrary CSS font values.
8. Use Help → Check for Updates to download, verify, and start the system installer; About also reports download and installation status.

## Offline and Network Behaviour

The editor never loads JavaScript, CSS, fonts, KaTeX, Mermaid, ECharts, ABC, Graphviz, or highlighting assets from a CDN. Simplified and Traditional Fusion Pixel fonts ship with the app; HTML and Word-compatible exports embed the selected offline font assets. Local-relative images, Data URI images, and built-in diagrams can be previewed and exported completely offline inside the app; a standalone HTML document containing KaTeX or highlighted code may still reference its existing external stylesheet URLs.

Network access occurs only for user-requested actions:

- Connecting to WebDAV, listing remote folders, opening or saving remote Markdown files, and uploading or reading private remote images
- Previewing public HTTPS images or embedding them into PDF, PNG, and Word-compatible exports; HTML exports retain the original public address
- Checking GitHub Releases for a new version
- Downloading an update from GitHub
- Opening an external link in a document
- Loading external resources referenced by an exported HTML file

## Build from Source

InkMark uses Go, [Wails v2](https://wails.io/), and Vue 3. Building requires Go 1.25+, Node.js, pnpm, and the native toolchain for the target platform.

macOS:

```bash
./scripts/build-macos.sh
./scripts/package-macos.sh
```

The scripts build a Universal app and create PKG, DMG, ZIP, and checksum files under `dist/`.

Windows:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1
```

The Windows build requires NSIS and the WebView2 Runtime. It creates a per-user installer and registers `.md` and `.markdown` file associations.

## Tests

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

The suite covers local file I/O, WebDAV authentication and directory parsing, operating-system credential-vault connection management, path encoding, ETag conflicts, remote saves, local and WebDAV workspace creation/rename/recursive deletion, safe image-entry preview, local and WebDAV image import, all four image rendering modes, emoji/footnotes/alerts/mentions, definition lists/abbreviations/mark/subscript/superscript, TOC/heading attributes/Wiki/Front Matter/citation-lite boundaries and malicious inputs, font-preset persistence and offline embedding, mind maps and the input limits plus malicious-payload rejection for three static diagram engines, public-image network boundaries, capability-bound workspace access, lazy sidebar expansion and context menus, recent items, unsaved-document transitions and the native close guard, language settings, native menus, verified update downloads and installer orchestration, atomic preview commits, version comparison and Release responses, export structure, external-resource policy, scroll synchronization, bounded large-document anchors, and offline asset integrity. Live WebDAV tests run only when a test endpoint and credentials are injected at runtime, and they remove their randomly named temporary resources.

## Project Layout

- `app.go`: native dialogs, file I/O, and export saving
- `close_guard.go`: save guard for native close requests on macOS and Windows
- `app_state.go`: language preferences, single-instance handling, and OS file-open events
- `workspace.go` and `recent.go`: capability-bound local folders, Markdown and image enumeration, safe file operations, and recent items
- `webdav.go` and `webdav_app.go`: HTTPS WebDAV protocol handling, remote capability sessions, directory and file operations, and write-lock plus ETag-safe saves
- `webdav_connections.go`: reusable WebDAV metadata, operating-system credential vault access, and direct reconnect from Recent
- `image_assets.go`: local and WebDAV image import, relative resource resolution, and restricted public HTTPS fetching
- `update.go`, `update_download.go`, and `update_launch.go`: release checks, installer selection, verified downloads, and platform-installer orchestration
- `menu.go`: native macOS and Windows menus
- `frontend/src/App.vue`: editor, live preview, Settings, and About
- `frontend/src/DirectorySidebar.vue` and `workspace-tree.ts`: folder sidebar, image entries, right-click file operations, and lazy tree state
- `frontend/src/i18n.ts`: Chinese and English UI and welcome-page content
- `frontend/src/ui-state.ts`: document status and pane-order preferences
- `frontend/src/document-guard.ts`: safe transitions for documents with unsaved changes
- `frontend/src/export-document.ts`: HTML, PDF, PNG, TXT, and Word-compatible exports
- `frontend/src/scroll-sync.ts`: stable bidirectional scroll synchronization
- `frontend/src/preview-render.ts`: atomic preview commits and Mermaid caching
- `frontend/src/markdown-extensions.ts`: emoji, footnotes, callouts, mentions, mind maps, and extended Markdown dialect adaptation
- `frontend/src/font-preferences.ts` and `font-preferences.css`: three independent font scopes, fixed safe stacks, offline Fusion Pixel assets, and export embedding
- `frontend/src/extended-diagrams.ts` and `graphviz-worker.ts`: restricted ECharts, ABC, and isolated Graphviz static rendering
- `frontend/src/image-resources.ts`: four image-source classes, preview-resource lifecycle, and export materialization
- `samples/markdown-rendering-test.md`: built-in bilingual comprehensive rendering sample
- `scripts/`: build, packaging, and regression-test scripts

## Versions and Updates

Releases use `vMAJOR.MINOR.PATCH` tags. InkMark queries the latest Release only when the About dialog is opened or you explicitly choose Check for Updates. When a newer version exists, it selects an installer matching the current operating system and architecture.

Author: Codex · Source: [github.com/chmod740/inkmark](https://github.com/chmod740/inkmark)
