<script lang="ts">
  import { hostStore } from '../hosts.svelte'
  import { describeTunnel, emptyTunnel, tunnels, type Tunnel, type TunnelState } from '../tunnels.svelte'
  import Icon from './Icon.svelte'

  let editing = $state<Tunnel | null>(null)
  let error = $state<{ field?: string; message: string } | null>(null)
  let saving = $state(false)
  let now = $state(Date.now())

  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 30000)
    return () => clearInterval(t)
  })

  function since(ms: number) {
    const m = Math.max(0, Math.floor((now - ms) / 60000))
    return m < 1 ? 'just now' : m < 60 ? `${m}m` : `${Math.floor(m / 60)}h ${m % 60}m`
  }

  function edit(t: Tunnel | null) {
    error = null
    editing = t ? { ...t } : emptyTunnel(hostStore.list.find((h) => h.protocol !== 'rdp')?.id ?? 0)
  }

  async function save() {
    if (!editing) return
    saving = true
    error = null
    try {
      const saved = await tunnels.save({ ...editing, bindPort: Number(editing.bindPort) || 0, targetPort: Number(editing.targetPort) || 0 })
      editing = { ...saved }
    } catch (e) {
      const cause = (e as { cause?: { field?: string; message?: string } })?.cause
      error = { field: cause?.field, message: cause?.message ?? String((e as Error)?.message ?? e) }
    } finally {
      saving = false
    }
  }

  async function remove() {
    if (!editing?.id) return
    await tunnels.remove(editing.id)
    editing = null
  }

  const fieldError = (f: string) => (error?.field === f ? error.message : null)
  const input =
    'h-10 w-full rounded-[11px] border border-strong bg-field px-3 text-[13.5px] text-fg placeholder:text-subtle focus:border-accent focus:outline-none focus-visible:outline-none aria-[invalid=true]:border-warning'
  const label = 'mb-1.5 block text-xs font-semibold text-text'
</script>

<main class="flex min-w-0 flex-1 flex-col gap-5 overflow-y-auto rounded-panel border border-line bg-ground px-7 py-6">
  <header class="flex flex-wrap items-center gap-3.5">
    <h1 class="font-display text-3xl font-bold tracking-[-0.03em]">Tunnels</h1>
    <span class="text-sm text-subtle">{tunnels.running} running</span>
    <button class="ml-auto h-[42px] rounded-[13px] bg-accent px-[18px] text-sm font-bold text-on-accent hover:bg-accent/90 disabled:opacity-50" disabled={!hostStore.list.length} onclick={() => edit(null)}>
      + New forward
    </button>
  </header>
  <p class="-mt-2 max-w-2xl text-sm text-subtle">
    Forward a local port through a saved host, or open a SOCKS proxy. Tunnels listen on 127.0.0.1 only, so other machines can't use them.
  </p>

  {#if tunnels.error}<p role="alert" class="text-sm text-warning">{tunnels.error}</p>{/if}

  {#if !tunnels.list.length}
    <div class="flex flex-col items-center gap-3 rounded-[20px] bg-panel px-6 py-14 text-center shadow-[inset_0_0_0_1px_var(--color-line)]">
      <span class="grid size-12 place-items-center rounded-2xl bg-raised text-subtle"><Icon name="tunnel" size={22} /></span>
      <p class="text-[15px] font-semibold">No tunnels yet</p>
      <p class="max-w-sm text-sm text-subtle">For example, reach a database on a server as localhost:5432 on this computer.</p>
    </div>
  {/if}

  <ul class="grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-3">
    {#each tunnels.list as t (t.id)}
      {@const busy = tunnels.starting.includes(t.id)}
      {@const host = hostStore.get(t.hostId)}
      <li class="flex flex-col gap-2.5 rounded-[14px] border bg-panel p-4 {editing?.id === t.id ? 'border-accent' : 'border-line'}">
        <div class="flex items-center gap-2 text-xs">
          <span class="tint-lime flex h-6 items-center rounded-md px-2 font-mono text-[11px] font-semibold tracking-wide">{t.kind === 'dynamic' ? 'SOCKS' : 'LOCAL'}</span>
          {#if t.name}<span class="truncate text-text">{t.name}</span>{/if}
          <span class="ml-auto font-mono {t.status === 'running' ? 'text-success' : t.status === 'failed' ? 'text-warning' : 'text-subtle'}">
            {busy ? 'starting…' : t.status === 'running' ? `● running${t.shared ? ' · via terminal' : ''}` : t.status === 'failed' ? 'failed' : 'idle'}
          </span>
        </div>
        <p class="font-mono text-base text-fg">{describeTunnel(t)}</p>
        <p class="text-xs text-subtle">via {host?.name ?? 'deleted host'}{t.status === 'running' ? ` · ${since(t.since)}` : ''}</p>
        {#if t.status === 'failed' && t.message}<p class="text-xs text-warning">{t.message}</p>{/if}
        <div class="flex gap-2">
          <button
            class="h-8 rounded-[9px] px-3 text-[12.5px] font-semibold disabled:opacity-50 {t.status === 'running' ? 'border border-strong text-fg hover:bg-raised' : 'bg-accent text-on-accent hover:bg-accent/90'}"
            disabled={busy}
            onclick={() => tunnels.toggle(t as TunnelState)}
          >
            {t.status === 'running' ? 'Stop' : 'Start'}
          </button>
          <button class="h-8 rounded-[9px] px-3 text-[12.5px] text-muted hover:bg-raised hover:text-fg" onclick={() => edit(t)}>Edit</button>
        </div>
      </li>
    {/each}
  </ul>
</main>

{#if editing}
  {@const t = editing}
  <aside aria-label={t.id ? 'Edit tunnel' : 'New tunnel'} class="flex w-[360px] shrink-0 flex-col gap-[18px] overflow-y-auto rounded-panel border border-line bg-panel px-6 py-6">
    <div class="flex items-center gap-3">
      <span class="tint-lime grid size-11 place-items-center rounded-[14px]"><Icon name="tunnel" size={20} /></span>
      <h2 class="flex-1 font-display text-lg font-bold">{t.id ? 'Edit tunnel' : 'New tunnel'}</h2>
      <button class="grid size-7 place-items-center rounded-lg bg-hop text-muted hover:text-fg" aria-label="Close" onclick={() => (editing = null)}>
        <Icon name="x" size={12} />
      </button>
    </div>
    <form
      class="flex flex-col gap-3"
      novalidate
      onsubmit={(e) => {
        e.preventDefault()
        void save()
      }}
    >
      <fieldset class="flex gap-2">
        <legend class={label}>Type</legend>
        {#each [['local', 'Local port'], ['dynamic', 'SOCKS proxy']] as const as [value, name] (value)}
          <label class="flex h-10 flex-1 cursor-pointer items-center justify-center rounded-[11px] border text-[13px] {t.kind === value ? 'border-accent bg-accent-soft text-accent-ink' : 'border-strong hover:bg-raised'}">
            <input type="radio" class="sr-only" name="tunnel-kind" {value} bind:group={t.kind} />{name}
          </label>
        {/each}
      </fieldset>
      <div>
        <label class={label} for="tunnel-host">Through host</label>
        <select id="tunnel-host" class={input} bind:value={t.hostId} aria-invalid={!!fieldError('hostId')}>
          {#each hostStore.list.filter((h) => h.protocol !== 'rdp') as h (h.id)}<option value={h.id}>{h.name}</option>{/each}
        </select>
        {#if fieldError('hostId')}<p class="mt-1 text-xs text-warning">{fieldError('hostId')}</p>{/if}
      </div>
      <div>
        <label class={label} for="tunnel-bind">Local port</label>
        <input id="tunnel-bind" class={input} type="number" min="1" max="65535" placeholder="5432" bind:value={t.bindPort} aria-invalid={!!fieldError('bindPort')} />
        {#if fieldError('bindPort')}<p class="mt-1 text-xs text-warning">{fieldError('bindPort')}</p>{/if}
      </div>
      {#if t.kind === 'local'}
        <div class="grid grid-cols-3 gap-2.5">
          <div class="col-span-2">
            <label class={label} for="tunnel-target">Target, as seen from the host</label>
            <input id="tunnel-target" class="{input} font-mono text-xs" placeholder="localhost" autocomplete="off" spellcheck="false" bind:value={t.targetHost} aria-invalid={!!fieldError('targetHost')} />
          </div>
          <div>
            <label class={label} for="tunnel-tport">Port</label>
            <input id="tunnel-tport" class={input} type="number" min="1" max="65535" placeholder="5432" bind:value={t.targetPort} aria-invalid={!!fieldError('targetPort')} />
          </div>
        </div>
        {#if fieldError('targetHost') || fieldError('targetPort')}<p class="-mt-1.5 text-xs text-warning">{fieldError('targetHost') ?? fieldError('targetPort')}</p>{/if}
      {/if}
      <div>
        <label class={label} for="tunnel-name">Name (optional)</label>
        <input id="tunnel-name" class={input} placeholder="Postgres on pg-primary" autocomplete="off" bind:value={t.name} />
      </div>
      {#if error && !error.field}<p role="alert" class="rounded-xl border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">{error.message}</p>{/if}
      <p class="text-[11.5px] leading-normal text-subtle">
        Uses your key or ssh agent. For password-only hosts, open a terminal to the host first; the tunnel then shares its connection.
      </p>
      <div class="mt-2 flex gap-2.5">
        {#if t.id}
          <button type="button" class="h-11 flex-1 rounded-[13px] border border-strong text-sm font-semibold text-danger-text hover:bg-raised" onclick={remove}>Delete</button>
        {/if}
        <button type="submit" disabled={saving} class="h-11 flex-[2] rounded-[13px] bg-fg text-sm font-bold text-panel hover:opacity-90 disabled:opacity-60">Save</button>
      </div>
    </form>
  </aside>
{/if}
