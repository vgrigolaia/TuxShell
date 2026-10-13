<script lang="ts">
  import { onMount } from 'svelte'
  import { Terminal, type FontWeight } from '@xterm/xterm'
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
  import { theme } from '../theme.svelte'
  import { prefs } from '../settings.svelte'
  import { schemeTheme } from '../schemes'

  interface Props {
    source: Source
    /** The pane's tab is showing. */
    visible: boolean
    /** This pane gets keyboard focus. */
    focused: boolean
    onStart?: () => void
    /** Claims typed input (e.g. to broadcast it); return true when handled. */
    onInput?: (data: string) => boolean
    /** Reports the username typed in the login prompt. */
    onLogin?: (user: string) => void
    onTitle?: (title: string) => void
    onSize?: (cols: number, rows: number) => void
    onExit?: (code: number) => void
  }

  let { source, visible, focused, onStart, onInput, onLogin, onTitle, onSize, onExit }: Props = $props()

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

  let bell = $state(false)

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

  // Set once the session machinery exists; writes to this pane's session only.
  let sendToSession: (data: string) => void = () => {}

  /** Writes input straight to this pane's session (used by broadcast). */
  export function send(data: string) {
    sendToSession(data)
  }

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
      await loadFont(prefs.fontFamily)
      if (disposed) return

      term = new Terminal({ allowProposedApi: true, ...termOptions() })
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

      sendToSession = (data) => {
        if (sessionId) void Terminals.Write(sessionId, data)
      }
      const input = term.onData((data) => {
        if (onInput?.(data)) return
        sendToSession(data)
      })
      const resize = term.onResize(({ cols, rows }) => {
        onSize?.(cols, rows)
        if (sessionId) void Terminals.Resize(sessionId, cols, rows)
      })
      const title = term.onTitleChange((t) => onTitle?.(t))
      let bellTimer: ReturnType<typeof setTimeout> | undefined
      const ring = term.onBell(() => {
        if (prefs.value.terminal.bell !== 'visual') return
        bell = true
        clearTimeout(bellTimer)
        bellTimer = setTimeout(() => (bell = false), 150)
      })
      // Copy on select: copy when the mouse selection is finished, not on every change.
      const onMouseUp = () => {
        const text = prefs.value.terminal.copyOnSelect && term?.hasSelection() ? term.getSelection() : ''
        if (text) {
          void Clipboard.SetText(text)
          clipboardHistory.record(text)
        }
      }
      const onContextMenu = (e: MouseEvent) => {
        if (!prefs.value.terminal.rightClickPaste) return
        e.preventDefault()
        void Clipboard.Text().then((text) => text && term?.paste(text))
      }
      host.addEventListener('mouseup', onMouseUp)
      host.addEventListener('contextmenu', onContextMenu)
      cleanups.push(() => {
        input.dispose()
        resize.dispose()
        title.dispose()
        ring.dispose()
        clearTimeout(bellTimer)
        host.removeEventListener('mouseup', onMouseUp)
        host.removeEventListener('contextmenu', onContextMenu)
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
        onLogin?.(user)
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
      if (focused) term.focus()
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

  function termOptions() {
    const t = prefs.value.terminal
    return {
      fontFamily: prefs.fontFamily,
      fontSize: prefs.fontSize,
      lineHeight: t.lineHeight,
      letterSpacing: t.letterSpacing,
      fontWeight: t.fontWeight as FontWeight,
      fontWeightBold: t.fontWeightBold as FontWeight,
      cursorStyle: t.cursorStyle as 'block' | 'bar' | 'underline',
      cursorBlink: t.cursorBlink,
      scrollback: t.scrollback,
      theme: schemeTheme(t.colorScheme, theme.resolved),
    }
  }

  // Waits for a font so xterm measures cells with it rather than a fallback.
  async function loadFont(family: string) {
    try {
      await document.fonts.load(`${prefs.fontSize}px ${family}`)
    } catch {
      // Unknown font: xterm uses the fallback in the list.
    }
  }

  // Apply preference changes to the live terminal.
  $effect(() => {
    const opts = termOptions()
    if (!term) return
    void loadFont(opts.fontFamily).then(() => {
      if (!term) return
      Object.assign(term.options, opts)
      // Cell size may have changed; refit so the PTY gets the new size.
      if (host.offsetWidth > 0) fit?.fit()
    })
  })

  $effect(() => {
    if (visible && focused && term && !asking) {
      requestAnimationFrame(() => {
        fit?.fit()
        term?.focus()
      })
    }
  })
</script>

<div class="relative h-full w-full">
  <!-- isolate keeps xterm's own z-indexed layers (e.g. the WebGL link canvas)
       inside this box, so the overlays below can sit on top and get clicks. -->
  <div bind:this={host} class="isolate h-full w-full select-text"></div>
  {#if bell}
    <div class="pointer-events-none absolute inset-0 z-10 rounded-md bg-fg/10" aria-hidden="true"></div>
  {/if}
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
