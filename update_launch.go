package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type updateLaunchRequest struct {
	InstallerPath string
	InstallerKind string
	ParentPID     int
}

// LaunchUpdateInstaller re-verifies the cached installer and hands it to the
// platform installer without a shell. The frontend calls this only after the
// unsaved-document guard succeeds, then confirms the native quit request.
// Windows NSIS waits for ParentPID before touching installed files; macOS
// Installer presents its confirmation UI while the old app closes.
func (a *App) LaunchUpdateInstaller() error {
	a.mu.Lock()
	if a.updateDownloading {
		a.mu.Unlock()
		return errors.New("the update is still downloading")
	}
	if a.updateLaunching {
		a.mu.Unlock()
		return errors.New("the update installer is already starting")
	}
	update := a.downloadedUpdate
	cacheOverride := a.updateCacheDir
	launcher := a.updateInstallerLauncher
	if update.Path == "" {
		a.mu.Unlock()
		return errors.New("no verified update installer is ready")
	}
	a.updateLaunching = true
	a.mu.Unlock()

	launchSucceeded := false
	defer func() {
		if !launchSucceeded {
			a.mu.Lock()
			a.updateLaunching = false
			a.mu.Unlock()
		}
	}()

	if !fileMatchesUpdate(update.Path, update.Digest, update.Size) {
		return errors.New("the downloaded update no longer passes checksum verification")
	}
	if installerKindForAsset(update.AssetName, runtime.GOOS) != update.InstallerKind {
		return errors.New("the downloaded update is not a supported installer for this platform")
	}
	cacheRoot, err := updateCacheRoot(cacheOverride)
	if err != nil {
		return err
	}
	if !pathWithinDirectory(update.Path, cacheRoot) {
		return errors.New("the downloaded installer is outside the private update cache")
	}
	request := updateLaunchRequest{
		InstallerPath: update.Path,
		InstallerKind: update.InstallerKind,
		ParentPID:     os.Getpid(),
	}
	if launcher == nil {
		launcher = startPlatformInstaller
	}
	if err := launcher(request); err != nil {
		return err
	}
	launchSucceeded = true
	return nil
}

func startPlatformInstaller(request updateLaunchRequest) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if strings.HasSuffix(strings.ToLower(request.InstallerPath), ".msi") {
			command = exec.Command("msiexec.exe", "/i", request.InstallerPath)
		} else {
			command = exec.Command(
				request.InstallerPath,
				"/UPDATE=1",
				"/WAITPID="+strconv.Itoa(request.ParentPID),
			)
		}
	case "darwin":
		command = exec.Command("/usr/bin/open", request.InstallerPath)
	case "linux":
		command = exec.Command("xdg-open", request.InstallerPath)
	default:
		return errors.New("automatic update installation is unsupported on this platform")
	}
	configureInstallerCommand(command)
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if err := command.Run(); err != nil {
			return fmt.Errorf("open update installer: %w", err)
		}
		return nil
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start update installer: %w", err)
	}
	return command.Process.Release()
}

func pathWithinDirectory(path string, directory string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absDirectory, err := filepath.Abs(directory)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(absDirectory, absPath)
	return err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
