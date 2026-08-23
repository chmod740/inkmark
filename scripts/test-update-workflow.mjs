import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import { UpdateDownloadSessionGate } from '../frontend/src/update-session.ts'

const appURL = new URL('../frontend/src/App.vue', import.meta.url)
const bindingsURL = new URL('../frontend/wailsjs/go/main/App.js', import.meta.url)
const backendURL = new URL('../update_download.go', import.meta.url)
const launchURL = new URL('../update_launch.go', import.meta.url)

test('the frontend downloads a verified backend-selected installer before requesting quit', async () => {
  const source = await readFile(appURL, 'utf8')
  const workflowStart = source.indexOf('async function upgradeApplication()')
  const workflowEnd = source.indexOf('async function cancelUpdateDownload()', workflowStart)
  const workflow = source.slice(workflowStart, workflowEnd)

  const download = workflow.indexOf('await DownloadUpdate(downloadSession)')
  const pending = workflow.indexOf('pendingUpdateInstall = true', download)
  const quit = workflow.indexOf('requestApplicationQuit()', pending)
  assert.ok(download >= 0 && pending > download && quit > pending)
  assert.match(workflow, /await DownloadUpdate\(downloadSession\)/)
  assert.doesNotMatch(workflow, /DownloadUpdate\([^)]*(?:URL|url)/, 'the frontend must not choose the download URL')
})

test('the native close guard resolves unsaved work before launching the installer', async () => {
  const source = await readFile(appURL, 'utf8')
  const closeStart = source.indexOf('async function handleCloseRequest()')
  const closeEnd = source.indexOf('async function showWelcome()', closeStart)
  const closeWorkflow = source.slice(closeStart, closeEnd)

  const guard = closeWorkflow.indexOf('await resolveAllDirtyTabsBeforeQuit()')
  const launch = closeWorkflow.indexOf('await LaunchUpdateInstaller()', guard)
  const confirm = closeWorkflow.indexOf('await ConfirmQuit()', launch)
  assert.ok(guard >= 0 && launch > guard && confirm > launch)
  assert.match(source, /async function resolveAllDirtyTabsBeforeQuit\(\)[\s\S]*requestUnsavedDecision\('quit'\)/)
  assert.match(closeWorkflow, /if \(!completed\)[\s\S]*CancelQuitRequest\(\)/)
  assert.match(closeWorkflow, /pendingUpdateInstall = false/)
})

test('download progress, cancellation, and install states are exposed in the About dialog', async () => {
  const source = await readFile(appURL, 'utf8')
  assert.match(source, /EventsOn\('inkmark:update-progress', handleUpdateProgress\)/)
  assert.match(source, /await CancelUpdateDownload\(\)/)
  assert.match(source, /type UpdateState = [^\n]*'downloading'[^\n]*'cancelling'[^\n]*'ready'[^\n]*'installing'/)
  assert.match(source, /updateDownloadSessions\.isActive\(payload\?\.sessionID \|\| ''\)/)
  assert.match(source, /<progress :value="updateDownload\?\.progress \|\| 0" max="1"><\/progress>/)
  assert.match(source, /updateInfo\?\.installable && updateInfo\?\.checksumAvailable/)
  assert.match(
    source,
    /v-else-if="activeDialog === 'about'[\s\S]*&& updateInfo\?\.updateAvailable[\s\S]*&& \['available', 'ready', 'installing'\]\.includes\(updateState\)"/,
    'the install action must only appear after a newer version is found',
  )
  assert.doesNotMatch(source, /action === 'upgrade'/, 'the native menu must not expose an unconditional upgrade action')
})

test('the current-version status never reports an older remote release as installed', async () => {
  const source = await readFile(appURL, 'utf8')
  const statusStart = source.indexOf('const updateStatusText = computed(')
  const statusEnd = source.indexOf('const updatePublishedAt = computed(', statusStart)
  const status = source.slice(statusStart, statusEnd)

  assert.match(status, /const currentVersion = updateInfo\.value\?\.currentVersion \|\| aboutVersion\.value/)
  assert.match(status, /updateState\.value === 'current'[\s\S]*version: currentVersion/)
  assert.match(status, /updateState\.value === 'available'[\s\S]*version: latestVersion/)
})

test('late results and progress from a cancelled download cannot enter a newer session', () => {
  const sessions = new UpdateDownloadSessionGate()
  const cancelled = sessions.begin(100)
  assert.equal(sessions.isActive(cancelled), true)

  const retry = sessions.begin(101)
  assert.equal(sessions.isActive(cancelled), false)
  assert.equal(sessions.finish(cancelled), false)
  assert.equal(sessions.isActive(retry), true)
  assert.equal(sessions.finish(retry), true)
  assert.equal(sessions.isActive(retry), false)
})

test('Wails bindings and backend expose download, cancellation, and safe installer handoff methods', async () => {
  const [bindings, backend, launch] = await Promise.all([
    readFile(bindingsURL, 'utf8'),
    readFile(backendURL, 'utf8'),
    readFile(launchURL, 'utf8'),
  ])
  for (const method of ['DownloadUpdate', 'CancelUpdateDownload', 'LaunchUpdateInstaller']) {
    assert.match(bindings, new RegExp(`export function ${method}\\(`))
  }
  assert.match(backend, /func \(a \*App\) DownloadUpdate\(sessionID string\)/)
  assert.match(backend, /sha256\.New\(\)/)
  assert.match(backend, /\.part/)
  assert.match(launch, /"\/WAITPID="\+strconv\.Itoa\(request\.ParentPID\)/)
  assert.match(launch, /exec\.Command\([\s\S]*request\.InstallerPath/)
  assert.match(launch, /fileMatchesUpdate\(update\.Path, update\.Digest, update\.Size\)/)
  assert.doesNotMatch(launch, /exec\.Command\((?:"cmd"|"cmd\.exe"|"sh"|"bash"|"powershell")/i)
})
