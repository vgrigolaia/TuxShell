/** Short messages at the bottom of the window that go away by themselves. */
class Notices {
  list = $state<{ id: number; text: string }[]>([])
  private next = 1

  show(text: string, ms = 7000) {
    const id = this.next++
    this.list.push({ id, text })
    setTimeout(() => this.dismiss(id), ms)
  }

  dismiss(id: number) {
    this.list = this.list.filter((n) => n.id !== id)
  }
}

export const notices = new Notices()
