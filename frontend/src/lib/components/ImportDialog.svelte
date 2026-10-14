<script lang="ts">
  import { onMount } from 'svelte'
  import * as HostIO from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/hostio/service'
  import type { Preview } from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/hostio/models'
  import { errorMessage } from '../hosts.svelte'
  import Icon from './Icon.svelte'
  import ImportSshConfig from './ImportSshConfig.svelte'
  import ImportFile from './ImportFile.svelte'

  interface Props {
    /** Opens straight at the ~/.ssh/config list instead of the source list. */
    start?: 'sshconfig'
    onClose: () => void
  }
  let { start, onClose }: Props = $props()

  /** Where hosts can come from. "sshconfig" is read in place; the others open a file. */
  const sources: { id: string; title: string; detail: string }[] = [
    { id: 'sshconfig', title: '~/.ssh/config', detail: 'Hosts from your OpenSSH config' },
    { id: 'csv', title: 'CSV or TSV file', detail: 'A spreadsheet saved as CSV, from Excel, LibreOffice or another tool' },
    { id: 'json', title: 'TuxShell export (.json)', detail: 'A file made with Export in Preferences → Hosts' },
  ]

  let dialog: HTMLDialogElement
  // svelte-ignore state_referenced_locally
  let step = $state<'source' | 'sshconfig' | 'file'>(start ?? 'source')
  let preview = $state<Preview | null>(null)
  let error = $state<string | null>(null)
  let opening = $state(false)

  const titles = { source: 'Import hosts', sshconfig: 'Import from ~/.ssh/config', file: 'Import hosts from a file' }

  onMount(() => dialog.showModal())

  async function choose(id: string) {
    error = null
    if (id === 'sshconfig') {
      step = 'sshconfig'
      return
    }
    opening = true
    try {
      const p = await HostIO.Open(id)
      if (p) {
        preview = p
        step = 'file'
      }
    } catch (e) {
      error = errorMessage(e)
    } finally {
      opening = false
    }
  }

  function back() {
    void HostIO.Cancel()
    preview = null
    error = null
    step = 'source'
  }
</script>

<dialog
  bind:this={dialog}
  aria-labelledby="import-title"
  class="m-auto flex max-h-[85vh] flex-col overflow-hidden rounded-2xl border border-line bg-panel p-0 text-fg shadow-2xl backdrop:bg-black/60 {step === 'file' ? 'w-[820px]' : 'w-[560px]'} max-w-[calc(100vw-32px)]"
  onclose={() => {
    void HostIO.Cancel()
    onClose()
  }}
>
  <div class="flex items-center gap-3 border-b border-line px-5 py-4">
    {#if step !== 'source'}
      <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Back to import sources" onclick={back}>
        <span class="rotate-180"><Icon name="chevron" size={14} /></span>
      </button>
    {:else}
      <span class="grid size-8 place-items-center rounded-lg bg-accent/15 text-accent"><Icon name="import" size={16} /></span>
    {/if}
    <h2 id="import-title" class="flex-1 font-display text-[15px] font-semibold">{titles[step]}</h2>
    <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Close" onclick={() => dialog.close()}>
      <Icon name="x" size={14} />
    </button>
  </div>

  {#if step === 'sshconfig'}
    <ImportSshConfig close={() => dialog.close()} />
  {:else if step === 'file' && preview}
    <ImportFile {preview} close={() => dialog.close()} />
  {:else}
    <div class="min-h-0 flex-1 overflow-y-auto p-3">
      {#if error}
        <p role="alert" class="mb-2 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">{error}</p>
      {/if}
      <ul class="flex flex-col gap-1">
        {#each sources as s (s.id)}
          <li>
            <button class="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left hover:bg-raised disabled:opacity-60" disabled={opening} onclick={() => void choose(s.id)}>
              <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-raised text-muted"><Icon name={s.id === 'sshconfig' ? 'terminal' : 'file'} size={15} /></span>
              <span class="min-w-0 flex-1">
                <span class="block text-[13px] font-medium">{s.title}</span>
                <span class="block truncate text-xs text-subtle">{s.detail}</span>
              </span>
              <Icon name="chevron" size={14} />
            </button>
          </li>
        {/each}
      </ul>
      <p class="mt-3 px-3 text-xs text-subtle">Passwords in imported files are never stored. Only 1Password references (op://…) are kept.</p>
    </div>
  {/if}
</dialog>
