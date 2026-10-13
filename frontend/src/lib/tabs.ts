/** What a terminal pane runs. Commands are built in Go from these. */
export type Source =
  | { kind: 'shell' }
  | { kind: 'host'; hostId: number; askUser?: boolean; /** Login to use instead of the host's saved user. */ user?: string }
  | { kind: 'target'; target: string }

/** Right-click menu actions for a pane; App implements them. */
export interface PaneActions {
  /** Splits with the same connection; missing when the tab is full. */
  split?: (orientation: 'row' | 'column' | 'auto') => void
  splitOther?: () => void
  newTab: () => void
  rename: () => void
  close: () => void
  toggleMaximized: () => void
  toggleReadOnly: () => void
  toggleScrollbar: () => void
  preferences: () => void
}

/** What the title bar needs to draw a tab. */
export interface TabInfo {
  key: number
  title: string
  exited: boolean
  panes: number
}
