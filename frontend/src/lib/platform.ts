/** True in the macOS build (WebKit there reports a Macintosh user agent). */
export const isMac = /Macintosh|Mac OS X/.test(navigator.userAgent)

/**
 * The app's shortcut modifier: ⌘ on macOS, Ctrl elsewhere. On macOS Ctrl is
 * left to the terminal (Ctrl+C interrupts there as usual).
 */
export function modKey(e: KeyboardEvent | MouseEvent): boolean {
  return isMac ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey
}

/** A shortcut label for this platform: "Ctrl+Shift+L" becomes "⇧⌘L" on macOS. */
export function keys(label: string): string {
  if (!isMac) return label
  return label
    .replace(/Ctrl\+Shift\+/g, '⇧⌘')
    .replace(/Ctrl\+Alt\+/g, '⌥⌘')
    .replace(/Ctrl\+click/g, '⌘-click')
    .replace(/Ctrl\+mouse wheel/g, '⌘+scroll')
    .replace(/Ctrl[+ ]/g, '⌘')
    .replace(/Cmd\+/g, '⌘')
    .replace(/Shift\+/g, '⇧')
}
