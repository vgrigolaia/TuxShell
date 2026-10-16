<script lang="ts">
  import * as HostIO from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/hostio/service'
  import type { Preview, Result, Row } from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/hostio/models'
  import { describe, errorMessage, hostStore } from '../hosts.svelte'
  import HostBadge from './HostBadge.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    /** A preview just opened by HostIO.Open. */
    preview: Preview
    close: () => void
  }
  let { preview: initial, close }: Props = $props()

  // svelte-ignore state_referenced_locally
  let preview = $state<Preview>(initial)
  let chosen = $state<Record<number, boolean>>({})
  // svelte-ignore state_referenced_locally
  let duplicate = $state<'skip' | 'update' | 'copy' | 'passwords'>(initial.passwords && initial.savePasswords ? 'passwords' : 'skip')
  // svelte-ignore state_referenced_locally
  let group = $state(initial.tabular ? 'Imported' : '')
  let busy = $state(false)
  let error = $state<string | null>(null)
  let result = $state<Result | null>(null)
  const id = $props.id()

  const fieldLabels: Record<string, string> = {
    '': "Don't import",
    name: 'Name',
    hostname: 'Host',
    user: 'User',
    port: 'Port',
    group: 'Group',
    tags: 'Tags',
    proxyJump: 'Jump host',
    alias: 'SSH config alias',
    os: 'Operating system',
    identityFile: 'Key file',
    password: '1Password (op://)',
    protocol: 'Type (SSH or RDP)',
    domain: 'Windows domain',
    rdpOptions: 'Remote desktop options',
  }

  const rows = $derived(preview.rows ?? [])
  const columns = $derived(preview.columns ?? [])
  const importable = (r: Row) => !r.error && !r.sameAs
  const selected = $derived(rows.filter((r) => importable(r) && chosen[r.line]))
  // Search narrows the list; select all acts on what's shown. Rows ticked
  // earlier stay ticked while hidden (the Import button counts them).
  let query = $state('')
  const shown = $derived.by(() => {
    const words = query.toLowerCase().split(/\s+/).filter(Boolean)
    if (!words.length) return rows
    return rows.filter((r) => {
      const h = r.host
      const text = [h.name, h.hostname, h.user, h.group || group, h.proxyJump, h.os, h.protocol, ...(h.tags ?? []), describe(h)].join(' ').toLowerCase()
      return words.every((w) => text.includes(w))
    })
  })
  const choosable = $derived(shown.filter(importable))
  const shownSelected = $derived(choosable.filter((r) => chosen[r.line]).length)

  // The header box: ticks or clears every shown row that can be imported.
  function chooseAll(on: boolean) {
    for (const r of choosable) chosen[r.line] = on
  }
  const existing = $derived(rows.filter((r) => importable(r) && r.existingId).length)
  const hasHost = $derived(!preview.tabular || columns.some((c) => c.field === 'hostname' || c.field === 'alias'))

  // New rows start selected; a remap keeps the choices already made.
  function adopt(p: Preview) {
    for (const r of p.rows ?? []) if (importable(r) && chosen[r.line] === undefined) chosen[r.line] = true
    preview = p
  }
  // svelte-ignore state_referenced_locally
  adopt(initial)

  async function remap(fields: string[], header: boolean) {
    error = null
    try {
      const next = await HostIO.Remap(preview.id, fields, header)
      if (next) adopt(next)
    } catch (e) {
      error = errorMessage(e)
    }
  }

  function setField(i: number, field: string) {
    // A field can only fill one column: the column that had it goes back to "Don't import".
    const fields = columns.map((c, j) => (j === i ? field : c.field === field && field ? '' : c.field))
    void remap(fields, preview.header)
  }

  async function run() {
    busy = true
    error = null
    try {
      result = await HostIO.Import(preview.id, selected.map((r) => r.line), duplicate, group)
      await hostStore.load()
    } catch (e) {
      error = errorMessage(e)
    } finally {
      busy = false
    }
  }

  function status(r: Row): { text: string; tone: string } {
    if (r.error) return { text: r.error, tone: 'text-warning' }
    if (r.sameAs) return { text: `Same as line ${r.sameAs}`, tone: 'text-subtle' }
    if (r.existingId) return { text: `Already saved as “${r.existing}”`, tone: 'text-muted' }
    return { text: 'New', tone: 'text-success' }
  }
</script>

<div class="min-h-0 flex-auto overflow-y-auto px-5 py-3">
  {#if error}
    <p role="alert" class="mb-3 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">{error}</p>
  {/if}

  {#if result}
    <div role="status" class="space-y-2 py-2 text-[13px]">
      <p>
        <span class="text-success">Imported {result.imported} {result.imported === 1 ? 'host' : 'hosts'}</span>{#if result.updated}, updated {result.updated}{/if}{#if result.skipped}, skipped {result.skipped}{/if}.
      </p>
      {#if result.errors?.length}
        <ul class="list-inside list-disc text-xs text-subtle">
          {#each result.errors as e (e)}<li>{e}</li>{/each}
        </ul>
      {/if}
      {#if result.passwords}
        <p class="text-xs text-muted">{result.passwords} {result.passwords === 1 ? 'password was' : 'passwords were'} saved in your system keyring.</p>
      {/if}
      {#if preview.passwords}
        {#if preview.source !== 'remmina'}<p class="text-xs text-muted">{preview.savePasswords ? '' : "Passwords weren't imported. "}Consider deleting <span class="font-mono">{preview.file}</span> now: it holds passwords in plain text.</p>{/if}
      {/if}
    </div>
  {:else}
    {#if preview.passwords}
      <p class="mb-3 rounded-lg border border-line bg-raised px-3 py-2 text-xs text-muted">
        {#if preview.savePasswords}
          <span class="font-medium text-fg">{preview.passwords} {preview.passwords === 1 ? 'password' : 'passwords'} will be saved</span>
          encrypted in your system keyring, for the hosts you import.
        {:else}
          <span class="font-medium text-fg">{preview.passwords} {preview.passwords === 1 ? 'password' : 'passwords'} won't be imported.</span>
          To import them, turn on <span class="font-medium">Allow saving passwords</span> in Preferences → Security and open the file again. 1Password references (op://…) are always kept.
        {/if}
      </p>
    {/if}

    {#if preview.tabular}
    <fieldset>
      <legend class="mb-2 flex w-full items-center gap-3 text-xs font-medium text-muted">
        Columns
        <label class="ml-auto flex items-center gap-2 font-normal">
          <input type="checkbox" class="size-4 accent-accent" checked={preview.header} onchange={(e) => void remap(columns.map((c) => c.field), e.currentTarget.checked)} />
          First row has column names
        </label>
      </legend>
      <div class="grid grid-cols-[repeat(auto-fill,minmax(148px,1fr))] gap-2">
        {#each columns as c, i (i)}
          <div class="min-w-0 rounded-lg border border-line bg-ground p-2">
            <label for="{id}-col-{i}" class="block truncate text-[11px] text-subtle">{c.header || `Column ${i + 1}`}</label>
            <select id="{id}-col-{i}" class="mt-1 h-8 w-full rounded-md border border-line bg-panel px-1.5 text-xs text-fg focus:border-accent focus:outline-none" value={c.field} onchange={(e) => setField(i, e.currentTarget.value)}>
              {#each Object.entries(fieldLabels) as [value, text] (value)}<option {value}>{text}</option>{/each}
            </select>
            <p class="mt-1.5 truncate font-mono text-[11px] text-subtle" title={(c.sample ?? []).join(', ')}>{(c.sample ?? []).join(', ') || '—'}</p>
          </div>
        {/each}
      </div>
      {#if !hasHost}<p role="alert" class="text-xs text-warning">Choose which column holds the host (name or IP address).</p>{/if}
    </fieldset>
    {/if}

    <div class="mt-3 grid grid-cols-2 gap-3">
      <div>
        <label class="mb-1 block text-xs font-medium text-muted" for="{id}-group">Group for hosts without one</label>
        <input id="{id}-group" class="h-8 w-full rounded-md border border-line bg-ground px-2 text-xs text-fg focus:border-accent focus:outline-none" bind:value={group} placeholder="No group" />
      </div>
      <div>
        <label class="mb-1 block text-xs font-medium text-muted" for="{id}-dup">Hosts you already have ({existing})</label>
        <select id="{id}-dup" class="h-8 w-full rounded-md border border-line bg-ground px-1.5 text-xs text-fg focus:border-accent focus:outline-none" bind:value={duplicate} disabled={!existing}>
          <option value="skip">Skip them</option>
          {#if preview.passwords && preview.savePasswords}<option value="passwords">Only add their passwords</option>{/if}
          <option value="update">Update them from the file</option>
          <option value="copy">Import them as copies</option>
        </select>
      </div>
    </div>

    <div class="mt-4 flex items-center gap-3">
      <label class="flex h-8 min-w-0 flex-1 items-center gap-2 rounded-md border border-line bg-ground px-2 text-subtle focus-within:border-accent">
        <Icon name="search" size={13} />
        <input class="h-full min-w-0 flex-1 bg-transparent text-xs text-fg placeholder:text-subtle focus:outline-none" type="search" placeholder="Search name, address, user or group" aria-label="Search the hosts in this file" bind:value={query} />
      </label>
      <span class="shrink-0 text-xs text-subtle">{query ? `${shown.length} of ${rows.length} shown · ` : ''}{selected.length} selected</span>
    </div>

    <table class="mt-2 w-full text-left text-xs">
      <caption class="sr-only">Hosts in {preview.file}</caption>
      <thead class="text-subtle">
        <tr class="border-b border-line">
          <th class="w-8 py-1.5">
            <input
              type="checkbox"
              class="size-4 accent-accent"
              aria-label="Select all"
              title="Select all / none"
              disabled={choosable.length === 0}
              checked={choosable.length > 0 && shownSelected === choosable.length}
              indeterminate={shownSelected > 0 && shownSelected < choosable.length}
              onchange={(e) => chooseAll(e.currentTarget.checked)}
            />
          </th>
          <th class="w-10 py-1.5 font-normal">Line</th>
          <th class="py-1.5 font-normal">Name</th>
          <th class="py-1.5 font-normal">Connects to</th>
          <th class="py-1.5 font-normal">Group</th>
          <th class="py-1.5 font-normal">Status</th>
        </tr>
      </thead>
      <tbody>
        {#each shown as r (r.line)}
          {@const st = status(r)}
          <tr class="border-b border-line last:border-0" class:opacity-60={!importable(r)}>
            <td class="py-1.5">
              <input type="checkbox" class="size-4 accent-accent" aria-label="Import line {r.line}" bind:checked={chosen[r.line]} disabled={!importable(r)} />
            </td>
            <td class="py-1.5 font-mono text-subtle">{r.line}</td>
            <td class="max-w-40 py-1.5"><span class="flex items-center gap-2"><HostBadge name={r.host.name} os={r.host.os} size={20} /><span class="truncate">{r.host.name}</span>{#if r.host.protocol === 'rdp'}<span class="shrink-0 rounded bg-hop px-1.5 py-px text-[10px] font-semibold text-muted" title="Remote desktop">RDP</span>{/if}</span></td>
            <td class="max-w-48 truncate py-1.5 font-mono text-muted">{describe(r.host)}{r.host.proxyJump ? ` · via ${r.host.proxyJump}` : ''}</td>
            <td class="max-w-28 truncate py-1.5 text-muted">{r.host.group || group}</td>
            <td class="max-w-56 truncate py-1.5 {st.tone}" title={st.text}>{st.text}</td>
          </tr>
        {:else}
          <tr><td colspan="6" class="py-6 text-center text-muted">No hosts match “{query}”</td></tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

<div class="flex items-center justify-end gap-2 border-t border-line px-5 py-3">
  {#if result}
    <button class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-on-accent hover:bg-accent/90" onclick={() => close()}>Done</button>
  {:else}
    <span class="mr-auto truncate text-xs text-subtle">{preview.file} · {rows.length} {rows.length === 1 ? 'row' : 'rows'}</span>
    <button class="rounded-lg px-3 py-1.5 text-[13px] text-muted hover:bg-raised hover:text-fg" onclick={() => close()}>Cancel</button>
    <button
      class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-on-accent hover:bg-accent/90 disabled:opacity-50"
      disabled={busy || selected.length === 0 || !hasHost}
      onclick={run}
    >
      {busy ? 'Importing…' : `Import ${selected.length || ''}`}
    </button>
  {/if}
</div>
