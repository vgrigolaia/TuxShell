<script lang="ts">
  import { onMount } from 'svelte'
  import { remoteDesktops, type Session } from '../rdp.svelte'
  import { hostStore } from '../hosts.svelte'
  import HostBadge from './HostBadge.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    onClose: () => void
  }
  let { onClose }: Props = $props()

  let box: HTMLDivElement
  let details = $state<{ id: string; lines: string[] } | null>(null)
  let now = $state(Date.now())

  onMount(() => {
    box.querySelector<HTMLElement>('button')?.focus()
    const tick = setInterval(() => (now = Date.now()), 30_000)
    const away = (e: MouseEvent) => !box.contains(e.target as Node) && onClose()
    // After the click that opened us.
    setTimeout(() => window.addEventListener('mousedown', away))
    return () => {
      clearInterval(tick)
      window.removeEventListener('mousedown', away)
    }
  })

  function since(s: Session) {
    const m = Math.round(((s.ended || now) - s.started) / 60000)
    return m < 1 ? 'under a minute' : m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${m % 60} min`
  }

  async function showDetails(s: Session) {
    details = details?.id === s.id ? null : { id: s.id, lines: await remoteDesktops.log(s.id) }
  }

  function reconnect(s: Session) {
    const host = hostStore.get(s.hostId)
    if (!host) return
    remoteDesktops.forget(s.id)
    remoteDesktops.connecting = host
    onClose()
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  bind:this={box}
  role="dialog"
  aria-label="Remote desktops"
  tabindex="-1"
  class="absolute top-full right-0 z-50 mt-2 w-[420px] rounded-2xl border border-line bg-panel p-2 text-fg shadow-2xl"
  onkeydown={(e) => e.key === 'Escape' && onClose()}
>
  <h2 class="px-2.5 pt-1.5 pb-2 text-xs font-semibold tracking-wider text-subtle uppercase">Remote desktops</h2>
  {#if !remoteDesktops.sessions.length}
    <p class="px-2.5 pb-3 text-[13px] text-muted">None open. Remote desktop hosts open in their own window; they're listed here while they run.</p>
  {/if}
  <ul class="flex flex-col gap-1">
    {#each remoteDesktops.sessions as s (s.id)}
      {@const host = hostStore.get(s.hostId)}
      <li class="rounded-xl px-2.5 py-2 hover:bg-raised/60">
        <div class="flex items-center gap-3">
          <HostBadge name={s.name} os={host?.os ?? ''} size={30} />
          <div class="min-w-0 flex-1">
            <p class="truncate text-[13px] font-semibold">{s.name}</p>
            <p class="truncate text-xs {s.error ? 'text-warning' : s.state === 'ended' ? 'text-subtle' : 'text-success'}">
              {#if s.state === 'connecting'}Connecting…{:else if s.state === 'open'}Open · {since(s)}{:else if s.error}{s.error}{:else}Closed after {since(s)}{/if}
              {#if s.tunnel && s.state !== 'ended'}<span class="text-subtle"> · {s.tunnel}</span>{/if}
            </p>
          </div>
          {#if s.state === 'ended'}
            {#if host}<button class="h-7 rounded-lg border border-strong px-2.5 text-xs font-semibold hover:bg-raised" onclick={() => reconnect(s)}>Reconnect</button>{/if}
            <button class="grid size-7 place-items-center rounded-lg text-subtle hover:bg-raised hover:text-fg" aria-label="Remove {s.name} from the list" onclick={() => remoteDesktops.forget(s.id)}>
              <Icon name="x" size={12} />
            </button>
          {:else}
            <button class="h-7 rounded-lg border border-strong px-2.5 text-xs font-semibold hover:bg-raised" onclick={() => remoteDesktops.disconnect(s.id)}>Disconnect</button>
          {/if}
        </div>
        {#if s.error || s.state === 'ended'}
          <button class="mt-1 ml-[42px] text-[11px] text-subtle underline-offset-2 hover:text-fg hover:underline" aria-expanded={details?.id === s.id} onclick={() => showDetails(s)}>
            {details?.id === s.id ? 'Hide details' : 'Show details'}
          </button>
          {#if details?.id === s.id}
            <pre class="mt-1.5 ml-[42px] max-h-40 overflow-auto rounded-lg bg-ground p-2 font-mono text-[10.5px] whitespace-pre-wrap text-muted">{details.lines.join('\n') || 'FreeRDP printed nothing.'}</pre>
          {/if}
        {/if}
      </li>
    {/each}
  </ul>
  <p class="px-2.5 pt-2 pb-1 text-[11px] text-subtle">Close a desktop's window, or log off, to end it. In fullscreen, the bar at the top has minimize and close.</p>
</div>
