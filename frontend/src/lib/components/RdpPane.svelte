<script lang="ts" module>
  import { installClipboard } from '../rdpClipboard'

  type RdpBackend = typeof import('@devolutions/iron-remote-desktop-rdp')

  // The RDP engine (7 MB of WebAssembly) loads on the first remote desktop
  // tab, not when TuxShell starts.
  let ready: Promise<RdpBackend> | undefined
  function engine(): Promise<RdpBackend> {
    ready ??= (async () => {
      installClipboard()
      const [, rdp] = await Promise.all([import('@devolutions/iron-remote-desktop'), import('@devolutions/iron-remote-desktop-rdp')])
      await rdp.init('WARN')
      return rdp
    })()
    return ready
  }
</script>

<script lang="ts">
  import { onMount } from 'svelte'
  import type { UserInteraction } from '@devolutions/iron-remote-desktop'
  import { remoteDesktops, type TabSession } from '../rdp.svelte'
  import { Clipboard } from '@wailsio/runtime'
  import { hostStore } from '../hosts.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    paneId: number
    hostId: number
    visible: boolean
    focused: boolean
    onStart?: () => void
    onExit?: (code: number) => void
  }
  let { paneId, hostId, visible, focused, onStart, onExit }: Props = $props()

  const host = $derived(hostStore.get(hostId))
  let rdp = $state<RdpBackend | null>(null)
  let box: HTMLDivElement
  let ui: UserInteraction | undefined
  let phase = $state<'password' | 'connecting' | 'open' | 'ended'>('password')
  let error = $state('')
  let password = $state('')
  let show = $state(false)
  let passwordInput = $state<HTMLInputElement>()
  let started = 0
  // Measured for the first screen: how long until the desktop showed.
  let firstFrameMs = $state(0)
  const clipboard = $derived(host?.rdp.clipboard ?? true)

  onMount(() => {
    // The connect window may have handed us a ticket and the typed password.
    const handoff = remoteDesktops.takeTab(hostId)
    if (handoff) {
      password = handoff.password
      void connect(handoff.session)
    } else {
      passwordInput?.focus()
    }
    return () => ui?.shutdown()
  })

  function onReady(e: Event) {
    ui = (e as CustomEvent<{ irgUserInteraction: UserInteraction }>).detail.irgUserInteraction
    readyResolve?.()
  }
  let readyResolve: (() => void) | undefined
  const componentReady = new Promise<void>((r) => (readyResolve = r))

  async function connect(session?: TabSession) {
    error = ''
    phase = 'connecting'
    lastClip = ''
    started = performance.now()
    try {
      if (!session) {
        if (!host) throw new Error('This host was deleted')
        const res = await remoteDesktops.openTab(host)
        if ('cert' in res) {
          // The certificate needs a decision: the connect window asks.
          remoteDesktops.connecting = host
          phase = 'password'
          return
        }
        session = res
      }
      rdp = await engine()
      await componentReady
      if (!ui) throw new Error('The remote desktop engine didn\'t start')
      const rect = box.getBoundingClientRect()
      const config = ui
        .configBuilder()
        .withUsername(session.user)
        .withPassword(password)
        .withDestination(session.destination)
        .withProxyAddress(session.proxy)
        .withServerDomain(session.domain)
        .withAuthToken(session.token)
        .withDesktopSize({ width: Math.max(Math.round(rect.width), 640), height: Math.max(Math.round(rect.height), 480) })
        .withExtension(rdp.displayControl(true))
        .build()
      ui.setEnableClipboard(clipboard)
      ui.setEnableAutoClipboard(false)
      // Copied on the remote desktop: put it on this computer's clipboard.
      ui.onClipboardRemoteUpdateCallback(() => {
        void ui
          ?.saveRemoteClipboardData()
          .then(() => Clipboard.Text())
          .then((t) => (lastClip = t ?? ''))
          .catch(() => {})
      })
      const info = await ui.connect(config)
      password = ''
      phase = 'open'
      firstFrameMs = Math.round(performance.now() - started)
      ui.setVisibility(true)
      void sendClipboard()
      onStart?.()
      const end = await info.run()
      phase = 'ended'
      error = end.reason() === 'Session closed' ? '' : end.reason()
      onExit?.(0)
    } catch (e) {
      password = ''
      phase = 'ended'
      error = describe(e)
      onExit?.(1)
    }
  }

  // This computer's clipboard goes to the remote desktop whenever the tab
  // gets focus (the server asks for it when something is pasted there).
  let lastClip = ''
  async function sendClipboard() {
    if (!clipboard || phase !== 'open' || !ui) return
    const text = (await Clipboard.Text().catch(() => '')) ?? ''
    if (!text || text === lastClip) return
    lastClip = text
    await ui.sendClipboardData().catch(() => {})
  }

  // IronRDP's errors carry a kind; turn it into words.
  function describe(e: unknown): string {
    const err = e as { kind?: () => number; backtrace?: () => string; message?: string }
    const kinds: Record<number, string> = {
      1: 'Wrong user name or password.',
      2: 'The server refused the login.',
      3: "This user isn't allowed to use Remote Desktop.",
      4: "Couldn't reach the server through TuxShell's relay (is the host online?).",
      5: "Couldn't reach the server.",
      6: "The server and TuxShell couldn't agree on how to secure the connection.",
    }
    if (typeof err?.kind === 'function') return kinds[err.kind()] ?? err.backtrace?.() ?? 'The connection failed.'
    return err?.message ?? String(e)
  }

  // Follow the tab's size, a moment after the last change.
  let resizeTimer: ReturnType<typeof setTimeout> | undefined
  $effect(() => {
    if (!box) return
    const ro = new ResizeObserver(() => {
      clearTimeout(resizeTimer)
      resizeTimer = setTimeout(() => {
        const r = box.getBoundingClientRect()
        if (phase === 'open' && r.width > 0 && r.height > 0) ui?.resize(Math.round(r.width), Math.round(r.height))
      }, 300)
    })
    ro.observe(box)
    return () => ro.disconnect()
  })

  $effect(() => {
    if (visible && focused && phase === 'open') {
      ;(box.querySelector('iron-remote-desktop') as HTMLElement | null)?.focus()
      void sendClipboard()
    }
  })

  // Coming back to TuxShell from another app, maybe with something new copied.
  $effect(() => {
    const back = () => visible && focused && void sendClipboard()
    window.addEventListener('focus', back)
    return () => window.removeEventListener('focus', back)
  })
</script>

<div class="relative flex h-full w-full flex-col">
  <!-- Its own row, so it never covers the remote desktop's own buttons. -->
  <div class="flex h-8 shrink-0 items-center justify-end gap-1 border-b border-line px-2">
    {#if phase === 'open'}
      {#if firstFrameMs}<span class="mr-auto px-1 font-mono text-[10px] text-subtle" title="Time until the desktop showed">{firstFrameMs} ms</span>{/if}
      <button class="rounded-md px-2 py-1 text-[11px] font-medium text-muted hover:bg-raised hover:text-fg" title="Send Ctrl+Alt+Del" onclick={() => ui?.ctrlAltDel()}>Ctrl+Alt+Del</button>
      <button class="rounded-md px-2 py-1 text-[11px] font-medium text-muted hover:bg-raised hover:text-fg" title="Disconnect" onclick={() => ui?.shutdown()}>Disconnect</button>
    {/if}
  </div>
  <div bind:this={box} class="min-h-0 w-full flex-1" class:invisible={phase !== 'open'} onfocusin={() => void sendClipboard()}>
    {#if rdp}<iron-remote-desktop scale="fit" flexcenter="true" module={rdp.Backend} onready={onReady}></iron-remote-desktop>{/if}
  </div>

  {#if phase !== 'open'}
    <div class="absolute inset-0 top-8 grid place-items-center">
      <form
        class="w-80 rounded-2xl border border-line bg-panel p-5 shadow-xl"
        onsubmit={(e) => {
          e.preventDefault()
          void connect()
        }}
      >
        <p class="flex items-center gap-2 text-[13px] font-semibold"><Icon name="monitor" size={14} />{host?.name ?? 'Remote desktop'} <span class="rounded bg-hop px-1.5 text-[10px] text-muted">beta</span></p>
        {#if phase === 'connecting'}
          <p class="mt-3 text-[13px] text-muted">Connecting…</p>
        {:else}
          {#if error}<p role="alert" class="mt-3 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">{error}</p>{/if}
          {#if phase === 'ended' && !error}<p class="mt-3 text-[13px] text-muted">The session ended.</p>{/if}
          <label class="mt-3 mb-1.5 block text-xs font-semibold text-text" for="rdp-pw-{paneId}">Password{host?.user ? ` for ${host.user}` : ''}</label>
          <div class="flex items-center gap-1 rounded-[11px] border border-strong bg-field pr-1 focus-within:border-accent">
            <input bind:this={passwordInput} id="rdp-pw-{paneId}" class="h-10 min-w-0 flex-1 bg-transparent px-3 text-[13.5px] focus:outline-none" type={show ? 'text' : 'password'} autocomplete="off" bind:value={password} />
            <button type="button" class="grid size-8 place-items-center rounded-lg text-muted hover:text-fg" aria-label={show ? 'Hide password' : 'Show password'} onclick={() => (show = !show)}>
              <Icon name={show ? 'eyeOff' : 'eye'} size={15} />
            </button>
          </div>
          <p class="mt-2 text-[11px] text-subtle">In a tab the password is typed each time; it goes from here straight to the server.</p>
          <button class="mt-3 h-9 w-full rounded-lg bg-accent text-[13px] font-semibold text-on-accent hover:bg-accent/90">{phase === 'ended' ? 'Reconnect' : 'Connect'}</button>
        {/if}
      </form>
    </div>
  {/if}
</div>
