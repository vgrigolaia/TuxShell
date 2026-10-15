<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Clipboard, Events } from '@wailsio/runtime'
  import * as Files from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/files/service'
  import type { Entry, Listing, Location } from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/files/models'
  import { errorMessage, hostStore } from '../hosts.svelte'
  import type { Pane } from '../workspace.svelte'
  import { edits } from '../edits.svelte'
  import Icon from './Icon.svelte'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import PermissionsDialog from './PermissionsDialog.svelte'

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
  // Selected names; Ctrl+click adds and removes.
  let selected = $state<string[]>([])
  let menu = $state<{ x: number; y: number; entry: Entry } | null>(null)
  let renaming = $state<string | null>(null)
  let creating = $state(false)
  let confirmDelete = $state<Entry[] | null>(null)
  let permissions = $state<Entry | null>(null)
  // Last folder per pane, so switching panes and back keeps your place.
  // Without a pane (e.g. on Home) the panel shows this computer under LOCAL.
  const LOCAL = -1
  const paneKey = $derived(pane?.id ?? LOCAL)
  const lastDir = new Map<number, string>()
  // The folder a pane's first listing opened in (its home), for "~" paths.
  const homeDir = new Map<number, string>()

  // Follow the focused terminal's folder (its prompt reports it). Remembered per viewer.
  let follow = $state(readFollow())
  function readFollow() {
    try {
      return localStorage.getItem('tuxshell.files.follow') === '1'
    } catch {
      return false
    }
  }
  function setFollow(on: boolean) {
    follow = on
    try {
      localStorage.setItem('tuxshell.files.follow', on ? '1' : '0')
    } catch {
      // not remembered
    }
    if (on && pane?.cwd) void open(pane.cwd)
  }

  const location = $derived<Location | null>(
    // A remote desktop tab browses this computer (its shared folders are local).
    !pane || pane.source.kind === 'shell' || pane.source.kind === 'rdp'
      ? { kind: 'local', hostId: 0, user: '', target: '' }
      : pane.source.kind === 'host'
        ? { kind: 'host', hostId: pane.source.hostId, user: pane.loginUser, target: '' }
        : { kind: 'target', hostId: 0, user: '', target: pane.source.target },
  )
  const where = $derived(
    !pane || pane.source.kind === 'shell' || pane.source.kind === 'rdp'
      ? 'This computer'
      : pane.source.kind === 'host'
        ? (hostStore.get(pane.source.hostId)?.name ?? pane.title)
        : pane.source.target,
  )

  // Reload when the focused pane (or its login) changes.
  let shownFor = ''
  $effect(() => {
    const key = pane ? `${pane.id}:${pane.loginUser}` : 'local'
    if (key === shownFor) return
    shownFor = key
    listing = null
    error = null
    void open((follow && pane?.cwd) || lastDir.get(paneKey) || '')
  })

  // While following, the list moves with the terminal's cd.
  let followed = ''
  $effect(() => {
    const cwd = follow && pane ? pane.cwd : ''
    if (!cwd || cwd === followed) return
    followed = cwd
    if (listing && (cwd === listing.path || cwd === shortPath(listing.path))) return
    void open(cwd)
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
      selected = []
      renaming = null
      creating = false
      lastDir.set(forPane, l.path)
      if (dir === '' && !homeDir.has(forPane)) homeDir.set(forPane, l.path)
    } catch (e) {
      error = errorMessage(e)
    } finally {
      loading = false
    }
  }

  const reload = () => open(listing?.path ?? '')
  const join = (dir: string, name: string) => (dir === '/' ? `/${name}` : `${dir}/${name}`)
  const parent = (dir: string) => dir.replace(/\/[^/]+\/?$/, '') || '/'
  const isDir = (e: Entry) => e.type === 'dir' || e.linkDir

  /** Runs a change, then shows its result and reloads the folder. */
  async function change(label: string, run: () => Promise<unknown>, done?: string) {
    busy = label
    notice = null
    try {
      await run()
      if (done) notice = { text: done, tone: 'ok' }
      await reload()
    } catch (err) {
      notice = { text: errorMessage(err), tone: 'warn' }
    } finally {
      busy = null
    }
  }

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

  async function edit(e: Entry) {
    if (!location || !listing) return
    busy = `Opening ${e.name}…`
    notice = null
    try {
      await edits.open(location, join(listing.path, e.name))
      notice = { text: `Opened ${e.name}. Save it in the editor, then upload it below.`, tone: 'ok' }
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

  // Enter and the blur that follows it both end an edit; only the first counts.
  function rename(e: Entry, to: string) {
    if (renaming !== e.name) return
    renaming = null
    to = to.trim()
    if (!location || !listing || !to || to === e.name) return
    void change(`Renaming ${e.name}…`, () => Files.Rename(location!, listing!.path, e.name, to))
  }

  function makeDir(name: string) {
    if (!creating) return
    creating = false
    name = name.trim()
    if (!location || !listing || !name) return
    void change(`Creating ${name}…`, () => Files.MakeDir(location!, listing!.path, name))
  }

  async function remove(list: Entry[]) {
    if (!location || !listing) return
    const names = list.map((e) => e.name)
    // The dialog shows the error and stays open if this throws.
    await Files.Delete(location, listing.path, names)
    notice = { text: names.length === 1 ? `Deleted ${names[0]}` : `Deleted ${names.length} items`, tone: 'ok' }
    await reload()
  }

  function deleteMessage(list: Entry[]) {
    if (list.length > 1) {
      const dirs = list.filter(isDir).length
      return `Delete ${list.length} items${dirs ? `, including ${dirs === 1 ? 'a folder' : `${dirs} folders`} and everything in ${dirs === 1 ? 'it' : 'them'}` : ''}? This can't be undone.`
    }
    const e = list[0]
    if (e.type === 'link') return `Delete the link “${e.name}”? What it points to is kept.`
    return isDir(e) ? `Delete the folder “${e.name}” and everything in it? This can't be undone.` : `Delete “${e.name}”? This can't be undone.`
  }

  // Single-quote for a POSIX shell.
  const shQuote = (s: string) => `'${s.replaceAll("'", `'\\''`)}'`

  function cdHere(dir: string) {
    if (pane) onRunInPane(pane.id, `cd -- ${shQuote(dir)}`)
  }

  function activate(e: Entry) {
    if (!listing) return
    if (isDir(e)) void open(join(listing.path, e.name))
    else if (e.type === 'file') void download(e)
  }

  function select(e: Entry, ev: MouseEvent) {
    if (ev.ctrlKey || ev.metaKey) selected = selected.includes(e.name) ? selected.filter((n) => n !== e.name) : [...selected, e.name]
    else selected = [e.name]
  }

  /** The entries an action applies to: the selection when e is in it, else e. */
  function targets(e: Entry): Entry[] {
    if (!listing || !selected.includes(e.name)) return [e]
    return (listing.entries ?? []).filter((x) => selected.includes(x.name))
  }

  function onRowKey(e: Entry, ev: KeyboardEvent) {
    if (ev.key === 'Enter') {
      ev.preventDefault()
      activate(e)
    } else if (ev.key === 'F2') {
      ev.preventDefault()
      void startRename(e)
    } else if (ev.key === 'Delete') {
      ev.preventDefault()
      confirmDelete = targets(e)
    } else if (ev.key === 'ContextMenu' || (ev.shiftKey && ev.key === 'F10')) {
      ev.preventDefault()
      const r = (ev.currentTarget as HTMLElement).getBoundingClientRect()
      menu = { x: r.left + 24, y: r.bottom, entry: e }
    }
  }

  async function startRename(e: Entry) {
    renaming = e.name
    await tick()
    const input = document.getElementById('file-rename') as HTMLInputElement | null
    if (!input) return
    input.focus()
    // Select the name without its extension, like file managers do.
    const dot = isDir(e) ? -1 : e.name.lastIndexOf('.')
    input.setSelectionRange(0, dot > 0 ? dot : e.name.length)
  }

  async function startCreate() {
    creating = true
    await tick()
    document.getElementById('file-create')?.focus()
  }

  function menuItems(e: Entry): MenuItem[] {
    const many = targets(e)
    const one = many.length === 1
    const path = listing ? join(listing.path, e.name) : ''
    const items: MenuItem[] = []
    if (one && isDir(e)) {
      items.push({ label: 'Open', icon: 'folder', run: () => activate(e) })
      if (pane) items.push({ label: 'cd here in the terminal', icon: 'terminal', run: () => cdHere(path) })
    }
    if (one && e.type === 'file') {
      items.push(
        { label: 'Edit', icon: 'edit', run: () => void edit(e) },
        { label: 'Download', icon: 'download', run: () => void download(e) },
      )
    }
    if (one) {
      items.push(
        { label: 'Rename…', icon: 'tag', hint: 'F2', separator: items.length > 0, run: () => void startRename(e) },
        { label: 'Permissions…', icon: 'lock', disabled: e.type === 'link', run: () => (permissions = e) },
        { label: 'Copy path', icon: 'clipboard', run: () => void Clipboard.SetText(path) },
      )
    }
    items.push({ label: one ? 'Delete…' : `Delete ${many.length} items…`, icon: 'trash', hint: 'Delete', danger: true, separator: true, run: () => (confirmDelete = many) })
    return items
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

  const iconBtn = 'grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg disabled:opacity-40 aria-pressed:bg-accent-chip aria-pressed:text-accent-ink'
  const rowBtn = 'grid size-6 place-items-center rounded-md text-subtle hover:bg-line hover:text-fg'
  const nameInput = 'h-7 min-w-0 flex-1 rounded-md border border-accent bg-ground px-2 text-[13px] text-fg focus:outline-none'
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
    {#if pane}
      <button
        class={iconBtn}
        aria-label="Follow the terminal's folder"
        aria-pressed={follow}
        title={follow ? "Following the terminal's folder (click to stop)" : "Follow the terminal's folder"}
        onclick={() => setFollow(!follow)}
      >
        <Icon name="follow" size={15} />
      </button>
    {/if}
    <button class={iconBtn} aria-label="New folder" title="New folder" disabled={!listing || !!busy} onclick={startCreate}>
      <Icon name="folderPlus" size={15} />
    </button>
    <button class={iconBtn} aria-label="Upload files" title="Upload files (or drop them here)" disabled={!listing || !!busy} onclick={upload}>
      <Icon name="upload" size={15} />
    </button>
    <button class={iconBtn} aria-label="Refresh" title="Refresh" disabled={loading} onclick={reload}>
      <Icon name="refresh" size={15} />
    </button>
    <button class={iconBtn} aria-label="Close files" onclick={onClose}>
      <Icon name="x" size={14} />
    </button>
  </div>
  {#if where}<p class="truncate px-4 pb-2 text-xs text-subtle">{where}</p>{/if}
  {#if follow && pane && !pane.cwd}
    <p class="mx-3 mb-2 rounded-lg bg-raised px-2.5 py-1.5 text-xs text-muted">This terminal's prompt doesn't report its folder yet. Press Enter in it or run cd; some prompts never do.</p>
  {/if}

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
        <button class="mt-1.5 block underline" onclick={reload}>Try again</button>
      </div>
    {:else if loading && !listing}
      <p class="px-3 pt-8 text-center text-[13px] text-muted">Loading…</p>
    {:else if listing && !listing.entries?.length && !creating}
      <p class="px-3 pt-8 text-center text-[13px] text-muted">This folder is empty.</p>
    {/if}

    {#if listing && !error}
      <ul aria-label="Folder contents" class:opacity-60={loading}>
        {#if creating}
          <li class="flex items-center gap-3 py-1.5 pr-2 pl-2.5">
            <span class="text-accent"><Icon name="folder" size={15} /></span>
            <input
              id="file-create"
              class={nameInput}
              aria-label="New folder name"
              placeholder="Folder name"
              spellcheck="false"
              onkeydown={(ev) => {
                if (ev.key === 'Enter') makeDir(ev.currentTarget.value)
                if (ev.key === 'Escape') creating = false
              }}
              onblur={(ev) => makeDir(ev.currentTarget.value)}
            />
          </li>
        {/if}
        {#each listing.entries ?? [] as e (e.name)}
          {@const dir = isDir(e)}
          {@const isSelected = selected.includes(e.name)}
          <li class="group relative">
            {#if renaming === e.name}
              <div class="flex items-center gap-3 py-1.5 pr-2 pl-2.5">
                <span class={dir ? 'text-accent' : 'text-subtle'}><Icon name={dir ? 'folder' : 'file'} size={15} /></span>
                <input
                  id="file-rename"
                  class={nameInput}
                  aria-label="New name for {e.name}"
                  value={e.name}
                  spellcheck="false"
                  onkeydown={(ev) => {
                    if (ev.key === 'Enter') rename(e, ev.currentTarget.value)
                    if (ev.key === 'Escape') renaming = null
                  }}
                  onblur={(ev) => rename(e, ev.currentTarget.value)}
                />
              </div>
            {:else}
              <button
                class="flex w-full items-center gap-3 rounded-lg py-2 pr-3 pl-2.5 text-left hover:bg-raised {isSelected ? 'bg-raised ring-1 ring-accent-line' : ''}"
                aria-pressed={isSelected}
                title="{dir ? `Open ${e.name}` : `Download ${e.name}`} · {dateFmt.format(new Date(e.modTime))} · {e.mode} · right-click for more"
                onclick={(ev) => select(e, ev)}
                ondblclick={() => activate(e)}
                onkeydown={(ev) => onRowKey(e, ev)}
                oncontextmenu={(ev) => {
                  ev.preventDefault()
                  if (!selected.includes(e.name)) selected = [e.name]
                  menu = { x: ev.clientX, y: ev.clientY, entry: e }
                }}
              >
                <span class={dir ? 'text-accent' : 'text-subtle'}><Icon name={dir ? 'folder' : 'file'} size={15} /></span>
                <span class="min-w-0 flex-1 truncate text-[13px] {e.type === 'link' ? 'italic' : ''}">{e.name}</span>
                <span class="shrink-0 font-mono text-[11px] text-subtle group-focus-within:opacity-0 group-hover:opacity-0">{dir ? 'dir' : size(e.size)}</span>
              </button>
              <div class="absolute top-1/2 right-1.5 flex -translate-y-1/2 gap-0.5 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100">
                {#if dir && pane}
                  <button class={rowBtn} aria-label="cd to {e.name} in the terminal" title="cd here" onclick={() => listing && cdHere(join(listing.path, e.name))}>
                    <Icon name="terminal" size={13} />
                  </button>
                {:else if e.type === 'file'}
                  <button class={rowBtn} aria-label="Edit {e.name}" title="Edit in your desktop's editor" disabled={!!busy} onclick={() => edit(e)}>
                    <Icon name="edit" size={13} />
                  </button>
                  <button class={rowBtn} aria-label="Download {e.name}" title="Download to Downloads" disabled={!!busy} onclick={() => download(e)}>
                    <Icon name="download" size={13} />
                  </button>
                {/if}
                <button
                  class={rowBtn}
                  aria-label="More actions for {e.name}"
                  title="More"
                  onclick={(ev) => {
                    const r = ev.currentTarget.getBoundingClientRect()
                    menu = { x: r.left, y: r.bottom + 4, entry: e }
                  }}
                >
                  <span aria-hidden="true" class="text-sm leading-none">⋯</span>
                </button>
              </div>
            {/if}
          </li>
        {/each}
      </ul>
      {#if listing.truncated}
        <p class="px-3 py-2 text-xs text-subtle">Showing the first 5,000 items.</p>
      {/if}
    {/if}
  </div>

  {#if edits.list.length}
    <section aria-label="Files open for editing" class="mx-3 mb-2 rounded-xl border border-line bg-ground/60">
      <h3 class="px-3 pt-2 text-[10px] font-semibold tracking-wider text-subtle uppercase">Editing</h3>
      <ul class="divide-y divide-line">
        {#each edits.list as ed (ed.id)}
          {@const st = edits.state[ed.id] ?? {}}
          <li class="px-3 py-2 text-xs">
            <div class="flex items-center gap-2">
              <span class="min-w-0 flex-1 truncate font-medium" title={ed.path}>{ed.name}</span>
              {#if ed.changed && !st.conflict}
                <button class="rounded-md bg-accent px-2 py-1 font-medium text-on-accent hover:bg-accent/90 disabled:opacity-60" disabled={st.busy} onclick={() => edits.upload(ed.id)}>
                  {st.busy ? 'Uploading…' : 'Upload'}
                </button>
              {/if}
              <button class="rounded-md px-2 py-1 text-muted hover:bg-raised hover:text-fg" title="Stop editing and delete the local copy" onclick={() => edits.close(ed.id)}>Done</button>
            </div>
            <p class="mt-0.5 truncate {st.error || st.conflict ? 'text-warning' : ed.changed ? 'text-warning' : 'text-subtle'}" aria-live="polite">
              {#if st.conflict}
                Changed on the server since you opened it.
                <button class="ml-1 font-medium underline" onclick={() => edits.upload(ed.id, true)}>Overwrite</button>
                <button class="ml-1 underline" onclick={() => (edits.state[ed.id] = {})}>Cancel</button>
              {:else if st.error}
                {st.error}
              {:else if ed.changed}
                Saved in the editor · not uploaded yet
              {:else if st.saved}
                <span class="text-success">Uploaded</span>
              {:else}
                Open in your editor · {ed.path}
              {/if}
            </p>
          </li>
        {/each}
      </ul>
    </section>
  {/if}

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

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} label="File actions" items={menuItems(menu.entry)} onClose={() => (menu = null)} />
{/if}
{#if confirmDelete}
  {@const list = confirmDelete}
  <ConfirmDialog
    title={list.length === 1 ? (isDir(list[0]) ? 'Delete folder' : 'Delete file') : 'Delete items'}
    message={deleteMessage(list)}
    confirmLabel="Delete"
    onConfirm={() => remove(list)}
    onClose={() => (confirmDelete = null)}
  />
{/if}
{#if permissions && listing}
  {@const e = permissions}
  {@const dir = listing.path}
  <PermissionsDialog
    name={join(dir, e.name)}
    mode={e.mode}
    isDir={isDir(e)}
    onApply={async (mode, recursive) => {
      await Files.Chmod(location!, dir, e.name, mode, recursive)
      notice = { text: `Permissions of ${e.name} set to ${mode}${recursive ? ' (and everything inside)' : ''}`, tone: 'ok' }
      await reload()
    }}
    onClose={() => (permissions = null)}
  />
{/if}

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
