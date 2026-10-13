/** What a terminal pane runs. Commands are built in Go from these. */
export type Source =
  | { kind: 'shell' }
  | { kind: 'host'; hostId: number; askUser?: boolean }
  | { kind: 'target'; target: string }

/** What the title bar needs to draw a tab. */
export interface TabInfo {
  key: number
  title: string
  exited: boolean
  panes: number
}
