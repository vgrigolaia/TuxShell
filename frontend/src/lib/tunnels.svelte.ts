import { Events } from '@wailsio/runtime'
import * as TunnelService from '../../bindings/github.com/vgrigolaia/tuxshell/internal/tunnels/service'
import type { State } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/tunnels/models'
import type { Tunnel } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/hosts/models'
import { errorMessage, hostStore } from './hosts.svelte'

export type { State as TunnelState, Tunnel }

export const emptyTunnel = (hostId = 0): Tunnel => ({ id: 0, name: '', hostId, kind: 'local', bindPort: 0, targetHost: 'localhost', targetPort: 0 })

/** Saved port forwards and whether each is running (kept up to date by Go events). */
class Tunnels {
  list = $state<State[]>([])
  error = $state<string | null>(null)
  /** Tunnels being started (Start waits a few seconds to confirm). */
  starting = $state<number[]>([])

  running = $derived(this.list.filter((t) => t.status === 'running').length)

  start() {
    void this.load()
    return Events.On('tunnels:changed', () => void this.load())
  }

  async load() {
    try {
      this.list = (await TunnelService.List()) ?? []
      this.error = null
    } catch (e) {
      this.error = errorMessage(e)
    }
  }

  async save(t: Tunnel): Promise<Tunnel> {
    const saved = await TunnelService.Save(t)
    await this.load()
    return saved
  }

  async remove(id: number) {
    await TunnelService.Delete(id)
  }

  /** Starts or stops a tunnel; a failed start shows on the tunnel itself. */
  async toggle(t: State) {
    if (t.status === 'running') return TunnelService.Stop(t.id)
    this.starting = [...this.starting, t.id]
    try {
      await TunnelService.Start(t.id)
    } catch {
      // The message arrives with the next List as status "failed".
    } finally {
      this.starting = this.starting.filter((id) => id !== t.id)
      await this.load()
    }
  }
}

export const tunnels = new Tunnels()

/** ":5432 → pg-primary:5432" or "SOCKS :1080" for display. */
export function describeTunnel(t: Tunnel): string {
  if (t.kind === 'dynamic') return `:${t.bindPort} → SOCKS proxy`
  const target = t.targetHost === 'localhost' || t.targetHost === '127.0.0.1' ? (hostStore.get(t.hostId)?.name ?? t.targetHost) : t.targetHost
  return `:${t.bindPort} → ${target}:${t.targetPort}`
}
