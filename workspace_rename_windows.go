//go:build windows

package main

import (
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

type workspaceFileRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func renameWorkspaceEntryNoReplace(parent *os.Root, sourceName string, destinationName string) error {
	directory, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	parentHandle := windows.Handle(directory.Fd())

	sourceObjectName, err := windows.NewNTUnicodeString(sourceName)
	if err != nil {
		return err
	}
	objectAttributes := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parentHandle,
		ObjectName:    sourceObjectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var sourceHandle windows.Handle
	var openStatus windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(
		&sourceHandle,
		windows.DELETE|windows.SYNCHRONIZE,
		objectAttributes,
		&openStatus,
		nil,
		0,
		windows.FILE_SHARE_DELETE|windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_OPEN,
		windows.FILE_OPEN_REPARSE_POINT|windows.FILE_OPEN_FOR_BACKUP_INTENT|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0,
		0,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(sourceHandle)

	destinationUTF16, err := windows.UTF16FromString(destinationName)
	if err != nil {
		return err
	}
	destinationUTF16 = destinationUTF16[:len(destinationUTF16)-1]
	nameOffset := int(unsafe.Offsetof(workspaceFileRenameInformation{}.FileName))
	buffer := make([]byte, nameOffset+len(destinationUTF16)*2)
	information := (*workspaceFileRenameInformation)(unsafe.Pointer(&buffer[0]))
	information.ReplaceIfExists = 0
	information.RootDirectory = parentHandle
	information.FileNameLength = uint32(len(destinationUTF16) * 2)
	nameBuffer := unsafe.Slice(&information.FileName[0], len(destinationUTF16))
	copy(nameBuffer, destinationUTF16)

	var renameStatus windows.IO_STATUS_BLOCK
	err = windows.NtSetInformationFile(
		sourceHandle,
		&renameStatus,
		&buffer[0],
		uint32(len(buffer)),
		windows.FileRenameInformation,
	)
	runtime.KeepAlive(directory)
	return err
}
