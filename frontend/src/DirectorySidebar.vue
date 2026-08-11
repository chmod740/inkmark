<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type {
  WorkspaceData,
  WorkspaceEntryData,
  WorkspaceProvider,
  WorkspaceTreeRow,
} from './workspace-tree'
import { isActiveWorkspaceFile, workspaceDirectoryKey, workspaceTreeIndent } from './workspace-tree'

interface SidebarLabels {
  title: string
  close: string
  refresh: string
  refreshing: string
  empty: string
  loading: string
  truncatedRoot: string
  truncatedDirectory: string
  expandDirectory: string
  collapseDirectory: string
  openFile: string
  previewImage: string
  contextMenu: string
  rootActions: string
  newMarkdown: string
  newDirectory: string
  rename: string
  delete: string
  providerWebDAV: string
}

const props = defineProps<{
  workspace: WorkspaceData
  rows: readonly WorkspaceTreeRow[]
  currentProvider: WorkspaceProvider | null
  currentWorkspaceId: string
  currentWorkspacePath: string
  labels: SidebarLabels
  disabled: boolean
  modalOpen: boolean
  refreshing: boolean
  truncatedDirectories: ReadonlySet<string>
}>()

const emit = defineEmits<{
  close: []
  refresh: []
  toggle: [entry: WorkspaceEntryData]
  open: [entry: WorkspaceEntryData]
  preview: [entry: WorkspaceEntryData]
  createMarkdown: [parentPath: string]
  createDirectory: [parentPath: string]
  rename: [entry: WorkspaceEntryData]
  delete: [entry: WorkspaceEntryData]
}>()

const contextMenu = ref<{ entry: WorkspaceEntryData | null; x: number; y: number } | null>(null)
const contextMenuElement = ref<HTMLElement | null>(null)
const contextMenuTrigger = ref<HTMLElement | null>(null)

function closeContextMenu({ restoreFocus = false } = {}) {
  const trigger = contextMenuTrigger.value
  contextMenu.value = null
  contextMenuTrigger.value = null
  if (!restoreFocus) return
  void nextTick(() => {
    if (trigger?.isConnected) trigger.focus({ preventScroll: true })
  })
}

function positionContextMenu(entry: WorkspaceEntryData | null, clientX: number, clientY: number) {
  if (props.disabled || props.modalOpen) return
  contextMenu.value = {
    entry,
    x: Math.max(8, Math.min(clientX, window.innerWidth - 220)),
    y: Math.max(8, Math.min(clientY, window.innerHeight - 220)),
  }
  void nextTick(() => {
    const menu = contextMenuElement.value
    if (!menu || !contextMenu.value) return
    const bounds = menu.getBoundingClientRect()
    contextMenu.value = {
      ...contextMenu.value,
      x: Math.max(8, Math.min(contextMenu.value.x, window.innerWidth - bounds.width - 8)),
      y: Math.max(8, Math.min(contextMenu.value.y, window.innerHeight - bounds.height - 8)),
    }
    menu.querySelector<HTMLElement>('[role="menuitem"]')?.focus({ preventScroll: true })
  })
}

function openPointerContextMenu(event: MouseEvent, entry: WorkspaceEntryData | null) {
  event.preventDefault()
  event.stopPropagation()
  contextMenuTrigger.value = event.currentTarget as HTMLElement
  positionContextMenu(entry, event.clientX, event.clientY)
}

function openKeyboardContextMenu(event: KeyboardEvent, entry: WorkspaceEntryData | null) {
  if (!(event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10'))) return false
  event.preventDefault()
  event.stopPropagation()
  contextMenuTrigger.value = event.currentTarget as HTMLElement
  const bounds = contextMenuTrigger.value.getBoundingClientRect()
  positionContextMenu(entry, bounds.left + Math.min(36, bounds.width), bounds.top + Math.min(24, bounds.height))
  return true
}

function contextParentPath() {
  const entry = contextMenu.value?.entry
  if (!entry) return ''
  if (entry.kind === 'directory') return entry.path
  const normalized = entry.path.replaceAll('\\', '/')
  const separator = normalized.lastIndexOf('/')
  return separator < 0 ? '' : normalized.slice(0, separator)
}

function runContextAction(action: 'create-markdown' | 'create-directory' | 'rename' | 'delete') {
  if (props.disabled || props.modalOpen) {
    closeContextMenu({ restoreFocus: true })
    return
  }
  const entry = contextMenu.value?.entry || null
  const parentPath = contextParentPath()
  const trigger = contextMenuTrigger.value
  if (trigger?.isConnected) trigger.focus({ preventScroll: true })
  contextMenu.value = null
  contextMenuTrigger.value = null
  if (action === 'create-markdown') emit('createMarkdown', parentPath)
  else if (action === 'create-directory') emit('createDirectory', parentPath)
  else if (action === 'rename' && entry) emit('rename', entry)
  else if (action === 'delete' && entry) emit('delete', entry)
}

function handleContextMenuKeydown(event: KeyboardEvent) {
  const menu = contextMenuElement.value
  if (!menu) return
  const items = Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"]:not([disabled])'))
  if (event.key === 'Escape' || event.key === 'Tab') {
    event.preventDefault()
    closeContextMenu({ restoreFocus: true })
    return
  }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key) || !items.length) return
  event.preventDefault()
  const index = items.indexOf(document.activeElement as HTMLElement)
  const nextIndex = event.key === 'Home'
    ? 0
    : event.key === 'End'
      ? items.length - 1
      : event.key === 'ArrowUp'
        ? (index <= 0 ? items.length - 1 : index - 1)
        : (index < 0 || index === items.length - 1 ? 0 : index + 1)
  items[nextIndex].focus({ preventScroll: true })
}

function handleDocumentPointerDown(event: PointerEvent) {
  if (!contextMenu.value || contextMenuElement.value?.contains(event.target as Node)) return
  closeContextMenu()
}

function handleViewportChange() {
  closeContextMenu()
}

onMounted(() => {
  document.addEventListener('pointerdown', handleDocumentPointerDown, true)
  document.addEventListener('scroll', handleViewportChange, true)
  window.addEventListener('blur', handleViewportChange)
  window.addEventListener('resize', handleViewportChange)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', handleDocumentPointerDown, true)
  document.removeEventListener('scroll', handleViewportChange, true)
  window.removeEventListener('blur', handleViewportChange)
  window.removeEventListener('resize', handleViewportChange)
})

watch(() => props.disabled || props.modalOpen, (unavailable) => {
  if (unavailable) closeContextMenu({ restoreFocus: true })
})

function activate(row: WorkspaceTreeRow) {
  if (row.entry.kind === 'directory') emit('toggle', row.entry)
  else if (row.entry.kind === 'image') emit('preview', row.entry)
  else emit('open', row.entry)
}

function handleTreeKeydown(event: KeyboardEvent, row: WorkspaceTreeRow) {
  if (openKeyboardContextMenu(event, row.entry)) return
  if (row.entry.kind !== 'directory') return
  if (event.key === 'ArrowRight' && !row.expanded) {
    event.preventDefault()
    emit('toggle', row.entry)
  } else if (event.key === 'ArrowLeft' && row.expanded) {
    event.preventDefault()
    emit('toggle', row.entry)
  }
}

function rowTitle(row: WorkspaceTreeRow) {
  if (row.entry.kind === 'directory') {
    return row.expanded ? props.labels.collapseDirectory : props.labels.expandDirectory
  }
  if (row.entry.kind === 'image') return props.labels.previewImage
  return props.labels.openFile
}

function isActive(row: WorkspaceTreeRow) {
  return row.entry.kind === 'markdown'
    && isActiveWorkspaceFile(
      props.workspace.provider,
      props.workspace.id,
      row.entry.path,
      props.currentProvider,
      props.currentWorkspaceId,
      props.currentWorkspacePath,
    )
}

function rowDisabled(row: WorkspaceTreeRow) {
  if (row.loading) return true
  if (!props.disabled) return false
  return !(row.entry.kind === 'directory' && row.loaded)
}
</script>

<template>
  <aside
    class="workspace-sidebar"
    :class="`provider-${workspace.provider}`"
    :aria-label="labels.title"
  >
    <header class="workspace-sidebar-header">
      <div class="workspace-sidebar-heading" :title="workspace.path">
        <span
          class="workspace-root-icon"
          :class="{ webdav: workspace.provider === 'webdav' }"
          aria-hidden="true"
        ></span>
        <span class="workspace-root-name">{{ workspace.name }}</span>
        <span
          v-if="workspace.provider === 'webdav'"
          class="workspace-provider-icon webdav"
          role="img"
          :aria-label="labels.providerWebDAV"
          :title="labels.providerWebDAV"
        >☁</span>
      </div>
      <button
        type="button"
        class="workspace-header-button workspace-refresh-button"
        :class="{ 'is-refreshing': refreshing }"
        :aria-label="refreshing ? labels.refreshing : labels.refresh"
        :title="refreshing ? labels.refreshing : labels.refresh"
        :disabled="disabled || refreshing"
        @click="emit('refresh')"
      ><span aria-hidden="true">↻</span></button>
      <button
        type="button"
        class="workspace-header-button workspace-close-button"
        :aria-label="labels.close"
        :title="labels.close"
        :disabled="disabled || modalOpen"
        @click="emit('close')"
      >×</button>
    </header>

    <div class="workspace-root-path" :title="workspace.path">{{ workspace.path }}</div>

    <nav
      class="workspace-tree"
      :aria-label="labels.title"
      aria-describedby="workspace-tree-actions-help"
      :title="labels.rootActions"
      tabindex="0"
      @contextmenu="openPointerContextMenu($event, null)"
      @keydown="openKeyboardContextMenu($event, null)"
    >
      <p id="workspace-tree-actions-help" class="sr-only">{{ labels.rootActions }}</p>
      <p v-if="rows.length === 0" class="workspace-empty">{{ labels.empty }}</p>
      <button
        v-for="row in rows"
        :key="`${row.entry.kind}:${row.entry.path}`"
        type="button"
        class="workspace-tree-row"
        :class="{
          'is-directory': row.entry.kind === 'directory',
          'is-active': isActive(row),
        }"
        :style="{ paddingInlineStart: `${workspaceTreeIndent(row.depth)}px` }"
        :aria-expanded="row.entry.kind === 'directory' ? row.expanded : undefined"
        :aria-current="isActive(row) ? 'page' : undefined"
        :data-workspace-path="row.entry.path"
        :title="`${rowTitle(row)} — ${row.entry.absolutePath || row.entry.path}`"
        :disabled="rowDisabled(row)"
        @click="activate(row)"
        @keydown="handleTreeKeydown($event, row)"
        @contextmenu="openPointerContextMenu($event, row.entry)"
      >
        <span class="workspace-disclosure" aria-hidden="true">
          {{ row.entry.kind === 'directory' ? (row.expanded ? '⌄' : '›') : '' }}
        </span>
        <span
          class="workspace-entry-icon"
          :class="row.entry.kind"
          aria-hidden="true"
        ></span>
        <span class="workspace-entry-name">{{ row.entry.name }}</span>
        <span
          v-if="row.entry.kind === 'directory' && truncatedDirectories.has(workspaceDirectoryKey(row.entry.path))"
          class="workspace-entry-truncated"
          :title="labels.truncatedDirectory"
          :aria-label="labels.truncatedDirectory"
        >…</span>
        <span v-if="row.loading" class="workspace-row-loading" aria-hidden="true"></span>
        <span v-if="row.loading" class="sr-only">{{ labels.loading }}</span>
      </button>
      <p v-if="workspace.truncated" class="workspace-truncated" role="status">{{ labels.truncatedRoot }}</p>
    </nav>

    <Teleport to="body">
      <div
        v-if="contextMenu"
        ref="contextMenuElement"
        class="workspace-context-menu"
        role="menu"
        :aria-label="labels.contextMenu"
        :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }"
        @keydown="handleContextMenuKeydown"
        @contextmenu.prevent
      >
        <button
          v-if="!contextMenu.entry || contextMenu.entry.kind === 'directory'"
          type="button"
          role="menuitem"
          @click="runContextAction('create-markdown')"
        >
          <span aria-hidden="true">M+</span>{{ labels.newMarkdown }}
        </button>
        <button
          v-if="!contextMenu.entry || contextMenu.entry.kind === 'directory'"
          type="button"
          role="menuitem"
          @click="runContextAction('create-directory')"
        >
          <span aria-hidden="true">▣+</span>{{ labels.newDirectory }}
        </button>
        <div
          v-if="contextMenu.entry?.kind === 'directory'"
          class="workspace-context-separator"
          role="separator"
        ></div>
        <button
          v-if="contextMenu.entry"
          type="button"
          role="menuitem"
          @click="runContextAction('rename')"
        ><span aria-hidden="true">✎</span>{{ labels.rename }}</button>
        <button
          v-if="contextMenu.entry"
          type="button"
          class="danger"
          role="menuitem"
          @click="runContextAction('delete')"
        ><span aria-hidden="true">×</span>{{ labels.delete }}</button>
      </div>
    </Teleport>
  </aside>
</template>
