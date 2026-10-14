<script lang="ts">
  interface Props {
    /** Size of the tile in px; the mark scales with it. */
    size?: number
  }
  let { size = 30 }: Props = $props()
</script>

<!-- Same mark as build/appicon.svg: a prompt whose cursor line runs on to two hosts. -->
<span class="logo grid shrink-0 place-items-center border border-strong bg-raised" style="width: {size}px; height: {size}px; border-radius: {size / 3}px" aria-hidden="true">
  <svg width={size * 0.66} height={size * 0.66} viewBox="0 0 24 24" fill="none" stroke-linecap="round" stroke-linejoin="round">
    <path class="text-subtle" stroke="currentColor" stroke-width="1.1" d="M14 17.85C18.4 17.85 17.25 5.55 21.6 5.55M14 17.85H21.6" />
    <path class="text-accent" stroke="currentColor" stroke-width="3.1" d="M1.6 5.7l5.6 5.85-5.6 5.85M9.8 17.85H14" />
    <circle class="text-success" fill="currentColor" cx="21.6" cy="5.55" r="2.25" />
    <circle class="text-success" fill="currentColor" cx="21.6" cy="17.85" r="2.25" />
  </svg>
</span>

<style>
  .logo {
    box-shadow: 0 0 20px color-mix(in srgb, var(--color-accent) 22%, transparent);
  }
</style>
