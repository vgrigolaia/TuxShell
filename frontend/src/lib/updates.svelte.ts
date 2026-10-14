import * as UpdateService from '../../bindings/github.com/vgrigolaia/tuxshell/internal/updates/service'
import type { Result } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/updates/models'

/** The running version (from package.json). */
export const VERSION = __APP_VERSION__

/** Asks the Go updates service whether a newer release is out. Nothing is downloaded. */
class Updates {
  checking = $state(false)
  result = $state<Result | null>(null)
  error = $state<string | null>(null)

  async check() {
    if (this.checking) return
    this.checking = true
    this.error = null
    try {
      this.result = await UpdateService.Check(VERSION)
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e)
    } finally {
      this.checking = false
    }
  }
}

export const updates = new Updates()
