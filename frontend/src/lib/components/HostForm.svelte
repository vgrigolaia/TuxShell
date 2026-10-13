<script lang="ts">
  import { onMount } from 'svelte'
  import { hostStore, initials, type Host, type SaveError } from '../hosts.svelte'
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

  onMount(() => nameInput.focus())

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
    'w-full rounded-lg border border-line bg-ground px-3 py-2 text-[13px] text-fg placeholder:text-subtle focus:border-accent focus:outline-none aria-[invalid=true]:border-warning'
  const label = 'mb-1 block text-xs text-muted'
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && formEl?.contains(document.activeElement) && onClose()} />

<aside
  aria-label={isNew ? 'New host' : `Edit ${host.name}`}
  class="flex w-80 shrink-0 flex-col overflow-hidden rounded-panel border border-line bg-panel"
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
    <div class="flex items-center gap-3 border-b border-line px-4 py-3">
      <span class="grid size-9 place-items-center rounded-lg bg-accent/15 font-mono text-xs font-semibold text-accent" aria-hidden="true">
        {draft.name ? initials(draft.name) : '?'}
      </span>
      <div class="min-w-0 flex-1">
        <h2 class="truncate font-display text-[15px] font-semibold">{isNew ? 'New host' : host.name}</h2>
        {#if draft.alias}
          <p class="truncate text-xs text-subtle">from ~/.ssh/config · {draft.alias}</p>
        {/if}
      </div>
      <button type="button" class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Close" onclick={onClose}>
        <Icon name="x" size={14} />
      </button>
    </div>

    <div class="flex-1 space-y-3 overflow-y-auto px-4 py-4">
      {#snippet field(id: string)}
        {@const msg = fieldError(id)}
        {#if msg}<p id="host-{id}-error" class="mt-1 text-xs text-warning">{msg}</p>{/if}
      {/snippet}

      <div>
        <label class={label} for="host-name">Name</label>
        <input id="host-name" class={input} bind:this={nameInput} bind:value={draft.name} placeholder="pg-primary"
          aria-invalid={!!fieldError('name')} aria-describedby="host-name-error" autocomplete="off" spellcheck="false" />
        {@render field('name')}
      </div>

      <div>
        <label class={label} for="host-group">Group</label>
        <input id="host-group" class={input} bind:value={draft.group} list="host-groups" placeholder="PostgreSQL · prod" autocomplete="off" />
        <datalist id="host-groups">
          {#each hostStore.groupNames as g (g)}<option value={g}></option>{/each}
        </datalist>
      </div>

      <div class="grid grid-cols-[1fr_5.5rem] gap-2">
        <div>
          <label class={label} for="host-hostname">Host</label>
          <input id="host-hostname" class={input} bind:value={draft.hostname} placeholder={draft.alias || '10.0.0.11'}
            aria-invalid={!!fieldError('hostname')} aria-describedby="host-hostname-error" autocomplete="off" spellcheck="false" />
        </div>
        <div>
          <label class={label} for="host-port">Port</label>
          <input id="host-port" class={input} type="number" min="1" max="65535" bind:value={draft.port}
            aria-invalid={!!fieldError('port')} aria-describedby="host-port-error" />
        </div>
      </div>
      {@render field('hostname')}
      {@render field('port')}

      <div>
        <label class={label} for="host-user">User</label>
        <input id="host-user" class={input} bind:value={draft.user} placeholder="Default from ssh config"
          aria-invalid={!!fieldError('user')} aria-describedby="host-user-error" autocomplete="off" spellcheck="false" />
        {@render field('user')}
      </div>

      <div>
        <label class={label} for="host-proxyJump">Jump host</label>
        <input id="host-proxyJump" class={input} bind:value={draft.proxyJump} list="host-jumps" placeholder="bastion-01"
          aria-invalid={!!fieldError('proxyJump')} aria-describedby="host-proxyJump-error" autocomplete="off" spellcheck="false" />
        <datalist id="host-jumps">
          {#each jumpCandidates as j (j)}<option value={j}></option>{/each}
        </datalist>
        {@render field('proxyJump')}
      </div>

      <div>
        <label class={label} for="host-identityFile">Key file</label>
        <input id="host-identityFile" class={input} bind:value={draft.identityFile} placeholder="~/.ssh/id_ed25519"
          aria-invalid={!!fieldError('identityFile')} aria-describedby="host-identityFile-error" autocomplete="off" spellcheck="false" />
        {@render field('identityFile')}
      </div>

      <fieldset class="rounded-xl border border-line bg-raised p-3">
        <legend class="flex items-center gap-1.5 px-1 text-xs text-muted"><Icon name="shield" size={13} /> 1Password</legend>
        <label class={label} for="host-passwordRef">Password reference</label>
        <input id="host-passwordRef" class="{input} font-mono" bind:value={draft.passwordRef} placeholder="op://Infra/pg-primary/password"
          aria-invalid={!!fieldError('passwordRef')} aria-describedby="host-passwordRef-error host-passwordRef-hint" autocomplete="off" spellcheck="false" />
        {@render field('passwordRef')}
        <p id="host-passwordRef-hint" class="mt-1.5 text-xs text-subtle">Only the reference is stored. The value is fetched when you connect.</p>
      </fieldset>

      <div>
        <label class={label} for="host-tags">Tags</label>
        <input id="host-tags" class={input} bind:value={tagText} placeholder="patroni, leader"
          aria-invalid={!!fieldError('tags')} aria-describedby="host-tags-error" autocomplete="off" />
        {@render field('tags')}
      </div>

      {#if error && !error.field}
        <p role="alert" class="rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">{error.message}</p>
      {/if}
    </div>

    <div class="flex items-center gap-2 border-t border-line p-3">
      {#if !isNew}
        {#if confirmDelete}
          <span class="text-xs text-muted">Delete?</span>
          <button type="button" class="rounded-lg bg-red-500/80 px-2.5 py-1.5 text-xs text-white hover:bg-red-500" onclick={remove}>Delete</button>
          <button type="button" class="rounded-lg px-2.5 py-1.5 text-xs text-muted hover:bg-raised" onclick={() => (confirmDelete = false)}>Keep</button>
        {:else}
          <button type="button" class="rounded-lg px-2.5 py-1.5 text-xs text-muted hover:bg-raised hover:text-fg" onclick={() => (confirmDelete = true)}>Delete</button>
        {/if}
      {/if}
      <span class="flex-1"></span>
      <button type="submit" disabled={saving} class="rounded-lg border border-line px-3 py-1.5 text-[13px] hover:bg-raised disabled:opacity-60">Save</button>
      {#if onConnect}
        <button type="button" disabled={saving} class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-ground hover:bg-accent/90 disabled:opacity-60" onclick={() => save(true)}>
          Save & connect
        </button>
      {/if}
    </div>
  </form>
</aside>
