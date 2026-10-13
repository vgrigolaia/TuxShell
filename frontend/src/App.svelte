<script lang="ts">
  import { onMount } from 'svelte'
  import TitleBar from './lib/components/TitleBar.svelte'
  import Sidebar from './lib/components/Sidebar.svelte'
  import HostForm from './lib/components/HostForm.svelte'
  import TerminalPane from './lib/components/TerminalPane.svelte'
  import ImportDialog from './lib/components/ImportDialog.svelte'
  import QuickConnect from './lib/components/QuickConnect.svelte'
  import ConfirmDialog from './lib/components/ConfirmDialog.svelte'
  import ClipboardPanel from './lib/components/ClipboardPanel.svelte'
  import { emptyHost, hostStore, type Host } from './lib/hosts.svelte'
  import type { Source, Tab } from './lib/tabs'

  type Panel = { kind: 'none' } | { kind: 'clipboard' } | { kind: 'host'; host: Host }

  let nextKey = 1
  const makeTab = (source: Source, title: string): Tab => ({ key: nextKey++, title, termTitle: '', source, exited: false })

  let tabs = $state<Tab[]>([makeTab({ kind: 'shell' }, 'Local shell')])
  let activeKey = $state(1)
  let panel = $state<Panel>({ kind: 'none' })
  let size = $state<Record<number, string>>({})
  let importOpen = $state(false)
  let quickOpen = $state(false)
  let deleting = $state<Host | null>(null)
  // Terminal panes by tab key, so clipboard history can paste into the active one.
  const panes: Record<number, TerminalPane> = {}

  const active = $derived(tabs.find((t) => t.key === activeKey))
  const activeHostId = $derived(active?.source.kind === 'host' ? active.source.hostId : null)

  onMount(() => {
    void hostStore.load()
    // Capture phase, so the shortcut wins over the focused terminal.
    const onKey = (e: KeyboardEvent) => {
      const key = e.key.toLowerCase()
      if (e.ctrlKey && !e.shiftKey && !e.altKey && !e.metaKey && key === 'k') {
        e.preventDefault()
        e.stopPropagation()
        if (!importOpen) quickOpen = true
      } else if (e.ctrlKey && e.shiftKey && !e.altKey && !e.metaKey && key === 'h') {
        e.preventDefault()
        e.stopPropagation()
        toggleClipboard()
      }
    }
    window.addEventListener('keydown', onKey, { capture: true })
    return () => window.removeEventListener('keydown', onKey, { capture: true })
  })

  function openTab(source: Source, title: string) {
    const tab = makeTab(source, title)
    tabs.push(tab)
    activeKey = tab.key
  }

  // Hosts without a saved user ask for one in the tab before connecting.
  function connect(host: Host) {
    openTab({ kind: 'host', hostId: host.id, askUser: !host.user }, host.name)
  }

  function connectAs(host: Host) {
    openTab({ kind: 'host', hostId: host.id, askUser: true }, host.name)
  }

  function closeTab(key: number) {
    const i = tabs.findIndex((t) => t.key === key)
    if (i < 0) return
    tabs.splice(i, 1)
    delete panes[key]
    if (tabs.length === 0) tabs.push(makeTab({ kind: 'shell' }, 'Local shell'))
    if (activeKey === key) activeKey = tabs[Math.min(i, tabs.length - 1)].key
  }

  function update(key: number, patch: Partial<Tab>) {
    const tab = tabs.find((t) => t.key === key)
    if (tab) Object.assign(tab, patch)
  }

  function onTitle(tab: Tab, title: string) {
    update(tab.key, tab.source.kind === 'shell' ? { title, termTitle: title } : { termTitle: title })
  }

  function toggleClipboard() {
    panel = panel.kind === 'clipboard' ? { kind: 'none' } : { kind: 'clipboard' }
  }

  function pasteToActive(text: string) {
    panes[activeKey]?.paste(text)
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

  // Keep open tabs' labels in sync when a host is renamed.
  function onSaved(saved: Host) {
    for (const t of tabs) if (t.source.kind === 'host' && t.source.hostId === saved.id) t.title = saved.name
    panel = { kind: 'host', host: saved }
  }
</script>

<div class="flex h-full flex-col">
  <TitleBar
    {tabs}
    {activeKey}
    clipboardOpen={panel.kind === 'clipboard'}
    onSelect={(key) => (activeKey = key)}
    onClose={closeTab}
    onNew={() => openTab({ kind: 'shell' }, 'Local shell')}
    onSearch={() => (quickOpen = true)}
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
      <div class="flex h-11 shrink-0 items-center gap-3 border-b border-line px-4">
        <h1 class="truncate font-mono text-[13px]">{active?.title ?? ''}</h1>
        {#if active?.termTitle && active.termTitle !== active.title}
          <span class="truncate font-mono text-xs text-subtle">{active.termTitle}</span>
        {/if}
      </div>
      <div class="min-h-0 flex-1 py-2 pl-3">
        {#each tabs as tab (tab.key)}
          <div id="panel-{tab.key}" role="tabpanel" aria-labelledby="tab-{tab.key}" class="h-full" class:hidden={tab.key !== activeKey}>
            <TerminalPane
              bind:this={panes[tab.key]}
              source={tab.source}
              active={tab.key === activeKey}
              onStart={() => update(tab.key, { exited: false })}
              onTitle={(title) => onTitle(tab, title)}
              onSize={(cols, rows) => (size[tab.key] = `${cols}×${rows}`)}
              onExit={() => update(tab.key, { exited: true })}
            />
          </div>
        {/each}
      </div>
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
    <span>{tabs.length} {tabs.length === 1 ? 'tab' : 'tabs'}</span>
    <span>{hostStore.list.length} hosts</span>
    <span class="flex-1"></span>
    <span>UTF-8</span>
    <span>{size[activeKey] ?? ''}</span>
  </footer>
</div>

{#if importOpen}
  <ImportDialog onClose={() => (importOpen = false)} />
{/if}

{#if quickOpen}
  <QuickConnect
    onClose={() => (quickOpen = false)}
    onConnectHost={connect}
    onConnectTarget={(target) => openTab({ kind: 'target', target }, target)}
    onNewShell={() => openTab({ kind: 'shell' }, 'Local shell')}
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
