<script lang="ts">
  import { onMount } from 'svelte'
  import { describe, hostStore, type Host } from '../hosts.svelte'
  import HostBadge from './HostBadge.svelte'
  import { snippets, type Snippet } from '../snippets.svelte'
  import { describeTunnel, tunnels, type TunnelState } from '../tunnels.svelte'
  import { best } from '../fuzzy'
  import Icon from './Icon.svelte'
  import { layouts, type Layout } from '../layouts.svelte'

  interface Props {
    placeholder?: string
    onClose: () => void
    onConnectHost: (host: Host) => void
    onConnectTarget: (target: string) => void
    onNewShell: () => void
    onNewHost: () => void
    onImport: () => void
    /** Opens several hosts as panes of one new tab; missing while splitting a tab. */
    onOpenSplit?: (hosts: Host[]) => void
    /** Pastes a snippet into the focused terminal; missing when none is open. */
    onSnippet?: (command: string) => void
    /** Opens a saved layout's tabs; missing while splitting a tab. */
    onLayout?: (layout: Layout) => void
  }
  let { placeholder = 'Search hosts, or type user@host to connect', onClose, onConnectHost, onConnectTarget, onNewShell, onNewHost, onImport, onOpenSplit, onSnippet, onLayout }: Props = $props()

  type Filter = 'all' | 'hosts' | 'snippets' | 'tunnels' | 'actions'
  let filter = $state<Filter>('all')

  type Item =
    | { kind: 'host'; id: string; host: Host }
    | { kind: 'target'; id: string; target: string }
    | { kind: 'snippet'; id: string; snippet: Snippet }
    | { kind: 'tunnel'; id: string; tunnel: TunnelState }
    | { kind: 'action'; id: string; label: string; hint?: string; icon: 'terminal' | 'plus' | 'import' | 'splitRight' | 'tab'; run: () => void }

  let dialog: HTMLDialogElement
  let input: HTMLInputElement
  let query = $state('')
  let index = $state(0)

  // Loose check for "[user@]host[:port]"; Go does the real validation.
  const looksLikeTarget = (q: string) => /^[^\s-][^\s]*$/.test(q) && /[@.:]/.test(q)

  const items = $derived.by<Item[]>(() => {
    const q = query.trim()
    const hosts = hostStore.list
      .map((host) => ({
        host,
        s: best(q, [host.name, host.hostname, host.alias, host.group, describe(host), ...(host.tags ?? [])]),
      }))
      .filter((r) => r.s > 0)
      .sort((a, b) => b.s - a.s)
      .slice(0, 8)
      .map(({ host }): Item => ({ kind: 'host', id: `h${host.id}`, host }))

    const show = (f: Filter) => filter === 'all' || filter === f
    const out: Item[] = show('hosts') ? [...hosts] : []
    if (show('hosts') && q && looksLikeTarget(q)) out.push({ kind: 'target', id: 'target', target: q })
    if (show('snippets') && onSnippet)
      for (const sn of snippets.list.filter((s) => !q || best(q, [s.name, s.group, s.command]) > 0).slice(0, filter === 'snippets' ? 20 : 4))
        out.push({ kind: 'snippet', id: `s${sn.id}`, snippet: sn })
    if (show('tunnels'))
      for (const t of tunnels.list.filter((t) => !q || best(q, [t.name, describeTunnel(t), hostStore.get(t.hostId)?.name ?? '']) > 0).slice(0, filter === 'tunnels' ? 20 : 3))
        out.push({ kind: 'tunnel', id: `t${t.id}`, tunnel: t })
    if (filter === 'hosts' || filter === 'snippets' || filter === 'tunnels') return out

    // A search that narrows to 2–4 hosts can open them side by side, ready to broadcast.
    const matched = hosts.map((i) => (i.kind === 'host' ? i.host : null)).filter((h): h is Host => !!h)
    if (q && onOpenSplit && matched.length >= 2 && matched.length <= 4) {
      const split = onOpenSplit
      out.push({ kind: 'action', id: 'split', label: `Open all ${matched.length} in split`, hint: `${matched.length} panes · broadcast ready`, icon: 'splitRight', run: () => split(matched) })
    }

    // Saved layouts: open all their tabs at once.
    if (onLayout) {
      const open = onLayout
      for (const l of layouts.list.filter((l) => !q || best(q, [l.name, 'layout']) > 0).slice(0, filter === 'actions' ? 20 : 4)) {
        const tabs = l.tabs?.length ?? 0
        out.push({ kind: 'action', id: `l:${l.name}`, label: `Open layout “${l.name}”`, hint: `${tabs} ${tabs === 1 ? 'tab' : 'tabs'}`, icon: 'tab', run: () => open(l) })
      }
    }

    const actions: Item[] = [
      { kind: 'action', id: 'shell', label: 'New local shell', icon: 'terminal', run: onNewShell },
      { kind: 'action', id: 'new', label: 'New host…', icon: 'plus', run: onNewHost },
      { kind: 'action', id: 'import', label: 'Import hosts…', icon: 'import', run: onImport },
    ]
    for (const a of actions) if (a.kind === 'action' && best(q, [a.label]) > 0) out.push(a)
    return out
  })

  $effect(() => {
    // Reset the highlight whenever the result set changes.
    void items
    index = 0
  })

  onMount(() => {
    dialog.showModal()
    input.focus()
  })

  function choose(item: Item | undefined) {
    if (!item) return
    dialog.close()
    if (item.kind === 'host') onConnectHost(item.host)
    else if (item.kind === 'target') onConnectTarget(item.target)
    else if (item.kind === 'snippet') onSnippet?.(item.snippet.command)
    else if (item.kind === 'tunnel') void tunnels.toggle(item.tunnel)
    else item.run()
  }

  function onKey(e: KeyboardEvent) {
    const n = items.length
    if (e.key === 'ArrowDown' || (e.key === 'n' && e.ctrlKey)) {
      e.preventDefault()
      index = n ? (index + 1) % n : 0
    } else if (e.key === 'ArrowUp' || (e.key === 'p' && e.ctrlKey)) {
      e.preventDefault()
      index = n ? (index - 1 + n) % n : 0
    } else if (e.key === 'Enter' && e.ctrlKey) {
      e.preventDefault()
      choose(items.find((i) => i.id === 'split'))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      choose(items[index])
    }
  }

  $effect(() => {
    document.getElementById(`qc-${items[index]?.id}`)?.scrollIntoView({ block: 'nearest' })
  })
</script>

<dialog
  bind:this={dialog}
  aria-label="Quick connect"
  class="mx-auto mt-[14vh] w-[620px] overflow-hidden rounded-2xl border border-line bg-panel p-0 text-fg shadow-2xl backdrop:bg-black/40"
  onclose={onClose}
  onclick={(e) => e.target === dialog && dialog.close()}
>
  <div class="flex items-center gap-3 border-b border-line px-4">
    <span class="text-subtle"><Icon name="search" size={16} /></span>
    <input
      bind:this={input}
      bind:value={query}
      onkeydown={onKey}
      class="h-14 flex-1 bg-transparent text-base placeholder:text-subtle focus:outline-none focus-visible:outline-none"
      {placeholder}
      role="combobox"
      aria-expanded="true"
      aria-controls="qc-list"
      aria-activedescendant={items[index] ? `qc-${items[index].id}` : undefined}
      aria-autocomplete="list"
      autocomplete="off"
      spellcheck="false"
    />
    <kbd class="rounded-md border border-line px-1.5 py-0.5 font-mono text-[11px] text-subtle">Esc</kbd>
  </div>

  <div role="radiogroup" aria-label="Show" class="flex gap-1 border-b border-line px-3 py-2">
    {#each [['all', 'All'], ['hosts', 'Hosts'], ['snippets', 'Snippets'], ['tunnels', 'Tunnels'], ['actions', 'Actions']] as const as [value, label] (value)}
      <button
        role="radio"
        aria-checked={filter === value}
        class="rounded-lg px-2.5 py-1 text-[13px] {filter === value ? 'bg-raised text-fg' : 'text-muted hover:text-fg'}"
        onclick={() => {
          filter = value
          input.focus()
        }}
      >
        {label}
      </button>
    {/each}
  </div>

  <ul id="qc-list" role="listbox" aria-label="Results" class="max-h-[50vh] overflow-y-auto p-2">
    {#each items as item, i (item.id)}
      <!-- Keyboard selection lives on the combobox input (aria-activedescendant). -->
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <li
        id="qc-{item.id}"
        role="option"
        aria-selected={i === index}
        class="flex cursor-pointer items-center gap-3 rounded-xl border px-3 py-2.5 {i === index ? 'border-line bg-raised' : 'border-transparent'}"
        onmousemove={() => (index = i)}
        onclick={() => choose(item)}
      >
        {#if item.kind === 'host'}
          <HostBadge name={item.host.name} os={item.host.os} size={32} />
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[14px]">{item.host.name}</span>
            <span class="block truncate font-mono text-xs text-subtle">
              {describe(item.host)}{item.host.proxyJump ? ` · via ${item.host.proxyJump}` : ''}
            </span>
          </span>
        {:else if item.kind === 'snippet'}
          <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-hop text-muted"><Icon name="code" size={15} /></span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[14px]">Snippet · {item.snippet.name}</span>
            <span class="block truncate font-mono text-xs text-subtle">{item.snippet.command}</span>
          </span>
        {:else if item.kind === 'tunnel'}
          {@const on = item.tunnel.status === 'running'}
          <span class="tint-lime grid size-8 shrink-0 place-items-center rounded-lg"><Icon name="tunnel" size={15} /></span>
          <span class="min-w-0 flex-1">
            <span class="block truncate font-mono text-[13px]">{describeTunnel(item.tunnel)}</span>
            <span class="block truncate text-xs text-subtle">via {hostStore.get(item.tunnel.hostId)?.name ?? '?'} · {on ? 'running' : 'stopped'}</span>
          </span>
        {:else if item.kind === 'target'}
          <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-success/15 text-success"><Icon name="terminal" size={15} /></span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[14px]">Connect to <span class="font-mono">{item.target}</span></span>
            <span class="block text-xs text-subtle">Ad-hoc ssh session, not saved</span>
          </span>
        {:else}
          <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-raised text-muted"><Icon name={item.icon} size={15} /></span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[14px] {item.hint ? '' : 'text-muted'}">{item.label}</span>
            {#if item.hint}<span class="block truncate text-xs text-subtle">{item.hint}</span>{/if}
          </span>
        {/if}
        {#if item.id === 'split'}
          <kbd class="shrink-0 rounded-md border border-line px-1.5 py-0.5 font-mono text-[11px] text-subtle">Ctrl ↵</kbd>
        {:else if i === index}
          <kbd class="shrink-0 rounded-md border border-line px-1.5 py-0.5 font-mono text-[11px] text-subtle"
            >↵ {item.kind === 'action' ? 'run' : item.kind === 'snippet' ? 'paste' : item.kind === 'tunnel' ? (item.tunnel.status === 'running' ? 'stop' : 'start') : 'connect'}</kbd
          >
        {/if}
      </li>
    {:else}
      <li class="px-3 py-6 text-center text-[13px] text-muted">No matches</li>
    {/each}
  </ul>
</dialog>
