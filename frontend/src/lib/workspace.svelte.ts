import { MAX_PANES, defaultSizes, neighbour, rects, type Direction, type Orientation } from './layout'
import type { Source } from './tabs'
import { paneProfile } from './profiles.svelte'

/** One terminal session. Panes outlive tab changes: moving one never remounts it. */
export interface Pane {
  id: number
  /** Label: host name, or the shell's title for local panes. */
  title: string
  /** Latest title the terminal set (e.g. user@host:~). */
  termTitle: string
  source: Source
  exited: boolean
  size: string
  /** Login typed at connect time, when it differs from the host's saved user. */
  loginUser: string
  /** Title set by the user; terminal title changes no longer replace it. */
  customTitle: boolean
  /** Typing and pasting are blocked. */
  readOnly: boolean
  showScrollbar: boolean
  /** The terminal rang its bell while the pane wasn't being looked at. */
  bell: boolean
  /** When the current session started (ms), or 0 before it connects. */
  startedAt: number
}

/** A workspace tab showing 1–4 panes. */
export interface Tab {
  key: number
  paneIds: number[]
  orientation: Orientation
  sizes: number[]
  activePane: number
  /** Keystrokes typed in an included pane go to every included pane. */
  broadcast: boolean
  /** Panes left out of broadcast. */
  excluded: number[]
  /** A pane temporarily filling the whole tab, or null. */
  maximized: number | null
  /** Name set by the user (or the host group it was opened from); '' means automatic. */
  name: string
}

/** activeKey of the Home screen (the hosts launchpad); real tabs start at 1. */
export const HOME = 0

class Workspace {
  panes = $state<Pane[]>([])
  tabs = $state<Tab[]>([])
  /** The shown tab, or HOME. */
  activeKey = $state(HOME)

  private nextPane = 1
  private nextTab = 1

  activeTab = $derived(this.tabs.find((t) => t.key === this.activeKey))
  activePane = $derived(this.activeTab ? this.pane(this.activeTab.activePane) : undefined)


  pane(id: number) {
    return this.panes.find((p) => p.id === id)
  }

  tabOf(paneId: number) {
    return this.tabs.find((t) => t.paneIds.includes(paneId))
  }

  /** Tab label: the pane title, or "a + b" for splits. */
  tabTitle(tab: Tab): string {
    if (tab.name) return tab.name
    const titles = tab.paneIds.map((id) => this.pane(id)?.title ?? '')
    return titles.length <= 2 ? titles.join(' + ') : `${titles[0]} +${titles.length - 1}`
  }

  private makePane(source: Source, title: string): Pane {
    const pane: Pane = { id: this.nextPane++, title, termTitle: '', source, exited: false, size: '', loginUser: '', customTitle: false, readOnly: false, showScrollbar: paneProfile(source).scrollbar, bell: false, startedAt: 0 }
    this.panes.push(pane)
    return pane
  }

  private makeTab(paneIds: number[], orientation: Orientation = 'row'): Tab {
    return {
      key: this.nextTab++,
      paneIds,
      orientation,
      sizes: defaultSizes(paneIds.length, orientation),
      activePane: paneIds[0],
      broadcast: false,
      excluded: [],
      maximized: null,
      name: '',
    }
  }

  /** Opens source in a new tab after the active one and switches to it. */
  openTab(source: Source, title: string) {
    const pane = this.makePane(source, title)
    const tab = this.makeTab([pane.id])
    const at = this.tabs.findIndex((t) => t.key === this.activeKey)
    this.tabs.splice(at < 0 ? this.tabs.length : at + 1, 0, tab)
    this.activeKey = tab.key
  }

  /** Adds a new pane to a tab, splitting it. Returns false when the tab is full. */
  splitTab(key: number, source: Source, title: string, orientation: Orientation): boolean {
    const tab = this.tabs.find((t) => t.key === key)
    if (!tab || tab.paneIds.length >= MAX_PANES) return false
    const pane = this.makePane(source, title)
    if (tab.paneIds.length === 1) tab.orientation = orientation
    tab.paneIds.push(pane.id)
    tab.sizes = defaultSizes(tab.paneIds.length, tab.orientation)
    tab.activePane = pane.id
    tab.maximized = null
    // A pane added mid-broadcast starts excluded, so it never gets keystrokes by surprise.
    if (tab.broadcast) tab.excluded = [...tab.excluded, pane.id]
    this.activeKey = key
    return true
  }

  /** Merges the given tabs into the first one, in tab-bar order. */
  mergeTabs(keys: number[], orientation: Orientation = 'row'): boolean {
    const chosen = this.tabs.filter((t) => keys.includes(t.key))
    const ids = chosen.flatMap((t) => t.paneIds)
    if (chosen.length < 2 || ids.length > MAX_PANES) return false
    const [target, ...rest] = chosen
    target.paneIds = ids
    target.orientation = orientation
    target.sizes = defaultSizes(ids.length, orientation)
    target.broadcast = false
    target.excluded = []
    target.maximized = null
    this.tabs = this.tabs.filter((t) => !rest.includes(t))
    this.activeKey = target.key
    return true
  }

  /** Moves a pane out of a split into its own tab, right after the split. */
  detachPane(paneId: number) {
    const tab = this.tabOf(paneId)
    if (!tab || tab.paneIds.length === 1) return
    this.removeFromTab(tab, paneId)
    const own = this.makeTab([paneId])
    this.tabs.splice(this.tabs.indexOf(tab) + 1, 0, own)
    this.activeKey = own.key
  }

  /** Splits every pane of a tab into its own tab. */
  unsplit(key: number) {
    const tab = this.tabs.find((t) => t.key === key)
    if (!tab || tab.paneIds.length === 1) return
    const [first, ...rest] = tab.paneIds
    tab.paneIds = [first]
    tab.sizes = defaultSizes(1)
    tab.activePane = first
    tab.broadcast = false
    tab.excluded = []
    this.tabs.splice(this.tabs.indexOf(tab) + 1, 0, ...rest.map((id) => this.makeTab([id])))
  }

  /** Closes one pane; closing the last pane of a tab closes the tab. */
  closePane(paneId: number) {
    const tab = this.tabOf(paneId)
    if (!tab) return
    if (tab.paneIds.length === 1) return this.closeTabs([tab.key])
    this.removeFromTab(tab, paneId)
    this.panes = this.panes.filter((p) => p.id !== paneId)
  }

  private removeFromTab(tab: Tab, paneId: number) {
    const i = tab.paneIds.indexOf(paneId)
    tab.paneIds.splice(i, 1)
    tab.sizes = defaultSizes(tab.paneIds.length, tab.orientation)
    tab.excluded = tab.excluded.filter((id) => id !== paneId)
    // Broadcasting to a single pane is just typing.
    if (tab.paneIds.length < 2) tab.broadcast = false
    if (tab.activePane === paneId) tab.activePane = tab.paneIds[Math.min(i, tab.paneIds.length - 1)]
    if (tab.maximized === paneId || tab.paneIds.length < 2) tab.maximized = null
  }

  /** Closes tabs and their panes, keeping at least one tab open. */
  closeTabs(keys: number[]) {
    const drop = new Set(keys)
    const before = [...this.tabs]
    const activeIndex = before.findIndex((t) => t.key === this.activeKey)
    const goneIds = new Set(before.filter((t) => drop.has(t.key)).flatMap((t) => t.paneIds))
    this.tabs = before.filter((t) => !drop.has(t.key))
    this.panes = this.panes.filter((p) => !goneIds.has(p.id))
    // Closing the last tab goes back to Home.
    if (this.tabs.length === 0) return void (this.activeKey = HOME)
    if (drop.has(this.activeKey)) {
      // Prefer the nearest surviving tab to the left of the old active one.
      const left = before.slice(0, activeIndex).reverse().find((t) => !drop.has(t.key))
      this.activeKey = (left ?? this.tabs[0]).key
    }
  }

  focusPane(paneId: number) {
    const tab = this.tabOf(paneId)
    if (tab) tab.activePane = paneId
  }

  /** Moves focus to the neighbouring pane in the active tab. */
  moveFocus(dir: Direction) {
    const tab = this.activeTab
    if (!tab || tab.paneIds.length < 2) return
    const all = rects(tab.paneIds.length, tab.orientation, tab.sizes)
    const next = neighbour(all, tab.paneIds.indexOf(tab.activePane), dir)
    if (next >= 0) tab.activePane = tab.paneIds[next]
  }

  /** Turns broadcast on (all panes included) or off for a split tab. */
  toggleBroadcast(key: number) {
    const tab = this.tabs.find((t) => t.key === key)
    if (!tab || tab.paneIds.length < 2) return
    tab.broadcast = !tab.broadcast
    tab.excluded = []
  }

  /** Includes or excludes a pane from its tab's broadcast. */
  toggleExcluded(paneId: number) {
    const tab = this.tabOf(paneId)
    if (!tab) return
    tab.excluded = tab.excluded.includes(paneId) ? tab.excluded.filter((id) => id !== paneId) : [...tab.excluded, paneId]
  }

  /** Panes that receive broadcast keystrokes from paneId, or null when it isn't broadcasting. */
  broadcastTargets(paneId: number): number[] | null {
    const tab = this.tabOf(paneId)
    if (!tab?.broadcast || tab.excluded.includes(paneId)) return null
    return tab.paneIds.filter((id) => !tab.excluded.includes(id))
  }

  /** The pane's connection again (same host and login), for splits and new tabs. */
  sameSource(paneId: number): Source | null {
    const p = this.pane(paneId)
    if (!p) return null
    if (p.source.kind !== 'host') return { ...p.source }
    return p.loginUser ? { kind: 'host', hostId: p.source.hostId, user: p.loginUser } : { kind: 'host', hostId: p.source.hostId, askUser: p.source.askUser }
  }

  /** Lets one pane fill its tab, or restores the split. */
  toggleMaximized(paneId: number) {
    const tab = this.tabOf(paneId)
    if (!tab || tab.paneIds.length < 2) return
    tab.maximized = tab.maximized === paneId ? null : paneId
    tab.activePane = paneId
  }

  /** Names a tab; an empty name goes back to the automatic one. */
  renameTab(key: number, name: string) {
    const tab = this.tabs.find((t) => t.key === key)
    if (tab) tab.name = name.trim()
  }

  /** Sets a pane's title; an empty title goes back to the automatic one. */
  renamePane(paneId: number, title: string) {
    const p = this.pane(paneId)
    if (!p) return
    const t = title.trim()
    p.customTitle = !!t
    if (t) p.title = t
    else if (p.source.kind === 'shell') p.title = p.termTitle || 'Local shell'
  }

  updatePane(id: number, patch: Partial<Pane>) {
    const pane = this.pane(id)
    if (pane) Object.assign(pane, patch)
  }

  /** Keeps pane labels in sync when a saved host is renamed. */
  renameHost(hostId: number, name: string) {
    for (const p of this.panes) if (p.source.kind === 'host' && p.source.hostId === hostId) p.title = name
  }
}

export const workspace = new Workspace()
