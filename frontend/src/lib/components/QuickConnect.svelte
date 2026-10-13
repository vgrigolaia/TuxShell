<script lang="ts">
  import { onMount } from 'svelte'
  import { describe, hostStore, initials, type Host } from '../hosts.svelte'
  import { best } from '../fuzzy'
  import Icon from './Icon.svelte'

  interface Props {
    placeholder?: string
    onClose: () => void
    onConnectHost: (host: Host) => void
    onConnectTarget: (target: string) => void
    onNewShell: () => void
    onNewHost: () => void
    onImport: () => void
  }
  let { placeholder = 'Search hosts, or type user@host to connect', onClose, onConnectHost, onConnectTarget, onNewShell, onNewHost, onImport }: Props = $props()

  type Item =
    | { kind: 'host'; id: string; host: Host }
    | { kind: 'target'; id: string; target: string }
    | { kind: 'action'; id: string; label: string; icon: 'terminal' | 'plus' | 'import'; run: () => void }

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

    const out: Item[] = [...hosts]
    if (q && looksLikeTarget(q)) out.push({ kind: 'target', id: 'target', target: q })

    const actions: Item[] = [
      { kind: 'action', id: 'shell', label: 'New local shell', icon: 'terminal', run: onNewShell },
      { kind: 'action', id: 'new', label: 'New host…', icon: 'plus', run: onNewHost },
      { kind: 'action', id: 'import', label: 'Import from ~/.ssh/config…', icon: 'import', run: onImport },
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
  class="mx-auto mt-[12vh] w-[600px] overflow-hidden rounded-2xl border border-line bg-panel p-0 text-fg shadow-2xl backdrop:bg-black/50"
  onclose={onClose}
  onclick={(e) => e.target === dialog && dialog.close()}
>
  <div class="flex items-center gap-3 border-b border-line px-4">
    <span class="text-subtle"><Icon name="search" size={16} /></span>
    <input
      bind:this={input}
      bind:value={query}
      onkeydown={onKey}
      class="h-12 flex-1 bg-transparent text-[15px] placeholder:text-subtle focus:outline-none"
      {placeholder}
      role="combobox"
      aria-expanded="true"
      aria-controls="qc-list"
      aria-activedescendant={items[index] ? `qc-${items[index].id}` : undefined}
      aria-autocomplete="list"
      autocomplete="off"
      spellcheck="false"
    />
    <kbd class="rounded border border-line px-1.5 py-0.5 font-mono text-[11px] text-subtle">Esc</kbd>
  </div>

  <ul id="qc-list" role="listbox" aria-label="Results" class="max-h-[50vh] overflow-y-auto p-2">
    {#each items as item, i (item.id)}
      <!-- Keyboard selection lives on the combobox input (aria-activedescendant). -->
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <li
        id="qc-{item.id}"
        role="option"
        aria-selected={i === index}
        class="flex cursor-pointer items-center gap-3 rounded-xl px-3 py-2 {i === index ? 'bg-raised' : ''}"
        onmousemove={() => (index = i)}
        onclick={() => choose(item)}
      >
        {#if item.kind === 'host'}
          <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-accent/15 font-mono text-[11px] font-semibold text-accent" aria-hidden="true">
            {initials(item.host.name)}
          </span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[14px]">{item.host.name}</span>
            <span class="block truncate font-mono text-xs text-subtle">
              {describe(item.host)}{item.host.proxyJump ? ` · via ${item.host.proxyJump}` : ''}
            </span>
          </span>
          {#if item.host.group}<span class="shrink-0 text-xs text-subtle">{item.host.group}</span>{/if}
        {:else if item.kind === 'target'}
          <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-success/15 text-success"><Icon name="terminal" size={15} /></span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[14px]">Connect to <span class="font-mono">{item.target}</span></span>
            <span class="block text-xs text-subtle">Ad-hoc ssh session, not saved</span>
          </span>
        {:else}
          <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-raised text-muted"><Icon name={item.icon} size={15} /></span>
          <span class="flex-1 text-[14px] text-muted">{item.label}</span>
        {/if}
        {#if i === index}<kbd class="shrink-0 rounded border border-line px-1.5 font-mono text-[11px] text-subtle">↵</kbd>{/if}
      </li>
    {:else}
      <li class="px-3 py-6 text-center text-[13px] text-muted">No matches</li>
    {/each}
  </ul>
</dialog>
