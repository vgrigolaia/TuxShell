/**
 * Split layouts as a tree: each split has its own direction and size, so any
 * pane can be split right or down and every divider moves on its own.
 * Geometry is in percentages of the terminal area.
 */

/** row: side by side (split right); column: stacked (split down). */
export type Dir = 'row' | 'column'
/** How merged tabs are arranged: a row, a column, or a grid (3: one wide over two; 4: 2×2). */
export type Orientation = Dir | 'grid'

/** A pane (leaf) or a split of two parts; ratio is the first part's share. */
export type Node = { pane: number } | { dir: Dir; ratio: number; a: Node; b: Node }

export interface Rect {
  left: number
  top: number
  width: number
  height: number
}

/** Most panes one tab can hold. */
export const MAX_PANES = 4
/** Smallest share either side of a split can be resized to. */
export const MIN_SHARE = 0.1

const isPane = (n: Node): n is { pane: number } => 'pane' in n
const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))
const ratioOf = (r: number) => (Number.isFinite(r) ? clamp(r, MIN_SHARE, 1 - MIN_SHARE) : 0.5)

export const single = (pane: number): Node => ({ pane })

/** Pane ids in reading order (left to right, top to bottom). */
export function leaves(n: Node): number[] {
  return isPane(n) ? [n.pane] : [...leaves(n.a), ...leaves(n.b)]
}

/** Replaces pane target with a split of target and pane (after it: right or below). */
export function splitPane(n: Node, target: number, pane: number, dir: Dir): Node {
  if (isPane(n)) return n.pane === target ? { dir, ratio: 0.5, a: n, b: { pane } } : n
  return { ...n, a: splitPane(n.a, target, pane, dir), b: splitPane(n.b, target, pane, dir) }
}

/** The tree without pane id: its sibling takes the split's place. null when it was the only pane. */
export function removePane(n: Node, id: number): Node | null {
  if (isPane(n)) return n.pane === id ? null : n
  const a = removePane(n.a, id)
  const b = removePane(n.b, id)
  if (!a) return b
  if (!b) return a
  return { ...n, a, b }
}

/** Swaps two panes' places. */
export function swapPanes(n: Node, x: number, y: number): Node {
  if (isPane(n)) return n.pane === x ? { pane: y } : n.pane === y ? { pane: x } : n
  return { ...n, a: swapPanes(n.a, x, y), b: swapPanes(n.b, x, y) }
}

/** Panes in a row or column with even shares. */
export function chain(ids: number[], dir: Dir): Node {
  if (ids.length === 1) return { pane: ids[0] }
  // The first pane gets 1/n; the rest share what's left the same way.
  return { dir, ratio: 1 / ids.length, a: { pane: ids[0] }, b: chain(ids.slice(1), dir) }
}

/** The tree for merged tabs or an old saved layout (orientation + shares). */
export function fromOrientation(ids: number[], orientation: Orientation, sizes: number[] = []): Node {
  if (ids.length === 0) throw new Error('no panes')
  if (ids.length === 1) return { pane: ids[0] }
  const c = ratioOf(sizes[0] ?? 0.5)
  const r = ratioOf(sizes[1] ?? 0.5)
  if (ids.length === 3 && orientation === 'grid')
    return { dir: 'column', ratio: r, a: { pane: ids[0] }, b: { dir: 'row', ratio: c, a: { pane: ids[1] }, b: { pane: ids[2] } } }
  if (ids.length === 4 && (orientation === 'grid' || sizes.length === 2))
    return {
      dir: 'column',
      ratio: r,
      a: { dir: 'row', ratio: c, a: { pane: ids[0] }, b: { pane: ids[1] } },
      b: { dir: 'row', ratio: c, a: { pane: ids[2] }, b: { pane: ids[3] } },
    }
  const dir: Dir = orientation === 'column' ? 'column' : 'row'
  if (sizes.length !== ids.length) return chain(ids, dir)
  // Keep the saved shares: each split's ratio is its first pane's share of what's left.
  const build = (i: number, rest: number): Node =>
    i === ids.length - 1 ? { pane: ids[i] } : { dir, ratio: ratioOf(sizes[i] / rest), a: { pane: ids[i] }, b: build(i + 1, rest - sizes[i]) }
  return build(0, sizes.reduce((x, y) => x + y, 0) || 1)
}

/** Each pane's rectangle. */
export function paneRects(n: Node, area: Rect = { left: 0, top: 0, width: 100, height: 100 }, out = new Map<number, Rect>()): Map<number, Rect> {
  if (isPane(n)) return out.set(n.pane, area)
  const [a, b] = parts(n, area)
  paneRects(n.a, a, out)
  return paneRects(n.b, b, out)
}

function parts(n: { dir: Dir; ratio: number }, r: Rect): [Rect, Rect] {
  if (n.dir === 'row') {
    const w = r.width * n.ratio
    return [{ ...r, width: w }, { ...r, left: r.left + w, width: r.width - w }]
  }
  const h = r.height * n.ratio
  return [{ ...r, height: h }, { ...r, top: r.top + h, height: r.height - h }]
}

export interface Divider {
  /** Which split it moves: 'a'/'b' steps from the root ('' is the root). */
  path: string
  /** 'x' for a vertical bar (moves left/right), 'y' for a horizontal one. */
  axis: 'x' | 'y'
  /** Position along the axis, in percent of the area. */
  at: number
  /** Where the bar starts and how long it is along the other axis, in percent. */
  from: number
  length: number
  /** The split's area along the axis: start and size, in percent. */
  start: number
  size: number
}

/** One draggable divider per split, only as long as its split. */
export function dividers(n: Node, area: Rect = { left: 0, top: 0, width: 100, height: 100 }, path = '', out: Divider[] = []): Divider[] {
  if (isPane(n)) return out
  const [a, b] = parts(n, area)
  out.push(
    n.dir === 'row'
      ? { path, axis: 'x', at: b.left, from: area.top, length: area.height, start: area.left, size: area.width }
      : { path, axis: 'y', at: b.top, from: area.left, length: area.width, start: area.top, size: area.height },
  )
  dividers(n.a, a, path + 'a', out)
  return dividers(n.b, b, path + 'b', out)
}

/** The tree with divider d dragged to pos (0–1 of the whole area along its axis). */
export function drag(n: Node, d: Divider, pos: number): Node {
  return setRatio(n, d.path, (pos * 100 - d.start) / d.size)
}

function setRatio(n: Node, path: string, ratio: number): Node {
  if (isPane(n)) return n
  if (path === '') return { ...n, ratio: ratioOf(ratio) }
  return path[0] === 'a' ? { ...n, a: setRatio(n.a, path.slice(1), ratio) } : { ...n, b: setRatio(n.b, path.slice(1), ratio) }
}

/** A tree read from storage, or null when it isn't a valid split of exactly these panes. */
export function checkTree(n: unknown, panes: number[]): Node | null {
  const seen: number[] = []
  const walk = (x: unknown, depth: number): Node | null => {
    if (!x || typeof x !== 'object' || depth > MAX_PANES) return null
    const o = x as Record<string, unknown>
    if (typeof o.pane === 'number') {
      if (!panes.includes(o.pane) || seen.includes(o.pane)) return null
      seen.push(o.pane)
      return { pane: o.pane }
    }
    if (o.dir !== 'row' && o.dir !== 'column') return null
    const a = walk(o.a, depth + 1)
    const b = a && walk(o.b, depth + 1)
    return a && b ? { dir: o.dir, ratio: ratioOf(Number(o.ratio)), a, b } : null
  }
  const tree = walk(n, 0)
  return tree && seen.length === panes.length ? tree : null
}

/** The tree with pane ids replaced through map (for saving and reopening layouts). */
export function mapPanes(n: Node, map: (id: number) => number): Node {
  return isPane(n) ? { pane: map(n.pane) } : { ...n, a: mapPanes(n.a, map), b: mapPanes(n.b, map) }
}

export type Direction = 'left' | 'right' | 'up' | 'down'

/** Index of the nearest pane in a direction from pane `from`, or -1. */
export function neighbour(all: Rect[], from: number, dir: Direction): number {
  const a = all[from]
  const ac = { x: a.left + a.width / 2, y: a.top + a.height / 2 }
  let best = -1
  let bestDist = Infinity
  all.forEach((b, i) => {
    if (i === from) return
    const bc = { x: b.left + b.width / 2, y: b.top + b.height / 2 }
    const dx = bc.x - ac.x
    const dy = bc.y - ac.y
    const ok = dir === 'left' ? dx < -1 : dir === 'right' ? dx > 1 : dir === 'up' ? dy < -1 : dy > 1
    if (!ok) return
    // Prefer panes straight along the axis over diagonal ones.
    const dist = dir === 'left' || dir === 'right' ? Math.abs(dx) + 2 * Math.abs(dy) : Math.abs(dy) + 2 * Math.abs(dx)
    if (dist < bestDist) {
      bestDist = dist
      best = i
    }
  })
  return best
}
