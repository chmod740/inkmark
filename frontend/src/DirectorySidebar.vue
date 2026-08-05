<script setup lang="ts">
import type { WorkspaceData, WorkspaceEntryData, WorkspaceTreeRow } from './workspace-tree'
import { sameWorkspaceFile, workspaceDirectoryKey, workspaceTreeIndent } from './workspace-tree'

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
}

const props = defineProps<{
  workspace: WorkspaceData
  rows: readonly WorkspaceTreeRow[]
  currentPath: string
  labels: SidebarLabels
  disabled: boolean
  refreshing: boolean
  truncatedDirectories: ReadonlySet<string>
}>()

const emit = defineEmits<{
  close: []
  refresh: []
  toggle: [entry: WorkspaceEntryData]
  open: [entry: WorkspaceEntryData]
}>()

function activate(row: WorkspaceTreeRow) {
  if (row.entry.kind === 'directory') emit('toggle', row.entry)
  else emit('open', row.entry)
}

function handleTreeKeydown(event: KeyboardEvent, row: WorkspaceTreeRow) {
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
  return props.labels.openFile
}

function isActive(row: WorkspaceTreeRow) {
  return row.entry.kind === 'markdown'
    && sameWorkspaceFile(row.entry.absolutePath, props.currentPath)
}

function rowDisabled(row: WorkspaceTreeRow) {
  if (row.loading) return true
  if (!props.disabled) return false
  return !(row.entry.kind === 'directory' && row.loaded)
}
</script>

<template>
  <aside class="workspace-sidebar" :aria-label="labels.title">
    <header class="workspace-sidebar-header">
      <div class="workspace-sidebar-heading" :title="workspace.path">
        <span class="workspace-root-icon" aria-hidden="true"></span>
        <span class="workspace-root-name">{{ workspace.name }}</span>
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
        @click="emit('close')"
      >×</button>
    </header>

    <div class="workspace-root-path" :title="workspace.path">{{ workspace.path }}</div>

    <nav class="workspace-tree" :aria-label="labels.title">
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
        :title="`${rowTitle(row)} — ${row.entry.absolutePath || row.entry.path}`"
        :disabled="rowDisabled(row)"
        @click="activate(row)"
        @keydown="handleTreeKeydown($event, row)"
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
  </aside>
</template>
