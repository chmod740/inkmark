package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxWorkspaceMutationPathLength = 4096

// CreateWorkspaceMarkdownFile creates a new, empty Markdown document beneath
// an active local-workspace capability. O_EXCL is intentional: a stale sidebar
// must never truncate a file which appeared since the directory was listed.
func (a *App) CreateWorkspaceMarkdownFile(workspaceID string, relativePath string) (Document, error) {
	a.mu.Lock()
	capability, err := a.activeWorkspaceLocked(workspaceID)
	if err != nil {
		a.mu.Unlock()
		return Document{}, err
	}
	document, err := createWorkspaceMarkdownFileLocked(capability, relativePath)
	a.mu.Unlock()
	if err == nil {
		a.recordRecentItem("file", document.Path)
	}
	return document, err
}

func createWorkspaceMarkdownFileLocked(capability *workspaceCapability, relativePath string) (Document, error) {
	if err := validateWorkspaceCapability(capability); err != nil {
		return Document{}, err
	}
	normalized, err := normalizeWorkspaceDestinationPath(relativePath)
	if err != nil || !isMarkdownFilename(normalized) {
		return Document{}, errors.New("Markdown 文件路径无效")
	}
	if err := validateWorkspaceMutationParent(capability.root, normalized); err != nil {
		return Document{}, err
	}
	file, err := capability.root.OpenFile(normalized, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Document{}, fmt.Errorf("创建 Markdown 文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = capability.root.Remove(normalized)
		return Document{}, fmt.Errorf("关闭新建 Markdown 文件失败: %w", err)
	}
	if capability.testAfterCreate != nil {
		capability.testAfterCreate(normalized)
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return Document{}, fmt.Errorf("Markdown 文件已创建于原工作区，但工作区目录随后失效: %w", err)
	}
	return openWorkspaceMarkdownPath(capability, normalized)
}

func (a *App) CreateWorkspaceDirectory(workspaceID string, relativePath string) (WorkspaceEntry, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	capability, err := a.activeWorkspaceLocked(workspaceID)
	if err != nil {
		return WorkspaceEntry{}, err
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return WorkspaceEntry{}, err
	}
	normalized, err := normalizeWorkspaceDestinationPath(relativePath)
	if err != nil {
		return WorkspaceEntry{}, err
	}
	if err := validateWorkspaceMutationParent(capability.root, normalized); err != nil {
		return WorkspaceEntry{}, err
	}
	if err := capability.root.Mkdir(normalized, 0o755); err != nil {
		return WorkspaceEntry{}, fmt.Errorf("创建文件夹失败: %w", err)
	}
	if capability.testAfterCreate != nil {
		capability.testAfterCreate(normalized)
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return WorkspaceEntry{}, fmt.Errorf("文件夹已创建于原工作区，但工作区目录随后失效: %w", err)
	}
	return workspaceEntryAtPath(capability, normalized)
}

// SaveWorkspaceMarkdownFile keeps subsequent saves for sidebar-opened local
// documents inside the same opaque os.Root and document capabilities. The
// caller cannot choose the save path: localDocumentID resolves to the exact
// filesystem object which was read, and that baseline is checked before the
// file is opened without truncation. A replacement which appeared at the same
// visible path after OpenWorkspaceFile is therefore never overwritten.
func (a *App) SaveWorkspaceMarkdownFile(workspaceID string, localDocumentID string, content string) (SaveResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	capability, err := a.activeWorkspaceLocked(workspaceID)
	if err != nil {
		return SaveResult{}, err
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return SaveResult{}, err
	}
	localDocumentID = strings.TrimSpace(localDocumentID)
	document, ok := capability.documents[localDocumentID]
	if !ok || localDocumentID == "" || document.info == nil {
		return SaveResult{}, errors.New("本地文档会话已关闭或失效，请重新打开文件")
	}
	normalized, err := normalizeWorkspaceExistingPath(document.path)
	if err != nil || !isMarkdownFilename(normalized) || normalized != document.path {
		delete(capability.documents, localDocumentID)
		return SaveResult{}, errors.New("本地文档会话路径无效，请重新打开文件")
	}
	if len(content) > maxDocumentSize || !utf8.ValidString(content) {
		return SaveResult{}, errors.New("Markdown 文档过大或不是有效的 UTF-8 文本")
	}
	if err := rejectWorkspaceSymlinks(capability.root, normalized); err != nil {
		return SaveResult{}, err
	}
	targetInfo, err := capability.root.Lstat(normalized)
	if err != nil {
		return SaveResult{}, fmt.Errorf("读取待保存 Markdown 文件失败: %w", err)
	}
	if !workspaceDocumentInfoMatches(document.info, targetInfo) {
		delete(capability.documents, localDocumentID)
		return SaveResult{}, errors.New("Markdown 文件已在打开后被替换或修改，请重新打开后再保存")
	}
	file, err := openRootWriteOnlyNonBlocking(capability.root, normalized)
	if err != nil {
		return SaveResult{}, fmt.Errorf("打开待保存 Markdown 文件失败: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil || !workspaceDocumentInfoMatches(document.info, openedInfo) || !os.SameFile(targetInfo, openedInfo) {
		_ = file.Close()
		delete(capability.documents, localDocumentID)
		return SaveResult{}, errors.New("Markdown 文件在打开保存句柄前发生了变化")
	}
	if capability.testAfterSaveHandleVerified != nil {
		capability.testAfterSaveHandleVerified()
	}
	if err := file.Truncate(0); err != nil {
		_ = file.Close()
		return SaveResult{}, fmt.Errorf("清空待保存 Markdown 文件失败: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return SaveResult{}, fmt.Errorf("定位待保存 Markdown 文件失败: %w", err)
	}
	if _, err := io.WriteString(file, content); err != nil {
		_ = file.Close()
		return SaveResult{}, fmt.Errorf("写入 Markdown 文件失败: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return SaveResult{}, fmt.Errorf("同步 Markdown 文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return SaveResult{}, fmt.Errorf("关闭 Markdown 文件失败: %w", err)
	}
	currentInfo, pathErr := capability.root.Lstat(normalized)
	if pathErr != nil || currentInfo.Mode()&os.ModeSymlink != 0 || !currentInfo.Mode().IsRegular() || !os.SameFile(openedInfo, currentInfo) {
		delete(capability.documents, localDocumentID)
		return SaveResult{}, errors.New("内容已写入已验证文件句柄，但工作区路径在保存期间发生了变化")
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		delete(capability.documents, localDocumentID)
		return SaveResult{}, fmt.Errorf("文档已保存，但工作区目录随后失效: %w", err)
	}
	document.info = currentInfo
	capability.documents[localDocumentID] = document
	absolute := filepath.Join(capability.path, filepath.FromSlash(normalized))
	return SaveResult{Path: absolute, Name: path.Base(normalized)}, nil
}

// CloseWorkspaceDocument releases an opaque local document capability. It is
// idempotent from the frontend's perspective only for a still-open workspace;
// unknown IDs are rejected so stale WebView state cannot be mistaken for a
// valid save authority.
func (a *App) CloseWorkspaceDocument(workspaceID string, localDocumentID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	capability, err := a.activeWorkspaceLocked(workspaceID)
	if err != nil {
		return err
	}
	localDocumentID = strings.TrimSpace(localDocumentID)
	if _, ok := capability.documents[localDocumentID]; !ok || localDocumentID == "" {
		return errors.New("本地文档会话已关闭或失效")
	}
	delete(capability.documents, localDocumentID)
	return nil
}

func workspaceDocumentInfoMatches(baseline fs.FileInfo, current fs.FileInfo) bool {
	return baseline != nil &&
		current != nil &&
		current.Mode()&os.ModeSymlink == 0 &&
		current.Mode().IsRegular() &&
		os.SameFile(baseline, current) &&
		baseline.Size() == current.Size() &&
		baseline.ModTime().Equal(current.ModTime())
}

// RenameWorkspaceEntry is deliberately a rename, not a move: both paths must
// have the same parent. The platform helper performs an atomic no-replace
// operation so a concurrent destination cannot be overwritten.
func (a *App) RenameWorkspaceEntry(workspaceID string, sourcePath string, destinationPath string, expectedKind string, expectedRevision string) (WorkspaceEntry, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	capability, err := a.activeWorkspaceLocked(workspaceID)
	if err != nil {
		return WorkspaceEntry{}, err
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return WorkspaceEntry{}, err
	}
	source, err := normalizeWorkspaceExistingPath(sourcePath)
	if err != nil {
		return WorkspaceEntry{}, err
	}
	destination, err := normalizeWorkspaceDestinationPath(destinationPath)
	if err != nil {
		return WorkspaceEntry{}, err
	}
	if source == destination {
		return WorkspaceEntry{}, errors.New("源路径和目标路径相同")
	}
	if path.Dir(source) != path.Dir(destination) {
		return WorkspaceEntry{}, errors.New("重命名不能移动到其他文件夹")
	}
	if err := rejectWorkspaceSymlinks(capability.root, source); err != nil {
		return WorkspaceEntry{}, err
	}
	if err := validateWorkspaceMutationParent(capability.root, destination); err != nil {
		return WorkspaceEntry{}, err
	}
	sourceInfo, err := capability.root.Lstat(source)
	if err != nil || sourceInfo.Mode()&os.ModeSymlink != 0 {
		return WorkspaceEntry{}, errors.New("源条目在重命名前发生了变化")
	}
	sourceEntry, err := workspaceEntryFromInfo(capability, source, sourceInfo)
	if err != nil {
		return WorkspaceEntry{}, err
	}
	if sourceEntry.Kind != expectedKind || !validWorkspaceEntryKind(expectedKind) {
		return WorkspaceEntry{}, errors.New("工作区条目类型已发生变化，请刷新后重试")
	}
	if err := validateWorkspaceEntrySnapshot(capability, source, expectedKind, expectedRevision, sourceInfo); err != nil {
		return WorkspaceEntry{}, err
	}
	if err := validateWorkspaceRenameKind(sourceEntry, destination); err != nil {
		return WorkspaceEntry{}, err
	}
	caseOnlyRename := false
	if destinationInfo, err := capability.root.Lstat(destination); err == nil {
		if !os.SameFile(sourceInfo, destinationInfo) {
			return WorkspaceEntry{}, errors.New("目标名称已存在")
		}
		caseOnlyRename = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return WorkspaceEntry{}, fmt.Errorf("检查目标名称失败: %w", err)
	}
	parentPath := path.Dir(source)
	parentRoot, err := capability.root.OpenRoot(parentPath)
	if err != nil {
		return WorkspaceEntry{}, fmt.Errorf("打开重命名所在文件夹失败: %w", err)
	}
	defer parentRoot.Close()
	renameErr := renameWorkspaceEntryVerifiedNoReplace(parentRoot, path.Base(source), path.Base(destination), sourceInfo, caseOnlyRename)
	if renameErr != nil {
		return WorkspaceEntry{}, fmt.Errorf("重命名失败: %w", renameErr)
	}
	capability.rebaseWorkspaceDocuments(source, destination)
	if capability.testAfterRename != nil {
		capability.testAfterRename(source, destination)
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		rollbackErr := renameWorkspaceEntryVerifiedNoReplace(parentRoot, path.Base(destination), path.Base(source), sourceInfo, false)
		if rollbackErr == nil {
			capability.rebaseWorkspaceDocuments(destination, source)
			return WorkspaceEntry{}, fmt.Errorf("工作区目录在重命名后失效，已恢复原名称: %w", err)
		}
		return WorkspaceEntry{}, fmt.Errorf("重命名已完成，但工作区目录随后失效且无法恢复原名称: %v; 回滚失败: %w", err, rollbackErr)
	}
	entry, err := workspaceEntryAtPath(capability, destination)
	if err != nil {
		rollbackErr := renameWorkspaceEntryVerifiedNoReplace(parentRoot, path.Base(destination), path.Base(source), sourceInfo, false)
		if rollbackErr == nil {
			capability.rebaseWorkspaceDocuments(destination, source)
			return WorkspaceEntry{}, fmt.Errorf("重命名后无法读取目标条目，已恢复原名称: %w", err)
		}
		return WorkspaceEntry{}, fmt.Errorf("重命名已完成，但无法读取目标条目且无法恢复原名称: %v; 回滚失败: %w", err, rollbackErr)
	}
	return entry, nil
}

func renameWorkspaceEntryVerifiedNoReplace(parentRoot *os.Root, sourceName string, destinationName string, expectedInfo fs.FileInfo, caseOnly bool) error {
	if !caseOnly {
		if err := renameWorkspaceEntryNoReplace(parentRoot, sourceName, destinationName); err != nil {
			return err
		}
		movedInfo, err := parentRoot.Lstat(destinationName)
		if err == nil && os.SameFile(expectedInfo, movedInfo) {
			return nil
		}
		if rollbackErr := renameWorkspaceEntryNoReplace(parentRoot, destinationName, sourceName); rollbackErr != nil {
			return errors.New("重命名对象身份发生变化，且无法恢复原名称")
		}
		return errors.New("重命名对象身份发生变化，已恢复原名称")
	}

	temporaryID, err := newOpaqueID()
	if err != nil {
		return errors.New("创建大小写重命名临时标识失败")
	}
	temporaryName := ".inkmark-rename-" + temporaryID
	if err := renameWorkspaceEntryNoReplace(parentRoot, sourceName, temporaryName); err != nil {
		return err
	}
	temporaryInfo, err := parentRoot.Lstat(temporaryName)
	if err != nil || !os.SameFile(expectedInfo, temporaryInfo) {
		if rollbackErr := renameWorkspaceEntryNoReplace(parentRoot, temporaryName, sourceName); rollbackErr != nil {
			return errors.New("大小写重命名对象身份发生变化，且无法恢复原名称")
		}
		return errors.New("大小写重命名对象身份发生变化，已恢复原名称")
	}
	if err := renameWorkspaceEntryNoReplace(parentRoot, temporaryName, destinationName); err != nil {
		if rollbackErr := renameWorkspaceEntryNoReplace(parentRoot, temporaryName, sourceName); rollbackErr != nil {
			return errors.New("大小写重命名失败，且无法恢复原名称")
		}
		return err
	}
	return nil
}

func (a *App) DeleteWorkspaceEntry(workspaceID string, relativePath string, recursive bool, expectedRevision string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	capability, err := a.activeWorkspaceLocked(workspaceID)
	if err != nil {
		return err
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return err
	}
	normalized, err := normalizeWorkspaceExistingPath(relativePath)
	if err != nil {
		return err
	}
	if err := rejectWorkspaceSymlinks(capability.root, normalized); err != nil {
		return err
	}
	expectedInfo, err := capability.root.Lstat(normalized)
	if err != nil || expectedInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("待删除条目在确认后发生了变化")
	}
	entry, err := workspaceEntryFromInfo(capability, normalized, expectedInfo)
	if err != nil {
		return err
	}
	if err := validateWorkspaceEntrySnapshot(capability, normalized, entry.Kind, expectedRevision, expectedInfo); err != nil {
		return err
	}
	switch entry.Kind {
	case "directory":
		if !recursive {
			return errors.New("删除文件夹必须明确启用递归删除")
		}
	case "markdown", "image":
		if recursive {
			return errors.New("删除文件不能启用递归删除")
		}
	default:
		return errors.New("不支持删除该工作区条目")
	}
	if (entry.Kind == "directory") != expectedInfo.IsDir() || expectedInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("待删除条目类型已发生变化，请刷新后重试")
	}
	var deletionFilesystemID uint64
	if entry.Kind == "directory" {
		preflightParent, err := capability.root.OpenRoot(path.Dir(normalized))
		if err != nil {
			return fmt.Errorf("打开待删除条目的父文件夹失败: %w", err)
		}
		deletionFilesystemID, err = preflightWorkspaceDirectory(capability, preflightParent, path.Base(normalized), expectedInfo)
		_ = preflightParent.Close()
		if err != nil {
			return fmt.Errorf("删除前安全检查失败: %w", err)
		}
	}
	if capability.testBeforeDeleteQuarantine != nil {
		capability.testBeforeDeleteQuarantine(normalized)
	}
	parentRoot, quarantineName, quarantinedInfo, err := quarantineWorkspaceEntry(capability, normalized, expectedInfo)
	if err != nil {
		return err
	}
	defer parentRoot.Close()
	if capability.testAfterDeleteQuarantine != nil {
		capability.testAfterDeleteQuarantine(normalized)
	}
	if entry.Kind == "directory" {
		verifiedFilesystemID, verifyErr := preflightWorkspaceDirectory(capability, parentRoot, quarantineName, quarantinedInfo)
		if verifyErr != nil || verifiedFilesystemID != deletionFilesystemID {
			rollbackErr := renameWorkspaceEntryNoReplace(parentRoot, quarantineName, path.Base(normalized))
			if rollbackErr != nil {
				capability.revokeWorkspaceDocumentsAtPath(normalized)
				return errors.New("隔离后的删除安全检查失败，且原名称已被占用；未删除隔离目录内容")
			}
			if verifyErr != nil {
				return fmt.Errorf("隔离后的删除安全检查失败，已恢复原名称: %w", verifyErr)
			}
			return errors.New("隔离后的文件系统身份发生变化，已恢复原名称")
		}
		started, err := removeWorkspaceQuarantinedDirectory(parentRoot, quarantineName, quarantinedInfo, deletionFilesystemID)
		if err != nil {
			if !started {
				if rollbackErr := renameWorkspaceEntryNoReplace(parentRoot, quarantineName, path.Base(normalized)); rollbackErr != nil {
					capability.revokeWorkspaceDocumentsAtPath(normalized)
					return errors.New("删除尚未开始但无法恢复隔离目录的原名称；未删除隔离目录内容")
				}
			} else {
				capability.revokeWorkspaceDocumentsAtPath(normalized)
			}
			return fmt.Errorf("删除文件夹失败: %w", err)
		}
		capability.revokeWorkspaceDocumentsAtPath(normalized)
	} else {
		if err := parentRoot.Remove(quarantineName); err != nil {
			capability.revokeWorkspaceDocumentsAtPath(normalized)
			return fmt.Errorf("删除隔离文件失败: %w", err)
		}
		capability.revokeWorkspaceDocumentsAtPath(normalized)
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return fmt.Errorf("条目已删除，但工作区目录随后失效: %w", err)
	}
	return nil
}

// quarantineWorkspaceEntry atomically moves the confirmed source to an
// unpredictable same-parent name before deletion. If another process reuses
// the original visible name while deletion is running, that replacement is
// never addressed by any subsequent remove call.
func quarantineWorkspaceEntry(capability *workspaceCapability, relativePath string, expectedInfo fs.FileInfo) (*os.Root, string, fs.FileInfo, error) {
	parentPath := path.Dir(relativePath)
	parentRoot, err := capability.root.OpenRoot(parentPath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("打开待删除条目的父文件夹失败: %w", err)
	}
	quarantineID, err := newOpaqueID()
	if err != nil {
		_ = parentRoot.Close()
		return nil, "", nil, errors.New("创建删除隔离标识失败")
	}
	quarantineName := ".inkmark-delete-" + quarantineID
	sourceName := path.Base(relativePath)
	if err := renameWorkspaceEntryNoReplace(parentRoot, sourceName, quarantineName); err != nil {
		_ = parentRoot.Close()
		return nil, "", nil, fmt.Errorf("隔离待删除条目失败: %w", err)
	}
	quarantinedInfo, err := parentRoot.Lstat(quarantineName)
	if err == nil && os.SameFile(expectedInfo, quarantinedInfo) {
		return parentRoot, quarantineName, quarantinedInfo, nil
	}
	rollbackErr := renameWorkspaceEntryNoReplace(parentRoot, quarantineName, sourceName)
	_ = parentRoot.Close()
	if rollbackErr != nil {
		capability.revokeWorkspaceDocumentsAtPath(relativePath)
		return nil, "", nil, errors.New("待删除条目在隔离期间发生变化，且无法恢复原名称；未删除任何隔离内容")
	}
	return nil, "", nil, errors.New("待删除条目在隔离期间发生变化；已恢复原名称")
}

// preflightWorkspaceDirectory is a complete no-write traversal. It rejects
// nested symlinks/reparse points, special files, and mount boundaries before
// the visible directory is quarantined, and is repeated after quarantine.
func preflightWorkspaceDirectory(capability *workspaceCapability, parentRoot *os.Root, name string, targetInfo fs.FileInfo) (uint64, error) {
	filesystemID, err := workspaceDeletionRootFilesystemIdentity(capability.root)
	if err != nil {
		return 0, err
	}
	if !workspaceDeletionEntrySafe(targetInfo) || !targetInfo.IsDir() {
		return 0, errors.New("待删除路径不是安全的普通文件夹")
	}
	if matches, matchErr := workspaceDeletionEntryFilesystemMatches(parentRoot, name, targetInfo, filesystemID); matchErr != nil || !matches {
		return 0, errors.New("不能递归删除跨文件系统的文件夹")
	}
	targetRoot, err := parentRoot.OpenRoot(name)
	if err != nil {
		return 0, fmt.Errorf("打开待删除文件夹失败: %w", err)
	}
	defer targetRoot.Close()
	openedInfo, err := targetRoot.Stat(".")
	if err != nil || !os.SameFile(targetInfo, openedInfo) || !workspaceDeletionEntrySafe(openedInfo) {
		return 0, errors.New("待删除文件夹在检查期间发生了变化")
	}
	if matches, matchErr := workspaceDeletionRootFilesystemMatches(targetRoot, filesystemID); matchErr != nil || !matches {
		return 0, errors.New("不能递归删除跨文件系统的文件夹")
	}
	if err := preflightWorkspaceDeletionTree(targetRoot, filesystemID); err != nil {
		return 0, err
	}
	return filesystemID, nil
}

// removeWorkspaceQuarantinedDirectory repeats identity checks, then deletes
// bottom-up only beneath the random quarantine name. The returned boolean says
// whether content deletion began, allowing a pre-delete failure to roll back
// the visible name without hiding a complete directory.
func removeWorkspaceQuarantinedDirectory(parentRoot *os.Root, quarantineName string, targetInfo fs.FileInfo, filesystemID uint64) (bool, error) {
	targetRoot, err := parentRoot.OpenRoot(quarantineName)
	if err != nil {
		return false, fmt.Errorf("打开隔离文件夹失败: %w", err)
	}
	openedInfo, err := targetRoot.Stat(".")
	if err != nil || !os.SameFile(targetInfo, openedInfo) || !workspaceDeletionEntrySafe(openedInfo) {
		_ = targetRoot.Close()
		return false, errors.New("隔离文件夹在删除前发生了变化")
	}
	if matches, matchErr := workspaceDeletionRootFilesystemMatches(targetRoot, filesystemID); matchErr != nil || !matches {
		_ = targetRoot.Close()
		return false, errors.New("隔离文件夹跨越了文件系统边界")
	}
	if err := deleteWorkspaceTreeContents(targetRoot, filesystemID); err != nil {
		_ = targetRoot.Close()
		return true, err
	}
	_ = targetRoot.Close()
	if err := parentRoot.Remove(quarantineName); err != nil {
		return true, fmt.Errorf("删除空隔离文件夹失败: %w", err)
	}
	return true, nil
}

func preflightWorkspaceDeletionTree(root *os.Root, filesystemID uint64) error {
	return walkWorkspaceDeletionEntries(root, func(entry fs.DirEntry, info fs.FileInfo) error {
		if !workspaceDeletionEntrySafe(info) {
			return fmt.Errorf("递归删除拒绝不安全条目 %q", entry.Name())
		}
		if matches, err := workspaceDeletionEntryFilesystemMatches(root, entry.Name(), info, filesystemID); err != nil || !matches {
			return fmt.Errorf("递归删除拒绝跨文件系统条目 %q", entry.Name())
		}
		if !info.IsDir() {
			return nil
		}
		childRoot, err := root.OpenRoot(entry.Name())
		if err != nil {
			return fmt.Errorf("打开子文件夹 %q 失败: %w", entry.Name(), err)
		}
		defer childRoot.Close()
		openedInfo, err := childRoot.Stat(".")
		if err != nil || !os.SameFile(info, openedInfo) || !workspaceDeletionEntrySafe(openedInfo) {
			return fmt.Errorf("子文件夹 %q 在检查期间发生了变化", entry.Name())
		}
		if matches, err := workspaceDeletionRootFilesystemMatches(childRoot, filesystemID); err != nil || !matches {
			return fmt.Errorf("递归删除拒绝跨文件系统子文件夹 %q", entry.Name())
		}
		return preflightWorkspaceDeletionTree(childRoot, filesystemID)
	})
}

func deleteWorkspaceTreeContents(root *os.Root, filesystemID uint64) error {
	for {
		directory, err := root.Open(".")
		if err != nil {
			return fmt.Errorf("读取待删除文件夹失败: %w", err)
		}
		entries, readErr := directory.ReadDir(256)
		_ = directory.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("读取待删除文件夹失败: %w", readErr)
		}
		if len(entries) == 0 {
			return nil
		}
		for _, entry := range entries {
			info, err := root.Lstat(entry.Name())
			if err != nil {
				return fmt.Errorf("读取待删除条目 %q 失败: %w", entry.Name(), err)
			}
			matches, matchErr := workspaceDeletionEntryFilesystemMatches(root, entry.Name(), info, filesystemID)
			if !workspaceDeletionEntrySafe(info) || matchErr != nil || !matches {
				return fmt.Errorf("条目 %q 在删除期间变得不安全", entry.Name())
			}
			if info.IsDir() {
				childRoot, err := root.OpenRoot(entry.Name())
				if err != nil {
					return fmt.Errorf("打开子文件夹 %q 失败: %w", entry.Name(), err)
				}
				openedInfo, err := childRoot.Stat(".")
				rootMatches, rootMatchErr := workspaceDeletionRootFilesystemMatches(childRoot, filesystemID)
				if err != nil || !os.SameFile(info, openedInfo) || !workspaceDeletionEntrySafe(openedInfo) || rootMatchErr != nil || !rootMatches {
					_ = childRoot.Close()
					return fmt.Errorf("子文件夹 %q 在删除期间发生了变化", entry.Name())
				}
				if err := deleteWorkspaceTreeContents(childRoot, filesystemID); err != nil {
					_ = childRoot.Close()
					return err
				}
				_ = childRoot.Close()
			}
			if err := root.Remove(entry.Name()); err != nil {
				return fmt.Errorf("删除条目 %q 失败: %w", entry.Name(), err)
			}
		}
	}
}

func walkWorkspaceDeletionEntries(root *os.Root, visit func(fs.DirEntry, fs.FileInfo) error) error {
	directory, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("读取待删除文件夹失败: %w", err)
	}
	defer directory.Close()
	for {
		entries, readErr := directory.ReadDir(256)
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				return fmt.Errorf("读取待删除条目 %q 失败: %w", entry.Name(), err)
			}
			if err := visit(entry, info); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("读取待删除文件夹失败: %w", readErr)
		}
		if len(entries) == 0 {
			return nil
		}
	}
}

func workspaceDeletionEntrySafe(info fs.FileInfo) bool {
	if info == nil || info.Mode()&os.ModeSymlink != 0 || !workspaceDeletionPlatformSafe(info) {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

func workspaceDeletionRootFilesystemMatches(root *os.Root, filesystemID uint64) (bool, error) {
	candidate, err := workspaceDeletionRootFilesystemIdentity(root)
	return err == nil && candidate == filesystemID, err
}

// ReadWorkspaceImage returns only a bounded, decoded-and-validated raster
// payload. It never exposes a generic file reader or trusts an absolute path
// supplied by the WebView.
func (a *App) ReadWorkspaceImage(workspaceID string, relativePath string) (ImageAssetData, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	capability, err := a.activeWorkspaceLocked(workspaceID)
	if err != nil {
		return ImageAssetData{}, err
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return ImageAssetData{}, err
	}
	normalized, err := normalizeWorkspaceExistingPath(relativePath)
	if err != nil || !isImageFilename(normalized) {
		return ImageAssetData{}, errors.New("图片路径无效")
	}
	if err := rejectWorkspaceSymlinks(capability.root, normalized); err != nil {
		return ImageAssetData{}, err
	}
	info, err := capability.root.Lstat(normalized)
	if err != nil {
		return ImageAssetData{}, fmt.Errorf("读取图片信息失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return ImageAssetData{}, errors.New("所选图片不是普通文件")
	}
	file, err := openRootReadOnlyNonBlocking(capability.root, normalized)
	if err != nil {
		return ImageAssetData{}, fmt.Errorf("打开工作区图片失败: %w", err)
	}
	defer file.Close()
	imageData, err := readValidatedImage(file, path.Base(normalized), "")
	if err != nil {
		return ImageAssetData{}, err
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return ImageAssetData{}, err
	}
	return imageData.bridgeData(), nil
}

func openWorkspaceMarkdownPath(capability *workspaceCapability, relativePath string) (Document, error) {
	if len(capability.documents) >= maxWorkspaceDocuments {
		return Document{}, errors.New("打开的本地工作区文档过多，请先关闭一些文档")
	}
	if err := rejectWorkspaceSymlinks(capability.root, relativePath); err != nil {
		return Document{}, err
	}
	targetInfo, err := capability.root.Lstat(relativePath)
	if err != nil || targetInfo.Mode()&os.ModeSymlink != 0 || !targetInfo.Mode().IsRegular() {
		return Document{}, errors.New("所选 Markdown 文件已被替换或不是普通文件")
	}
	file, err := openRootReadOnlyNonBlocking(capability.root, relativePath)
	if err != nil {
		return Document{}, fmt.Errorf("打开文件失败: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil || !workspaceDocumentInfoMatches(targetInfo, openedInfo) {
		_ = file.Close()
		return Document{}, errors.New("Markdown 文件在打开读取句柄前发生了变化")
	}
	absolute := filepath.Join(capability.path, filepath.FromSlash(relativePath))
	document, err := readDocumentFromFile(file, absolute)
	_ = file.Close()
	if err != nil {
		return Document{}, err
	}
	currentInfo, err := capability.root.Lstat(relativePath)
	if err != nil || !workspaceDocumentInfoMatches(openedInfo, currentInfo) {
		return Document{}, errors.New("Markdown 文件在读取期间发生了变化，请重新打开")
	}
	if err := validateWorkspaceCapability(capability); err != nil {
		return Document{}, err
	}
	localDocumentID, err := newOpaqueID()
	if err != nil {
		return Document{}, errors.New("创建本地文档会话失败")
	}
	capability.documents[localDocumentID] = workspaceDocumentCapability{path: relativePath, info: currentInfo}
	document.WorkspaceID = capability.id
	document.WorkspacePath = relativePath
	document.LocalDocumentID = localDocumentID
	return document, nil
}

func (capability *workspaceCapability) rebaseWorkspaceDocuments(source string, destination string) {
	for documentID, document := range capability.documents {
		if rebased, ok := rebaseWorkspaceDescendantPath(document.path, source, destination); ok {
			document.path = rebased
			capability.documents[documentID] = document
		}
	}
}

func (capability *workspaceCapability) revokeWorkspaceDocumentsAtPath(target string) {
	for documentID, document := range capability.documents {
		if workspacePathAtOrBelow(document.path, target) {
			delete(capability.documents, documentID)
		}
	}
}

func rebaseWorkspaceDescendantPath(candidate string, source string, destination string) (string, bool) {
	if candidate == source {
		return destination, true
	}
	if strings.HasPrefix(candidate, source+"/") {
		return destination + strings.TrimPrefix(candidate, source), true
	}
	return candidate, false
}

func workspacePathAtOrBelow(candidate string, target string) bool {
	return candidate == target || strings.HasPrefix(candidate, target+"/")
}

func workspaceEntryAtPath(capability *workspaceCapability, relativePath string) (WorkspaceEntry, error) {
	info, err := capability.root.Lstat(relativePath)
	if err != nil {
		return WorkspaceEntry{}, fmt.Errorf("读取工作区条目信息失败: %w", err)
	}
	return workspaceEntryFromInfo(capability, relativePath, info)
}

func workspaceEntryFromInfo(capability *workspaceCapability, relativePath string, info fs.FileInfo) (WorkspaceEntry, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		return WorkspaceEntry{}, errors.New("工作区不允许访问符号链接")
	}
	kind := ""
	switch {
	case info.IsDir():
		kind = "directory"
	case info.Mode().IsRegular() && isMarkdownFilename(relativePath):
		kind = "markdown"
	case info.Mode().IsRegular() && isImageFilename(relativePath):
		kind = "image"
	default:
		return WorkspaceEntry{}, errors.New("工作区条目类型不受支持")
	}
	return WorkspaceEntry{
		Name:         path.Base(relativePath),
		Path:         relativePath,
		AbsolutePath: filepath.Join(capability.path, filepath.FromSlash(relativePath)),
		Kind:         kind,
		Revision:     capability.rememberWorkspaceEntry(relativePath, kind, info),
	}, nil
}

func validateWorkspaceEntrySnapshot(capability *workspaceCapability, relativePath string, kind string, revision string, currentInfo fs.FileInfo) error {
	snapshot, ok := capability.workspaceEntrySnapshot(revision)
	if !ok || snapshot.path != relativePath || snapshot.kind != kind || snapshot.info == nil || currentInfo == nil || !os.SameFile(snapshot.info, currentInfo) {
		return errors.New("工作区条目已在显示后发生变化，请刷新后重试")
	}
	return nil
}

func validateWorkspaceRenameKind(source WorkspaceEntry, destination string) error {
	switch source.Kind {
	case "directory":
		return nil
	case "markdown":
		if isMarkdownFilename(destination) {
			return nil
		}
		return errors.New("Markdown 文件重命名后必须保留 .md 或 .markdown 扩展名")
	case "image":
		if sameImageExtensionFamily(source.Path, destination) {
			return nil
		}
		return errors.New("图片重命名后必须保留原图片格式（.jpg 与 .jpeg 可互换）")
	default:
		return errors.New("不支持重命名该工作区条目")
	}
}

func validateWorkspaceMutationParent(root *os.Root, relativePath string) error {
	parent := path.Dir(relativePath)
	if err := rejectWorkspaceSymlinks(root, parent); err != nil {
		return err
	}
	info, err := root.Lstat(parent)
	if err != nil {
		return fmt.Errorf("读取父文件夹失败: %w", err)
	}
	if !info.IsDir() {
		return errors.New("目标父路径不是文件夹")
	}
	return nil
}

func normalizeWorkspaceExistingPath(relativePath string) (string, error) {
	if len(relativePath) == 0 || len(relativePath) > maxWorkspaceMutationPathLength || !utf8.ValidString(relativePath) {
		return "", errors.New("工作区相对路径无效")
	}
	normalized, err := normalizeWorkspacePath(relativePath)
	if err != nil || normalized == "." {
		return "", errors.New("不能修改工作区根目录")
	}
	for _, character := range normalized {
		if character == 0 || character < 0x20 || character == 0x7f {
			return "", errors.New("工作区路径包含控制字符")
		}
	}
	return normalized, nil
}

func normalizeWorkspaceDestinationPath(relativePath string) (string, error) {
	normalized, err := normalizeWorkspaceExistingPath(relativePath)
	if err != nil {
		return "", err
	}
	if err := validatePortableWorkspaceName(path.Base(normalized)); err != nil {
		return "", err
	}
	return normalized, nil
}

func validWorkspaceEntryKind(kind string) bool {
	return kind == "directory" || kind == "markdown" || kind == "image"
}

func validatePortableWorkspaceName(name string) error {
	if name == "" || len(name) > 255 || !utf8.ValidString(name) {
		return errors.New("文件名为空、过长或编码无效")
	}
	if strings.HasSuffix(name, " ") || strings.HasSuffix(name, ".") {
		return errors.New("文件名不能以空格或句点结尾")
	}
	if strings.ContainsAny(name, `<>:"|?*`) {
		return errors.New("文件名包含 Windows 不支持的字符")
	}
	for _, character := range name {
		if character == 0 || character < 0x20 || character == 0x7f {
			return errors.New("文件名包含控制字符")
		}
	}
	base := strings.ToUpper(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " ."))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
		"COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³":
		return errors.New("文件名是 Windows 保留设备名")
	}
	return nil
}
