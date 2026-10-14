<script lang="ts">
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import * as History from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/hosts/historyservice'
  import type { Visit } from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/hosts/models'
  import { errorMessage, hostStore, type Host } from '../hosts.svelte'
  import { prefs } from '../settings.svelte'
  import HostBadge from './HostBadge.svelte'
  import Icon from './Icon.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import RecordingPlayer from './RecordingPlayer.svelte'
  import * as Recordings from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/recordings/service'
  import type { Recording } from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/recordings/models'

  interface Props {
    onConnect: (host: Host) => void
    onConnectTarget: (target: string) => void
    /** Opens Preferences (history settings are under General). */
    onPreferences: () => void
  }
  let { onConnect, onConnectTarget, onPreferences }: Props = $props()

  let visits = $state<Visit[]>([])
  let loaded = $state(false)
  let error = $state<string | null>(null)
  let query = $state('')
  let confirmClear = $state(false)
  let tab = $state<'connections' | 'recordings'>('connections')
  let recordings = $state<Recording[]>([])
  let playing = $state<Recording | null>(null)
  let confirmDeleteRec = $state<Recording | null>(null)

  async function loadRecordings() {
    try {
      recordings = (await Recordings.List()) ?? []
    } catch (e) {
      error = errorMessage(e)
    }
  }

  function mb(n: number) {
    return n < 1024 * 1024 ? `${Math.max(1, Math.round(n / 1024))} KB` : `${(n / 1024 / 1024).toFixed(1)} MB`
  }

  // Re-renders "open" durations.
  let now = $state(Date.now())

  async function load() {
    try {
      visits = (await History.List(query)) ?? []
      error = null
    } catch (e) {
      error = errorMessage(e)
    } finally {
      loaded = true
    }
  }

  onMount(() => {
    void load()
    void loadRecordings()
    const off = Events.On('history:changed', () => void load())
    const offRec = Events.On('terminal:recording', () => void loadRecordings())
    const tick = setInterval(() => (now = Date.now()), 30_000)
    return () => {
      off()
      offRec()
      clearInterval(tick)
    }
  })

  // Search as you type, a little after the last key.
  let timer: ReturnType<typeof setTimeout> | undefined
  function search(v: string) {
    query = v
    clearTimeout(timer)
    timer = setTimeout(() => void load(), 150)
  }

  async function remove(v: Visit) {
    await History.Delete(v.id)
    visits = visits.filter((x) => x.id !== v.id)
  }

  function reconnect(v: Visit) {
    const host = v.hostId ? hostStore.get(v.hostId) : undefined
    if (host) onConnect(host)
    else onConnectTarget(v.target)
  }

  const dayFmt = new Intl.DateTimeFormat(undefined, { weekday: 'long', month: 'long', day: 'numeric' })
  const timeFmt = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' })

  function dayLabel(ms: number) {
    const d = new Date(ms)
    const today = new Date()
    const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1)
    if (d.toDateString() === today.toDateString()) return 'Today'
    if (d.toDateString() === yesterday.toDateString()) return 'Yesterday'
    return dayFmt.format(d)
  }

  /** Visits grouped by day, newest first (the list already is). */
  const days = $derived.by(() => {
    const out: { label: string; visits: Visit[] }[] = []
    for (const v of visits) {
      const label = dayLabel(v.started)
      if (out.at(-1)?.label !== label) out.push({ label, visits: [] })
      out.at(-1)!.visits.push(v)
    }
    return out
  })

  function duration(ms: number) {
    const m = Math.round(ms / 60000)
    if (m < 1) return 'under a minute'
    if (m < 60) return `${m} min`
    const h = Math.floor(m / 60)
    return m % 60 ? `${h} h ${m % 60} min` : `${h} h`
  }

  const enabled = $derived(prefs.value.history?.enabled ?? true)
</script>

<main class="flex min-w-0 flex-1 flex-col gap-5 overflow-y-auto rounded-panel border border-line bg-ground px-7 py-6">
  <header class="flex flex-wrap items-center gap-3.5">
    <h1 class="font-display text-3xl font-bold tracking-[-0.03em]">History</h1>
    <div class="flex gap-1 rounded-full bg-panel p-1 shadow-[inset_0_0_0_1px_var(--color-line)]" role="tablist" aria-label="History">
      {#each [['connections', `Connections ${visits.length || ''}`], ['recordings', `Recordings ${recordings.length || ''}`]] as const as [t, name] (t)}
        <button
          role="tab"
          aria-selected={tab === t}
          class="h-8 rounded-full px-3.5 text-[13px] font-semibold {tab === t ? 'bg-raised text-fg' : 'text-muted hover:text-fg'}"
          onclick={() => (tab = t)}>{name.trim()}</button
        >
      {/each}
    </div>
    {#if tab === 'recordings'}
      <button class="ml-auto h-[42px] rounded-[13px] border border-strong bg-panel px-4 text-sm font-semibold hover:bg-raised" onclick={() => void Recordings.OpenFolder()}>Open folder</button>
    {:else}
    <button
      class="ml-auto h-[42px] rounded-[13px] border border-strong bg-panel px-4 text-sm font-semibold hover:bg-raised disabled:opacity-50"
      disabled={!visits.length && !query}
      onclick={() => (confirmClear = true)}>Clear history</button
    >
    {/if}
  </header>

  {#if tab === 'recordings'}
    <p class="-mt-2 max-w-2xl text-sm text-subtle">
      Terminal output you recorded (right-click a terminal → Start recording, or turn on "Record sessions automatically" in a profile). Only what was on screen is saved, never your keystrokes.
      The files are standard asciinema recordings.
    </p>
    {#if !recordings.length}
      <div class="flex flex-col items-center gap-3 rounded-[20px] bg-panel px-6 py-14 text-center shadow-[inset_0_0_0_1px_var(--color-line)]">
        <span class="grid size-12 place-items-center rounded-2xl bg-raised text-subtle"><Icon name="record" size={22} /></span>
        <p class="text-[15px] font-semibold">No recordings yet</p>
        <p class="max-w-sm text-sm text-subtle">Right-click a terminal and choose Start recording.</p>
      </div>
    {:else}
      <ul class="divide-y divide-line overflow-hidden rounded-[14px] border border-line bg-panel">
        {#each recordings as r (r.name)}
          <li class="group flex items-center gap-3.5 px-4 py-3">
            <span class="grid size-[34px] shrink-0 place-items-center rounded-[10px] bg-danger/15 text-danger-text"><Icon name="record" size={16} /></span>
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm font-semibold">{r.title || r.name}</p>
              <p class="truncate font-mono text-xs text-subtle">{r.name}</p>
            </div>
            <div class="shrink-0 text-right text-xs">
              <p class="text-text">{dayLabel(r.started)} · {timeFmt.format(new Date(r.started))}</p>
              <p class="text-subtle">{duration(r.duration * 1000)} · {mb(r.size)}</p>
            </div>
            <button class="flex h-8 shrink-0 items-center gap-1.5 rounded-lg border border-strong px-3 text-xs font-semibold hover:bg-raised" onclick={() => (playing = r)} aria-label="Play {r.title || r.name}">
              <Icon name="play" size={11} />Play
            </button>
            <button
              class="grid size-8 shrink-0 place-items-center rounded-lg text-subtle opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 hover:bg-raised hover:text-danger-text"
              aria-label="Delete recording {r.name}"
              title="Delete recording"
              onclick={() => (confirmDeleteRec = r)}
            >
              <Icon name="trash" size={13} />
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  {:else}
  <p class="-mt-2 max-w-2xl text-sm text-subtle">
    Which host you connected to, when, for how long and whether it worked. TuxShell never keeps what you typed or what was on screen.
    <button class="underline-offset-2 hover:text-fg hover:underline" onclick={onPreferences}>Settings</button>
  </p>

  {#if !enabled}
    <p class="rounded-xl border border-line bg-panel px-4 py-3 text-sm text-muted">
      History is turned off, so new connections aren't added.
      <button class="ml-1 font-medium text-accent-text hover:underline" onclick={() => prefs.setHistory({ enabled: true })}>Turn on</button>
    </p>
  {/if}

  <label class="flex h-11 max-w-xl items-center gap-2.5 rounded-[13px] border border-strong bg-panel px-3.5 focus-within:border-accent">
    <span class="text-subtle"><Icon name="search" size={15} /></span>
    <span class="sr-only">Search history</span>
    <input class="min-w-0 flex-1 bg-transparent text-sm placeholder:text-subtle focus:outline-none" placeholder="Host name or address" value={query} oninput={(e) => search(e.currentTarget.value)} />
  </label>

  {#if error}<p role="alert" class="text-sm text-warning">{error}</p>{/if}

  {#if loaded && !visits.length}
    <div class="flex flex-col items-center gap-3 rounded-[20px] bg-panel px-6 py-14 text-center shadow-[inset_0_0_0_1px_var(--color-line)]">
      <span class="grid size-12 place-items-center rounded-2xl bg-raised text-subtle"><Icon name="clock" size={22} /></span>
      <p class="text-[15px] font-semibold">{query ? 'Nothing matches' : 'No connections yet'}</p>
      <p class="max-w-sm text-sm text-subtle">{query ? 'Try another name or address.' : 'Connections you open show up here, so you can find and reopen them.'}</p>
    </div>
  {/if}

  {#each days as day (day.label)}
    <section aria-labelledby="day-{day.visits[0].id}">
      <h2 id="day-{day.visits[0].id}" class="mb-2 text-xs font-semibold tracking-wider text-subtle uppercase">{day.label}</h2>
      <ul class="divide-y divide-line overflow-hidden rounded-[14px] border border-line bg-panel">
        {#each day.visits as v (v.id)}
          {@const host = v.hostId ? hostStore.get(v.hostId) : undefined}
          <li class="group flex items-center gap-3.5 px-4 py-3">
            <HostBadge name={v.name} os={host?.os ?? ''} size={34} />
            <div class="min-w-0 flex-1">
              <p class="flex items-center gap-2 text-sm font-semibold">
                <span class="truncate">{v.name}</span>
                {#if v.hostId && !host}<span class="shrink-0 text-xs font-normal text-subtle">(deleted host)</span>{/if}
              </p>
              <p class="truncate font-mono text-xs text-subtle">{v.target}</p>
            </div>
            <div class="shrink-0 text-right text-xs">
              <p class="text-text">{timeFmt.format(new Date(v.started))}</p>
              <p class={v.result === 'failed' ? 'text-warning' : v.result === 'open' ? 'text-success' : 'text-subtle'}>
                {#if v.result === 'open'}
                  Open · {duration(now - v.started)}
                {:else if v.result === 'failed'}
                  ssh error · {duration(v.ended - v.started)}
                {:else}
                  {duration(v.ended - v.started)}
                {/if}
              </p>
            </div>
            <button class="h-8 shrink-0 rounded-lg border border-strong px-3 text-xs font-semibold hover:bg-raised" onclick={() => reconnect(v)} aria-label="Connect to {v.name} again">Connect</button>
            <button
              class="grid size-8 shrink-0 place-items-center rounded-lg text-subtle opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 hover:bg-raised hover:text-fg"
              aria-label="Remove this entry"
              title="Remove from history"
              onclick={() => remove(v)}
            >
              <Icon name="x" size={13} />
            </button>
          </li>
        {/each}
      </ul>
    </section>
  {/each}
  {/if}
</main>

{#if playing}
  <RecordingPlayer name={playing.name} title={playing.title} onClose={() => (playing = null)} />
{/if}
{#if confirmDeleteRec}
  {@const r = confirmDeleteRec}
  <ConfirmDialog
    title="Delete recording"
    message="Delete “{r.title || r.name}” ({mb(r.size)})? This can't be undone."
    confirmLabel="Delete"
    onConfirm={async () => {
      await Recordings.Delete(r.name)
      await loadRecordings()
    }}
    onClose={() => (confirmDeleteRec = null)}
  />
{/if}

{#if confirmClear}
  <ConfirmDialog
    title="Clear history"
    message="Remove every entry from the connection history? Your saved hosts stay."
    confirmLabel="Clear"
    onConfirm={async () => {
      await History.Clear()
      await load()
    }}
    onClose={() => (confirmClear = false)}
  />
{/if}
