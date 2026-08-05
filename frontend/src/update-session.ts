export class UpdateDownloadSessionGate {
  private sequence = 0
  private activeSession = ''

  begin(timestamp = Date.now()): string {
    this.activeSession = `ui-${timestamp}-${++this.sequence}`
    return this.activeSession
  }

  isActive(sessionID: string): boolean {
    return sessionID !== '' && sessionID === this.activeSession
  }

  finish(sessionID: string): boolean {
    if (!this.isActive(sessionID)) return false
    this.activeSession = ''
    return true
  }
}
