<script lang="ts">
  import { onMount } from 'svelte'
  import { Window } from '@wailsio/runtime'
  import Icon from './Icon.svelte'
  import type { Tab } from '../tabs'

  interface Props {
    tabs: Tab[]
    activeKey: number
    clipboardOpen: boolean
    onSelect: (key: number) => void
    onClose: (key: number) => void
    onNew: () => void
    onSearch: () => void
    onToggleClipboard: () => void
  }

  let { tabs, activeKey, clipboardOpen, onSelect, onClose, onNew, onSearch, onToggleClipboard }: Props = $props()

  let maximised = $state(false)

  async function syncMaximised() {
    maximised = await Window.IsMaximised()
  }

  onMount(() => {
    void syncMaximised()
    window.addEventListener('resize', syncMaximised)
    return () => window.removeEventListener('resize', syncMaximised)
  })

  // Arrow keys move between tabs, per the WAI-ARIA tabs pattern.
  function onTabKey(e: KeyboardEvent, index: number) {
    const step = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0
    if (!step) return
    e.preventDefault()
    const next = tabs[(index + step + tabs.length) % tabs.length]
    onSelect(next.key)
    document.getElementById(`tab-${next.key}`)?.focus()
  }

  const iconButton =
    'no-drag grid size-8 place-items-center rounded-lg text-muted transition-colors hover:bg-raised hover:text-fg'
</script>

<!-- Double-click to maximize is a mouse shortcut; the Maximize button covers keyboard users. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<header
  class="drag flex h-12 shrink-0 items-center gap-3 px-3"
  ondblclick={(e) => e.target === e.currentTarget && Window.ToggleMaximise()}
>
  <div class="flex items-center gap-2 pr-1 pl-1">
    <span class="grid size-7 place-items-center rounded-lg bg-accent text-ground">
      <Icon name="terminal" size={15} />
    </span>
    <span class="font-display text-[15px] font-semibold tracking-tight">TuxShell</span>
  </div>

  <div class="no-drag flex min-w-0 items-center gap-1 rounded-xl border border-line bg-panel p-1">
    <div role="tablist" aria-label="Workspace tabs" class="flex min-w-0 items-center gap-1 overflow-x-auto">
      {#each tabs as tab, i (tab.key)}
        <div
          class="group flex shrink-0 items-center rounded-lg transition-colors {tab.key === activeKey
            ? 'bg-raised text-fg'
            : 'text-muted hover:text-fg'}"
        >
          <button
            id="tab-{tab.key}"
            role="tab"
            aria-selected={tab.key === activeKey}
            aria-controls="panel-{tab.key}"
            tabindex={tab.key === activeKey ? 0 : -1}
            class="flex max-w-48 items-center gap-2 py-1 pr-1 pl-2.5 text-[13px]"
            onclick={() => onSelect(tab.key)}
            onkeydown={(e) => onTabKey(e, i)}
          >
            <span class="size-1.5 shrink-0 rounded-full {tab.exited ? 'bg-subtle' : 'bg-success'}"></span>
            <span class="truncate">{tab.title}</span>
          </button>
          <button
            class="mr-1 grid size-5 place-items-center rounded text-subtle hover:bg-line hover:text-fg"
            aria-label="Close {tab.title}"
            onclick={() => onClose(tab.key)}
          >
            <Icon name="x" size={12} />
          </button>
        </div>
      {/each}
    </div>
    <button class="grid size-7 shrink-0 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="New local shell tab" onclick={onNew}>
      <Icon name="plus" size={14} />
    </button>
  </div>

  <div class="flex-1"></div>

  <button
    class="no-drag flex h-8 w-64 min-w-40 shrink items-center gap-2 rounded-lg border border-line bg-panel px-3 text-[13px] text-subtle transition-colors hover:border-accent/50 hover:text-muted"
    aria-keyshortcuts="Control+K"
    onclick={onSearch}
  >
    <Icon name="search" size={14} />
    <span class="min-w-0 flex-1 truncate text-left whitespace-nowrap">Search hosts, connect…</span>
    <kbd class="shrink-0 rounded border border-line px-1.5 font-mono text-[11px]">Ctrl K</kbd>
  </button>

  <button
    class="{iconButton} {clipboardOpen ? 'bg-raised text-accent' : ''}"
    aria-label="Clipboard history"
    aria-pressed={clipboardOpen}
    title="Clipboard history (Ctrl+Shift+H)"
    aria-keyshortcuts="Control+Shift+H"
    onclick={onToggleClipboard}
  >
    <Icon name="clipboard" />
  </button>

  <div class="flex items-center gap-0.5 border-l border-line pl-2">
    <button class={iconButton} aria-label="Minimize" onclick={() => Window.Minimise()}>
      <Icon name="minimize" size={14} />
    </button>
    <button
      class={iconButton}
      aria-label={maximised ? 'Restore' : 'Maximize'}
      onclick={async () => {
        await Window.ToggleMaximise()
        void syncMaximised()
      }}
    >
      <Icon name={maximised ? 'restore' : 'maximize'} size={13} />
    </button>
    <button class="{iconButton} hover:bg-red-500/80 hover:text-white" aria-label="Close window" onclick={() => Window.Close()}>
      <Icon name="x" size={14} />
    </button>
  </div>
</header>
