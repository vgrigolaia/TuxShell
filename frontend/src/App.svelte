<script lang="ts">
  import TitleBar from './lib/components/TitleBar.svelte'
  import Sidebar from './lib/components/Sidebar.svelte'
  import TerminalPane from './lib/components/TerminalPane.svelte'
  import { sampleGroups } from './lib/sample-hosts'
  import type { Tab } from './lib/tabs'

  let nextKey = 1
  const newTab = (): Tab => ({ key: nextKey++, title: 'Local shell', exited: false })

  let tabs = $state<Tab[]>([newTab()])
  let activeKey = $state(1)
  let clipboardOpen = $state(false)
  let size = $state<Record<number, string>>({})

  const active = $derived(tabs.find((t) => t.key === activeKey))

  function openTab() {
    const tab = newTab()
    tabs.push(tab)
    activeKey = tab.key
  }

  function closeTab(key: number) {
    const i = tabs.findIndex((t) => t.key === key)
    if (i < 0) return
    tabs.splice(i, 1)
    if (tabs.length === 0) tabs.push(newTab())
    if (activeKey === key) activeKey = tabs[Math.min(i, tabs.length - 1)].key
  }

  function update(key: number, patch: Partial<Tab>) {
    const tab = tabs.find((t) => t.key === key)
    if (tab) Object.assign(tab, patch)
  }
</script>

<div class="flex h-full flex-col">
  <TitleBar
    {tabs}
    {activeKey}
    {clipboardOpen}
    onSelect={(key) => (activeKey = key)}
    onClose={closeTab}
    onNew={openTab}
    onToggleClipboard={() => (clipboardOpen = !clipboardOpen)}
  />

  <main class="flex min-h-0 flex-1 gap-3 px-3">
    <Sidebar groups={sampleGroups} />

    <section class="flex min-w-0 flex-1 flex-col overflow-hidden rounded-panel border border-line bg-panel">
      <div class="flex h-11 shrink-0 items-center border-b border-line px-4">
        <h1 class="truncate font-mono text-[13px] text-muted">{active?.title ?? ''}</h1>
      </div>
      <div class="min-h-0 flex-1 py-2 pl-3">
        {#each tabs as tab (tab.key)}
          <div id="panel-{tab.key}" role="tabpanel" aria-labelledby="tab-{tab.key}" class="h-full" class:hidden={tab.key !== activeKey}>
            <TerminalPane
              active={tab.key === activeKey}
              onTitle={(title) => update(tab.key, { title })}
              onSize={(cols, rows) => (size[tab.key] = `${cols}×${rows}`)}
              onExit={() => update(tab.key, { exited: true })}
            />
          </div>
        {/each}
      </div>
    </section>

    {#if clipboardOpen}
      <aside aria-label="Clipboard history" class="flex w-72 shrink-0 flex-col rounded-panel border border-line bg-panel p-4">
        <h2 class="font-display text-[15px] font-semibold">Clipboard</h2>
        <p class="mt-2 text-[13px] text-subtle">Encrypted clipboard history is coming in a later step.</p>
      </aside>
    {/if}
  </main>

  <footer class="flex h-8 shrink-0 items-center gap-4 px-5 font-mono text-[11px] text-subtle">
    <span>{tabs.length} {tabs.length === 1 ? 'tab' : 'tabs'}</span>
    <span class="flex-1"></span>
    <span>UTF-8</span>
    <span>{size[activeKey] ?? ''}</span>
  </footer>
</div>
