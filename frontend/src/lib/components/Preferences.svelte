<script lang="ts">
  import { onMount } from 'svelte'
  import { BUNDLED_FONT, limits, loginShells, prefs, systemFonts, type Terminal } from '../settings.svelte'
  import { schemes, schemeTheme } from '../schemes'
  import { theme } from '../theme.svelte'
  import Icon from './Icon.svelte'
  import { hostStatus } from '../status.svelte'
  import { updates, VERSION } from '../updates.svelte'
  import { Browser } from '@wailsio/runtime'
  import Logo from './Logo.svelte'
  import ExportHosts from './ExportHosts.svelte'

  const REPO = 'https://github.com/vgrigolaia/TuxShell'

  interface Props {
    onClose: () => void
    /** Opens the import window (it stacks over Preferences). */
    onImport: () => void
  }
  let { onClose, onImport }: Props = $props()

  type Section = 'text' | 'cursor' | 'colours' | 'behaviour' | 'hosts' | 'shortcuts' | 'about'
  const sections: { id: Section; label: string }[] = [
    { id: 'text', label: 'Text' },
    { id: 'cursor', label: 'Cursor' },
    { id: 'colours', label: 'Colours' },
    { id: 'behaviour', label: 'Behaviour' },
    { id: 'hosts', label: 'Hosts' },
    { id: 'shortcuts', label: 'Shortcuts' },
    { id: 'about', label: 'About' },
  ]
  let section = $state<Section>('text')
  let dialog: HTMLDialogElement
  let fonts = $state<string[] | null>(null)
  let shells = $state<string[]>([])
  let confirmReset = $state(false)
  const id = $props.id()

  const t = $derived(prefs.value.terminal)
  const fontMissing = $derived(!!fonts && !!t.fontFamily && t.fontFamily !== BUNDLED_FONT && !fonts.includes(t.fontFamily))
  const weights = [100, 200, 300, 400, 500, 600, 700, 800, 900]

  onMount(() => {
    dialog.showModal()
    void systemFonts().then((l) => (fonts = l))
    void loginShells().then((l) => (shells = l))
  })

  function setNumber(key: keyof Terminal, raw: string, [lo, hi]: readonly [number, number]) {
    const v = Number(raw)
    if (raw.trim() !== '' && Number.isFinite(v)) prefs.update({ [key]: Math.min(Math.max(v, lo), hi) })
  }

  const shortcuts: [string, string][] = [
    ['Ctrl+K', 'Quick connect / search hosts'],
    ['Ctrl+,', 'Preferences'],
    ['Ctrl+Shift+L', 'Show or hide the hosts sidebar'],
    ['Ctrl+Shift+E', 'Files panel'],
    ['Ctrl+Shift+H', 'Clipboard history'],
    ['Ctrl+Shift+B', 'Broadcast typing to split panes'],
    ['Ctrl+Shift+A', 'Split auto (same connection)'],
    ['Ctrl+Shift+O', 'Split down (same connection)'],
    ['Ctrl+Shift+T', 'Open the same connection in a new tab'],
    ['Ctrl+Shift+X', 'Maximize pane / restore split'],
    ['Ctrl+Shift+S', 'Show or hide the scrollbar'],
    ['Ctrl+Alt+W', 'Set pane title'],
    ['Ctrl+Shift+W', 'Close pane'],
    ['Ctrl+Shift+Arrows', 'Move between panes'],
    ['Ctrl+= / Ctrl+-', 'Zoom terminal text in / out'],
    ['Ctrl+mouse wheel', 'Zoom terminal text'],
    ['Ctrl+0', 'Reset zoom'],
    ['Right-click terminal', 'Copy, paste, split, title, read only, zoom, preferences'],
    ['Ctrl+Shift+C / Ctrl+Insert', 'Copy selection'],
    ['Ctrl+Shift+V / Shift+Insert', 'Paste'],
    ['Middle-click tab', 'Close tab'],
    ['Ctrl+click tab', 'Select tabs (merge, close)'],
  ]

  const label = 'mb-1.5 block text-xs font-medium text-muted'
  const field = 'h-9 rounded-lg border border-line bg-ground px-2.5 text-[13px] text-fg focus:border-accent focus:outline-none'
  const check = 'flex items-start gap-2.5 text-[13px]'
  const hint = 'mt-0.5 block text-xs text-subtle'
</script>

<dialog
  bind:this={dialog}
  aria-labelledby="{id}-title"
  class="m-auto flex h-[min(600px,90vh)] w-[min(820px,94vw)] flex-col overflow-hidden rounded-2xl border border-line bg-panel p-0 text-fg shadow-2xl backdrop:bg-black/60"
  onclose={onClose}
>
  <header class="flex items-center gap-3 border-b border-line px-5 py-3.5">
    <span class="text-accent"><Icon name="gear" size={16} /></span>
    <h2 id="{id}-title" class="flex-1 font-display text-[15px] font-semibold">Preferences</h2>
    <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Close preferences" onclick={() => dialog.close()}>
      <Icon name="x" size={14} />
    </button>
  </header>

  <div class="flex min-h-0 flex-1">
    <nav aria-label="Preference sections" class="w-40 shrink-0 border-r border-line p-2">
      <ul>
        {#each sections as s (s.id)}
          <li>
            <button
              class="w-full rounded-lg px-3 py-2 text-left text-[13px] {section === s.id ? 'bg-raised font-medium text-fg' : 'text-muted hover:bg-raised/60 hover:text-fg'}"
              aria-current={section === s.id ? 'page' : undefined}
              onclick={() => (section = s.id)}
            >
              {s.label}
            </button>
          </li>
        {/each}
      </ul>
    </nav>

    <div class="min-w-0 flex-1 overflow-y-auto px-6 py-5">
      {#if section === 'text'}
        <h3 class="sr-only">Text</h3>
        <div class="grid grid-cols-2 gap-x-5 gap-y-4">
          <div class="col-span-2">
            <label class={label} for="{id}-font">Font</label>
            <select id="{id}-font" class="{field} w-full" value={t.fontFamily || BUNDLED_FONT} onchange={(e) => prefs.update({ fontFamily: e.currentTarget.value === BUNDLED_FONT ? '' : e.currentTarget.value })}>
              <optgroup label="Bundled">
                <option value={BUNDLED_FONT}>JetBrains Mono (default)</option>
              </optgroup>
              {#if fontMissing}
                <option value={t.fontFamily}>{t.fontFamily} (not installed)</option>
              {/if}
              <optgroup label={fonts === null ? 'Installed (loading…)' : `Installed monospace fonts (${fonts.length})`}>
                {#each fonts ?? [] as f (f)}
                  <option value={f}>{f}</option>
                {/each}
              </optgroup>
            </select>
            {#if fontMissing}
              <p class="mt-1 text-xs text-warning">This font isn't installed, so terminals use JetBrains Mono.</p>
            {/if}
          </div>

          <div>
            <label class={label} for="{id}-size">Size</label>
            <div class="flex items-center gap-1.5">
              <button class="{field} w-9 px-0 hover:bg-raised" aria-label="Smaller text" onclick={() => prefs.update({ fontSize: Math.max(t.fontSize - 1, limits.fontSize[0]) })}>−</button>
              <input id="{id}-size" type="number" class="{field} w-20 text-center" min={limits.fontSize[0]} max={limits.fontSize[1]} value={t.fontSize} oninput={(e) => setNumber('fontSize', e.currentTarget.value, limits.fontSize)} />
              <button class="{field} w-9 px-0 hover:bg-raised" aria-label="Larger text" onclick={() => prefs.update({ fontSize: Math.min(t.fontSize + 1, limits.fontSize[1]) })}>+</button>
              <span class="text-xs text-subtle">px</span>
            </div>
            {#if prefs.zoom}
              <p class="mt-1 text-xs text-subtle">
                Zoomed to {prefs.fontSize}px ·
                <button class="underline hover:text-fg" onclick={() => (prefs.zoom = 0)}>reset (Ctrl+0)</button>
              </p>
            {/if}
          </div>

          <div>
            <label class={label} for="{id}-letter">Letter spacing</label>
            <div class="flex items-center gap-1.5">
              <input id="{id}-letter" type="number" class="{field} w-20 text-center" min={limits.letterSpacing[0]} max={limits.letterSpacing[1]} value={t.letterSpacing} oninput={(e) => setNumber('letterSpacing', e.currentTarget.value, limits.letterSpacing)} />
              <span class="text-xs text-subtle">px</span>
            </div>
          </div>

          <div class="col-span-2">
            <label class={label} for="{id}-line">Line spacing <span class="font-mono text-subtle">{t.lineHeight.toFixed(2)}</span></label>
            <input id="{id}-line" type="range" class="w-full accent-accent" min={limits.lineHeight[0]} max={limits.lineHeight[1]} step="0.05" value={t.lineHeight} oninput={(e) => setNumber('lineHeight', e.currentTarget.value, limits.lineHeight)} />
          </div>

          <div>
            <label class={label} for="{id}-weight">Weight</label>
            <select id="{id}-weight" class="{field} w-full" value={t.fontWeight} onchange={(e) => prefs.update({ fontWeight: Number(e.currentTarget.value) })}>
              {#each weights as w (w)}<option value={w}>{w}{w === 400 ? ' (normal)' : ''}</option>{/each}
            </select>
          </div>
          <div>
            <label class={label} for="{id}-bold">Bold weight</label>
            <select id="{id}-bold" class="{field} w-full" value={t.fontWeightBold} onchange={(e) => prefs.update({ fontWeightBold: Number(e.currentTarget.value) })}>
              {#each weights as w (w)}<option value={w}>{w}{w === 700 ? ' (bold)' : ''}</option>{/each}
            </select>
          </div>
        </div>
        {@render preview()}
      {:else if section === 'cursor'}
        <fieldset>
          <legend class={label}>Cursor shape</legend>
          <div class="flex gap-2">
            {#each [['block', 'Block'], ['bar', 'Bar'], ['underline', 'Underline']] as [value, name] (value)}
              <label class="flex flex-1 cursor-pointer flex-col items-center gap-2 rounded-xl border px-3 py-3 text-[13px] {t.cursorStyle === value ? 'border-accent bg-accent/10' : 'border-line hover:bg-raised'}">
                <input type="radio" class="sr-only" name="{id}-cursor" {value} checked={t.cursorStyle === value} onchange={() => prefs.update({ cursorStyle: value })} />
                <span class="flex h-6 items-end font-mono text-base" aria-hidden="true">
                  $&nbsp;<span class={value === 'block' ? 'h-5 w-2.5 bg-accent' : value === 'bar' ? 'h-5 w-0.5 bg-accent' : 'mb-0.5 h-0.5 w-2.5 bg-accent'}></span>
                </span>
                {name}
              </label>
            {/each}
          </div>
        </fieldset>
        <label class="{check} mt-5">
          <input type="checkbox" class="mt-0.5 accent-accent" checked={t.cursorBlink} onchange={(e) => prefs.update({ cursorBlink: e.currentTarget.checked })} />
          <span>Blinking cursor</span>
        </label>
      {:else if section === 'colours'}
        <fieldset>
          <legend class={label}>App theme</legend>
          <div class="flex gap-2">
            {#each [['dark', 'Dark', 'moon'], ['light', 'Light', 'sun'], ['system', 'Follow system', 'monitor']] as const as [value, name, icon] (value)}
              <label class="flex flex-1 cursor-pointer items-center justify-center gap-2 rounded-xl border px-3 py-2.5 text-[13px] {theme.mode === value ? 'border-accent bg-accent/10' : 'border-line hover:bg-raised'}">
                <input type="radio" class="sr-only" name="{id}-theme" {value} checked={theme.mode === value} onchange={() => prefs.setTheme(value)} />
                <Icon name={icon} size={15} />
                {name}
              </label>
            {/each}
          </div>
        </fieldset>

        <fieldset class="mt-6">
          <legend class={label}>Terminal colours</legend>
          <div class="grid grid-cols-3 gap-2">
            {#each [{ id: 'auto', name: 'Match app theme' }, ...schemes] as s (s.id)}
              {@const palette = schemeTheme(s.id, theme.resolved)}
              <label class="cursor-pointer rounded-xl border p-1.5 text-xs {t.colorScheme === s.id ? 'border-accent ring-1 ring-accent' : 'border-line hover:border-subtle'}">
                <input type="radio" class="sr-only" name="{id}-scheme" value={s.id} checked={t.colorScheme === s.id} onchange={() => prefs.update({ colorScheme: s.id })} />
                <span class="block rounded-lg px-2.5 py-2 font-mono text-[11px]" style:background={palette.background} style:color={palette.foreground}>
                  <span style:color={palette.green}>user@host</span>:<span style:color={palette.blue}>~</span>$ ls
                  <span class="mt-1.5 flex gap-1" aria-hidden="true">
                    {#each [palette.red, palette.green, palette.yellow, palette.blue, palette.magenta, palette.cyan] as c, i (i)}
                      <span class="size-2.5 rounded-full" style:background={c}></span>
                    {/each}
                  </span>
                </span>
                <span class="mt-1.5 block px-1 pb-0.5">{s.name}</span>
              </label>
            {/each}
          </div>
        </fieldset>
        {@render preview()}
      {:else if section === 'behaviour'}
        <div class="flex flex-col gap-4">
          <div>
            <label class={label} for="{id}-scrollback">Scrollback</label>
            <div class="flex items-center gap-2">
              <input id="{id}-scrollback" type="number" class="{field} w-32" min="0" max={limits.scrollback[1]} step="1000" value={t.scrollback} oninput={(e) => setNumber('scrollback', e.currentTarget.value, limits.scrollback)} />
              <span class="text-xs text-subtle">lines kept per terminal</span>
            </div>
          </div>

          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={t.copyOnSelect} onchange={(e) => prefs.update({ copyOnSelect: e.currentTarget.checked })} />
            <span>Copy on select<span class={hint}>Selecting text with the mouse copies it to the clipboard.</span></span>
          </label>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={t.rightClickPaste} onchange={(e) => prefs.update({ rightClickPaste: e.currentTarget.checked })} />
            <span>Right-click pastes<span class={hint}>Like PuTTY and MobaXterm. Shift+right-click still opens the menu.</span></span>
          </label>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={t.ctrlWheelZoom} onchange={(e) => prefs.update({ ctrlWheelZoom: e.currentTarget.checked })} />
            <span>Ctrl+mouse wheel zooms text</span>
          </label>
          <label class={check}>
            <input
              type="checkbox"
              class="mt-0.5 accent-accent"
              checked={prefs.value.hostStatus}
              onchange={(e) => {
                prefs.setHostStatus(e.currentTarget.checked)
                if (e.currentTarget.checked) setTimeout(() => hostStatus.refresh(), 400)
              }}
            />
            <span
              >Show which hosts are up<span class={hint}
                >Pings saved hosts once a minute (ICMP, never the SSH port, so server logs and fail2ban aren't affected). Hosts behind a jump host show as up while a session is open.</span
              ></span
            >
          </label>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={t.confirmClose} onchange={(e) => prefs.update({ confirmClose: e.currentTarget.checked })} />
            <span>Ask before closing running sessions<span class={hint}>When closing a pane, tabs or the window.</span></span>
          </label>

          <div>
            <label class={label} for="{id}-bell">Terminal bell</label>
            <select id="{id}-bell" class="{field} w-48" value={t.bell} onchange={(e) => prefs.update({ bell: e.currentTarget.value })}>
              <option value="off">Off</option>
              <option value="visual">Flash the pane</option>
            </select>
          </div>

          <div>
            <label class={label} for="{id}-shell">Shell for local tabs</label>
            <select
              id="{id}-shell"
              class="{field} w-64 font-mono"
              value={t.shell}
              aria-invalid={prefs.error?.field === 'shell'}
              aria-describedby="{id}-shell-hint"
              onchange={(e) => prefs.update({ shell: e.currentTarget.value })}
            >
              <option value="">Default ($SHELL)</option>
              {#each shells as sh (sh)}<option value={sh}>{sh}</option>{/each}
            </select>
            <span id="{id}-shell-hint" class={hint}>From /etc/shells. Applies to new local tabs.</span>
            {#if prefs.error?.field === 'shell'}<p role="alert" class="mt-1 text-xs text-warning">{prefs.error.message}</p>{/if}
          </div>
        </div>
      {:else if section === 'hosts'}
        <h3 class="sr-only">Hosts</h3>
        <section aria-labelledby="{id}-import">
          <h4 id="{id}-import" class="text-[13px] font-medium">Import hosts</h4>
          <p class={hint}>From ~/.ssh/config, CSV/TSV, Excel (.xlsx) or a TuxShell export. You see every host before it's saved. Passwords are never imported.</p>
          <button class="mt-3 rounded-lg bg-accent px-3 py-1.5 text-xs font-medium text-on-accent hover:bg-accent/90" onclick={onImport}>Import hosts…</button>
        </section>
        <ExportHosts />
      {:else if section === 'about'}
        <h3 class="sr-only">About</h3>
        <div class="flex items-center gap-4">
          <Logo size={56} />
          <div>
            <p class="font-display text-xl font-bold tracking-[-0.02em]">TuxShell</p>
            <p class="text-[13px] text-muted">Version <span class="font-mono text-fg">{VERSION}</span></p>
          </div>
        </div>
        <p class="mt-4 text-[13px] text-muted">A free, open-source SSH connection manager for Linux.</p>
        <p class="mt-1 text-xs text-subtle">© vgrigolaia · GPL-3.0-or-later</p>

        <div class="mt-5 flex flex-wrap gap-2">
          {#each [['Source code', REPO], ['Releases', `${REPO}/releases`], ['Report a problem', `${REPO}/issues`]] as [name, url] (url)}
            <button class="rounded-lg border border-line px-2.5 py-1.5 text-xs text-muted hover:bg-raised hover:text-fg" onclick={() => void Browser.OpenURL(url)}>{name}</button>
          {/each}
        </div>

        <section class="mt-6 border-t border-line pt-5" aria-labelledby="{id}-updates">
          <h4 id="{id}-updates" class={label}>Updates</h4>
          <div class="flex flex-wrap items-center gap-3">
            <button
              class="rounded-lg bg-accent px-3 py-1.5 text-xs font-medium text-on-accent hover:bg-accent/90 disabled:opacity-60"
              disabled={updates.checking}
              onclick={() => void updates.check()}>{updates.checking ? 'Checking…' : 'Check for updates'}</button
            >
            <span class="text-[13px]" aria-live="polite">
              {#if updates.error}
                <span class="text-warning">{updates.error}</span>
              {:else if updates.result?.newer}
                TuxShell <span class="font-mono">{updates.result.latest}</span> is available.
                <button class="ml-1 text-accent underline-offset-2 hover:underline" onclick={() => void Browser.OpenURL(updates.result!.url)}>Download</button>
              {:else if updates.result}
                <span class="text-success">You're up to date.</span>
              {/if}
            </span>
          </div>
          <label class="{check} mt-4">
            <input type="checkbox" class="mt-0.5 accent-accent" checked={prefs.value.updateCheck} onchange={(e) => prefs.setUpdateCheck(e.currentTarget.checked)} />
            <span>Check for updates at startup<span class={hint}>Asks GitHub for the latest release once when TuxShell starts. Nothing is downloaded or installed.</span></span>
          </label>
        </section>
      {:else}
        <h3 class="sr-only">Shortcuts</h3>
        <table class="w-full text-[13px]">
          <tbody>
            {#each shortcuts as [keys, action] (keys)}
              <tr class="border-b border-line last:border-0">
                <td class="py-2 pr-4 whitespace-nowrap"><kbd class="rounded border border-line bg-ground px-1.5 py-0.5 font-mono text-[11px]">{keys}</kbd></td>
                <td class="py-2 text-muted">{action}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        <p class="mt-3 text-xs text-subtle">Custom key bindings are planned for a later version.</p>
      {/if}
    </div>
  </div>

  <footer class="flex items-center gap-3 border-t border-line px-5 py-3 text-xs">
    <span class="min-w-0 flex-1 truncate text-subtle" aria-live="polite">
      {#if prefs.error && prefs.error.field !== 'shell'}
        <span class="text-warning">Not saved: {prefs.error.message}</span>
      {:else}
        Changes apply right away · saved in <span class="font-mono">~/.config/tuxshell/settings.json</span>
      {/if}
    </span>
    {#if confirmReset}
      <span class="text-muted">Reset everything?</span>
      <button class="rounded-lg px-2.5 py-1.5 text-muted hover:bg-raised hover:text-fg" onclick={() => (confirmReset = false)}>Cancel</button>
      <button
        class="rounded-lg bg-danger px-2.5 py-1.5 font-medium text-white hover:bg-danger/90"
        onclick={async () => {
          await prefs.reset()
          confirmReset = false
        }}>Reset</button
      >
    {:else}
      <button class="rounded-lg px-2.5 py-1.5 text-muted hover:bg-raised hover:text-fg" onclick={() => (confirmReset = true)}>Reset to defaults</button>
    {/if}
    <button class="rounded-lg bg-accent px-3 py-1.5 font-medium text-on-accent hover:bg-accent/90" onclick={() => dialog.close()}>Done</button>
  </footer>
</dialog>

{#snippet preview()}
  {@const palette = schemeTheme(t.colorScheme, theme.resolved)}
  <figure class="mt-6">
    <figcaption class={label}>Preview</figcaption>
    <div
      class="overflow-hidden rounded-xl border border-line px-4 py-3"
      style:background={palette.background}
      style:color={palette.foreground}
      style:font-family={prefs.fontFamily}
      style:font-size="{prefs.fontSize}px"
      style:line-height={t.lineHeight}
      style:letter-spacing="{t.letterSpacing}px"
      style:font-weight={t.fontWeight}
    >
      <div><span style:color={palette.green} style:font-weight={t.fontWeightBold}>admin@web-01</span>:<span style:color={palette.blue}>~/app</span>$ git status</div>
      <div>On branch <span style:color={palette.cyan}>main</span> · <span style:color={palette.red}>2 files changed</span>, <span style:color={palette.yellow}>1 warning</span></div>
      <div>0O 1lI {'{}'} [] =&gt; != ~ ä ü ß
        <span class="inline-block align-middle" style:background={palette.cursor} style:width={t.cursorStyle === 'bar' ? '2px' : '0.6em'} style:height={t.cursorStyle === 'underline' ? '2px' : '1.1em'}></span>
      </div>
    </div>
  </figure>
{/snippet}
