<script lang="ts">
  // Small inline stroke icons, so no icon library is needed.
  const paths = {
    terminal: 'M4 17l6-6-6-6M12 19h8',
    search: 'M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16zM21 21l-4.3-4.3',
    clipboard: 'M9 4h6v3H9zM9 5H6a1 1 0 0 0-1 1v14a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V6a1 1 0 0 0-1-1h-3',
    plus: 'M12 5v14M5 12h14',
    x: 'M6 6l12 12M18 6L6 18',
    minimize: 'M5 12h14',
    maximize: 'M5 5h14v14H5z',
    restore: 'M8 8h11v11H8zM5 16V5h11',
    edit: 'M4 20h4L19 9l-4-4L4 16zM13.5 6.5l4 4',
    import: 'M12 4v11M7 10l5 5 5-5M5 20h14',
    chevron: 'M9 6l6 6-6 6',
    server: 'M4 5h16v6H4zM4 13h16v6H4zM8 8h.01M8 16h.01',
    shield: 'M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z',
  } as const

  interface Props {
    name: keyof typeof paths
    size?: number
  }
  let { name, size = 16 }: Props = $props()
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.8"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  <path d={paths[name]} />
</svg>
