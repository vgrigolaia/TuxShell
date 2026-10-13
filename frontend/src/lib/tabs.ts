/** What a terminal tab runs. Commands are built in Go from these. */
export type Source =
  | { kind: 'shell' }
  | { kind: 'host'; hostId: number; askUser?: boolean }
  | { kind: 'target'; target: string }

export interface Tab {
  key: number
  /** Tab label: host name, or the shell's title for local tabs. */
  title: string
  /** Latest title the terminal set (e.g. user@host:~), shown in the pane header. */
  termTitle: string
  source: Source
  exited: boolean
}
