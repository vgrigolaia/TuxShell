# Changelog

All notable changes to TuxShell are listed here. Versions follow
[Semantic Versioning](https://semver.org).

## [Unreleased]

## [1.4.1] - 2026-10-20

### Added

- Remmina import brings the connections' passwords along: those Remmina keeps in the system
  keyring and those in its own files. They're saved in your keyring when "Allow saving
  passwords" is on, like passwords from other imports.
- For hosts you already have, the import can now "Only add their passwords", which leaves the
  hosts themselves as they are. It's the default when the file has passwords to save.
- Shortcuts can be changed: Preferences → Shortcuts → Change, then press the new keys. Each can
  also be turned off or reset. TuxShell warns when a combo is already used or when the shell
  uses it too, and keeps Ctrl+C, Ctrl+D and Ctrl+Z for the terminal.
- A tab closes by itself when you log out (exit or logout). If the connection fails or drops,
  it stays open so you can read why and reconnect. Turn it off in Preferences.

### Changed

- On Home, clicking a host card only selects it. Double-click or Enter connects, and the
  right-click menu (or the pencil on the card) edits it.

### Fixed

- After a Remmina import, TuxShell no longer suggests deleting the file because of passwords in
  plain text (Remmina's aren't).

## [1.4.0] - 2026-10-15

### Added

- macOS (beta): one universal app for Apple silicon and Intel (macOS 12 or newer), as a `.dmg`.
  It isn't notarized yet, so the first start needs System Settings → Privacy & Security →
  *Open Anyway*.
  - The window keeps macOS's own close, minimize and zoom buttons, with a standard menu bar.
  - The app's shortcuts use ⌘ (⌘K quick connect, ⇧⌘L hosts, ⌘C/⌘V in the terminal), so Ctrl
    stays with the terminal; labels and Preferences → Shortcuts show the ⌘ keys.
  - Files live in `~/Library/Application Support/TuxShell`; saved passwords go to the Keychain.
  - Homebrew tools are found (`op`, `sdl-freerdp`), and the 1Password SSH agent's macOS socket
    is used.

### Fixed

- Preferences shows the real settings and recordings folders on each system.

## [1.3.0] - 2026-10-15

### Added

- 1Password: a host can sign in with a 1Password reference (`op://Vault/Item/field`). When the
  server asks for the password, TuxShell gets it from 1Password through the `op` CLI (1Password
  may ask you to approve). The value is never stored, logged or shown, and it works with
  *Allow saving passwords* off. Remote desktops use it in their own window.
  - **Check** in the host form confirms a reference works.
  - The password box explains when 1Password couldn't give the password, or when the server
    refused it.
  - Preferences → Security shows whether the 1Password CLI is installed, with setup steps.
- *Use the 1Password SSH agent* (Preferences → Security, off by default): TuxShell's ssh uses the
  SSH keys in 1Password, also through jump hosts. Hosts whose `~/.ssh/config` picks an agent keep it.
- Home: hide the "Found hosts in ~/.ssh/config" reminder with its ×. It comes back only when new
  hosts appear; hidden hosts are still listed in Import.

### Fixed

- Leftover password-channel sockets from runs that didn't close cleanly are removed at startup.

## [1.2.1] - 2026-10-15

### Fixed

- Typing, switching tabs and resizing the window no longer lag while a terminal prints a lot
  (logs, builds, `top`). Output now reaches the screen once per frame instead of in thousands of
  tiny pieces.
- Changing a setting, such as hiding the hosts panel, no longer redraws every open terminal.
- Maximizing or restoring the window resizes each terminal once, when it settles, instead of on
  every frame of the animation.

## [1.2.0] - 2026-10-15

### Added

- Remote desktops (RDP): add Windows machines as hosts and open them in their own window
  (FreeRDP 3) or inside a TuxShell tab (beta, built in, nothing extra to install).
  - Own window: shared folders, copy and paste of text and files, sound and microphone,
    quality presets, RD Gateway, multiple monitors, an SSH jump host, and readable error
    messages. Packages recommend FreeRDP.
  - Tab: copy and paste of text and files (drop files on the desktop, or Send files, and
    save what Windows copied to Downloads), resizing with the tab and Ctrl+Alt+Del. The
    password is typed each time; shared folders and sound need the own window.
  - Server certificates are pinned on first use, with a clear warning if one changes.
  - Remmina connections import with their RDP settings; RDP hosts are kept in JSON and CSV
    exports.
- Open tabs in the left panel, above your hosts. With the panel hidden they become a strip of
  icons. Drag to reorder. Preferences → General → Open tabs switches back to the strip in the
  title bar.
- Drag host groups, and hosts within a group or into another group, to put them in your own
  order (also from the right-click menu and with Alt+↑/↓). The order is remembered.
- Import preview: select all or none, and search the list.
- "Don't ask again" when closing a tab, pane or window with running sessions.

### Changed

- "Allow saving passwords" and the startup update check are now on by default for new
  installs. A password is still saved only when you enter or import one, and the update check
  only links to the new release. Your existing settings are not changed.
- The saved-password field in the host form sits under the user name.

### Fixed

- The hosts list scrollbar no longer covers the group fold arrows.

## [1.1.9] - 2026-10-14

### Added

- Connection history under Home → History: which host, when, for how long and whether it
  worked, with search and one-click reconnect. Never commands or screen content. Can be
  turned off or limited in Preferences → General.
- Session recording: start and stop it from the terminal's right-click menu, or record
  automatically per profile. Only what the terminal shows is saved, never keystrokes, as
  standard asciinema files readable only by you. A REC badge shows while recording.
- A player for recordings under History → Recordings, with speed, seeking and long pauses
  skipped.

## [1.1.8] - 2026-10-14

### Added

- File panel: a right-click menu on files and folders with rename, new folder, delete and
  permissions. Ctrl+click selects several items. Deleting always asks first.
- Edit a file in your desktop's default app; when you save it there, upload it from the panel.
  TuxShell warns before overwriting a file that changed on the server, and keeps its permissions.
- Follow the terminal: the file panel can move with `cd` in the focused terminal, using the
  folder the server's prompt reports.

## [1.1.7] - 2026-10-14

### Added

- Terminal profiles: named sets of terminal settings, made in Preferences → Profiles and
  chosen per host in the host form. Local tabs and hosts without one use Default.
- Custom colour schemes (all 16 colours plus background, text, cursor and selection).
- A background image per profile, with a shade to keep text readable.
- A pane header colour per profile, for example red for production servers.
- Scrolling options: scroll on output or when typing, mouse wheel speed, scrollbar for new panes.
- Bell options: flash, a bell icon on the pane and tab, and a short sound.
- Run on connect: a command the server runs after login, optionally keeping a shell open.
- What the Backspace and Delete keys send, for older servers.

### Changed

- Preferences are grouped into terminal profile settings and app settings.

## [1.1.6] - 2026-10-14

### Added

- Optional saved passwords for people who don't use keys or 1Password. Turn on "Allow saving
  passwords" in Preferences → Security (off by default). Passwords are kept only in your
  system keyring, never in TuxShell's files, exports or logs, and are handed to ssh privately.
- When a server asks for a password, a password box appears over the terminal, with an
  option to remember it for that host.
- A saved password can be set, changed or removed in the host form. Home shows which hosts
  have one.
- Imported CSV/Excel passwords are saved to the keyring when saving passwords is on.

### Security

- A saved password is tried only once; if the server refuses it, you're asked instead.
- Jump host prompts, host key questions and key passphrases still appear in the terminal.

## [1.1.5] - 2026-10-14

### Fixed

- The import window opened empty on some systems (seen on a fresh Fedora install).
- GNOME Software and App Center showed a generic icon and no details for TuxShell. Packages
  now include AppStream metadata, PNG icons and, for Debian/Ubuntu, the license file.

### Changed

- Clearer package description.

## [1.1.4] - 2026-10-14

### Added

- Import hosts from CSV/TSV and Excel (.xlsx) files. Columns are matched from the header row
  and can be changed; every row is previewed, with problems and already-saved hosts shown.
- Import from MobaXterm, Remmina, PuTTY (~/.putty or a Windows .reg export) and Ansible INI
  inventories.
- A CSV template to fill in and import, from the import window or Preferences → Hosts.
- Export hosts as TuxShell JSON, CSV or ssh_config, in Preferences → Hosts. Exports never
  contain passwords and are readable only by you.
- Optional operating system per host, shown as its logo on host badges.
- A bastion in an imported file can be the name of another host; it's saved as its address.

### Changed

- The import window starts with a choice of source. ~/.ssh/config import works as before.
- Preferences has a new Hosts section.

## [1.1.3] - 2026-10-14

### Added

- About page in Preferences with the version, license and project links.
- Check for updates: asks GitHub for the latest release and links to it. An optional check at
  startup (off by default) shows an Update button in the title bar when a newer version is out.

### Fixed

- The dock and app switcher showed a generic icon instead of the TuxShell icon.

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

[1.4.1]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.4.1
[1.4.0]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.4.0
[1.3.0]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.3.0
[1.2.1]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.2.1
[1.2.0]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.2.0
[1.1.9]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.1.9
[1.1.8]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.1.8
[1.1.7]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.1.7
[1.1.6]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.1.6
[1.1.5]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.1.5
[1.1.4]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.1.4
[1.1.3]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.1.3
[1.1.1]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.1.1
[1.1.0]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.1.0
[1.0.0]: https://github.com/vgrigolaia/tuxshell/releases/tag/v1.0.0
