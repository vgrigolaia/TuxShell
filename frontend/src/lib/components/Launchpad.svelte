<script lang="ts" module>
  // ~/.ssh/config is scanned once per run (it starts an `ssh -G` per alias).
  let newInConfig: Promise<number> | undefined
</script>

<script lang="ts">
  import type { Snippet } from 'svelte'
  import { onMount } from 'svelte'
  import * as SshConfig from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/sshconfig/service'
  import { describe, hostStore, type Host } from '../hosts.svelte'
  import { hostStatus } from '../status.svelte'
  import { best } from '../fuzzy'
  import HostBadge from './HostBadge.svelte'
  import StatusDot from './StatusDot.svelte'
  import Icon from './Icon.svelte'
  import SnippetsPage from './SnippetsPage.svelte'
  import TunnelsPage from './TunnelsPage.svelte'
  import { snippets } from '../snippets.svelte'
  import { tunnels } from '../tunnels.svelte'

  interface Props {
    selectedId: number | null
    onSelect: (host: Host) => void
    onConnect: (host: Host) => void
    onConnectTarget: (target: string) => void
    onNew: () => void
    onImport: () => void
    /** The host editor, shown on the right when a host is selected. */
    editor?: Snippet
    /** Which Home section shows. */
    page?: 'hosts' | 'snippets' | 'tunnels'
  }
  let { selectedId, onSelect, onConnect, onConnectTarget, onNew, onImport, editor, page = $bindable('hosts') }: Props = $props()

  let query = $state('')
  let group = $state<string | null>(null)
  let found = $state(0)
  let search = $state<HTMLInputElement>()

  const hour = new Date().getHours()
  const greeting = hour < 5 ? 'Good evening' : hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening'

  const groups = $derived(hostStore.groups.filter((g) => g.hosts.length))
  const cards = $derived.by(() => {
    const q = query.trim()
    const inGroup = group ? (groups.find((g) => g.name === group)?.hosts ?? []) : hostStore.list
    if (!q) return inGroup
    return inGroup
      .map((h) => ({ h, s: best(q, [h.name, h.hostname, h.alias, h.group, describe(h), ...(h.tags ?? [])]) }))
      .filter((r) => r.s > 0)
      .sort((a, b) => b.s - a.s)
      .map((r) => r.h)
  })

  // Loose check for "[user@]host[:port]"; Go does the real validation.
  const looksLikeTarget = (q: string) => /^[^\s-][^\s]*$/.test(q) && /[@.:]/.test(q)

  onMount(() => {
    newInConfig ??= SshConfig.Scan().then(
      (list) => (list ?? []).filter((c) => !c.imported && !c.error).length,
      () => 0,
    )
    void newInConfig.then((n) => (found = n))
    search?.focus()
    void snippets.load()
  })

  function submit() {
    const q = query.trim()
    if (cards.length && q) onConnect(cards[0])
    else if (q && looksLikeTarget(q)) onConnectTarget(q)
  }

  function auth(h: Host) {
    if (h.proxyJump) return `via ${h.proxyJump}`
    if (h.passwordRef) return '1Password ref'
    return h.identityFile ? 'key file' : 'default keys'
  }

  const chip = 'flex h-[34px] items-center gap-1.5 rounded-full px-3.5 text-[13px] font-semibold'
</script>

<div class="flex min-h-0 flex-1 gap-2.5">
  <nav aria-label="Home" class="flex w-[232px] shrink-0 flex-col gap-1 rounded-panel border border-line bg-raised px-3.5 py-5">
    {#each [['hosts', 'Hosts', 'server', hostStore.list.length], ['snippets', 'Snippets', 'code', snippets.list.length], ['tunnels', 'Tunnels', 'tunnel', tunnels.list.length]] as const as [id, name, icon, count] (id)}
      <button
        class="flex h-10 items-center gap-3 rounded-xl px-3 text-sm font-medium {page === id ? 'bg-panel text-fg shadow-sm' : 'text-text hover:bg-panel/60'}"
        aria-current={page === id ? 'page' : undefined}
        onclick={() => (page = id)}
      >
        <span class={page === id ? 'text-accent' : ''}><Icon name={icon} size={17} /></span>{name}
        {#if count}<span class="ml-auto text-xs text-subtle">{id === 'tunnels' && tunnels.running ? `${tunnels.running} on` : count}</span>{/if}
      </button>
    {/each}
    <span class="flex h-10 cursor-default items-center gap-3 px-3 text-sm font-medium text-subtle" title="1Password integration arrives in a later version">
      <Icon name="shield" size={17} />Keychain<span class="ml-auto rounded-full bg-hop px-2 py-0.5 text-[10.5px]">soon</span>
    </span>

    <div class="mt-auto flex flex-col gap-2 rounded-2xl border border-line bg-panel p-3.5">
      <div class="flex items-center gap-2 text-[13px] font-bold">
        <span class="text-subtle"><Icon name="shield" size={14} /></span>1Password
      </div>
      <p class="text-xs leading-relaxed text-subtle">Vault references and the SSH agent arrive in a later version. Secrets are never written to disk.</p>
    </div>
  </nav>

  {#if page === 'snippets'}
    <SnippetsPage />
  {:else if page === 'tunnels'}
    <TunnelsPage />
  {:else}
  <main class="flex min-w-0 flex-1 flex-col gap-5 overflow-y-auto rounded-panel border border-line bg-ground px-7 py-6">
    <header class="flex flex-wrap items-center gap-3.5">
      <h1 class="font-display text-3xl font-bold tracking-[-0.03em]">{greeting}</h1>
      <div class="ml-auto flex gap-2.5">
        <button class="h-[42px] rounded-[13px] border border-strong bg-panel px-4 text-sm font-semibold hover:bg-raised" onclick={onImport}>Import</button>
        <button class="new-host h-[42px] rounded-[13px] bg-accent px-[18px] text-sm font-bold text-on-accent hover:bg-accent/90" onclick={onNew}>+ New host</button>
      </div>
    </header>

    <form
      class="search flex h-[58px] shrink-0 items-center gap-3 rounded-[18px] bg-panel px-[18px]"
      onsubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <span class="text-subtle"><Icon name="search" size={20} /></span>
      <label class="min-w-0 flex-1">
        <span class="sr-only">Quick connect</span>
        <input
          bind:this={search}
          bind:value={query}
          class="w-full bg-transparent text-base placeholder:text-subtle focus:outline-none focus-visible:outline-none"
          placeholder="user@host, a name, or a tag — Enter connects"
          autocomplete="off"
          spellcheck="false"
          onkeydown={(e) => e.key === 'Escape' && (query = '')}
        />
      </label>
      <span class="kbd">Ctrl K</span>
    </form>

    {#if found > 0}
      <div class="flex flex-wrap items-center gap-3.5 rounded-2xl bg-accent-soft px-[18px] py-3.5 text-[13.5px] text-accent-ink">
        <span class="text-accent-text"><Icon name="import" size={18} /></span>
        <span><strong>Found {found} {found === 1 ? 'host' : 'hosts'}</strong> in ~/.ssh/config that {found === 1 ? "isn't" : "aren't"} saved yet.</span>
        <button class="ml-auto h-8 rounded-[10px] bg-accent-text px-3.5 text-[13px] font-bold text-on-accent hover:opacity-90" onclick={onImport}>Review & import</button>
      </div>
    {/if}

    {#if hostStore.list.length}
      <div role="radiogroup" aria-label="Group" class="flex flex-wrap gap-2">
        <button role="radio" aria-checked={group === null} class="{chip} {group === null ? 'bg-fg text-panel' : 'chip-off'}" onclick={() => (group = null)}>
          All {hostStore.list.length}
        </button>
        {#each groups as g (g.name)}
          <button role="radio" aria-checked={group === g.name} class="{chip} {group === g.name ? 'bg-fg text-panel' : 'chip-off'}" onclick={() => (group = g.name)}>
            {g.name} {g.hosts.length}
          </button>
        {/each}
      </div>
    {/if}

    {#if hostStore.loaded && hostStore.list.length === 0}
      <div class="flex flex-col items-center gap-3 rounded-[20px] bg-panel px-6 py-14 text-center">
        <span class="grid size-12 place-items-center rounded-2xl bg-raised text-subtle"><Icon name="server" size={22} /></span>
        <p class="text-[15px] font-semibold">No saved hosts yet</p>
        <p class="max-w-sm text-sm text-subtle">Add one, import your ~/.ssh/config, or type user@host above and press Enter to connect right away.</p>
      </div>
    {:else if query && !cards.length}
      <p class="py-6 text-center text-sm text-subtle">
        No saved host matches “{query}”.{#if looksLikeTarget(query.trim())} Press Enter to connect to it.{/if}
      </p>
    {/if}

    <ul class="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-4">
      {#each cards as h (h.id)}
        {@const st = hostStatus.state(h.id)}
        {@const off = st === 'down'}
        <li>
          <button
            class="card flex h-full w-full flex-col gap-3.5 rounded-[20px] p-[18px] text-left {h.id === selectedId ? 'card-selected' : ''} {off ? 'card-off' : 'bg-panel'}"
            title="Click to edit · double-click or Enter to connect"
            onclick={() => onSelect(h)}
            ondblclick={() => onConnect(h)}
            onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), onConnect(h))}
          >
            <span class="flex items-center gap-3">
              <HostBadge name={h.name} size={44} dim={off} />
              <span class="flex min-w-0 flex-col">
                <span class="truncate text-[15px] font-bold {off ? 'text-subtle' : ''}">{h.name}</span>
                <span class="truncate font-mono text-xs text-subtle">{describe(h)}</span>
              </span>
            </span>
            {#if h.tags?.length}
              <span class="flex flex-wrap gap-1.5">
                {#each h.tags as t (t)}<span class="flex h-[22px] items-center rounded-full bg-hop px-2.5 text-[11.5px] font-semibold text-text">{t}</span>{/each}
              </span>
            {/if}
            <span class="mt-auto flex items-center gap-2 text-[12.5px] text-text">
              <StatusDot state={st} label={hostStatus.label(h.id)} />
              <span aria-hidden="true">{hostStatus.label(h.id) || h.group || 'Saved host'}</span>
              <span class="ml-auto truncate text-subtle">{auth(h)}</span>
            </span>
          </button>
        </li>
      {/each}
    </ul>
  </main>

  {@render editor?.()}
  {/if}
</div>

<style>
  .search {
    box-shadow:
      0 1px 2px rgb(16 24 40 / 0.05),
      0 10px 30px rgb(16 24 40 / 0.07);
  }
  .card {
    box-shadow:
      0 1px 2px rgb(16 24 40 / 0.05),
      0 8px 24px rgb(16 24 40 / 0.06);
    outline-offset: 3px;
  }
  :global(:root:not([data-theme='light'])) .card,
  :global(:root:not([data-theme='light'])) .search {
    box-shadow: inset 0 0 0 1px var(--color-line);
  }
  .card-selected {
    box-shadow:
      0 0 0 2px var(--color-accent),
      0 10px 30px color-mix(in srgb, var(--color-accent) 20%, transparent) !important;
  }
  .card-off {
    background: var(--color-raised);
    box-shadow: inset 0 0 0 1px var(--color-line);
  }
  .chip-off {
    background: var(--color-panel);
    color: var(--color-text);
    box-shadow: inset 0 0 0 1px var(--color-line);
  }
  .new-host {
    box-shadow: 0 6px 18px color-mix(in srgb, var(--color-accent) 20%, transparent);
  }
</style>
