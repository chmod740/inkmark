Unicode true

####
## This project file is based on the Wails v2.13 installer template.
## It is kept in the project so file-association commands can be corrected
## after Wails expands its generated association macros.
####

!include "wails_tools.nsh"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING

!insertmacro MUI_PAGE_WELCOME
!define MUI_PAGE_CUSTOMFUNCTION_PRE SkipUpdateDirectoryPage
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
!ifdef WAILS_INSTALL_SCOPE
  !if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
  !else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
  !endif
!else
  InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif
ShowInstDetails show

Var UpdateMode
Var UpdateWaitPID
Var UpdateWaitResult
Var ExistingInstallFound

!ifndef UPDATE_WAIT_TIMEOUT_MS
  !define UPDATE_WAIT_TIMEOUT_MS 120000
!endif

!ifdef WAILS_INSTALL_SCOPE
  !if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDirRegKey HKCU "${UNINST_KEY}" "InstallLocation"
  !else
    InstallDirRegKey HKLM "${UNINST_KEY}" "InstallLocation"
  !endif
!else
  InstallDirRegKey HKLM "${UNINST_KEY}" "InstallLocation"
!endif

Function SkipUpdateDirectoryPage
    # A verified update must stay in the registered installation directory.
    # Skipping this page prevents an update from being redirected while still
    # using the old installation's association and uninstall metadata.
    StrCmp $UpdateMode "1" 0 update_directory_page_done
    StrCmp $ExistingInstallFound "1" 0 update_directory_page_done
    Abort

    update_directory_page_done:
FunctionEnd

Function ResolveExistingInstallDirectory
    SetRegView 64
    StrCpy $0 ""
    StrCpy $ExistingInstallFound "0"

    # Newer installers persist the exact install directory. For older
    # versions, derive it from DisplayIcon without executing registry data.
    !ifdef WAILS_INSTALL_SCOPE
      !if "${WAILS_INSTALL_SCOPE}" == "user"
        ReadRegStr $0 HKCU "${UNINST_KEY}" "InstallLocation"
        StrCmp $0 "" 0 validate_existing_install
        ReadRegStr $1 HKCU "${UNINST_KEY}" "DisplayIcon"
      !else
        ReadRegStr $0 HKLM "${UNINST_KEY}" "InstallLocation"
        StrCmp $0 "" 0 validate_existing_install
        ReadRegStr $1 HKLM "${UNINST_KEY}" "DisplayIcon"
      !endif
    !else
        ReadRegStr $0 HKLM "${UNINST_KEY}" "InstallLocation"
        StrCmp $0 "" 0 validate_existing_install
        ReadRegStr $1 HKLM "${UNINST_KEY}" "DisplayIcon"
    !endif

    StrCmp $1 "" resolve_existing_install_done
    ${GetParent} "$1" $0

    validate_existing_install:
    StrCmp $0 "" resolve_existing_install_done
    IfFileExists "$0\${PRODUCT_EXECUTABLE}" 0 resolve_existing_install_done
    IfFileExists "$0\uninstall.exe" 0 resolve_existing_install_done
    StrCpy $INSTDIR "$0"
    StrCpy $ExistingInstallFound "1"

    resolve_existing_install_done:
FunctionEnd

Function WaitForUpdateProcess
    StrCpy $UpdateWaitResult "1"
    StrCmp $UpdateWaitPID "" update_wait_done
    IntFmt $0 "%u" $UpdateWaitPID
    StrCmp $0 $UpdateWaitPID 0 update_wait_invalid
    IntCmp $UpdateWaitPID 0 update_wait_invalid update_wait_invalid update_wait_open

    update_wait_open:
    # SYNCHRONIZE is sufficient to wait for a process and cannot terminate it.
    System::Call 'kernel32::OpenProcess(i 0x00100000, i 0, i $UpdateWaitPID) p.r0'
    StrCmp $0 0 update_wait_open_failed

    update_wait_again:
    System::Call 'kernel32::WaitForSingleObject(p r0, i ${UPDATE_WAIT_TIMEOUT_MS}) i.r1'
    StrCmp $1 0 update_wait_close
    StrCmp $1 258 update_wait_timeout update_wait_failed

    update_wait_timeout:
    IfSilent update_wait_abort update_wait_prompt

    update_wait_prompt:
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION \
      "${INFO_PRODUCTNAME} is still closing. Retry after it exits, or cancel this update." \
      IDRETRY update_wait_again IDCANCEL update_wait_abort

    update_wait_open_failed:
    # ERROR_INVALID_PARAMETER means the PID has already exited.
    System::Call 'kernel32::GetLastError() i.r1'
    StrCmp $1 87 update_wait_done update_wait_failed_no_handle

    update_wait_failed:
    System::Call 'kernel32::CloseHandle(p r0)'

    update_wait_failed_no_handle:
    IfSilent update_wait_abort update_wait_error_prompt

    update_wait_error_prompt:
    MessageBox MB_OK|MB_ICONSTOP \
      "The running ${INFO_PRODUCTNAME} process could not be verified as closed. The update was cancelled."
    Goto update_wait_abort

    update_wait_close:
    System::Call 'kernel32::CloseHandle(p r0)'
    Goto update_wait_done

    update_wait_invalid:
    IfSilent update_wait_abort update_wait_invalid_prompt

    update_wait_invalid_prompt:
    MessageBox MB_OK|MB_ICONSTOP "The update process identifier is invalid. The update was cancelled."

    update_wait_abort:
    StrCpy $UpdateWaitResult "0"
    SetErrorLevel 1618

    update_wait_done:
FunctionEnd

Function .onInit
   !insertmacro wails.checkArchitecture

   StrCpy $UpdateMode "0"
   StrCpy $UpdateWaitPID ""
   ${GetParameters} $0

   ClearErrors
   ${GetOptions} $0 "/UPDATE=" $1
   IfErrors parse_update_flag
   StrCmp $1 "1" update_mode_enabled update_mode_parsed

   parse_update_flag:
   ClearErrors
   ${GetOptions} $0 "/UPDATE" $1
   IfErrors update_mode_parsed

   update_mode_enabled:
   StrCpy $UpdateMode "1"

   update_mode_parsed:
   ClearErrors
   ${GetOptions} $0 "/WAITPID=" $UpdateWaitPID
   IfErrors update_wait_pid_parsed

   update_wait_pid_parsed:
   Call ResolveExistingInstallDirectory
   StrCmp $UpdateMode "1" 0 update_init_done
   Call WaitForUpdateProcess
   StrCmp $UpdateWaitResult "1" update_init_done
   Abort

   update_init_done:
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    # Re-running APP_ASSOCIATE during an update would replace the original
    # association backups with our own ProgID. Keep extension defaults and
    # backups intact, but refresh the installed ProgID and icon paths.
    StrCmp $ExistingInstallFound "1" update_existing_associations install_new_associations

    install_new_associations:
        !insertmacro wails.associateFiles
        Goto file_associations_done

    update_existing_associations:
        File "..\appicon.ico"
        WriteRegStr SHELL_CONTEXT "Software\Classes\InkMark Markdown Document" "" "InkMark Markdown Document"
        WriteRegStr SHELL_CONTEXT "Software\Classes\InkMark Markdown Document\DefaultIcon" "" "$INSTDIR\appicon.ico"
        WriteRegStr SHELL_CONTEXT "Software\Classes\InkMark Markdown Document\shell" "" "open"
        WriteRegStr SHELL_CONTEXT "Software\Classes\InkMark Markdown Document\shell\open" "" "Open with ${INFO_PRODUCTNAME}"
        WriteRegStr SHELL_CONTEXT "Software\Classes\InkMark Markdown Document\shell\open\command" "" "$\"$INSTDIR\${PRODUCT_EXECUTABLE}$\" $\"%1$\""

    file_associations_done:

    # Wails 2.13 does not quote the executable in its generated association
    # command. That breaks launches when the install directory contains spaces.
    # Both configured extensions share this ProgID, so overwrite it once after
    # the generated macros run.
    WriteRegStr SHELL_CONTEXT "Software\Classes\InkMark Markdown Document\shell\open\command" "" "$\"$INSTDIR\${PRODUCT_EXECUTABLE}$\" $\"%1$\""

    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
    SetRegView 64
    WriteRegStr SHELL_CONTEXT "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
    ${RefreshShellIcons}
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    # Remove only files owned by InkMark. A user-selected installation folder
    # must never be recursively deleted, and settings live outside $INSTDIR.
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
    Delete "$INSTDIR\appicon.ico"

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
    RMDir "$INSTDIR"
SectionEnd
