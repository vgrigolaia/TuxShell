import { Events } from '@wailsio/runtime'

export interface SessionHandlers {
  data: (bytes: Uint8Array) => void
  exit: (code: number) => void
}

type Pending = { kind: 'data'; bytes: Uint8Array } | { kind: 'exit'; code: number }

const handlers = new Map<string, SessionHandlers>()
// A process can print and even exit before Start* resolves with its ID,
// so events for not-yet-claimed sessions are held until a pane claims them.
const pending = new Map<string, Pending[]>()

function decode(b64: string): Uint8Array {
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return bytes
}

function deliver(id: string, ev: Pending) {
  const h = handlers.get(id)
  if (!h) {
    const queue = pending.get(id) ?? []
    queue.push(ev)
    pending.set(id, queue)
    return
  }
  if (ev.kind === 'data') h.data(ev.bytes)
  else {
    handlers.delete(id)
    h.exit(ev.code)
  }
}

Events.On('terminal:data', (ev) => deliver(ev.data.id, { kind: 'data', bytes: decode(ev.data.data) }))
Events.On('terminal:exit', (ev) => deliver(ev.data.id, { kind: 'exit', code: ev.data.code }))

/** Routes a session's events to h, replaying anything that arrived first. */
export function claim(id: string, h: SessionHandlers) {
  handlers.set(id, h)
  const queue = pending.get(id)
  pending.delete(id)
  queue?.forEach((ev) => deliver(id, ev))
}

/** Stops routing a session's events, e.g. when its pane is destroyed. */
export function release(id: string) {
  handlers.delete(id)
  pending.delete(id)
}
