import * as HostService from '../../bindings/github.com/vgrigolaia/tuxshell/internal/hosts/service'
import type { Host } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/hosts/models'

export type { Host }

export interface HostGroup {
  name: string
  hosts: Host[]
}

/** A field-level error from the Go validator, or a general one. */
export interface SaveError {
  field?: string
  message: string
}

export const emptyHost = (): Host => ({
  id: 0,
  name: '',
  group: '',
  alias: '',
  hostname: '',
  user: '',
  port: 22,
  proxyJump: '',
  identityFile: '',
  passwordRef: '',
  tags: [],
})

/** Name of the automatic group holding jump hosts. */
export const JUMP_GROUP = 'Jump hosts'

class HostStore {
  list = $state<Host[]>([])
  loaded = $state(false)
  error = $state<string | null>(null)

  /** Hosts other hosts connect through (named in their ProxyJump). */
  gateways = $derived.by(() => {
    const names = new Set<string>()
    for (const h of this.list)
      for (const j of (h.proxyJump ?? '').split(',')) {
        // "user@bastion:2222" → "bastion"
        const n = j.trim().replace(/^[^@]*@/, '').replace(/:\d+$/, '')
        if (n) names.add(n.toLowerCase())
      }
    return new Set(this.list.filter((h) => [h.name, h.alias, h.hostname].some((n) => n && names.has(n.toLowerCase()))).map((h) => h.id))
  })

  groups = $derived.by<HostGroup[]>(() => {
    const byName = new Map<string, Host[]>()
    // Jump hosts come first, in their own group, as in the design.
    const jump = this.list.filter((h) => this.gateways.has(h.id))
    if (jump.length) byName.set(JUMP_GROUP, jump)
    for (const h of this.list) {
      if (this.gateways.has(h.id)) continue
      const key = h.group || 'Ungrouped'
      if (!byName.has(key)) byName.set(key, [])
      byName.get(key)!.push(h)
    }
    return [...byName].map(([name, hosts]) => ({ name, hosts }))
  })

  groupNames = $derived([...new Set(this.list.map((h) => h.group).filter(Boolean))])

  async load() {
    try {
      this.list = (await HostService.List()) ?? []
      this.error = null
    } catch (e) {
      this.error = errorMessage(e)
    } finally {
      this.loaded = true
    }
  }

  /** Saves a host; throws a SaveError on failure. */
  async save(host: Host): Promise<Host> {
    try {
      const saved = await HostService.Save(host)
      await this.load()
      return saved
    } catch (e) {
      throw toSaveError(e)
    }
  }

  /** Saves the login user for a host. */
  async setUser(id: number, user: string) {
    await HostService.SetUser(id, user)
    await this.load()
  }

  async remove(id: number) {
    await HostService.Delete(id)
    await this.load()
  }

  /** Deletes several hosts at once; returns how many were removed. */
  async removeMany(ids: number[]): Promise<number> {
    const n = await HostService.DeleteMany(ids)
    await this.load()
    return n
  }

  get(id: number) {
    return this.list.find((h) => h.id === id)
  }
}

export const hostStore = new HostStore()

/** Wails rejects with { message, cause } where cause is the marshalled Go error. */
function toSaveError(e: unknown): SaveError {
  const cause = (e as { cause?: { field?: string; message?: string } })?.cause
  if (cause?.field && cause.message) return { field: cause.field, message: cause.message }
  return { message: errorMessage(e) }
}

export function errorMessage(e: unknown): string {
  if (e && typeof e === 'object' && 'message' in e) return String((e as { message: unknown }).message)
  return String(e)
}

/** "user@hostname:port" for display. */
export function describe(h: Host): string {
  const host = h.hostname || h.alias
  const user = h.user ? `${h.user}@` : ''
  const port = h.port && h.port !== 22 ? `:${h.port}` : ''
  return `${user}${host}${port}`
}

export function initials(name: string): string {
  // IPv4 names all start alike ("10.50…"), so show the last octet instead.
  const ip = /^\d{1,3}(?:\.\d{1,3}){3}$/.exec(name)
  if (ip) return name.slice(name.lastIndexOf('.') + 1)
  // The first word names the service ("pg-primary" → PG, "haproxy-01" → HA).
  const first = name.split(/[-_.\s]+/).find(Boolean) ?? name
  return first.slice(0, 2).toUpperCase()
}

/** Badge tints; a host keeps its colour as long as its initials don't change. */
const tints = ['violet', 'amber', 'teal', 'lime', 'blue', 'rose'] as const
export type Tint = (typeof tints)[number]

export function tint(name: string): Tint {
  const key = initials(name)
  let h = 0
  for (const c of key) h = (h * 31 + c.charCodeAt(0)) >>> 0
  return tints[h % tints.length]
}
