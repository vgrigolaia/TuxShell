<script lang="ts">
  import { onMount } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { WebglAddon } from '@xterm/addon-webgl'
  import { SearchAddon } from '@xterm/addon-search'
  import { WebLinksAddon } from '@xterm/addon-web-links'
  import { Unicode11Addon } from '@xterm/addon-unicode11'
  import { Browser, Clipboard } from '@wailsio/runtime'
  import { claim, release } from '../terminal-events'
  import * as Terminals from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/terminal/service'
  import type { Source } from '../tabs'
  import { errorMessage, hostStore } from '../hosts.svelte'
  import LoginPrompt from './LoginPrompt.svelte'
  import { clipboardHistory } from '../clipboard.svelte'

  interface Props {
    source: Source
    active: boolean
    onStart?: () => void
    onTitle?: (title: string) => void
    onSize?: (cols: number, rows: number) => void
    onExit?: (code: number) => void
  }

  let { source, active, onStart, onTitle, onSize, onExit }: Props = $props()

  let host: HTMLDivElement
  let term: Terminal | undefined
  let fit: FitAddon | undefined
  let exited = $state<number | null>(null)
  let failed = $state<string | null>(null)
  let reconnect: () => void = () => {}

  // Username typed in the login prompt; overrides the host's saved user.
  let loginUser = $state<string | null>(null)
  // svelte-ignore state_referenced_locally
  let asking = $state(source.kind === 'host' && !!source.askUser)
  let loginError = $state<string | null>(null)
  const hostName = $derived(source.kind === 'host' ? (hostStore.get(source.hostId)?.name ?? 'host') : '')
  const savedUser = $derived(source.kind === 'host' ? (hostStore.get(source.hostId)?.user ?? '') : '')

  const FONT = '"JetBrains Mono Variable", monospace'

  function launch(cols: number, rows: number): Promise<string> {
    switch (source.kind) {
      case 'host':
        return loginUser !== null
          ? Terminals.StartHostAs(source.hostId, loginUser, cols, rows)
          : Terminals.StartHost(source.hostId, cols, rows)
      case 'target':
        return Terminals.StartTarget(source.target, cols, rows)
      default:
        return Terminals.StartShell(cols, rows)
    }
  }

  let submitLogin: (user: string, save: boolean) => void = () => {}

  /** Pastes text into the session (used by clipboard history). */
  export function paste(text: string) {
    if (asking || exited !== null) return
    term?.paste(text)
    term?.focus()
  }

  onMount(() => {
    let sessionId: string | undefined
    let disposed = false
    const cleanups: (() => void)[] = []

    const start = async () => {
      // Measure cell size with the real font, not the fallback.
      await document.fonts.load(`13px ${FONT}`)
      if (disposed) return

      term = new Terminal({
        allowProposedApi: true,
        cursorBlink: true,
        fontFamily: FONT,
        fontSize: 13,
        lineHeight: 1.25,
        scrollback: 10000,
        theme: {
          background: '#111318',
          foreground: '#e6e7eb',
          cursor: '#8b7cff',
          cursorAccent: '#111318',
          selectionBackground: '#8b7cff55',
          black: '#1e212a',
          brightBlack: '#5c6170',
          green: '#a3e635',
          brightGreen: '#bef264',
          yellow: '#fb923c',
          brightYellow: '#fdba74',
          blue: '#7aa2ff',
          brightBlue: '#a5c0ff',
          magenta: '#8b7cff',
          brightMagenta: '#b1a7ff',
        },
      })
      fit = new FitAddon()
      term.loadAddon(fit)
      term.loadAddon(new SearchAddon())
      term.loadAddon(new WebLinksAddon((_e, uri) => void Browser.OpenURL(uri)))
      term.loadAddon(new Unicode11Addon())
      term.unicode.activeVersion = '11'
      // Linux terminal conventions; xterm.js leaves copy/paste keys to the embedder.
      term.attachCustomKeyEventHandler((e) => {
        if (e.type !== 'keydown') return true
        const key = e.key.toLowerCase()
        const copy = (e.ctrlKey && e.shiftKey && key === 'c') || (e.ctrlKey && !e.shiftKey && key === 'insert')
        const paste = (e.ctrlKey && e.shiftKey && key === 'v') || (e.shiftKey && !e.ctrlKey && key === 'insert')
        if (copy) {
          e.preventDefault()
          const text = term?.getSelection()
          if (text) {
            void Clipboard.SetText(text)
            clipboardHistory.record(text)
          }
          return false
        }
        if (paste) {
          e.preventDefault()
          void Clipboard.Text().then((text) => text && term?.paste(text))
          return false
        }
        return true
      })
      term.open(host)
      try {
        const webgl = new WebglAddon()
        webgl.onContextLoss(() => webgl.dispose())
        term.loadAddon(webgl)
      } catch {
        // No WebGL: xterm falls back to its DOM renderer.
      }
      fit.fit()

      const input = term.onData((data) => {
        if (sessionId) void Terminals.Write(sessionId, data)
      })
      const resize = term.onResize(({ cols, rows }) => {
        onSize?.(cols, rows)
        if (sessionId) void Terminals.Resize(sessionId, cols, rows)
      })
      const title = term.onTitleChange((t) => onTitle?.(t))
      cleanups.push(() => {
        input.dispose()
        resize.dispose()
        title.dispose()
      })

      let frame = 0
      const observer = new ResizeObserver(() => {
        cancelAnimationFrame(frame)
        // Hidden tabs have zero size; fitting them would shrink the PTY to 1x1.
        frame = requestAnimationFrame(() => host.offsetWidth > 0 && fit?.fit())
      })
      observer.observe(host)
      cleanups.push(() => observer.disconnect())

      onSize?.(term.cols, term.rows)
      reconnect = () => void connect()
      submitLogin = async (user: string, save: boolean) => {
        loginError = null
        if (save && source.kind === 'host') {
          try {
            await hostStore.setUser(source.hostId, user)
          } catch (e) {
            loginError = (e as { cause?: { message?: string } })?.cause?.message ?? errorMessage(e)
            return
          }
        }
        loginUser = user
        asking = false
        await connect()
      }
      if (!asking) await connect()
    }

    const connect = async () => {
      if (!term) return
      if (exited !== null) term.write('\r\n\x1b[90m── reconnecting ──\x1b[0m\r\n')
      exited = null
      failed = null
      try {
        sessionId = await launch(term.cols, term.rows)
      } catch (err) {
        // A bad typed username goes back to the prompt rather than the error bar.
        const cause = (err as { cause?: { field?: string; message?: string } })?.cause
        if (loginUser !== null && cause?.field === 'user') {
          loginError = cause.message ?? 'Invalid username'
          asking = true
          return
        }
        failed = (err as Error)?.message ?? String(err)
        exited = -1
        onExit?.(-1)
        return
      }
      if (disposed) {
        release(sessionId)
        void Terminals.Close(sessionId)
        return
      }
      onStart?.()
      const id = sessionId
      claim(id, {
        data: (bytes) => term?.write(bytes),
        exit: (code) => {
          if (sessionId === id) sessionId = undefined
          exited = code
          onExit?.(code)
        },
      })
      if (active) term.focus()
    }
    void start()

    return () => {
      disposed = true
      if (sessionId) {
        release(sessionId)
        void Terminals.Close(sessionId)
      }
      cleanups.forEach((fn) => fn())
      term?.dispose()
    }
  })

  $effect(() => {
    if (active && term && !asking) {
      requestAnimationFrame(() => {
        fit?.fit()
        term?.focus()
      })
    }
  })
</script>

<div class="relative h-full w-full" class:hidden={!active}>
  <!-- isolate keeps xterm's own z-indexed layers (e.g. the WebGL link canvas)
       inside this box, so the overlays below can sit on top and get clicks. -->
  <div bind:this={host} class="isolate h-full w-full select-text"></div>
  {#if asking && source.kind === 'host'}
    <LoginPrompt
      hostId={source.hostId}
      {hostName}
      initialUser={loginUser ?? savedUser}
      error={loginError}
      onSubmit={(user, save) => submitLogin(user, save)}
    />
  {:else if exited !== null}
    <div
      role="status"
      class="absolute right-4 bottom-3 z-10 flex max-w-[80%] items-center gap-3 rounded-lg border border-line bg-raised py-1.5 pr-1.5 pl-3 text-xs"
    >
      <span class="min-w-0 truncate font-mono {failed ? 'text-warning' : 'text-muted'}">
        {failed ?? `Session ended · exit code ${exited}`}
      </span>
      <button class="shrink-0 rounded-md bg-accent px-2.5 py-1 font-medium text-ground hover:bg-accent/90" onclick={() => reconnect()}>
        {source.kind === 'shell' ? 'Restart' : 'Reconnect'}
      </button>
    </div>
  {/if}
</div>
