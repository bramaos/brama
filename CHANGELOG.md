# Changelog

All notable changes to `brama` are documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

See `docs/agents/changelog.md` for how entries are written.

## [Unreleased]

### Added

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

- `brama` now exits `1` rather than `2` for an unknown command or a bad flag.

[unreleased]: https://github.com/bramaos/brama/commits/main
