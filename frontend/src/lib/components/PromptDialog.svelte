<script lang="ts">
  import { onMount } from 'svelte'

  interface Props {
    title: string
    label: string
    initial: string
    hint?: string
    submitLabel: string
    onSubmit: (value: string) => void
    onClose: () => void
  }
  let { title, label, initial, hint, submitLabel, onSubmit, onClose }: Props = $props()

  let dialog: HTMLDialogElement
  let input: HTMLInputElement
  // svelte-ignore state_referenced_locally
  let value = $state(initial)
  const id = $props.id()

  onMount(() => {
    dialog.showModal()
    input.select()
  })
</script>

<dialog
  bind:this={dialog}
  aria-labelledby="{id}-title"
  class="m-auto w-[400px] rounded-2xl border border-line bg-panel p-5 text-fg shadow-2xl backdrop:bg-black/60"
  onclose={onClose}
>
  <form
    onsubmit={(e) => {
      e.preventDefault()
      onSubmit(value)
      dialog.close()
    }}
  >
    <h2 id="{id}-title" class="font-display text-[15px] font-semibold">{title}</h2>
    <label class="mt-4 mb-1 block text-xs text-muted" for="{id}-input">{label}</label>
    <input
      bind:this={input}
      bind:value
      id="{id}-input"
      class="w-full rounded-lg border border-line bg-ground px-3 py-2 text-[13px] text-fg focus:border-accent focus:outline-none"
      autocomplete="off"
      spellcheck="false"
      aria-describedby={hint ? `${id}-hint` : undefined}
    />
    {#if hint}<p id="{id}-hint" class="mt-1.5 text-xs text-subtle">{hint}</p>{/if}
    <div class="mt-5 flex justify-end gap-2">
      <button type="button" class="rounded-lg px-3 py-1.5 text-[13px] text-muted hover:bg-raised hover:text-fg" onclick={() => dialog.close()}>Cancel</button>
      <button type="submit" class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-on-accent hover:bg-accent/90">{submitLabel}</button>
    </div>
  </form>
</dialog>
