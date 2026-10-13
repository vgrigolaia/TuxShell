import { Events } from '@wailsio/runtime'
import * as ClipService from '../../bindings/github.com/vgrigolaia/tuxshell/internal/cliphist/service'
import type { Clip, Status } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/cliphist/models'

export type { Clip }

class ClipboardHistory {
  list = $state<Clip[]>([])
  status = $state<Status>({ enabled: false, paused: false, error: '' })

  constructor() {
    Events.On('clipboard:changed', () => void this.load())
  }

  async load() {
    this.status = await ClipService.Status()
    this.list = (await ClipService.List()) ?? []
  }

  /** Records text copied inside TuxShell (filtered and encrypted in Go). */
  record(text: string) {
    void ClipService.Add(text)
  }

  async setPaused(paused: boolean) {
    await ClipService.SetPaused(paused)
    this.status = await ClipService.Status()
  }

  remove(id: number) {
    return ClipService.Delete(id)
  }

  clear() {
    return ClipService.Clear()
  }
}

export const clipboardHistory = new ClipboardHistory()
