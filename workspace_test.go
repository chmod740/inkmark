package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkspaceListsMarkdownAndDirectoriesLazily(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"b-dir", "A-dir", "looks-like.md", filepath.Join("A-dir", "nested")} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range map[string]string{
		"B.md":                          "# B",
		"a.markdown":                    "# A",
		"ignore.txt":                    "ignore",
		filepath.Join("A-dir", "嵌套.md"): "# nested",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "B.md"), filepath.Join(root, "linked.md")); err != nil {
		t.Logf("symlink unavailable: %v", err)
	}

	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	if workspace.ID == "" || strings.Contains(workspace.ID, root) {
		t.Fatalf("workspace ID must be opaque: %q", workspace.ID)
	}
	if workspace.Path != root || workspace.Name != filepath.Base(root) || workspace.Truncated {
		t.Fatalf("unexpected workspace metadata: %#v", workspace)
	}
	wantNames := []string{"A-dir", "b-dir", "looks-like.md", "a.markdown", "B.md"}
	gotNames := make([]string, 0, len(workspace.Entries))
	for _, entry := range workspace.Entries {
		gotNames = append(gotNames, entry.Name)
		if !filepath.IsAbs(entry.AbsolutePath) || !strings.HasPrefix(entry.AbsolutePath, root) {
			t.Errorf("entry has invalid absolute path: %#v", entry)
		}
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("expected entries %q, got %q", wantNames, gotNames)
	}
	if workspace.Entries[2].Kind != "directory" || workspace.Entries[3].Kind != "markdown" {
		t.Fatalf("directories must sort before Markdown files: %#v", workspace.Entries)
	}

	nested, err := app.ReadWorkspaceDirectory(workspace.ID, "A-dir")
	if err != nil {
		t.Fatal(err)
	}
	if nested.Path != "A-dir" || len(nested.Entries) != 2 || nested.Entries[0].Name != "nested" || nested.Entries[1].Name != "嵌套.md" {
		t.Fatalf("unexpected lazy directory response: %#v", nested)
	}
}

func TestWorkspaceRejectsTraversalSymlinksAndNonMarkdownFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "valid.MD"), []byte("# valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "not-markdown.txt"), []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkCreated := os.Symlink(outside, filepath.Join(root, "linked.md")) == nil

	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	document, err := app.OpenWorkspaceFile(workspace.ID, "valid.MD")
	if err != nil || document.Content != "# valid" || document.Path != filepath.Join(root, "valid.MD") {
		t.Fatalf("valid Markdown file did not open: %#v, %v", document, err)
	}
	if recent := app.recentItemsSnapshot(); len(recent) < 2 || recent[0].Kind != "file" || recent[0].Path != document.Path || recent[1].Kind != "directory" {
		t.Fatalf("workspace and opened Markdown were not synchronously added to recent items: %#v", recent)
	}
	for _, invalid := range []string{"../outside.md", "/outside.md", "not-markdown.txt", "."} {
		if _, err := app.OpenWorkspaceFile(workspace.ID, invalid); err == nil {
			t.Errorf("expected %q to be rejected", invalid)
		}
	}
	if symlinkCreated {
		if _, err := app.OpenWorkspaceFile(workspace.ID, "linked.md"); err == nil {
			t.Fatal("expected a symlinked Markdown file to be rejected")
		}
	}
	if _, err := app.ReadWorkspaceDirectory(workspace.ID, "../"); err == nil {
		t.Fatal("expected directory traversal to be rejected")
	}
	if _, err := app.ReadWorkspaceDirectory("wrong-capability", "."); err == nil {
		t.Fatal("expected a stale workspace ID to be rejected")
	}
}

func TestWorkspaceCapabilitiesRemainIndependentUntilClosed(t *testing.T) {
	app := &App{}
	first, err := app.activateWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.mu.RLock()
	firstRoot := app.activeWorkspace.root
	app.mu.RUnlock()
	second, err := app.activateWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("workspace capabilities must not be reused")
	}
	if _, err := firstRoot.Stat("."); err != nil {
		t.Fatalf("opening a second workspace closed the first capability: %v", err)
	}
	app.CloseWorkspace(first.ID)
	if _, err := firstRoot.Stat("."); err == nil {
		t.Fatal("closing the first workspace did not revoke its root")
	}
	if _, err := app.ReadWorkspaceDirectory(second.ID, "."); err != nil {
		t.Fatalf("closing the first workspace affected the second: %v", err)
	}
	app.CloseWorkspace(second.ID)
	if _, err := app.ReadWorkspaceDirectory(second.ID, "."); err == nil {
		t.Fatal("matching close request did not revoke the capability")
	}
}

func TestWorkspaceRejectsMovedOrReplacedRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "document.md"), []byte("# original"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	moved := root + "-moved"
	if err := os.Rename(root, moved); err != nil {
		t.Skipf("platform does not allow moving an open directory: %v", err)
	}
	defer os.RemoveAll(moved)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "document.md"), []byte("# replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ReadWorkspaceDirectory(workspace.ID, "."); err == nil || !strings.Contains(err.Error(), "移动或替换") {
		t.Fatalf("expected replaced workspace root to be rejected, got %v", err)
	}
	if _, err := app.OpenWorkspaceFile(workspace.ID, "document.md"); err == nil || !strings.Contains(err.Error(), "移动或替换") {
		t.Fatalf("expected file open through replaced workspace root to fail, got %v", err)
	}
}

func TestWorkspaceStreamingFilterFindsMarkdownAfterManyIgnoredFiles(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 4200; index++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("%05d.txt", index)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "zzzz-after-ignored.md"), []byte("# found"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	if workspace.Truncated || len(workspace.Entries) != 1 || workspace.Entries[0].Name != "zzzz-after-ignored.md" {
		t.Fatalf("streaming filter omitted late Markdown: %#v", workspace)
	}
}

func TestWorkspaceDirectoryResultsAreBounded(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < maxWorkspaceDirectoryResults+25; index++ {
		name := filepath.Join(root, fmt.Sprintf("file-%05d.md", index))
		if err := os.WriteFile(name, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	if !workspace.Truncated || len(workspace.Entries) != maxWorkspaceDirectoryResults {
		t.Fatalf("expected a bounded truncated result, got %d entries, truncated=%t", len(workspace.Entries), workspace.Truncated)
	}
	if workspace.Entries[0].Name != "file-00000.md" || workspace.Entries[len(workspace.Entries)-1].Name != "file-01999.md" {
		t.Fatalf("bounded result must keep the stable first entries: first=%q last=%q", workspace.Entries[0].Name, workspace.Entries[len(workspace.Entries)-1].Name)
	}
}
