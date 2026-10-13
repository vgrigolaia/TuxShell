<script lang="ts">
  import { onMount } from 'svelte'

  interface Props {
    title: string
    message: string
    confirmLabel: string
    onConfirm: () => void | Promise<void>
    onClose: () => void
  }
  let { title, message, confirmLabel, onConfirm, onClose }: Props = $props()

  let dialog: HTMLDialogElement
  let error = $state<string | null>(null)

  onMount(() => dialog.showModal())

  async function confirm() {
    try {
      await onConfirm()
      dialog.close()
    } catch (e) {
      error = String((e as Error)?.message ?? e)
    }
  }
</script>

<dialog
  bind:this={dialog}
  aria-labelledby="confirm-title"
  aria-describedby="confirm-message"
  class="m-auto w-[400px] rounded-2xl border border-line bg-panel p-5 text-fg shadow-2xl backdrop:bg-black/60"
  onclose={onClose}
>
  <h2 id="confirm-title" class="font-display text-[15px] font-semibold">{title}</h2>
  <p id="confirm-message" class="mt-2 text-[13px] text-muted">{message}</p>
  {#if error}<p role="alert" class="mt-2 text-xs text-warning">{error}</p>{/if}
  <div class="mt-5 flex justify-end gap-2">
    <!-- Cancel is first in tab order and focused by default, so Enter doesn't delete by accident. -->
    <button class="rounded-lg px-3 py-1.5 text-[13px] text-muted hover:bg-raised hover:text-fg" onclick={() => dialog.close()}>Cancel</button>
    <button class="rounded-lg bg-danger px-3 py-1.5 text-[13px] font-medium text-white hover:bg-danger/90" onclick={confirm}>{confirmLabel}</button>
  </div>
</dialog>
