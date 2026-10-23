import { Clipboard } from '@wailsio/runtime'

/** Replaces the empty window with what went wrong when the app can't start, so a screenshot is enough to find the cause. */
export function showStartupError(err: unknown) {
  console.error(err)
  const details = [err instanceof Error ? `${err.name}: ${err.message}\n${err.stack ?? ''}` : String(err), '', `TuxShell ${__APP_VERSION__}`, navigator.userAgent].join('\n')

  const el = (tag: string, style: string, text = '') => {
    const node = document.createElement(tag)
    node.style.cssText = style
    node.textContent = text
    return node
  }
  const button = (text: string, primary: boolean, onclick: () => void) => {
    const b = el(
      'button',
      `padding:8px 14px;border-radius:10px;font:inherit;cursor:pointer;border:1px solid var(--color-line);` +
        (primary ? 'background:var(--color-accent);color:var(--color-on-accent)' : 'background:var(--color-raised);color:var(--color-fg)'),
      text,
    ) as HTMLButtonElement
    b.type = 'button'
    b.onclick = onclick
    return b
  }

  const box = el('div', 'max-width:720px;width:100%;background:var(--color-panel);border:1px solid var(--color-line);border-radius:16px;padding:24px')
  box.setAttribute('role', 'alert')
  box.append(
    el('h1', 'margin:0 0 6px;font-size:18px;color:var(--color-fg)', "TuxShell couldn't start"),
    el('p', 'margin:0 0 14px;color:var(--color-muted)', 'Please copy the details below and send them with a bug report.'),
  )
  const pre = el(
    'pre',
    'margin:0 0 16px;max-height:50vh;overflow:auto;white-space:pre-wrap;word-break:break-word;user-select:text;-webkit-user-select:text;' +
      'background:var(--color-field);color:var(--color-text);border:1px solid var(--color-line);border-radius:10px;padding:12px;font:12px/1.5 "JetBrains Mono Variable",monospace',
    details,
  )
  const copy = button('Copy details', true, () => {
    Clipboard.SetText(details).then(
      () => (copy.textContent = 'Copied'),
      () => getSelection()?.selectAllChildren(pre),
    )
  })
  const row = el('div', 'display:flex;gap:8px')
  row.append(copy, button('Reload', false, () => location.reload()))
  box.append(pre, row)

  const root = el(
    'main',
    'position:fixed;inset:0;display:flex;align-items:center;justify-content:center;padding:48px 24px 24px;' +
      'background:var(--color-ground);color:var(--color-text);font:14px/1.5 "Geist Variable",system-ui,sans-serif',
  )
  root.append(box)
  const app = document.getElementById('app')
  if (app) app.replaceChildren(root)
  else document.body.append(root)
}
