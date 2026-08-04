export type UnsavedDecision = 'save' | 'discard' | 'cancel'
export type DocumentTransition = 'new' | 'open' | 'quit'

export interface UnsavedGuardOptions {
  dirty: boolean
  requestDecision: () => Promise<UnsavedDecision>
  save: () => Promise<boolean>
}

export interface DocumentTransitionOptions extends UnsavedGuardOptions {
  transition: () => Promise<void> | void
}

export async function guardUnsavedChanges(options: UnsavedGuardOptions): Promise<boolean> {
  if (!options.dirty) return true

  const decision = await options.requestDecision()
  if (decision === 'discard') return true
  if (decision === 'save') return options.save()
  return false
}

export async function runGuardedDocumentTransition(options: DocumentTransitionOptions): Promise<boolean> {
  if (!(await guardUnsavedChanges(options))) return false
  await options.transition()
  return true
}
