import { Events } from '@wailsio/runtime'
import * as Login from '../../bindings/github.com/vgrigolaia/tuxshell/internal/login/service'
import type { PasswordRequest, Status } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/login/models'
import { hostStore } from './hosts.svelte'

export type { PasswordRequest }

/**
 * Password boxes ssh asked for (through the Go login service), keyed by the
 * terminal session they belong to, and the saved-password status.
 */
class Logins {
  requests = $state<PasswordRequest[]>([])
  status = $state<Status>({ available: true, saved: 0 })

  start() {
    void this.refresh()
    const offAsk = Events.On('login:password', (ev) => {
      const r = ev.data as PasswordRequest
      this.requests = [...this.requests.filter((x) => x.sessionId !== r.sessionId), r]
    })
    const offDone = Events.On('login:done', (ev) => this.drop(ev.data as string))
    return () => {
      offAsk()
      offDone()
    }
  }

  async refresh() {
    try {
      this.status = await Login.Status()
    } catch {
      // keep the last status
    }
  }

  forSession(id: string | undefined): PasswordRequest | undefined {
    return id ? this.requests.find((r) => r.sessionId === id) : undefined
  }

  private drop(requestId: string) {
    this.requests = this.requests.filter((r) => r.requestId !== requestId)
  }

  async answer(r: PasswordRequest, password: string, remember: boolean) {
    this.drop(r.requestId)
    await Login.Answer(r.requestId, password, remember)
    if (remember) {
      // The host now shows "password saved".
      setTimeout(() => {
        void hostStore.load()
        void this.refresh()
      }, 300)
    }
  }

  async cancel(r: PasswordRequest) {
    this.drop(r.requestId)
    await Login.Cancel(r.requestId).catch(() => {})
  }
}

export const logins = new Logins()
