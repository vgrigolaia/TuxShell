import * as Terminals from '../../bindings/github.com/vgrigolaia/tuxshell/internal/terminal/service'
import type { Echo } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/terminal/models'

/** A moment the window stopped responding. */
export interface Freeze {
  at: number
  ms: number
}

// WebKit keeps about 16 WebGL contexts and silently drops the oldest beyond
// that, which left those terminals on the slow DOM renderer for good. Only the
// most recently shown terminals keep one; the others give theirs back.
export const MAX_GPU = 12
// A timer that fires this much late means the window was busy (not counted
// while hidden, or after a suspend).
const FREEZE_MS = 150
const MAX_FREEZES = 20
// How often the send/draw times shown in the status bar are refreshed.
const PUBLISH_MS = 250

/** Speed of the active terminal and of the window, for the status bar and Preferences → About. */
class Perf {
  /** Echo of the watched session, refreshed every 2 s. */
  echo = $state<Echo | null>(null)
  /** Moving average (ms) from a key's echo arriving to it being drawn, per session. */
  draw = $state<Record<string, number>>({})
  /** Moving average (ms) of handing typed keys to Go (the call's round trip), per session. */
  send = $state<Record<string, number>>({})
  /** Window stalls, newest first. */
  freezes = $state<Freeze[]>([])
  /** Switching to an open tab until it's drawn (ms): the last one and a moving average. */
  tabSwitch = $state<{ last: number; avg: number } | null>(null)
  /** How each terminal draws, by pane token. */
  renderer = $state<Record<number, 'gpu' | 'standard'>>({})

  private order = $state<number[]>([])
  private nextToken = 1
  // Averages are kept here on every key and copied to draw/send at most every
  // PUBLISH_MS, so typing doesn't re-render the status bar for each key.
  private rawDraw: Record<string, number> = {}
  private rawSend: Record<string, number> = {}
  private publishTimer: ReturnType<typeof setTimeout> | undefined

  private publish() {
    this.publishTimer ??= setTimeout(() => {
      this.publishTimer = undefined
      this.draw = { ...this.rawDraw }
      this.send = { ...this.rawSend }
    }, PUBLISH_MS)
  }

  /** Starts the freeze monitor; returns its stop function. */
  start() {
    let last = performance.now()
    const tick = setInterval(() => {
      const now = performance.now()
      const late = now - last - 1000
      last = now
      if (late >= FREEZE_MS && late < 30_000 && document.visibilityState === 'visible') {
        this.freezes = [{ at: Date.now(), ms: Math.round(late) }, ...this.freezes].slice(0, MAX_FREEZES)
      }
    }, 1000)
    return () => clearInterval(tick)
  }

  /** Polls the echo time of session id ("" for none); returns the stop function. */
  watch(id: string) {
    this.echo = null
    if (!id) return () => {}
    const poll = () => void Terminals.Echo(id).then((e) => (this.echo = e?.samples ? e : null), () => (this.echo = null))
    poll()
    const timer = setInterval(poll, 2000)
    return () => clearInterval(timer)
  }

  /** Records one echo's drawing time for a session. */
  drawn(id: string, ms: number) {
    const avg = this.rawDraw[id]
    this.rawDraw[id] = avg === undefined ? ms : avg + (ms - avg) / 8
    this.publish()
  }

  /** Records how long one write of typed keys took to reach Go and return. */
  sent(id: string, ms: number) {
    const avg = this.rawSend[id]
    this.rawSend[id] = avg === undefined ? ms : avg + (ms - avg) / 8
    this.publish()
  }

  forgetSession(id: string) {
    delete this.rawDraw[id]
    delete this.rawSend[id]
    this.publish()
  }

  /** Records one switch to an already open tab. */
  tabSwitched(ms: number) {
    const avg = this.tabSwitch?.avg
    this.tabSwitch = { last: ms, avg: avg === undefined ? ms : avg + (ms - avg) / 4 }
  }

  /** A token for a new terminal pane. */
  token() {
    return this.nextToken++
  }

  /** The pane was shown: it moves to the front of the GPU queue. */
  shown(token: number) {
    if (this.order[0] !== token) this.order = [token, ...this.order.filter((t) => t !== token)]
  }

  /** Whether the pane may hold a WebGL context now. */
  mayUseGpu(token: number) {
    const i = this.order.indexOf(token)
    return i >= 0 && i < MAX_GPU
  }

  /** The pane is gone. */
  forget(token: number) {
    this.order = this.order.filter((t) => t !== token)
    delete this.renderer[token]
  }
}

export const perf = new Perf()
