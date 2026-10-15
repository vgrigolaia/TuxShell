import { SvelteSet } from 'svelte/reactivity'
import type { MenuItem } from './components/ContextMenu.svelte'
import type { TabInfo } from './tabs'
import { MAX_PANES, type Orientation } from './layout'

/** What the tab views (title bar strip, left panel list, icon rail) can do; App implements it. */
export interface TabActions {
  select: (key: number) => void
  close: (key: number) => void
  closeMany: (keys: number[]) => void
  merge: (keys: number[], orientation: Orientation) => void
  split: (key: number, orientation: Orientation) => void
  unsplit: (key: number) => void
  rename: (key: number) => void
  /** Moves a tab to a new position in the list. */
  move: (key: number, index: number) => void
  newShell: () => void
  home: () => void
}

/** Ctrl+click picks several tabs for bulk actions; shared by every tab view. */
class TabPicker {
  readonly picked = new SvelteSet<number>()

  /** A plain click selects the tab; Ctrl+click adds it to (or removes it from) the picked set. */
  click(e: MouseEvent, key: number, activeKey: number, select: (key: number) => void) {
    if (e.ctrlKey || e.metaKey) {
      if (this.picked.has(key)) this.picked.delete(key)
      else this.picked.add(key)
      // Ctrl+clicking starts a selection that includes the current tab.
      if (this.picked.size && !this.picked.has(activeKey)) this.picked.add(activeKey)
      return
    }
    this.picked.clear()
    select(key)
  }

  /** Forgets tabs that were closed. */
  prune(tabs: TabInfo[]) {
    const keys = new Set(tabs.map((t) => t.key))
    for (const k of this.picked) if (!keys.has(k)) this.picked.delete(k)
  }
}

export const tabPicker = new TabPicker()

/** The right-click menu for tab key. */
export function tabMenu(tabs: TabInfo[], key: number, a: TabActions): MenuItem[] {
  const picked = tabPicker.picked
  const i = tabs.findIndex((t) => t.key === key)
  const tab = tabs[i]
  if (!tab) return []
  const others = tabs.filter((t) => t.key !== key).map((t) => t.key)
  const after = tabs.slice(i + 1).map((t) => t.key)
  const items: MenuItem[] = []
  if (picked.size > 1) {
    const sel = [...picked]
    const total = tabs.filter((t) => picked.has(t.key)).reduce((n, t) => n + t.panes, 0)
    if (total <= MAX_PANES) {
      items.push({ label: `Split selected side by side (${sel.length})`, run: () => (a.merge(sel, 'row'), picked.clear()) })
      if (total < 4) items.push({ label: `Split selected stacked (${sel.length})`, run: () => (a.merge(sel, 'column'), picked.clear()) })
    }
    items.push({ label: `Close selected (${sel.length})`, run: () => (a.closeMany(sel), picked.clear()) })
  }
  if (tab.panes < MAX_PANES) {
    items.push({ label: 'Split right…', run: () => a.split(key, 'row') })
    items.push({ label: 'Split down…', run: () => a.split(key, 'column') })
  }
  if (tab.panes > 1) items.push({ label: 'Unsplit into tabs', run: () => a.unsplit(key) })
  items.push({ label: 'Rename tab…', separator: true, run: () => a.rename(key) })
  items.push({ label: 'Close', run: () => a.close(key) })
  if (others.length) items.push({ label: 'Close others', run: () => a.closeMany(others) })
  if (after.length) items.push({ label: 'Close tabs after this one', run: () => a.closeMany(after) })
  items.push({ label: `Close all (${tabs.length})`, run: () => a.closeMany(tabs.map((t) => t.key)), danger: true })
  return items
}
