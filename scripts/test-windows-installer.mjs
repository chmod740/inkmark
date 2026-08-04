import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const projectUrl = new URL('../build/windows/installer/project.nsi', import.meta.url)
const configUrl = new URL('../wails.json', import.meta.url)
const versionInfoUrl = new URL('../build/windows/info.json', import.meta.url)

test('Windows installer quotes the executable used by file associations', async () => {
  const source = await readFile(projectUrl, 'utf8')
  const generatedAssociation = source.indexOf('!insertmacro wails.associateFiles')
  const correctedAssociation = source.indexOf(
    'WriteRegStr SHELL_CONTEXT "Software\\Classes\\InkMark Markdown Document\\shell\\open\\command" "" "$\\\"$INSTDIR\\${PRODUCT_EXECUTABLE}$\\\" $\\\"%1$\\\""',
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
  }
})
