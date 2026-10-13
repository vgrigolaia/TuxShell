<script lang="ts">
  import { onMount } from 'svelte'
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
  import { emptyHost, hostStore, type Host } from './lib/hosts.svelte'
  import { workspace } from './lib/workspace.svelte'
  import type { Orientation, Direction } from './lib/layout'
  import type { Source, TabInfo } from './lib/tabs'

  type Panel = { kind: 'none' } | { kind: 'clipboard' } | { kind: 'host'; host: Host }

  let panel = $state<Panel>({ kind: 'none' })
  let importOpen = $state(false)
  let quickOpen = $state(false)
  // When set, the next connection from quick connect splits this tab instead of opening a new one.
  let splitInto = $state<{ key: number; orientation: Orientation } | null>(null)
  let deleting = $state<Host | null>(null)
  // Terminal panes by pane id, so clipboard history can paste into the focused one.
  let panes = $state<Record<number, TerminalPane>>({})

  const active = $derived(workspace.activePane)
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
    // Capture phase, so shortcuts win over the focused terminal.
    const onKey = (e: KeyboardEvent) => {
      if (!e.ctrlKey || e.altKey || e.metaKey) return
      const key = e.key.toLowerCase()
      let handled = true
      if (!e.shiftKey && key === 'k') {
        if (!importOpen) openQuick(null)
      } else if (e.shiftKey && key === 'w') {
        if (active) workspace.closePane(active.id)
      } else if (e.shiftKey && key === 'b') {
        if (workspace.activeTab) workspace.toggleBroadcast(workspace.activeTab.key)
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
    return () => window.removeEventListener('keydown', onKey, { capture: true })
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

  function toggleClipboard() {
    panel = panel.kind === 'clipboard' ? { kind: 'none' } : { kind: 'clipboard' }
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
    onSelect={(key) => (workspace.activeKey = key)}
    onClose={(key) => workspace.closeTabs([key])}
    onCloseMany={(keys) => workspace.closeTabs(keys)}
    onMerge={(keys, orientation) => workspace.mergeTabs(keys, orientation)}
    onSplit={(key, orientation) => openQuick({ key, orientation })}
    onUnsplit={(key) => workspace.unsplit(key)}
    onNew={() => workspace.openTab({ kind: 'shell' }, 'Local shell')}
    onSearch={() => openQuick(null)}
    onToggleClipboard={toggleClipboard}
  />

  <main class="flex min-h-0 flex-1 gap-3 px-3">
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

    <section class="flex min-w-0 flex-1 flex-col overflow-hidden rounded-panel border border-line bg-panel">
      <div class="flex h-11 shrink-0 items-center gap-3 border-b border-line pr-2 pl-4">
        <h1 class="truncate font-mono text-[13px]">{active?.title ?? ''}</h1>
        {#if active?.termTitle && active.termTitle !== active.title}
          <span class="truncate font-mono text-xs text-subtle">{active.termTitle}</span>
        {/if}
        <span class="flex-1"></span>
        {#if tab && tab.paneIds.length > 1}
          <button
            class="flex h-7 items-center gap-1.5 rounded-lg px-2 text-xs transition-colors {tab.broadcast
              ? 'bg-warning/15 text-warning'
              : 'text-muted hover:bg-raised hover:text-fg'}"
            aria-pressed={tab.broadcast}
            aria-keyshortcuts="Control+Shift+B"
            title="Broadcast typing to all panes (Ctrl+Shift+B)"
            onclick={() => workspace.toggleBroadcast(tab.key)}
          >
            <Icon name="broadcast" size={14} /> Broadcast
          </button>
        {/if}
        {#if workspace.activeTab && workspace.activeTab.paneIds.length < 4}
          {@const key = workspace.activeTab.key}
          <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Split right" title="Split right" onclick={() => openQuick({ key, orientation: 'row' })}>
            <Icon name="splitRight" size={15} />
          </button>
          <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Split down" title="Split down" onclick={() => openQuick({ key, orientation: 'column' })}>
            <Icon name="splitDown" size={15} />
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
      <TerminalArea bind:refs={panes} />
    </section>

    {#if panel.kind === 'host'}
      {#key panel.host}
        <HostForm host={panel.host} onClose={() => (panel = { kind: 'none' })} {onSaved} onConnect={connect} />
      {/key}
    {:else if panel.kind === 'clipboard'}
      <ClipboardPanel onPaste={pasteToActive} onClose={() => (panel = { kind: 'none' })} />
    {/if}
  </main>

  <footer class="flex h-8 shrink-0 items-center gap-4 px-5 font-mono text-[11px] text-subtle">
    <span>{workspace.tabs.length} {workspace.tabs.length === 1 ? 'tab' : 'tabs'}</span>
    <span>{hostStore.list.length} hosts</span>
    {#if workspace.activeTab && workspace.activeTab.paneIds.length > 1}
      <span>Ctrl+Shift+arrows: move between panes</span>
    {/if}
    <span class="flex-1"></span>
    <span>UTF-8</span>
    <span>{active?.size ?? ''}</span>
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
  />
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
