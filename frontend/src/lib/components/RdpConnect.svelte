<script lang="ts">
  import { onMount } from 'svelte'
  import { prettyFingerprint, remoteDesktops, type CertProblem } from '../rdp.svelte'
  import { hostStore, type Host } from '../hosts.svelte'
  import { prefs } from '../settings.svelte'
  import HostBadge from './HostBadge.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    host: Host
    onClose: () => void
    /** Opens the remote desktop in a TuxShell tab (beta). */
    onOpenTab: (host: Host) => void
  }
  let { host, onClose, onOpenTab }: Props = $props()

  let dialog: HTMLDialogElement
  let passwordInput = $state<HTMLInputElement>()
  let password = $state('')
  let show = $state(false)
  let remember = $state(false)
  let busy = $state(false)
  let error = $state('')
  let cert = $state<CertProblem | null>(null)
  let missing = $state('')
  const id = $props.id()

  // A saved password (while saving is on) is used in its own window; tabs
  // always ask, since a saved password never leaves Go. Nothing starts until
  // Connect is pressed, so the window/tab choice comes first.
  const inTab = $derived(remoteDesktops.mode === 'tab')
  // Go reads the password from 1Password or the keyring; a tab never gets it.
  const fromVault = $derived(!!host.passwordRef && !inTab)
  const useSaved = $derived(fromVault || (host.passwordSaved && prefs.value.savePasswords && !inTab))
  const target = $derived(`${host.user ? host.user + '@' : ''}${host.hostname}${host.port !== 3389 ? ':' + host.port : ''}`)
  let connectButton = $state<HTMLButtonElement>()

  onMount(() => {
    dialog.showModal()
    void remoteDesktops.checkClient().then((c) => {
      if (c && !c.installed && !inTab) missing = c.hint
      else if (useSaved) connectButton?.focus()
      else passwordInput?.focus()
    })
  })

  async function connect(trust = '') {
    busy = true
    error = ''
    if (inTab) {
      try {
        const res = await remoteDesktops.openTab(host, trust)
        busy = false
        if ('cert' in res) {
          cert = res.cert
          return
        }
        remoteDesktops.giveTab(host.id, res, password)
        password = ''
        onOpenTab(host)
        dialog.close()
      } catch (e) {
        busy = false
        cert = null
        error = e instanceof Error ? e.message : String(e)
      }
      return
    }
    const res = await remoteDesktops.connect(host, useSaved ? '' : password, remember && !useSaved, trust)
    busy = false
    if (res.ok) {
      password = ''
      if (remember) void hostStore.load()
      dialog.close()
    } else if ('cert' in res) {
      cert = res.cert
    } else {
      cert = null
      error = res.error
    }
  }

  const dateFmt = new Intl.DateTimeFormat(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
</script>

<dialog
  bind:this={dialog}
  aria-labelledby="{id}-title"
  class="m-auto w-[460px] rounded-2xl border border-line bg-panel p-0 text-fg shadow-2xl backdrop:bg-black/60"
  onclose={() => {
    password = ''
    onClose()
  }}
>
  <header class="flex items-center gap-3 border-b border-line px-5 py-4">
    <HostBadge name={host.name} os={host.os} size={36} />
    <div class="min-w-0 flex-1">
      <h2 id="{id}-title" class="truncate font-display text-[15px] font-semibold">{host.name}</h2>
      <p class="truncate font-mono text-xs text-subtle">Remote desktop · {target}{host.proxyJump ? ` via ${host.proxyJump}` : ''}</p>
    </div>
    <button class="grid size-7 place-items-center rounded-lg text-muted hover:bg-raised hover:text-fg" aria-label="Close" onclick={() => dialog.close()}>
      <Icon name="x" size={14} />
    </button>
  </header>

  <div class="px-5 pt-4">
    <div class="flex gap-1 rounded-[10px] bg-ground p-1" role="radiogroup" aria-label="Open in">
      {#each [['window', 'Own window'], ['tab', 'Tab (beta)']] as const as [m, name] (m)}
        <button
          type="button"
          role="radio"
          aria-checked={remoteDesktops.mode === m}
          class="h-8 flex-1 rounded-lg text-xs font-semibold {remoteDesktops.mode === m ? 'bg-panel text-fg shadow-sm' : 'text-muted hover:text-fg'}"
          onclick={() => {
            remoteDesktops.setMode(m)
            missing = ''
            cert = null
            error = ''
            if (m === 'tab') setTimeout(() => passwordInput?.focus())
          }}>{name}</button
        >
      {/each}
    </div>
    {#if inTab}<p class="mt-2 text-[11px] text-subtle">Drawn inside TuxShell. Copying text and files works (drop files on it, or Send files); shared folders, sound and saved passwords only work in its own window for now.</p>{/if}
  </div>
  <div class="px-5 py-4">
    {#if missing}
      <p class="text-[13px] text-muted">Remote desktops open with FreeRDP, which isn't installed.</p>
      <p class="mt-2 rounded-lg bg-raised px-3 py-2 font-mono text-xs break-words">{missing}</p>
    {:else if cert}
      {#if cert.kind === 'changed'}
        <div role="alert" class="rounded-xl border border-danger/50 bg-danger/10 px-3.5 py-3">
          <p class="flex items-center gap-2 text-[13px] font-semibold text-danger-text"><Icon name="shield" size={14} />The server's certificate changed</p>
          <p class="mt-1.5 text-xs text-text">
            This isn't the certificate you trusted before. That happens when the server's certificate is renewed or Windows is reinstalled, but it can also mean someone is
            intercepting the connection. Continue only if you expected this.
          </p>
        </div>
      {:else}
        <p class="text-[13px] text-muted">First connection to this computer. Check that this is its certificate; TuxShell remembers it and warns you if it ever changes.</p>
      {/if}
      <dl class="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-xs">
        <dt class="text-subtle">Name</dt>
        <dd class="truncate">{cert.cert.subject}{cert.cert.selfSigned ? ' (self-signed, the Windows default)' : ''}</dd>
        {#if !cert.cert.selfSigned}<dt class="text-subtle">Issued by</dt><dd class="truncate">{cert.cert.issuer}</dd>{/if}
        <dt class="text-subtle">Valid until</dt>
        <dd>{dateFmt.format(new Date(cert.cert.notAfter))}</dd>
        <dt class="text-subtle">SHA-256</dt>
        <dd class="font-mono text-[10.5px] leading-relaxed break-all">{prettyFingerprint(cert.cert.fingerprint)}</dd>
        {#if cert.previous}
          <dt class="text-subtle">Before</dt>
          <dd class="font-mono text-[10.5px] leading-relaxed break-all text-subtle">{prettyFingerprint(cert.previous)}</dd>
        {/if}
      </dl>
    {:else if useSaved}
      {#if fromVault}
        <p class="text-[13px] text-muted">{busy ? 'Getting the password from 1Password…' : 'Uses the password from 1Password, which may ask you to approve. Press Connect to start.'}</p>
      {:else}
        <p class="text-[13px] text-muted">{busy ? 'Connecting with the saved password…' : 'Uses the password saved in your keyring. Press Connect to start.'}</p>
      {/if}
    {:else}
      <form
        onsubmit={(e) => {
          e.preventDefault()
          void connect()
        }}
      >
        <label class="mb-1.5 block text-xs font-semibold text-text" for="{id}-pw">Password{host.user ? ` for ${host.rdp.domain ? host.rdp.domain + '\\' : ''}${host.user}` : ''}</label>
        <div class="flex items-center gap-1 rounded-[11px] border border-strong bg-field pr-1 focus-within:border-accent">
          <input
            bind:this={passwordInput}
            id="{id}-pw"
            class="h-10 min-w-0 flex-1 bg-transparent px-3 text-[13.5px] focus:outline-none"
            type={show ? 'text' : 'password'}
            autocomplete="off"
            spellcheck="false"
            bind:value={password}
          />
          <button type="button" class="grid size-8 place-items-center rounded-lg text-muted hover:text-fg" aria-label={show ? 'Hide password' : 'Show password'} onclick={() => (show = !show)}>
            <Icon name={show ? 'eyeOff' : 'eye'} size={15} />
          </button>
        </div>
        {#if prefs.value.savePasswords}
          <label class="mt-3 flex items-center gap-2 text-[13px]">
            <input type="checkbox" class="accent-accent" bind:checked={remember} />Remember for this host
          </label>
        {:else}
          <p class="mt-2 text-xs text-subtle">To skip this next time, turn on "Allow saving passwords" in Preferences → Security.</p>
        {/if}
      </form>
    {/if}

    {#if error}<p role="alert" class="mt-3 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">{error}</p>{/if}
  </div>

  <footer class="flex justify-end gap-2 border-t border-line px-5 py-3">
    <button class="rounded-lg px-3 py-1.5 text-[13px] text-muted hover:bg-raised hover:text-fg" onclick={() => dialog.close()}>Cancel</button>
    {#if missing}
      <!-- nothing to connect with -->
    {:else if cert}
      <button
        class="rounded-lg px-3 py-1.5 text-[13px] font-medium text-white disabled:opacity-60 {cert.kind === 'changed' ? 'bg-danger hover:bg-danger/90' : 'bg-accent text-on-accent hover:bg-accent/90'}"
        disabled={busy}
        onclick={() => connect(cert!.cert.fingerprint)}>{busy ? 'Connecting…' : cert.kind === 'changed' ? 'Trust the new certificate' : 'Trust and connect'}</button
      >
    {:else}
      <button bind:this={connectButton} class="rounded-lg bg-accent px-3 py-1.5 text-[13px] font-medium text-on-accent hover:bg-accent/90 disabled:opacity-60" disabled={busy} onclick={() => connect()}>
        {busy ? 'Connecting…' : error ? 'Try again' : 'Connect'}
      </button>
    {/if}
  </footer>
</dialog>
