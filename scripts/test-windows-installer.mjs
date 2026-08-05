import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const projectUrl = new URL('../build/windows/installer/project.nsi', import.meta.url)
const configUrl = new URL('../wails.json', import.meta.url)
const versionInfoUrl = new URL('../build/windows/info.json', import.meta.url)
const buildScriptUrl = new URL('./build-windows.ps1', import.meta.url)

test('Windows installer exposes a safe in-place update protocol', async () => {
  const source = await readFile(projectUrl, 'utf8')
  const waitCall = source.indexOf('kernel32::WaitForSingleObject')
  const fileInstall = source.indexOf('!insertmacro wails.files')

  assert.match(source, /\$\{GetOptions\}\s+\$0\s+"\/UPDATE="/)
  assert.match(source, /StrCmp \$1 "1" update_mode_enabled update_mode_parsed/)
  assert.match(source, /\$\{GetOptions\}\s+\$0\s+"\/UPDATE"/)
  assert.match(source, /\$\{GetOptions\}\s+\$0\s+"\/WAITPID="\s+\$UpdateWaitPID/)
  assert.match(source, /IntFmt \$0 "%u" \$UpdateWaitPID/)
  assert.match(source, /StrCmp \$0 \$UpdateWaitPID 0 update_wait_invalid/)
  assert.match(source, /StrCmp \$UpdateWaitPID "0" update_wait_invalid update_wait_open/)
  assert.doesNotMatch(source, /IntCmp \$UpdateWaitPID/, 'Windows process identifiers are unsigned')
  assert.match(
    source,
    /kernel32::OpenProcess\(i 0x00100000[^\r\n]+p\.r0 \?e'\s*\r?\n\s*Pop \$1\s*\r?\n\s*StrCmp \$0 0 update_wait_open_failed/,
    'OpenProcess must capture GetLastError atomically before checking a stale PID',
  )
  assert.doesNotMatch(source, /kernel32::GetLastError/, 'a second plug-in call can overwrite the OpenProcess error')
  assert.match(source, /StrCmp \$UpdateWaitPID "" update_wait_invalid/)
  assert.match(source, /!define UPDATE_WAIT_TIMEOUT_MS 120000/)
  assert.match(
    source,
    /kernel32::WaitForSingleObject\(p r0, i \$\{UPDATE_WAIT_TIMEOUT_MS\}\) i\.r1 \?e'\s*\r?\n\s*Pop \$2/,
  )
  assert.match(source, /kernel32::CloseHandle/)
  assert.match(
    source,
    /update_wait_timeout:[\s\S]*IfSilent update_wait_close_abort update_wait_prompt[\s\S]*IDCANCEL update_wait_close_abort[\s\S]*update_wait_close_abort:[\s\S]*kernel32::CloseHandle\(p r0\)[\s\S]*Goto update_wait_abort/,
    'timeout cancellation must close the process handle before aborting',
  )
  assert.ok(waitCall >= 0 && waitCall < fileInstall, 'the old process must exit before app files are replaced')
  assert.match(source, /MUI_PAGE_CUSTOMFUNCTION_PRE SkipUpdateDirectoryPage/)
  assert.match(source, /Function SkipUpdateDirectoryPage[\s\S]*StrCmp \$UpdateMode "1"[\s\S]*StrCmp \$ExistingInstallFound "1"[\s\S]*Abort/)

  assert.doesNotMatch(
    source,
    /\b(?:taskkill|TerminateProcess|wmic|powershell|cmd\.exe|ExecWait|nsExec)\b/i,
    'the installer must not force-kill the app or execute registry-controlled command strings',
  )
})

test('in-place updates reuse only a validated existing install directory', async () => {
  const source = await readFile(projectUrl, 'utf8')
  const resolverStart = source.indexOf('Function ResolveExistingInstallDirectory')
  const resolverEnd = source.indexOf('FunctionEnd', resolverStart)
  const resolver = source.slice(resolverStart, resolverEnd)

  assert.match(source, /InstallDirRegKey HKCU "\$\{UNINST_KEY\}" "InstallLocation"/)
  assert.match(resolver, /ReadRegStr \$0 HKCU "\$\{UNINST_KEY\}" "InstallLocation"/)
  assert.match(resolver, /ReadRegStr \$1 HKCU "\$\{UNINST_KEY\}" "DisplayIcon"/)
  assert.match(resolver, /IfFileExists "\$0\\\$\{PRODUCT_EXECUTABLE\}"/)
  assert.match(resolver, /IfFileExists "\$0\\uninstall\.exe"/)
  assert.match(resolver, /StrCpy \$INSTDIR "\$0"/)
  assert.match(resolver, /StrCpy \$ExistingInstallFound "1"/)
})

test('updates preserve user data and refresh Windows integration', async () => {
  const source = await readFile(projectUrl, 'utf8')
  const installStart = source.indexOf('Section\n')
  const uninstallStart = source.indexOf('Section "uninstall"')
  const installSection = source.slice(installStart, uninstallStart)
  const updateBranch = installSection.indexOf('update_existing_associations:')
  const normalAssociation = installSection.indexOf('!insertmacro wails.associateFiles')
  const normalBranch = installSection.indexOf('install_new_associations:')

  assert.ok(updateBranch >= 0, 'the update-only integration branch must exist')
  assert.ok(
    normalBranch >= 0 && normalAssociation > normalBranch && updateBranch > normalAssociation,
    'updates must not overwrite the original file-association backup values',
  )
  assert.doesNotMatch(installSection, /\b(?:RMDir|Delete)\s+\/r\b/i)
  assert.match(installSection, /CreateShortcut "\$SMPROGRAMS/)
  assert.match(installSection, /CreateShortCut "\$DESKTOP/)
  assert.match(installSection, /!insertmacro wails\.writeUninstaller/)
  assert.match(installSection, /WriteRegStr SHELL_CONTEXT "\$\{UNINST_KEY\}" "InstallLocation" "\$INSTDIR"/)
  assert.match(installSection, /Software\\Classes\\InkMark Markdown Document\\shell\\open\\command/)
  assert.match(installSection, /\$\{RefreshShellIcons\}/)
})

test('uninstall removes only InkMark-owned files and never recursively deletes a user-selected folder', async () => {
  const source = await readFile(projectUrl, 'utf8')
  const uninstallStart = source.indexOf('Section "uninstall"')
  const uninstall = source.slice(uninstallStart)

  assert.match(uninstall, /Delete "\$INSTDIR\\\$\{PRODUCT_EXECUTABLE\}"/)
  assert.match(uninstall, /Delete "\$INSTDIR\\appicon\.ico"/)
  assert.match(uninstall, /RMDir "\$INSTDIR"/)
  assert.doesNotMatch(uninstall, /RMDir\s+\/r/i)
  assert.doesNotMatch(uninstall, /\$AppData/i)
})

test('Windows packaging remains per-user and runs installer regression tests', async () => {
  const buildScript = await readFile(buildScriptUrl, 'utf8')

  assert.match(buildScript, /wails[^\r\n]+build[^\r\n]+-nsis[^\r\n]+-installscope user/i)
  assert.match(buildScript, /pnpm --dir frontend test:installer/)
})

test('Windows installer quotes the executable used by file associations', async () => {
  const source = await readFile(projectUrl, 'utf8')
  const generatedAssociation = source.indexOf('!insertmacro wails.associateFiles')
  const correctedAssociation = source.indexOf(
    'WriteRegStr SHELL_CONTEXT "Software\\Classes\\InkMark Markdown Document\\shell\\open\\command" "" "$\\\"$INSTDIR\\${PRODUCT_EXECUTABLE}$\\\" $\\\"%1$\\\""',
    generatedAssociation + 1,
  )

  assert.ok(generatedAssociation >= 0, 'the Wails association macro must run')
  assert.ok(
    correctedAssociation > generatedAssociation,
    'the quoted command must overwrite the generated unquoted command',
  )
})

test('Markdown extensions use the ProgID corrected by the installer', async () => {
  const config = JSON.parse(await readFile(configUrl, 'utf8'))
  const associations = config.info.fileAssociations

  assert.deepEqual(
    associations.map(({ ext, name }) => ({ ext, name })),
    [
      { ext: 'md', name: 'InkMark Markdown Document' },
      { ext: 'markdown', name: 'InkMark Markdown Document' },
    ],
  )
})

test('Windows executable embeds complete bilingual version metadata', async () => {
  const versionInfo = JSON.parse(await readFile(versionInfoUrl, 'utf8'))

  assert.equal(versionInfo.fixed.file_version, '{{.Info.ProductVersion}}.0')
  assert.equal(versionInfo.fixed.product_version, '{{.Info.ProductVersion}}.0')
  assert.equal(versionInfo.fixed.type, 'app')
  assert.deepEqual(Object.keys(versionInfo.info).sort(), ['0409', '0804'])
  for (const language of ['0409', '0804']) {
    const fields = versionInfo.info[language]
    for (const key of [
      'CompanyName',
      'FileDescription',
      'FileVersion',
      'InternalName',
      'LegalCopyright',
      'OriginalFilename',
      'ProductName',
      'ProductVersion',
    ]) {
      assert.ok(fields[key]?.trim(), `${language}.${key} must be present`)
    }
    assert.equal(fields.OriginalFilename, '{{.OutputFilename}}')
  }
})
