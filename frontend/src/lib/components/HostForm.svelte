<script lang="ts">
  import { onMount } from 'svelte'
  import { defaultRDP, errorMessage, hostStore, type Host, type SaveError } from '../hosts.svelte'
  import * as RDP from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/rdp/service'
  import * as Status from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/status/service'
  import * as OSDetect from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/osdetect/service'
  import HostBadge from './HostBadge.svelte'
  import { OSES } from '../os'
  import Icon from './Icon.svelte'
  import * as Login from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/login/service'
  import * as OnePassword from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/onepassword/service'
  import * as KeePassXC from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/keepassxc/service'
  import { logins } from '../logins.svelte'
  import { prefs } from '../settings.svelte'

  interface Props {
    host: Host
    onClose: () => void
    onSaved: (host: Host) => void
    onConnect?: (host: Host) => void
  }
  let { host, onClose, onSaved, onConnect }: Props = $props()

  // Edit a copy so Cancel discards changes.
  // svelte-ignore state_referenced_locally
  let draft = $state<Host>({
    ...host,
    protocol: host.protocol || 'ssh',
    tags: [...(host.tags ?? [])],
    rdp: {
      ...defaultRDP(),
      ...host.rdp,
      folders: (host.rdp?.folders ?? []).map((f) => ({ ...f })),
      gateway: { ...defaultRDP().gateway, ...host.rdp?.gateway },
      usb: (host.rdp?.usb ?? []).map((d) => ({ ...d })),
      remoteApp: { ...defaultRDP().remoteApp, ...host.rdp?.remoteApp },
    },
  })
  const isRDP = $derived(draft.protocol === 'rdp')

  function setProtocol(p: 'ssh' | 'rdp') {
    if (p === draft.protocol) return
    draft.protocol = p
    // Move the port along with the default for the protocol.
    if (p === 'rdp' && (!draft.port || Number(draft.port) === 22)) draft.port = 3389
    if (p === 'ssh' && Number(draft.port) === 3389) draft.port = 22
  }

  async function addFolder() {
    try {
      const path = await RDP.ChooseFolder()
      if (!path) return
      const base = (path.split('/').filter(Boolean).pop() ?? 'Folder').replace(/[^\p{L}\p{N} _.-]/gu, '').slice(0, 32) || 'Folder'
      let name = base
      for (let i = 2; draft.rdp.folders?.some((f) => f.name.toLowerCase() === name.toLowerCase()); i++) name = `${base.slice(0, 29)} ${i}`
      draft.rdp.folders = [...(draft.rdp.folders ?? []), { name, path }]
    } catch (e) {
      error = { message: errorMessage(e) }
    }
  }
  // svelte-ignore state_referenced_locally
  let tagText = $state((host.tags ?? []).join(', '))
  let error = $state<SaveError | null>(null)
  let saving = $state(false)
  let confirmDelete = $state(false)
  // A new password to save with the host; never read back from Go.
  let password = $state('')
  let showPassword = $state(false)
  // svelte-ignore state_referenced_locally
  let passwordSaved = $state(host.passwordSaved)
  let changingPassword = $state(false)
  let nameInput: HTMLInputElement
  let formEl: HTMLFormElement

  const isNew = $derived(draft.id === 0)
  const jumpCandidates = $derived(hostStore.list.filter((h) => h.id !== draft.id && h.protocol !== 'rdp').map((h) => h.alias || h.hostname))

  // New hosts start typing in Name; picking an existing host shouldn't steal focus.
  onMount(() => {
    if (isNew) nameInput.focus()
  })

  let testing = $state(false)
  let tested = $state<{ ok: boolean; message: string } | null>(null)

  // Logs in with the values as typed (not saved) and reports the result.
  async function test() {
    testing = true
    tested = null
    error = null
    try {
      const r = await Status.Test({ ...draft, port: Number(draft.port) || 0, tags: [] })
      tested = { ok: r.ok, message: r.ok ? `${r.message} · ${Math.round(r.ms)} ms` : r.message }
    } catch (e) {
      const cause = (e as { cause?: SaveError })?.cause
      if (cause?.field) error = cause
      else tested = { ok: false, message: errorMessage(e) }
    } finally {
      testing = false
    }
  }

  // Only when clicked: reads /etc/os-release through an open session to the
  // saved host, or else the ssh server's greeting (no login).
  let detecting = $state(false)
  let detected = $state<{ ok: boolean; message: string } | null>(null)
  async function detectOS() {
    detecting = true
    detected = null
    try {
      const r = await OSDetect.Detect(draft.id)
      if (r.os) {
        draft.os = r.os
        const from = r.source === 'os-release' ? 'from /etc/os-release' : `from the SSH greeting (${r.detail})`
        const name = r.source === 'os-release' && r.detail ? r.detail : (OSES[r.os]?.label ?? r.os)
        detected = { ok: true, message: `${name}, ${from}. Save to keep it.` }
      } else detected = { ok: false, message: r.detail && r.source === 'os-release' ? `${r.detail}: ${r.message}` : r.message }
    } catch (e) {
      detected = { ok: false, message: errorMessage(e) }
    } finally {
      detecting = false
    }
  }

  // USB devices plugged in here, listed when "Add a USB device" is opened.
  let usbChoices = $state<{ id: string; name: string }[] | null>(null)
  async function listUSB() {
    usbChoices = (await RDP.USBDevices()) ?? []
  }
  function toggleUSB(d: { id: string; name: string }, on: boolean) {
    const rest = (draft.rdp.usb ?? []).filter((x) => x.id !== d.id)
    draft.rdp.usb = on ? [...rest, { ...d }] : rest
  }

  // Asks 1Password or KeePassXC for the reference; only "worked" or the reason comes back.
  let checking = $state(false)
  let checked = $state<{ ok: boolean; message: string } | null>(null)
  const isKeePassXC = $derived(draft.passwordRef.trim().startsWith('keepassxc:'))
  async function checkRef() {
    checking = true
    checked = null
    try {
      const kpxc = isKeePassXC
      const reason = kpxc ? await KeePassXC.Check(draft.passwordRef) : await OnePassword.Check(draft.passwordRef)
      checked = reason ? { ok: false, message: reason } : { ok: true, message: `${kpxc ? 'KeePassXC' : '1Password'} returned a password for this reference.` }
    } catch (e) {
      checked = { ok: false, message: errorMessage(e) }
    } finally {
      checking = false
    }
  }

  /** keepassxc:ssh://user@host[:port] for this host: the URL to give its KeePassXC entry. */
  function keepassxcRef(): string {
    const scheme = isRDP ? 'rdp' : 'ssh'
    const host = draft.hostname.trim() || draft.alias.trim() || draft.name.trim()
    const port = Number(draft.port) || 0
    let at = host.includes(':') ? `[${host}]` : host
    if (port && port !== (isRDP ? 3389 : 22)) at += `:${port}`
    const user = draft.user.trim()
    return `keepassxc:${scheme}://${user ? encodeURIComponent(user) + '@' : ''}${at}`
  }

  function fieldError(field: string) {
    return error?.field === field ? error.message : null
  }

  async function save(connect: boolean) {
    saving = true
    error = null
    draft.tags = tagText.split(',').map((t) => t.trim()).filter(Boolean)
    draft.port = Number(draft.port) || 0
    try {
      const saved = await hostStore.save(draft)
      if (password) {
        try {
          await Login.SetPassword(saved.id, password)
          password = ''
          passwordSaved = true
          changingPassword = false
          void hostStore.load()
          void logins.refresh()
        } catch (e) {
          error = { message: `Host saved, but the password wasn't: ${errorMessage(e)}` }
          return
        }
      }
      onSaved(saved)
      if (connect) onConnect?.(saved)
    } catch (e) {
      error = e as SaveError
      if (error.field) document.getElementById(`host-${error.field}`)?.focus()
    } finally {
      saving = false
    }
  }

  async function removePassword() {
    try {
      await Login.ClearPassword(draft.id)
      passwordSaved = false
      void hostStore.load()
      void logins.refresh()
    } catch (e) {
      error = { message: errorMessage(e) }
    }
  }

  async function remove() {
    try {
      await hostStore.remove(draft.id)
      onClose()
    } catch (e) {
      error = { message: String((e as Error)?.message ?? e) }
    }
  }

  const input =
    'h-10 w-full rounded-[11px] border border-strong bg-field px-3 text-[13.5px] text-fg placeholder:text-subtle focus:border-accent focus:outline-none focus-visible:outline-none aria-[invalid=true]:border-warning'
  const label = 'mb-1.5 block text-xs font-semibold text-text'
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && formEl?.contains(document.activeElement) && onClose()} />

<aside
  aria-label={isNew ? 'New host' : `Edit ${host.name}`}
  class="flex w-[360px] shrink-0 flex-col overflow-hidden rounded-panel border border-line bg-panel"
>
  <form
    class="flex min-h-0 flex-1 flex-col"
    onsubmit={(e) => {
      e.preventDefault()
      void save(false)
    }}
    bind:this={formEl}
    novalidate
  >
    <div class="flex items-center gap-3 px-6 pt-6 pb-2">
      <HostBadge name={draft.name} os={draft.os} size={44} />
      <div class="min-w-0 flex-1">
        <h2 class="truncate font-display text-lg font-bold">{isNew ? 'New host' : host.name}</h2>
        <p class="truncate text-xs text-subtle">
          {draft.alias ? `from ~/.ssh/config · ${draft.alias}` : draft.group || (isNew ? (isRDP ? 'Opens in its own window with FreeRDP' : 'Saved hosts connect with your ssh') : isRDP ? 'Remote desktop' : 'Saved host')}
        </p>
      </div>
      <button type="button" class="grid size-7 place-items-center rounded-lg bg-hop text-muted hover:text-fg" aria-label="Close" onclick={onClose}>
        <Icon name="x" size={12} />
      </button>
    </div>

    <div class="flex flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-4">
      {#snippet field(id: string)}
        {@const msg = fieldError(id)}
        {#if msg}<p id="host-{id}-error" class="mt-1 text-xs text-warning">{msg}</p>{/if}
      {/snippet}

      {#snippet rdpOptions()}
        {@const r = draft.rdp}
        {@const box = 'flex min-w-0 flex-col gap-2.5 rounded-2xl border border-line bg-ground/60 p-4'}
        {@const head = 'flex items-center gap-2 text-[13px] font-bold'}
        {@const seg = (on: boolean) => `h-8 flex-1 rounded-lg text-xs font-semibold ${on ? 'bg-panel text-fg shadow-sm' : 'text-muted hover:text-fg'}`}
        {@const check = 'flex items-start gap-2.5 text-[13px]'}
        <fieldset class={box}>
          <legend class="sr-only">Display</legend>
          <p class={head} aria-hidden="true"><span class="text-muted"><Icon name="monitor" size={14} /></span>Display</p>
          <div class="flex gap-1 rounded-[10px] bg-ground p-1" role="radiogroup" aria-label="Window">
            <button type="button" role="radio" aria-checked={r.window === 'window'} class={seg(r.window === 'window')} onclick={() => (r.window = 'window')}>Window</button>
            <button type="button" role="radio" aria-checked={r.window === 'fullscreen'} class={seg(r.window === 'fullscreen')} onclick={() => (r.window = 'fullscreen')}>Fullscreen</button>
          </div>
          <label class={check}>
            <input type="radio" class="mt-0.5 accent-accent" name="rdp-size" checked={r.size === 'fit'} onchange={() => (r.size = 'fit')} />
            <span>Fit the window<span class="mt-0.5 block text-[11.5px] text-subtle">The remote desktop resizes when you resize the window.</span></span>
          </label>
          <label class={check}>
            <input type="radio" class="mt-0.5 accent-accent" name="rdp-size" checked={r.size === 'fixed'} onchange={() => (r.size = 'fixed')} />
            <span class="flex flex-1 flex-wrap items-center gap-1.5">Fixed size
              {#if r.size === 'fixed'}
                <input class="h-7 w-16 rounded-md border border-strong bg-field px-1.5 text-center text-xs" type="number" min="200" max="8192" aria-label="Width" bind:value={r.width} />×
                <input class="h-7 w-16 rounded-md border border-strong bg-field px-1.5 text-center text-xs" type="number" min="200" max="8192" aria-label="Height" bind:value={r.height} />
              {/if}
            </span>
          </label>
          {@render field('rdp.size')}
          {#if r.window === 'fullscreen'}
            <label class={check}><input type="checkbox" class="mt-0.5 accent-accent" bind:checked={r.multimon} /><span>Use all monitors</span></label>
          {/if}
        </fieldset>

        <fieldset class={box}>
          <legend class="sr-only">Sharing</legend>
          <p class={head} aria-hidden="true"><span class="text-muted"><Icon name="folder" size={14} /></span>Sharing</p>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" bind:checked={r.clipboard} />
            <span>Copy and paste<span class="mt-0.5 block text-[11.5px] text-subtle">Text and files, both ways.</span></span>
          </label>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" bind:checked={r.homeFolder} />
            <span>Share my home folder</span>
          </label>
          {#if r.folders?.length}
            <ul class="flex flex-col gap-1.5" aria-label="Shared folders">
              {#each r.folders as f, i (i)}
                <li class="flex items-center gap-2">
                  <input class="h-8 w-28 shrink-0 rounded-lg border border-strong bg-field px-2 text-xs" aria-label="Name the remote computer sees" bind:value={f.name} maxlength="32" />
                  <span class="min-w-0 flex-1 truncate font-mono text-[11px] text-subtle" title={f.path}>{f.path}</span>
                  <button type="button" class="grid size-7 shrink-0 place-items-center rounded-lg text-subtle hover:bg-raised hover:text-fg" aria-label="Stop sharing {f.name}"
                    onclick={() => (r.folders = (r.folders ?? []).filter((_, j) => j !== i))}><Icon name="x" size={12} /></button>
                </li>
              {/each}
            </ul>
          {/if}
          <button type="button" class="self-start rounded-lg border border-strong px-2.5 py-1.5 text-xs font-semibold hover:bg-raised" onclick={addFolder}>+ Share a folder…</button>
          {@render field('rdp.folders')}
          <p class="text-[11.5px] leading-normal text-subtle">Shared folders show under This PC on the remote computer, which can read and change the files in them.</p>
        </fieldset>

        <fieldset class={box}>
          <legend class="sr-only">Devices</legend>
          <p class={head} aria-hidden="true"><span class="text-muted"><Icon name="server" size={14} /></span>Devices</p>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" bind:checked={r.printers} />
            <span>Share my printers<span class="mt-0.5 block text-[11.5px] text-subtle">The printers set up on this computer.</span></span>
          </label>
          <label class={check}>
            <input type="checkbox" class="mt-0.5 accent-accent" bind:checked={r.smartCards} />
            <span>Use my smart card readers<span class="mt-0.5 block text-[11.5px] text-subtle">For smart card sign-in; needs the pcscd service running here.</span></span>
          </label>
          <p class="text-xs font-semibold text-text">USB devices <span class="font-normal text-subtle">(beta)</span></p>
          {#if r.usb?.length}
            <ul class="flex flex-col gap-1.5" aria-label="USB devices passed through">
              {#each r.usb as d (d.id)}
                <li class="flex items-center gap-2 text-xs">
                  <span class="min-w-0 flex-1 truncate">{d.name || 'USB device'} <span class="font-mono text-[11px] text-subtle">{d.id}</span></span>
                  <button type="button" class="grid size-7 shrink-0 place-items-center rounded-lg text-subtle hover:bg-raised hover:text-fg" aria-label="Stop passing {d.name || d.id} through"
                    onclick={() => toggleUSB(d, false)}><Icon name="x" size={12} /></button>
                </li>
              {/each}
            </ul>
          {/if}
          {#if usbChoices === null}
            <button type="button" class="self-start rounded-lg border border-strong px-2.5 py-1.5 text-xs font-semibold hover:bg-raised" onclick={listUSB}>+ Add a USB device…</button>
          {:else if usbChoices.length === 0}
            <p class="text-[11.5px] text-subtle">No USB devices found on this computer.</p>
          {:else}
            <ul class="flex max-h-40 flex-col gap-1 overflow-y-auto rounded-lg border border-line p-2" aria-label="USB devices on this computer">
              {#each usbChoices as d (d.id)}
                <li>
                  <label class="flex items-center gap-2 text-xs">
                    <input type="checkbox" class="accent-accent" checked={!!r.usb?.some((x) => x.id === d.id)} onchange={(e) => toggleUSB(d, e.currentTarget.checked)} />
                    <span class="min-w-0 flex-1 truncate">{d.name}</span><span class="font-mono text-[11px] text-subtle">{d.id}</span>
                  </label>
                </li>
              {/each}
            </ul>
          {/if}
          {@render field('rdp.usb')}
          <p class="text-[11.5px] leading-normal text-subtle">
            While connected, the remote computer uses the device instead of this one. FreeRDP may need permission to open it (udev rules). Devices, printers and smart cards work in
            the own-window mode, not in a tab.
          </p>
        </fieldset>

        <fieldset class={box}>
          <legend class="sr-only">Sound and quality</legend>
          <p class={head} aria-hidden="true"><span class="text-muted"><Icon name="gear" size={14} /></span>Sound and quality</p>
          <label class="text-xs font-semibold text-text" for="rdp-sound">Sound</label>
          <select id="rdp-sound" class={input} bind:value={r.sound}>
            <option value="local">Play on this computer</option>
            <option value="remote">Play on the remote computer</option>
            <option value="off">Off</option>
          </select>
          <label class={check}><input type="checkbox" class="mt-0.5 accent-accent" bind:checked={r.microphone} /><span>Use my microphone</span></label>
          <label class="text-xs font-semibold text-text" for="rdp-quality">Quality</label>
          <select id="rdp-quality" class={input} bind:value={r.quality}>
            <option value="best">Best (fast network)</option>
            <option value="balanced">Balanced</option>
            <option value="low">Low bandwidth (slow or mobile links)</option>
          </select>
        </fieldset>

        <details class="rounded-2xl border border-line bg-ground/60 px-4 py-3" open={!!r.gateway?.host}>
          <summary class="cursor-pointer text-[13px] font-bold">RD Gateway <span class="font-normal text-subtle">(optional)</span></summary>
          <div class="mt-3 grid grid-cols-3 gap-2.5">
            <div class="col-span-2">
              <label class={label} for="host-rdp.gateway">Gateway</label>
              <input id="host-rdp.gateway" class={input} bind:value={r.gateway.host} placeholder="gateway.example.com" autocomplete="off" spellcheck="false"
                aria-invalid={!!fieldError('rdp.gateway')} aria-describedby="host-rdp.gateway-error" />
            </div>
            <div>
              <label class={label} for="rdp-gw-port">Port</label>
              <input id="rdp-gw-port" class={input} type="number" min="1" max="65535" bind:value={r.gateway.port} />
            </div>
            <div class="col-span-2">
              <label class={label} for="rdp-gw-user">Gateway user</label>
              <input id="rdp-gw-user" class={input} bind:value={r.gateway.user} placeholder="Same as the host" autocomplete="off" spellcheck="false" />
            </div>
            <div>
              <label class={label} for="rdp-gw-domain">Domain</label>
              <input id="rdp-gw-domain" class={input} bind:value={r.gateway.domain} autocomplete="off" spellcheck="false" />
            </div>
            <div class="col-span-3">{@render field('rdp.gateway')}</div>
          </div>
          <p class="mt-2 text-[11.5px] leading-normal text-subtle">The gateway logs in with the host's password.</p>
        </details>

        <details class="rounded-2xl border border-line bg-ground/60 px-4 py-3" open={!!r.remoteApp?.program}>
          <summary class="cursor-pointer text-[13px] font-bold">One program only (RemoteApp) <span class="font-normal text-subtle">(optional)</span></summary>
          <div class="mt-3 flex flex-col gap-2.5">
            <div>
              <label class={label} for="host-rdp.remoteApp">Program</label>
              <input id="host-rdp.remoteApp" class={input} bind:value={r.remoteApp.program} placeholder="||notepad or C:\Program Files\App\app.exe" autocomplete="off" spellcheck="false"
                aria-invalid={!!fieldError('rdp.remoteApp')} aria-describedby="host-rdp.remoteApp-error" />
            </div>
            <div class="grid grid-cols-2 gap-2.5">
              <div>
                <label class={label} for="rdp-app-name">Window name</label>
                <input id="rdp-app-name" class={input} bind:value={r.remoteApp.name} autocomplete="off" spellcheck="false" />
              </div>
              <div>
                <label class={label} for="rdp-app-args">Arguments</label>
                <input id="rdp-app-args" class={input} bind:value={r.remoteApp.args} autocomplete="off" spellcheck="false" />
              </div>
            </div>
            {@render field('rdp.remoteApp')}
          </div>
          <p class="mt-2 text-[11.5px] leading-normal text-subtle">
            Opens just this program in its own window instead of the whole desktop. Use its alias (||name) or its full path; the server must allow it. Own-window mode only.
          </p>
        </details>
      {/snippet}

      <div class="grid grid-cols-2 gap-1 rounded-[13px] bg-ground p-1" role="radiogroup" aria-label="Connection type">
        {#each [['ssh', 'SSH terminal', 'terminal'], ['rdp', 'Remote desktop', 'monitor']] as const as [p, name, icon] (p)}
          <button
            type="button"
            role="radio"
            aria-checked={draft.protocol === p}
            class="flex h-9 items-center justify-center gap-2 rounded-[10px] text-[13px] font-semibold {draft.protocol === p ? 'bg-panel text-fg shadow-sm' : 'text-muted hover:text-fg'}"
            onclick={() => setProtocol(p)}
          >
            <Icon name={icon} size={14} />{name}
          </button>
        {/each}
      </div>

      <div class="grid grid-cols-3 gap-2.5">
        <div class="col-span-3">
          <label class={label} for="host-name">Name</label>
          <input id="host-name" class={input} bind:this={nameInput} bind:value={draft.name} placeholder="pg-primary"
            aria-invalid={!!fieldError('name')} aria-describedby="host-name-error" autocomplete="off" spellcheck="false" />
          {@render field('name')}
        </div>
        <div class="col-span-2">
          <label class={label} for="host-hostname">Host</label>
          <input id="host-hostname" class={input} bind:value={draft.hostname} placeholder={draft.alias || '10.0.0.11'}
            aria-invalid={!!fieldError('hostname')} aria-describedby="host-hostname-error" autocomplete="off" spellcheck="false" />
        </div>
        <div>
          <label class={label} for="host-port">Port</label>
          <input id="host-port" class={input} type="number" min="1" max="65535" bind:value={draft.port}
            aria-invalid={!!fieldError('port')} aria-describedby="host-port-error" />
        </div>
        <div class="col-span-3 -mt-1.5 empty:hidden">{@render field('hostname')}{@render field('port')}</div>
        <div class="col-span-3">
          <label class={label} for="host-user">User</label>
          <input id="host-user" class={input} bind:value={draft.user} placeholder={isRDP ? 'Administrator' : 'Asked when you connect'}
            aria-invalid={!!fieldError('user')} aria-describedby="host-user-error" autocomplete="off" spellcheck="false" />
          {@render field('user')}
        </div>
        {#if isRDP}
          <div class="col-span-3">
            <label class={label} for="host-rdp.domain">Domain <span class="font-normal text-subtle">(optional)</span></label>
            <input id="host-rdp.domain" class={input} bind:value={draft.rdp.domain} placeholder="CORP" autocomplete="off" spellcheck="false"
              aria-invalid={!!fieldError('rdp.domain')} aria-describedby="host-rdp.domain-error" />
            {@render field('rdp.domain')}
          </div>
        {/if}
        {#if prefs.value.savePasswords}
          <div class="col-span-3">
            <label class={label} for="host-password">Password <span class="font-normal text-subtle">(optional)</span></label>
            {#if passwordSaved && !changingPassword}
              <div class="flex h-9 items-center gap-2 text-[13px]">
                <span class="text-success">✓ Saved in your keyring</span>
                <span class="flex-1"></span>
                <button type="button" class="rounded-lg px-2 py-1 text-xs text-muted hover:bg-raised hover:text-fg" onclick={() => (changingPassword = true)}>Change</button>
                <button type="button" class="rounded-lg px-2 py-1 text-xs text-muted hover:bg-raised hover:text-fg" onclick={removePassword}>Remove</button>
              </div>
            {:else}
              <div class="relative">
                <input id="host-password" class="{input} pr-10 font-mono text-xs" type={showPassword ? 'text' : 'password'} bind:value={password}
                  placeholder={passwordSaved ? 'New password' : 'Leave empty to type it when connecting'} autocomplete="new-password" spellcheck="false"
                  aria-describedby="host-password-hint" />
                <button type="button" class="absolute top-1/2 right-2 grid size-7 -translate-y-1/2 place-items-center rounded-md text-subtle hover:text-fg"
                  aria-label={showPassword ? 'Hide password' : 'Show password'} aria-pressed={showPassword} onclick={() => (showPassword = !showPassword)}>
                  <Icon name={showPassword ? 'eyeOff' : 'eye'} size={14} />
                </button>
              </div>
            {/if}
            <span id="host-password-hint" class="mt-1 block text-[11.5px] text-subtle">Stored encrypted in your system keyring. Keys or 1Password are safer.</span>
          </div>
        {/if}
        <div class="col-span-3">
          <label class={label} for="host-proxyJump">{isRDP ? 'SSH jump host' : 'Jump host'} {#if isRDP}<span class="font-normal text-subtle">(optional)</span>{/if}</label>
          <input id="host-proxyJump" class={input} bind:value={draft.proxyJump} list="host-jumps" placeholder="bastion-01"
            aria-invalid={!!fieldError('proxyJump')} aria-describedby="host-proxyJump-error" autocomplete="off" spellcheck="false" />
          <datalist id="host-jumps">
            {#each jumpCandidates as j (j)}<option value={j}></option>{/each}
          </datalist>
          {@render field('proxyJump')}
          {#if isRDP}<span class="mt-1 block text-[11.5px] text-subtle">Reach the desktop through an SSH tunnel (the jump host needs a key or the agent).</span>{/if}
        </div>
        <div class="col-span-3">
          <label class={label} for="host-group">Group</label>
          <input id="host-group" class={input} bind:value={draft.group} list="host-groups" placeholder="PostgreSQL · prod" autocomplete="off" />
          <datalist id="host-groups">
            {#each hostStore.groupNames as g (g)}<option value={g}></option>{/each}
          </datalist>
        </div>
        {#if !isRDP}
        <div class="col-span-3">
          <label class={label} for="host-identityFile">Key file</label>
          <input id="host-identityFile" class="{input} font-mono text-xs" bind:value={draft.identityFile} placeholder="~/.ssh/id_ed25519"
            aria-invalid={!!fieldError('identityFile')} aria-describedby="host-identityFile-error" autocomplete="off" spellcheck="false" />
          {@render field('identityFile')}
        </div>
        {/if}
        <div class="col-span-3">
          <label class={label} for="host-tags">Tags</label>
          <input id="host-tags" class={input} bind:value={tagText} placeholder="patroni, leader"
            aria-invalid={!!fieldError('tags')} aria-describedby="host-tags-error" autocomplete="off" />
          {@render field('tags')}
        </div>
        <div class="col-span-3">
          <label class={label} for="host-os">Operating system <span class="font-normal text-subtle">(optional)</span></label>
          <div class="flex gap-2">
            <select id="host-os" class="{input} min-w-0 flex-1" bind:value={draft.os} aria-invalid={!!fieldError('os')} aria-describedby="host-os-error host-os-detected">
              <option value="">Not set</option>
              {#each Object.entries(OSES) as [value, o] (value)}<option {value}>{o.label}</option>{/each}
            </select>
            {#if !isRDP}
              <button
                type="button"
                class="h-10 shrink-0 rounded-[11px] border border-strong px-3 text-xs font-semibold hover:bg-raised disabled:opacity-50"
                disabled={isNew || detecting}
                title={isNew
                  ? 'Save the host first'
                  : 'With a session open to this host, runs “cat /etc/os-release” through it. Otherwise reads the ssh server’s greeting, without logging in.'}
                onclick={detectOS}>{detecting ? 'Detecting…' : 'Detect'}</button
              >
            {/if}
          </div>
          {@render field('os')}
          {#if detected}
            <p id="host-os-detected" role="status" class="mt-1 text-xs {detected.ok ? 'text-muted' : 'text-warning'}">{detected.message}</p>
          {/if}
        </div>
        {#if !isRDP}
        <div class="col-span-3">
          <label class={label} for="host-profile">Terminal profile</label>
          <select id="host-profile" class={input} bind:value={draft.profile} aria-invalid={!!fieldError('profile')} aria-describedby="host-profile-error host-profile-hint">
            <option value="">Default</option>
            {#each prefs.value.profiles ?? [] as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
          </select>
          <span id="host-profile-hint" class="mt-1 block text-[11.5px] text-subtle">Fonts, colours, header colour and a command to run on connect. Make profiles in Preferences → Profiles.</span>
          {@render field('profile')}
        </div>
        {/if}
      </div>

      {#if isRDP}
        {@render rdpOptions()}
      {/if}

      <fieldset class="flex flex-col gap-2.5 rounded-2xl border border-accent-line bg-accent-soft p-4">
        <legend class="sr-only">Password manager</legend>
        <p class="flex items-center gap-2 text-[13px] font-bold text-accent-ink" aria-hidden="true">
          <span class="text-accent-text"><Icon name="shield" size={15} /></span>Credentials from 1Password or KeePassXC
        </p>
        <div>
          <label class={label} for="host-passwordRef">Password reference</label>
          <div class="flex gap-2">
            <input id="host-passwordRef" class="{input} min-w-0 flex-1 font-mono text-xs" bind:value={draft.passwordRef} placeholder="op://Infra/pg-primary/password or keepassxc:ssh://user@host"
              oninput={() => (checked = null)}
              aria-invalid={!!fieldError('passwordRef')} aria-describedby="host-passwordRef-error host-passwordRef-hint" autocomplete="off" spellcheck="false" />
            <button type="button" class="shrink-0 rounded-[10px] border border-strong bg-panel px-3 text-xs font-semibold hover:bg-raised disabled:opacity-60"
              disabled={checking || !draft.passwordRef.trim()} onclick={checkRef}>
              {checking ? 'Checking…' : 'Check'}
            </button>
          </div>
          {@render field('passwordRef')}
          <button type="button" class="mt-1.5 text-xs font-medium text-accent-text underline-offset-2 hover:underline disabled:opacity-50"
            disabled={!(draft.hostname.trim() || draft.alias.trim() || draft.name.trim())}
            onclick={() => ((draft.passwordRef = keepassxcRef()), (checked = null))}>Use KeePassXC for this host</button>
        </div>
        {#if checked}
          <p role="status" class="text-xs {checked.ok ? 'text-success' : 'text-warning'}">{checked.ok ? '✓ ' : ''}{checked.message}</p>
        {/if}
        <p id="host-passwordRef-hint" class="text-[11.5px] leading-normal text-accent-ink/80">
          Only the reference is stored, never the value. When the server asks for a password, TuxShell gets it from
          {#if isKeePassXC}
            KeePassXC, which must be open and unlocked and may ask you to allow access. Give the KeePassXC entry the URL after
            <span class="font-mono">keepassxc:</span> (for example <span class="font-mono">{draft.passwordRef.trim().slice('keepassxc:'.length)}</span>); the
            user picks the entry when several share a URL. Connect TuxShell once in Preferences → Security.
          {:else}
            1Password, which may ask you to approve. In 1Password, copy it with <span class="font-medium">Copy Secret Reference</span>
            on the field. For KeePassXC, use the link above.
          {/if}{#if isRDP}{' '}Remote desktops in a tab still ask each time.{/if}
        </p>
      </fieldset>


      {#if tested}
        <p role="status" class="rounded-xl border px-3 py-2 text-xs {tested.ok ? 'border-success/40 bg-success/10 text-success' : 'border-warning/40 bg-warning/10 text-warning'}">
          {tested.ok ? '✓ ' : ''}{tested.message}
        </p>
      {/if}
      {#if error && !error.field}
        <p role="alert" class="rounded-xl border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">{error.message}</p>
      {/if}
    </div>

    <div class="flex flex-col gap-2.5 px-6 pt-2 pb-6">
      <div class="flex items-center gap-2 text-xs">
        {#if !isNew}
          {#if confirmDelete}
            <span class="text-muted">Delete this host?</span>
            <button type="button" class="rounded-lg bg-danger px-2.5 py-1 text-white hover:bg-danger/90" onclick={remove}>Delete</button>
            <button type="button" class="rounded-lg px-2.5 py-1 text-muted hover:bg-raised" onclick={() => (confirmDelete = false)}>Keep</button>
          {:else}
            <button type="button" class="rounded-lg px-2 py-1 text-muted hover:bg-raised hover:text-fg" onclick={() => (confirmDelete = true)}>Delete…</button>
          {/if}
        {/if}
        <span class="flex-1"></span>
        <button type="submit" disabled={saving} class="rounded-lg px-2.5 py-1 font-semibold text-accent-text hover:bg-raised disabled:opacity-60">Save</button>
      </div>
      <div class="flex gap-2.5">
        {#if !isRDP}
          <button type="button" disabled={testing} class="h-11 flex-1 rounded-[13px] border border-strong bg-panel text-sm font-semibold hover:bg-raised disabled:opacity-60" onclick={test}>
            {testing ? 'Testing…' : 'Test'}
          </button>
        {/if}
        {#if onConnect}
          <button type="button" disabled={saving} class="h-11 flex-[2] rounded-[13px] bg-fg text-sm font-bold text-panel hover:opacity-90 disabled:opacity-60" onclick={() => save(true)}>
            Save & connect
          </button>
        {/if}
      </div>
    </div>
  </form>
</aside>

