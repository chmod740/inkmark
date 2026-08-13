package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
)

func TestNormalizeMenuLocale(t *testing.T) {
	tests := map[string]string{
		"zh":      "zh-CN",
		"zh_CN":   "zh-CN",
		"zh-Hans": "zh-CN",
		"en":      "en",
		"en-US":   "en",
		"ja-JP":   "en",
		"":        "en",
	}
	for input, want := range tests {
		if got := normalizeMenuLocale(input); got != want {
			t.Errorf("normalizeMenuLocale(%q): expected %q, got %q", input, want, got)
		}
	}
}

func TestExpandedMenuTopLevels(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		locale   string
		want     []string
	}{
		{
			name:     "macOS Chinese",
			platform: "darwin",
			locale:   "zh-CN",
			want:     []string{"墨笺", "文件", "编辑", "视图", "格式", "窗口", "帮助"},
		},
		{
			name:     "macOS English",
			platform: "darwin",
			locale:   "en-US",
			want:     []string{"InkMark", "File", "Edit", "View", "Format", "Window", "Help"},
		},
		{
			name:     "Windows Chinese",
			platform: "windows",
			locale:   "zh-CN",
			want:     []string{"文件", "编辑", "视图", "格式", "帮助"},
		},
		{
			name:     "Windows English",
			platform: "windows",
			locale:   "en",
			want:     []string{"File", "Edit", "View", "Format", "Help"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			applicationMenu := NewApp().applicationMenuFor(test.platform, test.locale)
			got := make([]string, 0, len(applicationMenu.Items))
			for _, item := range applicationMenu.Items {
				got = append(got, item.Label)
				if item.Role != 0 {
					t.Fatalf("top-level menu %q unexpectedly uses a fixed-language Wails role", item.Label)
				}
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("expected top-level menus %q, got %q", test.want, got)
			}
			assertMenuLeafCallbacks(t, applicationMenu)
		})
	}
}

func TestMenuSelectionDefaults(t *testing.T) {
	applicationMenu := NewApp().applicationMenuFor("windows", "en")
	viewMenu := findTopLevelMenu(t, applicationMenu, "View")
	if len(viewMenu.Items) < 6 {
		t.Fatalf("View menu is incomplete: %#v", viewMenu.Items)
	}
	for index, checked := range []bool{false, true, false} {
		item := viewMenu.Items[index]
		if item.Type != menu.RadioType || item.Checked != checked {
			t.Errorf("view mode item %d: expected radio checked=%t, got type=%v checked=%t", index, checked, item.Type, item.Checked)
		}
	}
	if syncItem := findMenuItem(t, viewMenu, "Synchronized Scrolling"); syncItem.Type != menu.CheckboxType || !syncItem.Checked {
		t.Errorf("synchronized scrolling should be checked by default: %#v", syncItem)
	}
	if previewItem := findMenuItem(t, viewMenu, "Preview Pane on Left"); previewItem.Type != menu.CheckboxType || previewItem.Checked {
		t.Errorf("preview-first should be unchecked by default: %#v", previewItem)
	}
	styleMenu := findMenuItem(t, viewMenu, "Preview Style").SubMenu
	if styleMenu == nil || len(styleMenu.Items) != 12 {
		t.Fatalf("preview style submenu is incomplete")
	}
	for index, checked := range []bool{true, false, false, false, false, false, false, false, false, false, false, false} {
		item := styleMenu.Items[index]
		if item.Type != menu.RadioType || item.Checked != checked {
			t.Errorf("preview style item %d: expected radio checked=%t, got type=%v checked=%t", index, checked, item.Type, item.Checked)
		}
	}
	for _, label := range []string{"Mist", "Rice Paper", "Pine Ink", "Sakura Gray", "Ocean Salt", "Midnight Indigo", "Polar Night", "Obsidian"} {
		if item := findMenuItem(t, styleMenu, label); item.Type != menu.RadioType || item.Click == nil {
			t.Errorf("new preview theme %q must be a clickable radio item: %#v", label, item)
		}
	}
}

func TestPreviewFirstMenuSelection(t *testing.T) {
	app := NewApp()
	app.menuState.PreviewFirst = true
	viewMenu := findTopLevelMenu(t, app.applicationMenuFor("windows", "en"), "View")
	item := findMenuItem(t, viewMenu, "Preview Pane on Left")
	if item.Type != menu.CheckboxType || !item.Checked {
		t.Fatalf("preview-first menu state was not reflected: %#v", item)
	}
}

func TestPlatformSpecificMenuShortcuts(t *testing.T) {
	macMenu := NewApp().applicationMenuFor("darwin", "en")
	macEdit := findTopLevelMenu(t, macMenu, "Edit")
	if got := keys.Stringify(macEdit.Items[1].Accelerator, "darwin"); got != "Cmd+Shift+Z" {
		t.Fatalf("unexpected macOS Redo shortcut: %s", got)
	}
	macView := findTopLevelMenu(t, macMenu, "View")
	if got := keys.Stringify(macView.Items[len(macView.Items)-1].Accelerator, "darwin"); got != "Cmd+Ctrl+F" {
		t.Fatalf("unexpected macOS Full Screen shortcut: %s", got)
	}

	windowsMenu := NewApp().applicationMenuFor("windows", "en")
	windowsEdit := findTopLevelMenu(t, windowsMenu, "Edit")
	windowsRedo := findMenuItemByLabelPrefix(t, windowsEdit, "Redo")
	if windowsRedo.Label != "Redo\tCtrl+Y" || windowsRedo.Accelerator != nil {
		t.Fatalf("Windows Redo must show Ctrl+Y without registering a native accelerator: %#v", windowsRedo)
	}
	windowsFile := findTopLevelMenu(t, windowsMenu, "File")
	if got := keys.Stringify(windowsFile.Items[len(windowsFile.Items)-1].Accelerator, "windows"); got != "Alt+F4" {
		t.Fatalf("unexpected Windows Exit shortcut: %s", got)
	}
}

func TestWindowsEditMenuUsesDisplayOnlyShortcutsAndCallbacksRemainAvailable(t *testing.T) {
	type editMenuExpectation struct {
		label       string
		accelerator string
	}
	tests := []struct {
		name      string
		platform  string
		locale    string
		menuLabel string
		want      map[string]editMenuExpectation
	}{
		{
			name:      "macOS English",
			platform:  "darwin",
			locale:    "en",
			menuLabel: "Edit",
			want: map[string]editMenuExpectation{
				"Undo":       {label: "Undo", accelerator: "Cmd+Z"},
				"Redo":       {label: "Redo", accelerator: "Cmd+Shift+Z"},
				"Cut":        {label: "Cut", accelerator: "Cmd+X"},
				"Copy":       {label: "Copy", accelerator: "Cmd+C"},
				"Paste":      {label: "Paste", accelerator: "Cmd+V"},
				"Select All": {label: "Select All", accelerator: "Cmd+A"},
				"Find…":      {label: "Find…", accelerator: "Cmd+F"},
			},
		},
		{
			name:      "Windows English",
			platform:  "windows",
			locale:    "en",
			menuLabel: "Edit",
			want: map[string]editMenuExpectation{
				"Undo":       {label: "Undo\tCtrl+Z"},
				"Redo":       {label: "Redo\tCtrl+Y"},
				"Cut":        {label: "Cut\tCtrl+X"},
				"Copy":       {label: "Copy\tCtrl+C"},
				"Paste":      {label: "Paste\tCtrl+V"},
				"Select All": {label: "Select All\tCtrl+A"},
				"Find…":      {label: "Find…\tCtrl+F"},
			},
		},
		{
			name:      "Windows Chinese",
			platform:  "windows",
			locale:    "zh-CN",
			menuLabel: "编辑",
			want: map[string]editMenuExpectation{
				"撤销": {label: "撤销\tCtrl+Z"},
				"重做": {label: "重做\tCtrl+Y"},
				"剪切": {label: "剪切\tCtrl+X"},
				"复制": {label: "复制\tCtrl+C"},
				"粘贴": {label: "粘贴\tCtrl+V"},
				"全选": {label: "全选\tCtrl+A"},
				"查找…": {label: "查找…\tCtrl+F"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			editMenu := findTopLevelMenu(t, NewApp().applicationMenuFor(test.platform, test.locale), test.menuLabel)
			for labelPrefix, want := range test.want {
				item := findMenuItemByLabelPrefix(t, editMenu, labelPrefix)
				if item.Label != want.label {
					t.Errorf("%s label: expected %q, got %q", labelPrefix, want.label, item.Label)
				}
				if want.accelerator == "" {
					if item.Accelerator != nil {
						t.Errorf("%s must leave the Windows WebView shortcut unshadowed; got %q", labelPrefix, keys.Stringify(item.Accelerator, test.platform))
					}
				} else if item.Accelerator == nil {
					t.Errorf("%s accelerator: expected %q, got none", labelPrefix, want.accelerator)
				} else if got := keys.Stringify(item.Accelerator, test.platform); got != want.accelerator {
					t.Errorf("%s accelerator: expected %q, got %q", labelPrefix, want.accelerator, got)
				}
				if item.Click == nil {
					t.Errorf("%s must remain bound to its native menu callback", labelPrefix)
				}
			}
		})
	}
}

func TestNativeMenuTranslationsAreComplete(t *testing.T) {
	for key, translation := range nativeMenuText {
		if translation.zh == "" || translation.en == "" {
			t.Errorf("menu translation %q is incomplete: %#v", key, translation)
		}
	}
}

func TestFormatMenuContainsLocalizedImageImport(t *testing.T) {
	for _, test := range []struct {
		platform string
		locale   string
		format   string
		image    string
	}{
		{platform: "darwin", locale: "zh-CN", format: "格式", image: "插入图片…"},
		{platform: "darwin", locale: "en", format: "Format", image: "Insert Image…"},
		{platform: "windows", locale: "zh-CN", format: "格式", image: "插入图片…"},
		{platform: "windows", locale: "en", format: "Format", image: "Insert Image…"},
	} {
		t.Run(test.platform+"/"+test.locale, func(t *testing.T) {
			formatMenu := findTopLevelMenu(t, NewApp().applicationMenuFor(test.platform, test.locale), test.format)
			item := findMenuItem(t, formatMenu, test.image)
			if item.Click == nil || item.Disabled {
				t.Fatalf("image import item must be enabled with a callback: %#v", item)
			}
		})
	}
}

func TestHelpMenuContainsUpdateAndAboutActionsWithoutRepository(t *testing.T) {
	applicationMenu := NewApp().applicationMenuFor("windows", "en")
	helpMenu := findTopLevelMenu(t, applicationMenu, "Help")
	want := map[string]bool{
		"Rendering Test Page": false,
		"Check for Updates…":  false,
		"About InkMark":       false,
	}
	for _, item := range helpMenu.Items {
		if _, ok := want[item.Label]; ok {
			want[item.Label] = true
		}
	}
	for label, found := range want {
		if !found {
			t.Errorf("Help menu is missing %q", label)
		}
	}

	for _, test := range []struct {
		platform  string
		locale    string
		helpLabel string
		forbidden []string
	}{
		{platform: "darwin", locale: "zh-CN", helpLabel: "帮助", forbidden: []string{"升级墨笺…", "源码仓库"}},
		{platform: "darwin", locale: "en", helpLabel: "Help", forbidden: []string{"Upgrade InkMark…", "Source Repository"}},
		{platform: "windows", locale: "zh-CN", helpLabel: "帮助", forbidden: []string{"升级墨笺…", "源码仓库"}},
		{platform: "windows", locale: "en", helpLabel: "Help", forbidden: []string{"Upgrade InkMark…", "Source Repository"}},
	} {
		help := findTopLevelMenu(t, NewApp().applicationMenuFor(test.platform, test.locale), test.helpLabel)
		for _, item := range help.Items {
			for _, forbidden := range test.forbidden {
				if item.Label == forbidden {
					t.Errorf("%s/%s Help menu must not expose %q", test.platform, test.locale, forbidden)
				}
			}
		}
	}
}

func TestFileMenuContainsFolderAndEmptyRecentMenu(t *testing.T) {
	applicationMenu := (&App{menuState: MenuState{ViewMode: "split", Theme: "github", SyncScroll: true}}).applicationMenuFor("windows", "en")
	fileMenu := findTopLevelMenu(t, applicationMenu, "File")
	openFile := findMenuItem(t, fileMenu, "Open File…")
	if got := keys.Stringify(openFile.Accelerator, "windows"); got != "Ctrl+O" {
		t.Fatalf("unexpected Open File shortcut: %s", got)
	}
	openFolder := findMenuItem(t, fileMenu, "Open Folder…")
	if got := keys.Stringify(openFolder.Accelerator, "windows"); got != "Ctrl+Shift+O" {
		t.Fatalf("unexpected Open Folder shortcut: %s", got)
	}
	connectWebDAV := findMenuItem(t, fileMenu, "Connect to WebDAV…")
	if connectWebDAV.Click == nil {
		t.Fatal("Connect to WebDAV must emit a menu action")
	}
	recent := findMenuItem(t, fileMenu, "Recent")
	if recent.SubMenu == nil {
		t.Fatal("Recent must be a submenu")
	}
	empty := findMenuItem(t, recent.SubMenu, "No Recent Items")
	clear := findMenuItem(t, recent.SubMenu, "Clear Recent Items")
	if !empty.Disabled || !clear.Disabled {
		t.Fatalf("empty recent menu entries must be disabled: %#v / %#v", empty, clear)
	}
}

func TestFileMenuContainsLocalizedWebDAVConnection(t *testing.T) {
	for _, test := range []struct {
		platform string
		locale   string
		file     string
		connect  string
	}{
		{platform: "darwin", locale: "zh-CN", file: "文件", connect: "连接 WebDAV…"},
		{platform: "darwin", locale: "en", file: "File", connect: "Connect to WebDAV…"},
		{platform: "windows", locale: "zh-CN", file: "文件", connect: "连接 WebDAV…"},
		{platform: "windows", locale: "en", file: "File", connect: "Connect to WebDAV…"},
	} {
		t.Run(test.platform+"/"+test.locale, func(t *testing.T) {
			fileMenu := findTopLevelMenu(t, NewApp().applicationMenuFor(test.platform, test.locale), test.file)
			item := findMenuItem(t, fileMenu, test.connect)
			if item.Click == nil || item.Disabled {
				t.Fatalf("WebDAV connection item must be enabled with a callback: %#v", item)
			}
		})
	}
}

func TestRecentMenuIsBoundedAndUsesStructuredCallbacks(t *testing.T) {
	app := &App{menuState: MenuState{ViewMode: "split", Theme: "github", SyncScroll: true}}
	for index := 0; index < maxRecentItems+2; index++ {
		item, ok := makeRecentItem("file", filepath.Join("/tmp", fmt.Sprintf("project-%02d", index), "README.md"))
		if !ok {
			t.Fatal("failed to create recent menu fixture")
		}
		app.recentItems = prependRecentItem(app.recentItems, item)
	}
	recent := findMenuItem(t, findTopLevelMenu(t, app.applicationMenuFor("darwin", "zh-CN"), "文件"), "最近")
	if recent.SubMenu == nil || len(recent.SubMenu.Items) != maxRecentItems+2 {
		t.Fatalf("expected ten items, separator, and clear action: %#v", recent.SubMenu)
	}
	for _, item := range recent.SubMenu.Items[:maxRecentItems] {
		if item.Click == nil || !strings.Contains(item.Label, "README.md") || strings.ContainsAny(item.Label, "\r\n\t") {
			t.Errorf("invalid recent menu item: %#v", item)
		}
	}
	clear := recent.SubMenu.Items[len(recent.SubMenu.Items)-1]
	if clear.Label != "清除最近项目" || clear.Disabled {
		t.Fatalf("unexpected clear recent item: %#v", clear)
	}
}

func TestRecentMenuLabelSanitizesControlCharacters(t *testing.T) {
	item := RecentItem{Kind: "file", Path: filepath.Join("/tmp", "project", "bad.md"), Name: "bad\r\n\tname.md"}
	if got := recentMenuLabel(item); got != "bad name.md — project" {
		t.Fatalf("unexpected sanitized label: %q", got)
	}
}

func TestRecentWebDAVMenuLabelUsesOnlySafeHost(t *testing.T) {
	item, ok := makeRecentItem("webdav", "https://EXAMPLE.com:443/netdisk/api/webdav/")
	if !ok {
		t.Fatal("failed to create WebDAV recent menu fixture")
	}
	if got := recentMenuLabel(item); got != "WebDAV — example.com" {
		t.Fatalf("unexpected WebDAV recent menu label: %q", got)
	}
	if item.Kind != "webdav" {
		t.Fatalf("recent menu callback would emit an unexpected kind: %#v", item)
	}
}

func TestRecentWebDAVAppearsInNativeRecentMenus(t *testing.T) {
	app := &App{language: LanguageState{Mode: "auto", Locale: "en"}}
	item, ok := makeRecentItem("webdav", "https://example.com/private/dav/")
	if !ok {
		t.Fatal("failed to create WebDAV recent menu fixture")
	}
	app.recentItems = []RecentItem{item}

	for _, test := range []struct {
		platform string
		locale   string
		file     string
		recent   string
	}{
		{platform: "darwin", locale: "zh-CN", file: "文件", recent: "最近"},
		{platform: "darwin", locale: "en", file: "File", recent: "Recent"},
		{platform: "windows", locale: "zh-CN", file: "文件", recent: "最近"},
		{platform: "windows", locale: "en", file: "File", recent: "Recent"},
	} {
		t.Run(test.platform+"/"+test.locale, func(t *testing.T) {
			fileMenu := findTopLevelMenu(t, app.applicationMenuFor(test.platform, test.locale), test.file)
			recentMenu := findMenuItem(t, fileMenu, test.recent)
			if recentMenu.SubMenu == nil {
				t.Fatal("Recent must be a submenu")
			}
			connection := findMenuItem(t, recentMenu.SubMenu, "WebDAV — example.com")
			if connection.Click == nil || connection.Disabled {
				t.Fatalf("WebDAV recent item must be enabled with a callback: %#v", connection)
			}
		})
	}
}

func findMenuItem(t *testing.T, current *menu.Menu, label string) *menu.MenuItem {
	t.Helper()
	for _, item := range current.Items {
		if item.Label == label {
			return item
		}
	}
	t.Fatalf("menu item %q not found", label)
	return nil
}

func findMenuItemByLabelPrefix(t *testing.T, current *menu.Menu, labelPrefix string) *menu.MenuItem {
	t.Helper()
	for _, item := range current.Items {
		if item.Label == labelPrefix || strings.HasPrefix(item.Label, labelPrefix+"\t") {
			return item
		}
	}
	t.Fatalf("menu item with label prefix %q not found", labelPrefix)
	return nil
}

func findTopLevelMenu(t *testing.T, applicationMenu *menu.Menu, label string) *menu.Menu {
	t.Helper()
	for _, item := range applicationMenu.Items {
		if item.Label == label && item.SubMenu != nil {
			return item.SubMenu
		}
	}
	t.Fatalf("top-level menu %q not found", label)
	return nil
}

func assertMenuLeafCallbacks(t *testing.T, current *menu.Menu) {
	t.Helper()
	for _, item := range current.Items {
		if item.SubMenu != nil {
			assertMenuLeafCallbacks(t, item.SubMenu)
			continue
		}
		if item.Type != menu.SeparatorType && item.Click == nil {
			t.Errorf("menu item %q is missing a callback", item.Label)
		}
	}
}
