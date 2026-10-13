import { Events } from '@wailsio/runtime'
import * as StatusService from '../../bindings/github.com/vgrigolaia/tuxshell/internal/status/service'
import type { Result } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/status/models'
import { prefs } from './settings.svelte'
import { workspace } from './workspace.svelte'

export type HostState = 'open' | 'up' | 'slow' | 'down' | 'unknown'

/** Ping results from the Go status service, plus which hosts have a live session. */
class HostStatus {
  private results = $state<Record<number, Result>>({})

  /** Hosts with a running session count as up, whatever ping says. */
  private open = $derived(new Set(workspace.panes.filter((p) => p.source.kind === 'host' && !p.exited).map((p) => (p.source as { hostId: number }).hostId)))

  start() {
    void StatusService.Results().then((list) => this.set(list ?? []), () => {})
    return Events.On('hosts:status', (ev) => this.set(ev.data ?? []))
  }

  private set(list: Result[]) {
    const next: Record<number, Result> = {}
    for (const r of list) next[r.hostId] = r
    this.results = next
  }

  state(hostId: number): HostState {
    if (this.open.has(hostId)) return 'open'
    if (!prefs.value.hostStatus) return 'unknown'
    const r = this.results[hostId]
    return r && r.state !== 'skipped' ? (r.state as HostState) : 'unknown'
  }

  ms(hostId: number): number | null {
    const r = this.results[hostId]
    return r && (r.state === 'up' || r.state === 'slow') ? r.ms : null
  }

  /** "23 up" in the hosts header: answering hosts plus hosts with a session. */
  upCount(ids: number[]): number {
    return ids.filter((id) => ['open', 'up', 'slow'].includes(this.state(id))).length
  }

  refresh() {
    void StatusService.Refresh().catch(() => {})
  }

  label(hostId: number): string {
    const ms = this.ms(hostId)
    switch (this.state(hostId)) {
      case 'open':
        return 'Session open'
      case 'up':
        return `Online · ${ms! < 1 ? '<1' : Math.round(ms!)} ms`
      case 'slow':
        return `Slow · ${Math.round(ms!)} ms`
      case 'down':
        return 'No ping reply'
      default:
        return ''
    }
  }
}

export const hostStatus = new HostStatus()
