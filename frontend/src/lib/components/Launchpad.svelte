<script lang="ts" module>
  // Scanning ~/.ssh/config starts an `ssh -G` per alias, so the result is
  // reused until the saved hosts change.
  let scanned: { key: string; aliases: Promise<string[]> } | undefined
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
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import SnippetsPage from './SnippetsPage.svelte'
  import TunnelsPage from './TunnelsPage.svelte'
  import HistoryPage from './HistoryPage.svelte'
  import LayoutsPage from './LayoutsPage.svelte'
  import { layouts } from '../layouts.svelte'
  import { snippets } from '../snippets.svelte'
  import { tunnels } from '../tunnels.svelte'
  import { onePassword } from '../onepassword.svelte'
  import { prefs } from '../settings.svelte'
  import { keybindings } from '../keybindings.svelte'

  interface Props {
    /** The host open in the editor. */
    selectedId: number | null
    onEdit: (host: Host) => void
    onConnect: (host: Host) => void
    onConnectAs: (host: Host) => void
    onDuplicate: (host: Host) => void
    onDelete: (host: Host) => void
    onConnectTarget: (target: string) => void
    onNew: () => void
    /** Opens the import window, at the ~/.ssh/config list when asked. */
    onImport: (start?: 'sshconfig') => void
    /** The host editor, shown on the right when a host is selected. */
    editor?: Snippet
    /** The open tabs, under the sections (Preferences → Open tabs). */
    openTabs?: Snippet
    /** Double-clicking Hosts opens the terminal view with the host list. */
    onOpenHostList: () => void
    /** Opens Preferences. */
    onPreferences: () => void
    /** Saves all open tabs as a layout; missing when none is open. */
    onSaveTabs?: () => void
    /** Which Home section shows. */
    page?: 'hosts' | 'layouts' | 'snippets' | 'tunnels' | 'history'
  }
  let { selectedId, onEdit, onConnect, onConnectAs, onDuplicate, onDelete, onConnectTarget, onNew, onImport, onOpenHostList, onPreferences, onSaveTabs, editor, openTabs, page = $bindable('hosts') }: Props = $props()

  let query = $state('')
  // A click only marks a card; double-click or Enter connects, right-click has the rest.
  let marked = $state<number | null>(null)
  let menu = $state<{ x: number; y: number; host: Host } | null>(null)

  function openMenu(e: MouseEvent | KeyboardEvent, host: Host) {
    e.preventDefault()
    marked = host.id
    if (e instanceof MouseEvent && e.type === 'contextmenu') {
      menu = { x: e.clientX, y: e.clientY, host }
    } else {
      const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
      menu = { x: r.left + 24, y: r.top + 48, host }
    }
  }

  function onCardKey(e: KeyboardEvent, host: Host) {
    if (e.key === 'Enter') {
      e.preventDefault()
      onConnect(host)
    } else if (e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey)) openMenu(e, host)
  }

  const menuItems = (host: Host): MenuItem[] => [
    { label: 'Connect', run: () => onConnect(host) },
    { label: 'Connect as…', run: () => onConnectAs(host) },
    { label: 'Edit…', run: () => onEdit(host) },
    { label: 'Duplicate…', run: () => onDuplicate(host) },
    { label: 'Delete…', run: () => onDelete(host), danger: true },
  ]
  let group = $state<string | null>(null)
  // Unsaved ~/.ssh/config aliases; the banner leaves out the ones hidden with its ×.
  let unsaved = $state<string[]>([])
  const hidden = $derived(new Set(prefs.value.hiddenSshConfigHosts ?? []))
  const found = $derived(unsaved.filter((a) => !hidden.has(a)).length)
  let search = $state<HTMLInputElement>()

  // Rescan when hosts are saved, imported or deleted, so the banner stays true.
  $effect(() => {
    if (!hostStore.loaded) return
    const key = hostStore.list.map((h) => h.alias).join('\n')
    if (scanned?.key !== key)
      scanned = {
        key,
        aliases: SshConfig.Scan().then(
          (list) => (list ?? []).filter((c) => !c.imported && !c.error).map((c) => c.alias),
          () => [],
        ),
      }
    const current = scanned
    void current.aliases.then((a) => scanned === current && (unsaved = a))
  })

  const hour = new Date().getHours()
  const greeting = hour < 5 ? 'Good evening' : hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening'

  const groups = $derived(hostStore.groups.filter((g) => g.hosts.length))
  const cards = $derived.by(() => {
    const q = query.trim()
    const inGroup = group ? (groups.find((g) => g.name === group)?.hosts ?? []) : groups.flatMap((g) => g.hosts)
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
    search?.focus()
    void snippets.load()
  })

  function submit() {
    const q = query.trim()
    if (cards.length && q) onConnect(cards[0])
    else if (q && looksLikeTarget(q)) onConnectTarget(q)
  }

  const refHosts = $derived(hostStore.list.filter((h) => h.passwordRef).length)
  onMount(() => void onePassword.refresh())

  function auth(h: Host) {
    if (h.proxyJump) return `via ${h.proxyJump}`
    if (h.passwordRef) return h.passwordRef.startsWith('keepassxc:') ? 'KeePassXC ref' : '1Password ref'
    if (h.protocol === 'rdp') return h.passwordSaved ? 'remote desktop · password saved' : 'remote desktop'
    return h.passwordSaved ? 'password saved' : h.identityFile ? 'key file' : 'default keys'
  }

  const chip = 'flex h-[34px] items-center gap-1.5 rounded-full px-3.5 text-[13px] font-semibold'
</script>

<div class="flex min-h-0 min-w-0 flex-1 gap-2.5">
  <nav aria-label="Home" class="flex w-[232px] shrink-0 flex-col gap-1 rounded-panel border border-line bg-raised px-3.5 py-5">
    {#each [['hosts', 'Hosts', 'server', hostStore.list.length], ['layouts', 'Layouts', 'tab', layouts.list.length], ['snippets', 'Snippets', 'code', snippets.list.length], ['tunnels', 'Tunnels', 'tunnel', tunnels.list.length], ['history', 'History', 'clock', 0]] as const as [id, name, icon, count] (id)}
      <button
        class="flex h-10 items-center gap-3 rounded-xl px-3 text-sm font-medium {page === id ? 'bg-panel text-fg shadow-sm' : 'text-text hover:bg-panel/60'}"
        aria-current={page === id ? 'page' : undefined}
        title={id === 'hosts' ? 'Double-click to open the host list with your terminals' : undefined}
        onclick={() => (page = id)}
        ondblclick={() => id === 'hosts' && onOpenHostList()}
      >
        <span class={page === id ? 'text-accent' : ''}><Icon name={icon} size={17} /></span>{name}
        {#if count}<span class="ml-auto text-xs text-subtle">{id === 'tunnels' && tunnels.running ? `${tunnels.running} on` : count}</span>{/if}
      </button>
    {/each}
    {#if openTabs}
      <div class="-mx-1 mt-3 flex min-h-0 flex-col border-t border-line px-1 pt-3">{@render openTabs()}</div>
    {/if}

    <div class="mt-auto flex flex-col gap-2 rounded-2xl border border-line bg-panel p-3.5">
      <div class="flex items-center gap-2 text-[13px] font-bold">
        <span class={onePassword.status?.version || refHosts ? 'text-accent-text' : 'text-subtle'}><Icon name="shield" size={14} /></span>1Password · KeePassXC
      </div>
      <p class="text-xs leading-relaxed text-subtle">
        {#if onePassword.status && !onePassword.status.version && !refHosts}
          Sign in with references to 1Password or KeePassXC: see Preferences → Security.
        {:else}
          {refHosts === 0 ? 'No hosts use a password manager reference yet.' : `${refHosts} ${refHosts === 1 ? 'host signs' : 'hosts sign'} in from a password manager.`}
          {#if prefs.value.onePasswordAgent}SSH agent on.{/if}
        {/if}
        TuxShell never stores their passwords.
      </p>
    </div>
  </nav>

  {#if page === 'layouts'}
    <LayoutsPage {onSaveTabs} />
  {:else if page === 'snippets'}
    <SnippetsPage />
  {:else if page === 'tunnels'}
    <TunnelsPage />
  {:else if page === 'history'}
    <HistoryPage {onConnect} {onConnectTarget} {onPreferences} />
  {:else}
  <main class="flex min-w-0 flex-1 flex-col gap-5 overflow-y-auto rounded-panel border border-line bg-ground px-7 py-6">
    <header class="flex flex-wrap items-center gap-3.5">
      <h1 class="font-display text-3xl font-bold tracking-[-0.03em]">{greeting}</h1>
      <div class="ml-auto flex gap-2.5">
        <button class="h-[42px] rounded-[13px] border border-strong bg-panel px-4 text-sm font-semibold hover:bg-raised" onclick={() => onImport()}>Import</button>
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
      {#if keybindings.label('quickConnect')}<span class="kbd">{keybindings.label('quickConnect')}</span>{/if}
    </form>

    {#if found > 0}
      <div class="flex flex-wrap items-center gap-3.5 rounded-2xl bg-accent-soft px-[18px] py-3.5 text-[13.5px] text-accent-ink">
        <span class="text-accent-text"><Icon name="import" size={18} /></span>
        <span><strong>Found {found} {found === 1 ? 'host' : 'hosts'}</strong> in ~/.ssh/config that {found === 1 ? "isn't" : "aren't"} saved yet.</span>
        <button class="ml-auto h-8 rounded-[10px] bg-accent-text px-3.5 text-[13px] font-bold text-on-accent hover:opacity-90" onclick={() => onImport('sshconfig')}>Review & import</button>
        <button
          class="-mr-1.5 grid size-8 place-items-center rounded-[10px] text-accent-ink/70 hover:bg-accent-text/10 hover:text-accent-ink"
          aria-label="Hide this reminder"
          title="Hide until ~/.ssh/config has new hosts (they stay in Import)"
          onclick={() => prefs.hideSshConfigHosts(unsaved)}
        >
          <Icon name="x" size={16} />
        </button>
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
        <li class="group relative">
          <button
            class="card flex h-full w-full flex-col gap-3.5 rounded-[20px] p-[18px] text-left {h.id === selectedId || h.id === marked ? 'card-selected' : ''} {off ? 'card-off' : 'bg-panel'}"
            title="Double-click or Enter to connect · right-click for more"
            aria-haspopup="menu"
            onclick={() => (marked = h.id)}
            ondblclick={() => onConnect(h)}
            oncontextmenu={(e) => openMenu(e, h)}
            onkeydown={(e) => onCardKey(e, h)}
          >
            <span class="flex items-center gap-3">
              <HostBadge name={h.name} os={h.os} size={44} dim={off} />
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
          <button
            class="absolute top-3 right-3 grid size-8 place-items-center rounded-lg text-subtle opacity-0 group-hover:opacity-100 hover:bg-raised hover:text-fg focus-visible:opacity-100"
            aria-label="Edit {h.name}"
            title="Edit"
            onclick={() => onEdit(h)}
          >
            <Icon name="edit" size={15} />
          </button>
        </li>
      {/each}
    </ul>
  </main>

  {@render editor?.()}
  {/if}
</div>

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} label="Actions for {menu.host.name}" items={menuItems(menu.host)} onClose={() => (menu = null)} />
{/if}

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
