import * as KeePassXC from '../../bindings/github.com/vgrigolaia/tuxshell/internal/keepassxc/service'
import type { Status } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/keepassxc/models'
import { errorMessage } from './hosts.svelte'

/** Whether KeePassXC answers and knows TuxShell (never any secret). */
class KeePassXCStatus {
  status = $state<Status | null>(null)
  /** Waiting for the user in KeePassXC's "allow TuxShell" window. */
  connecting = $state(false)
  error = $state('')

  async refresh() {
    try {
      this.status = await KeePassXC.Status()
    } catch {
      // keep the last status
    }
  }

  async connect() {
    this.connecting = true
    this.error = ''
    try {
      await KeePassXC.Connect()
    } catch (e) {
      this.error = errorMessage(e)
    } finally {
      this.connecting = false
    }
    await this.refresh()
  }

  async disconnect() {
    this.error = ''
    try {
      await KeePassXC.Disconnect()
    } catch (e) {
      this.error = errorMessage(e)
    }
    await this.refresh()
  }
}

export const keePassXC = new KeePassXCStatus()
