import * as Backgrounds from '../../bindings/github.com/vgrigolaia/tuxshell/internal/backgrounds/service'
import { hostStore } from './hosts.svelte'
import { prefs, type Profile } from './settings.svelte'
import type { Source } from './tabs'

/** The profile a pane uses: its saved host's profile, otherwise the default one. */
export function paneProfile(source: Source): Profile {
  return prefs.profile(source.kind === 'host' ? hostStore.get(source.hostId)?.profile || undefined : undefined)
}

/** Background images as data: URLs, loaded once per image. */
class BackgroundImages {
  private urls = $state<Record<string, string>>({})
  private loading = new Set<string>()

  /** The image's URL, or '' while it loads (or if it's gone). */
  url(name: string): string {
    if (!name) return ''
    if (!(name in this.urls) && !this.loading.has(name)) {
      this.loading.add(name)
      void Backgrounds.Data(name)
        .then((u) => (this.urls[name] = u))
        .catch(() => (this.urls[name] = ''))
        .finally(() => this.loading.delete(name))
    }
    return this.urls[name] ?? ''
  }

  /** Asks for an image; returns its name, or '' when cancelled. */
  async choose(): Promise<string> {
    return (await Backgrounds.Choose()) ?? ''
  }
}

export const backgroundImages = new BackgroundImages()

/** Plays a short, quiet beep for the terminal bell. */
let audio: AudioContext | undefined
export function beep() {
  try {
    audio ??= new AudioContext()
    const osc = audio.createOscillator()
    const gain = audio.createGain()
    osc.frequency.value = 880
    gain.gain.setValueAtTime(0.08, audio.currentTime)
    gain.gain.exponentialRampToValueAtTime(0.0001, audio.currentTime + 0.15)
    osc.connect(gain).connect(audio.destination)
    osc.start()
    osc.stop(audio.currentTime + 0.15)
  } catch {
    // No audio output: the bell stays silent.
  }
}
