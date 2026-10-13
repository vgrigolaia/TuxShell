<script lang="ts">
  import { hostStore } from '../hosts.svelte'
  import { snippets } from '../snippets.svelte'
  import { tunnels } from '../tunnels.svelte'
  import Icon from './Icon.svelte'

  export type Page = 'hosts' | 'snippets' | 'tunnels'

  interface Props {
    page: Page
    onPage: (page: Page) => void
  }
  let { page, onPage }: Props = $props()

  const tabs = $derived([
    { id: 'hosts', name: 'Hosts', icon: 'server', count: `${hostStore.list.length}` },
    { id: 'snippets', name: 'Snippets', icon: 'code', count: snippets.list.length ? `${snippets.list.length}` : '' },
    { id: 'tunnels', name: 'Tunnels', icon: 'tunnel', count: tunnels.running ? `${tunnels.running} on` : tunnels.list.length ? `${tunnels.list.length}` : '' },
  ] as const)
</script>

<!-- Home sections; the host list itself is in the sidebar, as in terminal view. -->
<nav aria-label="Home sections" class="-mt-1 flex flex-wrap items-center gap-1">
  {#each tabs as t (t.id)}
    <button
      class="flex h-9 items-center gap-2 rounded-xl px-3 text-[13px] font-medium {page === t.id ? 'bg-panel text-fg shadow-[inset_0_0_0_1px_var(--color-line)]' : 'text-text hover:bg-panel/60'}"
      aria-current={page === t.id ? 'page' : undefined}
      onclick={() => onPage(t.id)}
    >
      <span class={page === t.id ? 'text-accent' : 'text-subtle'}><Icon name={t.icon} size={15} /></span>{t.name}
      {#if t.count}<span class="text-xs text-subtle">{t.count}</span>{/if}
    </button>
  {/each}
  <span class="flex h-9 cursor-default items-center gap-2 px-3 text-[13px] font-medium text-subtle" title="1Password vault references and the SSH agent arrive in a later version">
    <Icon name="shield" size={15} />Keychain<span class="rounded-full bg-hop px-2 py-0.5 text-[10.5px]">soon</span>
  </span>
</nav>
