import { MAX_PANES, defaultSizes, neighbour, rects, type Direction, type Orientation } from './layout'
import type { Source } from './tabs'

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
}

/** A workspace tab showing 1–4 panes. */
export interface Tab {
  key: number
  paneIds: number[]
  orientation: Orientation
  sizes: number[]
  activePane: number
}

class Workspace {
  panes = $state<Pane[]>([])
  tabs = $state<Tab[]>([])
  activeKey = $state(0)

  private nextPane = 1
  private nextTab = 1

  activeTab = $derived(this.tabs.find((t) => t.key === this.activeKey))
  activePane = $derived(this.activeTab ? this.pane(this.activeTab.activePane) : undefined)

  constructor() {
    this.openTab({ kind: 'shell' }, 'Local shell')
  }

  pane(id: number) {
    return this.panes.find((p) => p.id === id)
  }

  tabOf(paneId: number) {
    return this.tabs.find((t) => t.paneIds.includes(paneId))
  }

  /** Tab label: the pane title, or "a + b" for splits. */
  tabTitle(tab: Tab): string {
    const titles = tab.paneIds.map((id) => this.pane(id)?.title ?? '')
    return titles.length <= 2 ? titles.join(' + ') : `${titles[0]} +${titles.length - 1}`
  }

  private makePane(source: Source, title: string): Pane {
    const pane: Pane = { id: this.nextPane++, title, termTitle: '', source, exited: false, size: '' }
    this.panes.push(pane)
    return pane
  }

  private makeTab(paneIds: number[], orientation: Orientation = 'row'): Tab {
    return { key: this.nextTab++, paneIds, orientation, sizes: defaultSizes(paneIds.length), activePane: paneIds[0] }
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
    tab.sizes = defaultSizes(tab.paneIds.length)
    tab.activePane = pane.id
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
    target.sizes = defaultSizes(ids.length)
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
    tab.sizes = defaultSizes(tab.paneIds.length)
    if (tab.activePane === paneId) tab.activePane = tab.paneIds[Math.min(i, tab.paneIds.length - 1)]
  }

  /** Closes tabs and their panes, keeping at least one tab open. */
  closeTabs(keys: number[]) {
    const drop = new Set(keys)
    const before = [...this.tabs]
    const activeIndex = before.findIndex((t) => t.key === this.activeKey)
    const goneIds = new Set(before.filter((t) => drop.has(t.key)).flatMap((t) => t.paneIds))
    this.tabs = before.filter((t) => !drop.has(t.key))
    this.panes = this.panes.filter((p) => !goneIds.has(p.id))
    if (this.tabs.length === 0) return this.openTab({ kind: 'shell' }, 'Local shell')
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
