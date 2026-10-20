<script lang="ts" module>
  import type { IconName } from './Icon.svelte'

  export interface MenuItem {
    label: string
    run: () => void
    danger?: boolean
    /** Greyed out and skipped by the keyboard. */
    disabled?: boolean
    /** Fixed key shown on the right, e.g. "F2". */
    hint?: string
    /** A changeable shortcut's combo (keybindings.combo(id)); shown instead of hint. */
    shortcut?: string
    /** Draws a divider above this item. */
    separator?: boolean
    /** Makes it a checkbox item (ticked when true). */
    checked?: boolean
    icon?: IconName
  }
</script>

<script lang="ts">
  import { isMac, keys } from '../platform'
  import { ariaCombo, formatCombo } from '../keybindings.svelte'
  import { onMount } from 'svelte'
  import Icon from './Icon.svelte'

  interface Props {
    x: number
    y: number
    label: string
    items: MenuItem[]
    onClose: () => void
  }
  let { x, y, label, items, onClose }: Props = $props()

  let menu: HTMLUListElement
  let pos = $state({ left: 0, top: 0 })
  // Remember what had focus so it can be restored when the menu closes.
  const opener = document.activeElement as HTMLElement | null

  onMount(() => {
    // Keep the menu inside the window.
    const r = menu.getBoundingClientRect()
    pos = { left: Math.min(x, innerWidth - r.width - 8), top: Math.min(y, innerHeight - r.height - 8) }
    focusItem(0)
  })

  function focusItem(i: number) {
    const buttons = menu.querySelectorAll<HTMLButtonElement>('[role^="menuitem"]:not(:disabled)')
    buttons[(i + buttons.length) % buttons.length]?.focus()
  }

  function close() {
    onClose()
    opener?.focus()
  }

  function onKey(e: KeyboardEvent) {
    const buttons = [...menu.querySelectorAll<HTMLButtonElement>('[role^="menuitem"]:not(:disabled)')]
    const i = buttons.indexOf(document.activeElement as HTMLButtonElement)
    if (e.key === 'ArrowDown') focusItem(i + 1)
    else if (e.key === 'ArrowUp') focusItem(i - 1)
    else if (e.key === 'Home') focusItem(0)
    else if (e.key === 'End') focusItem(-1)
    else if (e.key === 'Escape' || e.key === 'Tab') close()
    else return
    e.preventDefault()
  }
</script>

<svelte:window onblur={close} onresize={close} />

<!-- Full-window catcher closes the menu on any outside click. -->
<div class="fixed inset-0 z-40" role="presentation" onmousedown={close} oncontextmenu={(e) => (e.preventDefault(), close())}></div>

<ul
  bind:this={menu}
  role="menu"
  aria-label={label}
  tabindex="-1"
  class="fixed z-50 min-w-44 rounded-xl border border-line bg-raised p-1 shadow-2xl"
  style:left="{pos.left}px"
  style:top="{pos.top}px"
  onkeydown={onKey}
>
  {#each items as item (item.label)}
    {#if item.separator}<li role="separator" class="mx-2 my-1 border-t border-line"></li>{/if}
    <li role="none">
      <button
        role={item.checked === undefined ? 'menuitem' : 'menuitemcheckbox'}
        aria-checked={item.checked}
        disabled={item.disabled}
        aria-keyshortcuts={item.shortcut ? ariaCombo(item.shortcut) : item.hint?.replaceAll('Ctrl', isMac ? 'Meta' : 'Control')}
        class="flex w-full items-center gap-6 rounded-lg px-3 py-1.5 text-left text-[13px] focus:outline-none disabled:pointer-events-none disabled:text-subtle {item.danger
          ? 'text-danger-text hover:bg-danger/15 focus:bg-danger/15'
          : 'hover:bg-line focus:bg-line'}"
        onclick={() => {
          onClose()
          item.run()
        }}
      >
        {#if item.checked !== undefined}
          <!-- Checkbox items show their state where icons go. -->
          <span class="-mr-3 grid size-4 place-items-center rounded border {item.checked ? 'border-accent bg-accent text-on-accent' : 'border-subtle'}" aria-hidden="true">
            {#if item.checked}<Icon name="check" size={11} />{/if}
          </span>
        {:else if items.some((i) => i.icon || i.checked !== undefined)}
          <span class="-mr-3 grid size-4 place-items-center text-muted" aria-hidden="true">{#if item.icon}<Icon name={item.icon} size={15} />{/if}</span>
        {/if}
        <span class="flex-1">{item.label}</span>
        {#if item.shortcut}<span class="font-mono text-[11px] text-subtle">{formatCombo(item.shortcut)}</span>
        {:else if item.hint}<span class="font-mono text-[11px] text-subtle">{keys(item.hint)}</span>{/if}
      </button>
    </li>
  {/each}
</ul>
