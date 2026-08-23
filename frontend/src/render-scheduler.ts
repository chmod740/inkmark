/**
 * A small latest-wins scheduler for preview work.  It deliberately owns the
 * debounce timer so an immediate activation can consume a pending edit render
 * instead of running a second copy of the same work.
 */
export class LatestRenderScheduler<Request> {
  private timer: number | undefined
  private pending: Request | null = null
  private readonly run: (request: Request) => Promise<void> | void

  constructor(run: (request: Request) => Promise<void> | void) {
    this.run = run
  }

  request(request: Request, delayMilliseconds = 0) {
    this.pending = request
    if (this.timer !== undefined) window.clearTimeout(this.timer)
    if (delayMilliseconds <= 0) {
      this.timer = undefined
      return this.flush()
    }
    this.timer = window.setTimeout(() => {
      this.timer = undefined
      void this.flush()
    }, delayMilliseconds)
    return Promise.resolve()
  }

  async flush() {
    if (this.timer !== undefined) {
      window.clearTimeout(this.timer)
      this.timer = undefined
    }
    const request = this.pending
    this.pending = null
    if (request === null) return false
    await this.run(request)
    return true
  }

  cancel() {
    if (this.timer !== undefined) window.clearTimeout(this.timer)
    this.timer = undefined
    this.pending = null
  }
}
