import * as OnePassword from '../../bindings/github.com/vgrigolaia/tuxshell/internal/onepassword/service'
import type { Status } from '../../bindings/github.com/vgrigolaia/tuxshell/internal/onepassword/models'

/** Whether the 1Password CLI and SSH agent are there (never any secret). */
class OnePasswordStatus {
  status = $state<Status | null>(null)

  async refresh() {
    try {
      this.status = await OnePassword.Status()
    } catch {
      // keep the last status
    }
  }
}

export const onePassword = new OnePasswordStatus()
