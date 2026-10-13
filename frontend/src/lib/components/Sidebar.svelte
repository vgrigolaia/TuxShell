<script lang="ts">
  import { describe, hostStore, initials, type Host } from '../hosts.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    selectedId: number | null
    onConnect: (host: Host) => void
    onEdit: (host: Host) => void
    onNew: () => void
  }
  let { selectedId, onConnect, onEdit, onNew }: Props = $props()
</script>

<aside aria-label="Hosts" class="flex w-64 shrink-0 flex-col overflow-hidden rounded-panel border border-line bg-panel">
  <div class="flex items-center justify-between px-4 pt-3.5 pb-2">
    <h2 class="font-display text-[15px] font-semibold">Hosts</h2>
    <div class="flex items-center gap-1">
      <span class="mr-1 font-mono text-[11px] text-subtle">{hostStore.list.length}</span>
      <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="New host" onclick={onNew}>
        <Icon name="plus" size={15} />
      </button>
    </div>
  </div>

  <nav aria-label="Saved hosts" class="flex-1 overflow-y-auto px-2 pb-2">
    {#if hostStore.error}
      <p role="alert" class="m-2 text-xs text-warning">Could not load hosts: {hostStore.error}</p>
    {:else if hostStore.loaded && hostStore.list.length === 0}
      <div class="flex flex-col items-center gap-3 px-4 pt-10 text-center">
        <span class="grid size-10 place-items-center rounded-xl bg-raised text-subtle"><Icon name="server" size={18} /></span>
        <p class="text-[13px] text-muted">No hosts yet</p>
        <button class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-ground hover:bg-accent/90" onclick={onNew}>Add a host</button>
      </div>
    {/if}

    {#each hostStore.groups as group (group.name)}
      <h3 class="px-2 pt-3 pb-1.5 font-mono text-[11px] tracking-wider text-subtle uppercase">{group.name}</h3>
      <ul>
        {#each group.hosts as host (host.id)}
          <li class="group relative">
            <button
              class="flex w-full items-center gap-2.5 rounded-lg py-1.5 pr-9 pl-2 text-left transition-colors hover:bg-raised"
              class:bg-raised={host.id === selectedId}
              title="Connect to {describe(host)}"
              onclick={() => onConnect(host)}
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
