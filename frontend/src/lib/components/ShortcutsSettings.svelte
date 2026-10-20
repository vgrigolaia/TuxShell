<script lang="ts">
  import { onDestroy } from 'svelte'
  import { isMac, keys } from '../platform'
  import { ACTIONS, checkCombo, comboOf, formatCombo, keybindings, type Action, type ActionId } from '../keybindings.svelte'

  // The action waiting for keys, and a pressed combo that needs a decision.
  let recording = $state<ActionId | null>(null)
  let pending = $state<{ id: ActionId; combo: string; error?: string; warning?: string; usedBy?: Action; extra?: boolean } | null>(null)

  const groups = ['App', 'Panes', 'Terminal'] as const

  // Keys and clicks that can't be changed.
  const fixed: [string, string][] = [
    ['Alt+Up / Alt+Down', 'Move the focused host or group in the hosts list'],
    ['F2', 'Rename the selected file (files panel)'],
    ['Ctrl+mouse wheel', 'Zoom terminal text'],
    ['Right-click terminal', 'Copy, paste, split, title, read only, zoom, preferences'],
    ['Middle-click tab', 'Close tab'],
    ['Ctrl+click tab', 'Select tabs (merge, close)'],
  ]

  function start(id: ActionId) {
    pending = null
    recording = id
    keybindings.recording = true
  }

  function stop() {
    recording = null
    keybindings.recording = false
  }

  onDestroy(stop)

  function onKey(e: KeyboardEvent) {
    if (!recording) return
    e.preventDefault()
    e.stopPropagation()
    if (e.key === 'Escape' && !e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey) {
      stop()
      pending = null
      return
    }
    const combo = comboOf(e)
    // A modifier on its own: wait for the key.
    if (!combo) return
    const id = recording
    const { error, warning } = checkCombo(combo)
    if (error) {
      // Keep listening, so the next try is one key press away.
      pending = { id, combo, error }
      return
    }
    stop()
    if (combo === keybindings.combo(id)) return void (pending = null)
    const other = keybindings.usedBy(combo, id)
    if (warning || other) pending = { id, combo, warning, usedBy: other?.action, extra: other?.extra }
    else {
      keybindings.set(id, combo)
      pending = null
    }
  }

  function accept() {
    if (pending) keybindings.take(pending.id, pending.combo)
    pending = null
  }

  const kbd = 'rounded border border-line bg-ground px-1.5 py-0.5 font-mono text-[11px] whitespace-nowrap'
  const btn = 'rounded-md px-2 py-1 text-xs text-muted hover:bg-raised hover:text-fg'
</script>

<svelte:window onkeydowncapture={onKey} />

<h3 class="sr-only">Shortcuts</h3>
<p class="mb-4 text-xs text-subtle">
  Click Change and press the new keys (Esc cancels). A shortcut needs {isMac ? '⌘, ⌥ or ⌃' : 'Ctrl or Alt'}, or an F key, so normal typing still reaches the terminal.
</p>

{#each groups as group (group)}
  <h4 class="mb-1 text-xs font-medium text-muted">{group}</h4>
  <ul class="mb-5">
    {#each ACTIONS.filter((a) => a.group === group) as a (a.id)}
      {@const combos = keybindings.combos(a.id)}
      <li class="border-b border-line py-1.5 last:border-0">
        <div class="flex min-h-7 items-center gap-2">
          <span class="min-w-0 flex-1 text-[13px]">{a.label}</span>
          {#if recording === a.id}
            <span class="rounded border border-accent px-1.5 py-0.5 text-[11px] text-accent-text">Press keys…</span>
          {:else if combos.length}
            {#each combos as c (c)}<kbd class={kbd}>{formatCombo(c)}</kbd>{/each}
          {:else}
            <span class="text-xs text-subtle">Off</span>
          {/if}
          <span class="flex w-[170px] shrink-0 justify-end gap-0.5">
            {#if recording === a.id}
              <button class={btn} onclick={() => ((pending = null), stop())}>Cancel</button>
            {:else}
              <button class={btn} aria-label="Change the shortcut for {a.label}" onclick={() => start(a.id)}>Change</button>
            {/if}
            {#if keybindings.combo(a.id) && recording !== a.id}
              <button class={btn} aria-label="Turn off the shortcut for {a.label}" onclick={() => keybindings.set(a.id, '')}>Turn off</button>
            {/if}
            {#if keybindings.changed(a.id) && recording !== a.id}
              <button class={btn} aria-label="Reset the shortcut for {a.label}" onclick={() => keybindings.reset(a.id)}>Reset</button>
            {/if}
          </span>
        </div>
        {#if pending?.id === a.id}
          <p class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs" role="status">
            {#if pending.error}
              <span class="text-warning">{pending.error}</span>
            {:else}
              <span class="text-warning">
                {#if pending.usedBy}{formatCombo(pending.combo)} is used by “{pending.usedBy.label}”.{pending.extra ? '' : ' Using it here turns that one off.'}{/if}
                {pending.warning ?? ''}
              </span>
              <button class="font-medium text-accent-text underline" onclick={accept}>Use {formatCombo(pending.combo)}</button>
              <button class="text-muted underline hover:text-fg" onclick={() => (pending = null)}>Cancel</button>
            {/if}
          </p>
        {/if}
      </li>
    {/each}
  </ul>
{/each}

{#if keybindings.anyChanged}
  <button class="mb-5 rounded-lg border border-line px-3 py-1.5 text-xs hover:bg-raised" onclick={() => keybindings.resetAll()}>Reset all shortcuts</button>
{/if}

<h4 class="mb-1 text-xs font-medium text-muted">Other keys and clicks</h4>
<table class="w-full text-[13px]">
  <tbody>
    {#each fixed as [combo, action] (combo)}
      <tr class="border-b border-line last:border-0">
        <td class="py-2 pr-4 whitespace-nowrap"><kbd class={kbd}>{keys(combo)}</kbd></td>
        <td class="py-2 text-muted">{action}</td>
      </tr>
    {/each}
  </tbody>
</table>
