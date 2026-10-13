import { Events } from '@wailsio/runtime'
import * as ClipService from '../../bindings/github.com/vgrigolaia/tuxshell/internal/cliphist/service'
import type { Clip, Settings, Status } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/cliphist/models'

export type { Clip, Settings }

class ClipboardHistory {
  list = $state<Clip[]>([])
  status = $state<Status>({ enabled: false, paused: false, error: '' })
  settings = $state<Settings>({ retentionDays: 30, maxEntries: 500, captureSystem: true })

  saved = $derived(this.list.filter((c) => c.pinned))
  history = $derived(this.list.filter((c) => !c.pinned))

  constructor() {
    Events.On('clipboard:changed', () => void this.load())
  }

  async load() {
    this.status = await ClipService.Status()
    this.settings = await ClipService.Settings()
    this.list = (await ClipService.List()) ?? []
  }

  /** Saves text as a pinned clip; rejects with the Go error message. */
  saveClip(text: string, label: string) {
    return ClipService.SaveClip(text, label)
  }

  updateClip(id: number, text: string, label: string) {
    return ClipService.UpdateClip(id, text, label)
  }

  setPinned(id: number, pinned: boolean) {
    return ClipService.SetPinned(id, pinned)
  }

  async saveSettings(settings: Settings) {
    await ClipService.SaveSettings(settings)
    this.settings = settings
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
