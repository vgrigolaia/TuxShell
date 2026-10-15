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
  import { updates } from './lib/updates.svelte'
  import { logins } from './lib/logins.svelte'
  import { edits } from './lib/edits.svelte'
  import { remoteDesktops } from './lib/rdp.svelte'
  import RdpConnect from './lib/components/RdpConnect.svelte'
  import PromptDialog from './lib/components/PromptDialog.svelte'
  import { prefs } from './lib/settings.svelte'
  import { closer } from './lib/close.svelte'
  import { hostStatus } from './lib/status.svelte'
  import { tunnels } from './lib/tunnels.svelte'
  import { snippets } from './lib/snippets.svelte'
  import { emptyHost, hostStore, type Host } from './lib/hosts.svelte'
  import { HOME, workspace } from './lib/workspace.svelte'
  import Launchpad from './lib/components/Launchpad.svelte'
  import { MAX_PANES, type Orientation, type Direction } from './lib/layout'
  import type { PaneActions, Source, TabInfo } from './lib/tabs'

  type Panel = { kind: 'none' } | { kind: 'clipboard' } | { kind: 'files' } | { kind: 'host'; host: Host }

  let panel = $state<Panel>({ kind: 'none' })
  let importOpen = $state(false)
  let importStart = $state<'sshconfig' | undefined>()
  function openImport(start?: 'sshconfig') {
    importStart = start
    importOpen = true
  }
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
  let renamingTab = $state<{ key: number; name: string } | null>(null)
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
    if (source.kind === 'host' || source.kind === 'rdp') return hostStore.get(source.hostId)?.name
    return source.kind === 'shell' ? 'Local shell' : source.target
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
  const activeHostId = $derived(active?.source.kind === 'host' ? active.source.hostId : null)
  const tabInfos = $derived<TabInfo[]>(
    workspace.tabs.map((t) => ({
      key: t.key,
      title: workspace.tabTitle(t),
      exited: t.paneIds.every((id) => workspace.pane(id)?.exited),
      bell: t.paneIds.some((id) => workspace.pane(id)?.bell),
      panes: t.paneIds.length,
    })),
  )

  const tab = $derived(workspace.activeTab)
  const home = $derived(workspace.activeKey === HOME)

  const arrows: Record<string, Direction> = { ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' }

  onMount(() => {
    void hostStore.load()
    // The startup update check is on by default (Preferences → About).
    void prefs.load().then(() => {
      if (prefs.value.updateCheck) void updates.check()
    })
    const offStatus = hostStatus.start()
    const offTunnels = tunnels.start()
    const offLogins = logins.start()
    const offEdits = edits.start()
    const offRdp = remoteDesktops.start()
    void snippets.load()
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
      offStatus()
      offTunnels()
      offLogins()
      offEdits()
      offRdp()
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
  // Remote desktop hosts open their own window instead of a terminal.
  function connect(host: Host) {
    if (host.protocol === 'rdp') remoteDesktops.connecting = host
    else open({ kind: 'host', hostId: host.id, askUser: !host.user }, host.name)
  }
  function connectAs(host: Host) {
    if (host.protocol === 'rdp') remoteDesktops.connecting = host
    else open({ kind: 'host', hostId: host.id, askUser: true }, host.name)
  }

  // Opens hosts as the panes of one new tab (quick connect's "Open all in split").
  function openSplit(all: Host[]) {
    for (const h of all) if (h.protocol === 'rdp') remoteDesktops.connecting = h
    const hosts = all.filter((h) => h.protocol !== 'rdp')
    if (!hosts.length) return
    const [first, ...rest] = hosts
    workspace.openTab({ kind: 'host', hostId: first.id, askUser: !first.user }, first.name)
    const key = workspace.activeKey
    // Hosts from one group make a tab named after it, e.g. "PostgreSQL · prod".
    const groups = new Set(hosts.map((h) => h.group))
    if (groups.size === 1 && first.group) workspace.renameTab(key, first.group)
    // Three or four tiles go in a grid: one wide on top over two, or 2×2.
    for (const h of rest) workspace.splitTab(key, { kind: 'host', hostId: h.id, askUser: !h.user }, h.name, hosts.length > 2 ? 'grid' : 'row')
  }

  // The last tab shown, so leaving Home goes back to it.
  let lastTab = $state<number | null>(null)
  $effect(() => {
    if (workspace.activeKey !== HOME) lastTab = workspace.activeKey
  })

  // Terminal view with the hosts sidebar (from Home's "Hosts", double-clicked).
  function openHostList() {
    if (!hostsOpen) prefs.setHostsSidebar(true)
    const back = workspace.tabs.find((t) => t.key === lastTab) ?? workspace.tabs[0]
    if (back) workspace.activeKey = back.key
    else workspace.openTab({ kind: 'shell' }, 'Local shell')
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

  // Pastes into the focused pane, or every receiving tile while multi-exec is on.
  // Never adds Enter: the user reviews the command first.
  function pasteSnippet(command: string) {
    if (!active) return
    const targets = workspace.broadcastTargets(active.id) ?? [active.id]
    for (const id of targets) if (!workspace.pane(id)?.exited) panes[id]?.paste(command)
    workspace.focusPane(active.id)
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

  // Check new or edited hosts right away rather than at the next round.
  let knownHosts = ''
  $effect(() => {
    const key = hostStore.list.map((h) => `${h.id}:${h.hostname}:${h.proxyJump}`).join(',')
    if (hostStore.loaded && key !== knownHosts) {
      if (knownHosts) hostStatus.refresh()
      knownHosts = key
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

{#snippet hostList()}
  <Sidebar
        selectedId={panel.kind === 'host' ? panel.host.id : activeHostId}
        onConnect={connect}
        onEdit={(host) => (panel = { kind: 'host', host })}
        onConnectAs={connectAs}
        onDuplicate={duplicate}
        onDelete={(host) => (deleting = host)}
        onNew={() => (panel = { kind: 'host', host: emptyHost() })}
        onImport={() => openImport()}
        onOpenTiles={openSplit}
      />
{/snippet}

<div class="flex h-full flex-col gap-2.5 p-2.5">
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
    onRename={(key) => {
      const t = workspace.tabs.find((x) => x.key === key)
      if (t) renamingTab = { key, name: workspace.tabTitle(t) }
    }}
    onNew={() => workspace.openTab({ kind: 'shell' }, 'Local shell')}
    onSearch={() => openQuick(null)}
    onToggleClipboard={toggleClipboard}
    onPreferences={() => (prefsOpen = true)}
    homeActive={home}
    onHome={() => (workspace.activeKey = HOME)}
  />

  <main class="relative flex min-h-0 flex-1 gap-2.5">
    {#if home}
      <Launchpad
        selectedId={panel.kind === 'host' ? panel.host.id : null}
        onSelect={(host) => (panel = { kind: 'host', host })}
        onConnect={connect}
        onConnectTarget={(target) => open({ kind: 'target', target }, target)}
        onNew={() => (panel = { kind: 'host', host: emptyHost() })}
        onImport={openImport}
        onOpenHostList={openHostList}
        onPreferences={() => (prefsOpen = true)}
      >
        {#snippet editor()}
          {#if panel.kind === 'host'}
            {#key panel.host}
              <HostForm host={panel.host} onClose={() => (panel = { kind: 'none' })} {onSaved} onConnect={connect} />
            {/key}
          {/if}
        {/snippet}
      </Launchpad>
    {/if}

    <!-- Terminals stay mounted while Home shows, so sessions keep running. -->
    {#if hostsOpen && !home}
      {@render hostList()}
    {/if}

    <section aria-label="Terminals" class:hidden={home} class="relative flex min-w-0 flex-1 flex-col gap-2.5 rounded-panel border border-line bg-panel p-2.5">
      {#if tab?.broadcast}
        {@const receivers = tab.paneIds.filter((id) => !tab.excluded.includes(id))}
        <div role="status" class="flex shrink-0 flex-wrap items-center gap-3 rounded-xl border border-accent-line bg-accent-soft px-3 py-2 text-[12.5px] text-accent-ink">
          <span class="text-accent"><Icon name="broadcast" size={16} /></span>
          <span><strong class="font-semibold text-fg">Multi-exec</strong> — typing goes to {receivers.length} tiles at once</span>
          <span class="flex flex-wrap gap-1">
            {#each receivers as id (id)}<span class="hop hop-active">{workspace.pane(id)?.title}</span>{/each}
          </span>
          {#if tab.excluded.length}
            <span class="text-xs text-subtle">{tab.excluded.map((id) => workspace.pane(id)?.title).join(', ')} excluded</span>
          {/if}
          <button
            class="ml-auto h-7 rounded-lg bg-accent px-3 text-xs font-semibold text-on-accent hover:bg-accent/90"
            aria-keyshortcuts="Control+Shift+B"
            title="Stop multi-exec (Ctrl+Shift+B)"
            onclick={() => workspace.toggleBroadcast(tab.key)}>Stop</button
          >
        </div>
      {/if}
      <TerminalArea bind:refs={panes} actions={paneActions} />

    </section>

    {#if panel.kind === 'clipboard'}
      <!-- Floats over the tiles (or Home), as in the design. -->
      <ClipboardPanel activeHost={activeHost?.name} onPaste={pasteToActive} onClose={() => (panel = { kind: 'none' })} />
    {/if}

    {#if panel.kind === 'files'}
      <!-- On Home there is no terminal, so it browses this computer. -->
      <FilePanel pane={home ? undefined : active} onRunInPane={runInPane} onClose={() => (panel = { kind: 'none' })} />
    {:else if home}
      <!-- Home shows the editor inside the launchpad. -->
    {:else if panel.kind === 'host'}
      {#key panel.host}
        <HostForm host={panel.host} onClose={() => (panel = { kind: 'none' })} {onSaved} onConnect={connect} />
      {/key}
    {/if}
  </main>

  <footer class="flex h-[22px] shrink-0 flex-wrap items-center gap-[18px] px-2 text-[11.5px] text-subtle">
    {#if active}
      <span class="flex items-center gap-1.5">
        <span class="size-2 rounded-full {active.exited ? 'border-2 border-warning' : active.startedAt ? 'bg-success' : 'bg-subtle'}" aria-hidden="true"></span>
        {active.exited ? 'Disconnected' : active.startedAt ? `Connected ${since(active.startedAt, now)}` : 'Connecting…'}
      </span>
    {/if}
    {#if activeHost?.proxyJump}<span>via {activeHost.proxyJump} · ProxyJump</span>{/if}
    {#if tunnels.running}<span>{tunnels.running} {tunnels.running === 1 ? 'tunnel' : 'tunnels'}</span>{/if}
    <span>{workspace.tabs.length} {workspace.tabs.length === 1 ? 'tab' : 'tabs'} · {workspace.panes.filter((p) => !p.exited).length} live</span>
    {#if workspace.activeTab && workspace.activeTab.paneIds.length > 1}
      <span>Ctrl+Shift+arrows: move between tiles</span>
    {/if}
    <span class="ml-auto font-mono">{['UTF-8', active?.size, displayServer].filter(Boolean).join(' · ')}</span>
  </footer>
</div>

{#if importOpen}
  <ImportDialog start={importStart} onClose={() => (importOpen = false)} />
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
    onImport={() => openImport()}
    onOpenSplit={splitInto ? undefined : openSplit}
    onSnippet={!home && active ? pasteSnippet : undefined}
  />
{/if}

{#if remoteDesktops.connecting}
  {#key remoteDesktops.connecting.id}
    <RdpConnect host={remoteDesktops.connecting} onClose={() => (remoteDesktops.connecting = null)} onOpenTab={(h) => open({ kind: 'rdp', hostId: h.id }, h.name)} />
  {/key}
{/if}
{#if prefsOpen}
  <Preferences onClose={() => (prefsOpen = false)} onImport={() => openImport()} />
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

{#if renamingTab}
  {@const r = renamingTab}
  <PromptDialog
    title="Rename tab"
    label="Name"
    initial={r.name}
    hint="Leave empty to name it after its panes again."
    submitLabel="Save"
    onSubmit={(name) => workspace.renameTab(r.key, name)}
    onClose={() => (renamingTab = null)}
  />
{/if}

{#if closer.pending}
  {@const pending = closer.pending}
  <ConfirmDialog title={pending.title} message={pending.message} confirmLabel="Close" onConfirm={pending.run} onClose={() => (closer.pending = null)} onDontAsk={() => closer.stopAsking()} />
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
