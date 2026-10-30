<script lang="ts">
  import { isMac } from '../platform'
  import { keybindings } from '../keybindings.svelte'
  import { onMount } from 'svelte'
  import { Terminal, type FontWeight } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { WebglAddon } from '@xterm/addon-webgl'
  import { SearchAddon } from '@xterm/addon-search'
  import { WebLinksAddon } from '@xterm/addon-web-links'
  import { Unicode11Addon } from '@xterm/addon-unicode11'
  import { Browser, Clipboard, Events } from '@wailsio/runtime'
  import { claim, release } from '../terminal-events'
  import { isTerminalReport } from '../broadcast'
  import * as Terminals from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/terminal/service'
  import type { PaneActions, Source } from '../tabs'
  import { errorMessage, hostStore } from '../hosts.svelte'
  import LoginPrompt from './LoginPrompt.svelte'
  import PasswordPrompt from './PasswordPrompt.svelte'
  import { logins } from '../logins.svelte'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import { clipboardHistory } from '../clipboard.svelte'
  import { theme } from '../theme.svelte'
  import { prefs } from '../settings.svelte'
  import { profileTheme } from '../schemes'
  import { beep, paneProfile } from '../profiles.svelte'

  interface Props {
    source: Source
    /** The pane's tab is showing. */
    visible: boolean
    /** This pane gets keyboard focus. */
    focused: boolean
    /** The session started; id is its terminal session. */
    onStart?: (id: string) => void
    /** Claims typed input (e.g. to broadcast it); return true when handled. */
    onInput?: (data: string) => boolean
    /** Reports the username typed in the login prompt. */
    onLogin?: (user: string) => void
    onTitle?: (title: string) => void
    onSize?: (cols: number, rows: number) => void
    onExit?: (code: number) => void
    /** The session ended the way a user ends it (exit, logout), not with an error. */
    onEnded?: () => void
    /** The shell reported its folder (OSC 7, or a "user@host:path" title). */
    onCwd?: (path: string) => void
    /** The bell rang and the profile marks it with an icon. */
    onBell?: () => void
    /** Typing and pasting are blocked. */
    readOnly?: boolean
    showScrollbar?: boolean
    /** Pane is in a split (enables maximize). */
    split?: boolean
    maximized?: boolean
    /** Right-click menu actions; splits reuse this pane's connection. */
    actions?: PaneActions
  }

  let { source, visible, focused, onStart, onInput, onLogin, onTitle, onSize, onExit, onEnded, onBell, onCwd, readOnly = false, showScrollbar = true, split = false, maximized = false, actions }: Props = $props()

  let host: HTMLDivElement
  let term: Terminal | undefined
  let fit: FitAddon | undefined
  let exited = $state<number | null>(null)
  let failed = $state<string | null>(null)
  let reconnect: () => void = () => {}

  // Username typed in the login prompt (or carried over from the pane this
  // one was split from); overrides the host's saved user.
  // svelte-ignore state_referenced_locally
  let loginUser = $state<string | null>(source.kind === 'host' && source.user ? source.user : null)
  // svelte-ignore state_referenced_locally
  let asking = $state(source.kind === 'host' && !!source.askUser)
  let loginError = $state<string | null>(null)
  const hostName = $derived(source.kind === 'host' ? (hostStore.get(source.hostId)?.name ?? 'host') : '')
  // Shown on clipboard entries copied here ("This host" filter).
  const clipSource = $derived(source.kind === 'host' ? hostName : source.kind === 'target' ? source.target : 'local shell')
  const savedUser = $derived(source.kind === 'host' ? (hostStore.get(source.hostId)?.user ?? '') : '')

  // The host's terminal profile (fonts, colours, scrolling, bell, keys).
  const profile = $derived(paneProfile(source))

  let bell = $state(false)
  // The running session's recording (the file name), and why one stopped by itself.
  let recording = $state('')
  let recordError = $state('')
  let menu = $state<{ x: number; y: number } | null>(null)

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

  // Handles typed input (read-only, broadcast, then the session).
  let typed: (data: string) => void = () => {}

  // Set once the session machinery exists; writes to this pane's session only.
  let sendToSession: (data: string) => void = () => {}

  /** Writes input straight to this pane's session (used by broadcast). */
  export function send(data: string) {
    sendToSession(data)
  }

  /** Pastes text into the session (used by clipboard history). */
  export function paste(text: string) {
    if (asking || exited !== null || readOnly) return
    term?.paste(text)
    term?.focus()
  }

  // The running session, for password boxes ssh asks for.
  let liveSession = $state<string | undefined>()

  onMount(() =>
    Events.On('terminal:recording', (ev) => {
      const r = ev.data as { id: string; recording: boolean; name: string; error: string }
      if (r.id !== liveSession) return
      recording = r.recording ? r.name : ''
      recordError = r.error
    }),
  )

  async function startRecording() {
    if (!liveSession) return
    recordError = ''
    try {
      recording = await Terminals.StartRecording(liveSession, clipSource)
    } catch (e) {
      recordError = errorMessage(e)
    }
  }

  function stopRecording() {
    if (liveSession) void Terminals.StopRecording(liveSession)
    recording = ''
  }
  const passwordRequest = $derived(logins.forSession(liveSession))

  onMount(() => {
    let sessionId: string | undefined
    let disposed = false
    const cleanups: (() => void)[] = []

    const start = async () => {
      // Measure cell size with the real font, not the fallback.
      await loadFont(prefs.fontFamilyOf(profile))
      if (disposed) return

      const opts = termOptions()
      applied = { ...opts }
      term = new Terminal({ allowProposedApi: true, ...opts })
      fit = new FitAddon()
      term.loadAddon(fit)
      term.loadAddon(new SearchAddon())
      term.loadAddon(new WebLinksAddon((_e, uri) => void Browser.OpenURL(uri)))
      term.loadAddon(new Unicode11Addon())
      term.unicode.activeVersion = '11'
      // Linux terminal conventions; xterm.js leaves copy/paste keys to the embedder.
      term.attachCustomKeyEventHandler((e) => {
        if (e.type !== 'keydown') return true
        // What Backspace and Delete send, for servers that expect other codes.
        const plain = !e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey
        const keyCode =
          plain && e.key === 'Backspace' && profile.backspace !== 'del'
            ? eraseCode(profile.backspace)
            : plain && e.key === 'Delete' && profile.delete !== 'escape'
              ? eraseCode(profile.delete)
              : undefined
        if (keyCode !== undefined) {
          e.preventDefault()
          typed(keyCode)
          return false
        }
        // Copy and paste keys come from Preferences → Shortcuts (by default
        // Ctrl+Shift+C/V, so Ctrl+C still interrupts; ⌘C/⌘V on macOS).
        const action = keybindings.match(e)
        if (action === 'copy') {
          e.preventDefault()
          copySelection()
          return false
        }
        if (action === 'paste') {
          e.preventDefault()
          pasteClipboard()
          return false
        }
        return true
      })
      term.open(host)
      opened = true
      fit.fit()

      sendToSession = (data) => {
        if (sessionId) void Terminals.Write(sessionId, data)
      }
      typed = (data) => {
        // Read-only drops keystrokes; terminal reports (e.g. cursor position) still go through.
        if (readOnly && !isTerminalReport(data)) return
        if (onInput?.(data)) return
        sendToSession(data)
      }
      const input = term.onData((data) => typed(data))
      const resize = term.onResize(({ cols, rows }) => {
        onSize?.(cols, rows)
        if (sessionId) void Terminals.Resize(sessionId, cols, rows)
      })
      // The prompt's folder, for the file panel's "follow terminal". A shell
      // that sends OSC 7 is trusted over titles, which programs also set.
      let osc7 = false
      const cwdReport = term.parser.registerOscHandler(7, (data) => {
        const p = cwdFromOsc7(data)
        if (p) {
          osc7 = true
          onCwd?.(p)
        }
        return true
      })
      const title = term.onTitleChange((t) => {
        onTitle?.(t)
        const p = osc7 ? '' : cwdFromTitle(t)
        if (p) onCwd?.(p)
      })
      let bellTimer: ReturnType<typeof setTimeout> | undefined
      const ring = term.onBell(() => {
        if (profile.bellSound) beep()
        if (profile.bellIcon) onBell?.()
        if (profile.bell !== 'visual') return
        bell = true
        clearTimeout(bellTimer)
        bellTimer = setTimeout(() => (bell = false), 150)
      })
      // Copy on select: copy when the mouse selection is finished, not on every change.
      const onMouseUp = () => {
        const text = prefs.value.terminal.copyOnSelect && term?.hasSelection() ? term.getSelection() : ''
        if (text) {
          void Clipboard.SetText(text)
          clipboardHistory.record(text, clipSource)
        }
      }
      // Our own menu replaces WebKit's (Cut/Inspect Element) over the terminal.
      // With right-click paste on, Shift+right-click still opens the menu.
      const onContextMenu = (e: MouseEvent) => {
        e.preventDefault()
        if (prefs.value.terminal.rightClickPaste && !e.shiftKey) pasteClipboard()
        else menu = { x: e.clientX, y: e.clientY }
      }
      // Ctrl+wheel zooms; trackpads send many small deltas, so add them up.
      let wheel = 0
      const onWheel = (e: WheelEvent) => {
        if (!(isMac ? e.metaKey : e.ctrlKey) || !prefs.value.terminal.ctrlWheelZoom) return
        e.preventDefault()
        e.stopPropagation()
        wheel += e.deltaMode === WheelEvent.DOM_DELTA_PIXEL ? e.deltaY : e.deltaY * 40
        if (Math.abs(wheel) < 40) return
        prefs.zoomBy(wheel < 0 ? 1 : -1)
        wheel = 0
      }
      host.addEventListener('mouseup', onMouseUp)
      host.addEventListener('contextmenu', onContextMenu)
      // Capture, so xterm doesn't scroll the buffer as well.
      host.addEventListener('wheel', onWheel, { capture: true, passive: false })
      cleanups.push(() => {
        input.dispose()
        resize.dispose()
        title.dispose()
        cwdReport.dispose()
        ring.dispose()
        clearTimeout(bellTimer)
        host.removeEventListener('mouseup', onMouseUp)
        host.removeEventListener('contextmenu', onContextMenu)
        host.removeEventListener('wheel', onWheel, { capture: true })
      })

      // Fit once the size settles: a maximize or panel drag resizes every frame,
      // and each fit reflows the scrollback and makes the remote side redraw.
      let settle: ReturnType<typeof setTimeout> | undefined
      const observer = new ResizeObserver(() => {
        clearTimeout(settle)
        // Hidden tabs have zero size; fitting them would shrink the PTY to 1x1.
        settle = setTimeout(() => host.offsetWidth > 0 && fit?.fit(), 50)
      })
      observer.observe(host)
      cleanups.push(() => {
        observer.disconnect()
        clearTimeout(settle)
      })

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

    // ssh exits 255 for its own errors (refused, lost connection, failed login);
    // other codes come from the remote shell. A run-once command keeps its
    // output on screen.
    const endedByUser = (code: number) =>
      code >= 0 && (source.kind === 'shell' || code !== 255 || serverGone()) && !(source.kind !== 'shell' && profile.command && !profile.keepShell)

    // ssh prints "Shared connection to X closed." (or "Connection to X closed
    // [by remote host].") only after a login worked, so a 255 with it means the
    // server shut down, rebooted or dropped; refused or failed logins don't print it.
    function serverGone(): boolean {
      const buf = term?.buffer.active
      if (!buf) return false
      const end = buf.baseY + buf.cursorY
      for (let y = end; y >= Math.max(0, end - 3); y--) {
        if (/\bConnection to \S+ closed\b/i.test(buf.getLine(y)?.translateToString(true) ?? '')) return true
      }
      return false
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
      const id = sessionId
      onStart?.(id)
      liveSession = id
      recording = ''
      if (profile.record) void startRecording()
      claim(id, {
        data: (bytes) => {
          term?.write(bytes)
          if (profile.scrollOnOutput) term?.scrollToBottom()
        },
        exit: (code) => {
          if (sessionId === id) sessionId = undefined
          if (liveSession === id) liveSession = undefined
          recording = ''
          exited = code
          onExit?.(code)
          // Wait for xterm to parse the last output, which serverGone() reads.
          term?.write('', () => endedByUser(code) && onEnded?.())
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

  function copySelection() {
    const text = term?.getSelection()
    if (text) {
      void Clipboard.SetText(text)
      clipboardHistory.record(text, clipSource)
    }
  }

  function pasteClipboard() {
    if (asking || exited !== null || readOnly) return
    void Clipboard.Text().then((text) => text && term?.paste(text))
  }

  function menuItems(): MenuItem[] {
    const live = !asking && exited === null
    const a = actions
    const items: MenuItem[] = [
      { label: 'Copy', icon: 'copy', shortcut: keybindings.combo('copy'), disabled: !term?.hasSelection(), run: copySelection },
      { label: 'Paste', icon: 'clipboard', shortcut: keybindings.combo('paste'), disabled: !live || readOnly, run: pasteClipboard },
      { label: 'Select all', icon: 'selectAll', run: () => term?.selectAll() },
      { label: 'Clear scrollback', icon: 'eraser', run: () => term?.clear() },
      recording
        ? { label: 'Stop recording', icon: 'record', run: stopRecording }
        : { label: 'Start recording', icon: 'record', disabled: !live || !liveSession, run: () => void startRecording() },
    ]
    if (!a) return items
    items.push({ label: 'Set title…', icon: 'tag', shortcut: keybindings.combo('rename'), separator: true, run: a.rename })
    if (a.split) {
      const sp = a.split
      items.push(
        { label: 'Split auto', icon: 'splitAuto', shortcut: keybindings.combo('splitAuto'), run: () => sp('auto') },
        { label: 'Split right', icon: 'splitRight', run: () => sp('row') },
        { label: 'Split down', icon: 'splitDown', shortcut: keybindings.combo('splitDown'), run: () => sp('column') },
      )
    }
    if (a.splitOther) items.push({ label: 'Split with another host…', icon: 'server', run: a.splitOther })
    items.push(
      { label: 'Open in new tab', icon: 'tab', shortcut: keybindings.combo('newTab'), run: a.newTab },
      { label: 'Close', icon: 'x', shortcut: keybindings.combo('closePane'), separator: true, run: a.close },
    )
    if (split) items.push({ label: maximized ? 'Restore split' : 'Maximize pane', icon: maximized ? 'restore' : 'maximize', shortcut: keybindings.combo('maximize'), run: a.toggleMaximized })
    items.push(
      { label: 'Read only', checked: readOnly, separator: !split, run: a.toggleReadOnly },
      { label: 'Show scrollbar', shortcut: keybindings.combo('scrollbar'), checked: showScrollbar, run: a.toggleScrollbar },
      { label: 'Zoom in', icon: 'zoomIn', shortcut: keybindings.combo('zoomIn'), separator: true, run: () => prefs.zoomBy(1) },
      { label: 'Zoom out', icon: 'zoomOut', shortcut: keybindings.combo('zoomOut'), run: () => prefs.zoomBy(-1) },
      { label: 'Reset zoom', icon: 'zoomReset', shortcut: keybindings.combo('zoomReset'), disabled: prefs.zoom === 0, run: () => (prefs.zoom = 0) },
      { label: 'Preferences…', icon: 'gear', shortcut: keybindings.combo('preferences'), separator: true, run: a.preferences },
    )
    return items
  }

  /** "file://host/path" (percent-encoded) to "/path". */
  function cwdFromOsc7(data: string): string {
    try {
      const u = new URL(data)
      return u.protocol === 'file:' ? decodeURIComponent(u.pathname) : ''
    } catch {
      return ''
    }
  }

  /** The folder in titles like "user@host: ~/app" or "user@host:/etc" (bash's defaults). */
  function cwdFromTitle(t: string): string {
    const m = /^[^\s@]+@[^\s:]+: ?([~/].*)$/.exec(t.trim())
    return m ? m[1] : ''
  }

  function eraseCode(v: string): string {
    return v === 'ctrl-h' ? '\x08' : v === 'del' ? '\x7f' : '\x1b[3~'
  }

  function termOptions() {
    const t = profile
    const palette = profileTheme(t, theme.resolved)
    // With a background image the terminal is see-through; the pane paints
    // the image and the shade behind it.
    const image = !!t.background
    return {
      fontFamily: prefs.fontFamilyOf(t),
      fontSize: prefs.fontSizeOf(t),
      lineHeight: t.lineHeight,
      letterSpacing: t.letterSpacing,
      fontWeight: t.fontWeight as FontWeight,
      fontWeightBold: t.fontWeightBold as FontWeight,
      cursorStyle: t.cursorStyle as 'block' | 'bar' | 'underline',
      cursorBlink: t.cursorBlink,
      scrollback: t.scrollback,
      scrollSensitivity: t.scrollSpeed,
      scrollOnUserInput: t.scrollOnKeypress,
      allowTransparency: image,
      theme: image ? { ...palette, background: '#00000000' } : palette,
    }
  }

  // Waits for a font so xterm measures cells with it rather than a fallback.
  async function loadFont(family: string) {
    try {
      await document.fonts.load(`${prefs.fontSizeOf(profile)}px ${family}`)
    } catch {
      // Unknown font: xterm uses the fallback in the list.
    }
  }

  // The GPU renderer, except behind a background image: WebKitGTK's WebGL
  // draws no text on a see-through terminal, so xterm's DOM renderer is used.
  let opened = $state(false)
  let webgl: WebglAddon | undefined
  $effect(() => {
    const wanted = opened && !profile.background
    if (!term || wanted === !!webgl) return
    if (wanted) {
      try {
        const addon = new WebglAddon()
        addon.onContextLoss(() => {
          addon.dispose()
          if (webgl === addon) webgl = undefined
        })
        term.loadAddon(addon)
        webgl = addon
      } catch {
        // No WebGL: xterm falls back to its DOM renderer.
      }
    } else {
      webgl?.dispose()
      webgl = undefined
    }
  })

  // Apply preference changes to the live terminal. Only options that really
  // changed are set: saving any setting hands back new objects, and setting
  // xterm's theme (even an equal one) redraws the whole terminal.
  let applied: Record<string, unknown> = {}
  $effect(() => {
    const opts = termOptions()
    if (!term) return
    const changed = Object.fromEntries(
      Object.entries(opts).filter(([k, v]) => JSON.stringify(v) !== JSON.stringify(applied[k])),
    ) as Partial<typeof opts>
    if (Object.keys(changed).length === 0) return
    Object.assign(applied, changed)
    void loadFont(opts.fontFamily).then(() => {
      if (!term) return
      Object.assign(term.options, changed)
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
  <div bind:this={host} class="isolate h-full w-full select-text" class:hide-scrollbar={!showScrollbar} class:see-through={!!profile.background}></div>
  {#if recording || recordError}
    <span
      class="pointer-events-none absolute top-1.5 z-10 flex items-center gap-1 rounded-md px-2 py-0.5 font-mono text-[10px] {recordError ? 'bg-warning/15 text-warning' : 'bg-danger/15 text-danger-text'} {readOnly ? 'right-24' : 'right-3'}"
      role="status"
      title={recordError || `Recording output to ${recording}`}
    >
      {#if recordError}recording stopped{:else}<span class="size-1.5 animate-pulse rounded-full bg-danger" aria-hidden="true"></span>REC{/if}
    </span>
  {/if}
  {#if readOnly}
    <span class="pointer-events-none absolute top-1.5 right-3 z-10 rounded-md bg-warning/15 px-2 py-0.5 font-mono text-[10px] text-warning">read only</span>
  {/if}
  {#if bell}
    <div class="pointer-events-none absolute inset-0 z-10 rounded-md bg-fg/10" aria-hidden="true"></div>
  {/if}
  {#if menu}
    <ContextMenu
      x={menu.x}
      y={menu.y}
      label="Terminal"
      items={menuItems()}
      onClose={() => {
        menu = null
        term?.focus()
      }}
    />
  {/if}
  {#if passwordRequest}
    {#key passwordRequest.requestId}<PasswordPrompt request={passwordRequest} />{/key}
  {:else if asking && source.kind === 'host'}
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
      <button class="shrink-0 rounded-md bg-accent px-2.5 py-1 font-medium text-on-accent hover:bg-accent/90" onclick={() => reconnect()}>
        {source.kind === 'shell' ? 'Restart' : 'Reconnect'}
      </button>
    </div>
  {/if}
</div>
