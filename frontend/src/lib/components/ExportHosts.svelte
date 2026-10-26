<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity'
  import * as HostIO from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/hostio/service'
  import { describe, errorMessage, hostStore } from '../hosts.svelte'

  const id = $props.id()
  const formats = [
    { id: 'json', label: 'TuxShell (.json)', hint: 'Everything, for a backup or another computer. Imports back exactly.' },
    { id: 'csv', label: 'CSV', hint: 'Opens in Excel or LibreOffice, and imports back.' },
    { id: 'sshconfig', label: 'SSH config', hint: 'Host blocks for plain ssh and other tools. Password manager references are left out.' },
  ] as const

  let format = $state<(typeof formats)[number]['id']>('json')
  let scope = $state<'all' | 'group' | 'pick'>('all')
  let group = $state('')
  const picked = new SvelteSet<number>()
  let busy = $state(false)
  let message = $state<{ text: string; error: boolean } | null>(null)

  const groups = $derived([...new Set(hostStore.list.map((h) => h.group))].sort((a, b) => a.localeCompare(b)))
  const ids = $derived(
    scope === 'all' ? hostStore.list.map((h) => h.id) : scope === 'group' ? hostStore.list.filter((h) => h.group === group).map((h) => h.id) : [...picked],
  )

  $effect(() => {
    if (scope === 'group' && !groups.includes(group)) group = groups[0] ?? ''
  })

  async function run() {
    busy = true
    message = null
    try {
      const path = await HostIO.Export(format, ids)
      if (path) message = { text: `Exported ${ids.length} ${ids.length === 1 ? 'host' : 'hosts'} to ${path}`, error: false }
    } catch (e) {
      message = { text: errorMessage(e), error: true }
    } finally {
      busy = false
    }
  }
</script>

<section aria-labelledby="{id}-title" class="mt-6 border-t border-line pt-5">
  <h4 id="{id}-title" class="text-[13px] font-medium">Export hosts</h4>
  <p class="mt-0.5 text-xs text-subtle">Exports never contain passwords: TuxShell doesn't store any. The file is readable only by you.</p>

  <fieldset class="mt-3">
    <legend class="mb-1.5 text-xs font-medium text-muted">Format</legend>
    <div class="flex flex-col gap-1.5">
      {#each formats as f (f.id)}
        <label class="flex items-start gap-2.5 text-[13px]">
          <input type="radio" class="mt-0.5 accent-accent" name="{id}-format" value={f.id} bind:group={format} />
          <span>{f.label}<span class="mt-0.5 block text-xs text-subtle">{f.hint}</span></span>
        </label>
      {/each}
    </div>
  </fieldset>

  <fieldset class="mt-4">
    <legend class="mb-1.5 text-xs font-medium text-muted">Hosts</legend>
    <div class="flex flex-wrap items-center gap-x-5 gap-y-2 text-[13px]">
      <label class="flex items-center gap-2"><input type="radio" class="accent-accent" name="{id}-scope" value="all" bind:group={scope} /> All ({hostStore.list.length})</label>
      <label class="flex items-center gap-2"><input type="radio" class="accent-accent" name="{id}-scope" value="group" bind:group={scope} disabled={groups.length === 0} /> One group</label>
      <label class="flex items-center gap-2"><input type="radio" class="accent-accent" name="{id}-scope" value="pick" bind:group={scope} /> Choose hosts</label>
    </div>
    {#if scope === 'group'}
      <select class="mt-2 h-8 w-64 rounded-md border border-line bg-ground px-1.5 text-xs text-fg focus:border-accent focus:outline-none" aria-label="Group to export" bind:value={group}>
        {#each groups as g (g)}<option value={g}>{g || 'No group'} ({hostStore.list.filter((h) => h.group === g).length})</option>{/each}
      </select>
    {:else if scope === 'pick'}
      <ul class="mt-2 max-h-48 overflow-y-auto rounded-lg border border-line bg-ground p-1" aria-label="Hosts to export">
        {#each hostStore.list as h (h.id)}
          <li>
            <label class="flex items-center gap-2.5 rounded-md px-2 py-1 text-xs hover:bg-raised">
              <input
                type="checkbox"
                class="size-3.5 accent-accent"
                checked={picked.has(h.id)}
                onchange={(e) => (e.currentTarget.checked ? picked.add(h.id) : picked.delete(h.id))}
              />
              <span class="min-w-0 flex-1 truncate">{h.name}</span>
              <span class="truncate font-mono text-[11px] text-subtle">{describe(h)}</span>
            </label>
          </li>
        {/each}
      </ul>
    {/if}
  </fieldset>

  <div class="mt-4 flex items-center gap-3">
    <button class="rounded-lg bg-accent px-3 py-1.5 text-xs font-medium text-on-accent hover:bg-accent/90 disabled:opacity-50" disabled={busy || ids.length === 0} onclick={run}>
      {busy ? 'Exporting…' : `Export ${ids.length} ${ids.length === 1 ? 'host' : 'hosts'}…`}
    </button>
    {#if message}
      <p role="status" class="min-w-0 truncate text-xs {message.error ? 'text-warning' : 'text-success'}" title={message.text}>{message.text}</p>
    {/if}
  </div>
</section>
