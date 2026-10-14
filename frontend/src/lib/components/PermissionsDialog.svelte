<script lang="ts">
  import { onMount } from 'svelte'

  interface Props {
    name: string
    /** Current permissions in octal, e.g. "644". */
    mode: string
    isDir: boolean
    onApply: (mode: string, recursive: boolean) => Promise<void>
    onClose: () => void
  }
  let { name, mode, isDir, onApply, onClose }: Props = $props()

  let dialog: HTMLDialogElement
  // svelte-ignore state_referenced_locally
  let bits = $state(parseInt(mode, 8) & 0o777)
  // What the number box shows; typing there isn't reformatted mid-edit.
  // svelte-ignore state_referenced_locally
  let text = $state((parseInt(mode, 8) & 0o777).toString(8).padStart(3, '0'))
  let recursive = $state(false)
  let error = $state('')
  let busy = $state(false)
  const id = $props.id()

  onMount(() => dialog.showModal())

  const who = [
    ['Owner', 6],
    ['Group', 3],
    ['Others', 0],
  ] as const
  const what = [
    ['Read', 4],
    ['Write', 2],
    ['Execute', 1],
  ] as const

  function toggle(shift: number, bit: number, on: boolean) {
    const b = bit << shift
    bits = on ? bits | b : bits & ~b
    text = bits.toString(8).padStart(3, '0')
  }

  function typed(v: string) {
    text = v
    if (/^[0-7]{1,3}$/.test(v)) bits = parseInt(v, 8)
  }

  async function apply() {
    error = ''
    busy = true
    try {
      await onApply(bits.toString(8).padStart(3, '0'), isDir && recursive)
      dialog.close()
    } catch (e) {
      error = String((e as Error)?.message ?? e)
    } finally {
      busy = false
    }
  }
</script>

<dialog
  bind:this={dialog}
  aria-labelledby="{id}-title"
  class="m-auto w-[400px] rounded-2xl border border-line bg-panel p-5 text-fg shadow-2xl backdrop:bg-black/60"
  onclose={onClose}
>
  <h2 id="{id}-title" class="font-display text-[15px] font-semibold">Permissions</h2>
  <p class="mt-1 truncate font-mono text-xs text-subtle" title={name}>{name}</p>

  <table class="mt-4 w-full text-[13px]">
    <thead>
      <tr class="text-xs text-muted">
        <th class="pb-2 text-left font-medium"><span class="sr-only">Who</span></th>
        {#each what as [label] (label)}<th class="pb-2 font-medium">{label}{label === 'Execute' && isDir ? ' (open)' : ''}</th>{/each}
      </tr>
    </thead>
    <tbody>
      {#each who as [person, shift] (person)}
        <tr>
          <th class="py-1.5 text-left font-medium">{person}</th>
          {#each what as [label, bit] (label)}
            <td class="text-center">
              <input
                type="checkbox"
                class="size-4 accent-accent"
                aria-label="{person} {label.toLowerCase()}"
                checked={(bits & (bit << shift)) !== 0}
                onchange={(e) => toggle(shift, bit, e.currentTarget.checked)}
              />
            </td>
          {/each}
        </tr>
      {/each}
    </tbody>
  </table>

  <div class="mt-4 flex items-center gap-3">
    <label class="text-xs font-medium text-muted" for="{id}-octal">As a number</label>
    <input
      id="{id}-octal"
      class="h-8 w-20 rounded-lg border border-line bg-ground px-2.5 text-center font-mono text-[13px] focus:border-accent focus:outline-none"
      inputmode="numeric"
      maxlength="3"
      value={text}
      oninput={(e) => typed(e.currentTarget.value)}
    />
  </div>
  {#if isDir}
    <label class="mt-4 flex items-start gap-2.5 text-[13px]">
      <input type="checkbox" class="mt-0.5 accent-accent" bind:checked={recursive} />
      <span>Apply to everything inside<span class="mt-0.5 block text-xs text-subtle">Sets the same permissions on every file and folder in it.</span></span>
    </label>
  {/if}
  {#if error}<p role="alert" class="mt-3 text-xs text-warning">{error}</p>{/if}
  <div class="mt-5 flex justify-end gap-2">
    <button class="rounded-lg px-3 py-1.5 text-[13px] text-muted hover:bg-raised hover:text-fg" onclick={() => dialog.close()}>Cancel</button>
    <button class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-on-accent hover:bg-accent/90 disabled:opacity-60" disabled={busy} onclick={apply}>Apply</button>
  </div>
</dialog>
