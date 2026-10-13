import * as SnippetService from '../../bindings/github.com/vgrigolaia/tuxshell/internal/hosts/snippetservice'
import type { Snippet } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/hosts/models'
import { errorMessage } from './hosts.svelte'

export type { Snippet }

export const emptySnippet = (): Snippet => ({ id: 0, name: '', command: '', group: '' })

/** Saved commands, pasted into terminals from quick connect or the Snippets page. */
class Snippets {
  list = $state<Snippet[]>([])
  loaded = $state(false)
  error = $state<string | null>(null)

  async load() {
    try {
      this.list = (await SnippetService.List()) ?? []
      this.error = null
    } catch (e) {
      this.error = errorMessage(e)
    } finally {
      this.loaded = true
    }
  }

  /** Saves a snippet; throws the backend error (with cause.field) on failure. */
  async save(sn: Snippet): Promise<Snippet> {
    const saved = await SnippetService.Save(sn)
    await this.load()
    return saved
  }

  async remove(id: number) {
    await SnippetService.Delete(id)
    await this.load()
  }
}

export const snippets = new Snippets()
