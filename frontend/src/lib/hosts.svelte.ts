import * as HostService from '../../bindings/github.com/vgrigolaia/tuxshell/internal/hosts/service'
import type { Host } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/hosts/models'
import { prefs } from './settings.svelte'

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
  os: '',
  passwordSaved: false,
  profile: '',
  protocol: 'ssh',
  rdp: defaultRDP(),
  rdpCert: '',
})

/** Mirrors hosts.DefaultRDP. */
export const defaultRDP = (): Host['rdp'] => ({
  domain: '',
  window: 'window',
  size: 'fit',
  width: 1600,
  height: 900,
  multimon: false,
  clipboard: true,
  folders: [],
  homeFolder: false,
  sound: 'local',
  microphone: false,
  quality: 'balanced',
  gateway: { host: '', port: 443, user: '', domain: '' },
})

/** Name of the automatic group holding jump hosts. */
export const JUMP_GROUP = 'Jump hosts'

/** Shown for hosts without a group; not a real group name. */
export const NO_GROUP = 'Ungrouped'

const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase()

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
      const key = h.group || NO_GROUP
      if (!byName.has(key)) byName.set(key, [])
      byName.get(key)!.push(h)
    }
    // Made with "New group" and still waiting for hosts.
    for (const name of prefs.value.emptyGroups ?? []) if (![...byName.keys()].some((k) => same(k, name))) byName.set(name, [])
    // The order the user dragged them into; groups not in it keep their place after those.
    const order = new Map((prefs.value.groupOrder ?? []).map((g, i) => [g, i]))
    return [...byName]
      .map(([name, hosts], i) => ({ name, hosts, rank: order.get(name) ?? order.size + i }))
      .sort((a, b) => a.rank - b.rank)
      .map(({ name, hosts }) => ({ name, hosts }))
  })

  /** Moves group name to position index among the groups and saves the order. */
  moveGroup(name: string, index: number) {
    const names = this.groups.map((g) => g.name).filter((g) => g !== name)
    names.splice(Math.max(0, Math.min(index, names.length)), 0, name)
    prefs.setGroupOrder(names)
  }

  groupNames = $derived([...new Set([...this.list.map((h) => h.group), ...(prefs.value.emptyGroups ?? [])].filter(Boolean))])

  /** The group's name as shown, when one exists (any case); NO_GROUP and the jump hosts group don't count. */
  findGroup(name: string): string | undefined {
    return this.groups.find((g) => g.name !== NO_GROUP && g.name !== JUMP_GROUP && same(g.name, name))?.name
  }

  /** Adds an empty group at the top (after the jump hosts) and returns its name; an existing one is returned as it is. */
  createGroup(name: string): string | undefined {
    name = name.trim()
    if (!name || same(name, NO_GROUP)) return undefined
    const existing = this.findGroup(name)
    if (existing) return existing
    prefs.setEmptyGroups([...(prefs.value.emptyGroups ?? []), name])
    const names = this.groups.map((g) => g.name).filter((g) => g !== name)
    names.splice(names[0] === JUMP_GROUP ? 1 : 0, 0, name)
    prefs.setGroupOrder(names)
    return name
  }

  /** Renames a group; a name that already exists merges the two. Its place in the list stays. */
  async renameGroup(from: string, to: string) {
    to = to.trim()
    if (same(to, NO_GROUP)) to = ''
    to = (to && this.findGroup(to)) || to
    if (!to || to === from) return
    // The order as shown now, so a new name doesn't move the group elsewhere.
    const names = this.groups.map((g) => g.name)
    const merge = names.includes(to)
    await this.regroup(from, to)
    prefs.setGroupOrder(names.flatMap((g) => (g === from ? (merge ? [] : [to]) : [g])))
  }

  /** Deletes a group; its hosts stay, without a group. */
  async deleteGroup(name: string) {
    const names = this.groups.map((g) => g.name).filter((g) => g !== name)
    await this.regroup(name, '')
    prefs.setGroupOrder(names)
  }

  private async regroup(from: string, to: string) {
    const empty = prefs.value.emptyGroups ?? []
    const rest = empty.filter((g) => g !== from)
    // An empty group exists only in the settings; one with hosts is renamed on the hosts.
    const moved = this.list.some((h) => h.group === from)
    if (moved) {
      try {
        await HostService.RenameGroup(from, to)
        this.error = null
      } catch (e) {
        this.error = errorMessage(e)
        return
      }
    }
    prefs.setEmptyGroups(to && !moved && !this.groups.some((g) => g.name === to) ? [...rest, to] : rest)
    if (moved) await this.load()
  }

  async load() {
    try {
      this.list = (await HostService.List()) ?? []
      this.error = null
      // A "New group" that got hosts is an ordinary group now.
      const empty = prefs.value.emptyGroups ?? []
      const kept = empty.filter((g) => !this.list.some((h) => h.group && same(h.group, g)))
      if (kept.length !== empty.length) prefs.setEmptyGroups(kept)
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

  /** Moves a host into a group ("" for none), before host `before` (0: at the end). */
  async moveHost(id: number, group: string, before: number) {
    try {
      await HostService.MoveHost(id, group, before)
      this.error = null
    } catch (e) {
      this.error = errorMessage(e)
    }
    await this.load()
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
