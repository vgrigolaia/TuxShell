import { prefs } from './settings.svelte'
import { workspace } from './workspace.svelte'

/** A close waiting for the user to confirm, shown by App as a dialog. */
export interface PendingClose {
  title: string
  message: string
  run: () => void | Promise<void>
}

class Closer {
  pending = $state<PendingClose | null>(null)

  /** Runs run(), first asking when it would end live sessions (if enabled). */
  confirm(paneIds: number[], what: string, run: () => void) {
    const live = paneIds.filter((id) => !workspace.pane(id)?.exited).length
    if (!live || !prefs.value.terminal.confirmClose) return run()
    this.ask(live, what, run)
  }

  /** "Don't ask again": turns off the question (Preferences can turn it back on). */
  stopAsking() {
    prefs.update({ confirmClose: false })
    return prefs.saveNow()
  }

  ask(live: number, what: string, run: () => void | Promise<void>) {
    this.pending = {
      title: `Close ${what}?`,
      message: `${live} ${live === 1 ? 'session is' : 'sessions are'} still running and will be disconnected.`,
      run,
    }
  }

  closePane(paneId: number) {
    this.confirm([paneId], 'this pane', () => workspace.closePane(paneId))
  }

  closeTabs(keys: number[]) {
    const ids = workspace.tabs.filter((t) => keys.includes(t.key)).flatMap((t) => t.paneIds)
    this.confirm(ids, keys.length === 1 ? 'this tab' : `${keys.length} tabs`, () => workspace.closeTabs(keys))
  }
}

export const closer = new Closer()
