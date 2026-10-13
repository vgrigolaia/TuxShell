<script lang="ts">
  import { onMount } from 'svelte'
  import { Clipboard } from '@wailsio/runtime'
  import { clipboardHistory, type Clip } from '../clipboard.svelte'
  import { score } from '../fuzzy'
  import ClipEditor from './ClipEditor.svelte'
  import ClipSettings from './ClipSettings.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    onPaste: (text: string) => void
    onClose: () => void
  }
  let { onPaste, onClose }: Props = $props()

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
  const first = $derived(saved[0] ?? history[0])

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
    return days === 0 ? 'kept forever' : days === 1 ? 'kept 1 day' : `kept ${days} days`
  }

  async function copy(clip: Clip) {
    await Clipboard.SetText(clip.text)
    copiedId = clip.id
    setTimeout(() => copiedId === clip.id && (copiedId = null), 1200)
  }

  const iconBtn = 'grid size-6 place-items-center rounded-md text-subtle hover:bg-line hover:text-fg'
</script>

{#snippet entry(clip: Clip)}
  <li class="group relative rounded-xl border border-transparent hover:border-line hover:bg-raised">
    <button class="block w-full px-3 py-2 pr-24 text-left" title="Paste into the active terminal" onclick={() => onPaste(clip.text)}>
      {#if clip.label}<span class="mb-0.5 block truncate text-[13px] font-medium">{clip.label}</span>{/if}
      <span class="line-clamp-3 font-mono text-xs break-all whitespace-pre-wrap {clip.label ? 'text-muted' : ''}">{clip.text}</span>
      {#if !clip.pinned}<span class="mt-1 block text-[11px] text-subtle">{ago(clip.createdAt)}</span>{/if}
    </button>
    <div class="absolute top-1.5 right-1.5 flex gap-0.5 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100">
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

<aside aria-label="Clipboard history" class="flex w-80 shrink-0 flex-col overflow-hidden rounded-panel border border-line bg-panel">
  <div class="flex items-center gap-1 px-4 pt-3.5 pb-2">
    <h2 class="flex-1 font-display text-[15px] font-semibold">Clipboard</h2>
    {#if clipboardHistory.status.enabled && view.kind === 'list'}
      <button
        class="rounded-lg px-2 py-1 text-xs {clipboardHistory.status.paused ? 'bg-warning/15 text-warning' : 'text-muted hover:bg-raised hover:text-fg'}"
        aria-pressed={clipboardHistory.status.paused}
        onclick={() => clipboardHistory.setPaused(!clipboardHistory.status.paused)}
      >
        {clipboardHistory.status.paused ? 'Paused' : 'Pause'}
      </button>
      <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="New saved clip" title="New saved clip" onclick={() => (view = { kind: 'edit', clip: null })}>
        <Icon name="plus" size={15} />
      </button>
      <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="History settings" title="History settings" onclick={() => (view = { kind: 'settings' })}>
        <Icon name="gear" size={15} />
      </button>
    {/if}
    <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Close clipboard history" onclick={onClose}>
      <Icon name="x" size={14} />
    </button>
  </div>

  {#if view.kind === 'edit'}
    {#key view.clip}
      <ClipEditor clip={view.clip} onDone={backToList} />
    {/key}
  {:else if view.kind === 'settings'}
    <ClipSettings onDone={backToList} />
  {:else}
    <div class="px-3 pb-2">
      <label class="flex items-center gap-2 rounded-lg border border-line bg-ground px-2.5 focus-within:border-accent">
        <span class="text-subtle"><Icon name="search" size={13} /></span>
        <span class="sr-only">Search clipboard history</span>
        <input
          bind:this={search}
          bind:value={query}
          class="h-8 min-w-0 flex-1 bg-transparent text-[13px] placeholder:text-subtle focus:outline-none"
          placeholder="Search"
          autocomplete="off"
          spellcheck="false"
          onkeydown={(e) => {
            if (e.key === 'Escape') query ? (query = '') : onClose()
            if (e.key === 'Enter' && first) onPaste(first.text)
          }}
        />
      </label>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
      {#if !clipboardHistory.status.enabled}
        <p role="alert" class="m-2 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">
          Clipboard history is off: {clipboardHistory.status.error || 'the keyring is unavailable'}. Nothing is stored without encryption.
        </p>
      {:else if clipboardHistory.list.length === 0}
        <p class="px-3 pt-8 text-center text-[13px] text-muted">
          Nothing yet. Copy with Ctrl+Shift+C in a terminal, copy anywhere and come back, or add a saved clip with +.
        </p>
      {:else if !saved.length && !history.length}
        <p class="px-3 pt-8 text-center text-[13px] text-muted">No matches</p>
      {/if}

      {#if saved.length}
        <h3 class="flex items-center gap-1.5 px-2 pt-1 pb-1.5 font-mono text-[11px] tracking-wider text-subtle uppercase">
          <Icon name="pin" size={11} /> Saved <span class="ml-auto">{saved.length}</span>
        </h3>
        <ul class="space-y-1">
          {#each saved as clip (clip.id)}{@render entry(clip)}{/each}
        </ul>
      {/if}

      {#if history.length}
        <h3 class="flex items-center px-2 pt-3 pb-1.5 font-mono text-[11px] tracking-wider text-subtle uppercase">
          History <span class="ml-auto">{history.length}</span>
        </h3>
        <ul class="space-y-1">
          {#each history as clip (clip.id)}{@render entry(clip)}{/each}
        </ul>
      {/if}
    </div>

    {#if clipboardHistory.status.enabled}
      <div class="flex items-center gap-2 border-t border-line px-4 py-2.5 text-xs text-subtle">
        <span class="flex-1">Encrypted · {keepFor(clipboardHistory.settings.retentionDays)}</span>
        {#if clipboardHistory.history.length}
          {#if confirmClear}
            <button class="rounded-md bg-danger px-2 py-1 text-white hover:bg-danger/90" onclick={() => clipboardHistory.clear().then(() => (confirmClear = false))}>Clear history</button>
            <button class="rounded-md px-2 py-1 text-muted hover:bg-raised" onclick={() => (confirmClear = false)}>Keep</button>
          {:else}
            <button class="rounded-md px-2 py-1 text-muted hover:bg-raised hover:text-fg" title="Saved clips are kept" onclick={() => (confirmClear = true)}>Clear…</button>
          {/if}
        {/if}
      </div>
    {/if}
  {/if}
</aside>
