<script lang="ts">
  import { onDestroy } from 'svelte'
  import Icon from './Icon.svelte'
  import HostBadge from './HostBadge.svelte'
  import ContextMenu from './ContextMenu.svelte'
  import type { TabInfo } from '../tabs'
  import { tabMenu, tabPicker, type TabActions } from '../tabbar.svelte'
  import { hostStore } from '../hosts.svelte'

  interface Props {
    tabs: TabInfo[]
    activeKey: number
    homeActive: boolean
    actions: TabActions
    /** "list" in the left panel (and Home); "rail" is the narrow strip when the panel is hidden. */
    variant: 'list' | 'rail'
  }
  let { tabs, activeKey, homeActive, actions, variant }: Props = $props()

  const picked = tabPicker.picked
  let menu = $state<{ x: number; y: number; key: number } | null>(null)
  let list = $state<HTMLElement>()

  $effect(() => tabPicker.prune(tabs))

  // Keep the active tab in view when the list scrolls.
  $effect(() => {
    const key = activeKey
    requestAnimationFrame(() => document.getElementById(`${variant}-tab-${key}`)?.scrollIntoView({ block: 'nearest' }))
  })

  function openMenu(e: MouseEvent, key: number) {
    e.preventDefault()
    if (!picked.has(key)) picked.clear()
    menu = { x: e.clientX, y: e.clientY, key }
  }

  // Arrow keys move between tabs, per the WAI-ARIA tabs pattern (vertical).
  function onKey(e: KeyboardEvent, index: number) {
    const step = e.key === 'ArrowDown' ? 1 : e.key === 'ArrowUp' ? -1 : 0
    if (!step) return
    e.preventDefault()
    const next = tabs[(index + step + tabs.length) % tabs.length]
    actions.select(next.key)
    document.getElementById(`${variant}-tab-${next.key}`)?.focus()
  }

  // Drag to reorder, with the pointer (HTML drag and drop is taken by file drops).
  // slot is where the tab would go: before row slot, or tabs.length for the end.
  let drag = $state<{ key: number; from: number; startY: number; slot: number; moved: boolean } | null>(null)
  let justDragged = false

  function onPointerDown(e: PointerEvent, key: number, index: number) {
    if (e.button !== 0 || e.ctrlKey || e.metaKey || tabs.length < 2) return
    drag = { key, from: index, startY: e.clientY, slot: index, moved: false }
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
  }

  function onPointerMove(e: PointerEvent) {
    if (!drag || !list) return
    if (!drag.moved && Math.abs(e.clientY - drag.startY) < 5) return
    drag.moved = true
    const rows = [...list.querySelectorAll<HTMLElement>('[data-tab-row]')]
    let slot = rows.length
    for (const [i, row] of rows.entries()) {
      const r = row.getBoundingClientRect()
      if (e.clientY < r.top + r.height / 2) {
        slot = i
        break
      }
    }
    drag.slot = slot
  }

  function onPointerUp() {
    stopDrag()
  }

  function stopDrag() {
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', onPointerUp)
    if (drag?.moved) {
      justDragged = true
      setTimeout(() => (justDragged = false))
      const to = drag.slot > drag.from ? drag.slot - 1 : drag.slot
      actions.move(drag.key, to)
    }
    drag = null
  }
  onDestroy(stopDrag)

  function onClick(e: MouseEvent, key: number) {
    if (justDragged) return
    tabPicker.click(e, key, activeKey, actions.select)
  }

  // Where the drop marker shows: above row i, or below the last one.
  const markAbove = (i: number) => drag?.moved && drag.slot === i && drag.slot !== drag.from && drag.slot !== drag.from + 1
  const markBelow = (i: number) => drag?.moved && i === tabs.length - 1 && drag.slot === tabs.length && drag.from !== tabs.length - 1

  const describe = (tab: TabInfo) => `${tab.title}${tab.exited ? ' (disconnected)' : ''}${tab.panes > 1 ? ` · ${tab.panes} tiles` : ''}`
</script>

{#snippet icon(tab: TabInfo, size: number)}
  {@const host = tab.hostId != null ? hostStore.get(tab.hostId) : undefined}
  {#if host}
    <HostBadge name={host.name} os={host.os} {size} dim={tab.exited} />
  {:else}
    <span class="grid shrink-0 place-items-center rounded-md bg-hop text-muted" style="width:{size}px;height:{size}px" aria-hidden="true">
      <Icon name={tab.kind === 'shell' ? 'terminal' : tab.kind === 'rdp' ? 'monitor' : 'server'} size={Math.round(size * 0.55)} />
    </span>
  {/if}
{/snippet}

{#snippet status(tab: TabInfo)}
  {#if tab.bell}<span class="shrink-0 text-warning" role="img" aria-label="bell rang"><Icon name="bell" size={11} /></span>{/if}
  {#if tab.exited}
    <span class="size-2 shrink-0 rounded-full border-2 border-warning" role="img" aria-label="disconnected"></span>
  {:else}
    <span class="size-2 shrink-0 rounded-full bg-success" role="img" aria-label="connected"></span>
  {/if}
{/snippet}

{#if variant === 'list'}
  <section aria-label="Open tabs" class="flex min-h-0 flex-col gap-1">
    <div class="flex items-center gap-1 px-1.5">
      <h2 class="cap flex-1">Open{tabs.length ? ` · ${tabs.length}` : ''}</h2>
      <button
        class="grid size-6 place-items-center rounded-md {homeActive ? 'bg-accent-chip text-accent-ink' : 'text-subtle hover:bg-raised hover:text-fg'}"
        aria-label="Home"
        aria-pressed={homeActive}
        title="Home: all hosts"
        onclick={actions.home}
      >
        <Icon name="home" size={13} />
      </button>
      <button class="grid size-6 place-items-center rounded-md text-subtle hover:bg-raised hover:text-fg" aria-label="New local shell tab" title="New local shell tab" onclick={actions.newShell}>
        <Icon name="plus" size={14} />
      </button>
    </div>
    {#if tabs.length}
      <!-- Reaches into the panel's right padding so the scrollbar doesn't cover the close buttons. -->
      <div
        bind:this={list}
        role="tablist"
        aria-label="Open tabs"
        aria-orientation="vertical"
        aria-multiselectable="true"
        class="-mr-2.5 flex min-h-0 flex-col gap-0.5 overflow-y-auto pr-2.5"
        class:select-none={drag?.moved}
      >
        {#each tabs as tab, i (tab.key)}
          {@const current = tab.key === activeKey && !homeActive}
          <!-- Middle-click closes, like browser and terminal tabs. -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            data-tab-row
            class="group relative flex h-9 shrink-0 items-center rounded-[10px] transition-colors {current ? 'bg-select text-fg' : 'text-text hover:bg-raised'} {picked.has(tab.key) ? 'ring-1 ring-accent' : ''} {drag?.moved && drag.key === tab.key ? 'opacity-50' : ''}"
            class:mark-above={markAbove(i)}
            class:mark-below={markBelow(i)}
            onauxclick={(e) => e.button === 1 && (e.preventDefault(), actions.close(tab.key))}
            oncontextmenu={(e) => openMenu(e, tab.key)}
            onpointerdown={(e) => onPointerDown(e, tab.key, i)}
          >
            <button
              id="list-tab-{tab.key}"
              role="tab"
              aria-selected={current || picked.has(tab.key)}
              aria-controls="panel-{tab.key}"
              tabindex={current ? 0 : -1}
              class="flex h-full min-w-0 flex-1 items-center gap-2.5 pl-2 text-left text-[13px]"
              title="{describe(tab)} — Ctrl+click to select, drag to move, right-click to split or close"
              onclick={(e) => onClick(e, tab.key)}
              onkeydown={(e) => onKey(e, i)}
            >
              {@render icon(tab, 22)}
              <span class="min-w-0 flex-1 truncate {tab.exited ? 'text-subtle' : ''}">{tab.title}</span>
              {#if tab.panes > 1}<span class="shrink-0 text-[11px] text-subtle" aria-label="{tab.panes} tiles"><Icon name="splitRight" size={12} /></span>{/if}
              {@render status(tab)}
            </button>
            <button
              class="mx-1 grid size-6 shrink-0 place-items-center rounded-md text-subtle opacity-0 group-hover:opacity-100 hover:bg-line hover:text-fg focus-visible:opacity-100"
              aria-label="Close {tab.title}"
              tabindex="-1"
              onclick={() => actions.close(tab.key)}
            >
              <Icon name="x" size={11} />
            </button>
          </div>
        {/each}
      </div>
    {:else}
      <p class="px-2 py-1 text-xs text-subtle">No open tabs. Double-click a host to connect.</p>
    {/if}
  </section>
{:else}
  <nav aria-label="Open tabs" class="flex w-12 shrink-0 flex-col items-center gap-1.5 rounded-panel border border-line bg-panel py-2.5">
    <button
      class="grid size-8 shrink-0 place-items-center rounded-[10px] {homeActive ? 'bg-accent-chip text-accent-ink' : 'text-muted hover:bg-raised hover:text-fg'}"
      aria-label="Home"
      aria-pressed={homeActive}
      title="Home: all hosts"
      onclick={actions.home}
    >
      <Icon name="home" size={15} />
    </button>
    <span class="h-px w-6 shrink-0 bg-line" aria-hidden="true"></span>
    <div
      bind:this={list}
      role="tablist"
      aria-label="Open tabs"
      aria-orientation="vertical"
      class="rail flex min-h-0 flex-col items-center gap-1.5 overflow-y-auto"
      class:select-none={drag?.moved}
    >
      {#each tabs as tab, i (tab.key)}
        {@const current = tab.key === activeKey && !homeActive}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          data-tab-row
          class="relative shrink-0 rounded-[10px] {drag?.moved && drag.key === tab.key ? 'opacity-50' : ''}"
          class:mark-above={markAbove(i)}
          class:mark-below={markBelow(i)}
          onauxclick={(e) => e.button === 1 && (e.preventDefault(), actions.close(tab.key))}
          oncontextmenu={(e) => openMenu(e, tab.key)}
          onpointerdown={(e) => onPointerDown(e, tab.key, i)}
        >
          <button
            id="rail-tab-{tab.key}"
            role="tab"
            aria-selected={current || picked.has(tab.key)}
            aria-label={describe(tab)}
            aria-controls="panel-{tab.key}"
            tabindex={current ? 0 : -1}
            class="grid size-9 place-items-center rounded-[10px] {current ? 'bg-select ring-1 ring-accent' : 'hover:bg-raised'} {picked.has(tab.key) ? 'ring-1 ring-accent' : ''}"
            title={describe(tab)}
            onclick={(e) => onClick(e, tab.key)}
            onkeydown={(e) => onKey(e, i)}
          >
            {@render icon(tab, 24)}
          </button>
          <span
            class="pointer-events-none absolute right-0.5 bottom-0.5 size-2 rounded-full ring-2 ring-panel {tab.exited ? 'bg-warning' : 'bg-success'}"
            aria-hidden="true"
          ></span>
        </div>
      {/each}
    </div>
    <button class="grid size-8 shrink-0 place-items-center rounded-[10px] text-muted hover:bg-raised hover:text-fg" aria-label="New local shell tab" title="New local shell tab" onclick={actions.newShell}>
      <Icon name="plus" size={15} />
    </button>
  </nav>
{/if}

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} label="Tab actions" items={tabMenu(tabs, menu.key, actions)} onClose={() => (menu = null)} />
{/if}

<style>
  /* Where a dragged tab will land. */
  .mark-above {
    box-shadow: inset 0 2px 0 var(--color-accent);
  }
  .mark-below {
    box-shadow: inset 0 -2px 0 var(--color-accent);
  }
  .rail {
    scrollbar-width: none;
  }
  .rail::-webkit-scrollbar {
    display: none;
  }
</style>
