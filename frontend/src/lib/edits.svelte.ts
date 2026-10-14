import { Events } from '@wailsio/runtime'
import * as Files from '../../bindings/github.com/vgrigolaia/tuxshell/internal/files/service'
import type { Edit } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/files/models'
import { errorMessage } from './hosts.svelte'

export type { Edit }

/** Message of files.ErrChangedOnServer. */
const CHANGED_ON_SERVER = 'the file changed on the server since you opened it'

/**
 * Files open in a desktop app (Go keeps the copies). A save in the app marks
 * the edit as changed; nothing is uploaded until the user says so.
 */
class Edits {
  list = $state<Edit[]>([])
  /** Per edit: an upload in progress, a conflict to confirm, or an error. */
  state = $state<Record<string, { busy?: boolean; conflict?: boolean; error?: string; saved?: boolean }>>({})

  readonly changed = $derived(this.list.some((e) => e.changed))

  start() {
    void this.refresh()
    return Events.On('files:edited', () => void this.refresh())
  }

  async refresh() {
    try {
      this.list = ((await Files.Edits()) ?? []).sort((a, b) => Number(a.id) - Number(b.id))
    } catch {
      // keep the last list
    }
  }

  async open(...args: Parameters<typeof Files.Edit>) {
    const e = await Files.Edit(...args)
    await this.refresh()
    return e
  }

  /** Uploads the edited copy; force overwrites a file changed on the server. */
  async upload(id: string, force = false) {
    this.state[id] = { busy: true }
    try {
      await Files.SaveEdit(id, force)
      this.state[id] = { saved: true }
    } catch (e) {
      const msg = errorMessage(e)
      this.state[id] = msg.includes(CHANGED_ON_SERVER) ? { conflict: true } : { error: msg }
    }
    await this.refresh()
  }

  async close(id: string) {
    await Files.CloseEdit(id)
    delete this.state[id]
    await this.refresh()
  }
}

export const edits = new Edits()
