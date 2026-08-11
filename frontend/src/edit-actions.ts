export type TextEditControl = HTMLInputElement | HTMLTextAreaElement

const selectableInputTypes = new Set(['password', 'search', 'tel', 'text', 'url'])

interface TextEditControlShape {
  tagName?: unknown
  type?: unknown
  disabled?: unknown
  readOnly?: unknown
  isConnected?: unknown
  value?: unknown
  selectionStart?: unknown
  selectionEnd?: unknown
  focus?: unknown
  setSelectionRange?: unknown
  setRangeText?: unknown
  dispatchEvent?: unknown
}

export function isTextEditControl(value: unknown): value is TextEditControl {
  if (!value || typeof value !== 'object') return false
  const control = value as TextEditControlShape
  const tagName = typeof control.tagName === 'string' ? control.tagName.toLowerCase() : ''
  if (tagName !== 'textarea') {
    if (tagName !== 'input') return false
    const inputType = typeof control.type === 'string' ? control.type.toLowerCase() : 'text'
    if (!selectableInputTypes.has(inputType || 'text')) return false
  }
  if (control.disabled === true || control.readOnly === true || control.isConnected === false) return false
  return typeof control.value === 'string'
    && typeof control.selectionStart === 'number'
    && typeof control.selectionEnd === 'number'
    && typeof control.focus === 'function'
    && typeof control.setSelectionRange === 'function'
    && typeof control.setRangeText === 'function'
    && typeof control.dispatchEvent === 'function'
}

export function resolveTextEditControl(
  activeElement: unknown,
  lastFocusedControl: unknown,
  fallbackControl: unknown,
): TextEditControl | null {
  if (isTextEditControl(activeElement)) return activeElement
  if (isTextEditControl(lastFocusedControl)) return lastFocusedControl
  if (isTextEditControl(fallbackControl)) return fallbackControl
  return null
}

export function dispatchTextEditInput(
  control: TextEditControl,
  inputType: string,
  data: string | null = null,
) {
  let event: Event
  if (typeof InputEvent === 'function') {
    event = new InputEvent('input', { bubbles: true, inputType, data })
  } else {
    event = new Event('input', { bubbles: true })
  }
  control.dispatchEvent(event)
}
