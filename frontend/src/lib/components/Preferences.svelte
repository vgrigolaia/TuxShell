<script lang="ts">
  import { isMac, keys } from '../platform'
  import { onMount } from 'svelte'
  import { BUNDLED_FONT, DEFAULT_PROFILE, defaultPalette, limits, loginShells, prefs, systemFonts, type Palette, type Profile } from '../settings.svelte'
  import { ansiNames, paletteTheme, profileTheme, schemes, schemeTheme } from '../schemes'
  import { backgroundImages } from '../profiles.svelte'
  import { theme } from '../theme.svelte'
  import Icon from './Icon.svelte'
  import { hostStatus } from '../status.svelte'
  import { updates, VERSION } from '../updates.svelte'
  import { Browser } from '@wailsio/runtime'
  import Logo from './Logo.svelte'
  import ExportHosts from './ExportHosts.svelte'
  import { logins } from '../logins.svelte'
  import { onePassword } from '../onepassword.svelte'
  import * as Login from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/login/service'
  import { hostStore } from '../hosts.svelte'
  import * as HostIO from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/hostio/service'

  const REPO = 'https://github.com/vgrigolaia/TuxShell'

  interface Props {
    onClose: () => void
    /** Opens the import window (it stacks over Preferences). */
    onImport: () => void
  }
  let { onClose, onImport }: Props = $props()

  type Section = 'profiles' | 'text' | 'cursor' | 'colours' | 'scrolling' | 'bell' | 'command' | 'general' | 'hosts' | 'security' | 'shortcuts' | 'about'
  // Profile sections edit the profile chosen at their top; app sections apply everywhere.
  const groups: { label: string; sections: { id: Section; label: string }[] }[] = [
    {
      label: 'Terminal profile',
      sections: [
        { id: 'profiles', label: 'Profiles' },
        { id: 'text', label: 'Text' },
        { id: 'cursor', label: 'Cursor' },
        { id: 'colours', label: 'Colours' },
        { id: 'scrolling', label: 'Scrolling' },
        { id: 'bell', label: 'Bell' },
        { id: 'command', label: 'Command & keys' },
      ],
    },
    {
      label: 'App',
      sections: [
        { id: 'general', label: 'General' },
        { id: 'hosts', label: 'Hosts' },
        { id: 'security', label: 'Security' },
        { id: 'shortcuts', label: 'Shortcuts' },
        { id: 'about', label: 'About' },
      ],
    },
  ]
  const profileSections: Section[] = ['text', 'cursor', 'colours', 'scrolling', 'bell', 'command']
  let section = $state<Section>('text')
  let dialog: HTMLDialogElement
  let fonts = $state<string[] | null>(null)
  let shells = $state<string[]>([])
  let confirmReset = $state(false)
  let templateSaved = $state('')
  let confirmForget = $state(false)
  let forgetError = $state('')

  $effect(() => {
    if (section === 'security') {
      void logins.refresh()
      void onePassword.refresh()
    }
  })

  async function setSavePasswords(on: boolean) {
    forgetError = ''
    if (on || logins.status.saved === 0) {
      prefs.setSavePasswords(on)
      confirmForget = false
      return
    }
    confirmForget = true
  }

  async function forgetAndTurnOff() {
    try {
      await Login.ForgetAll()
      prefs.setSavePasswords(false)
      confirmForget = false
      await Promise.all([logins.refresh(), hostStore.load()])
    } catch (e) {
      forgetError = e instanceof Error ? e.message : String(e)
    }
  }
  const id = $props.id()

  // The profile being edited; a deleted one falls back to the default.
  let editingId = $state(DEFAULT_PROFILE)
  const editing = $derived(prefs.value.profiles?.some((p) => p.id === editingId) ? editingId : DEFAULT_PROFILE)
  const t = $derived(prefs.profile(editing))
  // App-wide terminal settings.
  const g = $derived(prefs.value.terminal)
  const setP = (patch: Partial<Profile>) => prefs.updateProfile(editing, patch)
  const editingName = $derived(editing === DEFAULT_PROFILE ? 'Default' : (prefs.value.profiles?.find((p) => p.id === editing)?.name ?? 'Default'))
  const hostsUsing = (pid: string) => hostStore.list.filter((h) => (h.profile || DEFAULT_PROFILE) === pid).length

  let bgError = $state('')
  async function chooseBackground() {
    bgError = ''
    try {
      const name = await backgroundImages.choose()
      if (name) setP({ background: name })
    } catch (e) {
      bgError = e instanceof Error ? e.message : String(e)
    }
  }

  function setPaletteColor(key: keyof Palette, value: string, index = 0) {
    const pal: Palette = { ...(t.palette ?? defaultPalette()), colors: [...(t.palette?.colors ?? defaultPalette().colors ?? [])] }
    if (key === 'colors') pal.colors![index] = value
    else (pal as unknown as Record<string, string>)[key] = value
    setP({ palette: pal, colorScheme: 'custom' })
  }

  /** Starts a custom palette from the scheme shown now. */
  function customize() {
    const th = profileTheme(t, theme.resolved)
    const names = ['black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white'] as const
    const colors = [...names.map((n) => th[n]), ...names.map((n) => th[`bright${n[0].toUpperCase()}${n.slice(1)}` as 'brightBlack'])]
    const hex = (c: string | undefined, d: string) => (c && /^#[0-9a-fA-F]{6}$/.test(c) ? c.toLowerCase() : d)
    const d = defaultPalette()
    setP({
      colorScheme: 'custom',
      palette: {
        background: hex(th.background, d.background),
        foreground: hex(th.foreground, d.foreground),
        cursor: hex(th.cursor, d.cursor),
        selection: hex(th.selectionBackground, d.selection),
        colors: colors.map((c, i) => hex(c, d.colors![i])),
      },
    })
  }

  let newName = $state('')
  function addProfile(from: string) {
    const base = from === DEFAULT_PROFILE ? 'Default' : (prefs.value.profiles?.find((p) => p.id === from)?.name ?? 'Profile')
    const name = newName.trim() || (from === DEFAULT_PROFILE && !newName.trim() ? 'New profile' : `${base} copy`)
    editingId = prefs.addProfile(name.slice(0, 40), from)
    newName = ''
  }
  let confirmDelete = $state<string | null>(null)

  const headerColors = ['#8b7cff', '#22d3ee', '#a3e635', '#facc15', '#fb923c', '#f43f5e', '#ec4899', '#94a3b8']
  const fontMissing = $derived(!!fonts && !!t.fontFamily && t.fontFamily !== BUNDLED_FONT && !fonts.includes(t.fontFamily))
  const weights = [100, 200, 300, 400, 500, 600, 700, 800, 900]

  onMount(() => {
    dialog.showModal()
    void systemFonts().then((l) => (fonts = l))
    void loginShells().then((l) => (shells = l))
  })

  function setNumber(key: keyof Profile, raw: string, [lo, hi]: readonly [number, number]) {
    const v = Number(raw)
    if (raw.trim() !== '' && Number.isFinite(v)) setP({ [key]: Math.min(Math.max(v, lo), hi) })
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
    ['Alt+Up / Alt+Down', 'Move the focused host or group in the hosts list'],
    ['Ctrl+= / Ctrl+-', 'Zoom terminal text in / out'],
    ['Ctrl+mouse wheel', 'Zoom terminal text'],
    ['Ctrl+0', 'Reset zoom'],
    ['Right-click terminal', 'Copy, paste, split, title, read only, zoom, preferences'],
    [isMac ? 'Cmd+C' : 'Ctrl+Shift+C / Ctrl+Insert', 'Copy selection'],
    [isMac ? 'Cmd+V' : 'Ctrl+Shift+V / Shift+Insert', 'Paste'],
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
    <nav aria-label="Preference sections" class="w-44 shrink-0 overflow-y-auto border-r border-line p-2">
      {#each groups as grp (grp.label)}
        <h3 class="px-3 pt-2 pb-1 text-[10px] font-semibold tracking-wider text-subtle uppercase">{grp.label}</h3>
        <ul class="mb-2">
          {#each grp.sections as s (s.id)}
            <li>
              <button
                class="w-full rounded-lg px-3 py-1.5 text-left text-[13px] {section === s.id ? 'bg-raised font-medium text-fg' : 'text-muted hover:bg-raised/60 hover:text-fg'}"
                aria-current={section === s.id ? 'page' : undefined}
                onclick={() => (section = s.id)}
              >
                {s.label}
              </button>
            </li>
          {/each}
        </ul>
      {/each}
    </nav>

    <div class="min-w-0 flex-1 overflow-y-auto px-6 py-5">
      {#if profileSections.includes(section)}
        <div class="mb-5 flex items-center gap-2 rounded-xl border border-line bg-ground/60 px-3 py-2">
          <label class="text-xs font-medium text-muted" for="{id}-editing">Editing profile</label>
          <select id="{id}-editing" class="{field} h-8 min-w-0 flex-1" value={editing} onchange={(e) => (editingId = e.currentTarget.value)}>
            <option value={DEFAULT_PROFILE}>Default</option>
            {#each prefs.value.profiles ?? [] as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
          </select>
          <button class="rounded-lg px-2.5 py-1.5 text-xs text-muted hover:bg-raised hover:text-fg" onclick={() => (section = 'profiles')}>Manage…</button>
        </div>
      {/if}
      {#if section === 'profiles'}
        <h3 class="sr-only">Profiles</h3>
        <p class="text-[13px] text-muted">
          A profile is a set of terminal settings: text, cursor, colours, background, scrolling, bell and a command to run on connect. Pick one per host in the host form. Local tabs and hosts
          without a profile use <span class="text-fg">Default</span>.
        </p>
        <ul class="mt-4 divide-y divide-line rounded-xl border border-line">
          {#each [{ id: DEFAULT_PROFILE, name: 'Default' }, ...(prefs.value.profiles ?? [])] as p (p.id)}
            {@const isDefault = p.id === DEFAULT_PROFILE}
            {@const pal = profileTheme(prefs.profile(p.id), theme.resolved)}
            <li class="flex items-center gap-3 px-3 py-2.5">
              <span class="grid size-8 shrink-0 place-items-center rounded-lg border border-line font-mono text-[11px]" style:background={pal.background} style:color={pal.green} aria-hidden="true">$_</span>
              {#if isDefault}
                <span class="min-w-0 flex-1 text-[13px] font-medium">Default</span>
              {:else}
                <input
                  class="{field} h-8 min-w-0 flex-1"
                  aria-label="Profile name"
                  maxlength="40"
                  value={p.name}
                  onchange={(e) => e.currentTarget.value.trim() && prefs.renameProfile(p.id, e.currentTarget.value.trim())}
                />
              {/if}
              <span class="shrink-0 text-xs text-subtle">{isDefault ? 'local tabs + ' : ''}{hostsUsing(p.id)} host{hostsUsing(p.id) === 1 ? '' : 's'}</span>
              <button
                class="rounded-lg px-2 py-1 text-xs text-muted hover:bg-raised hover:text-fg"
                onclick={() => {
                  editingId = p.id
                  section = 'text'
                }}>Edit</button
              >
              <button class="rounded-lg px-2 py-1 text-xs text-muted hover:bg-raised hover:text-fg" onclick={() => addProfile(p.id)} aria-label="Duplicate {p.name}">Duplicate</button>
              {#if !isDefault}
                {#if confirmDelete === p.id}
                  <button class="rounded-lg bg-danger px-2 py-1 text-xs font-medium text-white hover:bg-danger/90" onclick={() => (prefs.deleteProfile(p.id), (confirmDelete = null))}>Delete</button>
                  <button class="rounded-lg px-2 py-1 text-xs text-muted hover:bg-raised" onclick={() => (confirmDelete = null)}>Keep</button>
                {:else}
                  <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-danger" aria-label="Delete {p.name}" onclick={() => (confirmDelete = p.id)}><Icon name="trash" size={14} /></button>
                {/if}
              {/if}
            </li>
          {/each}
        </ul>
        {#if confirmDelete}
          <p class="mt-2 text-xs text-muted" role="status">Hosts using it will use Default instead.</p>
        {/if}
        <form
          class="mt-4 flex items-center gap-2"
          onsubmit={(e) => {
            e.preventDefault()
            addProfile(DEFAULT_PROFILE)
          }}
        >
          <input class="{field} w-64" placeholder="New profile name, e.g. Production" maxlength="40" aria-label="New profile name" bind:value={newName} />
          <button class="rounded-lg bg-accent px-3 py-2 text-xs font-medium text-on-accent hover:bg-accent/90" disabled={(prefs.value.profiles?.length ?? 0) >= 50}>New profile</button>
          <span class="text-xs text-subtle">Starts as a copy of Default.</span>
        </form>
      {:else if section === 'text'}
        <h3 class="sr-only">Text</h3>
        <div class="grid grid-cols-2 gap-x-5 gap-y-4">
          <div class="col-span-2">
            <label class={label} for="{id}-font">Font</label>
            <select id="{id}-font" class="{field} w-full" value={t.fontFamily || BUNDLED_FONT} onchange={(e) => setP({ fontFamily: e.currentTarget.value === BUNDLED_FONT ? '' : e.currentTarget.value })}>
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
              <button class="{field} w-9 px-0 hover:bg-raised" aria-label="Smaller text" onclick={() => setP({ fontSize: Math.max(t.fontSize - 1, limits.fontSize[0]) })}>−</button>
              <input id="{id}-size" type="number" class="{field} w-20 text-center" min={limits.fontSize[0]} max={limits.fontSize[1]} value={t.fontSize} oninput={(e) => setNumber('fontSize', e.currentTarget.value, limits.fontSize)} />
              <button class="{field} w-9 px-0 hover:bg-raised" aria-label="Larger text" onclick={() => setP({ fontSize: Math.min(t.fontSize + 1, limits.fontSize[1]) })}>+</button>
              <span class="text-xs text-subtle">px</span>
            </div>
            {#if prefs.zoom}
              <p class="mt-1 text-xs text-subtle">
                Zoomed to {prefs.fontSize}px ·
                <button class="underline hover:text-fg" onclick={() => (prefs.zoom = 0)}>reset ({keys('Ctrl+0')})</button>
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
            <select id="{id}-weight" class="{field} w-full" value={t.fontWeight} onchange={(e) => setP({ fontWeight: Number(e.currentTarget.value) })}>
              {#each weights as w (w)}<option value={w}>{w}{w === 400 ? ' (normal)' : ''}</option>{/each}
            </select>
          </div>
          <div>
            <label class={label} for="{id}-bold">Bold weight</label>
            <select id="{id}-bold" class="{field} w-full" value={t.fontWeightBold} onchange={(e) => setP({ fontWeightBold: Number(e.currentTarget.value) })}>
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
                <input type="radio" class="sr-only" name="{id}-cursor" {value} checked={t.cursorStyle === value} onchange={() => setP({ cursorStyle: value })} />
                <span class="flex h-6 items-end font-mono text-base" aria-hidden="true">
                  $&nbsp;<span class={value === 'block' ? 'h-5 w-2.5 bg-accent' : value === 'bar' ? 'h-5 w-0.5 bg-accent' : 'mb-0.5 h-0.5 w-2.5 bg-accent'}></span>
                </span>
                {name}
              </label>
            {/each}
          </div>
        </fieldset>
        <label class="{check} mt-5">
          <input type="checkbox" class="mt-0.5 accent-accent" checked={t.cursorBlink} onchange={(e) => setP({ cursorBlink: e.currentTarget.checked })} />
          <span>Blinking cursor</span>
        </label>
      {:else if section === 'colours'}
        <fieldset>
          <legend class={label}>Terminal colours</legend>
          <div class="grid grid-cols-3 gap-2">
            {#each [{ id: 'auto', name: 'Match app theme' }, ...schemes, { id: 'custom', name: 'Custom' }] as s (s.id)}
              {@const palette = s.id === 'custom' ? paletteTheme(t.palette ?? defaultPalette()) : schemeTheme(s.id, theme.resolved)}
              <label class="cursor-pointer rounded-xl border p-1.5 text-xs {t.colorScheme === s.id ? 'border-accent ring-1 ring-accent' : 'border-line hover:border-subtle'}">
                <input type="radio" class="sr-only" name="{id}-scheme" value={s.id} checked={t.colorScheme === s.id} onchange={() => setP({ colorScheme: s.id })} />
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

        <section class="mt-6" aria-labelledby="{id}-palette">
          <div class="flex items-center gap-3">
            <h4 id="{id}-palette" class="{label} mb-0 flex-1">Custom colours</h4>
            <button class="rounded-lg border border-line px-2.5 py-1 text-xs text-muted hover:bg-raised hover:text-fg" onclick={customize}>Start from the scheme shown</button>
          </div>
          <div class="mt-2 grid grid-cols-4 gap-2">
            {#each [['background', 'Background'], ['foreground', 'Text'], ['cursor', 'Cursor'], ['selection', 'Selection']] as const as [key, name] (key)}
              <label class="flex items-center gap-2 text-xs text-muted">
                <input type="color" class="h-7 w-9 cursor-pointer rounded border border-line bg-ground" value={t.palette?.[key] ?? defaultPalette()[key]} oninput={(e) => setPaletteColor(key, e.currentTarget.value)} />
                {name}
              </label>
            {/each}
          </div>
          <div class="mt-3 grid grid-cols-8 gap-1.5" role="group" aria-label="ANSI colours, normal then bright">
            {#each t.palette?.colors ?? defaultPalette().colors ?? [] as c, i (i)}
              <input
                type="color"
                class="h-7 w-full cursor-pointer rounded border border-line bg-ground"
                value={c}
                aria-label="{i >= 8 ? 'Bright ' : ''}{ansiNames[i % 8]}"
                title="{i >= 8 ? 'Bright ' : ''}{ansiNames[i % 8]}"
                oninput={(e) => setPaletteColor('colors', e.currentTarget.value, i)}
              />
            {/each}
          </div>
          <p class={hint}>Changing a colour switches this profile to Custom.</p>
        </section>

        <section class="mt-6" aria-labelledby="{id}-bg">
          <h4 id="{id}-bg" class={label}>Background image</h4>
          <div class="flex items-center gap-3">
            {#if t.background}
              {@const url = backgroundImages.url(t.background)}
              <span class="h-12 w-20 shrink-0 rounded-lg border border-line bg-cover bg-center" style:background-image={url ? `url(${url})` : undefined} aria-hidden="true"></span>
            {/if}
            <button class="rounded-lg border border-line px-2.5 py-1.5 text-xs text-muted hover:bg-raised hover:text-fg" onclick={chooseBackground}>
              <span class="inline-flex items-center gap-1.5"><Icon name="image" size={13} />{t.background ? 'Change image…' : 'Choose image…'}</span>
            </button>
            {#if t.background}
              <button class="rounded-lg px-2.5 py-1.5 text-xs text-muted hover:bg-raised hover:text-fg" onclick={() => setP({ background: '' })}>Remove</button>
            {/if}
          </div>
          {#if bgError}<p role="alert" class="mt-1 text-xs text-warning">{bgError}</p>{/if}
          {#if t.background}
            <label class="mt-3 block {label}" for="{id}-shade">Shade <span class="font-mono text-subtle">{t.backgroundShade}%</span></label>
            <input id="{id}-shade" type="range" class="w-full accent-accent" min="0" max="100" step="5" value={t.backgroundShade} oninput={(e) => setP({ backgroundShade: Number(e.currentTarget.value) })} />
            <span class={hint}>How much of the scheme's background colour covers the image, so text stays readable.</span>
          {:else}
            <span class={hint}>PNG, JPEG or WebP. TuxShell keeps its own copy.</span>
          {/if}
        </section>

        <fieldset class="mt-6">
          <legend class={label}>Pane header colour</legend>
          <div class="flex flex-wrap items-center gap-2">
            <label class="cursor-pointer rounded-lg border px-2.5 py-1 text-xs {t.headerColor === '' ? 'border-accent bg-accent/10' : 'border-line hover:bg-raised'}">
              <input type="radio" class="sr-only" name="{id}-header" checked={t.headerColor === ''} onchange={() => setP({ headerColor: '' })} />None
            </label>
            {#each headerColors as c (c)}
              <label class="grid size-7 cursor-pointer place-items-center rounded-full border-2 {t.headerColor === c ? 'border-fg' : 'border-transparent'}" title={c}>
                <input type="radio" class="sr-only" name="{id}-header" value={c} checked={t.headerColor === c} onchange={() => setP({ headerColor: c })} aria-label="Header colour {c}" />
                <span class="size-5 rounded-full" style:background={c}></span>
              </label>
            {/each}
            <label class="flex items-center gap-1.5 text-xs text-muted">
              <input type="color" class="h-7 w-9 cursor-pointer rounded border border-line bg-ground" value={t.headerColor || '#8b7cff'} oninput={(e) => setP({ headerColor: e.currentTarget.value })} />
              Other
            </label>
          </div>
          <span class={hint}>Tints the header of panes using this profile, e.g. red for production servers.</span>
        </fieldset>
        {@render preview()}
      {:else if section === 'scrolling'}
        <div class="flex flex-col gap-4">
          <div>
            <label class={label} for="{id}-scrollback">Scrollback</label>
            <div class="flex items-center gap-2">
              <input id="{id}-scrollback" type="number" class="{field} w-32" min="0" max={limits.scrollback[1]} step="1000" value={t.scrollback} oninput={(e) => setNumber('scrollback', e.currentTarget.value, limits.scrollback)} />
              <span class="text-xs text-subtle">lines kept per terminal</span>
            </div>
          </div>
          <div>
            <label class={label} for="{id}-speed">Mouse wheel speed <span class="font-mono text-subtle">×{t.scrollSpeed}</span></label>
            <input id="{id}-speed" type="range" class="w-64 accent-accent" min={limits.scrollSpeed[0]} max={limits.scrollSpeed[1]} step="1" value={t.scrollSpeed} oninput={(e) => setNumber('scrollSpeed', e.currentTarget.value, limits.scrollSpeed)} />
          </div>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={t.scrollbar} onchange={(e) => setP({ scrollbar: e.currentTarget.checked })} />
            <span>Show the scrollbar<span class={hint}>For new panes. {keys('Ctrl+Shift+S')} still shows or hides it per pane.</span></span>
          </label>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={t.scrollOnOutput} onchange={(e) => setP({ scrollOnOutput: e.currentTarget.checked })} />
            <span>Scroll to the bottom on output<span class={hint}>Otherwise reading back stays put while new output arrives.</span></span>
          </label>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={t.scrollOnKeypress} onchange={(e) => setP({ scrollOnKeypress: e.currentTarget.checked })} />
            <span>Scroll to the bottom when typing</span>
          </label>
        </div>
      {:else if section === 'bell'}
        <p class="text-[13px] text-muted">What happens when a program rings the terminal bell (for example Tab completion with no match).</p>
        <div class="mt-4 flex flex-col gap-4">
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={t.bell === 'visual'} onchange={(e) => setP({ bell: e.currentTarget.checked ? 'visual' : 'off' })} />
            <span>Flash the pane</span>
          </label>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={t.bellIcon} onchange={(e) => setP({ bellIcon: e.currentTarget.checked })} />
            <span>Show a bell icon<span class={hint}>On the pane and its tab when you're looking elsewhere, until you open it.</span></span>
          </label>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={t.bellSound} onchange={(e) => setP({ bellSound: e.currentTarget.checked })} />
            <span>Play a sound<span class={hint}>A short, quiet beep.</span></span>
          </label>
        </div>
      {:else if section === 'command'}
        <div class="flex flex-col gap-4">
          <div>
            <label class={label} for="{id}-command">Run on connect</label>
            <input
              id="{id}-command"
              class="{field} w-full font-mono"
              placeholder="e.g. tmux new -A -s main"
              maxlength={limits.command}
              spellcheck="false"
              value={t.command}
              aria-describedby="{id}-command-hint"
              onchange={(e) => setP({ command: e.currentTarget.value.replace(/[\r\n]+/g, ' ').trim() })}
            />
            <span id="{id}-command-hint" class={hint}>SSH hosts only: the server runs this after you log in. Local tabs use the shell from General.</span>
          </div>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={t.keepShell} disabled={!t.command} onchange={(e) => setP({ keepShell: e.currentTarget.checked })} />
            <span>Keep a shell open after the command ends<span class={hint}>Off: the session closes when the command finishes.</span></span>
          </label>

          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={t.record} onchange={(e) => setP({ record: e.currentTarget.checked })} />
            <span
              >Record sessions automatically<span class={hint}
                >Saves what the terminal shows (never your keystrokes) to ~/.local/share/tuxshell/recordings, readable only by you. Watch them under Home → History → Recordings. Anything shown on screen, such as a password printed by a command, ends up in the file.</span
              ></span
            >
          </label>

          <fieldset class="mt-2 border-t border-line pt-4">
            <legend class="{label} pt-4">Compatibility</legend>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class={label} for="{id}-bs">Backspace key sends</label>
                <select id="{id}-bs" class="{field} w-full" value={t.backspace} onchange={(e) => setP({ backspace: e.currentTarget.value })}>
                  <option value="del">ASCII DEL (^?, default)</option>
                  <option value="ctrl-h">Control-H (^H)</option>
                </select>
              </div>
              <div>
                <label class={label} for="{id}-del">Delete key sends</label>
                <select id="{id}-del" class="{field} w-full" value={t.delete} onchange={(e) => setP({ delete: e.currentTarget.value })}>
                  <option value="escape">Escape sequence (default)</option>
                  <option value="del">ASCII DEL (^?)</option>
                  <option value="ctrl-h">Control-H (^H)</option>
                </select>
              </div>
            </div>
            <span class={hint}>Change these only if Backspace or Delete type odd characters on an old server.</span>
          </fieldset>
        </div>
      {:else if section === 'general'}
        <fieldset class="mb-6">
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

        <fieldset class="mb-6">
          <legend class={label}>Open tabs</legend>
          <div class="flex gap-2">
            {#each [['left', 'In the left panel', 'sidebar'], ['top', 'In the title bar', 'tab']] as const as [value, name, icon] (value)}
              <label class="flex flex-1 cursor-pointer items-center justify-center gap-2 rounded-xl border px-3 py-2.5 text-[13px] {prefs.value.tabsPosition === value ? 'border-accent bg-accent/10' : 'border-line hover:bg-raised'}">
                <input type="radio" class="sr-only" name="{id}-tabs" {value} checked={prefs.value.tabsPosition === value} onchange={() => prefs.setTabsPosition(value)} />
                <Icon name={icon} size={15} />
                {name}
              </label>
            {/each}
          </div>
          <span class={hint}>In the left panel, open tabs are listed above your hosts; with the panel hidden they become a narrow strip of icons.</span>
        </fieldset>

        <div class="flex flex-col gap-4">
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={g.copyOnSelect} onchange={(e) => prefs.update({ copyOnSelect: e.currentTarget.checked })} />
            <span>Copy on select<span class={hint}>Selecting text with the mouse copies it to the clipboard.</span></span>
          </label>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={g.rightClickPaste} onchange={(e) => prefs.update({ rightClickPaste: e.currentTarget.checked })} />
            <span>Right-click pastes<span class={hint}>Like PuTTY and MobaXterm. Shift+right-click still opens the menu.</span></span>
          </label>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" checked={g.ctrlWheelZoom} onchange={(e) => prefs.update({ ctrlWheelZoom: e.currentTarget.checked })} />
            <span>{keys('Ctrl+mouse wheel')} zooms text</span>
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
            <input type="checkbox" class="mt-0.5 accent-accent" checked={g.confirmClose} onchange={(e) => prefs.update({ confirmClose: e.currentTarget.checked })} />
            <span>Ask before closing running sessions<span class={hint}>When closing a pane, tabs or the window.</span></span>
          </label>

          <fieldset class="flex flex-col gap-3 border-t border-line pt-4">
            <legend class="{label} pt-4">Connection history</legend>
            <label class={check}>
              <input type="checkbox" class="mt-0.5 accent-accent" checked={prefs.value.history?.enabled ?? true} onchange={(e) => prefs.setHistory({ enabled: e.currentTarget.checked })} />
              <span
                >Keep a history of connections<span class={hint}>Host, time, how long and whether it worked, shown under Home → History. Never what you typed or what was on screen.</span></span
              >
            </label>
            <div class="flex items-center gap-2 text-[13px]">
              <label for="{id}-hdays">Keep for</label>
              <input
                id="{id}-hdays"
                type="number"
                class="{field} w-20 text-center"
                min="1"
                max="3650"
                value={prefs.value.history?.days ?? 90}
                oninput={(e) => {
                  const v = Number(e.currentTarget.value)
                  if (v >= 1 && v <= 3650) prefs.setHistory({ days: Math.round(v) })
                }}
              />
              <span class="text-subtle">days, at most</span>
              <label class="sr-only" for="{id}-hmax">Most entries kept</label>
              <input
                id="{id}-hmax"
                type="number"
                class="{field} w-24 text-center"
                min="50"
                max="100000"
                step="50"
                value={prefs.value.history?.max ?? 1000}
                oninput={(e) => {
                  const v = Number(e.currentTarget.value)
                  if (v >= 50 && v <= 100000) prefs.setHistory({ max: Math.round(v) })
                }}
              />
              <span class="text-subtle">entries</span>
            </div>
          </fieldset>

          <div>
            <label class={label} for="{id}-shell">Shell for local tabs</label>
            <select
              id="{id}-shell"
              class="{field} w-64 font-mono"
              value={g.shell}
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
          <p class={hint}>From ~/.ssh/config, CSV/TSV, Excel (.xlsx), a TuxShell export, MobaXterm, Remmina, PuTTY or an Ansible inventory. You see every host before it's saved. Passwords are imported only if you allow saving them (Security).</p>
          <button class="mt-3 rounded-lg bg-accent px-3 py-1.5 text-xs font-medium text-on-accent hover:bg-accent/90" onclick={onImport}>Import hosts…</button>
          <button
            class="mt-3 ml-2 rounded-lg border border-line px-3 py-1.5 text-xs text-muted hover:bg-raised hover:text-fg"
            onclick={async () => {
              const path = await HostIO.SaveTemplate().catch(() => '')
              if (path) templateSaved = path
            }}>Download CSV template</button
          >
          {#if templateSaved}<p role="status" class="mt-2 truncate text-xs text-success" title={templateSaved}>Template saved to {templateSaved}</p>{/if}
        </section>
        <ExportHosts />
      {:else if section === 'security'}
        <h3 class="sr-only">Security</h3>
        <section aria-labelledby="{id}-how">
          <h4 id="{id}-how" class="text-[13px] font-medium">How TuxShell logs you in</h4>
          <ul class="mt-2 flex flex-col gap-2 text-[13px]">
            <li class="rounded-lg border border-line bg-ground px-3 py-2">
              <span class="font-medium">SSH keys or your ssh agent</span> <span class="ml-1 rounded bg-success/15 px-1.5 py-0.5 text-[11px] text-success">safest</span>
              <span class={hint}>No password is needed or stored. Set a key file on the host, or use your agent.</span>
            </li>
            <li class="rounded-lg border border-line bg-ground px-3 py-2">
              <span class="font-medium">1Password</span>
              <span class={hint}>The password stays in your vault; TuxShell asks 1Password when you connect. Put a reference in a host's settings.</span>
            </li>
            <li class="rounded-lg border border-line bg-ground px-3 py-2">
              <span class="font-medium">Saved password</span>
              <span class={hint}>For servers that only take passwords, if you don't use 1Password. Stored on this computer only.</span>
            </li>
          </ul>
        </section>

        <section aria-labelledby="{id}-op" class="mt-6 border-t border-line pt-5">
          <h4 id="{id}-op" class="text-[13px] font-medium">1Password</h4>
          {#if onePassword.status}
            {@const op = onePassword.status}
            <p class="mt-2 text-xs {op.version ? 'text-success' : 'text-warning'}" role="status">
              {#if op.version}✓ 1Password CLI {op.version} found.{:else if op.installed}The 1Password CLI didn't run: {op.error}{:else}The 1Password CLI (op) isn't installed.{/if}
            </p>
            {#if !op.version}
              <ol class="mt-2 list-decimal pl-5 text-xs leading-relaxed text-muted">
                <li>Install the 1Password app and the 1Password CLI from 1Password's own repository
                  (<button class="text-accent-text hover:underline" onclick={() => void Browser.OpenURL('https://developer.1password.com/docs/cli/get-started/')}>how to install</button>).</li>
                <li>In the 1Password app, turn on Settings → Developer → <span class="font-medium">Integrate with 1Password CLI</span>.</li>
                <li>Come back here; the status updates when you open this page.</li>
              </ol>
            {:else}
              <p class="mt-1 text-xs text-subtle">
                If a reference doesn't work, check that <span class="font-medium">Integrate with 1Password CLI</span> is on in the 1Password app (Settings → Developer).
              </p>
            {/if}
          {/if}
          <label class="{check} mt-4">
            <input
              type="checkbox"
              class="mt-0.5 accent-accent"
              checked={prefs.value.onePasswordAgent}
              onchange={(e) => prefs.setOnePasswordAgent(e.currentTarget.checked)}
            />
            <span
              >Use the 1Password SSH agent<span class={hint}
                >TuxShell's ssh uses the SSH keys in 1Password (~/.1password/agent.sock), also through jump hosts. Hosts that set IdentityAgent in
                ~/.ssh/config keep it. Takes effect for new connections.</span
              ></span
            >
          </label>
          {#if onePassword.status}
            {@const op = onePassword.status}
            {#if prefs.value.onePasswordAgent && !op.agent}
              <p role="alert" class="mt-2 text-xs text-warning">
                The agent isn't running. Open the 1Password app and turn on Settings → Developer → <span class="font-medium">Use the SSH agent</span>.
              </p>
            {:else if op.configUsesAgent}
              <p class="mt-2 text-xs text-subtle">Your ~/.ssh/config already uses the 1Password agent for some hosts; that keeps working either way.</p>
            {/if}
          {/if}
        </section>

        <section aria-labelledby="{id}-save" class="mt-6 border-t border-line pt-5">
          <h4 id="{id}-save" class="sr-only">Saved passwords</h4>
          <label class={check}>
            <input
              type="checkbox"
              class="mt-0.5 accent-accent"
              checked={prefs.value.savePasswords}
              disabled={!logins.status.available && !prefs.value.savePasswords}
              onchange={(e) => {
                const on = e.currentTarget.checked
                void setSavePasswords(on)
                if (!on && logins.status.saved > 0) e.currentTarget.checked = true
              }}
            />
            <span
              >Allow saving passwords on this computer<span class={hint}
                >Passwords are stored encrypted in your system keyring (GNOME Keyring or KWallet) and unlocked when you log in to your desktop. Anyone using
                your logged-in session can use them. They're never put in exports, logs or ssh's command line.</span
              ></span
            >
          </label>
          {#if !logins.status.available}
            <p role="alert" class="mt-2 text-xs text-warning">No system keyring found. Install and unlock GNOME Keyring or KWallet to save passwords.</p>
          {/if}
          {#if prefs.value.savePasswords}
            <p class="mt-3 text-xs text-muted">
              {logins.status.saved === 0 ? 'No passwords saved yet.' : `${logins.status.saved} ${logins.status.saved === 1 ? 'password' : 'passwords'} saved.`}
              When ssh asks for a password, TuxShell shows a box where you can choose to remember it. You can also set one in a host's settings.
            </p>
          {/if}
          {#if confirmForget}
            <div role="alertdialog" aria-labelledby="{id}-forget" class="mt-3 rounded-lg border border-warning/40 bg-warning/10 p-3 text-xs">
              <p id="{id}-forget" class="text-fg">Turn off saved passwords and delete the {logins.status.saved} saved {logins.status.saved === 1 ? 'password' : 'passwords'}?</p>
              <div class="mt-2 flex gap-2">
                <button class="rounded-lg px-2.5 py-1.5 text-muted hover:bg-raised hover:text-fg" onclick={() => (confirmForget = false)}>Keep them</button>
                <button class="rounded-lg bg-danger px-2.5 py-1.5 font-medium text-white hover:bg-danger/90" onclick={forgetAndTurnOff}>Delete and turn off</button>
              </div>
              {#if forgetError}<p role="alert" class="mt-2 text-warning">{forgetError}</p>{/if}
            </div>
          {/if}
        </section>
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
            {#each shortcuts as [combo, action] (combo)}
              <tr class="border-b border-line last:border-0">
                <td class="py-2 pr-4 whitespace-nowrap"><kbd class="rounded border border-line bg-ground px-1.5 py-0.5 font-mono text-[11px]">{keys(combo)}</kbd></td>
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
  {@const palette = profileTheme(t, theme.resolved)}
  <figure class="mt-6">
    <figcaption class={label}>Preview</figcaption>
    <div
      class="overflow-hidden rounded-xl border border-line px-4 py-3"
      style:background={palette.background}
      style:color={palette.foreground}
      style:font-family={prefs.fontFamilyOf(t)}
      style:font-size="{prefs.fontSizeOf(t)}px"
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
