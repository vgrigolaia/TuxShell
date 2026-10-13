<script lang="ts">
  import type { HostGroup, HostStatus } from '../sample-hosts'
  import Icon from './Icon.svelte'

  interface Props {
    groups: HostGroup[]
  }
  let { groups }: Props = $props()

  const total = $derived(groups.reduce((n, g) => n + g.hosts.length, 0))

  const dot: Record<HostStatus, string> = {
    online: 'bg-success',
    degraded: 'border-2 border-warning',
    offline: 'bg-subtle/40',
  }
</script>

<aside
  aria-label="Hosts"
  class="flex w-64 shrink-0 flex-col overflow-hidden rounded-panel border border-line bg-panel"
>
  <div class="flex items-baseline justify-between px-4 pt-4 pb-2">
    <h2 class="font-display text-[15px] font-semibold">Hosts</h2>
    <span class="font-mono text-[11px] text-subtle">{total} · sample</span>
  </div>

  <nav class="flex-1 overflow-y-auto px-2 pb-2">
    {#each groups as group (group.name)}
      <h3 class="px-2 pt-3 pb-1.5 font-mono text-[11px] tracking-wider text-subtle uppercase">{group.name}</h3>
      <ul>
        {#each group.hosts as host (host.name)}
          <li>
            <button
              class="flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-raised"
              class:opacity-60={host.status === 'offline'}
              title="{host.target} — connecting arrives with SSH support"
            >
              <span
                class="grid size-7 shrink-0 place-items-center rounded-md bg-accent/15 font-mono text-[10px] font-semibold text-accent"
                aria-hidden="true">{host.name.slice(0, 2).toUpperCase()}</span
              >
              <span class="min-w-0 flex-1 truncate text-[13px]">{host.name}</span>
              <span class="flex items-center gap-1.5 font-mono text-[11px] text-subtle">
                <span class="size-1.5 rounded-full {dot[host.status]}" aria-hidden="true"></span>
                {host.latency ?? host.status}
              </span>
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
