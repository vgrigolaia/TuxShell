<script lang="ts">
  import TerminalPane from './TerminalPane.svelte'
  import Icon from './Icon.svelte'
  import { workspace, type Pane } from '../workspace.svelte'
  import { MAX_PANES, dividers, drag, rects, type Divider } from '../layout'
  import { hostStore } from '../hosts.svelte'
  import type { PaneActions } from '../tabs'
  import { isTerminalReport } from '../broadcast'
  import { prefs } from '../settings.svelte'
  import { closer } from '../close.svelte'
  import { schemeBackground } from '../schemes'

  interface Props {
    /** Registered so clipboard history can paste into the focused pane. */
    refs: Record<number, TerminalPane>
    /** Right-click menu actions for a pane. */
    actions: (paneId: number) => PaneActions
  }
  let { refs = $bindable(), actions }: Props = $props()

  let area: HTMLDivElement
  const tileBtn = 'grid size-6 place-items-center rounded-md text-subtle hover:bg-hop hover:text-fg aria-pressed:text-accent-text'
  // A chosen colour scheme paints the gap around the terminal too.
  const paneBackground = $derived(schemeBackground(prefs.value.terminal.colorScheme))

  // Every pane is rendered exactly once, keyed by id; tabs only decide where
  // it sits. Merging or splitting tabs therefore never restarts a session.
  function placement(pane: Pane) {
    const tab = workspace.tabOf(pane.id)
    if (!tab) return null
    const i = tab.paneIds.indexOf(pane.id)
    // A maximized pane fills the tab; the others stay mounted but hidden.
    const max = tab.maximized
    return {
      tab,
      rect: max === pane.id ? { left: 0, top: 0, width: 100, height: 100 } : rects(tab.paneIds.length, tab.orientation, tab.sizes)[i],
      visible: tab.key === workspace.activeKey && (max === null || max === pane.id),
      maximized: max === pane.id,
      split: tab.paneIds.length > 1,
      focused: tab.activePane === pane.id,
      broadcasting: tab.broadcast && !tab.excluded.includes(pane.id),
      excluded: tab.broadcast && tab.excluded.includes(pane.id),
    }
  }

  // Typing in a broadcasting pane goes to every included, still-connected pane.
  function broadcastInput(paneId: number, data: string): boolean {
    const targets = workspace.broadcastTargets(paneId)
    if (!targets || isTerminalReport(data)) return false
    for (const id of targets) if (!workspace.pane(id)?.exited) refs[id]?.send(data)
    return true
  }

  const activeDividers = $derived(
    workspace.activeTab && workspace.activeTab.maximized === null ? dividers(workspace.activeTab.paneIds.length, workspace.activeTab.orientation, workspace.activeTab.sizes) : [],
  )

  function setSize(d: Divider, pos: number) {
    const tab = workspace.activeTab
    if (tab) tab.sizes = drag(tab.paneIds.length, tab.sizes, d, pos, tab.orientation)
  }

  function onDividerDown(e: PointerEvent, d: Divider) {
    const el = e.currentTarget as HTMLElement
    el.setPointerCapture(e.pointerId)
    const move = (ev: PointerEvent) => {
      const r = area.getBoundingClientRect()
      setSize(d, d.axis === 'x' ? (ev.clientX - r.left) / r.width : (ev.clientY - r.top) / r.height)
    }
    const up = () => {
      el.removeEventListener('pointermove', move)
      el.removeEventListener('pointerup', up)
    }
    el.addEventListener('pointermove', move)
    el.addEventListener('pointerup', up)
  }

  // Arrow keys nudge a focused divider by 5%.
  function onDividerKey(e: KeyboardEvent, d: Divider) {
    const step = { ArrowLeft: -0.05, ArrowUp: -0.05, ArrowRight: 0.05, ArrowDown: 0.05 }[e.key]
    if (step === undefined) return
    e.preventDefault()
    setSize(d, d.at / 100 + step)
  }
</script>

<!-- -m-[5px]: each tile pads 5px, so neighbours sit 10px apart and the outer
     edges line up with the panel padding. -->
<div bind:this={area} id="terminal-area" role="tabpanel" aria-labelledby="tab-{workspace.activeKey}" class="relative -m-[5px] min-h-0 flex-1">
  {#each workspace.panes as pane (pane.id)}
    {@const at = placement(pane)}
    {#if at}
      {@const host = pane.source.kind === 'host' ? hostStore.get(pane.source.hostId) : undefined}
      {@const jumps = host?.proxyJump ? host.proxyJump.split(',').map((j) => j.trim()).filter(Boolean) : []}
      <!-- Focus follows clicks and keyboard focus inside the pane. -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="absolute p-[5px]"
        class:hidden={!at.visible}
        data-pane-id={pane.id}
        style:left="{at.rect.left}%"
        style:top="{at.rect.top}%"
        style:width="{at.rect.width}%"
        style:height="{at.rect.height}%"
        onpointerdown={() => workspace.focusPane(pane.id)}
        onfocusin={() => workspace.focusPane(pane.id)}
      >
        <section
          aria-label="{pane.title} terminal"
          class="group/tile flex h-full flex-col overflow-hidden rounded-[14px] border-[1.5px] bg-field transition-[border-color,box-shadow] {at.split && at.focused
            ? 'tile-focused border-accent'
            : at.broadcasting
              ? 'border-accent-line'
              : 'border-line'} {at.excluded ? 'opacity-70' : ''}"
          style:background={paneBackground}
        >
          <header class="flex min-h-9 shrink-0 flex-wrap items-center gap-2 border-b border-line px-3 py-1.5 text-xs">
            <span class="size-2 shrink-0 rounded-full {pane.exited ? 'border-2 border-warning' : 'bg-success'}" aria-hidden="true"></span>
            <span class="truncate font-semibold {at.focused ? 'text-fg' : 'text-text'}">
              {#if at.maximized}<span class="font-normal text-accent-text">{at.tab.paneIds.length - 1} hidden · </span>{/if}{pane.source.kind === 'shell' && !pane.customTitle ? 'Local shell' : pane.title}
            </span>
            {#if pane.source.kind === 'host'}
              <!-- The route the connection takes: you → jump host(s) → this host. -->
              <span class="ml-1.5 flex min-w-0 items-center gap-1" aria-label="Route: {['you', ...jumps, pane.title].join(' to ')}">
                {#if at.focused}<span class="hop">{pane.loginUser || host?.user || 'you'}</span><span class="text-subtle" aria-hidden="true">→</span>{/if}
                {#each jumps as j (j)}
                  <span class="hop tint-lime">{j}</span><span class="text-subtle" aria-hidden="true">→</span>
                {/each}
                <span class="hop truncate {at.focused ? 'hop-active' : ''}">{host?.hostname || pane.title}</span>
              </span>
            {:else if pane.termTitle}
              <span class="ml-1.5 truncate font-mono text-[11px] text-subtle">{pane.termTitle}</span>
            {/if}
            <span class="flex-1"></span>
            {#if at.broadcasting || at.excluded}
              <button
                class="hop {at.broadcasting ? 'hop-active' : ''} hover:brightness-125"
                aria-pressed={at.broadcasting}
                aria-label="{at.broadcasting ? 'Exclude' : 'Include'} {pane.title} {at.broadcasting ? 'from' : 'in'} multi-exec"
                title={at.broadcasting ? 'Receiving typing — click to exclude' : 'Excluded — click to include'}
                onclick={() => workspace.toggleExcluded(pane.id)}
              >
                {at.broadcasting ? 'receiving' : 'excluded'}
              </button>
            {/if}
            <!-- Tile actions show on hover/focus to keep headers calm, as in the design. -->
            <span class="hidden items-center gap-0.5 group-focus-within/tile:flex group-hover/tile:flex">
              {#if at.tab.paneIds.length < MAX_PANES}
                <button class={tileBtn} aria-label="Split {pane.title} right" title="Split right" onclick={() => actions(pane.id).split?.('row')}>
                  <Icon name="splitRight" size={13} />
                </button>
                <button class={tileBtn} aria-label="Split {pane.title} down" title="Split down" onclick={() => actions(pane.id).split?.('column')}>
                  <Icon name="splitDown" size={13} />
                </button>
              {/if}
              {#if at.split}
                <button
                  class={tileBtn}
                  aria-label="Multi-exec: type into all tiles"
                  aria-pressed={at.tab.broadcast}
                  title="Multi-exec: type into all tiles (Ctrl+Shift+B)"
                  onclick={() => workspace.toggleBroadcast(at.tab.key)}
                >
                  <Icon name="broadcast" size={13} />
                </button>
                <button
                  class={tileBtn}
                  aria-label="{at.maximized ? 'Restore split' : 'Maximize'} {pane.title}"
                  aria-pressed={at.maximized}
                  title="{at.maximized ? 'Restore split' : 'Maximize pane'} (Ctrl+Shift+X)"
                  onclick={() => workspace.toggleMaximized(pane.id)}
                >
                  <Icon name={at.maximized ? 'restore' : 'maximize'} size={12} />
                </button>
                <button class={tileBtn} aria-label="Move {pane.title} to its own tab" title="Move to own tab" onclick={() => workspace.detachPane(pane.id)}>
                  <Icon name="detach" size={12} />
                </button>
              {/if}
              <button class={tileBtn} aria-label="Close {pane.title}" title="Close (Ctrl+Shift+W)" onclick={() => closer.closePane(pane.id)}>
                <Icon name="x" size={12} />
              </button>
            </span>
          </header>
          <div class="min-h-0 flex-1 py-2 pl-3">
            <TerminalPane
              bind:this={refs[pane.id]}
              source={pane.source}
              visible={at.visible}
              focused={at.visible && at.focused}
              onStart={() => workspace.updatePane(pane.id, { exited: false, startedAt: Date.now() })}
              onInput={(data) => broadcastInput(pane.id, data)}
              onLogin={(user) => workspace.updatePane(pane.id, { loginUser: user })}
              onTitle={(title) =>
                workspace.updatePane(pane.id, pane.source.kind === 'shell' && !pane.customTitle ? { title, termTitle: title } : { termTitle: title })}
              onSize={(cols, rows) => workspace.updatePane(pane.id, { size: `${cols}×${rows}` })}
              onExit={() => workspace.updatePane(pane.id, { exited: true })}
              readOnly={pane.readOnly}
              showScrollbar={pane.showScrollbar}
              split={at.split}
              maximized={at.maximized}
              actions={actions(pane.id)}
            />
          </div>
        </section>
      </div>
    {/if}
  {/each}

  {#each activeDividers as d (d.axis + d.index)}
    <!-- A focusable separator with a value is an interactive widget in ARIA
         (window splitter pattern); Svelte's checker treats every separator as static. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
    <div
      role="separator"
      tabindex="0"
      aria-orientation={d.axis === 'x' ? 'vertical' : 'horizontal'}
      aria-label="Resize panes"
      aria-valuenow={Math.round(d.at)}
      aria-valuemin={0}
      aria-valuemax={100}
      class="group absolute z-20 {d.axis === 'x' ? 'top-0 bottom-0 w-2 -translate-x-1/2 cursor-col-resize' : 'right-0 left-0 h-2 -translate-y-1/2 cursor-row-resize'}"
      style:left={d.axis === 'x' ? `${d.at}%` : undefined}
      style:top={d.axis === 'y' ? `${d.at}%` : d.from ? `${d.from}%` : undefined}
      onpointerdown={(e) => onDividerDown(e, d)}
      onkeydown={(e) => onDividerKey(e, d)}
    >
      <span class="absolute rounded-full bg-accent opacity-0 transition-opacity group-hover:opacity-60 group-focus-visible:opacity-100 {d.axis === 'x' ? 'inset-y-2 left-1/2 w-0.5 -translate-x-1/2' : 'inset-x-2 top-1/2 h-0.5 -translate-y-1/2'}"></span>
    </div>
  {/each}
</div>
