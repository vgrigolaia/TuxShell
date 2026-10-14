<script lang="ts">
  import { onMount } from 'svelte'
  import { errorMessage, hostStore, type Host, type SaveError } from '../hosts.svelte'
  import * as Status from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/status/service'
  import HostBadge from './HostBadge.svelte'
  import { OSES } from '../os'
  import Icon from './Icon.svelte'

  interface Props {
    host: Host
    onClose: () => void
    onSaved: (host: Host) => void
    onConnect?: (host: Host) => void
  }
  let { host, onClose, onSaved, onConnect }: Props = $props()

  // Edit a copy so Cancel discards changes.
  // svelte-ignore state_referenced_locally
  let draft = $state<Host>({ ...host, tags: [...(host.tags ?? [])] })
  // svelte-ignore state_referenced_locally
  let tagText = $state((host.tags ?? []).join(', '))
  let error = $state<SaveError | null>(null)
  let saving = $state(false)
  let confirmDelete = $state(false)
  let nameInput: HTMLInputElement
  let formEl: HTMLFormElement

  const isNew = $derived(draft.id === 0)
  const jumpCandidates = $derived(hostStore.list.filter((h) => h.id !== draft.id).map((h) => h.alias || h.hostname))

  // New hosts start typing in Name; picking an existing host shouldn't steal focus.
  onMount(() => {
    if (isNew) nameInput.focus()
  })

  let testing = $state(false)
  let tested = $state<{ ok: boolean; message: string } | null>(null)

  // Logs in with the values as typed (not saved) and reports the result.
  async function test() {
    testing = true
    tested = null
    error = null
    try {
      const r = await Status.Test({ ...draft, port: Number(draft.port) || 0, tags: [] })
      tested = { ok: r.ok, message: r.ok ? `${r.message} · ${Math.round(r.ms)} ms` : r.message }
    } catch (e) {
      const cause = (e as { cause?: SaveError })?.cause
      if (cause?.field) error = cause
      else tested = { ok: false, message: errorMessage(e) }
    } finally {
      testing = false
    }
  }

  function fieldError(field: string) {
    return error?.field === field ? error.message : null
  }

  async function save(connect: boolean) {
    saving = true
    error = null
    draft.tags = tagText.split(',').map((t) => t.trim()).filter(Boolean)
    draft.port = Number(draft.port) || 0
    try {
      const saved = await hostStore.save(draft)
      onSaved(saved)
      if (connect) onConnect?.(saved)
    } catch (e) {
      error = e as SaveError
      if (error.field) document.getElementById(`host-${error.field}`)?.focus()
    } finally {
      saving = false
    }
  }

  async function remove() {
    try {
      await hostStore.remove(draft.id)
      onClose()
    } catch (e) {
      error = { message: String((e as Error)?.message ?? e) }
    }
  }

  const input =
    'h-10 w-full rounded-[11px] border border-strong bg-field px-3 text-[13.5px] text-fg placeholder:text-subtle focus:border-accent focus:outline-none focus-visible:outline-none aria-[invalid=true]:border-warning'
  const label = 'mb-1.5 block text-xs font-semibold text-text'
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && formEl?.contains(document.activeElement) && onClose()} />

<aside
  aria-label={isNew ? 'New host' : `Edit ${host.name}`}
  class="flex w-[360px] shrink-0 flex-col overflow-hidden rounded-panel border border-line bg-panel"
>
  <form
    class="flex min-h-0 flex-1 flex-col"
    onsubmit={(e) => {
      e.preventDefault()
      void save(false)
    }}
    bind:this={formEl}
    novalidate
  >
    <div class="flex items-center gap-3 px-6 pt-6 pb-2">
      <HostBadge name={draft.name} os={draft.os} size={44} />
      <div class="min-w-0 flex-1">
        <h2 class="truncate font-display text-lg font-bold">{isNew ? 'New host' : host.name}</h2>
        <p class="truncate text-xs text-subtle">
          {draft.alias ? `from ~/.ssh/config · ${draft.alias}` : draft.group || (isNew ? 'Saved hosts connect with your ssh' : 'Saved host')}
        </p>
      </div>
      <button type="button" class="grid size-7 place-items-center rounded-lg bg-hop text-muted hover:text-fg" aria-label="Close" onclick={onClose}>
        <Icon name="x" size={12} />
      </button>
    </div>

    <div class="flex flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-4">
      {#snippet field(id: string)}
        {@const msg = fieldError(id)}
        {#if msg}<p id="host-{id}-error" class="mt-1 text-xs text-warning">{msg}</p>{/if}
      {/snippet}

      <div class="grid grid-cols-3 gap-2.5">
        <div class="col-span-3">
          <label class={label} for="host-name">Name</label>
          <input id="host-name" class={input} bind:this={nameInput} bind:value={draft.name} placeholder="pg-primary"
            aria-invalid={!!fieldError('name')} aria-describedby="host-name-error" autocomplete="off" spellcheck="false" />
          {@render field('name')}
        </div>
        <div class="col-span-2">
          <label class={label} for="host-hostname">Host</label>
          <input id="host-hostname" class={input} bind:value={draft.hostname} placeholder={draft.alias || '10.0.0.11'}
            aria-invalid={!!fieldError('hostname')} aria-describedby="host-hostname-error" autocomplete="off" spellcheck="false" />
        </div>
        <div>
          <label class={label} for="host-port">Port</label>
          <input id="host-port" class={input} type="number" min="1" max="65535" bind:value={draft.port}
            aria-invalid={!!fieldError('port')} aria-describedby="host-port-error" />
        </div>
        <div class="col-span-3 -mt-1.5 empty:hidden">{@render field('hostname')}{@render field('port')}</div>
        <div class="col-span-3">
          <label class={label} for="host-user">User</label>
          <input id="host-user" class={input} bind:value={draft.user} placeholder="Asked when you connect"
            aria-invalid={!!fieldError('user')} aria-describedby="host-user-error" autocomplete="off" spellcheck="false" />
          {@render field('user')}
        </div>
        <div class="col-span-3">
          <label class={label} for="host-proxyJump">Jump host</label>
          <input id="host-proxyJump" class={input} bind:value={draft.proxyJump} list="host-jumps" placeholder="bastion-01"
            aria-invalid={!!fieldError('proxyJump')} aria-describedby="host-proxyJump-error" autocomplete="off" spellcheck="false" />
          <datalist id="host-jumps">
            {#each jumpCandidates as j (j)}<option value={j}></option>{/each}
          </datalist>
          {@render field('proxyJump')}
        </div>
        <div class="col-span-3">
          <label class={label} for="host-group">Group</label>
          <input id="host-group" class={input} bind:value={draft.group} list="host-groups" placeholder="PostgreSQL · prod" autocomplete="off" />
          <datalist id="host-groups">
            {#each hostStore.groupNames as g (g)}<option value={g}></option>{/each}
          </datalist>
        </div>
        <div class="col-span-3">
          <label class={label} for="host-identityFile">Key file</label>
          <input id="host-identityFile" class="{input} font-mono text-xs" bind:value={draft.identityFile} placeholder="~/.ssh/id_ed25519"
            aria-invalid={!!fieldError('identityFile')} aria-describedby="host-identityFile-error" autocomplete="off" spellcheck="false" />
          {@render field('identityFile')}
        </div>
        <div class="col-span-3">
          <label class={label} for="host-tags">Tags</label>
          <input id="host-tags" class={input} bind:value={tagText} placeholder="patroni, leader"
            aria-invalid={!!fieldError('tags')} aria-describedby="host-tags-error" autocomplete="off" />
          {@render field('tags')}
        </div>
        <div class="col-span-3">
          <label class={label} for="host-os">Operating system <span class="font-normal text-subtle">(optional)</span></label>
          <select id="host-os" class={input} bind:value={draft.os} aria-invalid={!!fieldError('os')} aria-describedby="host-os-error">
            <option value="">Not set</option>
            {#each Object.entries(OSES) as [value, o] (value)}<option {value}>{o.label}</option>{/each}
          </select>
          {@render field('os')}
        </div>
      </div>

      <fieldset class="flex flex-col gap-2.5 rounded-2xl border border-accent-line bg-accent-soft p-4">
        <legend class="sr-only">1Password</legend>
        <p class="flex items-center gap-2 text-[13px] font-bold text-accent-ink" aria-hidden="true">
          <span class="text-accent-text"><Icon name="shield" size={15} /></span>Credentials from 1Password
        </p>
        <div>
          <label class={label} for="host-passwordRef">Password reference</label>
          <input id="host-passwordRef" class="{input} font-mono text-xs" bind:value={draft.passwordRef} placeholder="op://Infra/pg-primary/password"
            aria-invalid={!!fieldError('passwordRef')} aria-describedby="host-passwordRef-error host-passwordRef-hint" autocomplete="off" spellcheck="false" />
          {@render field('passwordRef')}
        </div>
        <p id="host-passwordRef-hint" class="text-[11.5px] leading-normal text-accent-ink/80">
          Only the reference is stored, never the value. Signing in with it arrives with 1Password support in a later version.
        </p>
      </fieldset>

      {#if tested}
        <p role="status" class="rounded-xl border px-3 py-2 text-xs {tested.ok ? 'border-success/40 bg-success/10 text-success' : 'border-warning/40 bg-warning/10 text-warning'}">
          {tested.ok ? '✓ ' : ''}{tested.message}
        </p>
      {/if}
      {#if error && !error.field}
        <p role="alert" class="rounded-xl border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">{error.message}</p>
      {/if}
    </div>

    <div class="flex flex-col gap-2.5 px-6 pt-2 pb-6">
      <div class="flex items-center gap-2 text-xs">
        {#if !isNew}
          {#if confirmDelete}
            <span class="text-muted">Delete this host?</span>
            <button type="button" class="rounded-lg bg-danger px-2.5 py-1 text-white hover:bg-danger/90" onclick={remove}>Delete</button>
            <button type="button" class="rounded-lg px-2.5 py-1 text-muted hover:bg-raised" onclick={() => (confirmDelete = false)}>Keep</button>
          {:else}
            <button type="button" class="rounded-lg px-2 py-1 text-muted hover:bg-raised hover:text-fg" onclick={() => (confirmDelete = true)}>Delete…</button>
          {/if}
        {/if}
        <span class="flex-1"></span>
        <button type="submit" disabled={saving} class="rounded-lg px-2.5 py-1 font-semibold text-accent-text hover:bg-raised disabled:opacity-60">Save</button>
      </div>
      <div class="flex gap-2.5">
        <button type="button" disabled={testing} class="h-11 flex-1 rounded-[13px] border border-strong bg-panel text-sm font-semibold hover:bg-raised disabled:opacity-60" onclick={test}>
          {testing ? 'Testing…' : 'Test'}
        </button>
        {#if onConnect}
          <button type="button" disabled={saving} class="h-11 flex-[2] rounded-[13px] bg-fg text-sm font-bold text-panel hover:opacity-90 disabled:opacity-60" onclick={() => save(true)}>
            Save & connect
          </button>
        {/if}
      </div>
    </div>
  </form>
</aside>
