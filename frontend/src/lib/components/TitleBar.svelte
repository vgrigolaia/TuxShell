<script lang="ts">
  import { onMount } from 'svelte'
  import { Window } from '@wailsio/runtime'
  import { SvelteSet } from 'svelte/reactivity'
  import Icon from './Icon.svelte'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import type { TabInfo } from '../tabs'
  import { theme } from '../theme.svelte'
  import { prefs } from '../settings.svelte'
  import type { Orientation } from '../layout'
  import { MAX_PANES } from '../layout'

  interface Props {
    tabs: TabInfo[]
    activeKey: number
    clipboardOpen: boolean
    hostsOpen: boolean
    onToggleHosts: () => void
    filesOpen: boolean
    onToggleFiles: () => void
    onSelect: (key: number) => void
    onClose: (key: number) => void
    onCloseMany: (keys: number[]) => void
    onMerge: (keys: number[], orientation: Orientation) => void
    onSplit: (key: number, orientation: Orientation) => void
    onUnsplit: (key: number) => void
    onRename: (key: number) => void
    onNew: () => void
    onSearch: () => void
    onToggleClipboard: () => void
    onPreferences: () => void
  }

  let { tabs, activeKey, clipboardOpen, hostsOpen, onToggleHosts, filesOpen, onToggleFiles, onSelect, onClose, onCloseMany, onMerge, onSplit, onUnsplit, onRename, onNew, onSearch, onToggleClipboard, onPreferences }: Props = $props()

  let maximised = $state(false)
  let strip: HTMLDivElement
  let menu = $state<{ x: number; y: number; key: number } | null>(null)
  // Ctrl+click builds a multi-tab selection for bulk actions.
  const picked = new SvelteSet<number>()

  // Forget selections for tabs that were closed.
  $effect(() => {
    const keys = new Set(tabs.map((t) => t.key))
    for (const k of picked) if (!keys.has(k)) picked.delete(k)
  })

  // Keep the active tab visible when the strip overflows.
  $effect(() => {
    const key = activeKey
    requestAnimationFrame(() => document.getElementById(`tab-${key}`)?.scrollIntoView({ block: 'nearest', inline: 'nearest' }))
  })

  // A vertical mouse wheel scrolls the strip sideways.
  function onWheel(e: WheelEvent) {
    if (Math.abs(e.deltaY) > Math.abs(e.deltaX)) {
      strip.scrollLeft += e.deltaY
      e.preventDefault()
    }
  }

  function onTabClick(e: MouseEvent, key: number) {
    if (e.ctrlKey || e.metaKey) {
      if (picked.has(key)) picked.delete(key)
      else picked.add(key)
      // Ctrl+clicking starts a selection that includes the current tab.
      if (picked.size && !picked.has(activeKey)) picked.add(activeKey)
      return
    }
    picked.clear()
    onSelect(key)
  }

  function openMenu(e: MouseEvent, key: number) {
    e.preventDefault()
    if (!picked.has(key)) picked.clear()
    menu = { x: e.clientX, y: e.clientY, key }
  }

  function menuItems(key: number): MenuItem[] {
    const i = tabs.findIndex((t) => t.key === key)
    const others = tabs.filter((t) => t.key !== key).map((t) => t.key)
    const right = tabs.slice(i + 1).map((t) => t.key)
    const tab = tabs[i]
    const items: MenuItem[] = []
    if (picked.size > 1) {
      const sel = [...picked]
      const total = tabs.filter((t) => picked.has(t.key)).reduce((n, t) => n + t.panes, 0)
      if (total <= MAX_PANES) {
        items.push({ label: `Split selected side by side (${sel.length})`, run: () => (onMerge(sel, 'row'), picked.clear()) })
        if (total < 4) items.push({ label: `Split selected stacked (${sel.length})`, run: () => (onMerge(sel, 'column'), picked.clear()) })
      }
      items.push({ label: `Close selected (${sel.length})`, run: () => (onCloseMany(sel), picked.clear()) })
    }
    if (tab.panes < MAX_PANES) {
      items.push({ label: 'Split right…', run: () => onSplit(key, 'row') })
      items.push({ label: 'Split down…', run: () => onSplit(key, 'column') })
    }
    if (tab.panes > 1) items.push({ label: 'Unsplit into tabs', run: () => onUnsplit(key) })
    items.push({ label: 'Rename tab…', separator: true, run: () => onRename(key) })
    items.push({ label: 'Close', run: () => onClose(key) })
    if (others.length) items.push({ label: 'Close others', run: () => onCloseMany(others) })
    if (right.length) items.push({ label: 'Close tabs to the right', run: () => onCloseMany(right) })
    items.push({ label: `Close all (${tabs.length})`, run: () => onCloseMany(tabs.map((t) => t.key)), danger: true })
    return items
  }

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

  // Window-control style buttons and the labelled "pill" buttons from the design.
  const wc = 'no-drag grid size-[30px] place-items-center rounded-[10px] bg-hop text-muted transition-colors hover:text-fg'
  const pill =
    'no-drag flex h-[34px] shrink-0 items-center gap-2 rounded-[11px] border border-strong bg-panel pr-3 pl-2.5 text-[13px] text-muted transition-colors hover:text-fg'
</script>

<!-- Double-click to maximize is a mouse shortcut; the Maximize button covers keyboard users. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<header
  class="drag flex shrink-0 items-center gap-3.5 pt-0.5 pr-1 pl-1.5"
  ondblclick={(e) => e.target === e.currentTarget && Window.ToggleMaximise()}
>
  <div class="flex items-center gap-2.5">
    <button
      class="{wc} {hostsOpen ? 'text-fg' : ''}"
      aria-label={hostsOpen ? 'Hide hosts' : 'Show hosts'}
      aria-pressed={hostsOpen}
      aria-keyshortcuts="Control+Shift+L"
      title="{hostsOpen ? 'Hide' : 'Show'} hosts (Ctrl+Shift+L)"
      onclick={onToggleHosts}
    >
      <Icon name="sidebar" size={14} />
    </button>
    <span class="logo grid size-[30px] place-items-center rounded-[10px] bg-accent text-on-accent" aria-hidden="true">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><path d="M5 17l6-5-6-5" /><path d="M13 19h6" /></svg>
    </span>
    <span class="font-display text-[17px] font-bold tracking-[-0.02em]">TuxShell</span>
  </div>

  <div class="no-drag ml-2 flex min-w-0 items-center gap-1 rounded-xl border border-line bg-panel p-1">
    <div
      bind:this={strip}
      role="tablist"
      aria-label="Workspace tabs"
      aria-multiselectable="true"
      class="tab-strip flex min-w-0 items-center gap-1 overflow-x-auto"
      onwheel={onWheel}
    >
      {#each tabs as tab, i (tab.key)}
        {@const current = tab.key === activeKey}
        <!-- Middle-click closes, like browser and terminal tabs. -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="group flex h-[30px] shrink-0 items-center rounded-[9px] transition-colors {current
            ? 'tab-current bg-tab text-fg'
            : 'text-muted hover:text-fg'} {picked.has(tab.key) ? 'ring-1 ring-accent' : ''}"
          onauxclick={(e) => e.button === 1 && (e.preventDefault(), onClose(tab.key))}
          oncontextmenu={(e) => openMenu(e, tab.key)}
        >
          <button
            id="tab-{tab.key}"
            role="tab"
            aria-selected={current || picked.has(tab.key)}
            aria-controls="panel-{tab.key}"
            tabindex={current ? 0 : -1}
            class="flex h-full max-w-56 items-center gap-2 pr-1 pl-3 text-[13px]"
            title="{tab.title}{tab.exited ? ' (disconnected)' : ''} — Ctrl+click to select, right-click to split or close"
            onclick={(e) => onTabClick(e, tab.key)}
            onkeydown={(e) => onTabKey(e, i)}
          >
            <span class="font-mono text-[11px] {current ? 'text-accent-text' : ''}">{i + 1}</span>
            <span class="truncate {tab.exited ? 'text-subtle line-through decoration-subtle/50' : ''}">{tab.title}</span>
            {#if tab.panes > 1}<span class="text-[11px] text-subtle" aria-label="{tab.panes} tiles">{tab.panes}</span>{/if}
            {#if tab.exited}<span class="size-2 shrink-0 rounded-full border-2 border-warning" aria-label="disconnected"></span>{/if}
          </button>
          <button
            class="mr-1.5 grid size-5 place-items-center rounded text-subtle opacity-0 group-hover:opacity-100 hover:bg-line hover:text-fg focus-visible:opacity-100"
            aria-label="Close {tab.title}"
            tabindex="-1"
            onclick={() => onClose(tab.key)}
          >
            <Icon name="x" size={11} />
          </button>
        </div>
      {/each}
    </div>
    <button class="grid size-[30px] shrink-0 place-items-center rounded-[9px] text-muted hover:bg-tab hover:text-fg" aria-label="New local shell tab" title="New local shell tab" onclick={onNew}>
      <Icon name="plus" size={14} />
    </button>
  </div>

  <div class="min-w-4 flex-1 self-stretch"></div>

  <button
    class="{pill} {clipboardOpen ? 'border-accent-line text-accent-ink' : ''}"
    aria-pressed={clipboardOpen}
    aria-keyshortcuts="Control+Shift+H"
    onclick={onToggleClipboard}
  >
    <Icon name="clipboard" size={14} />
    <span class="hidden xl:inline">Clipboard</span>
    <span class="kbd">Ctrl Shift H</span>
  </button>

  <button class={pill} aria-keyshortcuts="Control+K" onclick={onSearch}>
    <Icon name="search" size={14} />
    <span class="whitespace-nowrap">Jump to anything</span>
    <span class="kbd">Ctrl K</span>
  </button>

  <div class="flex items-center gap-1.5">
    <button
      class="{wc} {filesOpen ? 'bg-accent-chip text-accent-ink' : ''}"
      aria-label="Files"
      aria-pressed={filesOpen}
      aria-keyshortcuts="Control+Shift+E"
      title="Files (Ctrl+Shift+E)"
      onclick={onToggleFiles}
    >
      <Icon name="folder" size={14} />
    </button>
    <button
      class={wc}
      aria-label="Theme: {theme.mode}. Switch theme"
      title="Theme: {theme.mode === 'system' ? 'follow system' : theme.mode} (click to change)"
      onclick={() => prefs.cycleTheme()}
    >
      <Icon name={theme.mode === 'dark' ? 'moon' : theme.mode === 'light' ? 'sun' : 'monitor'} size={14} />
    </button>
    <button class={wc} aria-label="Preferences" aria-keyshortcuts="Control+," title="Preferences (Ctrl+,)" onclick={onPreferences}>
      <Icon name="gear" size={14} />
    </button>
  </div>

  <div class="flex items-center gap-1.5 border-l border-line pl-2.5">
    <button class={wc} aria-label="Minimize" onclick={() => Window.Minimise()}>
      <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M2 5h6" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" /></svg>
    </button>
    <button
      class={wc}
      aria-label={maximised ? 'Restore' : 'Maximize'}
      onclick={async () => {
        await Window.ToggleMaximise()
        void syncMaximised()
      }}
    >
      <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><rect x="2" y="2" width="6" height="6" rx="1.5" fill="none" stroke="currentColor" stroke-width="1.4" /></svg>
    </button>
    <button class="{wc} bg-close text-close-ink hover:bg-danger hover:text-white" aria-label="Close window" onclick={() => Window.Close()}>
      <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M2.5 2.5l5 5M7.5 2.5l-5 5" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" /></svg>
    </button>
  </div>
</header>

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} label="Tab actions" items={menuItems(menu.key)} onClose={() => (menu = null)} />
{/if}

<style>
  .logo {
    box-shadow: 0 0 24px color-mix(in srgb, var(--color-accent) 40%, transparent);
  }
  .tab-current {
    box-shadow: inset 0 0 0 1px var(--color-tab-ring);
  }
  /* Scroll with the wheel instead of a scrollbar drawn over the close buttons. */
  .tab-strip {
    scrollbar-width: none;
  }
  .tab-strip::-webkit-scrollbar {
    display: none;
  }
</style>
