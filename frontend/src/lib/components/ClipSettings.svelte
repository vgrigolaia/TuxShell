<script lang="ts">
  import { clipboardHistory, type Settings } from '../clipboard.svelte'
  import { errorMessage } from '../hosts.svelte'

  interface Props {
    onDone: () => void
  }
  let { onDone }: Props = $props()

  const retentionOptions = [
    { value: 1, label: '1 day' },
    { value: 7, label: '7 days' },
    { value: 30, label: '30 days' },
    { value: 90, label: '90 days' },
    { value: 365, label: '1 year' },
    { value: 0, label: 'Forever' },
  ]
  const entryOptions = [
    { value: 100, label: '100' },
    { value: 500, label: '500' },
    { value: 1000, label: '1,000' },
    { value: 5000, label: '5,000' },
    { value: 0, label: 'No limit' },
  ]

  // svelte-ignore state_referenced_locally
  let draft = $state<Settings>({ ...clipboardHistory.settings })
  let error = $state<string | null>(null)

  // Keep a custom value the user set elsewhere selectable.
  const withCurrent = (opts: { value: number; label: string }[], v: number, unit: string) =>
    opts.some((o) => o.value === v) ? opts : [...opts, { value: v, label: `${v} ${unit}` }]

  const unpinned = $derived(clipboardHistory.history.length)
  const trims = $derived(draft.maxEntries > 0 && unpinned > draft.maxEntries ? unpinned - draft.maxEntries : 0)

  async function save(e: SubmitEvent) {
    e.preventDefault()
    error = null
    try {
      await clipboardHistory.saveSettings({ ...draft, retentionDays: Number(draft.retentionDays), maxEntries: Number(draft.maxEntries) })
      onDone()
    } catch (err) {
      error = errorMessage(err)
    }
  }

  const select =
    'w-full rounded-lg border border-line bg-ground px-3 py-2 text-[13px] focus:border-accent focus:outline-none'
</script>

<form class="flex flex-col gap-4 px-4 pb-4" onsubmit={save}>
  <h3 class="text-[13px] font-medium">History settings</h3>

  <div>
    <label for="clip-retention" class="mb-1 block text-xs text-muted">Keep history for</label>
    <select id="clip-retention" class={select} bind:value={draft.retentionDays}>
      {#each withCurrent(retentionOptions, draft.retentionDays, 'days') as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
    </select>
  </div>

  <div>
    <label for="clip-max" class="mb-1 block text-xs text-muted">Maximum entries</label>
    <select id="clip-max" class={select} bind:value={draft.maxEntries} aria-describedby="clip-max-hint">
      {#each withCurrent(entryOptions, draft.maxEntries, 'entries') as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
    </select>
    <p id="clip-max-hint" class="mt-1 text-xs {trims ? 'text-warning' : 'text-subtle'}">
      {trims ? `Saving removes the ${trims} oldest history entries.` : 'Saved clips never count toward the limit.'}
    </p>
  </div>

  <label class="flex items-start gap-2.5 text-[13px]">
    <input type="checkbox" class="mt-0.5 size-4 accent-accent" bind:checked={draft.captureSystem} />
    <span>
      Record clipboard from other apps
      <span class="block text-xs text-subtle">Captured when you switch back to TuxShell. Off: only Ctrl+Shift+C in a terminal.</span>
    </span>
  </label>

  {#if error}<p role="alert" class="text-xs text-warning">{error}</p>{/if}

  <div class="flex justify-end gap-2">
    <button type="button" class="rounded-lg px-3 py-1.5 text-[13px] text-muted hover:bg-raised hover:text-fg" onclick={onDone}>Cancel</button>
    <button type="submit" class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-on-accent hover:bg-accent/90">Save</button>
  </div>
</form>
