import type { ITheme } from '@xterm/xterm'
import { terminalThemes, type Resolved } from './theme.svelte'

export interface Scheme {
  id: string
  name: string
  theme: ITheme
}

/** Builds a palette from the 16 ANSI colours (normal then bright). */
function ansi(
  base: {
    background: string
    foreground: string
    cursor: string
    selection: string
  },
  c: string[],
): ITheme {
  const names = ['black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white'] as const
  const theme: ITheme = {
    background: base.background,
    foreground: base.foreground,
    cursor: base.cursor,
    cursorAccent: base.background,
    selectionBackground: base.selection,
  }
  names.forEach((n, i) => {
    theme[n] = c[i]
    theme[`bright${n[0].toUpperCase()}${n.slice(1)}` as keyof ITheme] = c[i + 8] as never
  })
  return theme
}

const solarized = ['#073642', '#dc322f', '#859900', '#b58900', '#268bd2', '#d33682', '#2aa198', '#eee8d5', '#002b36', '#cb4b16', '#586e75', '#657b83', '#839496', '#6c71c4', '#93a1a1', '#fdf6e3']

/** Well-known terminal colour schemes. "auto" (follow the app theme) is handled by schemeTheme. */
export const schemes: Scheme[] = [
  { id: 'tuxshell-dark', name: 'TuxShell Dark', theme: terminalThemes.dark },
  { id: 'tuxshell-light', name: 'TuxShell Light', theme: terminalThemes.light },
  {
    id: 'solarized-dark',
    name: 'Solarized Dark',
    theme: ansi(
      {
        background: '#002b36',
        foreground: '#839496',
        cursor: '#93a1a1',
        selection: '#073642',
      },
      solarized,
    ),
  },
  {
    id: 'solarized-light',
    name: 'Solarized Light',
    theme: ansi(
      {
        background: '#fdf6e3',
        foreground: '#657b83',
        cursor: '#586e75',
        selection: '#eee8d5',
      },
      solarized,
    ),
  },
  {
    id: 'dracula',
    name: 'Dracula',
    theme: ansi(
      {
        background: '#282a36',
        foreground: '#f8f8f2',
        cursor: '#f8f8f2',
        selection: '#44475a',
      },
      ['#21222c', '#ff5555', '#50fa7b', '#f1fa8c', '#bd93f9', '#ff79c6', '#8be9fd', '#f8f8f2', '#6272a4', '#ff6e6e', '#69ff94', '#ffffa5', '#d6acff', '#ff92df', '#a4ffff', '#ffffff'],
    ),
  },
  {
    id: 'nord',
    name: 'Nord',
    theme: ansi(
      {
        background: '#2e3440',
        foreground: '#d8dee9',
        cursor: '#d8dee9',
        selection: '#434c5e',
      },
      ['#3b4252', '#bf616a', '#a3be8c', '#ebcb8b', '#81a1c1', '#b48ead', '#88c0d0', '#e5e9f0', '#4c566a', '#bf616a', '#a3be8c', '#ebcb8b', '#81a1c1', '#b48ead', '#8fbcbb', '#eceff4'],
    ),
  },
  {
    id: 'gruvbox-dark',
    name: 'Gruvbox Dark',
    theme: ansi(
      {
        background: '#282828',
        foreground: '#ebdbb2',
        cursor: '#ebdbb2',
        selection: '#504945',
      },
      ['#282828', '#cc241d', '#98971a', '#d79921', '#458588', '#b16286', '#689d6a', '#a89984', '#928374', '#fb4934', '#b8bb26', '#fabd2f', '#83a598', '#d3869b', '#8ec07c', '#ebdbb2'],
    ),
  },
  {
    id: 'one-dark',
    name: 'One Dark',
    theme: ansi(
      {
        background: '#282c34',
        foreground: '#abb2bf',
        cursor: '#528bff',
        selection: '#3e4451',
      },
      ['#3f4451', '#e06c75', '#98c379', '#e5c07b', '#61afef', '#c678dd', '#56b6c2', '#d7dae0', '#5c6370', '#e06c75', '#98c379', '#e5c07b', '#61afef', '#c678dd', '#56b6c2', '#ffffff'],
    ),
  },
  {
    id: 'tango-dark',
    name: 'Tango Dark',
    theme: ansi(
      {
        background: '#2e3436',
        foreground: '#d3d7cf',
        cursor: '#d3d7cf',
        selection: '#555753',
      },
      ['#2e3436', '#cc0000', '#4e9a06', '#c4a000', '#3465a4', '#75507b', '#06989a', '#d3d7cf', '#555753', '#ef2929', '#8ae234', '#fce94f', '#729fcf', '#ad7fa8', '#34e2e2', '#eeeeec'],
    ),
  },
]

/** The xterm palette for a scheme id; "auto" or an unknown id follows the app theme. */
export function schemeTheme(id: string, resolved: Resolved): ITheme {
  return schemes.find((s) => s.id === id)?.theme ?? terminalThemes[resolved]
}

/** A background to paint around the terminal, or undefined to keep the panel colour. */
export function schemeBackground(id: string): string | undefined {
  return schemes.find((s) => s.id === id)?.theme.background
}
