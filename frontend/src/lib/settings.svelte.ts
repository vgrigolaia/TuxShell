import * as SettingsService from '../../bindings/github.com/vgrigolaia/tuxshell/internal/settings/service'
import type { Settings, Terminal } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/settings/models'
import { theme, type ThemeMode } from './theme.svelte'

export type { Settings, Terminal }

/** Mirrors the Go limits; the backend clamps too. */
export const limits = {
  fontSize: [6, 72],
  lineHeight: [1, 2],
  letterSpacing: [-2, 10],
  scrollback: [0, 200000],
} as const

export const BUNDLED_FONT = 'JetBrains Mono'
const BUNDLED_CSS = '"JetBrains Mono Variable"'

const defaults = (): Settings => ({
  theme: 'dark',
  hostsSidebar: true,
  terminal: {
    fontFamily: '',
    fontSize: 13,
    lineHeight: 1.25,
    letterSpacing: 0,
    fontWeight: 400,
    fontWeightBold: 700,
    cursorStyle: 'block',
    cursorBlink: true,
    colorScheme: 'auto',
    scrollback: 10000,
    bell: 'off',
    confirmClose: true,
    copyOnSelect: false,
    rightClickPaste: false,
    shell: '',
  },
})

/** Older builds kept these in browser storage; moved into settings.json on first run. */
function legacyHostsOpen(): boolean | null {
  try {
    const v = localStorage.getItem('tuxshell.hostsOpen')
    return v === null ? null : v === '1'
  } catch {
    return null
  }
}

/**
 * User preferences, stored by the Go settings service in
 * ~/.config/tuxshell/settings.json. Edits apply at once and are saved shortly after.
 */
class Preferences {
  value = $state<Settings>(defaults())
  loaded = $state(false)
  /** The last save error (e.g. a shell not in /etc/shells), cleared on the next good save. */
  error = $state<{ field?: string; message: string } | null>(null)
  /** Temporary font zoom (Ctrl+= / Ctrl+-), not saved. */
  zoom = $state(0)

  readonly fontSize = $derived(clamp(this.value.terminal.fontSize + this.zoom, limits.fontSize[0], limits.fontSize[1]))
  /** CSS font-family list for terminals; always falls back to the bundled font. */
  readonly fontFamily = $derived(
    this.value.terminal.fontFamily && this.value.terminal.fontFamily !== BUNDLED_FONT ? `"${this.value.terminal.fontFamily}", ${BUNDLED_CSS}, monospace` : `${BUNDLED_CSS}, monospace`,
  )

  private timer: ReturnType<typeof setTimeout> | undefined
  private edits = 0

  async load() {
    try {
      const { settings, saved } = await SettingsService.Get()
      if (saved) {
        this.value = settings
        theme.set(settings.theme as ThemeMode)
      } else {
        // First run with settings.json: keep the choices made before it existed.
        this.value = settings
        this.value.theme = theme.mode
        this.value.hostsSidebar = legacyHostsOpen() ?? settings.hostsSidebar
        this.queueSave()
      }
    } catch (e) {
      console.error('load settings', e)
    } finally {
      this.loaded = true
    }
  }

  /** Changes terminal settings, e.g. update({ fontSize: 14 }). */
  update(patch: Partial<Terminal>) {
    Object.assign(this.value.terminal, patch)
    this.queueSave()
  }

  setTheme(mode: ThemeMode) {
    theme.set(mode)
    this.value.theme = mode
    this.queueSave()
  }

  cycleTheme() {
    this.setTheme(theme.mode === 'dark' ? 'light' : theme.mode === 'light' ? 'system' : 'dark')
  }

  setHostsSidebar(open: boolean) {
    this.value.hostsSidebar = open
    this.queueSave()
  }

  zoomBy(step: number) {
    const t = this.value.terminal.fontSize
    this.zoom = clamp(t + this.zoom + step, limits.fontSize[0], limits.fontSize[1]) - t
  }

  async reset() {
    clearTimeout(this.timer)
    this.zoom = 0
    try {
      this.value = await SettingsService.Reset()
      theme.set(this.value.theme as ThemeMode)
      this.error = null
    } catch (e) {
      this.error = asError(e)
    }
  }

  private queueSave() {
    this.edits++
    clearTimeout(this.timer)
    this.timer = setTimeout(() => void this.flush(), 300)
  }

  private async flush() {
    const at = this.edits
    try {
      const stored = await SettingsService.Save($state.snapshot(this.value) as Settings)
      this.error = null
      // Show clamped values, unless the user kept editing meanwhile.
      if (at === this.edits) this.value = stored
    } catch (e) {
      this.error = asError(e)
    }
  }
}

function clamp(v: number, lo: number, hi: number) {
  return Math.min(Math.max(v, lo), hi)
}

function asError(e: unknown): { field?: string; message: string } {
  const cause = (e as { cause?: { field?: string; message?: string } })?.cause
  if (cause?.message) return { field: cause.field, message: cause.message }
  return { message: String((e as Error)?.message ?? e) }
}

export const prefs = new Preferences()

/** Installed monospace fonts, fetched once when Preferences first opens. */
let fontList: Promise<string[]> | undefined
export function systemFonts(): Promise<string[]> {
  fontList ??= SettingsService.Fonts().then(
    (l) => l ?? [],
    () => [],
  )
  return fontList
}

export async function loginShells(): Promise<string[]> {
  return (await SettingsService.Shells().catch(() => null)) ?? []
}
