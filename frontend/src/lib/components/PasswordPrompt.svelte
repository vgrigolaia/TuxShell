<script lang="ts">
  import { onMount } from 'svelte'
  import { logins, type PasswordRequest } from '../logins.svelte'
  import { prefs } from '../settings.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    request: PasswordRequest
  }
  let { request }: Props = $props()

  const uid = $props.id()
  let password = $state('')
  let show = $state(false)
  let remember = $state(false)
  let input: HTMLInputElement

  const canRemember = $derived(request.canSave && prefs.value.savePasswords)

  onMount(() => input.focus())

  function submit() {
    const pw = password
    password = ''
    void logins.answer(request, pw, canRemember && remember)
  }
</script>

<div class="absolute inset-0 z-10 grid place-items-center bg-panel/90">
  <form
    class="w-80 rounded-2xl border border-line bg-raised p-5 shadow-2xl"
    aria-labelledby="pw-title-{uid}"
    onsubmit={(e) => {
      e.preventDefault()
      submit()
    }}
  >
    <h2 id="pw-title-{uid}" class="font-display text-[15px] font-semibold">Password for {request.hostName}</h2>
    <p class="mt-1 truncate font-mono text-[11px] text-subtle" title={request.prompt}>{request.prompt}</p>
    {#if request.savedFailed}
      <p role="alert" class="mt-3 rounded-lg border border-warning/40 bg-warning/10 px-2.5 py-1.5 text-xs text-warning">The saved password didn't work. Type the current one.</p>
    {/if}

    <label class="mt-4 mb-1 block text-xs text-muted" for="pw-input-{uid}">Password</label>
    <div class="relative">
      <input
        id="pw-input-{uid}"
        bind:this={input}
        bind:value={password}
        type={show ? 'text' : 'password'}
        class="w-full rounded-lg border border-line bg-ground py-2 pr-9 pl-3 font-mono text-[13px] focus:border-accent focus:outline-none"
        autocomplete="off"
        spellcheck="false"
        onkeydown={(e) => {
          if (e.key === 'Escape') {
            e.stopPropagation()
            void logins.cancel(request)
          }
        }}
      />
      <button
        type="button"
        class="absolute top-1/2 right-1.5 grid size-7 -translate-y-1/2 place-items-center rounded-md text-subtle hover:text-fg"
        aria-label={show ? 'Hide password' : 'Show password'}
        aria-pressed={show}
        onclick={() => (show = !show)}
      >
        <Icon name={show ? 'eyeOff' : 'eye'} size={14} />
      </button>
    </div>

    {#if canRemember}
      <label class="mt-3 flex items-start gap-2 text-xs text-muted">
        <input type="checkbox" class="mt-px size-4 accent-accent" bind:checked={remember} />
        <span>{request.savedFailed ? 'Replace the saved password' : 'Remember for this host'}<span class="block text-[11px] text-subtle">Saved encrypted in your system keyring.</span></span>
      </label>
    {/if}

    <div class="mt-4 flex gap-2">
      <button type="button" class="flex-1 rounded-lg border border-line px-3 py-2 text-[13px] text-muted hover:bg-panel hover:text-fg" onclick={() => void logins.cancel(request)}>Cancel</button>
      <button type="submit" class="flex-1 rounded-lg bg-accent px-3 py-2 text-[13px] font-medium text-on-accent hover:bg-accent/90">Connect</button>
    </div>
  </form>
</div>
