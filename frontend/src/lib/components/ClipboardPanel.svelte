<script lang="ts">
  import { onMount } from 'svelte'
  import { Clipboard } from '@wailsio/runtime'
  import { clipboardHistory, type Clip } from '../clipboard.svelte'
  import { score } from '../fuzzy'
  import ClipEditor from './ClipEditor.svelte'
  import ClipSettings from './ClipSettings.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    /** Name of the focused pane's host, for the "This host" filter. */
    activeHost?: string
    onPaste: (text: string) => void
    onClose: () => void
  }
  let { activeHost, onPaste, onClose }: Props = $props()

  let show = $state<'all' | 'pinned' | 'host'>('all')
  let panelEl: HTMLElement

  type View = { kind: 'list' } | { kind: 'edit'; clip: Clip | null } | { kind: 'settings' }

  let view = $state<View>({ kind: 'list' })
  let query = $state('')
  let confirmClear = $state(false)
  let copiedId = $state<number | null>(null)
  let search = $state<HTMLInputElement>()

  const matches = (c: Clip) => {
    const q = query.trim()
    return !q || score(q, c.text) > 0 || (!!c.label && score(q, c.label) > 0)
  }
  const saved = $derived(clipboardHistory.saved.filter(matches))
  const history = $derived(clipboardHistory.history.filter(matches))
  const shown = $derived(show === 'pinned' ? saved : show === 'host' ? [...saved, ...history].filter((c) => c.source === activeHost) : [...saved, ...history])
  const first = $derived(shown[0])
  type Show = typeof show
  const filters = $derived<[Show, string][]>(activeHost ? [['all', 'All'], ['pinned', 'Pinned'], ['host', 'This host']] : [['all', 'All'], ['pinned', 'Pinned']])

  onMount(() => {
    void clipboardHistory.load()
    search?.focus()
  })

  function backToList() {
    view = { kind: 'list' }
    requestAnimationFrame(() => search?.focus())
  }

  function ago(iso: string): string {
    const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000)
    if (s < 60) return 'just now'
    if (s < 3600) return `${Math.floor(s / 60)}m ago`
    if (s < 86400) return `${Math.floor(s / 3600)}h ago`
    return `${Math.floor(s / 86400)}d ago`
  }

  function keepFor(days: number) {
    return days === 0 ? 'kept forever' : days === 1 ? '1 day' : `${days} days`
  }

  async function copy(clip: Clip) {
    await Clipboard.SetText(clip.text)
    copiedId = clip.id
    setTimeout(() => copiedId === clip.id && (copiedId = null), 1200)
  }

  const iconBtn = 'grid size-6 place-items-center rounded-md text-subtle hover:bg-line hover:text-fg'
  const headBtn = 'grid size-7 place-items-center rounded-lg bg-hop text-muted hover:text-fg'
</script>

{#snippet entry(clip: Clip)}
  <li class="group relative">
    <button
      class="flex w-full flex-col gap-1.5 rounded-xl border bg-raised px-3 py-2.5 pr-3 text-left hover:border-strong {clip.pinned ? 'border-accent-line' : 'border-line'}"
      title="Paste into the active terminal{clip.text.length > 60 ? `\n\n${clip.text.slice(0, 400)}` : ''}"
      onclick={() => onPaste(clip.text)}
    >
      <span class="flex items-center gap-2 text-[11px] text-subtle">
        {#if clip.pinned}
          <span class="text-accent"><Icon name="pin" size={11} /></span>pinned{clip.label ? ` · ${clip.label}` : ''}
        {:else}
          <span class="size-2 rounded-full {clip.source ? 'bg-success' : 'bg-subtle'}" aria-hidden="true"></span>{clip.source || 'other app'} · {ago(clip.createdAt)}
        {/if}
      </span>
      <span class="truncate font-mono text-xs text-fg">{clip.text}</span>
    </button>
    <div class="absolute top-1.5 right-1.5 flex gap-0.5 rounded-lg bg-raised opacity-0 group-focus-within:opacity-100 group-hover:opacity-100">
      <button
        class="{iconBtn} {clip.pinned ? 'text-accent' : ''}"
        aria-label={clip.pinned ? 'Unpin' : 'Pin to saved clips'}
        aria-pressed={clip.pinned}
        onclick={() => clipboardHistory.setPinned(clip.id, !clip.pinned)}
      >
        <Icon name="pin" size={13} />
      </button>
      {#if clip.pinned}
        <button class={iconBtn} aria-label="Edit saved clip" onclick={() => (view = { kind: 'edit', clip })}>
          <Icon name="edit" size={13} />
        </button>
      {/if}
      <button class={iconBtn} aria-label="Copy to clipboard" onclick={() => copy(clip)}>
        <Icon name={copiedId === clip.id ? 'check' : 'clipboard'} size={13} />
      </button>
      <button class={iconBtn} aria-label="Delete entry" onclick={() => clipboardHistory.remove(clip.id)}>
        <Icon name="x" size={13} />
      </button>
    </div>
  </li>
{/snippet}

<svelte:window onkeydown={(e) => e.key === 'Escape' && view.kind === 'list' && panelEl?.contains(document.activeElement) && !query && onClose()} />

<aside
  bind:this={panelEl}
  aria-label="Clipboard history"
  class="clip-panel absolute top-2 right-2 bottom-2 z-30 flex w-80 flex-col gap-2.5 rounded-panel border px-3 py-3.5 backdrop-blur"
>
  <div class="flex items-center gap-2 px-1">
    <span class="text-accent"><Icon name="clipboard" size={16} /></span>
    <h2 class="flex-1 font-display text-[15px] font-semibold">Clipboard</h2>
    {#if clipboardHistory.status.enabled && view.kind === 'list'}
      <button
        class="rounded-lg px-2 py-1 text-xs {clipboardHistory.status.paused ? 'bg-warning/15 text-warning' : 'text-muted hover:bg-hop hover:text-fg'}"
        aria-pressed={clipboardHistory.status.paused}
        onclick={() => clipboardHistory.setPaused(!clipboardHistory.status.paused)}
      >
        {clipboardHistory.status.paused ? 'Paused' : 'Pause'}
      </button>
      <button class={headBtn} aria-label="New saved clip" title="New saved clip" onclick={() => (view = { kind: 'edit', clip: null })}>
        <Icon name="plus" size={14} />
      </button>
      <button class={headBtn} aria-label="History settings" title="History settings" onclick={() => (view = { kind: 'settings' })}>
        <Icon name="gear" size={14} />
      </button>
    {/if}
    <button class={headBtn} aria-label="Close clipboard" onclick={onClose}>
      <Icon name="x" size={12} />
    </button>
  </div>

  {#if view.kind === 'edit'}
    {#key view.clip}
      <ClipEditor clip={view.clip} onDone={backToList} />
    {/key}
  {:else if view.kind === 'settings'}
    <ClipSettings onDone={backToList} />
  {:else}
    <label class="block">
      <span class="sr-only">Search clipboard</span>
      <input
        bind:this={search}
        bind:value={query}
        class="h-[34px] w-full rounded-[10px] border border-strong bg-field px-3 text-[13px] placeholder:text-subtle focus:border-accent focus:outline-none focus-visible:outline-none"
        placeholder="Search copied text…"
        autocomplete="off"
        spellcheck="false"
        onkeydown={(e) => {
          if (e.key === 'Escape') query ? (query = '') : onClose()
          if (e.key === 'Enter' && first) onPaste(first.text)
        }}
      />
    </label>
    <div role="radiogroup" aria-label="Show" class="flex gap-1.5">
      {#each filters as [value, label] (value)}
        <button
          role="radio"
          aria-checked={show === value}
          class="flex h-6 items-center rounded-full px-2.5 text-[11.5px] {show === value ? 'bg-accent-chip text-accent-ink' : 'bg-hop text-muted hover:text-fg'}"
          onclick={() => (show = value)}
        >
          {label}
        </button>
      {/each}
    </div>

    <div class="-mx-1 min-h-0 flex-1 overflow-y-auto px-1">
      {#if !clipboardHistory.status.enabled}
        <p role="alert" class="rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">
          Clipboard history is off: {clipboardHistory.status.error || 'the keyring is unavailable'}. Nothing is stored without encryption.
        </p>
      {:else if clipboardHistory.list.length === 0}
        <p class="px-3 pt-8 text-center text-[13px] text-muted">
          Nothing yet. Copy with Ctrl+Shift+C in a terminal, copy anywhere and come back, or add a saved clip with +.
        </p>
      {:else if !shown.length}
        <p class="px-3 pt-8 text-center text-[13px] text-muted">{query ? 'No matches' : show === 'host' ? `Nothing copied from ${activeHost} yet` : 'No pinned clips yet'}</p>
      {/if}
      <ul class="flex flex-col gap-1.5">
        {#each shown as clip (clip.id)}{@render entry(clip)}{/each}
      </ul>
    </div>

    {#if clipboardHistory.status.enabled}
      <div class="flex items-center gap-2 px-1 text-[11px] leading-normal text-subtle">
        <span class="flex-1">Encrypted locally · {keepFor(clipboardHistory.settings.retentionDays)} · secrets skipped. Esc closes.</span>
        {#if clipboardHistory.history.length}
          {#if confirmClear}
            <button class="rounded-md bg-danger px-2 py-1 text-white hover:bg-danger/90" onclick={() => clipboardHistory.clear().then(() => (confirmClear = false))}>Clear</button>
            <button class="rounded-md px-2 py-1 text-muted hover:bg-hop" onclick={() => (confirmClear = false)}>Keep</button>
          {:else}
            <button class="shrink-0 rounded-md px-2 py-1 text-muted hover:bg-hop hover:text-fg" title="Saved clips are kept" onclick={() => (confirmClear = true)}>Clear…</button>
          {/if}
        {/if}
      </div>
    {/if}
  {/if}
</aside>

<style>
  .clip-panel {
    background: color-mix(in srgb, var(--color-panel) 95%, transparent);
    border-color: var(--color-strong);
    box-shadow: 0 30px 80px rgb(0 0 0 / 0.45);
  }
</style>
