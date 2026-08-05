//go:build windows

package main

import (
	"os/exec"

	"golang.org/x/sys/windows"
)

const createNewProcessGroup = 0x00000200

func configureInstallerCommand(command *exec.Cmd) {
	command.SysProcAttr = &windows.SysProcAttr{CreationFlags: createNewProcessGroup}
}
