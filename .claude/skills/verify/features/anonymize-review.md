# Anonymize review

`brama anonymize review` compares the Classification and Approvals in `brama.yaml` with
what a Pull would do. It writes the changes that need no one's decision, and lists the
decisions still owed. While any are owed it exits `42` with `reason` `review_required`.

## Sub-features

- `review-pending-keep` lists a `keep` column that an environment has not approved.
- `review-clean` exits `0` once every keep is approved.
- `review-invalid` refuses an invalid Classification and points at `check`.
- `review-write` applies automatic changes to `brama.yaml` (needs a schema; rig only).

## How to get to it (user POV)

- Run `brama anonymize review` after editing the Classification or pulling a newer brama.
- Add `--env <name>` to review one environment's Approvals only.
- Add `--non-interactive` (or `--json`) in scripts so it never asks.

## Driving it with verify.sh

Preconditions:

- `verify.sh doctor` passes.
- A fresh sandbox per bullet.

- **Pending keep.** `S=$($V sandbox classified)`, then
  `$V run "$S" review --expect 42 -- --json --non-interactive anonymize review`. `status`
  `partial`, `reason` `review_required`, `pending_decisions` `1`, `pending_keeps`
  `["local: users.display_name keep → fake.full_name"]`, `written` `false`;
  `brama.yaml (unchanged)`.
- **Clean.** `S=$($V sandbox approved)`, same command with `--expect 0`.
  `pending_decisions` `0`, `pending_keeps` `[]`; `brama.yaml (unchanged)`.
- **Invalid.** `S=$($V sandbox classified)`, `sed -i 's/fake.email/fake.nonsense/' "$S/brama.yaml"`,
  then `$V run "$S" review-invalid --expect 42 -- --json --non-interactive anonymize review`.
  `status` `refused`, `reason` `invalid_classification`, `fix` `brama anonymize check`;
  `brama.yaml (unchanged)`.

## Gotchas

- A pending review renders a full Result **and** exits `42`; its `status` is `partial`,
  not `refused`. Assert `reason` and `pending_decisions`, not `status` alone.
- Offline there is no schema, so no new columns or keys appear and `automatic_changes`
  stays `0`. The write path (`written: true`) needs the rig.
- Without `--non-interactive` (and without `--json`) review may prompt and wait.
