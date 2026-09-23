# Changelog

All notable changes to `brama` are documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

See `docs/agents/changelog.md` for how entries are written.

## [Unreleased]

### Added

- Classify discriminator values by prefix: a `keys` entry ending in `*` — `_transient_*`
  — covers every key starting with what comes before it, so a new transient hash no
  longer leaves a key unclassified. An entry naming the key exactly wins, then the
  longest prefix. `anonymize check` refuses a `*` anywhere else in a key and names the
  prefix to write. (#67, #8)
- Classify the rows with no discriminator value, NULL or empty, under the key `""`. (#67, #8)
- Approve a prefix entry as written: `wp_options.option_name=_transient_*`. An approval
  names the entry, not a key under it; `check` refuses `…=_transient_abc` and names the
  prefix to approve instead. (#67, #8)
- Match the discriminator keys that carry `$table_prefix` against your project's own
  prefix too. WordPress writes `$table_prefix` + `capabilities` into `usermeta`, so an
  install on `acme_` holds `acme_capabilities`, and the preset classifies it there rather
  than leaving it unclassified and refusing the pull. The prefix is still read out of the
  project on every run and never written to `brama.yaml`. (#68, #8)
- Match the `wordpress` preset against your project's own `$table_prefix`, so an install
  on `acme_` gets the preset an install on `wp_` always got. The prefix is read out of
  the project's own config — `wp-config.php`, or `.env` and `config/application.php` on
  Bedrock — every time the preset is resolved, and is never declared in `brama.yaml`,
  where it would be a second copy of the truth and the one nobody updates. (#58, #7)
- Refuse a prefix brama cannot read, naming the files it looked in and the preset waiting
  on the answer. It never falls back to `wp_`: a preset applied under a guessed prefix
  does not classify nothing — it classifies whatever table sorted into the accounts
  table's place. (#58, #7)
- Approve one discriminator value, in `environments.<name>.anonymize.approved`:
  `wp_usermeta.meta_key=admin_color` beside the `table.column` form. A key is classified
  one at a time, so it is approved one at a time — approving the column holding its value
  would approve every other key at once, which is why that is still refused. The
  interactive review offers discriminator values like any other pending keep. (#59, #7)
- Answer the pending half in a terminal. `brama anonymize review` now puts what only you
  can decide as a checklist — one per destination environment, one line per column — and
  the question it asks is "do you approve exposing this column as real data?" rather than
  "keep this column?". Nothing starts ticked. (#57, #7)
- See what declining writes before you decide it: each line shows the classification that
  stands if you leave it unticked, so nothing is a leap. A column no generator claims says
  so, and is left for you. (#57, #7)
- Tick a line and the approval is recorded under that destination's
  `environments.<name>.anonymize.approved` and no other. Leave it unticked and the
  classification the line showed is written to `anonymize.tables` — unless another
  destination approves the same column, which leaves the `keep` standing and sends the
  fabricated value to the destination that declined. That holds for a destination this
  run never asked about, so `--env local` cannot revoke an approval staging already had.
  (#57, #7)
- Decide a preset's keeps once: they arrive as a single line naming how many columns it
  is about, openable to the full list before you answer it. A hundred checkboxes before
  anybody has pulled anything makes accept-all the only realistic answer, which authorizes
  exactly as blindly as trusting the preset would have. (#57, #7)
- Accept or hold a preset's loosenings as their own question, asked after the
  destinations and never mixed into one of their lists. (#57, #7)
- `--non-interactive`, on every command, for a person who wants the run an agent gets. It
  refuses to ask rather than skipping a guardrail: the answer brama takes when nobody
  answers is always the narrower one. `--json`, a pipe, and no terminal do the same. (#57)
- Reconcile a classification with `brama anonymize review`, the one command that writes
  one. With nobody at the keyboard it does the half of the job that is not a decision and
  hands back the half that is, which is the whole of what a CI runner or an agent gets.
  (#56, #7)
- Apply and write down what narrows what leaves production: the columns a preset now
  classifies more strictly than your file does, and the columns a migration added that a
  generator declares a claim on. Neither needs anybody's approval, and both are listed so
  that nothing lands invisibly. The comments, the key order and every line the change is
  not about survive the write, and a run with nothing to apply writes no bytes at all.
  (#56, #7)
- Hand back what widens it, applying none of it: the columns a preset would loosen, and
  the kept columns a destination approves nothing for. Each is named, and the run exits
  `review_required` at 42 — nothing went wrong, and the work left is work only you can
  do. A run with nothing waiting exits 0. (#56, #7)
- Read both lanes in `--json` under `applied_preset_tightenings`, `applied_new_columns`,
  `pending_preset_loosenings` and `pending_keeps`, with `reason` carrying the exit. All
  four keys are always present, and an empty list means nothing was found rather than
  nothing was looked for. (#56, #7)
- Resolve what each environment would actually receive, from its
  `environments.<name>.anonymize.approved` list read together with the classification. An
  approval counts for that environment and no other, so the same `keep` column can send
  real values to staging and fabricated ones to a laptop. (#55, #7)
- Send a kept column the destination has not approved as the value its generator would
  have produced, rather than as the real one. A pull can only ever expose less than the
  file suggests, never more. (#55, #7)
- Refuse instead, with reason `no_fallback` and exit code 42, where no generator claims
  the column: emptying a column brama could not name is a decision only a human makes, and
  the message names the column and the three ways out of it. (#55, #7)
- Report both in `brama anonymize check`, per environment and listed apart — the kept
  columns a generator stands in for, and the ones that would stop a pull. `--env` narrows
  the report to one destination, and a run with no `--env` covers every one. A run that
  finds a column with no fallback is reported as `partial`. (#55, #7)
- Name them in `--json` under `keep_substituted` and `keep_no_fallback`. Both keys are
  always present, and an empty list means every kept column resolves to what the file
  already says. (#55, #7)
- Apply a preset's answer over your own where the preset is the stricter of the two, so
  a preset brama tightens in a later release protects a column in every project that
  names it — including the projects whose `brama.yaml` still says `keep`. (#54, #7)
- Hold a preset's answer where it is the looser of the two. A preset that starts keeping
  a column your file fabricates never widens what leaves production on its own: your
  file's answer stands until `brama anonymize review` accepts the change. (#54, #7)
- Report both in `brama anonymize check`, listed apart — the columns brama is already
  anonymizing more strictly than the file says, and the ones it is holding — so neither
  reads as the other. A run that finds either is reported as `partial`. (#54, #7)
- Name the drifted columns in `--json` under `preset_drift_applied` and
  `preset_drift_held`, so a CI job can report which column changed instead of only that
  something did. Both keys are always present, and an empty list means the preset and
  your file agree. (#54, #7)
- Accept an approval of a column a preset has since tightened, rather than refusing the
  run over it. Approving a column your file keeps is not a mistake, and a tightening that
  stopped the pull it exists to make safe would be the wrong way round — `brama anonymize
  check` says the approval sends nothing while the tightening is applied. (#54, #7)
- Compare a preset against your file only where your file has an answer. A project that
  has recorded no classification has nothing for the preset to disagree with, and every
  preset answer applies as written. (#54, #7)
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
  the preset. An override that would send more real data than the preset does is held
  for review rather than applied. (#53, #7, #54)
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
- `brama anonymize check` now names the keyed form to write when an approval names a
  discriminator or the column it selects for, instead of only refusing it. It also
  reports a keyed approval naming a key nothing classifies, the wrong discriminator, or
  a table with no discriminator at all. (#59, #7)

### Fixed

- Stop `brama anonymize check` counting a preset table the database does not have. Where
  a schema was read, the counts are of the tables that schema has, so a project the
  preset half-fits no longer reads as one with twelve tables classified. With no schema
  in reach nothing narrows the count, because nothing knows what is there. (#58)
- Report a failed write instead of finishing successfully, so a closed pipe or a full
  disk is no longer silent.
- Let a stock WordPress project reach exit 0 on `brama anonymize review`. The eighteen
  `wp_usermeta` keys the wordpress preset classifies `keep` could not be approved or
  declined by anybody, so the run exited 42 on a project where every decision had been
  made. (#59, #7)

### Security

- Update `golang.org/x/text` to 0.39.0, closing an infinite loop on malformed input
  that `brama` could reach while rendering styled output (GO-2026-5970).

[unreleased]: https://github.com/bramaos/brama/commits/main
