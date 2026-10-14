import { Events } from '@wailsio/runtime'
import * as RDP from '../../bindings/github.com/vgrigolaia/tuxshell/internal/rdp/service'
import type { ClientInfo, Session } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/rdp/models'
import { errorMessage, type Host } from './hosts.svelte'

export type { ClientInfo, Session }

/** What rdp.CertError carries when a certificate needs the user's OK. */
export interface CertProblem {
  kind: 'new' | 'changed'
  cert: { fingerprint: string; subject: string; issuer: string; notAfter: number; selfSigned: boolean }
  previous: string
}

/** The result of one connection attempt. */
export type Attempt = { ok: true } | { ok: false; cert: CertProblem } | { ok: false; error: string; field?: string }

/**
 * Remote desktops. They open in FreeRDP's own window; TuxShell keeps the
 * list, the connect window (password and certificate) and the reasons they ended.
 */
class RemoteDesktops {
  sessions = $state<Session[]>([])
  client = $state<ClientInfo | null>(null)
  /** The host whose connect window is open. */
  connecting = $state<Host | null>(null)

  readonly open = $derived(this.sessions.filter((s) => s.state !== 'ended').length)

  start() {
    void this.refresh()
    return Events.On('rdp:changed', () => void this.refresh())
  }

  async refresh() {
    try {
      this.sessions = ((await RDP.Sessions()) ?? []).sort((a, b) => b.started - a.started)
    } catch {
      // keep the last list
    }
  }

  async checkClient() {
    try {
      this.client = await RDP.Client()
    } catch {
      this.client = null
    }
    return this.client
  }

  async connect(host: Host, password: string, remember: boolean, trust = ''): Promise<Attempt> {
    try {
      await RDP.Connect(host.id, { password, remember, trust })
      await this.refresh()
      return { ok: true }
    } catch (e) {
      const cause = (e as { cause?: Partial<CertProblem> & { field?: string; message?: string } })?.cause
      if (cause?.kind === 'new' || cause?.kind === 'changed') return { ok: false, cert: cause as CertProblem }
      return { ok: false, error: cause?.message ?? errorMessage(e), field: cause?.field }
    }
  }

  disconnect(id: string) {
    void RDP.Disconnect(id)
  }

  forget(id: string) {
    void RDP.Forget(id)
  }

  log(id: string) {
    return RDP.Log(id).then((l) => l ?? [])
  }
}

export const remoteDesktops = new RemoteDesktops()

/** "AB:12:…" for showing a fingerprint. */
export const prettyFingerprint = (fp: string) => (fp.match(/.{2}/g) ?? []).join(':').toUpperCase()
