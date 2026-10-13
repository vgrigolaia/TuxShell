# Changelog

All notable changes to TuxShell are listed here. Versions follow
[Semantic Versioning](https://semver.org).

## [1.1.1] - 2026-10-13

### Changed

- New app icon: a prompt whose cursor line runs on to two hosts. The title bar logo matches it.

### Fixed

- Files did nothing on Home. It now opens on this computer, in your home folder.

## [1.1.0] - 2026-10-13

### Added

- New look for the main window, after the design concepts in `docs/design/`.
- Home page with hosts, snippets and tunnels. Double-click Hosts to open the host list
  beside your sessions.
- Host status: ping shows which hosts are up, slow or down (ICMP only, can be turned off).
- Test button in the host form that tries a login and explains what went wrong.
- Snippets: saved commands you paste into a terminal from Ctrl+K.
- Tunnels: local port forwards and SOCKS proxies, bound to 127.0.0.1. Password-only hosts
  can reuse an open terminal's connection.
- Quick connect filters (hosts, snippets, tunnels, actions) and open in a split.
- Clipboard history shows which host each clip came from.

### Changed

- TuxShell opens on Home instead of a local shell, and closing the last tab returns there.
- Clipboard history opens as a floating panel from a small icon in the title bar.

## [1.0.0] - 2026-10-13

First release.

### Added

- Saved hosts with groups, tags, jump hosts and identity files, stored in SQLite.
- Import from `~/.ssh/config`, with usernames optional.
- Username prompt when connecting to a host without a saved user, with an option to save it.
- Bulk select and delete of hosts.
- Quick connect (Ctrl+K) with fuzzy search and `user@host:port` targets.
- Tabs with close all, close others and close selected.
- Split view with up to four panes per tab; merge, detach and maximize panes.
- Broadcast typing to the panes of a tab, with per-pane exclusion.
- Shared SSH connections (OpenSSH ControlMaster), so splits and the file browser reuse a login.
- File browser panel: browse, download, upload by picker or drag-and-drop.
- Encrypted clipboard history with pinned clips and configurable retention.
- Preferences: fonts, spacing, weights, cursor, colour schemes, scrollback, copy on select,
  right-click paste, bell, close confirmation and local shell.
- Terminal right-click menu with copy, paste, set title, splits, read only, scrollbar and zoom.
- Ctrl+mouse wheel and Ctrl+= / Ctrl+- / Ctrl+0 text zoom.
- Dark and light themes, or follow the system.
- Packages for Fedora/RHEL (.rpm), Debian/Ubuntu (.deb) and an AppImage.

[1.1.1]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.1.1
[1.1.0]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.1.0
[1.0.0]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.0.0
