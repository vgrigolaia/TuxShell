/** Split layouts: geometry in percentages of the terminal area. */

export type Orientation = 'row' | 'column'

export interface Rect {
  left: number
  top: number
  width: number
  height: number
}

/** Most panes one tab can hold (a 2×2 grid). */
export const MAX_PANES = 4
/** Smallest share a pane can be resized to. */
export const MIN_SHARE = 0.15

/** Even shares for n panes along one axis. */
export const evenSizes = (n: number): number[] => Array.from({ length: n }, () => 1 / n)

/**
 * Pane rectangles for n panes. 1 fills the area; 2–3 line up along the
 * orientation using sizes; 4 form a grid where sizes = [column split, row split].
 */
export function rects(n: number, orientation: Orientation, sizes: number[]): Rect[] {
  if (n <= 1) return [{ left: 0, top: 0, width: 100, height: 100 }]
  if (n === 4) {
    const c = (sizes[0] ?? 0.5) * 100
    const r = (sizes[1] ?? 0.5) * 100
    return [
      { left: 0, top: 0, width: c, height: r },
      { left: c, top: 0, width: 100 - c, height: r },
      { left: 0, top: r, width: c, height: 100 - r },
      { left: c, top: r, width: 100 - c, height: 100 - r },
    ]
  }
  const out: Rect[] = []
  let at = 0
  for (let i = 0; i < n; i++) {
    const share = (sizes[i] ?? 1 / n) * 100
    out.push(orientation === 'row' ? { left: at, top: 0, width: share, height: 100 } : { left: 0, top: at, width: 100, height: share })
    at += share
  }
  return out
}

/** Default sizes for n panes. */
export function defaultSizes(n: number): number[] {
  return n === 4 ? [0.5, 0.5] : evenSizes(n)
}

export interface Divider {
  /** Axis the divider moves along: 'x' for a vertical bar, 'y' for a horizontal one. */
  axis: 'x' | 'y'
  /** Position in percent along that axis. */
  at: number
  /** Index into sizes that dragging this divider changes. */
  index: number
}

/** Draggable dividers for a layout. */
export function dividers(n: number, orientation: Orientation, sizes: number[]): Divider[] {
  if (n <= 1) return []
  if (n === 4) return [
    { axis: 'x', at: (sizes[0] ?? 0.5) * 100, index: 0 },
    { axis: 'y', at: (sizes[1] ?? 0.5) * 100, index: 1 },
  ]
  const out: Divider[] = []
  let at = 0
  for (let i = 0; i < n - 1; i++) {
    at += (sizes[i] ?? 1 / n) * 100
    out.push({ axis: orientation === 'row' ? 'x' : 'y', at, index: i })
  }
  return out
}

/**
 * New sizes after dragging divider d to position pos (0–1 along its axis),
 * keeping every pane at least MIN_SHARE.
 */
export function drag(n: number, sizes: number[], d: Divider, pos: number): number[] {
  const next = [...sizes]
  if (n === 4) {
    next[d.index] = clamp(pos, MIN_SHARE, 1 - MIN_SHARE)
    return next
  }
  // Only the two panes either side of the divider change.
  const before = sizes.slice(0, d.index).reduce((a, b) => a + b, 0)
  const pair = sizes[d.index] + sizes[d.index + 1]
  const left = clamp(pos - before, MIN_SHARE, pair - MIN_SHARE)
  next[d.index] = left
  next[d.index + 1] = pair - left
  return next
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

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))
