package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkspaceListsMarkdownImagesAndDirectoriesInStableKindOrder(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"z.md", "A.markdown", "z.png", "a.JPEG", "ignored.svg", "ignored.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	got := make([]string, 0, len(workspace.Entries))
	for _, entry := range workspace.Entries {
		got = append(got, entry.Kind+":"+entry.Name)
	}
	want := []string{"directory:folder", "markdown:A.markdown", "markdown:z.md", "image:a.JPEG", "image:z.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected mixed workspace order: got %q want %q", got, want)
	}
}

func TestWorkspaceFileOperationsAndImagePreview(t *testing.T) {
	root := t.TempDir()
	legacyDirectory := workspaceLegacyDirectoryNameForTest()
	if err := os.Mkdir(filepath.Join(root, legacyDirectory), 0o755); err != nil {
		t.Fatal(err)
	}
	pngPayload := makePNG(t, 6, 4)
	if err := os.WriteFile(filepath.Join(root, "preview.png"), pngPayload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "occupied.md"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)

	directory, err := app.CreateWorkspaceDirectory(workspace.ID, "notes")
	if err != nil || directory.Kind != "directory" || directory.AbsolutePath != filepath.Join(root, "notes") {
		t.Fatalf("unexpected created directory: %#v, %v", directory, err)
	}
	document, err := app.CreateWorkspaceMarkdownFile(workspace.ID, "notes/new.md")
	if err != nil {
		t.Fatal(err)
	}
	if document.WorkspaceID != workspace.ID || document.WorkspacePath != "notes/new.md" || document.LocalDocumentID == "" || document.StorageKind != "local" || document.Content != "" {
		t.Fatalf("unexpected created document: %#v", document)
	}
	if _, err := app.SaveWorkspaceMarkdownFile(workspace.ID, document.LocalDocumentID, "# saved\n"); err != nil {
		t.Fatal(err)
	}
	if payload, err := os.ReadFile(filepath.Join(root, "notes", "new.md")); err != nil || string(payload) != "# saved\n" {
		t.Fatalf("workspace save failed: %q, %v", payload, err)
	}
	renamed, err := app.RenameWorkspaceEntry(workspace.ID, "notes/new.md", "notes/Renamed.markdown", "markdown", workspaceRevisionForTest(t, app, "notes/new.md"))
	if err != nil || renamed.Path != "notes/Renamed.markdown" || renamed.AbsolutePath != filepath.Join(root, "notes", "Renamed.markdown") {
		t.Fatalf("unexpected renamed document: %#v, %v", renamed, err)
	}
	if _, err := app.RenameWorkspaceEntry(workspace.ID, "notes/Renamed.markdown", "occupied.md", "markdown", workspaceRevisionForTest(t, app, "notes/Renamed.markdown")); err == nil {
		t.Fatal("rename unexpectedly moved across directories or overwrote a destination")
	}
	if payload, err := os.ReadFile(filepath.Join(root, "occupied.md")); err != nil || string(payload) != "keep" {
		t.Fatalf("rename changed occupied destination: %q, %v", payload, err)
	}
	if _, err := app.RenameWorkspaceEntry(workspace.ID, "preview.png", "preview.jpg", "image", workspaceRevisionForTest(t, app, "preview.png")); err == nil {
		t.Fatal("image rename changed the encoded format family")
	}
	caseRenamed, err := app.RenameWorkspaceEntry(workspace.ID, "preview.png", "Preview.png", "image", workspaceRevisionForTest(t, app, "preview.png"))
	if err != nil || caseRenamed.Path != "Preview.png" {
		t.Fatalf("case-only image rename failed: %#v, %v", caseRenamed, err)
	}
	preview, err := app.ReadWorkspaceImage(workspace.ID, "Preview.png")
	if err != nil {
		t.Fatal(err)
	}
	decoded, decodeErr := base64.StdEncoding.DecodeString(preview.DataBase64)
	if decodeErr != nil || !bytes.Equal(decoded, pngPayload) || preview.Width != 6 || preview.Height != 4 || preview.MIMEType != "image/png" {
		t.Fatalf("workspace image preview changed payload: %#v, %v", preview, decodeErr)
	}
	legacyPortablePath := filepath.ToSlash(filepath.Join(legacyDirectory, "portable.md"))
	legacyRenamedPath := filepath.ToSlash(filepath.Join(legacyDirectory, "renamed.md"))
	if _, err := app.CreateWorkspaceMarkdownFile(workspace.ID, legacyPortablePath); err != nil {
		t.Fatalf("safe target inside a legacy directory was rejected: %v", err)
	}
	if _, err := app.RenameWorkspaceEntry(workspace.ID, legacyPortablePath, legacyRenamedPath, "markdown", workspaceRevisionForTest(t, app, legacyPortablePath)); err != nil {
		t.Fatalf("safe rename inside a legacy directory was rejected: %v", err)
	}
	if err := app.DeleteWorkspaceEntry(workspace.ID, legacyRenamedPath, false, workspaceRevisionForTest(t, app, legacyRenamedPath)); err != nil {
		t.Fatalf("deleting an existing entry inside a legacy directory was rejected: %v", err)
	}
	if _, err := app.CreateWorkspaceMarkdownFile(workspace.ID, "CON .md"); err == nil {
		t.Fatal("Windows reserved device alias was accepted")
	}
	if err := app.DeleteWorkspaceEntry(workspace.ID, "notes/Renamed.markdown", true, workspaceRevisionForTest(t, app, "notes/Renamed.markdown")); err == nil {
		t.Fatal("recursive file deletion was accepted")
	}
	if err := app.DeleteWorkspaceEntry(workspace.ID, "notes/Renamed.markdown", false, workspaceRevisionForTest(t, app, "notes/Renamed.markdown")); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteWorkspaceEntry(workspace.ID, "notes", false, workspaceRevisionForTest(t, app, "notes")); err == nil {
		t.Fatal("directory deletion without recursive confirmation was accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "hidden.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteWorkspaceEntry(workspace.ID, "notes", true, workspaceRevisionForTest(t, app, "notes")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "notes")); !os.IsNotExist(err) {
		t.Fatalf("recursive directory deletion left the source: %v", err)
	}
}

func TestNormalizeWorkspaceDestinationRejectsNonPortableNames(t *testing.T) {
	invalid := []string{
		"CON.md", "PRN.md", "AUX.md", "NUL.md", "CONIN$.md", "CONOUT$.md",
		"COM¹.md", "COM².md", "COM³.md", "LPT¹.md", "LPT².md", "LPT³.md",
		"trailing ", "trailing.", "less<.md", "greater>.md", `quote".md`, "pipe|.md", "question?.md", "star*.md", "colon:.md",
		"control\x00.md", "control\n.md", "../escape.md", `folder\escape.md`, strings.Repeat("a", 256) + ".md",
	}
	for number := 1; number <= 9; number++ {
		invalid = append(invalid, fmt.Sprintf("COM%d.md", number), fmt.Sprintf("LPT%d.md", number))
	}
	for _, candidate := range invalid {
		t.Run(fmt.Sprintf("%q", candidate), func(t *testing.T) {
			if normalized, err := normalizeWorkspaceDestinationPath(candidate); err == nil {
				t.Fatalf("non-portable destination was accepted as %q", normalized)
			}
		})
	}
}

func TestWorkspaceRecursiveDeletePreflightPreservesUnsafeTree(t *testing.T) {
	root := t.TempDir()
	victim := filepath.Join(root, "victim")
	if err := os.MkdirAll(filepath.Join(victim, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside-sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(victim, "nested", "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	if err := app.DeleteWorkspaceEntry(workspace.ID, "victim", true, workspaceRevisionForTest(t, app, "victim")); err == nil {
		t.Fatal("recursive deletion accepted a nested symlink")
	}
	if _, err := os.Lstat(victim); err != nil {
		t.Fatalf("failed preflight hid or removed the original directory: %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("failed preflight changed the nested symlink: %v", err)
	}
	if payload, err := os.ReadFile(outside); err != nil || string(payload) != "outside-sentinel" {
		t.Fatalf("failed preflight changed the outside target: %q, %v", payload, err)
	}
	quarantines, err := filepath.Glob(filepath.Join(root, ".inkmark-delete-*"))
	if err != nil || len(quarantines) != 0 {
		t.Fatalf("failed preflight leaked quarantine names: %q, %v", quarantines, err)
	}
}

func TestWorkspaceDeleteQuarantinePreservesConcurrentReplacement(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "victim", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "victim", "nested", "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	var hookErr error
	app.activeWorkspace.testAfterDeleteQuarantine = func(relativePath string) {
		if relativePath != "victim" {
			hookErr = os.ErrInvalid
			return
		}
		if err := os.Mkdir(filepath.Join(root, "victim"), 0o755); err != nil {
			hookErr = err
			return
		}
		hookErr = os.WriteFile(filepath.Join(root, "victim", "replacement.txt"), []byte("replacement-sentinel"), 0o644)
	}
	if err := app.DeleteWorkspaceEntry(workspace.ID, "victim", true, workspaceRevisionForTest(t, app, "victim")); err != nil {
		t.Fatal(err)
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	payload, err := os.ReadFile(filepath.Join(root, "victim", "replacement.txt"))
	if err != nil || string(payload) != "replacement-sentinel" {
		t.Fatalf("quarantined delete touched a concurrent replacement: %q, %v", payload, err)
	}
	quarantines, _ := filepath.Glob(filepath.Join(root, ".inkmark-delete-*"))
	if len(quarantines) != 0 {
		t.Fatalf("successful delete leaked quarantine names: %q", quarantines)
	}
}

func TestWorkspaceFileDeleteQuarantinePreservesConcurrentReplacement(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "victim.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	var hookErr error
	app.activeWorkspace.testAfterDeleteQuarantine = func(relativePath string) {
		if relativePath != "victim.md" {
			hookErr = os.ErrInvalid
			return
		}
		hookErr = os.WriteFile(filepath.Join(root, "victim.md"), []byte("replacement-sentinel"), 0o644)
	}
	if err := app.DeleteWorkspaceEntry(workspace.ID, "victim.md", false, workspaceRevisionForTest(t, app, "victim.md")); err != nil {
		t.Fatal(err)
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if payload, err := os.ReadFile(filepath.Join(root, "victim.md")); err != nil || string(payload) != "replacement-sentinel" {
		t.Fatalf("quarantined file delete touched a concurrent replacement: %q, %v", payload, err)
	}
	quarantines, _ := filepath.Glob(filepath.Join(root, ".inkmark-delete-*"))
	if len(quarantines) != 0 {
		t.Fatalf("successful file delete leaked quarantine names: %q", quarantines)
	}
}

func TestWorkspaceDeleteIdentityChangeBeforeQuarantinePreservesReplacement(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "victim.md")
	moved := filepath.Join(root, "moved-original.md")
	if err := os.WriteFile(source, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	var hookErr error
	app.activeWorkspace.testBeforeDeleteQuarantine = func(relativePath string) {
		if relativePath != "victim.md" {
			hookErr = os.ErrInvalid
			return
		}
		if err := os.Rename(source, moved); err != nil {
			hookErr = err
			return
		}
		hookErr = os.WriteFile(source, []byte("replacement-sentinel"), 0o644)
	}
	if err := app.DeleteWorkspaceEntry(workspace.ID, "victim.md", false, workspaceRevisionForTest(t, app, "victim.md")); err == nil {
		t.Fatal("delete accepted a source identity change before quarantine")
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if payload, err := os.ReadFile(source); err != nil || string(payload) != "replacement-sentinel" {
		t.Fatalf("identity mismatch changed replacement: %q, %v", payload, err)
	}
	if payload, err := os.ReadFile(moved); err != nil || string(payload) != "old" {
		t.Fatalf("identity mismatch changed moved original: %q, %v", payload, err)
	}
	quarantines, _ := filepath.Glob(filepath.Join(root, ".inkmark-delete-*"))
	if len(quarantines) != 0 {
		t.Fatalf("identity mismatch leaked quarantine names: %q", quarantines)
	}
}

func TestWorkspaceDeleteReportsCapabilityInvalidationAfterMutation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "victim.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	movedRoot := root + "-moved"
	defer os.RemoveAll(movedRoot)
	var hookErr error
	app.activeWorkspace.testAfterDeleteQuarantine = func(string) {
		if err := os.Rename(root, movedRoot); err != nil {
			hookErr = err
			return
		}
		if err := os.Mkdir(root, 0o755); err != nil {
			hookErr = err
			return
		}
		hookErr = os.WriteFile(filepath.Join(root, "replacement.md"), []byte("replacement-sentinel"), 0o644)
	}
	operationErr := app.DeleteWorkspaceEntry(workspace.ID, "victim.md", false, workspaceRevisionForTest(t, app, "victim.md"))
	if workspaceRootReplacementUnavailableForTest(hookErr) {
		if operationErr != nil {
			t.Fatalf("Windows workspace deletion failed after the open root rejected replacement: %v", operationErr)
		}
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			t.Fatalf("Windows workspace root changed after replacement was rejected: %#v, %v", info, err)
		}
		if _, err := os.Lstat(movedRoot); !os.IsNotExist(err) {
			t.Fatalf("Windows moved workspace unexpectedly exists: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(root, "victim.md")); !os.IsNotExist(err) {
			t.Fatalf("Windows deletion did not remove the confirmed entry: %v", err)
		}
		return
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if operationErr == nil || !strings.Contains(operationErr.Error(), "条目已删除") {
		t.Fatalf("delete did not report post-mutation capability invalidation: %v", operationErr)
	}
	if _, err := os.Lstat(filepath.Join(movedRoot, "victim.md")); !os.IsNotExist(err) {
		t.Fatalf("confirmed mutation did not delete the original entry: %v", err)
	}
	if payload, err := os.ReadFile(filepath.Join(root, "replacement.md")); err != nil || string(payload) != "replacement-sentinel" {
		t.Fatalf("delete touched replacement root: %q, %v", payload, err)
	}
}

func TestWorkspaceCreateReportsCapabilityInvalidationWithoutTouchingReplacement(t *testing.T) {
	for _, test := range []struct {
		name   string
		path   string
		create func(*App, string, string) error
	}{
		{name: "markdown", path: "created.md", create: func(app *App, workspaceID string, relativePath string) error {
			_, err := app.CreateWorkspaceMarkdownFile(workspaceID, relativePath)
			return err
		}},
		{name: "directory", path: "created-folder", create: func(app *App, workspaceID string, relativePath string) error {
			_, err := app.CreateWorkspaceDirectory(workspaceID, relativePath)
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			app := &App{}
			workspace, err := app.activateWorkspace(root)
			if err != nil {
				t.Fatal(err)
			}
			defer app.shutdown(nil)
			movedRoot := root + "-moved"
			defer os.RemoveAll(movedRoot)
			var hookErr error
			app.activeWorkspace.testAfterCreate = func(string) {
				if err := os.Rename(root, movedRoot); err != nil {
					hookErr = err
					return
				}
				if err := os.Mkdir(root, 0o755); err != nil {
					hookErr = err
					return
				}
				hookErr = os.WriteFile(filepath.Join(root, "replacement-sentinel"), []byte("replacement"), 0o644)
			}
			operationErr := test.create(app, workspace.ID, test.path)
			if workspaceRootReplacementUnavailableForTest(hookErr) {
				if operationErr != nil {
					t.Fatalf("Windows workspace create failed after the open root rejected replacement: %v", operationErr)
				}
				if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(test.path))); err != nil {
					t.Fatalf("Windows create did not retain the new entry in the open workspace: %v", err)
				}
				if _, err := os.Lstat(movedRoot); !os.IsNotExist(err) {
					t.Fatalf("Windows moved workspace unexpectedly exists: %v", err)
				}
				if _, err := os.Lstat(filepath.Join(root, "replacement-sentinel")); !os.IsNotExist(err) {
					t.Fatalf("Windows replacement sentinel unexpectedly exists: %v", err)
				}
				return
			}
			if hookErr != nil {
				t.Fatal(hookErr)
			}
			if operationErr == nil || !strings.Contains(operationErr.Error(), "已创建于原工作区") {
				t.Fatalf("create did not report post-mutation capability invalidation: %v", operationErr)
			}
			if _, err := os.Lstat(filepath.Join(movedRoot, filepath.FromSlash(test.path))); err != nil {
				t.Fatalf("created object was not retained in the original workspace: %v", err)
			}
			if payload, err := os.ReadFile(filepath.Join(root, "replacement-sentinel")); err != nil || string(payload) != "replacement" {
				t.Fatalf("create touched replacement root: %q, %v", payload, err)
			}
		})
	}
}

func TestWorkspaceRenameRollsBackWhenCapabilityRootPathIsReplaced(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.md")
	if err := os.WriteFile(source, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	movedRoot := root + "-moved"
	defer os.RemoveAll(movedRoot)
	var hookErr error
	app.activeWorkspace.testAfterRename = func(_, _ string) {
		if err := os.Rename(root, movedRoot); err != nil {
			hookErr = err
			return
		}
		if err := os.Mkdir(root, 0o755); err != nil {
			hookErr = err
			return
		}
		hookErr = os.WriteFile(filepath.Join(root, "replacement.md"), []byte("replacement-sentinel"), 0o644)
	}
	_, operationErr := app.RenameWorkspaceEntry(workspace.ID, "source.md", "renamed.md", "markdown", workspaceRevisionForTest(t, app, "source.md"))
	if workspaceRootReplacementUnavailableForTest(hookErr) {
		if operationErr != nil {
			t.Fatalf("Windows workspace rename failed after the open root rejected replacement: %v", operationErr)
		}
		if payload, err := os.ReadFile(filepath.Join(root, "renamed.md")); err != nil || string(payload) != "original" {
			t.Fatalf("Windows rename changed the confirmed entry: %q, %v", payload, err)
		}
		if _, err := os.Lstat(filepath.Join(root, "source.md")); !os.IsNotExist(err) {
			t.Fatalf("Windows rename left the source name behind: %v", err)
		}
		if _, err := os.Lstat(movedRoot); !os.IsNotExist(err) {
			t.Fatalf("Windows moved workspace unexpectedly exists: %v", err)
		}
		return
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if operationErr == nil || !strings.Contains(operationErr.Error(), "已恢复原名称") {
		t.Fatalf("capability replacement did not produce a rolled-back error: %v", operationErr)
	}
	if payload, err := os.ReadFile(filepath.Join(movedRoot, "source.md")); err != nil || string(payload) != "original" {
		t.Fatalf("rename rollback did not restore the original source: %q, %v", payload, err)
	}
	if _, err := os.Lstat(filepath.Join(movedRoot, "renamed.md")); !os.IsNotExist(err) {
		t.Fatalf("rename rollback left the destination behind: %v", err)
	}
	if payload, err := os.ReadFile(filepath.Join(root, "replacement.md")); err != nil || string(payload) != "replacement-sentinel" {
		t.Fatalf("rename rollback touched replacement root: %q, %v", payload, err)
	}
}

func TestWorkspaceRenameDoesNotLoseExistingHardlinkDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "a.md")
	destination := filepath.Join(root, "A.md")
	if err := os.WriteFile(source, []byte("shared"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, destination); err != nil {
		t.Skipf("hard links are unavailable or the volume is case-insensitive: %v", err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	if _, err := app.RenameWorkspaceEntry(workspace.ID, "a.md", "A.md", "markdown", workspaceRevisionForTest(t, app, "a.md")); err == nil {
		t.Fatal("rename unexpectedly replaced an existing hardlink destination")
	}
	for _, name := range []string{"a.md", "A.md"} {
		if payload, err := os.ReadFile(filepath.Join(root, name)); err != nil || string(payload) != "shared" {
			t.Fatalf("failed rename changed %s: %q, %v", name, payload, err)
		}
	}
}

func TestWorkspaceMutationRejectsSameKindReplacementAfterListing(t *testing.T) {
	for _, test := range []struct {
		name        string
		kind        string
		source      string
		destination string
		recursive   bool
		operation   string
	}{
		{name: "rename markdown", kind: "markdown", source: "source.md", destination: "renamed.md", operation: "rename"},
		{name: "delete markdown", kind: "markdown", source: "source.md", operation: "delete"},
		{name: "rename directory", kind: "directory", source: "source", destination: "renamed", operation: "rename"},
		{name: "delete directory", kind: "directory", source: "source", recursive: true, operation: "delete"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, test.source)
			moved := filepath.Join(root, "moved-original")
			if test.kind == "directory" {
				if err := os.Mkdir(source, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(source, "old.txt"), []byte("old"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(source, []byte("old"), 0o644); err != nil {
				t.Fatal(err)
			}
			app := &App{}
			workspace, err := app.activateWorkspace(root)
			if err != nil {
				t.Fatal(err)
			}
			defer app.shutdown(nil)
			revision := workspaceRevisionForTest(t, app, test.source)
			if err := os.Rename(source, moved); err != nil {
				t.Fatal(err)
			}
			if test.kind == "directory" {
				if err := os.Mkdir(source, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(source, "replacement.txt"), []byte("replacement-sentinel"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(source, []byte("replacement-sentinel"), 0o644); err != nil {
				t.Fatal(err)
			}
			if test.operation == "rename" {
				if _, err := app.RenameWorkspaceEntry(workspace.ID, test.source, test.destination, test.kind, revision); err == nil {
					t.Fatal("rename accepted a same-kind replacement after listing")
				}
				if _, err := os.Lstat(filepath.Join(root, test.destination)); !os.IsNotExist(err) {
					t.Fatalf("rejected rename created a destination: %v", err)
				}
			} else if err := app.DeleteWorkspaceEntry(workspace.ID, test.source, test.recursive, revision); err == nil {
				t.Fatal("delete accepted a same-kind replacement after listing")
			}
			if test.kind == "directory" {
				if payload, err := os.ReadFile(filepath.Join(source, "replacement.txt")); err != nil || string(payload) != "replacement-sentinel" {
					t.Fatalf("mutation changed replacement directory: %q, %v", payload, err)
				}
			} else if payload, err := os.ReadFile(source); err != nil || string(payload) != "replacement-sentinel" {
				t.Fatalf("mutation changed replacement file: %q, %v", payload, err)
			}
		})
	}
}

func workspaceRevisionForTest(t *testing.T, app *App, relativePath string) string {
	t.Helper()
	entry, err := workspaceEntryAtPath(app.activeWorkspace, relativePath)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Revision == "" {
		t.Fatalf("workspace entry %q did not receive a revision", relativePath)
	}
	return entry.Revision
}

func TestWorkspaceSaveVerifiedHandleNeverWritesConcurrentReplacement(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "document.md")
	moved := filepath.Join(root, "moved-original.md")
	if err := os.WriteFile(current, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	document, err := app.OpenWorkspaceFile(workspace.ID, "document.md")
	if err != nil {
		t.Fatal(err)
	}
	var hookErr error
	app.activeWorkspace.testAfterSaveHandleVerified = func() {
		if err := os.Rename(current, moved); err != nil {
			hookErr = err
			return
		}
		hookErr = os.WriteFile(current, []byte("replacement-sentinel"), 0o644)
	}
	if _, err := app.SaveWorkspaceMarkdownFile(workspace.ID, document.LocalDocumentID, "saved-through-handle"); err == nil || !strings.Contains(err.Error(), "路径在保存期间发生了变化") {
		t.Fatalf("concurrent path replacement was not reported: %v", err)
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if payload, err := os.ReadFile(current); err != nil || string(payload) != "replacement-sentinel" {
		t.Fatalf("save overwrote concurrent replacement: %q, %v", payload, err)
	}
	if payload, err := os.ReadFile(moved); err != nil || string(payload) != "saved-through-handle" {
		t.Fatalf("verified original handle was not the only object written: %q, %v", payload, err)
	}
}

func TestWorkspaceSaveRejectsMovedAndReplacedRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "document.md"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	document, err := app.OpenWorkspaceFile(workspace.ID, "document.md")
	if err != nil {
		t.Fatal(err)
	}
	moved := root + "-moved"
	if err := os.Rename(root, moved); err != nil {
		t.Skipf("platform cannot move an open workspace root: %v", err)
	}
	defer os.RemoveAll(moved)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(root, "document.md")
	if err := os.WriteFile(replacement, []byte("replacement-sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveWorkspaceMarkdownFile(workspace.ID, document.LocalDocumentID, "unsafe-write"); err == nil {
		t.Fatal("save accepted a moved and replaced workspace root")
	}
	if payload, err := os.ReadFile(replacement); err != nil || string(payload) != "replacement-sentinel" {
		t.Fatalf("save changed replacement root sentinel: %q, %v", payload, err)
	}
	if payload, err := os.ReadFile(filepath.Join(moved, "document.md")); err != nil || string(payload) != "original" {
		t.Fatalf("rejected save changed original moved root: %q, %v", payload, err)
	}
}

func TestWorkspaceDocumentCapabilityRejectsReplacementAfterOpen(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "document.md")
	original := filepath.Join(root, "original-moved.md")
	if err := os.WriteFile(visible, []byte("original-baseline"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	document, err := app.OpenWorkspaceFile(workspace.ID, "document.md")
	if err != nil || document.LocalDocumentID == "" {
		t.Fatalf("workspace open did not issue a local document capability: %#v, %v", document, err)
	}
	if err := os.Rename(visible, original); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(visible, []byte("replacement-sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveWorkspaceMarkdownFile(workspace.ID, document.LocalDocumentID, "unsafe-editor-content"); err == nil {
		t.Fatal("save accepted a same-path replacement created after the document was opened")
	}
	if payload, err := os.ReadFile(visible); err != nil || string(payload) != "replacement-sentinel" {
		t.Fatalf("save changed replacement sentinel: %q, %v", payload, err)
	}
	if payload, err := os.ReadFile(original); err != nil || string(payload) != "original-baseline" {
		t.Fatalf("rejected save changed the original object: %q, %v", payload, err)
	}
}

func TestWorkspaceDocumentCapabilityRebasesOnRenameAndRevokesOnDelete(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "child.md"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	workspace, err := app.activateWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	document, err := app.OpenWorkspaceFile(workspace.ID, "notes/child.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RenameWorkspaceEntry(
		workspace.ID,
		"notes",
		"renamed",
		"directory",
		workspaceRevisionForTest(t, app, "notes"),
	); err != nil {
		t.Fatal(err)
	}
	if session := app.activeWorkspace.documents[document.LocalDocumentID]; session.path != "renamed/child.md" {
		t.Fatalf("rename did not rebase local document capability: %#v", session)
	}
	if _, err := app.SaveWorkspaceMarkdownFile(workspace.ID, document.LocalDocumentID, "saved after rename"); err != nil {
		t.Fatal(err)
	}
	if payload, err := os.ReadFile(filepath.Join(root, "renamed", "child.md")); err != nil || string(payload) != "saved after rename" {
		t.Fatalf("rebased document did not save to renamed path: %q, %v", payload, err)
	}
	if err := app.DeleteWorkspaceEntry(
		workspace.ID,
		"renamed/child.md",
		false,
		workspaceRevisionForTest(t, app, "renamed/child.md"),
	); err != nil {
		t.Fatal(err)
	}
	if _, ok := app.activeWorkspace.documents[document.LocalDocumentID]; ok {
		t.Fatal("successful delete retained the local document capability")
	}
	if _, err := app.SaveWorkspaceMarkdownFile(workspace.ID, document.LocalDocumentID, "must not recreate"); err == nil {
		t.Fatal("revoked local document capability remained usable after delete")
	}
}

func TestWorkspaceDocumentCapabilityCannotSaveAfterWorkspaceCloseOrSwitch(t *testing.T) {
	for _, test := range []struct {
		name            string
		switchWorkspace bool
	}{
		{name: "close"},
		{name: "switch", switchWorkspace: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			visible := filepath.Join(root, "document.md")
			moved := filepath.Join(root, "original.md")
			if err := os.WriteFile(visible, []byte("original"), 0o644); err != nil {
				t.Fatal(err)
			}
			app := &App{}
			workspace, err := app.activateWorkspace(root)
			if err != nil {
				t.Fatal(err)
			}
			defer app.shutdown(nil)
			document, err := app.OpenWorkspaceFile(workspace.ID, "document.md")
			if err != nil {
				t.Fatal(err)
			}
			if test.switchWorkspace {
				if _, err := app.activateWorkspace(t.TempDir()); err != nil {
					t.Fatal(err)
				}
			} else {
				app.CloseWorkspace(workspace.ID)
			}
			if err := os.Rename(visible, moved); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(visible, []byte("replacement-sentinel"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := app.SaveWorkspaceMarkdownFile(workspace.ID, document.LocalDocumentID, "must-not-save"); err == nil {
				t.Fatal("closed workspace document capability remained usable")
			}
			if payload, err := os.ReadFile(visible); err != nil || string(payload) != "replacement-sentinel" {
				t.Fatalf("closed workspace save touched replacement: %q, %v", payload, err)
			}
		})
	}
}
