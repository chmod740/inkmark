package main

import (
	"reflect"
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
	if syncItem := viewMenu.Items[4]; syncItem.Type != menu.CheckboxType || !syncItem.Checked {
		t.Errorf("synchronized scrolling should be checked by default: %#v", syncItem)
	}
	styleMenu := viewMenu.Items[5].SubMenu
	if styleMenu == nil || len(styleMenu.Items) != 4 {
		t.Fatalf("preview style submenu is incomplete: %#v", viewMenu.Items[5])
	}
	for index, checked := range []bool{true, false, false, false} {
		item := styleMenu.Items[index]
		if item.Type != menu.RadioType || item.Checked != checked {
			t.Errorf("preview style item %d: expected radio checked=%t, got type=%v checked=%t", index, checked, item.Type, item.Checked)
		}
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
	if got := keys.Stringify(windowsEdit.Items[1].Accelerator, "windows"); got != "Ctrl+Y" {
		t.Fatalf("unexpected Windows Redo shortcut: %s", got)
	}
	windowsFile := findTopLevelMenu(t, windowsMenu, "File")
	if got := keys.Stringify(windowsFile.Items[len(windowsFile.Items)-1].Accelerator, "windows"); got != "Alt+F4" {
		t.Fatalf("unexpected Windows Exit shortcut: %s", got)
	}
}

func TestNativeMenuTranslationsAreComplete(t *testing.T) {
	for key, translation := range nativeMenuText {
		if translation.zh == "" || translation.en == "" {
			t.Errorf("menu translation %q is incomplete: %#v", key, translation)
		}
	}
}

func TestHelpMenuContainsUpdateAndRepositoryActions(t *testing.T) {
	applicationMenu := NewApp().applicationMenuFor("windows", "en")
	helpMenu := findTopLevelMenu(t, applicationMenu, "Help")
	want := map[string]bool{
		"Check for Updates…": false,
		"Upgrade InkMark…":   false,
		"Source Repository":  false,
		"About InkMark":      false,
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
