# Changelog

All notable changes to `brama` are documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

See `docs/agents/changelog.md` for how entries are written.

## [Unreleased]

### Added

- Add `brama anonymize check`, which validates the classification in `brama.yaml`
  and exits 42 when it does not hold together. It writes nothing and reaches no
  database, so it runs on a CI runner with no route to production. (#50, #7)
- Refuse a `fake.<generator>` naming a generator `brama` does not have, listing the
  ones it does. (#50, #7)
- Refuse `correlate` beside `keep` or `drop`, where it decides nothing: real values
  already correlate, and a dropped value has nothing to relate. (#50, #7)
- Refuse a correlation group with one member — the guard against `correlate:
  custmer` quietly becoming a group of its own — and suggest the near-miss group
  name when there is one. (#50, #7)
- Refuse an approval of a column that is not classified `keep`, or that nothing
  classifies at all. Approval only means something where real values would
  otherwise be sent. (#50, #7)
- Check every environment's approvals by default, and narrow to one with
  `brama anonymize check --env <name>`. There is no `--ci` flag: `--json` is the
  machine-readable contract. (#50, #7)
- Classify a column in `brama.yaml` with `action`, which is `fake.<generator>`,
  `keep`, or `drop`. `fake` on its own is refused: naming the generator is part of
  the decision. (#48, #7)
- Add `correlate` beside `action`, which names the group a column shares one
  fabricated identity with, so the joins between its members survive. (#48, #7)
- Add `environments.<name>.anonymize.approved`, a list of `table.column` entries
  naming the columns that environment may receive as real data. Approval is per
  destination: approved for staging is not approved for local. (#48, #7)
- Add `anonymize.tables.<table>.columns`, so a table that classifies by
  `discriminator` and `keys` can classify its ordinary columns in the same entry.
  Previously it could do one or the other, leaving a column such as
  `wp_usermeta.umeta_id` nowhere to be classified. (#48, #7)
- Add `anonymize.tables.<table>.value`, naming the column holding the value a
  `discriminator` selects for. (#48, #7)
- Reject the shorthand `email: fake.email` where a column is expected, naming the
  column and the `action:` line that replaces it. A column is always an object. (#48)
- Add `make testenv-up`, which starts two containerised servers running real `sshd`
  for `brama server add` and the shim install to be tested against. (#41)
- Publish `brama` for `linux` and `darwin`, on `amd64` and `arm64`, as an archive per
  release. Each release carries a checksum file, an SBOM per archive, and a build
  provenance attestation, so a download can be traced to the commit and the workflow
  that produced it: `gh attestation verify <file> --repo bramaos/brama`.
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
