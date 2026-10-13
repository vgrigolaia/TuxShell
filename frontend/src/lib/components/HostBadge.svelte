<script lang="ts">
  import { initials, tint } from '../hosts.svelte'

  interface Props {
    name: string
    /** Square size in px. */
    size?: number
    dim?: boolean
  }
  let { name, size = 28, dim = false }: Props = $props()
</script>

<span
  class="tint-{tint(name)} grid shrink-0 place-items-center rounded-lg font-mono font-semibold"
  class:opacity-50={dim}
  style:width="{size}px"
  style:height="{size}px"
  style:font-size="{Math.round(size * 0.38)}px"
  aria-hidden="true"
>
  {name ? initials(name) : '?'}
</span>
