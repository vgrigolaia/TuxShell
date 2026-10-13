<script lang="ts">
  import { onMount } from 'svelte'
  import { Events, Window } from '@wailsio/runtime'
  import * as Terminals from '../bindings/github.com/vgrigolaia/tuxshell/internal/terminal/service'
  import TitleBar from './lib/components/TitleBar.svelte'
  import Sidebar from './lib/components/Sidebar.svelte'
  import HostForm from './lib/components/HostForm.svelte'
  import TerminalArea from './lib/components/TerminalArea.svelte'
  import type TerminalPane from './lib/components/TerminalPane.svelte'
  import ImportDialog from './lib/components/ImportDialog.svelte'
  import QuickConnect from './lib/components/QuickConnect.svelte'
  import ConfirmDialog from './lib/components/ConfirmDialog.svelte'
  import ClipboardPanel from './lib/components/ClipboardPanel.svelte'
  import Icon from './lib/components/Icon.svelte'
  import FilePanel from './lib/components/FilePanel.svelte'
  import Preferences from './lib/components/Preferences.svelte'
  import PromptDialog from './lib/components/PromptDialog.svelte'
  import { prefs } from './lib/settings.svelte'
  import { closer } from './lib/close.svelte'
  import { emptyHost, hostStore, type Host } from './lib/hosts.svelte'
  import { workspace } from './lib/workspace.svelte'
  import { MAX_PANES, type Orientation, type Direction } from './lib/layout'
  import type { PaneActions, Source, TabInfo } from './lib/tabs'

  type Panel = { kind: 'none' } | { kind: 'clipboard' } | { kind: 'files' } | { kind: 'host'; host: Host }

  let panel = $state<Panel>({ kind: 'none' })
  let importOpen = $state(false)
  let quickOpen = $state(false)
  // When set, the next connection from quick connect splits this tab instead of opening a new one.
  let splitInto = $state<{ key: number; orientation: Orientation } | null>(null)
  let deleting = $state<Host | null>(null)
  let prefsOpen = $state(false)
  // Clock for "Connected 12m" in the status bar.
  let now = $state(Date.now())
  let displayServer = $state('')

  function since(start: number, at: number) {
    const m = Math.max(0, Math.floor((at - start) / 60000))
    return m < 1 ? 'just now' : m < 60 ? `${m}m` : `${Math.floor(m / 60)}h ${m % 60}m`
  }
  let renaming = $state<{ paneId: number; title: string } | null>(null)
  const hostsOpen = $derived(prefs.value.hostsSidebar)

  // Splits a pane's tab with the same connection (no new login thanks to
  // shared ssh connections). "auto" splits along the pane's longer side.
  function splitSame(paneId: number, orientation: Orientation | 'auto') {
    const t = workspace.tabOf(paneId)
    const p = workspace.pane(paneId)
    const source = workspace.sameSource(paneId)
    if (!t || !p || !source) return
    if (orientation === 'auto') {
      const r = document.querySelector(`[data-pane-id="${paneId}"]`)?.getBoundingClientRect()
      orientation = r && r.height > r.width ? 'column' : 'row'
    }
    workspace.focusPane(paneId)
    workspace.splitTab(t.key, source, p.customTitle ? p.title : (hostTitle(source) ?? p.title), orientation)
  }

  function hostTitle(source: Source) {
    return source.kind === 'host' ? hostStore.get(source.hostId)?.name : source.kind === 'shell' ? 'Local shell' : source.target
  }

  function newTabSame(paneId: number) {
    const source = workspace.sameSource(paneId)
    if (source) workspace.openTab(source, hostTitle(source) ?? 'Terminal')
  }

  function paneActions(paneId: number): PaneActions {
    const t = workspace.tabOf(paneId)
    const full = !t || t.paneIds.length >= MAX_PANES
    return {
      split: full ? undefined : (o) => splitSame(paneId, o),
      splitOther: full
        ? undefined
        : () => {
            workspace.focusPane(paneId)
            if (t) openQuick({ key: t.key, orientation: 'row' })
          },
      newTab: () => newTabSame(paneId),
      rename: () => (renaming = { paneId, title: workspace.pane(paneId)?.title ?? '' }),
      close: () => closer.closePane(paneId),
      toggleMaximized: () => workspace.toggleMaximized(paneId),
      toggleReadOnly: () => workspace.updatePane(paneId, { readOnly: !workspace.pane(paneId)?.readOnly }),
      toggleScrollbar: () => workspace.updatePane(paneId, { showScrollbar: !workspace.pane(paneId)?.showScrollbar }),
      preferences: () => (prefsOpen = true),
    }
  }

  function toggleHosts() {
    prefs.setHostsSidebar(!hostsOpen)
  }
  // Terminal panes by pane id, so clipboard history can paste into the focused one.
  let panes = $state<Record<number, TerminalPane>>({})

  const active = $derived(workspace.activePane)
  const activeHost = $derived(active?.source.kind === 'host' ? hostStore.get(active.source.hostId) : undefined)
  // "user@host" like the prompt; local shells show their own title.
  const paneLabel = $derived.by(() => {
    if (!active) return ''
    if (active.customTitle || active.source.kind === 'target') return active.title
    if (active.source.kind === 'shell') return active.termTitle || active.title
    const user = active.loginUser || activeHost?.user
    return user ? `${user}@${active.title}` : active.title
  })
  const activeHostId = $derived(active?.source.kind === 'host' ? active.source.hostId : null)
  const tabInfos = $derived<TabInfo[]>(
    workspace.tabs.map((t) => ({
      key: t.key,
      title: workspace.tabTitle(t),
      exited: t.paneIds.every((id) => workspace.pane(id)?.exited),
      panes: t.paneIds.length,
    })),
  )

  const tab = $derived(workspace.activeTab)
  const receiving = $derived(tab?.broadcast ? tab.paneIds.filter((id) => !tab.excluded.includes(id)).length : 0)

  const arrows: Record<string, Direction> = { ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' }

  onMount(() => {
    void hostStore.load()
    void prefs.load()
    void Terminals.DisplayServer().then((d) => (displayServer = d), () => {})
    const clock = setInterval(() => (now = Date.now()), 30000)
    // The window manager's close (or ours) was held back because sessions are live.
    const offClose = Events.On('app:confirm-close', (ev) =>
      closer.ask(ev.data, 'TuxShell', async () => {
        await Terminals.CloseAll()
        await Window.Close()
      }),
    )
    // Capture phase, so shortcuts win over the focused terminal.
    const onKey = (e: KeyboardEvent) => {
      if (!e.ctrlKey || e.metaKey) return
      const key = e.key.toLowerCase()
      let handled = true
      if (e.altKey) {
        // Ctrl+Alt+W: set the pane title (Terminator's key).
        if (key === 'w' && active) renaming = { paneId: active.id, title: active.title }
        else handled = false
      } else if (!e.shiftKey && key === ',') {
        prefsOpen = true
      } else if (key === '=' || key === '+') {
        prefs.zoomBy(1)
      } else if (!e.shiftKey && key === '-') {
        prefs.zoomBy(-1)
      } else if (!e.shiftKey && key === '0') {
        prefs.zoom = 0
      } else if (!e.shiftKey && key === 'k') {
        if (!importOpen) openQuick(null)
      } else if (e.shiftKey && key === 'w') {
        if (active) closer.closePane(active.id)
      } else if (e.shiftKey && key === 'a') {
        if (active) splitSame(active.id, 'auto')
      } else if (e.shiftKey && key === 'o') {
        if (active) splitSame(active.id, 'column')
      } else if (e.shiftKey && key === 't') {
        if (active) newTabSame(active.id)
      } else if (e.shiftKey && key === 'x') {
        if (active) workspace.toggleMaximized(active.id)
      } else if (e.shiftKey && key === 's') {
        if (active) workspace.updatePane(active.id, { showScrollbar: !active.showScrollbar })
      } else if (e.shiftKey && key === 'b') {
        if (workspace.activeTab) workspace.toggleBroadcast(workspace.activeTab.key)
      } else if (e.shiftKey && key === 'e') {
        toggleFiles()
      } else if (e.shiftKey && key === 'l') {
        toggleHosts()
      } else if (e.shiftKey && key === 'h') {
        toggleClipboard()
      } else if (e.shiftKey && arrows[e.key]) {
        workspace.moveFocus(arrows[e.key])
      } else {
        handled = false
      }
      if (handled) {
        e.preventDefault()
        e.stopPropagation()
      }
    }
    window.addEventListener('keydown', onKey, { capture: true })
    return () => {
      clearInterval(clock)
      offClose()
      window.removeEventListener('keydown', onKey, { capture: true })
    }
  })

  function openQuick(split: { key: number; orientation: Orientation } | null) {
    splitInto = split
    quickOpen = true
  }

  /** Opens a source in a new tab, or splits the pending tab. */
  function open(source: Source, title: string) {
    if (splitInto) {
      const ok = workspace.splitTab(splitInto.key, source, title, splitInto.orientation)
      splitInto = null
      if (ok) return
    }
    workspace.openTab(source, title)
  }

  // Hosts without a saved user ask for one in the pane before connecting.
  const connect = (host: Host) => open({ kind: 'host', hostId: host.id, askUser: !host.user }, host.name)
  const connectAs = (host: Host) => open({ kind: 'host', hostId: host.id, askUser: true }, host.name)

  // Opens hosts as the panes of one new tab (quick connect's "Open all in split").
  function openSplit(hosts: Host[]) {
    const [first, ...rest] = hosts
    workspace.openTab({ kind: 'host', hostId: first.id, askUser: !first.user }, first.name)
    const key = workspace.activeKey
    for (const h of rest) workspace.splitTab(key, { kind: 'host', hostId: h.id, askUser: !h.user }, h.name, 'row')
  }

  function toggleClipboard() {
    panel = panel.kind === 'clipboard' ? { kind: 'none' } : { kind: 'clipboard' }
  }

  function toggleFiles() {
    panel = panel.kind === 'files' ? { kind: 'none' } : { kind: 'files' }
  }

  // Clears the current input line (Ctrl+U) and runs line in the pane's shell.
  function runInPane(paneId: number, line: string) {
    panes[paneId]?.send(`\x15${line}\r`)
    workspace.focusPane(paneId)
  }

  function pasteToActive(text: string) {
    if (active) panes[active.id]?.paste(text)
  }

  function duplicate(host: Host) {
    panel = { kind: 'host', host: { ...host, id: 0, name: `${host.name} copy`, tags: [...(host.tags ?? [])] } }
  }

  async function confirmDelete(host: Host) {
    await hostStore.remove(host.id)
    if (panel.kind === 'host' && panel.host.id === host.id) panel = { kind: 'none' }
  }

  // Close the editor if its host was deleted (e.g. by a bulk delete).
  $effect(() => {
    if (panel.kind === 'host' && panel.host.id !== 0 && hostStore.loaded && !hostStore.get(panel.host.id)) {
      panel = { kind: 'none' }
    }
  })

  // Forget refs of closed panes.
  $effect(() => {
    const ids = new Set(workspace.panes.map((p) => p.id))
    for (const id of Object.keys(panes).map(Number)) if (!ids.has(id)) delete panes[id]
  })

  function onSaved(saved: Host) {
    workspace.renameHost(saved.id, saved.name)
    panel = { kind: 'host', host: saved }
  }
</script>

<div class="flex h-full flex-col">
  <TitleBar
    tabs={tabInfos}
    activeKey={workspace.activeKey}
    clipboardOpen={panel.kind === 'clipboard'}
    {hostsOpen}
    onToggleHosts={toggleHosts}
    filesOpen={panel.kind === 'files'}
    onToggleFiles={toggleFiles}
    onSelect={(key) => (workspace.activeKey = key)}
    onClose={(key) => closer.closeTabs([key])}
    onCloseMany={(keys) => closer.closeTabs(keys)}
    onMerge={(keys, orientation) => workspace.mergeTabs(keys, orientation)}
    onSplit={(key, orientation) => openQuick({ key, orientation })}
    onUnsplit={(key) => workspace.unsplit(key)}
    onNew={() => workspace.openTab({ kind: 'shell' }, 'Local shell')}
    onSearch={() => openQuick(null)}
    onToggleClipboard={toggleClipboard}
    onPreferences={() => (prefsOpen = true)}
  />

  <main class="flex min-h-0 flex-1 gap-3 px-3">
    {#if hostsOpen}
    <Sidebar
      selectedId={panel.kind === 'host' ? panel.host.id : activeHostId}
      onConnect={connect}
      onEdit={(host) => (panel = { kind: 'host', host })}
      onConnectAs={connectAs}
      onDuplicate={duplicate}
      onDelete={(host) => (deleting = host)}
      onNew={() => (panel = { kind: 'host', host: emptyHost() })}
      onImport={() => (importOpen = true)}
    />
    {/if}

    <section class="flex min-w-0 flex-1 flex-col overflow-hidden rounded-panel border border-line bg-panel">
      <div class="flex h-14 shrink-0 items-center gap-2 border-b border-line pr-3 pl-4">
        <h1 class="mr-1 truncate font-mono text-sm">{paneLabel}</h1>
        {#if activeHost?.proxyJump}
          <span class="flex shrink-0 items-center gap-1 rounded-lg border border-line px-2 py-0.5 text-xs text-muted" title="Connects through {activeHost.proxyJump}">
            <span aria-hidden="true">→</span> via {activeHost.proxyJump}
          </span>
        {/if}
        {#if active?.source.kind !== 'shell' && active?.termTitle && active.termTitle !== paneLabel}
          <span class="truncate font-mono text-xs text-subtle">{active.termTitle}</span>
        {/if}
        <span class="flex-1"></span>
        {#if workspace.activeTab && workspace.activeTab.paneIds.length < 4}
          {@const key = workspace.activeTab.key}
          <button class="grid size-8 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Split right" title="Split right" onclick={() => openQuick({ key, orientation: 'row' })}>
            <Icon name="splitRight" size={16} />
          </button>
          <button class="grid size-8 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Split down" title="Split down" onclick={() => openQuick({ key, orientation: 'column' })}>
            <Icon name="splitDown" size={16} />
          </button>
        {/if}
        {#if tab && tab.paneIds.length > 1}
          <button
            class="grid size-8 place-items-center rounded-lg transition-colors {tab.broadcast ? 'bg-warning/15 text-warning' : 'text-muted hover:bg-raised hover:text-fg'}"
            aria-label="Broadcast typing to all panes"
            aria-pressed={tab.broadcast}
            aria-keyshortcuts="Control+Shift+B"
            title="Broadcast typing to all panes (Ctrl+Shift+B)"
            onclick={() => workspace.toggleBroadcast(tab.key)}
          >
            <Icon name="broadcast" size={16} />
          </button>
        {/if}
      </div>
      {#if tab?.broadcast}
        <div role="status" class="mx-2 mt-2 flex shrink-0 items-center gap-3 rounded-xl border border-warning/50 bg-warning/10 px-3 py-2 text-[13px]">
          <span class="text-warning"><Icon name="broadcast" size={15} /></span>
          <span class="flex-1">
            <strong class="font-semibold">Broadcast on</strong> — keystrokes go to {receiving} of {tab.paneIds.length} panes
            {#if tab.excluded.length}
              <span class="text-subtle">· {tab.excluded.map((id) => workspace.pane(id)?.title).join(', ')} excluded</span>
            {/if}
          </span>
          <button class="rounded-lg bg-warning px-2.5 py-1 text-xs font-semibold text-ground hover:bg-warning/90" onclick={() => workspace.toggleBroadcast(tab.key)}>
            Stop · Ctrl+Shift+B
          </button>
        </div>
      {/if}
      <TerminalArea bind:refs={panes} actions={paneActions} />
    </section>

    {#if panel.kind === 'host'}
      {#key panel.host}
        <HostForm host={panel.host} onClose={() => (panel = { kind: 'none' })} {onSaved} onConnect={connect} />
      {/key}
    {:else if panel.kind === 'files'}
      <FilePanel pane={active} onRunInPane={runInPane} onClose={() => (panel = { kind: 'none' })} />
    {:else if panel.kind === 'clipboard'}
      <ClipboardPanel onPaste={pasteToActive} onClose={() => (panel = { kind: 'none' })} />
    {/if}
  </main>

  <footer class="flex h-9 shrink-0 items-center gap-5 px-5 text-xs text-subtle">
    <span>{active?.exited ? 'Disconnected' : active?.startedAt ? `Connected ${since(active.startedAt, now)}` : 'Connecting…'}</span>
    <span>{workspace.tabs.length} {workspace.tabs.length === 1 ? 'tab' : 'tabs'} · {workspace.panes.filter((p) => !p.exited).length} live</span>
    {#if workspace.activeTab && workspace.activeTab.paneIds.length > 1}
      <span>Ctrl+Shift+arrows: move between panes</span>
    {/if}
    <span class="flex-1"></span>
    <span class="font-mono">{['UTF-8', active?.size, displayServer].filter(Boolean).join(' · ')}</span>
  </footer>
</div>

{#if importOpen}
  <ImportDialog onClose={() => (importOpen = false)} />
{/if}

{#if quickOpen}
  <QuickConnect
    placeholder={splitInto ? `Split ${splitInto.orientation === 'row' ? 'right' : 'down'}: search hosts or type user@host` : undefined}
    onClose={() => {
      quickOpen = false
      // Dismissing the picker cancels a pending split.
      queueMicrotask(() => (splitInto = null))
    }}
    onConnectHost={connect}
    onConnectTarget={(target) => open({ kind: 'target', target }, target)}
    onNewShell={() => open({ kind: 'shell' }, 'Local shell')}
    onNewHost={() => (panel = { kind: 'host', host: emptyHost() })}
    onImport={() => (importOpen = true)}
    onOpenSplit={splitInto ? undefined : openSplit}
  />
{/if}

{#if prefsOpen}
  <Preferences onClose={() => (prefsOpen = false)} />
{/if}

{#if renaming}
  {@const r = renaming}
  <PromptDialog
    title="Set pane title"
    label="Title"
    initial={r.title}
    hint="Leave empty to use the terminal's own title again."
    submitLabel="Save"
    onSubmit={(title) => workspace.renamePane(r.paneId, title)}
    onClose={() => (renaming = null)}
  />
{/if}

{#if closer.pending}
  {@const pending = closer.pending}
  <ConfirmDialog title={pending.title} message={pending.message} confirmLabel="Close" onConfirm={pending.run} onClose={() => (closer.pending = null)} />
{/if}

{#if deleting}
  {@const host = deleting}
  <ConfirmDialog
    title="Delete {host.name}?"
    message="This removes the saved host from TuxShell. Open sessions keep running, and your ~/.ssh/config is not changed."
    confirmLabel="Delete"
    onConfirm={() => confirmDelete(host)}
    onClose={() => (deleting = null)}
  />
{/if}
