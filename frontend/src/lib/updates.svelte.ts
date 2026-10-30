import * as UpdateService from '../../bindings/github.com/vgrigolaia/tuxshell/internal/updates/service'
import type { Result } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/updates/models'

/** The running version (from package.json). */
export const VERSION = __APP_VERSION__

/** Asks the Go updates service whether a newer release is out. Nothing is downloaded. */
class Updates {
  checking = $state(false)
  result = $state<Result | null>(null)
  error = $state<string | null>(null)
  /** True in the Flatpak: Flathub and the software center keep TuxShell up to date. */
  managed = $state(false)
  /** "apt" or "dnf" when TuxShell was installed from its package repository (pkg.tuxshell.io), else "". */
  tool = $state('')
  /** The command that updates TuxShell from that repository. */
  command = $derived(
    this.tool === 'apt' ? 'sudo apt update && sudo apt upgrade' : this.tool === 'dnf' ? 'sudo dnf upgrade --refresh tuxshell' : '',
  )

  /** The check at startup when it's on, unless Flathub does the updating. */
  async atStartup(on: boolean) {
    try {
      this.managed = await UpdateService.Managed()
      this.tool = await UpdateService.Repo()
    } catch {
      // Older Go side or a binding error: behave as before.
    }
    if (on && !this.managed) await this.check()
  }

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
