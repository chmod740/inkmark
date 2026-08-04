export type ScrollPane = 'editor' | 'preview'

export interface ScrollViewport {
  scrollTop: number
  scrollHeight: number
  clientHeight: number
}

const scrollTolerance = 0.5

export class ScrollSyncController {
  private activePane: ScrollPane | null = null

  begin(pane: ScrollPane) {
    this.activePane = pane
  }

  reset() {
    this.activePane = null
  }

  sync(pane: ScrollPane, source: ScrollViewport, target: ScrollViewport) {
    // Scroll events caused by writing target.scrollTop must never become a
    // second source event. Only the pane with current user intent may drive.
    if (this.activePane !== pane) return false

    const sourceRange = Math.max(0, source.scrollHeight - source.clientHeight)
    const targetRange = Math.max(0, target.scrollHeight - target.clientHeight)
    if (sourceRange === 0 || targetRange === 0) return false

    const ratio = Math.min(1, Math.max(0, source.scrollTop / sourceRange))
    const nextTop = ratio * targetRange
    if (Math.abs(target.scrollTop - nextTop) <= scrollTolerance) return false

    target.scrollTop = nextTop
    return true
  }
}
