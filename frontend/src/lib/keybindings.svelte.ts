import { isMac } from './platform'
import { prefs } from './settings.svelte'

/**
 * App shortcuts the user can change in Preferences → Shortcuts. A combo is
 * written "Ctrl+Alt+Cmd+Shift+Key" (modifiers in that order, as the Go
 * settings check them). Keys are physical keys, so shortcuts work the same
 * with any keyboard layout. Settings keep only the changed ones; "" is off.
 */
export type ActionId =
  | 'quickConnect'
  | 'preferences'
  | 'toggleHosts'
  | 'toggleFiles'
  | 'toggleClipboard'
  | 'broadcast'
  | 'splitAuto'
  | 'splitDown'
  | 'newTab'
  | 'maximize'
  | 'scrollbar'
  | 'rename'
  | 'closePane'
  | 'focusLeft'
  | 'focusRight'
  | 'focusUp'
  | 'focusDown'
  | 'zoomIn'
  | 'zoomOut'
  | 'zoomReset'
  | 'copy'
  | 'paste'

export interface Action {
  id: ActionId
  label: string
  group: 'App' | 'Panes' | 'Terminal'
  /** The default combo; ⌘ replaces Ctrl on macOS. */
  combo: string
  /** Extra defaults that work until the action is changed. */
  also?: string[]
}

// Above ACTIONS: mod() calls canonical() while ACTIONS is built (on macOS), and a
// const used before its line throws, which left the window blank in 1.4.1/1.4.2.
const MODS = ['Ctrl', 'Alt', 'Cmd', 'Shift'] as const

/** Puts a combo's modifiers in the stored order. */
function canonical(combo: string): string {
  const parts = combo.split('+')
  const key = parts.pop() ?? ''
  return [...MODS.filter((m) => parts.includes(m)), key].join('+')
}

/** On macOS the app's shortcuts use ⌘ and Ctrl stays with the terminal. */
const mod = (combo: string) => (isMac ? canonical(combo.replace('Ctrl+', 'Cmd+')) : combo)

export const ACTIONS: Action[] = [
  { id: 'quickConnect', group: 'App', label: 'Quick connect / search hosts and snippets', combo: mod('Ctrl+K') },
  { id: 'preferences', group: 'App', label: 'Preferences', combo: mod('Ctrl+,') },
  { id: 'toggleHosts', group: 'App', label: 'Show or hide the hosts sidebar', combo: mod('Ctrl+Shift+L') },
  { id: 'toggleFiles', group: 'App', label: 'Files panel', combo: mod('Ctrl+Shift+E') },
  { id: 'toggleClipboard', group: 'App', label: 'Clipboard history', combo: mod('Ctrl+Shift+H') },
  { id: 'broadcast', group: 'Panes', label: 'Broadcast typing to split panes', combo: mod('Ctrl+Shift+B') },
  { id: 'splitAuto', group: 'Panes', label: 'Split auto (same connection)', combo: mod('Ctrl+Shift+A') },
  { id: 'splitDown', group: 'Panes', label: 'Split down (same connection)', combo: mod('Ctrl+Shift+O') },
  { id: 'newTab', group: 'Panes', label: 'Open the same connection in a new tab', combo: mod('Ctrl+Shift+T') },
  { id: 'maximize', group: 'Panes', label: 'Maximize pane / restore split', combo: mod('Ctrl+Shift+X') },
  { id: 'scrollbar', group: 'Panes', label: 'Show or hide the scrollbar', combo: mod('Ctrl+Shift+S') },
  { id: 'rename', group: 'Panes', label: 'Set pane title', combo: mod('Ctrl+Alt+W') },
  { id: 'closePane', group: 'Panes', label: 'Close pane', combo: mod('Ctrl+Shift+W') },
  { id: 'focusLeft', group: 'Panes', label: 'Move to the pane on the left', combo: mod('Ctrl+Shift+Left') },
  { id: 'focusRight', group: 'Panes', label: 'Move to the pane on the right', combo: mod('Ctrl+Shift+Right') },
  { id: 'focusUp', group: 'Panes', label: 'Move to the pane above', combo: mod('Ctrl+Shift+Up') },
  { id: 'focusDown', group: 'Panes', label: 'Move to the pane below', combo: mod('Ctrl+Shift+Down') },
  { id: 'zoomIn', group: 'Terminal', label: 'Zoom text in', combo: mod('Ctrl+='), also: [mod('Ctrl+Shift+=')] },
  { id: 'zoomOut', group: 'Terminal', label: 'Zoom text out', combo: mod('Ctrl+-') },
  { id: 'zoomReset', group: 'Terminal', label: 'Reset zoom', combo: mod('Ctrl+0') },
  // Linux terminals copy with Ctrl+Shift+C, since Ctrl+C interrupts.
  { id: 'copy', group: 'Terminal', label: 'Copy selection', combo: isMac ? 'Cmd+C' : 'Ctrl+Shift+C', also: isMac ? [] : ['Ctrl+Insert'] },
  { id: 'paste', group: 'Terminal', label: 'Paste', combo: isMac ? 'Cmd+V' : 'Ctrl+Shift+V', also: isMac ? [] : ['Shift+Insert'] },
]

const byId = new Map(ACTIONS.map((a) => [a.id, a]))

const CODES: Record<string, string> = {
  Comma: ',',
  Period: '.',
  Slash: '/',
  Semicolon: ';',
  Quote: "'",
  BracketLeft: '[',
  BracketRight: ']',
  Backslash: '\\',
  Backquote: '`',
  Minus: '-',
  Equal: '=',
  ArrowUp: 'Up',
  ArrowDown: 'Down',
  ArrowLeft: 'Left',
  ArrowRight: 'Right',
  Insert: 'Insert',
  Delete: 'Delete',
  Home: 'Home',
  End: 'End',
  PageUp: 'PageUp',
  PageDown: 'PageDown',
  Tab: 'Tab',
  Enter: 'Enter',
  NumpadEnter: 'Enter',
  Space: 'Space',
  Backspace: 'Backspace',
}

function keyOf(e: KeyboardEvent): string | null {
  const c = e.code
  if (/^Key[A-Z]$/.test(c)) return c.slice(3)
  if (/^Digit[0-9]$/.test(c)) return c.slice(5)
  if (/^F([1-9]|1[0-9]|2[0-4])$/.test(c)) return c
  if (CODES[c]) return CODES[c]
  // No physical key reported (some input methods): fall back to the character.
  return !c && /^[a-z0-9]$/i.test(e.key) ? e.key.toUpperCase() : null
}

/** The combo a key press makes, or null for a lone modifier or an unknown key. */
export function comboOf(e: KeyboardEvent): string | null {
  const key = keyOf(e)
  if (!key) return null
  const on = {
    Ctrl: e.ctrlKey,
    Alt: e.altKey,
    Cmd: e.metaKey,
    Shift: e.shiftKey,
  }
  return [...MODS.filter((m) => on[m]), key].join('+')
}

const MAC_SYMBOLS: Record<string, string> = {
  Ctrl: '⌃',
  Alt: '⌥',
  Shift: '⇧',
  Cmd: '⌘',
}
const KEY_NAMES: Record<string, string> = {
  Up: '↑',
  Down: '↓',
  Left: '←',
  Right: '→',
  PageUp: 'Page Up',
  PageDown: 'Page Down',
}
const MAC_KEYS: Record<string, string> = {
  Enter: '↩',
  Backspace: '⌫',
  Delete: '⌦',
  Tab: '⇥',
}

/** A combo as people read it: "Ctrl+Shift+L", or "⇧⌘L" on macOS. */
export function formatCombo(combo: string): string {
  if (!combo) return ''
  const parts = combo.split('+')
  const key = parts.pop() ?? ''
  const name = (isMac && MAC_KEYS[key]) || KEY_NAMES[key] || key
  if (isMac)
    return (
      ['Ctrl', 'Alt', 'Shift', 'Cmd']
        .filter((m) => parts.includes(m))
        .map((m) => MAC_SYMBOLS[m])
        .join('') + name
    )
  return [...parts.map((m) => (m === 'Cmd' ? 'Super' : m)), name].join('+')
}

const ARIA_KEYS: Record<string, string> = {
  Up: 'ArrowUp',
  Down: 'ArrowDown',
  Left: 'ArrowLeft',
  Right: 'ArrowRight',
  Space: 'Space',
}

/** A combo in aria-keyshortcuts form ("Control+Shift+L"). */
export function ariaCombo(combo: string): string | undefined {
  if (!combo) return undefined
  return combo
    .split('+')
    .map((p) => ({ Ctrl: 'Control', Cmd: 'Meta' })[p] ?? ARIA_KEYS[p] ?? p)
    .join('+')
}

const LETTER = /^[A-Z0-9]$/

class Keybindings {
  /** True while Preferences waits for a new combo; shortcuts don't run meanwhile. */
  recording = $state(false)

  private custom = $derived(prefs.value.keybindings ?? {})

  /** Every working combo and its action. */
  private table = $derived.by(() => {
    const m = new Map<string, ActionId>()
    for (const a of ACTIONS) for (const c of this.combos(a.id)) if (!m.has(c)) m.set(c, a.id)
    return m
  })

  /** The action's combo, "" when turned off. */
  combo(id: ActionId): string {
    const c = this.custom[id]
    return c ?? byId.get(id)?.combo ?? ''
  }

  /** The combo plus the extra defaults while the action is unchanged. */
  combos(id: ActionId): string[] {
    const main = this.combo(id)
    const also = this.changed(id) ? [] : (byId.get(id)?.also ?? [])
    return [main, ...also].filter(Boolean)
  }

  changed(id: ActionId): boolean {
    return this.custom[id] !== undefined && this.custom[id] !== byId.get(id)?.combo
  }

  get anyChanged(): boolean {
    return ACTIONS.some((a) => this.changed(a.id))
  }

  /** The combo as shown in labels, "" when off. */
  label(id: ActionId): string {
    return formatCombo(this.combo(id))
  }

  /** "Close (Ctrl+Shift+W)", or just "Close" when the shortcut is off. */
  title(text: string, id: ActionId): string {
    const k = this.label(id)
    return k ? `${text} (${k})` : text
  }

  aria(id: ActionId): string | undefined {
    return ariaCombo(this.combo(id))
  }

  /** The action a key press runs, if any. */
  match(e: KeyboardEvent): ActionId | null {
    if (this.recording) return null
    const c = comboOf(e)
    return c ? (this.table.get(c) ?? null) : null
  }

  /** The other action that already has combo, and whether only as an extra default. */
  usedBy(combo: string, except: ActionId): { action: Action; extra: boolean } | null {
    for (const a of ACTIONS) {
      if (a.id === except) continue
      if (this.combo(a.id) === combo) return { action: a, extra: false }
      if (this.combos(a.id).includes(combo)) return { action: a, extra: true }
    }
    return null
  }

  set(id: ActionId, combo: string) {
    prefs.setKeybinding(id, combo === byId.get(id)?.combo ? null : combo)
  }

  /** Gives combo to id; the action that had it is turned off (or loses only its extra default). */
  take(id: ActionId, combo: string) {
    const other = this.usedBy(combo, id)
    if (other) this.set(other.action.id, other.extra ? this.combo(other.action.id) : '')
    this.set(id, combo)
  }

  reset(id: ActionId) {
    prefs.setKeybinding(id, null)
  }

  resetAll() {
    prefs.resetKeybindings()
  }
}

export const keybindings = new Keybindings()

/**
 * Whether combo can be a shortcut. An error refuses it; a warning asks first.
 * Shortcuts are caught before the terminal, so keys the shell needs are kept free.
 */
export function checkCombo(combo: string): {
  error?: string
  warning?: string
} {
  const parts = combo.split('+')
  const key = parts.pop() ?? ''
  const has = (m: string) => parts.includes(m)
  const only = (...mods: string[]) => parts.length === mods.length && mods.every(has)
  const shown = formatCombo(combo)
  if (/^F\d+$/.test(key)) return {}
  if (!has('Ctrl') && !has('Alt') && !has('Cmd'))
    return {
      error: `${shown} alone would stop that key from typing. Add ${isMac ? '⌘, ⌥ or ⌃' : 'Ctrl or Alt'}, or use an F key.`,
    }
  if (isMac && only('Cmd') && ['Q', 'H', 'M', 'W', 'Tab'].includes(key)) return { error: `macOS uses ${shown} itself.` }
  if (!isMac && only('Ctrl') && ['C', 'D', 'Z'].includes(key))
    return {
      error: `The terminal needs ${shown} (${key === 'C' ? 'interrupt' : key === 'D' ? 'end of input' : 'suspend'}).`,
    }
  if (only('Ctrl') && LETTER.test(key))
    return {
      warning: `Shells use ${shown} too; the terminal won't get it any more.`,
    }
  if (only('Alt') && LETTER.test(key))
    return {
      warning: isMac
        ? `${shown} types a special character; you won't be able to type it.`
        : `Shells use ${shown} (e.g. Alt+B, Alt+F move by word); the terminal won't get it any more.`,
    }
  return {}
}
