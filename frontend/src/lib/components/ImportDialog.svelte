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

  /** Where hosts can come from. "sshconfig" is read in place; the others go through HostIO.Open. */
  type Source = { id: string; title: string; detail: string }
  const sections: { label: string; sources: Source[] }[] = [
    {
      label: 'Files',
      sources: [
        { id: 'sshconfig', title: '~/.ssh/config', detail: 'Hosts from your OpenSSH config' },
        { id: 'csv', title: 'CSV or TSV file', detail: 'A spreadsheet saved as CSV, from Excel, LibreOffice or another tool' },
        { id: 'xlsx', title: 'Excel workbook (.xlsx)', detail: 'The first sheet, one host per row' },
        { id: 'json', title: 'TuxShell export (.json)', detail: 'A file made with Export in Preferences → Hosts' },
      ],
    },
    {
      label: 'Other apps',
      sources: [
        { id: 'mobaxterm', title: 'MobaXterm', detail: 'A session export (.mxtsessions): Settings → Export sessions' },
        { id: 'remmina', title: 'Remmina', detail: 'SSH and remote desktop (RDP) connections saved in Remmina' },
        { id: 'putty', title: 'PuTTY', detail: 'Sessions saved by PuTTY on this computer (~/.putty)' },
        { id: 'puttyreg', title: 'PuTTY on Windows', detail: 'A .reg export of the PuTTY Sessions registry key' },
        { id: 'ansible', title: 'Ansible inventory', detail: 'An INI inventory file; groups and host variables are read' },
      ],
    },
  ]

  let dialog: HTMLDialogElement
  // svelte-ignore state_referenced_locally
  let step = $state<'source' | 'sshconfig' | 'file'>(start ?? 'source')
  let preview = $state<Preview | null>(null)
  let error = $state<string | null>(null)
  let opening = $state(false)
  let templateSaved = $state('')

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

  async function saveTemplate() {
    error = null
    try {
      const path = await HostIO.SaveTemplate()
      if (path) templateSaved = path
    } catch (e) {
      error = errorMessage(e)
    }
  }

  function back() {
    void HostIO.Cancel()
    preview = null
    error = null
    step = 'source'
  }
</script>

<!-- The body sizes from its content (flex-auto) and scrolls once the dialog hits max-height.
     flex-1 (a zero base size) collapses to nothing in some WebKitGTK versions. -->
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
    <div class="min-h-0 flex-auto overflow-y-auto p-3">
      {#if error}
        <p role="alert" class="mb-2 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">{error}</p>
      {/if}
      {#each sections as sec (sec.label)}
        <h3 class="px-3 pt-2 pb-1 text-[11px] font-semibold tracking-wide text-subtle uppercase">{sec.label}</h3>
        <ul class="flex flex-col gap-0.5">
          {#each sec.sources as s (s.id)}
            <li>
              <button class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left hover:bg-raised disabled:opacity-60" disabled={opening} onclick={() => void choose(s.id)}>
                <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-raised text-muted"><Icon name={s.id === 'sshconfig' ? 'terminal' : sec.label === 'Files' ? 'file' : 'import'} size={15} /></span>
                <span class="min-w-0 flex-1">
                  <span class="block text-[13px] font-medium">{s.title}</span>
                  <span class="block truncate text-xs text-subtle">{s.detail}</span>
                </span>
                <Icon name="chevron" size={14} />
              </button>
            </li>
          {/each}
        </ul>
      {/each}
      <div class="mx-3 mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-line bg-ground px-3 py-2.5 text-xs">
        <span class="min-w-0 flex-1 text-muted">Starting a host list from scratch? Fill in our CSV template in Excel or LibreOffice, then import it as a CSV file.</span>
        <button class="shrink-0 rounded-md border border-strong px-2.5 py-1 font-medium hover:bg-raised" onclick={saveTemplate}>Download template</button>
        {#if templateSaved}<p role="status" class="w-full truncate text-success" title={templateSaved}>Saved to {templateSaved}</p>{/if}
      </div>
      <p class="mt-3 px-3 text-xs text-subtle">Passwords in files are saved only if you allow it in Preferences → Security, and then only in your system keyring.</p>
    </div>
  {/if}
</dialog>
