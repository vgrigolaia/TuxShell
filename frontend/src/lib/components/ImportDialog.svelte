<script lang="ts">
  import { onMount } from 'svelte'
  import * as SSHConfig from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/sshconfig/service'
  import type { Candidate } from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/sshconfig/models'
  import { errorMessage, hostStore } from '../hosts.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    onClose: () => void
  }
  let { onClose }: Props = $props()

  let dialog: HTMLDialogElement
  let candidates = $state<Candidate[] | null>(null)
  let chosen = $state<Record<string, boolean>>({})
  let error = $state<string | null>(null)
  let busy = $state(false)
  let result = $state<{ imported: number; skipped: string[] } | null>(null)

  const importable = $derived((candidates ?? []).filter((c) => !c.imported && !c.error))
  const selected = $derived(importable.filter((c) => chosen[c.alias]))
  const allChosen = $derived(importable.length > 0 && selected.length === importable.length)

  onMount(() => {
    dialog.showModal()
    void scan()
  })

  async function scan() {
    try {
      candidates = (await SSHConfig.Scan()) ?? []
      chosen = Object.fromEntries(candidates.filter((c) => !c.imported && !c.error).map((c) => [c.alias, true]))
    } catch (e) {
      error = errorMessage(e)
    }
  }

  async function run() {
    busy = true
    error = null
    try {
      const res = await SSHConfig.Import(selected.map((c) => c.alias))
      result = { imported: res.imported, skipped: res.skipped ?? [] }
      await hostStore.load()
    } catch (e) {
      error = errorMessage(e)
    } finally {
      busy = false
    }
  }

  function toggleAll() {
    const next = !allChosen
    for (const c of importable) chosen[c.alias] = next
  }

  function describe(c: Candidate) {
    const user = c.user ? `${c.user}@` : ''
    const port = c.port && c.port !== 22 ? `:${c.port}` : ''
    const via = c.proxyJump ? ` · via ${c.proxyJump}` : ''
    return `${user}${c.hostname}${port}${via}`
  }
</script>

<dialog
  bind:this={dialog}
  aria-labelledby="import-title"
  class="m-auto flex max-h-[80vh] w-[560px] flex-col overflow-hidden rounded-2xl border border-line bg-panel p-0 text-fg shadow-2xl backdrop:bg-black/60"
  onclose={onClose}
>
  <div class="flex items-center gap-3 border-b border-line px-5 py-4">
    <span class="grid size-8 place-items-center rounded-lg bg-accent/15 text-accent"><Icon name="import" size={16} /></span>
    <div class="flex-1">
      <h2 id="import-title" class="font-display text-[15px] font-semibold">Import from ~/.ssh/config</h2>
      <p class="text-xs text-subtle">Hosts connect through their alias, so the rest of your config still applies.</p>
    </div>
    <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Close" onclick={() => dialog.close()}>
      <Icon name="x" size={14} />
    </button>
  </div>

  <div class="min-h-0 flex-1 overflow-y-auto px-2 py-2">
    {#if error}
      <p role="alert" class="m-3 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">{error}</p>
    {/if}

    {#if result}
      <div role="status" class="space-y-2 px-3 py-4 text-[13px]">
        <p><span class="text-success">Imported {result.imported} {result.imported === 1 ? 'host' : 'hosts'}</span> into the "SSH config" group.</p>
        {#if result.skipped.length}
          <p class="text-muted">Skipped:</p>
          <ul class="list-inside list-disc font-mono text-xs text-subtle">
            {#each result.skipped as s (s)}<li>{s}</li>{/each}
          </ul>
        {/if}
      </div>
    {:else if candidates === null && !error}
      <p class="px-3 py-6 text-center text-[13px] text-muted">Reading your ssh config…</p>
    {:else if candidates && candidates.length === 0}
      <p class="px-3 py-6 text-center text-[13px] text-muted">No Host entries found in ~/.ssh/config.</p>
    {:else if candidates}
      <fieldset>
        <legend class="sr-only">Hosts to import</legend>
        <label class="flex items-center gap-3 rounded-lg px-3 py-2 text-xs text-muted hover:bg-raised">
          <input type="checkbox" class="size-4 accent-accent" checked={allChosen} disabled={importable.length === 0} onchange={toggleAll} />
          Select all ({importable.length} new)
        </label>
        {#each candidates as c (c.alias)}
          <label class="flex items-center gap-3 rounded-lg px-3 py-2 hover:bg-raised" class:opacity-60={c.imported || !!c.error}>
            <input type="checkbox" class="size-4 accent-accent" bind:checked={chosen[c.alias]} disabled={c.imported || !!c.error} />
            <span class="min-w-0 flex-1">
              <span class="block truncate text-[13px]">{c.alias}</span>
              <span class="block truncate font-mono text-[11px] {c.error ? 'text-warning' : 'text-subtle'}">{c.error || describe(c)}</span>
            </span>
            {#if c.imported}<span class="shrink-0 rounded-md bg-raised px-2 py-0.5 text-[11px] text-muted">added</span>{/if}
          </label>
        {/each}
      </fieldset>
    {/if}
  </div>

  <div class="flex items-center justify-end gap-2 border-t border-line px-5 py-3">
    {#if result}
      <button class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-ground hover:bg-accent/90" onclick={() => dialog.close()}>Done</button>
    {:else}
      <button class="rounded-lg px-3 py-1.5 text-[13px] text-muted hover:bg-raised hover:text-fg" onclick={() => dialog.close()}>Cancel</button>
      <button
        class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-ground hover:bg-accent/90 disabled:opacity-50"
        disabled={busy || selected.length === 0}
        onclick={run}
      >
        {busy ? 'Importing…' : `Import ${selected.length || ''}`}
      </button>
    {/if}
  </div>
</dialog>
