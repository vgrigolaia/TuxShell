<script lang="ts">
  import { keybindings, type ActionId } from './lib/keybindings.svelte'
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
  import type { TabActions } from './lib/tabbar.svelte'
  import OpenTabs from './lib/components/OpenTabs.svelte'
  import { hostStatus } from './lib/status.svelte'
  import { tunnels } from './lib/tunnels.svelte'
  import { snippets } from './lib/snippets.svelte'
  import { emptyHost, hostStore, type Host } from './lib/hosts.svelte'
  import { HOME, workspace } from './lib/workspace.svelte'
  import Launchpad from './lib/components/Launchpad.svelte'
  import { MAX_PANES, type Orientation } from './lib/layout'
  import { layouts } from './lib/layouts.svelte'
  import { notices } from './lib/notices.svelte'
  import Notices from './lib/components/Notices.svelte'
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
  // Saving tabs as a layout: the name dialog, then (for a name in use) the replace question.
  let savingLayout = $state<{ keys: number[]; name: string } | null>(null)
  let replacingLayout = $state<{ keys: number[]; name: string } | null>(null)
  function saveLayout(keys: number[], name: string) {
    layouts.save(name, keys)
    notices.show(`Saved the layout “${name.trim()}”. Open it from Home → Layouts or the search.`)
  }
  const hostsOpen = $derived(prefs.value.hostsSidebar)

  // Dragging the gap beside the hosts sidebar sets its width (saved in settings;
  // the limits match settings.MinSidebarWidth/MaxSidebarWidth in Go).
  const SIDEBAR_DEFAULT = 272
  const SIDEBAR_MIN = 200
  const SIDEBAR_MAX = 520
  let sidebarDrag = $state<number | null>(null)
  const sidebarWidth = $derived(sidebarDrag ?? (prefs.value.sidebarWidth || SIDEBAR_DEFAULT))
  const clampSidebar = (px: number) => Math.round(Math.max(SIDEBAR_MIN, Math.min(SIDEBAR_MAX, window.innerWidth / 2, px)))

  function onResizeDown(e: PointerEvent) {
    if (e.button !== 0) return
    e.preventDefault()
    const handle = e.currentTarget as HTMLElement
    try {
      handle.setPointerCapture(e.pointerId) // keeps the drag when the mouse runs ahead of the handle
    } catch {
      // No such pointer (a synthetic event); the drag still works while over the handle.
    }
    const startX = e.clientX
    const start = sidebarWidth
    const move = (ev: PointerEvent) => (sidebarDrag = clampSidebar(start + ev.clientX - startX))
    const done = () => {
      handle.removeEventListener('pointermove', move)
      handle.removeEventListener('pointerup', done)
      handle.removeEventListener('pointercancel', done)
      if (sidebarDrag !== null) prefs.setSidebarWidth(sidebarDrag)
      sidebarDrag = null
    }
    handle.addEventListener('pointermove', move)
    handle.addEventListener('pointerup', done)
    handle.addEventListener('pointercancel', done)
  }

  function onResizeKey(e: KeyboardEvent) {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    const step = (e.shiftKey ? 40 : 10) * (e.key === 'ArrowLeft' ? -1 : 1)
    prefs.setSidebarWidth(clampSidebar(sidebarWidth + step))
  }

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
  function firstHost(paneId: number): { hostId: number | null; kind: Source['kind'] } {
    const src = workspace.pane(paneId)?.source
    if (src?.kind === 'host' || src?.kind === 'rdp') return { hostId: src.hostId, kind: src.kind }
    return { hostId: null, kind: src?.kind ?? 'shell' }
  }
  const tabInfos = $derived<TabInfo[]>(
    workspace.tabs.map((t) => ({
      key: t.key,
      title: workspace.tabTitle(t),
      exited: t.paneIds.every((id) => workspace.pane(id)?.exited),
      bell: t.paneIds.some((id) => workspace.pane(id)?.bell),
      panes: t.paneIds.length,
      ...firstHost(t.paneIds[0]),
    })),
  )

  const tab = $derived(workspace.activeTab)

  // Where the open tabs show: the left panel (or the icon rail), or the title bar.
  const tabsLeft = $derived(prefs.value.tabsPosition !== 'top')
  const tabActions: TabActions = {
    select: (key) => (workspace.activeKey = key),
    close: (key) => closer.closeTabs([key]),
    closeMany: (keys) => closer.closeTabs(keys),
    merge: (keys, orientation) => workspace.mergeTabs(keys, orientation),
    split: (key, orientation) => openQuick({ key, orientation }),
    unsplit: (key) => workspace.unsplit(key),
    rename: (key) => {
      const t = workspace.tabs.find((x) => x.key === key)
      if (t) renamingTab = { key, name: workspace.tabTitle(t) }
    },
    duplicate: (key) => layouts.duplicate(key),
    move: (key, index) => workspace.moveTab(key, index),
    newShell: () => workspace.openTab({ kind: 'shell' }, 'Local shell'),
    home: () => (workspace.activeKey = HOME),
    saveLayout: (keys) => {
      const t = keys.length === 1 ? workspace.tabs.find((x) => x.key === keys[0]) : undefined
      savingLayout = { keys, name: t ? workspace.tabTitle(t) : '' }
    },
  }
  const home = $derived(workspace.activeKey === HOME)

  // What each shortcut does (Preferences → Shortcuts changes the keys).
  const run: Record<Exclude<ActionId, 'copy' | 'paste'>, () => void> = {
    quickConnect: () => !importOpen && openQuick(null),
    preferences: () => (prefsOpen = true),
    toggleHosts: () => toggleHosts(),
    toggleFiles: () => toggleFiles(),
    toggleClipboard: () => toggleClipboard(),
    broadcast: () => workspace.activeTab && workspace.toggleBroadcast(workspace.activeTab.key),
    splitAuto: () => active && splitSame(active.id, 'auto'),
    splitDown: () => active && splitSame(active.id, 'column'),
    newTab: () => active && newTabSame(active.id),
    maximize: () => active && workspace.toggleMaximized(active.id),
    scrollbar: () => active && workspace.updatePane(active.id, { showScrollbar: !active.showScrollbar }),
    rename: () => active && (renaming = { paneId: active.id, title: active.title }),
    closePane: () => active && closer.closePane(active.id),
    focusLeft: () => workspace.moveFocus('left'),
    focusRight: () => workspace.moveFocus('right'),
    focusUp: () => workspace.moveFocus('up'),
    focusDown: () => workspace.moveFocus('down'),
    zoomIn: () => prefs.zoomBy(1),
    zoomOut: () => prefs.zoomBy(-1),
    zoomReset: () => (prefs.zoom = 0),
  }

  onMount(() => {
    const hostsLoaded = hostStore.load()
    const prefsLoaded = prefs.load()
    // The startup update check is on by default (Preferences → About); the
    // Flatpak leaves updates to Flathub.
    void prefsLoaded.then(() => updates.atStartup(prefs.value.updateCheck))
    // The layout marked "Open at startup", once its hosts and the settings are in.
    void Promise.all([hostsLoaded, prefsLoaded]).then(() => {
      const startup = layouts.startup
      if (startup) layouts.open(startup)
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
    // Capture phase, so shortcuts win over the focused terminal. Copy and
    // paste are left to the terminal, which knows about the selection.
    const onKey = (e: KeyboardEvent) => {
      const id = keybindings.match(e)
      if (!id || id === 'copy' || id === 'paste') return
      e.preventDefault()
      e.stopPropagation()
      run[id]()
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
    // The saved password stays with the original (keyring entries are per host id).
    panel = { kind: 'host', host: { ...host, id: 0, name: `${host.name} copy`, tags: [...(host.tags ?? [])], passwordSaved: false } }
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
        openTabs={tabsLeft ? openTabsList : undefined}
        width={sidebarWidth}
      />
  <!-- Sits in the gap between the sidebar and the terminals. -->
  <!-- A focusable separator is the ARIA window-splitter pattern. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
  <div
    role="separator"
    aria-orientation="vertical"
    aria-label="Resize the hosts panel"
    aria-valuemin={SIDEBAR_MIN}
    aria-valuemax={SIDEBAR_MAX}
    aria-valuenow={sidebarWidth}
    tabindex="0"
    title="Drag to resize · double-click for the default width"
    class="group/resize relative -mx-2.5 w-2.5 shrink-0 cursor-col-resize touch-none focus-visible:outline-none"
    onpointerdown={onResizeDown}
    ondblclick={() => prefs.setSidebarWidth(0)}
    onkeydown={onResizeKey}
  >
    <span
      class="absolute inset-y-4 left-1/2 w-0.5 -translate-x-1/2 rounded-full bg-accent transition-opacity group-hover/resize:opacity-60 group-focus-visible/resize:opacity-100 {sidebarDrag !== null
        ? 'opacity-100'
        : 'opacity-0'}"
    ></span>
  </div>
{/snippet}

{#snippet openTabsList()}
  <OpenTabs tabs={tabInfos} activeKey={workspace.activeKey} homeActive={home} actions={tabActions} variant="list" />
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
    showTabs={!tabsLeft}
    actions={tabActions}
    onSearch={() => openQuick(null)}
    onToggleClipboard={toggleClipboard}
    onPreferences={() => (prefsOpen = true)}
    homeActive={home}
  />

  <main class="relative flex min-h-0 flex-1 gap-2.5">
    {#if home}
      <Launchpad
        selectedId={panel.kind === 'host' ? panel.host.id : null}
        onEdit={(host) => (panel = { kind: 'host', host })}
        onConnect={connect}
        onConnectAs={connectAs}
        onDuplicate={duplicate}
        onDelete={(host) => (deleting = host)}
        onConnectTarget={(target) => open({ kind: 'target', target }, target)}
        onNew={() => (panel = { kind: 'host', host: emptyHost() })}
        onImport={openImport}
        onOpenHostList={openHostList}
        onPreferences={() => (prefsOpen = true)}
        onSaveTabs={workspace.tabs.length ? () => tabActions.saveLayout(workspace.tabs.map((t) => t.key)) : undefined}
        openTabs={tabsLeft ? openTabsList : undefined}
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
    {:else if tabsLeft && !home}
      <!-- Hosts hidden: the open tabs shrink to a strip of icons. -->
      <OpenTabs tabs={tabInfos} activeKey={workspace.activeKey} homeActive={home} actions={tabActions} variant="rail" />
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
            aria-keyshortcuts={keybindings.aria('broadcast')}
            title={keybindings.title('Stop multi-exec', 'broadcast')}
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
      <FilePanel
        pane={home ? undefined : active}
        onRunInPane={runInPane}
        onOpenTarget={(target) => workspace.openTab({ kind: 'target', target }, target)}
        onAddHost={(ssh) => (panel = { kind: 'host', host: { ...emptyHost(), name: ssh.host, hostname: ssh.host, user: ssh.user, port: ssh.port || 22 } })}
        onClose={() => (panel = { kind: 'none' })}
      />
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
      {@const moves = (['focusLeft', 'focusRight', 'focusUp', 'focusDown'] as const).map((id) => keybindings.label(id)).filter(Boolean)}
      {#if moves.length}<span>{moves.join(' ')}: move between tiles</span>{/if}
    {/if}
    <span class="ml-auto font-mono">{['UTF-8', active?.size, displayServer].filter(Boolean).join(' · ')}</span>
  </footer>
</div>

{#if importOpen}
  <ImportDialog start={importStart} onClose={() => (importOpen = false)} />
{/if}

<Notices />

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
    onLayout={splitInto ? undefined : (l) => layouts.open(l)}
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

{#if savingLayout}
  {@const s = savingLayout}
  <PromptDialog
    title={s.keys.length === 1 ? 'Save tab as layout' : `Save ${s.keys.length} tabs as layout`}
    label="Layout name"
    initial={s.name}
    hint="Keeps which hosts are open and how the panes are split, nothing typed in them. Open it again from Home → Layouts or the search."
    submitLabel="Save"
    onSubmit={(name) => {
      if (!name.trim()) return
      const old = layouts.find(name)
      if (old) replacingLayout = { keys: s.keys, name: old.name }
      else saveLayout(s.keys, name)
    }}
    onClose={() => (savingLayout = null)}
  />
{/if}
{#if replacingLayout}
  {@const r = replacingLayout}
  <ConfirmDialog
    title="Replace “{r.name}”?"
    message="A layout with this name already exists. Replacing it saves the tabs you picked instead."
    confirmLabel="Replace"
    onConfirm={() => saveLayout(r.keys, r.name)}
    onClose={() => (replacingLayout = null)}
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
