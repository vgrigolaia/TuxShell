<script lang="ts">
  import type { HostState } from '../status.svelte'

  interface Props {
    state: HostState
    label: string
  }
  let { state, label }: Props = $props()
</script>

<!-- Filled green when up, hollow orange when slow, hollow grey when not answering. -->
{#if state !== 'unknown'}
  <span
    class="size-2 shrink-0 rounded-full {state === 'open' || state === 'up' ? 'bg-success' : state === 'slow' ? 'border-2 border-warning' : 'border-2 border-subtle'}"
    title={label}
    aria-hidden="true"
  ></span>
  <span class="sr-only">{label}</span>
{/if}
