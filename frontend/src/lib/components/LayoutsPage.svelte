<script lang="ts">
  import { keybindings } from '../keybindings.svelte'
  import { hostStore } from '../hosts.svelte'
  import { layouts, type Layout } from '../layouts.svelte'
  import type { LayoutPane } from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/settings/models'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import PromptDialog from './PromptDialog.svelte'
  import Icon from './Icon.svelte'
  import { notices } from '../notices.svelte'

  interface Props {
    /** Saves all open tabs as a layout; missing when no tab is open. */
    onSaveTabs?: () => void
  }
  let { onSaveTabs }: Props = $props()

  let renaming = $state<string | null>(null)
  let deleting = $state<string | null>(null)

  /** A pane as people know it; null when its host was deleted. */
  function paneName(p: LayoutPane): string | null {
    if (p.title) return p.title
    if (p.kind === 'shell') return 'Local shell'
    if (p.kind === 'target') return p.target
    const name = hostStore.get(p.hostId)?.name
    return name ? (p.user ? `${p.user} @ ${name}` : name) : null
  }

  const tabCount = (l: Layout) => l.tabs?.length ?? 0
  const paneCount = (l: Layout) => (l.tabs ?? []).reduce((n, t) => n + (t.panes?.length ?? 0), 0)
</script>

<main class="flex min-w-0 flex-1 flex-col gap-5 overflow-y-auto rounded-panel border border-line bg-ground px-7 py-6">
  <header class="flex flex-wrap items-center gap-3.5">
    <h1 class="font-display text-3xl font-bold tracking-[-0.03em]">Layouts</h1>
    <span class="text-sm text-subtle">{layouts.list.length} saved</span>
    <button
      class="ml-auto h-[42px] rounded-[13px] bg-accent px-[18px] text-sm font-bold text-on-accent hover:bg-accent/90 disabled:opacity-40"
      disabled={!onSaveTabs}
      title={onSaveTabs ? undefined : 'Open some tabs first'}
      onclick={() => onSaveTabs?.()}>Save open tabs</button
    >
  </header>
  <p class="-mt-2 max-w-2xl text-sm text-subtle">
    A layout opens a set of tabs and their split panes in one go. Save one from here, or right-click a tab → Save tab as layout. Open it with Open below, or
    from the search{#if keybindings.label('quickConnect')}{' '}(<span class="kbd">{keybindings.label('quickConnect')}</span>){/if}. It keeps which hosts are open and how
    they're split, never what's typed in them.
  </p>

  {#if layouts.list.length === 0}
    <div class="flex flex-col items-center gap-3 rounded-2xl border border-dashed border-strong px-6 py-12 text-center">
      <span class="grid size-10 place-items-center rounded-xl bg-raised text-subtle"><Icon name="tab" size={18} /></span>
      <p class="text-sm text-muted">No layouts yet. Open the tabs you use together, then save them here.</p>
    </div>
  {:else}
    <ul class="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(320px,1fr))]">
      {#each layouts.list as l (l.name)}
        <li class="flex flex-col gap-3 rounded-2xl border border-line bg-panel p-4">
          <div class="flex items-start gap-3">
            <span class="grid size-9 shrink-0 place-items-center rounded-xl bg-accent-soft text-accent"><Icon name="tab" size={16} /></span>
            <div class="min-w-0 flex-1">
              <h2 class="truncate font-display text-[15px] font-bold">{l.name}</h2>
              <p class="text-xs text-subtle">
                {tabCount(l)}
                {tabCount(l) === 1 ? 'tab' : 'tabs'} · {paneCount(l)}
                {paneCount(l) === 1 ? 'pane' : 'panes'}
              </p>
            </div>
          </div>
          <ul class="flex flex-col gap-1 text-[12.5px]">
            {#each l.tabs ?? [] as t, i (i)}
              <li class="flex min-w-0 items-center gap-1.5 text-text">
                <span class="shrink-0 text-subtle"><Icon name={(t.panes?.length ?? 0) > 1 ? 'splitRight' : 'terminal'} size={12} /></span>
                {#if t.name}<span class="shrink-0 font-semibold">{t.name}:</span>{/if}
                <span class="truncate">
                  {#each t.panes ?? [] as p, j (j)}
                    {@const name = paneName(p)}
                    {#if j > 0}<span class="mx-1 text-subtle">+</span>{/if}
                    {#if name}{name}{:else}<span class="text-warning" title="This host was deleted; the pane won't open">deleted host</span>{/if}
                  {/each}
                </span>
              </li>
            {/each}
          </ul>
          <div class="mt-auto flex flex-wrap items-center gap-2 border-t border-line pt-3">
            <button class="h-8 rounded-[9px] bg-accent px-3.5 text-[12.5px] font-semibold text-on-accent hover:bg-accent/90" onclick={() => layouts.open(l)}>Open</button>
            <button class="h-8 rounded-[9px] px-2.5 text-[12.5px] text-muted hover:bg-raised hover:text-fg" onclick={() => (renaming = l.name)}>Rename…</button>
            <button class="h-8 rounded-[9px] px-2.5 text-[12.5px] text-muted hover:bg-raised hover:text-fg" onclick={() => (deleting = l.name)}>Delete…</button>
            <label class="ml-auto flex items-center gap-1.5 text-[12.5px] text-text" title="Open this layout's tabs when TuxShell starts (one layout at most)">
              <input type="checkbox" class="size-4 accent-accent" checked={l.startup} onchange={(e) => layouts.setStartup(l.name, e.currentTarget.checked)} />
              Open at startup
            </label>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</main>

{#if renaming}
  {@const from = renaming}
  <PromptDialog
    title="Rename {from}"
    label="Name"
    initial={from}
    hint="Each layout needs a name of its own."
    submitLabel="Rename"
    onSubmit={(to) => layouts.rename(from, to) || notices.show(`There's already a layout called “${to.trim()}”.`)}
    onClose={() => (renaming = null)}
  />
{/if}
{#if deleting}
  {@const name = deleting}
  <ConfirmDialog
    title="Delete the layout “{name}”?"
    message="Only the saved layout goes; open tabs and your hosts stay as they are."
    confirmLabel="Delete"
    onConfirm={() => layouts.remove(name)}
    onClose={() => (deleting = null)}
  />
{/if}
