import { Clipboard } from '@wailsio/runtime'

/**
 * The RDP engine only talks to navigator.clipboard, which WebKitGTK blocks
 * here: wails:// isn't a secure origin, and reads need a fresh click. So the
 * engine gets a text clipboard backed by the desktop's (through Wails), the
 * same one the terminals use. With only readText/writeText the engine uses
 * its manual text mode: RdpPane sends and saves at the right moments.
 */
export function installClipboard() {
  const shim = {
    readText: async () => (await Clipboard.Text()) ?? '',
    writeText: (text: string) => Clipboard.SetText(text),
  }
  Object.defineProperty(navigator, 'clipboard', { value: shim, configurable: true })
  // The engine checks this before touching the clipboard at all.
  if (!window.isSecureContext) Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })
}
