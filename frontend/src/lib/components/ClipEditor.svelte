<script lang="ts">
  import { onMount } from 'svelte'
  import { clipboardHistory, type Clip } from '../clipboard.svelte'
  import { errorMessage } from '../hosts.svelte'

  interface Props {
    /** Clip to edit, or null for a new saved clip. */
    clip: Clip | null
    onDone: () => void
  }
  let { clip, onDone }: Props = $props()

  // svelte-ignore state_referenced_locally
  let text = $state(clip?.text ?? '')
  // svelte-ignore state_referenced_locally
  let label = $state(clip?.label ?? '')
  let error = $state<string | null>(null)
  let saving = $state(false)
  let area: HTMLTextAreaElement

  onMount(() => area.focus())

  async function save(e: SubmitEvent) {
    e.preventDefault()
    saving = true
    error = null
    try {
      if (clip) await clipboardHistory.updateClip(clip.id, text, label)
      else await clipboardHistory.saveClip(text, label)
      onDone()
    } catch (err) {
      error = errorMessage(err)
    } finally {
      saving = false
    }
  }
</script>

<form class="flex min-h-0 flex-1 flex-col gap-3 px-4 pb-4" onsubmit={save}>
  <h3 class="text-[13px] font-medium">{clip ? 'Edit saved clip' : 'New saved clip'}</h3>
  <div>
    <label for="clip-label" class="mb-1 block text-xs text-muted">Name (optional)</label>
    <input
      id="clip-label"
      bind:value={label}
      maxlength="100"
      class="w-full rounded-lg border border-line bg-ground px-3 py-2 text-[13px] placeholder:text-subtle focus:border-accent focus:outline-none"
      placeholder="restart patroni"
      autocomplete="off"
    />
  </div>
  <div class="flex min-h-0 flex-1 flex-col">
    <label for="clip-text" class="mb-1 block text-xs text-muted">Text</label>
    <textarea
      id="clip-text"
      bind:this={area}
      bind:value={text}
      rows="8"
      class="min-h-32 w-full flex-1 resize-none rounded-lg border border-line bg-ground px-3 py-2 font-mono text-xs placeholder:text-subtle focus:border-accent focus:outline-none aria-[invalid=true]:border-warning"
      placeholder="sudo systemctl restart patroni"
      spellcheck="false"
      aria-invalid={!!error}
      aria-describedby={error ? 'clip-error' : 'clip-hint'}
      onkeydown={(e) => {
        if (e.key === 'Enter' && e.ctrlKey) (e.currentTarget as HTMLTextAreaElement).form?.requestSubmit()
        if (e.key === 'Escape') onDone()
      }}
    ></textarea>
    {#if error}
      <p id="clip-error" role="alert" class="mt-1 text-xs text-warning">{error}</p>
    {:else}
      <p id="clip-hint" class="mt-1 text-xs text-subtle">Saved clips are encrypted and kept until you delete them. Ctrl+Enter saves.</p>
    {/if}
  </div>
  <div class="flex justify-end gap-2">
    <button type="button" class="rounded-lg px-3 py-1.5 text-[13px] text-muted hover:bg-raised hover:text-fg" onclick={onDone}>Cancel</button>
    <button type="submit" disabled={saving || !text.trim()} class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-on-accent hover:bg-accent/90 disabled:opacity-50">Save</button>
  </div>
</form>
