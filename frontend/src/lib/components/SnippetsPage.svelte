<script lang="ts">
  import { Clipboard } from '@wailsio/runtime'
  import { onMount } from 'svelte'
  import { emptySnippet, snippets, type Snippet } from '../snippets.svelte'
  import Icon from './Icon.svelte'

  let editing = $state<Snippet | null>(null)
  let error = $state<{ field?: string; message: string } | null>(null)
  let saving = $state(false)
  let copied = $state<number | null>(null)

  onMount(() => void snippets.load())

  async function save() {
    if (!editing) return
    saving = true
    error = null
    try {
      editing = { ...(await snippets.save(editing)) }
    } catch (e) {
      const cause = (e as { cause?: { field?: string; message?: string } })?.cause
      error = { field: cause?.field, message: cause?.message ?? String((e as Error)?.message ?? e) }
    } finally {
      saving = false
    }
  }

  async function remove() {
    if (!editing?.id) return
    await snippets.remove(editing.id)
    editing = null
  }

  async function copy(sn: Snippet) {
    await Clipboard.SetText(sn.command)
    copied = sn.id
    setTimeout(() => copied === sn.id && (copied = null), 1200)
  }

  const fieldError = (f: string) => (error?.field === f ? error.message : null)
  const input =
    'w-full rounded-[11px] border border-strong bg-field px-3 text-[13.5px] text-fg placeholder:text-subtle focus:border-accent focus:outline-none focus-visible:outline-none aria-[invalid=true]:border-warning'
  const label = 'mb-1.5 block text-xs font-semibold text-text'
</script>

<main class="flex min-w-0 flex-1 flex-col gap-5 overflow-y-auto rounded-panel border border-line bg-ground px-7 py-6">
  <header class="flex flex-wrap items-center gap-3.5">
    <h1 class="font-display text-3xl font-bold tracking-[-0.03em]">Snippets</h1>
    <span class="text-sm text-subtle">{snippets.list.length} saved</span>
    <button class="ml-auto h-[42px] rounded-[13px] bg-accent px-[18px] text-sm font-bold text-on-accent hover:bg-accent/90" onclick={() => ((error = null), (editing = emptySnippet()))}>
      + New snippet
    </button>
  </header>
  <p class="-mt-2 max-w-2xl text-sm text-subtle">
    Saved commands. In a terminal, press <span class="kbd">Ctrl K</span> and pick a snippet to paste it; it never runs until you press Enter.
  </p>

  {#if snippets.error}<p role="alert" class="text-sm text-warning">{snippets.error}</p>{/if}

  {#if snippets.loaded && !snippets.list.length}
    <div class="flex flex-col items-center gap-3 rounded-[20px] bg-panel px-6 py-14 text-center shadow-[inset_0_0_0_1px_var(--color-line)]">
      <span class="grid size-12 place-items-center rounded-2xl bg-raised text-subtle"><Icon name="code" size={22} /></span>
      <p class="text-[15px] font-semibold">No snippets yet</p>
      <p class="max-w-sm text-sm text-subtle">Save the commands you type often, like checking replication lag or restarting a service.</p>
    </div>
  {/if}

  <ul class="grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-3">
    {#each snippets.list as sn (sn.id)}
      <li class="group relative">
        <button
          class="flex w-full flex-col gap-2 rounded-[14px] border bg-panel p-4 text-left hover:border-strong {editing?.id === sn.id ? 'border-accent' : 'border-line'}"
          onclick={() => ((error = null), (editing = { ...sn }))}
        >
          <span class="flex items-center gap-2 pr-8">
            <span class="text-accent-text"><Icon name="code" size={14} /></span>
            <span class="truncate text-sm font-semibold">{sn.name}</span>
            {#if sn.group}<span class="ml-auto shrink-0 text-xs text-subtle">{sn.group}</span>{/if}
          </span>
          <span class="line-clamp-2 font-mono text-xs break-all whitespace-pre-wrap text-text">{sn.command}</span>
        </button>
        <button
          class="absolute top-3 right-3 grid size-7 place-items-center rounded-lg text-subtle opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 hover:bg-hop hover:text-fg"
          aria-label="Copy {sn.name}"
          title="Copy command"
          onclick={() => copy(sn)}
        >
          <Icon name={copied === sn.id ? 'check' : 'copy'} size={13} />
        </button>
      </li>
    {/each}
  </ul>
</main>

{#if editing}
  {@const sn = editing}
  <aside aria-label={sn.id ? 'Edit snippet' : 'New snippet'} class="flex w-[360px] shrink-0 flex-col gap-[18px] overflow-y-auto rounded-panel border border-line bg-panel px-6 py-6">
    <div class="flex items-center gap-3">
      <span class="tint-violet grid size-11 place-items-center rounded-[14px]"><Icon name="code" size={20} /></span>
      <h2 class="flex-1 font-display text-lg font-bold">{sn.id ? 'Edit snippet' : 'New snippet'}</h2>
      <button class="grid size-7 place-items-center rounded-lg bg-hop text-muted hover:text-fg" aria-label="Close" onclick={() => (editing = null)}>
        <Icon name="x" size={12} />
      </button>
    </div>
    <form
      class="flex flex-1 flex-col gap-3"
      novalidate
      onsubmit={(e) => {
        e.preventDefault()
        void save()
      }}
    >
      <div>
        <label class={label} for="snippet-name">Name</label>
        <input id="snippet-name" class="{input} h-10" placeholder="Replication lag" autocomplete="off" bind:value={sn.name} aria-invalid={!!fieldError('name')} />
        {#if fieldError('name')}<p class="mt-1 text-xs text-warning">{fieldError('name')}</p>{/if}
      </div>
      <div class="flex flex-1 flex-col">
        <label class={label} for="snippet-command">Command</label>
        <textarea
          id="snippet-command"
          class="{input} min-h-40 flex-1 py-2.5 font-mono text-xs"
          placeholder="select * from pg_stat_replication;"
          spellcheck="false"
          bind:value={sn.command}
          aria-invalid={!!fieldError('command')}
        ></textarea>
        {#if fieldError('command')}<p class="mt-1 text-xs text-warning">{fieldError('command')}</p>{/if}
      </div>
      <div>
        <label class={label} for="snippet-group">Group (optional)</label>
        <input id="snippet-group" class="{input} h-10" placeholder="PostgreSQL" autocomplete="off" bind:value={sn.group} />
      </div>
      <p class="text-[11.5px] leading-normal text-subtle">Stored like your hosts (not encrypted), so keep passwords out of snippets.</p>
      {#if error && !error.field}<p role="alert" class="rounded-xl border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">{error.message}</p>{/if}
      <div class="mt-2 flex gap-2.5">
        {#if sn.id}
          <button type="button" class="h-11 flex-1 rounded-[13px] border border-strong text-sm font-semibold text-danger-text hover:bg-raised" onclick={remove}>Delete</button>
        {/if}
        <button type="submit" disabled={saving} class="h-11 flex-[2] rounded-[13px] bg-fg text-sm font-bold text-panel hover:opacity-90 disabled:opacity-60">Save</button>
      </div>
    </form>
  </aside>
{/if}
