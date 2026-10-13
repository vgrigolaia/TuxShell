<script lang="ts">
  import { onMount } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { WebglAddon } from '@xterm/addon-webgl'
  import { SearchAddon } from '@xterm/addon-search'
  import { WebLinksAddon } from '@xterm/addon-web-links'
  import { Unicode11Addon } from '@xterm/addon-unicode11'
  import { Browser, Events } from '@wailsio/runtime'
  import * as Terminals from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/terminal/service'

  interface Props {
    active: boolean
    onTitle?: (title: string) => void
    onSize?: (cols: number, rows: number) => void
    onExit?: (code: number) => void
  }

  let { active, onTitle, onSize, onExit }: Props = $props()

  let host: HTMLDivElement
  let term: Terminal | undefined
  let fit: FitAddon | undefined
  let exited = $state<number | null>(null)

  const FONT = '"JetBrains Mono Variable", monospace'

  function decode(b64: string): Uint8Array {
    const bin = atob(b64)
    const bytes = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
    return bytes
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
      term.open(host)
      try {
        const webgl = new WebglAddon()
        webgl.onContextLoss(() => webgl.dispose())
        term.loadAddon(webgl)
      } catch {
        // No WebGL: xterm falls back to its DOM renderer.
      }
      fit.fit()

      cleanups.push(
        Events.On('terminal:data', (ev) => {
          if (ev.data.id === sessionId) term?.write(decode(ev.data.data))
        }),
        Events.On('terminal:exit', (ev) => {
          if (ev.data.id !== sessionId) return
          exited = ev.data.code
          sessionId = undefined
          onExit?.(ev.data.code)
        }),
      )

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
      try {
        sessionId = await Terminals.StartShell(term.cols, term.rows)
      } catch (err) {
        term.write(`\r\n\x1b[31mCould not start shell: ${err}\x1b[0m\r\n`)
        return
      }
      if (disposed) void Terminals.Close(sessionId)
      if (active) term.focus()
    }
    void start()

    return () => {
      disposed = true
      if (sessionId) void Terminals.Close(sessionId)
      cleanups.forEach((fn) => fn())
      term?.dispose()
    }
  })

  $effect(() => {
    if (active && term) {
      requestAnimationFrame(() => {
        fit?.fit()
        term?.focus()
      })
    }
  })
</script>

<div class="relative h-full w-full" class:hidden={!active}>
  <div bind:this={host} class="h-full w-full select-text"></div>
  {#if exited !== null}
    <p class="absolute right-4 bottom-3 rounded-lg border border-line bg-raised px-3 py-1.5 font-mono text-xs text-muted">
      Process exited with code {exited}
    </p>
  {/if}
</div>
