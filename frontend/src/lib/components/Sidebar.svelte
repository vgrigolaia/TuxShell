<script lang="ts">
  import { keys } from '../platform'
  import { JUMP_GROUP, NO_GROUP, describe, hostStore, type Host, type HostGroup } from '../hosts.svelte'
  import { MAX_PANES } from '../layout'
  import { workspace } from '../workspace.svelte'
  import HostBadge from './HostBadge.svelte'
  import StatusDot from './StatusDot.svelte'
  import { hostStatus } from '../status.svelte'
  import { best } from '../fuzzy'
  import { SvelteSet } from 'svelte/reactivity'
  import { onDestroy, tick } from 'svelte'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import PromptDialog from './PromptDialog.svelte'
  import Icon from './Icon.svelte'
  import type { Snippet } from 'svelte'

  interface Props {
    selectedId: number | null
    onConnect: (host: Host) => void
    onEdit: (host: Host) => void
    onConnectAs: (host: Host) => void
    onDuplicate: (host: Host) => void
    onDelete: (host: Host) => void
    onNew: () => void
    onImport: () => void
    /** Opens the selected hosts as tiles of one tab. */
    onOpenTiles: (hosts: Host[]) => void
    /** The open tabs, shown above the hosts (Preferences → Open tabs). */
    openTabs?: Snippet
  }
  let { selectedId, onConnect, onEdit, onConnectAs, onDuplicate, onDelete, onNew, onImport, onOpenTiles, openTabs }: Props = $props()

  const COLLAPSED_KEY = 'tuxshell.sidebar.collapsed'

  let filter = $state('')
  let collapsed = $state<Record<string, boolean>>(loadCollapsed())
  let menu = $state<{ x: number; y: number; host: Host } | null>(null)

  // Selection mode for bulk delete.
  let selecting = $state(false)
  let confirmBulk = $state(false)
  const picked = new SvelteSet<number>()
  let anchor: number | null = null

  // Per-viewer UI preference only; the app works the same without it.
  function loadCollapsed(): Record<string, boolean> {
    try {
      return JSON.parse(localStorage.getItem(COLLAPSED_KEY) ?? '{}')
    } catch {
      return {}
    }
  }

  function toggle(group: string) {
    collapsed[group] = !collapsed[group]
    try {
      localStorage.setItem(COLLAPSED_KEY, JSON.stringify(collapsed))
    } catch {
      // Storage unavailable; the toggle still works for this session.
    }
  }

  const visible = $derived.by<HostGroup[]>(() => {
    const q = filter.trim()
    if (!q) return hostStore.groups
    return hostStore.groups
      .map((g) => ({
        name: g.name,
        hosts: g.hosts.filter((h) => best(q, [h.name, h.hostname, h.alias, g.name, ...(h.tags ?? [])]) > 0),
      }))
      .filter((g) => g.hosts.length > 0)
  })

  const visibleHosts = $derived(visible.flatMap((g) => g.hosts))

  // Reordering groups: drag a group's header (not while filtering or selecting,
  // when the list isn't the whole picture), its menu, or Alt+arrows.
  let list = $state<HTMLElement>()
  let groupMenu = $state<{ x: number; y: number; name: string } | null>(null)
  let gdrag = $state<{ name: string; from: number; startY: number; slot: number; moved: boolean } | null>(null)
  let justDragged = false
  const canReorder = $derived(!filter.trim() && !selecting && hostStore.groups.length > 1)

  async function moveGroup(name: string, index: number) {
    hostStore.moveGroup(name, index)
    await tick()
    document.getElementById(`group-head-${name}`)?.focus()
  }

  function onGroupPointerDown(e: PointerEvent, name: string, index: number) {
    if (e.button !== 0 || !canReorder) return
    gdrag = { name, from: index, startY: e.clientY, slot: index, moved: false }
    window.addEventListener('pointermove', onGroupPointerMove)
    window.addEventListener('pointerup', stopGroupDrag)
  }

  function onGroupPointerMove(e: PointerEvent) {
    if (!gdrag || !list) return
    if (!gdrag.moved && Math.abs(e.clientY - gdrag.startY) < 5) return
    gdrag.moved = true
    // The slot is decided by the headers: above a header's middle goes before that group.
    const heads = [...list.querySelectorAll<HTMLElement>('[data-group-head]')]
    let slot = heads.length
    for (const [i, h] of heads.entries()) {
      const r = h.getBoundingClientRect()
      if (e.clientY < r.top + r.height / 2) {
        slot = i
        break
      }
    }
    gdrag.slot = slot
    // Scroll when dragging near the list's edges.
    const box = list.getBoundingClientRect()
    if (e.clientY < box.top + 24) list.scrollTop -= 12
    else if (e.clientY > box.bottom - 24) list.scrollTop += 12
  }

  function stopGroupDrag() {
    window.removeEventListener('pointermove', onGroupPointerMove)
    window.removeEventListener('pointerup', stopGroupDrag)
    if (gdrag?.moved) {
      justDragged = true
      setTimeout(() => (justDragged = false))
      const to = gdrag.slot > gdrag.from ? gdrag.slot - 1 : gdrag.slot
      if (to !== gdrag.from) hostStore.moveGroup(gdrag.name, to)
    }
    gdrag = null
  }
  onDestroy(stopGroupDrag)

  function onGroupKey(e: KeyboardEvent, name: string, index: number) {
    if (e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey)) {
      e.preventDefault()
      const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
      groupMenu = { x: r.left + 24, y: r.bottom, name }
      return
    }
    if (!e.altKey || !canReorder) return
    const step = e.key === 'ArrowUp' ? -1 : e.key === 'ArrowDown' ? 1 : 0
    if (!step) return
    e.preventDefault()
    void moveGroup(name, index + step)
  }

  const groupMenuItems = (name: string): MenuItem[] => {
    const i = hostStore.groups.findIndex((g) => g.name === name)
    const last = hostStore.groups.length - 1
    return [
      { label: 'Move to top', disabled: !canReorder || i === 0, run: () => void moveGroup(name, 0) },
      { label: 'Move up', hint: 'Alt+↑', disabled: !canReorder || i === 0, run: () => void moveGroup(name, i - 1) },
      { label: 'Move down', hint: 'Alt+↓', disabled: !canReorder || i === last, run: () => void moveGroup(name, i + 1) },
      { label: 'Move to bottom', disabled: !canReorder || i === last, run: () => void moveGroup(name, last) },
      ...(name === NO_GROUP || name === JUMP_GROUP
        ? []
        : [
            { label: 'Rename…', separator: true, run: () => (renaming = name) },
            { label: 'Delete group…', danger: true, run: () => deleteGroup(name) },
          ]),
    ]
  }

  // Groups: "New group" adds an empty one to drag hosts into; Rename and Delete are in a group's menu.
  let newGroup = $state(false)
  let renaming = $state<string | null>(null)
  let deleting = $state<{ name: string; count: number } | null>(null)

  async function showGroup(name: string | undefined) {
    if (!name) return
    if (collapsed[name]) toggle(name)
    await tick()
    const head = document.getElementById(`group-head-${name}`)
    head?.scrollIntoView({ block: 'nearest' })
    head?.focus()
  }

  async function renameGroup(from: string, to: string) {
    const wasCollapsed = !!collapsed[from]
    await hostStore.renameGroup(from, to)
    const name = hostStore.findGroup(to.trim())
    if (wasCollapsed && name && name !== from) {
      delete collapsed[from]
      collapsed[name] = true
    }
    await showGroup(name)
  }

  function deleteGroup(name: string) {
    const count = hostStore.groups.find((g) => g.name === name)?.hosts.length ?? 0
    // An empty group has nothing to lose, so it goes without asking.
    if (count === 0) void hostStore.deleteGroup(name)
    else deleting = { name, count }
  }

  // Moving hosts: drag a row within its group or into another one (a folded or
  // empty group takes it at the end), the host menu, or Alt+↑/↓. The jump hosts
  // group fills itself, so nothing is dragged from or into it.
  interface HostDrag {
    host: Host
    group: string
    startY: number
    moved: boolean
    /** Where it would land: a group, and the host it goes before (0: the end). */
    target: { group: string; before: number } | null
  }
  let hdrag = $state<HostDrag | null>(null)
  let moveTo = $state<Host | null>(null)
  const realGroup = (name: string) => (name === NO_GROUP ? '' : name)

  function onHostPointerDown(e: PointerEvent, host: Host, group: string) {
    if (e.button !== 0 || e.ctrlKey || e.metaKey || e.shiftKey || !canReorder || group === JUMP_GROUP) return
    hdrag = { host, group, startY: e.clientY, moved: false, target: null }
    window.addEventListener('pointermove', onHostPointerMove)
    window.addEventListener('pointerup', stopHostDrag)
  }

  function hostTarget(y: number, id: number): HostDrag['target'] {
    if (!list) return null
    for (const block of list.querySelectorAll<HTMLElement>('[data-group-block]')) {
      const name = block.dataset.groupBlock!
      const r = block.getBoundingClientRect()
      if (name === JUMP_GROUP || y < r.top || y > r.bottom) continue
      const rows = [...block.querySelectorAll<HTMLElement>('[data-host-row]')].filter((row) => Number(row.dataset.hostRow) !== id)
      const head = block.querySelector('[data-group-head]')!.getBoundingClientRect()
      // Over the header, or over a folded group: the top of an open group, the end of a folded one.
      if (collapsed[name] || !rows.length) return { group: name, before: 0 }
      if (y < head.bottom) return { group: name, before: Number(rows[0].dataset.hostRow) }
      const next = rows.find((row) => {
        const b = row.getBoundingClientRect()
        return y < b.top + b.height / 2
      })
      return { group: name, before: next ? Number(next.dataset.hostRow) : 0 }
    }
    return null
  }

  function onHostPointerMove(e: PointerEvent) {
    if (!hdrag || !list) return
    if (!hdrag.moved && Math.abs(e.clientY - hdrag.startY) < 5) return
    hdrag.moved = true
    // Between two groups there is no target: keep the last one.
    hdrag.target = hostTarget(e.clientY, hdrag.host.id) ?? hdrag.target
    const box = list.getBoundingClientRect()
    if (e.clientY < box.top + 24) list.scrollTop -= 12
    else if (e.clientY > box.bottom - 24) list.scrollTop += 12
  }

  function stopHostDrag() {
    window.removeEventListener('pointermove', onHostPointerMove)
    window.removeEventListener('pointerup', stopHostDrag)
    const d = hdrag
    hdrag = null
    if (!d?.moved) return
    justDragged = true
    setTimeout(() => (justDragged = false))
    if (d.target) void hostStore.moveHost(d.host.id, realGroup(d.target.group), d.target.before)
  }
  onDestroy(stopHostDrag)

  /** Alt+↑/↓: one place up or down in its group. */
  async function nudgeHost(host: Host, step: number) {
    const g = hostStore.groups.find((x) => x.hosts.some((h) => h.id === host.id))
    if (!g || g.name === JUMP_GROUP || !canReorder) return
    const i = g.hosts.findIndex((h) => h.id === host.id)
    const j = i + step
    if (j < 0 || j >= g.hosts.length) return
    // Up: before the host above. Down: before the host two below, or at the end.
    const before = step < 0 ? g.hosts[j].id : (g.hosts[j + 1]?.id ?? 0)
    await hostStore.moveHost(host.id, realGroup(g.name), before)
    await tick()
    document.getElementById(`host-row-${host.id}`)?.focus()
  }

  // Marker above host `h` (it is where the dragged one would go before it) and below a group's last row.
  const hostMarkAbove = (group: string, h: Host) => hdrag?.moved && hdrag.target?.group === group && hdrag.target.before === h.id && hdrag.host.id !== h.id
  const hostMarkBelow = (group: string, hosts: Host[], h: Host) => {
    if (!hdrag?.moved || hdrag.target?.group !== group || hdrag.target.before !== 0 || collapsed[group]) return false
    const rest = hosts.filter((x) => x.id !== hdrag!.host.id)
    return rest.length > 0 && rest[rest.length - 1].id === h.id
  }
  const hostDropInto = (group: string, hosts: Host[]) =>
    !!hdrag?.moved && hdrag.target?.group === group && hdrag.target.before === 0 && (collapsed[group] || hosts.every((x) => x.id === hdrag!.host.id))

  const markAbove = (i: number) => gdrag?.moved && gdrag.slot === i && i !== gdrag.from && i !== gdrag.from + 1
  const markBelow = (i: number) => gdrag?.moved && i === visible.length - 1 && gdrag.slot === visible.length && gdrag.from !== visible.length - 1
  const up = $derived(hostStatus.upCount(hostStore.list.map((h) => h.id)))
  // Hosts with a running session in some pane.
  const open = $derived(new Set(workspace.panes.filter((p) => p.source.kind === 'host' && !p.exited).map((p) => (p.source as { hostId: number }).hostId)))
  const pickedVisible = $derived(visibleHosts.filter((h) => picked.has(h.id)))
  const allPicked = $derived(visibleHosts.length > 0 && pickedVisible.length === visibleHosts.length)

  // Drop selections for hosts that no longer exist.
  $effect(() => {
    const ids = new Set(hostStore.list.map((h) => h.id))
    for (const id of picked) if (!ids.has(id)) picked.delete(id)
  })

  function startSelecting(first?: Host) {
    selecting = true
    picked.clear()
    if (first) picked.add(first.id)
    anchor = first?.id ?? null
  }

  function stopSelecting() {
    selecting = false
    picked.clear()
    anchor = null
  }

  // Click toggles one host; Shift+click selects the range from the last click.
  function pick(host: Host, e: MouseEvent | KeyboardEvent) {
    if (e.shiftKey && anchor !== null) {
      const ids = visibleHosts.map((h) => h.id)
      const [a, b] = [ids.indexOf(anchor), ids.indexOf(host.id)].sort((x, y) => x - y)
      if (a >= 0) {
        for (const id of ids.slice(a, b + 1)) picked.add(id)
        return
      }
    }
    if (picked.has(host.id)) picked.delete(host.id)
    else picked.add(host.id)
    anchor = host.id
  }

  function toggleAll() {
    if (allPicked) for (const h of visibleHosts) picked.delete(h.id)
    else for (const h of visibleHosts) picked.add(h.id)
  }

  function toggleGroup(group: HostGroup) {
    const all = group.hosts.every((h) => picked.has(h.id))
    for (const h of group.hosts) {
      if (all) picked.delete(h.id)
      else picked.add(h.id)
    }
  }

  async function deletePicked() {
    await hostStore.removeMany([...picked])
    stopSelecting()
  }

  function openMenu(e: MouseEvent | KeyboardEvent, host: Host) {
    e.preventDefault()
    if (e instanceof MouseEvent && e.type === 'contextmenu') {
      menu = { x: e.clientX, y: e.clientY, host }
    } else {
      const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
      menu = { x: r.left + 24, y: r.bottom, host }
    }
  }

  function onRowKey(e: KeyboardEvent, host: Host) {
    if (e.altKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
      e.preventDefault()
      void nudgeHost(host, e.key === 'ArrowUp' ? -1 : 1)
      return
    }
    if (e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey)) openMenu(e, host)
  }

  // Ctrl+click (or Shift+click for a range) picks hosts to open together.
  function onRowClick(e: MouseEvent, host: Host) {
    if (justDragged) return
    if (e.ctrlKey || e.metaKey || e.shiftKey) {
      e.preventDefault()
      pick(host, e)
    } else onConnect(host)
  }

  const pickedHosts = $derived(hostStore.list.filter((h) => picked.has(h.id)))

  function openTiles() {
    onOpenTiles(pickedHosts.slice(0, MAX_PANES))
    picked.clear()
  }

  const menuItems = (host: Host): MenuItem[] => [
    { label: 'Connect', run: () => onConnect(host) },
    { label: 'Connect as…', run: () => onConnectAs(host) },
    { label: 'Edit…', run: () => onEdit(host) },
    { label: 'Duplicate…', run: () => onDuplicate(host) },
    { label: picked.has(host.id) ? 'Unpick for tiles' : 'Pick for tiles', hint: 'Ctrl+click', run: () => pick(host, new MouseEvent('click')) },
    { label: 'Move to group…', run: () => (moveTo = host), disabled: hostStore.groups.find((g) => g.name === JUMP_GROUP)?.hosts.some((h) => h.id === host.id) },
    { label: 'Select…', run: () => startSelecting(host) },
    { label: 'Delete…', run: () => onDelete(host), danger: true },
  ]
</script>

<aside aria-label="Hosts" class="flex w-60 shrink-0 flex-col gap-3.5 overflow-hidden rounded-panel border border-line bg-panel px-2.5 py-3.5">
  {#if openTabs}
    <!-- At most 40% of the panel, so the hosts stay reachable with many tabs open. -->
    <div class="flex max-h-[40%] min-h-0 shrink-0 flex-col border-b border-line pb-3">{@render openTabs()}</div>
  {/if}
  <div class="flex items-center gap-1 px-1.5">
    <h2 class="font-display text-[15px] font-semibold">Hosts</h2>
    <span class="ml-auto font-mono text-[11px] whitespace-nowrap text-subtle">{hostStore.list.length}{up ? ` · ${up} up` : ''}</span>
    {#if hostStore.list.length > 0}
      <button
        class="ml-1 rounded-md px-1.5 py-0.5 text-[11px] {selecting ? 'bg-accent-chip text-accent-ink' : 'text-subtle hover:bg-raised hover:text-fg'}"
        aria-pressed={selecting}
        title="Select hosts to delete"
        onclick={() => (selecting ? stopSelecting() : startSelecting())}>Select</button
      >
    {/if}
    <button class="grid size-6 place-items-center rounded-md text-subtle hover:bg-raised hover:text-fg" aria-label="Import hosts" title="Import hosts" onclick={onImport}>
      <Icon name="import" size={13} />
    </button>
    <button class="grid size-6 place-items-center rounded-md text-subtle hover:bg-raised hover:text-fg" aria-label="New group" title="New group" onclick={() => (newGroup = true)}>
      <Icon name="folderPlus" size={14} />
    </button>
    <button class="grid size-6 place-items-center rounded-md text-subtle hover:bg-raised hover:text-fg" aria-label="New host" title="New host" onclick={onNew}>
      <Icon name="plus" size={14} />
    </button>
  </div>

  {#if hostStore.list.length > 8}
    <label class="-mt-1 flex items-center gap-2 rounded-[10px] border border-strong bg-field px-2.5 focus-within:border-accent">
      <span class="text-subtle"><Icon name="search" size={13} /></span>
      <span class="sr-only">Filter hosts</span>
      <input
        bind:value={filter}
        class="h-8 min-w-0 flex-1 bg-transparent text-[13px] placeholder:text-subtle focus:outline-none focus-visible:outline-none"
        placeholder="Filter hosts"
        autocomplete="off"
        spellcheck="false"
        onkeydown={(e) => e.key === 'Escape' && (filter = '')}
      />
    </label>
  {/if}

  {#if selecting}
    <div role="toolbar" aria-label="Selection" class="-mt-1 flex items-center gap-2 rounded-[10px] border border-strong bg-raised px-2.5 py-1.5">
      <label class="flex flex-1 items-center gap-2 text-xs text-muted">
        <input type="checkbox" class="size-4 accent-accent" checked={allPicked} indeterminate={pickedVisible.length > 0 && !allPicked} onchange={toggleAll} />
        {pickedVisible.length ? `${pickedVisible.length} selected` : `Select all${filter ? ' matching' : ''}`}
      </label>
      <button
        class="rounded-md bg-danger px-2.5 py-1 text-xs font-medium text-white hover:bg-danger/90 disabled:opacity-40"
        disabled={picked.size === 0}
        onclick={() => (confirmBulk = true)}
      >
        Delete{picked.size ? ` ${picked.size}` : ''}
      </button>
      <button class="rounded-md px-1.5 py-1 text-xs text-muted hover:text-fg" onclick={stopSelecting}>Done</button>
    </div>
  {/if}

  <!-- Reaches into the panel's right padding, so the scrollbar (drawn over
       the content in WebKitGTK) runs there instead of over the buttons. -->
  <nav bind:this={list} aria-label="Saved hosts" class:select-none={gdrag?.moved || hdrag?.moved} class="-mr-2.5 -ml-1 flex min-h-0 flex-1 flex-col gap-3.5 overflow-y-auto pr-2.5 pl-1">
    {#if hostStore.error}
      <p role="alert" class="m-2 text-xs text-warning">Could not load hosts: {hostStore.error}</p>
    {:else if hostStore.loaded && hostStore.list.length === 0}
      <div class="flex flex-col items-center gap-3 px-4 pt-10 text-center">
        <span class="grid size-10 place-items-center rounded-xl bg-raised text-subtle"><Icon name="server" size={18} /></span>
        <p class="text-[13px] text-muted">No hosts yet</p>
        <button class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-on-accent hover:bg-accent/90" onclick={onNew}>Add a host</button>
        <button class="text-[13px] text-accent-text hover:underline" onclick={onImport}>Import hosts</button>
      </div>
    {:else if filter && visible.length === 0}
      <p class="px-3 pt-6 text-center text-[13px] text-muted">No hosts match “{filter}”</p>
    {/if}

    {#each visible as group, gi (group.name)}
      {@const expanded = !collapsed[group.name] || !!filter}
      {@const groupPicked = group.hosts.filter((h) => picked.has(h.id)).length}
      <div
        data-group-block={group.name}
        class="flex flex-col gap-0.5 rounded-lg {gdrag?.moved && gdrag.name === group.name ? 'opacity-50' : ''}"
        class:mark-above={markAbove(gi)}
        class:mark-below={markBelow(gi)}
        class:drop-into={hostDropInto(group.name, group.hosts)}
      >
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <h3
          data-group-head
          class="flex items-center"
          oncontextmenu={(e) => {
            e.preventDefault()
            groupMenu = { x: e.clientX, y: e.clientY, name: group.name }
          }}
          onpointerdown={(e) => onGroupPointerDown(e, group.name, gi)}
        >
          {#if selecting}
            <input
              type="checkbox"
              class="ml-2 size-3.5 shrink-0 accent-accent"
              aria-label="Select all in {group.name}"
              checked={groupPicked === group.hosts.length}
              indeterminate={groupPicked > 0 && groupPicked < group.hosts.length}
              onchange={() => toggleGroup(group)}
            />
          {/if}
          <button
            id="group-head-{group.name}"
            class="cap group/h flex w-full items-center gap-1.5 rounded-md px-2 pb-1.5 hover:text-muted"
            aria-expanded={expanded}
            aria-controls="host-group-{gi}"
            title={canReorder ? 'Click to fold · drag (or Alt+↑/↓) to move · right-click for more' : undefined}
            onclick={() => !justDragged && toggle(group.name)}
            onkeydown={(e) => onGroupKey(e, group.name, gi)}
          >
            <span class="flex-1 truncate text-left">{group.name}</span>
            <!-- The chevron only shows when it matters: hovered, focused or collapsed. -->
            <span class="transition-transform {expanded ? 'rotate-90 opacity-0 group-hover/h:opacity-100 group-focus-visible/h:opacity-100' : ''}"><Icon name="chevron" size={11} /></span>
          </button>
        </h3>
        <ul id="host-group-{gi}" class="flex flex-col gap-0.5" hidden={!expanded}>
          {#if group.hosts.length === 0}
            <li class="rounded-[10px] border border-dashed border-strong px-2.5 py-2 text-xs text-subtle">Drag hosts here, or use “Move to group…” on a host.</li>
          {/if}
          {#each group.hosts as host (host.id)}
            {@const chosen = picked.has(host.id)}
            <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
            <li
              class="group relative rounded-[10px] {hdrag?.moved && hdrag.host.id === host.id ? 'opacity-40' : ''}"
              class:mark-above={hostMarkAbove(group.name, host)}
              class:mark-below={hostMarkBelow(group.name, group.hosts, host)}
              data-host-row={host.id}
              onpointerdown={(e) => onHostPointerDown(e, host, group.name)}
            >
              {#if selecting}
                <label class="flex w-full cursor-pointer items-center gap-2.5 rounded-[10px] px-2 py-1.5 text-[13px] hover:bg-raised {chosen ? 'bg-select text-fg' : 'text-text'}">
                  <input
                    type="checkbox"
                    class="size-4 shrink-0 accent-accent"
                    checked={chosen}
                    onclick={(e) => {
                      e.preventDefault()
                      pick(host, e)
                    }}
                  />
                  <HostBadge name={host.name} os={host.os} size={22} />
                  <span class="min-w-0 flex-1 truncate">{host.name}</span>
                </label>
              {:else}
                <button
                  id="host-row-{host.id}"
                  class="flex w-full items-center gap-2.5 rounded-[10px] px-2 py-1.5 text-left text-[13px] transition-colors hover:bg-raised {chosen || host.id === selectedId ? 'bg-select text-fg' : 'text-text'}"
                  title="Connect to {describe(host)}{hostStatus.label(host.id) ? ` · ${hostStatus.label(host.id)}` : ''} · {keys('Ctrl+click')} to pick for tiles"
                  aria-haspopup="menu"
                  aria-pressed={chosen || undefined}
                  onclick={(e) => onRowClick(e, host)}
                  oncontextmenu={(e) => openMenu(e, host)}
                  onkeydown={(e) => onRowKey(e, host)}
                >
                  <HostBadge name={host.name} os={host.os} />
                  <span class="min-w-0 flex-1 truncate">{host.name}</span>
                  {#if group.name === JUMP_GROUP}
                    <span class="text-[10.5px] text-subtle group-focus-within:opacity-0 group-hover:opacity-0">gateway</span>
                  {:else}
                    <span class="flex group-focus-within:opacity-0 group-hover:opacity-0">
                      <StatusDot state={hostStatus.state(host.id)} label={hostStatus.label(host.id)} />
                    </span>
                  {/if}
                </button>
                <button
                  class="absolute top-1/2 right-1.5 grid size-6 -translate-y-1/2 place-items-center rounded-md text-subtle opacity-0 group-hover:opacity-100 hover:bg-line hover:text-fg focus-visible:opacity-100"
                  aria-label="Edit {host.name}"
                  onclick={() => onEdit(host)}
                >
                  <Icon name="edit" size={13} />
                </button>
              {/if}
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  </nav>

  {#if hostStore.list.length > 1 && !selecting}
    <div class="flex flex-col gap-2 rounded-xl border border-strong bg-raised p-3">
      <span class="text-xs text-text">
        {#if pickedHosts.length}
          {pickedHosts.length} picked{pickedHosts.length > MAX_PANES ? ` · first ${MAX_PANES} open` : ''} ·
          <button class="underline hover:text-fg" onclick={() => picked.clear()}>clear</button>
        {:else}
          {keys('Ctrl+click')} hosts, then
        {/if}
      </span>
      <button
        class="h-8 rounded-[9px] bg-accent text-[12.5px] font-semibold text-on-accent hover:bg-accent/90 disabled:opacity-40"
        disabled={pickedHosts.length === 0}
        onclick={openTiles}
      >
        Open {pickedHosts.length ? Math.min(pickedHosts.length, MAX_PANES) : ''} in tiles
      </button>
    </div>
  {/if}
</aside>

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} label="Actions for {menu.host.name}" items={menuItems(menu.host)} onClose={() => (menu = null)} />
{/if}
{#if moveTo}
  {@const host = moveTo}
  <PromptDialog
    title="Move {host.name}"
    label="Group"
    initial={host.group}
    hint="Type an existing group or a new name. Leave empty for no group."
    submitLabel="Move"
    onSubmit={(g) => void hostStore.moveHost(host.id, g.trim(), 0)}
    onClose={() => (moveTo = null)}
  />
{/if}
{#if groupMenu}
  <ContextMenu x={groupMenu.x} y={groupMenu.y} label="Actions for {groupMenu.name}" items={groupMenuItems(groupMenu.name)} onClose={() => (groupMenu = null)} />
{/if}
{#if newGroup}
  <PromptDialog
    title="New group"
    label="Name"
    initial=""
    hint="Then drag hosts into it, or pick it in a host's Group field."
    submitLabel="Create"
    onSubmit={(name) => void showGroup(hostStore.createGroup(name))}
    onClose={() => (newGroup = false)}
  />
{/if}
{#if renaming}
  {@const from = renaming}
  <PromptDialog
    title="Rename {from}"
    label="Name"
    initial={from}
    hint="The name of another group moves these hosts into it."
    submitLabel="Rename"
    onSubmit={(to) => void renameGroup(from, to)}
    onClose={() => (renaming = null)}
  />
{/if}
{#if deleting}
  {@const d = deleting}
  <ConfirmDialog
    title="Delete the group {d.name}?"
    message="Its {d.count === 1 ? 'host stays' : `${d.count} hosts stay`} in TuxShell, under {NO_GROUP}. Nothing else changes."
    confirmLabel="Delete group"
    onConfirm={() => hostStore.deleteGroup(d.name)}
    onClose={() => (deleting = null)}
  />
{/if}

{#if confirmBulk}
  <ConfirmDialog
    title="Delete {picked.size} {picked.size === 1 ? 'host' : 'hosts'}?"
    message="This removes them from TuxShell. Open sessions keep running, and your ~/.ssh/config is not changed."
    confirmLabel="Delete {picked.size}"
    onConfirm={deletePicked}
    onClose={() => (confirmBulk = false)}
  />
{/if}

<svelte:window onkeydown={(e) => e.key === 'Escape' && selecting && !confirmBulk && stopSelecting()} />

<style>
  /* Where a dragged group will land. */
  .mark-above {
    box-shadow: 0 -2px 0 var(--color-accent);
  }
  .mark-below {
    box-shadow: 0 2px 0 var(--color-accent);
  }
  /* A folded or empty group that would take the dragged host. */
  .drop-into {
    outline: 2px solid var(--color-accent);
    outline-offset: 1px;
  }
</style>
