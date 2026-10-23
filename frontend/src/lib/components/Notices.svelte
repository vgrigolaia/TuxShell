<script lang="ts">
  import { notices } from '../notices.svelte'
  import Icon from './Icon.svelte'
</script>

<div role="status" aria-live="polite" class="pointer-events-none fixed bottom-9 left-1/2 z-50 flex -translate-x-1/2 flex-col items-center gap-2">
  {#each notices.list as n (n.id)}
    <p class="pointer-events-auto flex max-w-[560px] items-center gap-3 rounded-xl border border-strong bg-raised px-4 py-2.5 text-[13px] text-fg shadow-2xl">
      <span>{n.text}</span>
      <button class="grid size-6 shrink-0 place-items-center rounded-md text-subtle hover:bg-line hover:text-fg" aria-label="Dismiss" onclick={() => notices.dismiss(n.id)}>
        <Icon name="x" size={12} />
      </button>
    </p>
  {/each}
</div>
