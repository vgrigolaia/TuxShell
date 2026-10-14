import * as SettingsService from '../../bindings/github.com/vgrigolaia/tuxshell/internal/settings/service'
import type { NamedProfile, Palette, Profile, Settings, Terminal } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/settings/models'
import { theme, type ThemeMode } from './theme.svelte'

export type { NamedProfile, Palette, Profile, Settings, Terminal }

/** The default profile's ID (its settings are Settings.terminal). */
export const DEFAULT_PROFILE = 'default'

/** Mirrors the Go limits; the backend clamps too. */
export const limits = {
  fontSize: [6, 72],
  lineHeight: [1, 2],
  letterSpacing: [-2, 10],
  scrollback: [0, 200000],
  scrollSpeed: [1, 10],
  command: 1000,
} as const

/** Mirrors settings.DefaultPalette. */
export const defaultPalette = (): Palette => ({
  background: '#111318',
  foreground: '#d3d7cf',
  cursor: '#8b7cff',
  selection: '#3b3f4c',
  colors: ['#2e3436', '#cc0000', '#4e9a06', '#c4a000', '#3465a4', '#75507b', '#06989a', '#d3d7cf', '#555753', '#ef2929', '#8ae234', '#fce94f', '#729fcf', '#ad7fa8', '#34e2e2', '#eeeeec'],
})

/** Mirrors settings.DefaultProfile. */
export const defaultProfile = (): Profile => ({
  fontFamily: '',
  fontSize: 13,
  lineHeight: 1.25,
  letterSpacing: 0,
  fontWeight: 400,
  fontWeightBold: 700,
  cursorStyle: 'block',
  cursorBlink: true,
  colorScheme: 'auto',
  palette: defaultPalette(),
  background: '',
  backgroundShade: 70,
  headerColor: '',
  scrollback: 10000,
  scrollbar: true,
  scrollOnOutput: false,
  scrollOnKeypress: true,
  scrollSpeed: 1,
  bell: 'off',
  bellIcon: false,
  bellSound: false,
  command: '',
  keepShell: true,
  backspace: 'del',
  delete: 'escape',
})

export const BUNDLED_FONT = 'JetBrains Mono'
const BUNDLED_CSS = '"JetBrains Mono Variable"'

const defaults = (): Settings => ({
  theme: 'dark',
  hostsSidebar: true,
  hostStatus: true,
  updateCheck: false,
  savePasswords: false,
  profiles: [],
  terminal: {
    ...defaultProfile(),
    confirmClose: true,
    copyOnSelect: false,
    rightClickPaste: false,
    ctrlWheelZoom: true,
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

  readonly fontSize = $derived(this.fontSizeOf(this.value.terminal))
  /** CSS font-family list for terminals; always falls back to the bundled font. */
  readonly fontFamily = $derived(this.fontFamilyOf(this.value.terminal))

  /** A profile's font size with the zoom applied. */
  fontSizeOf(p: Profile): number {
    return clamp(p.fontSize + this.zoom, limits.fontSize[0], limits.fontSize[1])
  }

  fontFamilyOf(p: Profile): string {
    return p.fontFamily && p.fontFamily !== BUNDLED_FONT ? `"${p.fontFamily}", ${BUNDLED_CSS}, monospace` : `${BUNDLED_CSS}, monospace`
  }

  /** The profile with id; a missing one (e.g. deleted) is the default profile. */
  profile(id: string | undefined): Profile {
    return this.value.profiles?.find((p) => p.id === id)?.profile ?? this.value.terminal
  }

  /** Changes a profile's settings, e.g. updateProfile('default', { fontSize: 14 }). */
  updateProfile(id: string, patch: Partial<Profile>) {
    const named = this.value.profiles?.find((p) => p.id === id)
    Object.assign(named ? named.profile : this.value.terminal, patch)
    this.queueSave()
  }

  /** Adds a profile copied from another one and returns its ID. */
  addProfile(name: string, from: string): string {
    const id = 'p' + Date.now().toString(36) + Math.floor(Math.random() * 1296).toString(36)
    const source = $state.snapshot(this.profile(from)) as Profile
    const copy: Profile = { ...defaultProfile() }
    for (const k of Object.keys(copy) as (keyof Profile)[]) (copy as unknown as Record<string, unknown>)[k] = source[k]
    this.value.profiles = [...(this.value.profiles ?? []), { id, name, profile: copy } as NamedProfile]
    this.queueSave()
    return id
  }

  renameProfile(id: string, name: string) {
    const p = this.value.profiles?.find((p) => p.id === id)
    if (!p) return
    p.name = name
    this.queueSave()
  }

  /** Removes a profile; hosts that used it fall back to the default profile. */
  deleteProfile(id: string) {
    this.value.profiles = (this.value.profiles ?? []).filter((p) => p.id !== id)
    this.queueSave()
  }

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

  setHostStatus(on: boolean) {
    this.value.hostStatus = on
    this.queueSave()
  }

  setSavePasswords(on: boolean) {
    this.value.savePasswords = on
    this.queueSave()
  }

  setUpdateCheck(on: boolean) {
    this.value.updateCheck = on
    this.queueSave()
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
