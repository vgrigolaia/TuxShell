<script lang="ts">
  import { initials, tint } from '../hosts.svelte'

  interface Props {
    name: string
    /** Square size in px. */
    size?: number
    dim?: boolean
  }
  let { name, size = 26, dim = false }: Props = $props()
</script>

<span
  class="tint-{tint(name)} grid shrink-0 place-items-center font-display font-bold"
  class:opacity-50={dim}
  style:width="{size}px"
  style:height="{size}px"
  style:font-size="{(size * 0.4).toFixed(1)}px"
  style:border-radius="{Math.round(size * 0.31)}px"
  aria-hidden="true"
>
  {name ? initials(name) : '?'}
</span>
