export type ThemeMode = 'dark' | 'light' | 'system'
export type Resolved = 'dark' | 'light'

// A cache of the saved theme, read synchronously so the first paint has the right colours.
const KEY = 'tuxshell.theme'

/** App colour theme. Dark is the default; "system" follows the desktop setting. */
class Theme {
  mode = $state<ThemeMode>('dark')
  private systemLight = $state(false)

  resolved = $derived<Resolved>(this.mode === 'system' ? (this.systemLight ? 'light' : 'dark') : this.mode)

  constructor() {
    try {
      const saved = localStorage.getItem(KEY)
      if (saved === 'dark' || saved === 'light' || saved === 'system') this.mode = saved
    } catch {
      // Storage unavailable: keep the default.
    }
    const mq = window.matchMedia('(prefers-color-scheme: light)')
    this.systemLight = mq.matches
    mq.addEventListener('change', (e) => (this.systemLight = e.matches))
    // Apply before first paint to avoid a flash of the wrong theme.
    document.documentElement.dataset.theme = this.resolved
    $effect.root(() => {
      $effect(() => {
        document.documentElement.dataset.theme = this.resolved
      })
    })
  }

  set(mode: ThemeMode) {
    this.mode = mode
    try {
      localStorage.setItem(KEY, mode)
    } catch {
      // Not persisted this time; the choice still applies now.
    }
  }
}

export const theme = new Theme()

/** xterm.js palettes per theme. */
export const terminalThemes = {
  dark: {
    background: '#0c0d12',
    foreground: '#c7cbd4',
    cursor: '#8b7cff',
    cursorAccent: '#0c0d12',
    selectionBackground: '#8b7cff55',
    black: '#1e212a',
    brightBlack: '#5c6170',
    green: '#a3e635',
    brightGreen: '#bef264',
    yellow: '#fb923c',
    brightYellow: '#fdba74',
    blue: '#7aa2ff',
    brightBlue: '#a5c0ff',
    magenta: '#8b7cff',
    brightMagenta: '#b1a7ff',
  },
  light: {
    background: '#ffffff',
    foreground: '#16181d',
    cursor: '#2f5bff',
    cursorAccent: '#ffffff',
    selectionBackground: '#2f5bff33',
    black: '#16181d',
    brightBlack: '#5e6474',
    white: '#c9ccd4',
    brightWhite: '#9aa0ad',
    red: '#c0262d',
    brightRed: '#dc2f37',
    green: '#3f6212',
    brightGreen: '#4d7c0f',
    yellow: '#a1520b',
    brightYellow: '#b4410c',
    blue: '#1d4ed8',
    brightBlue: '#2563eb',
    magenta: '#5b4be0',
    brightMagenta: '#7262f0',
    cyan: '#0e7490',
    brightCyan: '#0891b2',
  },
}
