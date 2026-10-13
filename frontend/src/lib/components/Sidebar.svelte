<script lang="ts">
  import { describe, hostStore, initials, type Host, type HostGroup } from '../hosts.svelte'
  import { best } from '../fuzzy'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    selectedId: number | null
    onConnect: (host: Host) => void
    onEdit: (host: Host) => void
    onDuplicate: (host: Host) => void
    onDelete: (host: Host) => void
    onNew: () => void
    onImport: () => void
  }
  let { selectedId, onConnect, onEdit, onDuplicate, onDelete, onNew, onImport }: Props = $props()

  const COLLAPSED_KEY = 'tuxshell.sidebar.collapsed'

  let filter = $state('')
  let collapsed = $state<Record<string, boolean>>(loadCollapsed())
  let menu = $state<{ x: number; y: number; host: Host } | null>(null)

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
    { label: 'Edit…', run: () => onEdit(host) },
    { label: 'Duplicate…', run: () => onDuplicate(host) },
    { label: 'Delete…', run: () => onDelete(host), danger: true },
  ]
</script>

<aside aria-label="Hosts" class="flex w-64 shrink-0 flex-col overflow-hidden rounded-panel border border-line bg-panel">
  <div class="flex items-center justify-between px-4 pt-3.5 pb-2">
    <h2 class="font-display text-[15px] font-semibold">Hosts</h2>
    <div class="flex items-center gap-1">
      <span class="mr-1 font-mono text-[11px] text-subtle">{hostStore.list.length}</span>
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
      {@const open = !collapsed[group.name] || !!filter}
      <h3>
        <button
          class="flex w-full items-center gap-1.5 rounded-md px-2 pt-3 pb-1.5 font-mono text-[11px] tracking-wider text-subtle uppercase hover:text-muted"
          aria-expanded={open}
          aria-controls="host-group-{gi}"
          onclick={() => toggle(group.name)}
        >
          <span class="transition-transform {open ? 'rotate-90' : ''}"><Icon name="chevron" size={11} /></span>
          <span class="flex-1 truncate text-left">{group.name}</span>
          <span>{group.hosts.length}</span>
        </button>
      </h3>
      <ul id="host-group-{gi}" hidden={!open}>
        {#each group.hosts as host (host.id)}
          <li class="group relative">
            <button
              class="flex w-full items-center gap-2.5 rounded-lg py-1.5 pr-9 pl-2 text-left transition-colors hover:bg-raised"
              class:bg-raised={host.id === selectedId}
              title="Connect to {describe(host)}"
              aria-haspopup="menu"
              onclick={() => onConnect(host)}
              oncontextmenu={(e) => openMenu(e, host)}
              onkeydown={(e) => onRowKey(e, host)}
            >
              <span class="grid size-7 shrink-0 place-items-center rounded-md bg-accent/15 font-mono text-[10px] font-semibold text-accent" aria-hidden="true">
                {initials(host.name)}
              </span>
              <span class="min-w-0 flex-1">
                <span class="block truncate text-[13px]">{host.name}</span>
                <span class="block truncate font-mono text-[11px] text-subtle">{describe(host)}</span>
              </span>
            </button>
            <button
              class="absolute top-1/2 right-1.5 grid size-6 -translate-y-1/2 place-items-center rounded-md text-subtle opacity-0 group-hover:opacity-100 hover:bg-line hover:text-fg focus-visible:opacity-100"
              aria-label="Edit {host.name}"
              onclick={() => onEdit(host)}
            >
              <Icon name="edit" size={13} />
            </button>
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
