<script lang="ts">
  import { initials, tint } from '../hosts.svelte'
  import { OSES } from '../os'

  interface Props {
    name: string
    /** Operating system id; shows its logo instead of the initials. */
    os?: string
    /** Square size in px. */
    size?: number
    dim?: boolean
  }
  let { name, os = '', size = 26, dim = false }: Props = $props()

  const logo = $derived(OSES[os])
</script>

<span
  class="{logo ? `os-${os}` : `tint-${tint(name)}`} grid shrink-0 place-items-center font-display font-bold"
  class:opacity-50={dim}
  style:width="{size}px"
  style:height="{size}px"
  style:font-size="{(size * 0.4).toFixed(1)}px"
  style:border-radius="{Math.round(size * 0.31)}px"
  title={logo?.label}
  aria-hidden="true"
>
  {#if logo}
    <svg width={Math.round(size * 0.58)} height={Math.round(size * 0.58)} viewBox="0 0 24 24" fill="currentColor"><path d={logo.path} /></svg>
  {:else}
    {name ? initials(name) : '?'}
  {/if}
</span>
