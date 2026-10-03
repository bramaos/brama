---
name: verify
description: Drive the real brama CLI binary against throwaway WordPress projects and capture evidence (command, exit code, JSON, brama.yaml diff). Use before committing a change to a brama command, to prove its behaviour the way a user runs it rather than through tests, or when asked to verify, reproduce or demo a brama command.
---

# Verify brama

brama is a CLI. A user runs it in a project directory; it reads that project and its
`brama.yaml`, answers with a rendered Result (or `--json`), and exits `0` (done), `1`
(broke) or `42` (Refusal: brama declined to act; not a failure). Proving a change means
running the built binary in a real project directory and reading what it printed, what it
exited and what it wrote.

Everything goes through `verify.sh` beside this file. Run it from the repo root:

```bash
.claude/skills/verify/verify.sh build
.claude/skills/verify/verify.sh doctor
S=$(.claude/skills/verify/verify.sh sandbox classified)
.claude/skills/verify/verify.sh run "$S" check --expect 0 -- --json anonymize check
.claude/skills/verify/verify.sh cleanup
```

`make check` proves the code. This proves the command. Do both.

## Scope

Offline only: no Server, SSH or database. What that covers and what it doesn't:

| Command | Offline | Notes |
|---|---|---|
| `version` | yes | |
| `init` | yes | WordPress (vanilla, Bedrock) detection, `--dry-run`, `--adapter` |
| `anonymize check` | yes | reads `brama.yaml` + `wp-config.php` only; no schema is read |
| `anonymize review` | yes | without a schema it reviews keeps and approvals only |
| `anonymize init` | **no** | needs a database; exits 1 "no environment is reachable" offline |
| `server add` | **no** | needs SSH to a Server |

For the last two, use the container rig (`make testenv-up`, `make testenv-seed`,
`make testenv-test`; see `test/testenv`) and say in the report that the offline driver
could not reach them. Never report them verified from an offline run.

## Launch

There is no server to keep alive. Launch means build once, then drive each case in its
own sandbox.

```bash
.claude/skills/verify/verify.sh build
```

This builds `.scratch/verify/brama` from the working tree, stamped with
`git describe --tags --always --dirty` (e.g. `f62ed06-dirty`). It is a plain `go build`:
no shims are embedded, which only matters to `server add`. Rebuild after every source edit.

## Doctor

Run this first, and again whenever output looks wrong:

```bash
.claude/skills/verify/verify.sh doctor
```

It checks three things, read-only: the binary exists, no non-test `.go` file under
`cmd/` or `internal/` is newer than it, and its `version` matches the checkout. It also
counts live sandboxes. Each `FAIL` line says the command that fixes it. Never drive a
stale binary: you would be proving the previous build.

## Drive

```bash
S=$(.claude/skills/verify/verify.sh sandbox <fixture>)
.claude/skills/verify/verify.sh run "$S" <label> [--expect N] -- <brama args...>
```

`sandbox` makes a fresh project under `$TMPDIR/brama-verify.*` and prints its path. It
sits outside the repo on purpose: brama looks for `brama.yaml` at or above the
working directory, so a sandbox must never find one that isn't its own.

| Fixture | What is in it |
|---|---|
| `bare` | an empty directory: no project brama recognises |
| `wordpress` | vanilla WordPress: `wp-config.php` (`WP_HOME`, `$table_prefix = 'wp_'`), `wp-content/uploads/` |
| `bedrock` | Bedrock: `web/app/uploads/`, `config/application.php`, `.env` with `WP_HOME` |
| `initialized` | `wordpress` plus the `brama.yaml` that `brama init` wrote; no `anonymize` block |
| `classified` | `initialized` plus a valid classification: `users.email`, `orders.billing_email` (`fake.email`, correlated), `users.display_name` (`keep`) |
| `approved` | `classified` plus `environments.local.anonymize.approved: [users.display_name]` |

To test a case no fixture covers, take the nearest one and edit files in `$S`
before `run` (e.g. `sed -i` a generator name to an unknown one).

`run` executes `brama <args>` inside the sandbox and writes one evidence file. It prints
that file and its path, and with `--expect N` it exits 1 when brama's exit code differs.
Always pass `--json` when you assert on output, and `--non-interactive` for any command
that may ask a question. Without them a prompt waits forever. Drop `--json` once
to see the human rendering when the change touches it.

Per-feature recipes, with the exact commands and the end state that proves each one, are
in [`features/README.md`](features/README.md). Read the matching feature file
before driving.

## Evidence

Each `run` writes `.scratch/verify/evidence/<sandbox-id>/NN-<label>.txt` with:

- the command line and the sandbox it ran in,
- the exit code (and the expected one),
- stdout and stderr, verbatim,
- what happened to `brama.yaml`: `(absent)`, `(created)` with the content, `(unchanged)`,
  or a diff.

Proof standards:

- Drive the path a user takes: the built binary in a project directory. Calling
  `run<Command>` or a test helper is not proof.
- Prove the action and the resulting state: the JSON fields that changed, and the
  `brama.yaml` section after the run.
- Check side effects separately from what was printed. `--dry-run` and every read-only
  command must show `brama.yaml (unchanged)` or `(absent)`. A command that writes must show
  the diff. A Refusal (`42`) must leave the file unchanged.
- A Refusal is a result, not a failure. Assert its `reason` (`unclassified`,
  `invalid_classification`, `review_required`, …) and its `fix` when present.
- Assert on JSON keys, never on the human wording, unless the wording is the change.
- When a feature has several entry points (e.g. vanilla and Bedrock for `init`), drive
  each one the change touches. Name any you skipped and why.

In the report, list each evidence file path with the one fact it proves.

## Cleanup

```bash
.claude/skills/verify/verify.sh cleanup
```

It removes only the sandboxes `sandbox` recorded in `.scratch/verify/sandboxes.list`,
and only paths matching `$TMPDIR/brama-verify.*`. It starts no processes, so there is
nothing to kill. Evidence under `.scratch/verify/evidence/` and the binary stay, and
`.scratch/` is gitignored. Run cleanup after every session, including failed ones.

## Keeping this honest

When a change adds or alters a command, flag, fixture need or output field, update the
matching `features/*.md` (and the fixture table above) in the same commit.
`/maintain-verification-skill` audits the whole map against the code.
