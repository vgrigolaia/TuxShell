<script lang="ts" module>
  export interface MenuItem {
    label: string
    run: () => void
    danger?: boolean
  }
</script>

<script lang="ts">
  import { onMount } from 'svelte'

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
    const buttons = menu.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    buttons[(i + buttons.length) % buttons.length]?.focus()
  }

  function close() {
    onClose()
    opener?.focus()
  }

  function onKey(e: KeyboardEvent) {
    const buttons = [...menu.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')]
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
    <li role="none">
      <button
        role="menuitem"
        class="w-full rounded-lg px-3 py-1.5 text-left text-[13px] focus:outline-none {item.danger
          ? 'text-danger-text hover:bg-danger/15 focus:bg-danger/15'
          : 'hover:bg-line focus:bg-line'}"
        onclick={() => {
          onClose()
          item.run()
        }}
      >
        {item.label}
      </button>
    </li>
  {/each}
</ul>
