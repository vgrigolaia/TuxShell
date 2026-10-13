<script lang="ts">
  import TerminalPane from './TerminalPane.svelte'
  import Icon from './Icon.svelte'
  import { workspace, type Pane } from '../workspace.svelte'
  import { dividers, drag, rects, type Divider } from '../layout'

  interface Props {
    /** Registered so clipboard history can paste into the focused pane. */
    refs: Record<number, TerminalPane>
  }
  let { refs = $bindable() }: Props = $props()

  let area: HTMLDivElement

  // Every pane is rendered exactly once, keyed by id; tabs only decide where
  // it sits. Merging or splitting tabs therefore never restarts a session.
  function placement(pane: Pane) {
    const tab = workspace.tabOf(pane.id)
    if (!tab) return null
    const i = tab.paneIds.indexOf(pane.id)
    return {
      tab,
      rect: rects(tab.paneIds.length, tab.orientation, tab.sizes)[i],
      visible: tab.key === workspace.activeKey,
      split: tab.paneIds.length > 1,
      focused: tab.activePane === pane.id,
    }
  }

  const activeDividers = $derived(
    workspace.activeTab ? dividers(workspace.activeTab.paneIds.length, workspace.activeTab.orientation, workspace.activeTab.sizes) : [],
  )

  function setSize(d: Divider, pos: number) {
    const tab = workspace.activeTab
    if (tab) tab.sizes = drag(tab.paneIds.length, tab.sizes, d, pos)
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

<div bind:this={area} id="terminal-area" role="tabpanel" aria-labelledby="tab-{workspace.activeKey}" class="relative min-h-0 flex-1">
  {#each workspace.panes as pane (pane.id)}
    {@const at = placement(pane)}
    {#if at}
      <!-- Focus follows clicks and keyboard focus inside the pane. -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="absolute p-1"
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
          aria-label={pane.title}
          class="flex h-full flex-col overflow-hidden rounded-xl border transition-colors {at.split && at.focused
            ? 'border-accent/70'
            : at.split
              ? 'border-line'
              : 'border-transparent'}"
        >
          {#if at.split}
            <header class="flex h-7 shrink-0 items-center gap-2 border-b border-line px-2.5">
              <span class="size-1.5 shrink-0 rounded-full {pane.exited ? 'bg-subtle' : 'bg-success'}" aria-hidden="true"></span>
              <span class="min-w-0 flex-1 truncate font-mono text-[11px] {at.focused ? 'text-fg' : 'text-muted'}">
                {pane.title}{#if pane.termTitle && pane.termTitle !== pane.title}<span class="text-subtle"> · {pane.termTitle}</span>{/if}
              </span>
              <button class="grid size-5 place-items-center rounded text-subtle hover:bg-line hover:text-fg" aria-label="Move {pane.title} to its own tab" title="Move to own tab" onclick={() => workspace.detachPane(pane.id)}>
                <Icon name="detach" size={12} />
              </button>
              <button class="grid size-5 place-items-center rounded text-subtle hover:bg-line hover:text-fg" aria-label="Close {pane.title}" title="Close pane" onclick={() => workspace.closePane(pane.id)}>
                <Icon name="x" size={12} />
              </button>
            </header>
          {/if}
          <div class="min-h-0 flex-1 py-1 pl-2">
            <TerminalPane
              bind:this={refs[pane.id]}
              source={pane.source}
              visible={at.visible}
              focused={at.visible && at.focused}
              onStart={() => workspace.updatePane(pane.id, { exited: false })}
              onTitle={(title) =>
                workspace.updatePane(pane.id, pane.source.kind === 'shell' ? { title, termTitle: title } : { termTitle: title })}
              onSize={(cols, rows) => workspace.updatePane(pane.id, { size: `${cols}×${rows}` })}
              onExit={() => workspace.updatePane(pane.id, { exited: true })}
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
      style:top={d.axis === 'y' ? `${d.at}%` : undefined}
      onpointerdown={(e) => onDividerDown(e, d)}
      onkeydown={(e) => onDividerKey(e, d)}
    >
      <span class="absolute rounded-full bg-accent opacity-0 transition-opacity group-hover:opacity-60 group-focus-visible:opacity-100 {d.axis === 'x' ? 'inset-y-2 left-1/2 w-0.5 -translate-x-1/2' : 'inset-x-2 top-1/2 h-0.5 -translate-y-1/2'}"></span>
    </div>
  {/each}
</div>
