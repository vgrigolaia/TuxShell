<script lang="ts">
  import { onMount } from 'svelte'
  import * as SSHConfig from '../../../bindings/github.com/vgrigolaia/tuxshell/internal/sshconfig/service'

  interface Props {
    hostId: number
    hostName: string
    initialUser: string
    onSubmit: (user: string, save: boolean) => void
    error?: string | null
  }
  let { hostId, hostName, initialUser, onSubmit, error = null }: Props = $props()

  // Unique per pane: two tabs can show a prompt for the same host.
  const uid = $props.id()

  // svelte-ignore state_referenced_locally
  let user = $state(initialUser)
  let save = $state(true)
  let input: HTMLInputElement

  onMount(() => {
    input.focus()
    // Prefill from ssh_config's User line, if any, unless something was typed.
    if (!initialUser) {
      void SSHConfig.SuggestUser(hostId).then((u) => {
        if (u && !user) user = u
      })
    }
  })
</script>

<div class="absolute inset-0 z-10 grid place-items-center bg-panel/90">
  <form
    class="w-80 rounded-2xl border border-line bg-raised p-5 shadow-2xl"
    aria-labelledby="login-title-{uid}"
    onsubmit={(e) => {
      e.preventDefault()
      if (user.trim()) onSubmit(user.trim(), save)
    }}
  >
    <h2 id="login-title-{uid}" class="font-display text-[15px] font-semibold">Connect to {hostName}</h2>
    <p class="mt-1 text-xs text-subtle">ssh will ask for the password in the terminal.</p>

    <label class="mt-4 mb-1 block text-xs text-muted" for="login-user-{uid}">Username</label>
    <input
      id="login-user-{uid}"
      bind:this={input}
      bind:value={user}
      class="w-full rounded-lg border border-line bg-ground px-3 py-2 font-mono text-[13px] placeholder:text-subtle focus:border-accent focus:outline-none aria-[invalid=true]:border-warning"
      placeholder="root"
      autocomplete="username"
      spellcheck="false"
      aria-invalid={!!error}
      aria-describedby={error ? `login-error-${uid}` : undefined}
    />
    {#if error}<p id="login-error-{uid}" class="mt-1 text-xs text-warning">{error}</p>{/if}

    <label class="mt-3 flex items-center gap-2 text-xs text-muted">
      <input type="checkbox" class="size-4 accent-accent" bind:checked={save} />
      Save username to this host
    </label>

    <button
      type="submit"
      class="mt-4 w-full rounded-lg bg-accent px-3 py-2 text-[13px] font-medium text-on-accent hover:bg-accent/90 disabled:opacity-50"
      disabled={!user.trim()}
    >
      Connect
    </button>
  </form>
</div>
