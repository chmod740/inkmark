package main

import (
	"container/heap"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	maxWorkspaceDirectoryResults = 2000
)

type WorkspaceEntry struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	AbsolutePath string `json:"absolutePath"`
	Kind         string `json:"kind"`
}

type WorkspaceDirectory struct {
	Path      string           `json:"path"`
	Entries   []WorkspaceEntry `json:"entries"`
	Truncated bool             `json:"truncated"`
}

type Workspace struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Path      string           `json:"path"`
	Entries   []WorkspaceEntry `json:"entries"`
	Truncated bool             `json:"truncated"`
}

// workspaceCapability deliberately keeps the filesystem root out of the
// Wails payload. The frontend receives an opaque ID and can only address
// descendants by validated relative paths.
type workspaceCapability struct {
	id   string
	path string
	root *os.Root
}

func (a *App) OpenDirectory() (Workspace, error) {
	english := a.currentLocale() == "en"
	title := "打开文件夹"
	if english {
		title = "Open Folder"
	}
	directory, err := runtime.OpenDirectoryDialog(a.currentContext(), runtime.OpenDialogOptions{Title: title})
	if err != nil {
		if english {
			return Workspace{}, fmt.Errorf("open folder dialog failed: %w", err)
		}
		return Workspace{}, fmt.Errorf("打开文件夹对话框失败: %w", err)
	}
	if strings.TrimSpace(directory) == "" {
		return Workspace{}, nil
	}
	return a.activateWorkspace(directory)
}

func (a *App) OpenRecentDirectory(recentID string) (Workspace, error) {
	item, err := a.recentItemByID(recentID, "directory")
	if err != nil {
		return Workspace{}, err
	}
	workspace, err := a.activateWorkspace(item.Path)
	if err != nil {
		a.removeRecentItemByID(item.ID)
	}
	return workspace, err
}

func (a *App) activateWorkspace(directory string) (Workspace, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return Workspace{}, fmt.Errorf("解析文件夹路径失败: %w", err)
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return Workspace{}, fmt.Errorf("打开文件夹失败: %w", err)
	}
	capability := &workspaceCapability{path: absolute, root: root}
	capability.id, err = newOpaqueID()
	if err != nil {
		_ = root.Close()
		return Workspace{}, fmt.Errorf("创建工作区标识失败: %w", err)
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		_ = root.Close()
		return Workspace{}, err
	}
	directoryData, err := scanWorkspaceDirectory(capability, ".")
	if err != nil {
		_ = root.Close()
		return Workspace{}, err
	}

	a.mu.Lock()
	previous := a.activeWorkspace
	a.activeWorkspace = capability
	a.mu.Unlock()
	if previous != nil {
		_ = previous.root.Close()
	}

	name := filepath.Base(filepath.Clean(absolute))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = absolute
	}
	workspace := Workspace{
		ID:        capability.id,
		Name:      name,
		Path:      capability.path,
		Entries:   directoryData.Entries,
		Truncated: directoryData.Truncated,
	}
	a.recordRecentItem("directory", capability.path)
	return workspace, nil
}

func (a *App) ReadWorkspaceDirectory(workspaceID string, relativePath string) (WorkspaceDirectory, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	capability, err := a.activeWorkspaceLocked(workspaceID)
	if err != nil {
		return WorkspaceDirectory{}, err
	}
	return scanWorkspaceDirectory(capability, relativePath)
}

func (a *App) OpenWorkspaceFile(workspaceID string, relativePath string) (Document, error) {
	a.mu.RLock()
	document, err := a.openWorkspaceFileLocked(workspaceID, relativePath)
	a.mu.RUnlock()
	if err == nil {
		a.recordRecentItem("file", document.Path)
	}
	return document, err
}

func (a *App) openWorkspaceFileLocked(workspaceID string, relativePath string) (Document, error) {
	capability, err := a.activeWorkspaceLocked(workspaceID)
	if err != nil {
		return Document{}, err
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return Document{}, err
	}
	relativePath, err = normalizeWorkspacePath(relativePath)
	if err != nil || relativePath == "." {
		return Document{}, errors.New("Markdown 文件路径无效")
	}
	if !isMarkdownFilename(relativePath) {
		return Document{}, errors.New("工作区只能打开 Markdown 文件")
	}
	if err := rejectWorkspaceSymlinks(capability.root, relativePath); err != nil {
		return Document{}, err
	}
	info, err := capability.root.Lstat(relativePath)
	if err != nil {
		return Document{}, fmt.Errorf("读取文件信息失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Document{}, errors.New("所选路径不是普通文件")
	}
	file, err := openRootReadOnlyNonBlocking(capability.root, relativePath)
	if err != nil {
		return Document{}, fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()
	absolute := filepath.Join(capability.path, filepath.FromSlash(relativePath))
	document, err := readDocumentFromFile(file, absolute)
	if err != nil {
		return Document{}, err
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return Document{}, err
	}
	return document, nil
}

func (a *App) OpenRecentFile(recentID string) (Document, error) {
	item, err := a.recentItemByID(recentID, "file")
	if err != nil {
		return Document{}, err
	}
	document, err := readDocument(item.Path)
	if err != nil {
		a.removeRecentItemByID(item.ID)
		return Document{}, err
	}
	a.recordRecentItem("file", document.Path)
	return document, nil
}

func (a *App) CloseWorkspace(workspaceID string) {
	a.mu.Lock()
	capability := a.activeWorkspace
	if capability == nil || capability.id != workspaceID {
		a.mu.Unlock()
		return
	}
	a.activeWorkspace = nil
	a.mu.Unlock()
	_ = capability.root.Close()
}

func (a *App) activeWorkspaceLocked(workspaceID string) (*workspaceCapability, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" || a.activeWorkspace == nil || a.activeWorkspace.id != workspaceID {
		return nil, errors.New("工作区已关闭或已被替换")
	}
	return a.activeWorkspace, nil
}

func scanWorkspaceDirectory(capability *workspaceCapability, relativePath string) (WorkspaceDirectory, error) {
	if err := validateWorkspaceCapability(capability); err != nil {
		return WorkspaceDirectory{}, err
	}
	relativePath, err := normalizeWorkspacePath(relativePath)
	if err != nil {
		return WorkspaceDirectory{}, err
	}
	if err := rejectWorkspaceSymlinks(capability.root, relativePath); err != nil {
		return WorkspaceDirectory{}, err
	}
	info, err := capability.root.Lstat(relativePath)
	if err != nil {
		return WorkspaceDirectory{}, fmt.Errorf("读取文件夹信息失败: %w", err)
	}
	if !info.IsDir() {
		return WorkspaceDirectory{}, errors.New("所选路径不是文件夹")
	}
	directoryRoot, err := capability.root.OpenRoot(relativePath)
	if err != nil {
		return WorkspaceDirectory{}, fmt.Errorf("打开文件夹失败: %w", err)
	}
	defer directoryRoot.Close()
	directory, err := directoryRoot.Open(".")
	if err != nil {
		return WorkspaceDirectory{}, fmt.Errorf("读取文件夹失败: %w", err)
	}
	defer directory.Close()

	entries := &workspaceEntryMaxHeap{}
	heap.Init(entries)
	truncated := false
	for {
		batch, readErr := directory.ReadDir(256)
		for _, entry := range batch {
			candidate, ok := workspaceEntryFromDirEntry(capability, relativePath, entry)
			if !ok {
				continue
			}
			if entries.Len() < maxWorkspaceDirectoryResults {
				heap.Push(entries, candidate)
				continue
			}
			truncated = true
			if workspaceEntryLess(candidate, (*entries)[0]) {
				(*entries)[0] = candidate
				heap.Fix(entries, 0)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return WorkspaceDirectory{}, fmt.Errorf("读取文件夹失败: %w", readErr)
		}
		if len(batch) == 0 {
			break
		}
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return WorkspaceDirectory{}, err
	}
	result := append([]WorkspaceEntry(nil), (*entries)...)
	sort.Slice(result, func(left, right int) bool { return workspaceEntryLess(result[left], result[right]) })
	return WorkspaceDirectory{Path: relativePath, Entries: result, Truncated: truncated}, nil
}

func validateWorkspaceCapability(capability *workspaceCapability) error {
	rootInfo, rootErr := capability.root.Stat(".")
	pathInfo, pathErr := os.Stat(capability.path)
	if rootErr != nil || pathErr != nil || !rootInfo.IsDir() || !pathInfo.IsDir() || !os.SameFile(rootInfo, pathInfo) {
		return errors.New("工作区目录已移动或替换，请重新打开文件夹")
	}
	return nil
}

func workspaceEntryFromDirEntry(capability *workspaceCapability, relativePath string, entry fs.DirEntry) (WorkspaceEntry, bool) {
	if strings.Contains(entry.Name(), "\\") {
		return WorkspaceEntry{}, false
	}
	entryInfo, err := entry.Info()
	if err != nil || entryInfo.Mode()&os.ModeSymlink != 0 {
		return WorkspaceEntry{}, false
	}
	kind := ""
	switch {
	case entryInfo.IsDir():
		kind = "directory"
	case entryInfo.Mode().IsRegular() && isMarkdownFilename(entry.Name()):
		kind = "markdown"
	default:
		return WorkspaceEntry{}, false
	}
	entryPath := entry.Name()
	if relativePath != "." {
		entryPath = path.Join(relativePath, entry.Name())
	}
	return WorkspaceEntry{
		Name:         entry.Name(),
		Path:         entryPath,
		AbsolutePath: filepath.Join(capability.path, filepath.FromSlash(entryPath)),
		Kind:         kind,
	}, true
}

func workspaceEntryLess(left WorkspaceEntry, right WorkspaceEntry) bool {
	if left.Kind != right.Kind {
		return left.Kind == "directory"
	}
	leftName := strings.ToLower(left.Name)
	rightName := strings.ToLower(right.Name)
	if leftName != rightName {
		return leftName < rightName
	}
	return left.Name < right.Name
}

type workspaceEntryMaxHeap []WorkspaceEntry

func (entries workspaceEntryMaxHeap) Len() int { return len(entries) }
func (entries workspaceEntryMaxHeap) Less(left int, right int) bool {
	return workspaceEntryLess(entries[right], entries[left])
}
func (entries workspaceEntryMaxHeap) Swap(left int, right int) {
	entries[left], entries[right] = entries[right], entries[left]
}
func (entries *workspaceEntryMaxHeap) Push(value any) {
	*entries = append(*entries, value.(WorkspaceEntry))
}
func (entries *workspaceEntryMaxHeap) Pop() any {
	old := *entries
	last := old[len(old)-1]
	*entries = old[:len(old)-1]
	return last
}

func normalizeWorkspacePath(relativePath string) (string, error) {
	if strings.Contains(relativePath, "\\") {
		return "", errors.New("工作区相对路径无效")
	}
	if relativePath == "" {
		relativePath = "."
	}
	if relativePath != "." && !fs.ValidPath(relativePath) {
		return "", errors.New("工作区相对路径无效")
	}
	return relativePath, nil
}

func rejectWorkspaceSymlinks(root *os.Root, relativePath string) error {
	if relativePath == "." {
		return nil
	}
	current := ""
	for _, component := range strings.Split(relativePath, "/") {
		current = path.Join(current, component)
		info, err := root.Lstat(current)
		if err != nil {
			return fmt.Errorf("读取工作区路径失败: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("工作区不允许访问符号链接")
		}
	}
	return nil
}

func isMarkdownFilename(name string) bool {
	extension := strings.ToLower(filepath.Ext(name))
	return extension == ".md" || extension == ".markdown"
}
