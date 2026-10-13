<script lang="ts">
  import { onMount } from 'svelte'
  import { Clipboard } from '@wailsio/runtime'
  import { clipboardHistory, type Clip } from '../clipboard.svelte'
  import { score } from '../fuzzy'
  import Icon from './Icon.svelte'

  interface Props {
    onPaste: (text: string) => void
    onClose: () => void
  }
  let { onPaste, onClose }: Props = $props()

  let query = $state('')
  let confirmClear = $state(false)
  let copiedId = $state<number | null>(null)
  let search: HTMLInputElement

  const shown = $derived(
    query.trim() ? clipboardHistory.list.filter((c) => score(query.trim(), c.text) > 0) : clipboardHistory.list,
  )

  onMount(() => {
    void clipboardHistory.load()
    search.focus()
  })

  function ago(iso: string): string {
    const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000)
    if (s < 60) return 'just now'
    if (s < 3600) return `${Math.floor(s / 60)}m ago`
    if (s < 86400) return `${Math.floor(s / 3600)}h ago`
    return `${Math.floor(s / 86400)}d ago`
  }

  async function copy(clip: Clip) {
    await Clipboard.SetText(clip.text)
    copiedId = clip.id
    setTimeout(() => copiedId === clip.id && (copiedId = null), 1200)
  }
</script>

<aside aria-label="Clipboard history" class="flex w-80 shrink-0 flex-col overflow-hidden rounded-panel border border-line bg-panel">
  <div class="flex items-center gap-2 px-4 pt-3.5 pb-2">
    <h2 class="flex-1 font-display text-[15px] font-semibold">Clipboard</h2>
    {#if clipboardHistory.status.enabled}
      <button
        class="rounded-lg px-2 py-1 text-xs {clipboardHistory.status.paused ? 'bg-warning/15 text-warning' : 'text-muted hover:bg-raised hover:text-fg'}"
        aria-pressed={clipboardHistory.status.paused}
        onclick={() => clipboardHistory.setPaused(!clipboardHistory.status.paused)}
      >
        {clipboardHistory.status.paused ? 'Paused' : 'Pause'}
      </button>
    {/if}
    <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Close clipboard history" onclick={onClose}>
      <Icon name="x" size={14} />
    </button>
  </div>

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
          if (e.key === 'Enter' && shown[0]) onPaste(shown[0].text)
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
        Nothing yet. Copy with Ctrl+Shift+C in a terminal, or copy anywhere and come back to TuxShell.
      </p>
    {:else if shown.length === 0}
      <p class="px-3 pt-8 text-center text-[13px] text-muted">No matches</p>
    {/if}

    <ul class="space-y-1">
      {#each shown as clip (clip.id)}
        <li class="group relative rounded-xl border border-transparent hover:border-line hover:bg-raised">
          <button class="block w-full px-3 py-2 pr-16 text-left" title="Paste into the active terminal" onclick={() => onPaste(clip.text)}>
            <span class="line-clamp-3 font-mono text-xs break-all whitespace-pre-wrap">{clip.text}</span>
            <span class="mt-1 block text-[11px] text-subtle">{ago(clip.createdAt)}</span>
          </button>
          <div class="absolute top-1.5 right-1.5 flex gap-0.5 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100">
            <button class="grid size-6 place-items-center rounded-md text-subtle hover:bg-line hover:text-fg" aria-label="Copy to clipboard" onclick={() => copy(clip)}>
              <Icon name={copiedId === clip.id ? 'check' : 'clipboard'} size={13} />
            </button>
            <button class="grid size-6 place-items-center rounded-md text-subtle hover:bg-line hover:text-fg" aria-label="Delete entry" onclick={() => clipboardHistory.remove(clip.id)}>
              <Icon name="x" size={13} />
            </button>
          </div>
        </li>
      {/each}
    </ul>
  </div>

  {#if clipboardHistory.list.length > 0}
    <div class="flex items-center gap-2 border-t border-line px-4 py-2.5 text-xs text-subtle">
      <span class="flex-1">Encrypted · kept 30 days</span>
      {#if confirmClear}
        <button class="rounded-md bg-red-500/80 px-2 py-1 text-white hover:bg-red-500" onclick={() => clipboardHistory.clear().then(() => (confirmClear = false))}>Clear all</button>
        <button class="rounded-md px-2 py-1 text-muted hover:bg-raised" onclick={() => (confirmClear = false)}>Keep</button>
      {:else}
        <button class="rounded-md px-2 py-1 text-muted hover:bg-raised hover:text-fg" onclick={() => (confirmClear = true)}>Clear…</button>
      {/if}
    </div>
  {/if}
</aside>
