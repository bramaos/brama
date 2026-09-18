# Changelog

All notable changes to `brama` are documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

See `docs/agents/changelog.md` for how entries are written.

## [Unreleased]

### Added

- Classify a project with `anonymize.preset`, which names a classification brama
  ships for a framework it knows rather than spelling it out. `tables:` then holds
  only what is specific to your project and the overrides you meant, so the block
  stays short enough to read in a diff. (#53, #7)
- Ship a `wordpress` preset covering the twelve tables a default WordPress install
  creates — fabricating the account and comment-author details, dropping live
  password-reset tokens and login sessions, and keeping the site's own content and
  configuration. (#53, #7)
- Reference the preset rather than copying it, so a preset brama tightens in a later
  release reaches your project without anyone editing `brama.yaml`. (#53, #7)
- Let `anonymize.tables` override a preset one column at a time. Writing a column of
  a table the preset knows changes that column and leaves the rest of the table to
  the preset. (#53, #7)
- Write `preset: <name>` from `brama anonymize init` when brama ships one for your
  adapter, and leave the columns it already answers for out of the file. A preset is
  a deliberate statement about a table brama knows; a generator's name pattern is an
  inference, and the specific one wins. (#53, #7)
- Count the columns a preset classifies as classified in `brama anonymize check`,
  without expanding them into the file — so a check against a reachable environment
  can now report full column coverage on a project that names one. (#53, #7)
- Refuse an `anonymize.preset` naming a preset brama does not ship, listing the ones
  it does. A name that resolved to nothing would silently unclassify every column the
  preset was carrying. (#53, #7)
- Grant no approval from a preset. A preset says what a column holds and never who
  may receive it, so a column it classifies `keep` still sends real values nowhere
  until a human approves it for that environment. (#53, #7)
- Add `brama anonymize init`, which reads an environment's schema and writes the
  classification for every column a generator claims — by name and by type — as
  `action: fake.<generator>`. It is the file `brama init` deliberately does not write.
  (#52, #7)
- Leave every column no generator claims out of the file entirely, rather than
  defaulting it to `drop`. Dropping is safe about privacy and reckless about everything
  else, and zeroing `orders.total_amount` is a product decision brama has no standing to
  make about data it could not name. What is omitted stays unclassified, which already
  refuses a pull. (#52, #7)
- Never write `keep` from `anonymize init`. Keeping a column sends real production data,
  and that decision enters `brama.yaml` only where a human put it. (#52, #7)
- Report `brama anonymize init` as `partial` while anything is still unclassified, and
  name each column it left for you, so the first `brama anonymize review` has a list to
  work from. (#52, #7)
- Refuse to overwrite an anonymize block that already exists, pointing at
  `brama anonymize review` — which changes decisions one at a time instead of replacing
  reviewed ones wholesale. (#52, #7)
- Write the block into `brama.yaml` line by line, so comments, key order and the aligned
  comments `brama init` wrote survive the edit untouched. (#52, #7)
- Compare the classification against the schema of a reachable environment, and name
  every column the database has that `brama.yaml` says nothing about. Only a schema can
  say a column is there at all, so this is the half of `brama anonymize check` that a CI
  runner cannot answer. (#51, #7)
- Refuse a `fake.<generator>` that cannot fill the column it was given — `fake.email` on
  a `varchar(20)`, or on an `int` — while the file is still open in an editor, rather
  than partway through a dump on a production server. (#51, #7)
- Report `brama anonymize check` as `partial` rather than `success` when no environment
  was reachable, naming the column coverage it could not verify. A clean run against no
  schema is not a clean bill of health, and a caller reading only the status must not
  take one for the other. (#51, #7)
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
