<script lang="ts">
  import { describe, hostStore, type Host, type HostGroup } from '../hosts.svelte'
  import { workspace } from '../workspace.svelte'
  import HostBadge from './HostBadge.svelte'
  import { best } from '../fuzzy'
  import { SvelteSet } from 'svelte/reactivity'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    selectedId: number | null
    onConnect: (host: Host) => void
    onEdit: (host: Host) => void
    onConnectAs: (host: Host) => void
    onDuplicate: (host: Host) => void
    onDelete: (host: Host) => void
    onNew: () => void
    onImport: () => void
  }
  let { selectedId, onConnect, onEdit, onConnectAs, onDuplicate, onDelete, onNew, onImport }: Props = $props()

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
    if (e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey)) openMenu(e, host)
  }

  const menuItems = (host: Host): MenuItem[] => [
    { label: 'Connect', run: () => onConnect(host) },
    { label: 'Connect as…', run: () => onConnectAs(host) },
    { label: 'Edit…', run: () => onEdit(host) },
    { label: 'Duplicate…', run: () => onDuplicate(host) },
    { label: 'Select…', run: () => startSelecting(host) },
    { label: 'Delete…', run: () => onDelete(host), danger: true },
  ]
</script>

<aside aria-label="Hosts" class="flex w-72 shrink-0 flex-col overflow-hidden rounded-panel border border-line bg-panel">
  <div class="flex items-center justify-between pt-4 pr-3 pb-2 pl-4">
    <h2 class="font-display text-base font-semibold">Hosts</h2>
    <div class="flex items-center gap-1">
      <span class="mr-1 font-mono text-[11px] text-subtle">{hostStore.list.length}{open.size ? ` · ${open.size} open` : ''}</span>
      {#if hostStore.list.length > 0}
        <button
          class="rounded-lg px-2 py-1 text-xs transition-colors {selecting ? 'bg-accent/15 text-accent' : 'text-muted hover:bg-raised hover:text-fg'}"
          aria-pressed={selecting}
          onclick={() => (selecting ? stopSelecting() : startSelecting())}
        >
          {selecting ? 'Done' : 'Select'}
        </button>
      {/if}
      <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Import from ~/.ssh/config" title="Import from ~/.ssh/config" onclick={onImport}>
        <Icon name="import" size={15} />
      </button>
      <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="New host" title="New host" onclick={onNew}>
        <Icon name="plus" size={15} />
      </button>
    </div>
  </div>

  {#if hostStore.list.length > 0}
    <div class="px-3 pb-1">
      <label class="flex items-center gap-2 rounded-lg border border-line bg-ground px-2.5 focus-within:border-accent">
        <span class="text-subtle"><Icon name="search" size={13} /></span>
        <span class="sr-only">Filter hosts</span>
        <input
          bind:value={filter}
          class="h-8 min-w-0 flex-1 bg-transparent text-[13px] placeholder:text-subtle focus:outline-none"
          placeholder="Filter"
          autocomplete="off"
          spellcheck="false"
          onkeydown={(e) => e.key === 'Escape' && (filter = '')}
        />
      </label>
    </div>
  {/if}

  {#if selecting}
    <div role="toolbar" aria-label="Selection" class="mx-3 mt-1 flex items-center gap-2 rounded-lg border border-line bg-raised px-2.5 py-1.5">
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
    </div>
  {/if}

  <nav aria-label="Saved hosts" class="flex-1 overflow-y-auto px-2 pb-2">
    {#if hostStore.error}
      <p role="alert" class="m-2 text-xs text-warning">Could not load hosts: {hostStore.error}</p>
    {:else if hostStore.loaded && hostStore.list.length === 0}
      <div class="flex flex-col items-center gap-3 px-4 pt-10 text-center">
        <span class="grid size-10 place-items-center rounded-xl bg-raised text-subtle"><Icon name="server" size={18} /></span>
        <p class="text-[13px] text-muted">No hosts yet</p>
        <button class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-ground hover:bg-accent/90" onclick={onNew}>Add a host</button>
        <button class="text-[13px] text-accent hover:underline" onclick={onImport}>Import from ~/.ssh/config</button>
      </div>
    {:else if filter && visible.length === 0}
      <p class="px-3 pt-6 text-center text-[13px] text-muted">No hosts match “{filter}”</p>
    {/if}

    {#each visible as group, gi (group.name)}
      {@const expanded = !collapsed[group.name] || !!filter}
      {@const groupPicked = group.hosts.filter((h) => picked.has(h.id)).length}
      <h3 class="flex items-center">
        {#if selecting}
          <input
            type="checkbox"
            class="mt-1.5 ml-2 size-3.5 shrink-0 accent-accent"
            aria-label="Select all in {group.name}"
            checked={groupPicked === group.hosts.length}
            indeterminate={groupPicked > 0 && groupPicked < group.hosts.length}
            onchange={() => toggleGroup(group)}
          />
        {/if}
        <button
          class="group/h flex w-full items-center gap-1.5 rounded-md px-2.5 pt-4 pb-2 font-mono text-[11px] tracking-[0.14em] text-subtle uppercase hover:text-muted"
          aria-expanded={expanded}
          aria-controls="host-group-{gi}"
          onclick={() => toggle(group.name)}
        >
          <span class="flex-1 truncate text-left">{group.name}</span>
          <!-- The chevron only shows when it matters: hovered, focused or collapsed. -->
          <span class="transition-transform {expanded ? 'rotate-90 opacity-0 group-hover/h:opacity-100 group-focus-visible/h:opacity-100' : ''}"><Icon name="chevron" size={11} /></span>
        </button>
      </h3>
      <ul id="host-group-{gi}" hidden={!expanded}>
        {#each group.hosts as host (host.id)}
          <li class="group relative">
            {#if selecting}
              <label class="flex w-full cursor-pointer items-center gap-2.5 rounded-lg py-1.5 pr-2 pl-2 hover:bg-raised" class:bg-raised={picked.has(host.id)}>
                <input
                  type="checkbox"
                  class="size-4 shrink-0 accent-accent"
                  checked={picked.has(host.id)}
                  onclick={(e) => {
                    e.preventDefault()
                    pick(host, e)
                  }}
                />
                <HostBadge name={host.name} size={24} />
                <span class="min-w-0 flex-1 truncate text-[13px]">{host.name}</span>
              </label>
            {:else}
            <button
              class="flex w-full items-center gap-3 rounded-xl border py-2 pr-3 pl-2.5 text-left transition-colors hover:bg-raised {host.id === selectedId ? 'border-line bg-raised' : 'border-transparent'}"
              title="Connect to {describe(host)}"
              aria-haspopup="menu"
              onclick={() => onConnect(host)}
              oncontextmenu={(e) => openMenu(e, host)}
              onkeydown={(e) => onRowKey(e, host)}
            >
              <HostBadge name={host.name} />
              <span class="min-w-0 flex-1 truncate text-sm">{host.name}</span>
              {#if open.has(host.id)}
                <span class="flex items-center gap-1.5 font-mono text-[11px] text-subtle group-focus-within:opacity-0 group-hover:opacity-0">
                  <span class="size-1.5 rounded-full bg-success" aria-hidden="true"></span>open
                </span>
              {/if}
            </button>
            <button
              class="absolute top-1/2 right-2 grid size-6 -translate-y-1/2 place-items-center rounded-md text-subtle opacity-0 group-hover:opacity-100 hover:bg-line hover:text-fg focus-visible:opacity-100"
              aria-label="Edit {host.name}"
              onclick={() => onEdit(host)}
            >
              <Icon name="edit" size={13} />
            </button>
            {/if}
          </li>
        {/each}
      </ul>
    {/each}
  </nav>

  <div class="m-2 flex items-start gap-2.5 rounded-xl border border-line bg-raised p-3">
    <span class="mt-0.5 text-subtle"><Icon name="shield" size={15} /></span>
    <div>
      <p class="text-[13px]">1Password not connected</p>
      <p class="text-xs text-subtle">Secrets are never written to disk.</p>
    </div>
  </div>
</aside>

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} label="Actions for {menu.host.name}" items={menuItems(menu.host)} onClose={() => (menu = null)} />
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
