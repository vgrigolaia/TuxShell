<script lang="ts">
  // A searchable dropdown for the host editor's jump host: "None", the saved SSH
  // hosts filtered as you type (name or address), and "Other" for a typed address.
  type Choice = { name: string; address: string }
  interface Props {
    id: string
    value: string // '' (none), a saved host's name, or other
    other: string // the value that means "type an address"
    hosts: Choice[]
    invalid?: boolean
    describedby?: string
    onpick: (value: string) => void
  }
  let { id, value, other, hosts, invalid = false, describedby, onpick }: Props = $props()

  const NONE_LABEL = 'None (connect directly)'
  const OTHER_LABEL = 'Other (type an address)…'
  const LIMIT = 60

  let open = $state(false)
  let query = $state('')
  let index = $state(0)
  let inputEl: HTMLInputElement
  let listEl = $state<HTMLUListElement>()

  const describe = (h: Choice) => (h.address && h.address !== h.name ? `${h.name} · ${h.address}` : h.name)
  const current = $derived(value === '' ? NONE_LABEL : value === other ? OTHER_LABEL : describe(hosts.find((h) => h.name === value) ?? { name: value, address: '' }))

  type Item = { value: string; label: string; key: string }
  const matches = $derived.by(() => {
    const q = query.trim().toLowerCase()
    return q ? hosts.filter((h) => h.name.toLowerCase().includes(q) || h.address.toLowerCase().includes(q)) : hosts
  })
  const items = $derived.by<Item[]>(() => {
    const list: Item[] = query.trim() ? [] : [{ value: '', label: NONE_LABEL, key: 'none' }]
    for (const h of matches.slice(0, LIMIT)) list.push({ value: h.name, label: describe(h), key: `h:${h.name}` })
    list.push({ value: other, label: OTHER_LABEL, key: 'other' })
    return list
  })
  const hidden = $derived(Math.max(0, matches.length - LIMIT))

  function show() {
    if (open) return
    open = true
    query = ''
    index = Math.max(0, items.findIndex((i) => i.value === value))
    queueMicrotask(() => scrollActive())
  }
  function close() {
    open = false
    query = ''
  }
  function choose(item: Item) {
    onpick(item.value)
    close()
    inputEl?.blur()
  }
  function scrollActive() {
    listEl?.querySelector<HTMLElement>(`[data-index="${index}"]`)?.scrollIntoView({ block: 'nearest' })
  }
  function move(step: number) {
    if (!open) return show()
    index = (index + step + items.length) % items.length
    queueMicrotask(scrollActive)
  }
  function onkeydown(e: KeyboardEvent) {
    switch (e.key) {
      case 'ArrowDown': e.preventDefault(); move(1); break
      case 'ArrowUp': e.preventDefault(); move(-1); break
      case 'Enter':
        // Enter picks here instead of submitting the form.
        if (open && items[index]) { e.preventDefault(); choose(items[index]) }
        break
      case 'Escape':
        if (open) { e.preventDefault(); e.stopPropagation(); close() }
        break
    }
  }
</script>

<div class="relative">
  <input
    bind:this={inputEl}
    {id}
    class="h-10 w-full rounded-[11px] border border-strong bg-field pr-9 pl-3 text-[13.5px] text-fg placeholder:text-subtle focus:border-accent focus:outline-none focus-visible:outline-none aria-[invalid=true]:border-warning"
    role="combobox"
    aria-expanded={open}
    aria-controls="{id}-list"
    aria-autocomplete="list"
    aria-activedescendant={open && items[index] ? `${id}-opt-${index}` : undefined}
    aria-invalid={invalid}
    aria-describedby={describedby}
    autocomplete="off"
    spellcheck="false"
    value={open ? query : current}
    placeholder={open ? 'Search saved hosts by name or address' : ''}
    oninput={(e) => { query = e.currentTarget.value; index = 0; open = true }}
    onfocus={show}
    onclick={show}
    onblur={close}
    {onkeydown}
  />
  <span class="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-subtle" aria-hidden="true">▾</span>
  {#if open}
    <ul bind:this={listEl} id="{id}-list" role="listbox" aria-label="Jump hosts"
      class="absolute z-40 mt-1 max-h-72 w-full overflow-y-auto rounded-xl border border-line bg-raised p-1 shadow-2xl">
      {#each items as item, i (item.key)}
        <!-- Keyboard selection lives on the combobox input (aria-activedescendant); mousedown keeps its focus. -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <li id="{id}-opt-{i}" data-index={i} role="option" aria-selected={item.value === value}
          class="cursor-pointer truncate rounded-lg px-2.5 py-1.5 text-[13px] {i === index ? 'bg-accent/15 text-fg' : 'text-text'} {item.key === 'none' || item.key === 'other' ? 'text-muted' : ''}"
          onmousedown={(e) => e.preventDefault()}
          onmousemove={() => (index = i)}
          onclick={() => choose(item)}>
          {item.label}
        </li>
      {/each}
      {#if hidden > 0}<li class="px-2.5 py-1.5 text-[11.5px] text-subtle" role="presentation">{hidden} more: type to narrow the list</li>{/if}
      {#if query.trim() && items.length === 1}<li class="px-2.5 py-1.5 text-[11.5px] text-subtle" role="presentation">No saved host matches “{query.trim()}”</li>{/if}
    </ul>
  {/if}
</div>
