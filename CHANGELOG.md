# Changelog

All notable changes to `brama` are documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

See `docs/agents/changelog.md` for how entries are written.

## [Unreleased]

### Added

- Upgrade the shim on a server to the version of `brama` reaching it, as a step of
  its own before anything else runs, never partway through. (#4)
- Name both versions when the shim moves — `shim upgraded 0.1.0 → 0.1.1` — so the
  step says what changed and not only that something did. (#4)
- Replace a shim `brama` is not newer than instead of stopping, which is what a
  server another developer reached with a newer `brama` leaves behind. (#4)
- Add `--dry-run` to `brama server add`, which reports the shim it would install or
  upgrade and writes nothing, on the server or in `brama.yaml`. It still connects:
  which shim a server already runs cannot be answered from `brama.yaml`. (#4)
- Add `brama server add <name> --host <host> [--user <user>]`, which registers a
  server and installs the shim on it. Authentication is OpenSSH's: `--host` is passed
  through untouched, so it may be a hostname, an IP, or a `~/.ssh/config` alias, and
  identity comes from ssh-agent. brama stores no credentials. (#2)
- Add the shim, installed at `~/.brama/shim` on a server. Builds for `linux/amd64`
  and `linux/arm64` are embedded in `brama` and streamed down the connection already
  open, so nothing is fetched over the server's own network. This build answers
  `--version`. (#2, #3)
- Add `brama version`, which prints the version the binary was built from.
- Add `brama init`, which detects the adapter and writes a starting `brama.yaml`. It
  writes no credentials and no classification — `brama anonymize init` owns the
  `anonymize` block, and until it runs every column is unclassified. (#1)
- Add the `brama.yaml` schema: `version`, `app.adapter`, `app.paths`, `environments`,
  `servers`, and `anonymize`. Unknown keys are rejected rather than ignored, so a
  typo cannot silently mean "no classification". (#1)
- Add `--json` and `--dry-run` to `brama init`. `--dry-run` shows the file it would
  write and writes nothing.
- Add `--adapter` to `brama init`, to name the adapter when detection cannot.
- Add `brama --version`, alongside the existing `brama version` subcommand.
- Add `--help` for every command, with generated manpages and shell completions.

### Changed

- Add `shim_change`, `shim_previous_version` and `dry_run` to `brama server add
  --json`. `shim_version` is now the version the server runs when the command
  finishes, which under `--dry-run` is the one it was already running. (#4)
- `brama` now exits `1` rather than `2` for an unknown command or a bad flag.
- Error messages now name the step that failed: `reading brama.yaml: permission
  denied` rather than `permission denied`.

### Fixed

- Report a failed write instead of finishing successfully, so a closed pipe or a full
  disk is no longer silent.

### Security

- Update `golang.org/x/text` to 0.39.0, closing an infinite loop on malformed input
  that `brama` could reach while rendering styled output (GO-2026-5970).

[unreleased]: https://github.com/bramaos/brama/commits/main
