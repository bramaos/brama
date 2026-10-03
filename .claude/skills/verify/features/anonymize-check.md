# Anonymize check

`brama anonymize check` reads the Classification in `brama.yaml` (and the project's table
prefix) and says whether a Pull could act on it. It changes nothing. A Classification that
is missing, contradicts itself or names an unknown Generator is refused with exit `42`, and
so is any column or key the schema has and the Classification does not, where a schema is
read.

## Sub-features

- `check-clean` accepts a consistent Classification and reports its counts.
- `check-unclassified` refuses a `brama.yaml` with no `anonymize` block.
- `check-invalid` refuses unknown Generators and one-column Correlation groups, listing
  every problem.
- `check-keep-substitution` reports `keep` columns that a non-approving environment
  replaces with a fake.
- `check-human` renders the same Result for a person.

## How to get to it (user POV)

- Run `brama anonymize check` in a project with a `brama.yaml`.
- Add `--env <name>` to check one environment's Approvals only.

## Driving it with verify.sh

Preconditions:

- `verify.sh doctor` passes.
- A fresh sandbox per bullet.

- **Clean.** `S=$($V sandbox classified)`, then
  `$V run "$S" check --expect 0 -- --json anonymize check`. `status` `partial`
  (no schema read offline), `tables` `2`, `columns` `3`, `correlation_groups` `1`,
  no `unclassified_columns` key, `keep_substituted` `["local: users.display_name keep → fake.full_name"]`;
  `brama.yaml (unchanged)`.
- **Approved keep.** `S=$($V sandbox approved)`, same command. `keep_substituted` `[]`.
- **Unclassified.** `S=$($V sandbox initialized)`, then
  `$V run "$S" check --expect 42 -- --json anonymize check`. `status` `refused`, `reason`
  `unclassified`, `fix` `brama anonymize init`; `brama.yaml (unchanged)`.
- **Invalid.** `S=$($V sandbox classified)`, `sed -i 's/fake.email/fake.nonsense/' "$S/brama.yaml"`,
  then `$V run "$S" check-invalid --expect 42 -- --json anonymize check`. `reason`
  `invalid_classification`, `detail` lists every problem (the unknown Generator, each with
  the known names); `brama.yaml (unchanged)`.
- **Human.** `S=$($V sandbox classified)`, then `$V run "$S" check-human --expect 0 -- anonymize check`.
  stdout is a field table (`File`, `Environments`, `Preset`, `Tables`, …), not JSON.

## Gotchas

- Offline `status` is `partial`, not `success`, and `preset` is `null`: no schema is
  read, so `schema_columns` is `0`. Only a rig run proves preset matching.
- Removing a column from a two-column Correlation group makes it a group of one, which
  is itself invalid. Edit fixtures with that in mind.
- `check` needs `wp-config.php` with `$table_prefix`; every `wordpress`-based fixture has
  it. A sandbox without it is a different case (the prefix is unknown).
