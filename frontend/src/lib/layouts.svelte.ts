import type { Layout, LayoutPane, LayoutTab } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/settings/models'
import { hostStore } from './hosts.svelte'
import { defaultSizes, type Orientation } from './layout'
import { notices } from './notices.svelte'
import { prefs } from './settings.svelte'
import type { Source } from './tabs'
import { workspace, type Pane } from './workspace.svelte'

export type { Layout }

const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase()

/** What a pane runs, as a layout stores it (nothing typed or shown in it). */
function paneOf(p: Pane): LayoutPane {
  const pane: LayoutPane = { kind: p.source.kind, hostId: 0, user: '', target: '', title: p.customTitle ? p.title : '' }
  const s = p.source
  if (s.kind === 'host') return { ...pane, hostId: s.hostId, user: p.loginUser || s.user || '' }
  if (s.kind === 'rdp') return { ...pane, hostId: s.hostId }
  if (s.kind === 'target') return { ...pane, target: s.target }
  return pane
}

/** The source to open for a stored pane, or null when its host is gone. */
function sourceOf(p: LayoutPane): { source: Source; title: string } | null {
  if (p.kind === 'shell') return { source: { kind: 'shell' }, title: 'Local shell' }
  if (p.kind === 'target' && p.target) return { source: { kind: 'target', target: p.target }, title: p.target }
  const h = hostStore.get(p.hostId)
  if (!h) return null
  if (p.kind === 'rdp' && h.protocol === 'rdp') return { source: { kind: 'rdp', hostId: h.id }, title: h.name }
  if (p.kind === 'host' && h.protocol !== 'rdp')
    return { source: p.user ? { kind: 'host', hostId: h.id, user: p.user } : { kind: 'host', hostId: h.id, askUser: !h.user }, title: h.name }
  return null
}

/** Saved sets of tabs (settings.layouts): save the open ones, open them again in one go. */
class Layouts {
  list = $derived(prefs.value.layouts ?? [])

  find(name: string): Layout | undefined {
    return this.list.find((l) => same(l.name, name.trim()))
  }

  /** The given tabs as they are now, in tab order. */
  snapshot(keys: number[]): LayoutTab[] {
    return workspace.tabs
      .filter((t) => keys.includes(t.key))
      .map((t) => ({
        name: t.name,
        orientation: t.orientation,
        sizes: [...t.sizes],
        panes: t.paneIds.flatMap((id) => {
          const p = workspace.pane(id)
          return p ? [paneOf(p)] : []
        }),
      }))
  }

  /** Saves tabs under name; a layout with that name (any case) is replaced in place. */
  save(name: string, keys: number[]) {
    name = name.trim()
    const tabs = this.snapshot(keys)
    if (!name || !tabs.length) return
    const old = this.find(name)
    const layout: Layout = { name, startup: old?.startup ?? false, tabs }
    prefs.setLayouts(old ? this.list.map((l) => (l === old ? layout : l)) : [...this.list, layout])
  }

  /** Renames a layout; false when another one already has the name. */
  rename(from: string, to: string): boolean {
    to = to.trim()
    if (!to) return true
    if (this.find(to) && !same(from, to)) return false
    prefs.setLayouts(this.list.map((l) => (l.name === from ? { ...l, name: to } : l)))
    return true
  }

  remove(name: string) {
    prefs.setLayouts(this.list.filter((l) => l.name !== name))
  }

  /** Marks one layout to open at startup (on), or none. */
  setStartup(name: string, on: boolean) {
    prefs.setLayouts(this.list.map((l) => ({ ...l, startup: on && l.name === name })))
  }

  /** Opens the layout's tabs after the current one; the first of them is shown. */
  open(layout: Layout) {
    let skipped = 0
    let first: number | null = null
    for (const t of layout.tabs ?? []) {
      const stored = t.panes ?? []
      const panes = stored.flatMap((p) => {
        const s = sourceOf(p)
        if (!s) skipped++
        return s ? [{ ...s, custom: p.title }] : []
      })
      if (!panes.length) continue
      const [head, ...rest] = panes
      workspace.openTab(head.source, head.title)
      const key = workspace.activeKey
      first ??= key
      for (const p of rest) workspace.splitTab(key, p.source, p.title, t.orientation as Orientation)
      const tab = workspace.tabs.find((x) => x.key === key)
      if (!tab) continue
      // The saved split, when nothing was left out and it fits the panes.
      const sizes = t.sizes ?? []
      if (panes.length === stored.length && sizes.length === defaultSizes(tab.paneIds.length, tab.orientation).length) tab.sizes = [...sizes]
      if (t.name) workspace.renameTab(key, t.name)
      panes.forEach((p, i) => p.custom && workspace.renamePane(tab.paneIds[i], p.custom))
    }
    if (first !== null) workspace.activeKey = first
    if (skipped)
      notices.show(`${skipped === 1 ? '1 pane' : `${skipped} panes`} of “${layout.name}” didn’t open: ${skipped === 1 ? 'its host was' : 'their hosts were'} deleted.`)
  }

  /** The layout marked "Open at startup", if any. */
  get startup(): Layout | undefined {
    return this.list.find((l) => l.startup)
  }
}

export const layouts = new Layouts()
