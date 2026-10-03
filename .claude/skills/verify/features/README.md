# brama verification map

The maintained source for verifying brama's user-facing commands. Read this index first,
then use the matching feature file as the recipe. Every command below is literal and runs
from the repo root.

## Baseline preconditions

- `.claude/skills/verify/verify.sh build` has run since the last source edit.
- `.claude/skills/verify/verify.sh doctor` prints no `FAIL` line.
- Each recipe starts from a fresh `verify.sh sandbox <fixture>`; never reuse a sandbox
  another recipe mutated.
- No Server, SSH or database is reachable or needed. Commands that need one are listed
  under "Not offline" and are not mapped.

## Driving conventions

- `V=.claude/skills/verify/verify.sh` in the recipes is shorthand; spell it out or set it.
- Run every command through `$V run "$S" <label> --expect <code> -- <args>`, so it leaves
  evidence and fails loudly on a wrong exit code.
- Pass `--json` to assert, `--non-interactive` on anything that may prompt.
- Exit codes: `0` done, `1` broke (or a partial result that is still a failure), `42`
  Refusal. A Refusal's JSON is `{"action", "status": "refused", "reason", "detail", "fix"?}`;
  an error's is `{"action", "status": "error", "detail"}`.
- Assert on JSON keys and the `brama.yaml` section of the evidence, not on prose.

## Proof and skip reporting

- Proof is the evidence file: command, exit code, stdout, and what happened to
  `brama.yaml`. List each path with the fact it proves.
- A read-only command or `--dry-run` must show `(unchanged)` or `(absent)`.
- Report an entry point you did not drive, and why. Never count it verified through a
  different entry point.

## Feature entry contract

Each feature file has an H1, one paragraph of user-visible behaviour, then four H2s in
order: `Sub-features`, `How to get to it (user POV)`, `Driving it with verify.sh`,
`Gotchas`. The map holds user paths, fixtures, commands and observable proof, never
implementation details.

## Features

- [Init](./init.md): `brama init` detects a WordPress project (vanilla or Bedrock) and
  writes a starting `brama.yaml`; `--dry-run`, `--adapter`, partial detection, existing file.
- [Anonymize check](./anonymize-check.md): `brama anonymize check` validates the
  Classification in `brama.yaml` without touching anything; clean, unclassified, invalid.
- [Anonymize review](./anonymize-review.md): `brama anonymize review` lists decisions
  still owed (pending keeps, unclassified keys) and applies automatic changes.
- `brama version`: `$V run "$S" version --expect 0 -- --json version` returns
  `{"action": "version", "status": "success", "version": "<git describe>"}` in any sandbox.

## Not offline

- `brama anonymize init` reads a database schema. Offline it exits `1` with
  `no environment is reachable`. Verify it against the rig in `test/testenv`.
- `brama server add` connects over SSH to a Server. Verify it against the rig.
