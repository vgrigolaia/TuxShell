<script lang="ts">
  import { onMount } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import * as Recordings from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/recordings/service'
  import { errorMessage } from '../hosts.svelte'
  import { prefs } from '../settings.svelte'
  import { profileTheme } from '../schemes'
  import { theme } from '../theme.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    name: string
    title: string
    onClose: () => void
  }
  let { name, title, onClose }: Props = $props()

  /** Pauses longer than this are shortened, like asciinema's idle limit. */
  const IDLE = 2

  type Event = { t: number; kind: string; data: string }

  let dialog: HTMLDialogElement
  let screen: HTMLDivElement
  let term: Terminal | undefined
  let events: Event[] = []
  let error = $state('')
  let loaded = $state(false)
  let playing = $state(false)
  let speed = $state(1)
  // Position in the (idle-shortened) recording, in seconds.
  let at = $state(0)
  let length = $state(0)
  let next = 0
  let timer: ReturnType<typeof setTimeout> | undefined
  let startedAt = 0
  let startedPos = 0
  const id = $props.id()

  function parse(text: string) {
    const lines = text.split('\n')
    const head = JSON.parse(lines[0]) as { width: number; height: number }
    const list: Event[] = []
    let last = 0
    let shift = 0
    for (const line of lines.slice(1)) {
      if (!line.trim()) continue
      try {
        const [t, kind, data] = JSON.parse(line) as [number, string, string]
        if (t - last > IDLE) shift += t - last - IDLE
        last = t
        list.push({ t: t - shift, kind, data })
      } catch {
        // a cut-off last line (the app quit mid-write)
      }
    }
    return { head, list }
  }

  onMount(() => {
    dialog.showModal()
    let disposed = false
    void (async () => {
      try {
        const { head, list } = parse(await Recordings.Load(name))
        if (disposed) return
        events = list
        length = list.at(-1)?.t ?? 0
        term = new Terminal({
          cols: head.width,
          rows: head.height,
          disableStdin: true,
          cursorBlink: false,
          fontFamily: prefs.fontFamily,
          fontSize: prefs.fontSize,
          theme: profileTheme(prefs.value.terminal, theme.resolved),
          scrollback: 1000,
        })
        term.open(screen)
        loaded = true
        play()
      } catch (e) {
        error = errorMessage(e)
      }
    })()
    return () => {
      disposed = true
      clearTimeout(timer)
      term?.dispose()
    }
  })

  function apply(ev: Event) {
    if (!term) return
    if (ev.kind === 'o') term.write(ev.data)
    else if (ev.kind === 'r') {
      const [c, r] = ev.data.split('x').map(Number)
      if (c > 0 && r > 0) term.resize(c, r)
    }
  }

  function schedule() {
    clearTimeout(timer)
    if (!playing) return
    if (next >= events.length) {
      playing = false
      at = length
      return
    }
    const now = startedPos + ((performance.now() - startedAt) / 1000) * speed
    // Write everything due, then wait for the next event.
    while (next < events.length && events[next].t <= now) apply(events[next++])
    at = Math.min(now, length)
    if (next >= events.length) return schedule()
    const wait = ((events[next].t - now) / speed) * 1000
    timer = setTimeout(schedule, Math.min(Math.max(wait, 0), 250))
  }

  function play() {
    if (next >= events.length) seek(0)
    playing = true
    startedAt = performance.now()
    startedPos = at
    schedule()
  }

  function pause() {
    playing = false
    clearTimeout(timer)
  }

  /** Jumps to a time by replaying everything before it. */
  function seek(t: number) {
    if (!term) return
    term.reset()
    next = 0
    while (next < events.length && events[next].t <= t) apply(events[next++])
    at = t
    startedAt = performance.now()
    startedPos = t
    if (playing) schedule()
  }

  function setSpeed(v: number) {
    startedPos = at
    startedAt = performance.now()
    speed = v
  }

  const clock = (s: number) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, '0')}`
</script>

<dialog
  bind:this={dialog}
  aria-labelledby="{id}-title"
  class="m-auto flex max-h-[92vh] w-[min(1100px,96vw)] flex-col overflow-hidden rounded-2xl border border-line bg-panel p-0 text-fg shadow-2xl backdrop:bg-black/60"
  onclose={onClose}
>
  <header class="flex items-center gap-3 border-b border-line px-5 py-3">
    <span class="text-danger-text"><Icon name="record" size={15} /></span>
    <h2 id="{id}-title" class="min-w-0 flex-1 truncate font-display text-[15px] font-semibold">{title || name}</h2>
    <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Close player" onclick={() => dialog.close()}>
      <Icon name="x" size={14} />
    </button>
  </header>
  <div class="min-h-0 flex-1 overflow-auto bg-field p-3">
    {#if error}
      <p role="alert" class="p-4 text-sm text-warning">{error}</p>
    {:else if !loaded}
      <p class="p-4 text-sm text-muted">Loading…</p>
    {/if}
    <div bind:this={screen} class="w-fit"></div>
  </div>
  <footer class="flex items-center gap-3 border-t border-line px-5 py-3 text-xs">
    <button
      class="grid size-8 place-items-center rounded-lg bg-accent text-on-accent hover:bg-accent/90 disabled:opacity-50"
      aria-label={playing ? 'Pause' : 'Play'}
      disabled={!loaded}
      onclick={() => (playing ? pause() : play())}
    >
      <Icon name={playing ? 'pause' : 'play'} size={14} />
    </button>
    <span class="w-20 font-mono text-muted tabular-nums">{clock(at)} / {clock(length)}</span>
    <label class="min-w-0 flex-1">
      <span class="sr-only">Position</span>
      <input type="range" class="w-full accent-accent" min="0" max={length || 1} step="0.1" value={at} disabled={!loaded} oninput={(e) => seek(Number(e.currentTarget.value))} />
    </label>
    <div class="flex gap-1" role="group" aria-label="Speed">
      {#each [1, 2, 4, 8] as v (v)}
        <button class="rounded-md px-2 py-1 font-mono {speed === v ? 'bg-accent-chip text-accent-ink' : 'text-muted hover:bg-raised'}" aria-pressed={speed === v} onclick={() => setSpeed(v)}>{v}×</button>
      {/each}
    </div>
    <span class="text-subtle" title="Pauses longer than {IDLE} seconds are shortened">Long pauses skipped</span>
  </footer>
</dialog>
