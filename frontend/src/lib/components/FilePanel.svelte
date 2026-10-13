<script lang="ts">
  import { onMount } from 'svelte'
  import { Clipboard, Events } from '@wailsio/runtime'
  import * as Files from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/files/service'
  import type { Entry, Listing, Location } from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/files/models'
  import { errorMessage, hostStore } from '../hosts.svelte'
  import type { Pane } from '../workspace.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    /** The focused terminal pane; files are browsed on its host, or on this computer without one. */
    pane: Pane | undefined
    /** Types a command line into the pane's shell. */
    onRunInPane: (paneId: number, line: string) => void
    onClose: () => void
  }
  let { pane, onRunInPane, onClose }: Props = $props()

  let listing = $state<Listing | null>(null)
  let error = $state<string | null>(null)
  let loading = $state(false)
  let pathInput = $state('')
  let notice = $state<{ text: string; tone: 'ok' | 'warn' } | null>(null)
  let busy = $state<string | null>(null)
  let selected = $state<string | null>(null)
  // Last folder per pane, so switching panes and back keeps your place.
  // Without a pane (e.g. on Home) the panel shows this computer under LOCAL.
  const LOCAL = -1
  const paneKey = $derived(pane?.id ?? LOCAL)
  const lastDir = new Map<number, string>()
  // The folder a pane's first listing opened in (its home), for "~" paths.
  const homeDir = new Map<number, string>()

  const location = $derived<Location | null>(
    !pane || pane.source.kind === 'shell'
      ? { kind: 'local', hostId: 0, user: '', target: '' }
      : pane.source.kind === 'host'
        ? { kind: 'host', hostId: pane.source.hostId, user: pane.loginUser, target: '' }
        : { kind: 'target', hostId: 0, user: '', target: pane.source.target },
  )
  const where = $derived(
    !pane || pane.source.kind === 'shell' ? 'This computer' : pane.source.kind === 'host' ? (hostStore.get(pane.source.hostId)?.name ?? pane.title) : pane.source.target,
  )

  // Reload when the focused pane (or its login) changes.
  let shownFor = ''
  $effect(() => {
    const key = pane ? `${pane.id}:${pane.loginUser}` : 'local'
    if (key === shownFor) return
    shownFor = key
    listing = null
    error = null
    void open(lastDir.get(paneKey) ?? '')
  })

  onMount(() =>
    Events.On('files:dropped', (ev) => {
      if (!location || !listing) return
      void transfer(`Uploading ${ev.data.names?.length ?? 0} file(s)…`, () => Files.UploadDropped(ev.data.id, location!, listing!.path))
    }),
  )

  async function open(dir: string) {
    if (!location) return
    const forPane = paneKey
    loading = true
    error = null
    try {
      const l = await Files.List(location, dir)
      if (paneKey !== forPane) return // focus moved while loading
      listing = l
      pathInput = l.path
      selected = null
      lastDir.set(forPane, l.path)
      if (dir === '' && !homeDir.has(forPane)) homeDir.set(forPane, l.path)
    } catch (e) {
      error = errorMessage(e)
    } finally {
      loading = false
    }
  }

  const join = (dir: string, name: string) => (dir === '/' ? `/${name}` : `${dir}/${name}`)
  const parent = (dir: string) => dir.replace(/\/[^/]+\/?$/, '') || '/'

  async function download(e: Entry) {
    if (!location || !listing) return
    const remote = join(listing.path, e.name)
    busy = `Downloading ${e.name}…`
    notice = null
    try {
      const saved = await Files.Download(location, remote)
      notice = { text: `Saved to ${saved}`, tone: 'ok' }
    } catch (err) {
      notice = { text: errorMessage(err), tone: 'warn' }
    } finally {
      busy = null
    }
  }

  async function transfer(label: string, run: () => Promise<{ uploaded: string[] | null; failed: string[] | null }>) {
    busy = label
    notice = null
    try {
      const res = await run()
      const up = res.uploaded ?? []
      const failed = res.failed ?? []
      if (up.length || failed.length) {
        notice = failed.length
          ? { text: `Uploaded ${up.length}. Skipped: ${failed.join('; ')}`, tone: 'warn' }
          : { text: `Uploaded ${up.join(', ')}`, tone: 'ok' }
      }
      if (listing) await open(listing.path)
    } catch (err) {
      notice = { text: errorMessage(err), tone: 'warn' }
    } finally {
      busy = null
    }
  }

  function upload() {
    if (location && listing) void transfer('Uploading…', () => Files.UploadPick(location!, listing!.path))
  }

  // Single-quote for a POSIX shell.
  const shQuote = (s: string) => `'${s.replaceAll("'", `'\\''`)}'`

  function cdHere(dir: string) {
    if (pane) onRunInPane(pane.id, `cd -- ${shQuote(dir)}`)
  }

  function activate(e: Entry) {
    if (!listing) return
    if (e.type === 'dir' || e.linkDir) void open(join(listing.path, e.name))
    else if (e.type === 'file') void download(e)
  }

  function size(n: number) {
    if (n < 1024) return `${n} B`
    const units = ['KB', 'MB', 'GB', 'TB']
    let v = n / 1024
    let i = 0
    while (v >= 1024 && i < units.length - 1) {
      v /= 1024
      i++
    }
    return `${v < 10 ? v.toFixed(1) : Math.round(v)} ${units[i]}`
  }

  // ~ for the home folder, like the prompt.
  function shortPath(p: string) {
    const home = homeDir.get(paneKey)
    return home && (p === home || p.startsWith(home + '/')) ? '~' + p.slice(home.length) : p
  }

  const dateFmt = new Intl.DateTimeFormat(undefined, { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })

  const iconBtn = 'grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg disabled:opacity-40'
  const rowBtn = 'grid size-6 place-items-center rounded-md text-subtle hover:bg-line hover:text-fg'
</script>

<aside
  id="file-panel"
  data-file-drop-target
  aria-label="Files"
  class="file-panel flex w-80 shrink-0 flex-col overflow-hidden rounded-panel border border-line bg-panel"
>
  <div class="flex items-center gap-1 pt-3.5 pr-2.5 pb-1 pl-4">
    <h2 class="font-display text-base font-semibold">Files</h2>
    <span class="ml-1.5 min-w-0 flex-1 truncate font-mono text-xs text-subtle" title={listing?.path}>{listing ? shortPath(listing.path) : ''}</span>
    <button class={iconBtn} aria-label="Upload files" title="Upload files (or drop them here)" disabled={!listing || !!busy} onclick={upload}>
      <Icon name="upload" size={15} />
    </button>
    <button class={iconBtn} aria-label="Refresh" title="Refresh" disabled={loading} onclick={() => open(listing?.path ?? '')}>
      <Icon name="refresh" size={15} />
    </button>
    <button class={iconBtn} aria-label="Close files" onclick={onClose}>
      <Icon name="x" size={14} />
    </button>
  </div>
  {#if where}<p class="truncate px-4 pb-2 text-xs text-subtle">{where}</p>{/if}

  <form
      class="flex items-center gap-1 px-3 pb-2"
      onsubmit={(e) => {
        e.preventDefault()
        void open(pathInput.trim())
      }}
    >
      <button type="button" class={iconBtn} aria-label="Parent folder" title="Parent folder" disabled={!listing || listing.path === '/'} onclick={() => listing && open(parent(listing.path))}>
        <Icon name="up" size={15} />
      </button>
      <label class="min-w-0 flex-1">
        <span class="sr-only">Folder path</span>
        <input
          bind:value={pathInput}
          class="h-7 w-full rounded-lg border border-transparent bg-transparent px-2 font-mono text-xs text-muted placeholder:text-subtle hover:border-line focus:border-accent focus:bg-ground focus:text-fg focus:outline-none"
          placeholder="/path/to/folder"
          autocomplete="off"
          spellcheck="false"
        />
      </label>
      {#if listing && pane}
        <button type="button" class="h-7 shrink-0 rounded-lg px-2 text-xs text-muted hover:bg-raised hover:text-fg" title="Type cd to this folder in the terminal" onclick={() => listing && cdHere(listing.path)}>cd here</button>
      {/if}
    </form>

    <div class="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
      {#if error}
        <div role="alert" class="m-2 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">
          {error}
          <button class="mt-1.5 block underline" onclick={() => open(listing?.path ?? '')}>Try again</button>
        </div>
      {:else if loading && !listing}
        <p class="px-3 pt-8 text-center text-[13px] text-muted">Loading…</p>
      {:else if listing && !listing.entries?.length}
        <p class="px-3 pt-8 text-center text-[13px] text-muted">This folder is empty.</p>
      {/if}

      {#if listing && !error}
        <ul aria-label="Folder contents" class:opacity-60={loading}>
          {#each listing.entries ?? [] as e (e.name)}
            {@const isDir = e.type === 'dir' || e.linkDir}
            <li class="group relative">
              <button
                class="flex w-full items-center gap-3 rounded-lg py-2 pr-3 pl-2.5 text-left hover:bg-raised {selected === e.name ? 'bg-raised' : ''}"
                title="{isDir ? `Open ${e.name}` : `Download ${e.name}`} · {dateFmt.format(new Date(e.modTime))} · {e.mode}"
                onclick={() => (selected = e.name)}
                ondblclick={() => activate(e)}
                onkeydown={(ev) => ev.key === 'Enter' && (ev.preventDefault(), activate(e))}
              >
                <span class={isDir ? 'text-accent' : 'text-subtle'}><Icon name={isDir ? 'folder' : 'file'} size={15} /></span>
                <span class="min-w-0 flex-1 truncate text-[13px] {e.type === 'link' ? 'italic' : ''}">{e.name}</span>
                <span class="shrink-0 font-mono text-[11px] text-subtle group-focus-within:opacity-0 group-hover:opacity-0">{isDir ? 'dir' : size(e.size)}</span>
              </button>
              <div class="absolute top-1/2 right-1.5 flex -translate-y-1/2 gap-0.5 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100">
                {#if isDir && pane}
                  <button class={rowBtn} aria-label="cd to {e.name} in the terminal" title="cd here" onclick={() => listing && cdHere(join(listing.path, e.name))}>
                    <Icon name="terminal" size={13} />
                  </button>
                {:else if e.type === 'file'}
                  <button class={rowBtn} aria-label="Download {e.name}" title="Download to Downloads" disabled={!!busy} onclick={() => download(e)}>
                    <Icon name="download" size={13} />
                  </button>
                {/if}
                <button class={rowBtn} aria-label="Copy path of {e.name}" title="Copy path" onclick={() => listing && Clipboard.SetText(join(listing.path, e.name))}>
                  <Icon name="clipboard" size={13} />
                </button>
              </div>
            </li>
          {/each}
        </ul>
        {#if listing.truncated}
          <p class="px-3 py-2 text-xs text-subtle">Showing the first 5,000 items.</p>
        {/if}
      {/if}
    </div>

    <div class="px-3 pb-3 text-xs" aria-live="polite">
      {#if busy}
        <p class="px-1 pb-2 text-muted">{busy}</p>
      {:else if notice}
        <p class="px-1 pb-2 break-all {notice.tone === 'ok' ? 'text-success' : 'text-warning'}">{notice.text}</p>
      {/if}
      <button
        class="drop-zone w-full rounded-xl border border-dashed border-line px-3 py-4 text-center text-[13px] text-subtle hover:border-subtle hover:text-muted disabled:opacity-50"
        disabled={!listing || !!busy}
        title="Double-click a folder to open it, a file to download it"
        onclick={upload}
      >
        Drop to upload · or click to pick files
      </button>
    </div>
</aside>

<style>
  /* Wails adds this class while files are dragged over the panel. */
  :global(.file-panel.file-drop-target-active) {
    outline: 2px dashed var(--color-accent);
    outline-offset: -4px;
  }
  :global(.file-panel.file-drop-target-active) .drop-zone {
    border-color: var(--color-accent);
    color: var(--color-accent);
  }
</style>
