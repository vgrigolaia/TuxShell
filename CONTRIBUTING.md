# Contributing to TuxShell

Thanks for helping. Bug reports, ideas and pull requests are all welcome.

## Reporting a problem

Open an [issue](https://github.com/vgrigolaia/TuxShell/issues) with:

- your TuxShell version (Preferences → About) and distribution,
- what you did, what you expected and what happened,
- a screenshot if it's about the UI.

Please don't paste passwords, private keys or real host names you'd rather keep private.
For a security problem, don't open a public issue: email vgrigolaia@outlook.com instead.

## Before you start on a pull request

For anything bigger than a small fix, open an issue first so we can agree on the approach.
It saves you work if the change doesn't fit the project.

## Contributor License Agreement

Before a pull request can be merged, you need to sign the
[Contributor License Agreement](CLA.md) once. A bot asks you to when you open your first
pull request; signing takes one click.

Why: TuxShell is GPL-3.0-or-later and stays free and open source. The CLA lets the project
keep the option to offer the same code under other terms too (for example to a company that
can't use GPL software), without asking every contributor again. You keep the copyright in
your contribution and can use it however you like.

## Working on the code

See [Build from source](README.md#build-from-source) for the requirements, then:

```sh
task tools   # once: installs the pinned golangci-lint and gitleaks
task dev     # run the app
task lint    # golangci-lint, secret scan, svelte-check and a frontend build
task test    # Go tests
```

Both `task lint` and `task test` must pass. If gitleaks flags a fake secret in a test, end
that line with a `// gitleaks:allow` comment.

Some ground rules for the code:

- **ssh**: TuxShell runs the system `ssh` and doesn't implement SSH itself. ssh commands are
  built only in Go, never from the frontend. Values are validated (no leading `-`) and `--`
  goes before the destination.
- **Secrets**: never log them, never send them to the frontend, and never write them anywhere
  except the system keyring.
- **UI**: use the theme tokens (no hard-coded colours), real buttons and labels, and an
  `aria-label` on icon-only buttons. Check both the dark and the light theme.
- **Dependencies**: ask in the issue before adding one, and say why it's needed.

## Commits and branches

- Branch from `develop` as `feature/<name>` (or `fix/<name>`), and open the pull request
  against `develop`. `main` only holds releases.
- Use [Conventional Commits](https://www.conventionalcommits.org): `feat:`, `fix:`, `docs:`,
  `refactor:`, `test:`, `chore:`, `ci:`.
- Keep commits small and focused.

## Name and logo

Forks are welcome under a different name. See [TRADEMARKS.md](TRADEMARKS.md).
