<script lang="ts">
  import { isMac, keys } from '../platform'
  import { onMount } from 'svelte'
  import { Browser, Window } from '@wailsio/runtime'
  import Icon from './Icon.svelte'
  import Logo from './Logo.svelte'
  import ContextMenu from './ContextMenu.svelte'
  import { tabMenu, tabPicker, type TabActions } from '../tabbar.svelte'
  import type { TabInfo } from '../tabs'
  import { theme } from '../theme.svelte'
  import { prefs } from '../settings.svelte'
  import { keybindings } from '../keybindings.svelte'
  import { updates } from '../updates.svelte'
  import { edits } from '../edits.svelte'
  import { remoteDesktops } from '../rdp.svelte'
  import { hostStore } from '../hosts.svelte'
  import RdpSessions from './RdpSessions.svelte'

  interface Props {
    tabs: TabInfo[]
    activeKey: number
    clipboardOpen: boolean
    hostsOpen: boolean
    onToggleHosts: () => void
    filesOpen: boolean
    onToggleFiles: () => void
    /** Tabs show here only when Preferences → Open tabs is "In the title bar". */
    showTabs: boolean
    actions: TabActions
    onSearch: () => void
    onToggleClipboard: () => void
    onPreferences: () => void
    /** The Home screen (hosts launchpad) is showing. */
    homeActive: boolean
  }

  let { tabs, activeKey, clipboardOpen, hostsOpen, onToggleHosts, filesOpen, onToggleFiles, showTabs, actions, onSearch, onToggleClipboard, onPreferences, homeActive }: Props = $props()

  let rdpOpen = $state(false)

  let maximised = $state(false)
  let strip = $state<HTMLDivElement>()
  let menu = $state<{ x: number; y: number; key: number } | null>(null)
  const picked = tabPicker.picked

  $effect(() => tabPicker.prune(tabs))

  // Keep the active tab visible when the strip overflows.
  $effect(() => {
    const key = activeKey
    requestAnimationFrame(() => document.getElementById(`tab-${key}`)?.scrollIntoView({ block: 'nearest', inline: 'nearest' }))
  })

  // A vertical mouse wheel scrolls the strip sideways.
  function onWheel(e: WheelEvent) {
    if (Math.abs(e.deltaY) > Math.abs(e.deltaX)) {
      if (strip) strip.scrollLeft += e.deltaY
      e.preventDefault()
    }
  }

  function openMenu(e: MouseEvent, key: number) {
    e.preventDefault()
    if (!picked.has(key)) picked.clear()
    menu = { x: e.clientX, y: e.clientY, key }
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
    actions.select(next.key)
    document.getElementById(`tab-${next.key}`)?.focus()
  }

  // Window-control style buttons and the labelled "pill" buttons from the design.
  const wc = 'no-drag grid size-[30px] place-items-center rounded-[10px] bg-hop text-muted transition-colors hover:text-fg'
  const pill =
    'no-drag flex h-[34px] shrink-0 items-center gap-2 rounded-[11px] border border-strong bg-panel pr-3 pl-2.5 text-[13px] text-muted transition-colors hover:text-fg'
</script>

<!-- Double-click to maximize is a mouse shortcut; the Maximize button covers keyboard users. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<!-- On macOS the system's own close/minimize/zoom buttons sit at the left, so
     the bar starts after them and has no buttons of its own. -->
<header
  class="drag flex shrink-0 items-center gap-3.5 pt-0.5 pr-1 {isMac ? 'pl-[78px]' : 'pl-1.5'}"
  ondblclick={(e) => e.target === e.currentTarget && Window.ToggleMaximise()}
>
  <div class="flex items-center gap-2.5">
    <button
      class="{wc} {hostsOpen ? 'text-fg' : ''}"
      aria-label={hostsOpen ? 'Hide hosts' : 'Show hosts'}
      aria-pressed={hostsOpen}
      aria-keyshortcuts={keybindings.aria('toggleHosts')}
      title={keybindings.title(hostsOpen ? 'Hide hosts' : 'Show hosts', 'toggleHosts')}
      onclick={onToggleHosts}
    >
      <Icon name="sidebar" size={14} />
    </button>
    <Logo />
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
      <button
        role="tab"
        id="tab-home"
        aria-selected={homeActive}
        tabindex={homeActive ? 0 : -1}
        class="flex h-[30px] shrink-0 items-center gap-2 rounded-[9px] px-3 text-[13px] transition-colors {homeActive ? 'tab-current bg-tab text-fg' : 'text-muted hover:text-fg'}"
        title="Home: all hosts"
        onclick={actions.home}
      >
        <span class={homeActive ? 'text-accent-text' : ''}><Icon name="home" size={14} /></span>Home
      </button>
      <!-- With tabs in the left panel, Home (and +) stay here on their own. -->
      {#if showTabs}
      {#each tabs as tab, i (tab.key)}
        {@const current = tab.key === activeKey}
        <!-- Middle-click closes, like browser and terminal tabs. -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="group flex h-[30px] shrink-0 items-center rounded-[9px] transition-colors {current
            ? 'tab-current bg-tab text-fg'
            : 'text-muted hover:text-fg'} {picked.has(tab.key) ? 'ring-1 ring-accent' : ''}"
          onauxclick={(e) => e.button === 1 && (e.preventDefault(), actions.close(tab.key))}
          oncontextmenu={(e) => openMenu(e, tab.key)}
        >
          <button
            id="tab-{tab.key}"
            role="tab"
            aria-selected={current || picked.has(tab.key)}
            aria-controls="panel-{tab.key}"
            tabindex={current ? 0 : -1}
            class="flex h-full max-w-56 items-center gap-2 pr-1 pl-3 text-[13px]"
            title="{tab.title}{tab.exited ? ' (disconnected)' : ''} — {keys('Ctrl+click')} to select, right-click to split or close"
            onclick={(e) => tabPicker.click(e, tab.key, activeKey, actions.select)}
            onkeydown={(e) => onTabKey(e, i)}
          >
            <span class="font-mono text-[11px] {current ? 'text-accent-text' : ''}">{i + 1}</span>
            <span class="truncate {tab.exited ? 'text-subtle line-through decoration-subtle/50' : ''}">{tab.title}</span>
            {#if tab.panes > 1}<span class="text-[11px] text-subtle" aria-label="{tab.panes} tiles">{tab.panes}</span>{/if}
            {#if tab.exited}<span class="size-2 shrink-0 rounded-full border-2 border-warning" aria-label="disconnected"></span>{/if}
            {#if tab.bell}<span class="shrink-0 text-warning" role="img" aria-label="bell rang"><Icon name="bell" size={12} /></span>{/if}
          </button>
          <button
            class="mr-1.5 grid size-5 place-items-center rounded text-subtle opacity-0 group-hover:opacity-100 hover:bg-line hover:text-fg focus-visible:opacity-100"
            aria-label="Close {tab.title}"
            tabindex="-1"
            onclick={() => actions.close(tab.key)}
          >
            <Icon name="x" size={11} />
          </button>
        </div>
      {/each}
      {/if}
    </div>
    <button class="grid size-[30px] shrink-0 place-items-center rounded-[9px] text-muted hover:bg-tab hover:text-fg" aria-label="New local shell tab" title="New local shell tab" onclick={actions.newShell}>
      <Icon name="plus" size={14} />
    </button>
  </div>

  <div class="min-w-4 flex-1 self-stretch"></div>

  <button class={pill} aria-keyshortcuts={keybindings.aria('quickConnect')} onclick={onSearch}>
    <Icon name="search" size={14} />
    <span class="whitespace-nowrap">Jump to anything</span>
    {#if keybindings.label('quickConnect')}<span class="kbd">{keybindings.label('quickConnect')}</span>{/if}
  </button>

  <div class="flex items-center gap-1.5">
    {#if updates.result?.newer}
      <button
        class="no-drag flex h-[30px] items-center gap-1.5 rounded-[10px] bg-accent-chip px-2.5 text-xs font-medium whitespace-nowrap text-accent-ink hover:bg-accent hover:text-on-accent"
        title="TuxShell {updates.result.latest} is available. Open the release page"
        onclick={() => void Browser.OpenURL(updates.result!.url)}
      >
        <Icon name="download" size={13} />
        Update {updates.result.latest}
      </button>
    {/if}
    {#if remoteDesktops.sessions.length || hostStore.list.some((h) => h.protocol === 'rdp')}
      <span class="relative">
        <button
          class="relative {wc} {rdpOpen ? 'bg-accent-chip text-accent-ink' : ''}"
          aria-label="Remote desktops{remoteDesktops.open ? ` (${remoteDesktops.open} open)` : ''}"
          aria-expanded={rdpOpen}
          title="Remote desktops"
          onclick={() => (rdpOpen = !rdpOpen)}
        >
          <Icon name="monitor" size={14} />
          {#if remoteDesktops.open}<span class="absolute -top-0.5 -right-0.5 grid min-w-3.5 place-items-center rounded-full bg-success px-1 text-[9px] leading-3.5 font-bold text-ground">{remoteDesktops.open}</span>{/if}
        </button>
        {#if rdpOpen}<RdpSessions onClose={() => (rdpOpen = false)} />{/if}
      </span>
    {/if}
    <button
      class="{wc} {clipboardOpen ? 'bg-accent-chip text-accent-ink' : ''}"
      aria-label="Clipboard history"
      aria-pressed={clipboardOpen}
      aria-keyshortcuts={keybindings.aria('toggleClipboard')}
      title={keybindings.title('Clipboard history', 'toggleClipboard')}
      onclick={onToggleClipboard}
    >
      <Icon name="clipboard" size={14} />
    </button>
    <button
      class="relative {wc} {filesOpen ? 'bg-accent-chip text-accent-ink' : ''}"
      aria-label={edits.changed ? 'Files (an edited file is waiting to be uploaded)' : 'Files'}
      aria-pressed={filesOpen}
      aria-keyshortcuts={keybindings.aria('toggleFiles')}
      title={keybindings.title(edits.changed ? 'Files: an edited file is waiting to be uploaded' : 'Files', 'toggleFiles')}
      onclick={onToggleFiles}
    >
      <Icon name="folder" size={14} />
      {#if edits.changed}<span class="absolute top-1 right-1 size-1.5 rounded-full bg-warning" aria-hidden="true"></span>{/if}
    </button>
    <button
      class={wc}
      aria-label="Theme: {theme.mode}. Switch theme"
      title="Theme: {theme.mode === 'system' ? 'follow system' : theme.mode} (click to change)"
      onclick={() => prefs.cycleTheme()}
    >
      <Icon name={theme.mode === 'dark' ? 'moon' : theme.mode === 'light' ? 'sun' : 'monitor'} size={14} />
    </button>
    <button class={wc} aria-label="Preferences" aria-keyshortcuts={keybindings.aria('preferences')} title={keybindings.title('Preferences', 'preferences')} onclick={onPreferences}>
      <Icon name="gear" size={14} />
    </button>
  </div>

  {#if !isMac}
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
  {/if}
</header>

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} label="Tab actions" items={tabMenu(tabs, menu.key, actions)} onClose={() => (menu = null)} />
{/if}

<style>
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
