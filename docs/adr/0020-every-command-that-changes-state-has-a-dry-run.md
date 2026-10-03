# Every command that changes state has a dry run

The product description promised `--dry-run` on "every acting command whose effect is
local". `brama server add`, whose effect is remote, got one by exception once the Shim
version check gave it something to say (ADR-0007). `brama anonymize review` writes
`brama.yaml` and had none: `anonymize check` stood in as its read-only twin.

Every command that changes state now takes `--dry-run`. State means `brama.yaml`, any
other file or database on this machine, and anything on a Server. Local or remote makes no
difference, and neither does a read-only twin.

A dry run:

- **resolves everything and changes nothing.** It may read, and it may connect:
  `server add --dry-run` has to reach the Server to say which Shim answers there. It writes
  nothing, here or on a Server, and takes no Recovery point. The Recovery point it would
  take is part of what it reports.
- **never asks.** A dry run that ends in `Continue? [y/N]` and acts on `y` is not dry. A
  real run shows its plan before it acts and asks there.
- **renders the real run's Result** with `dry_run: true` in its Fields, so the JSON says
  which of the two it was.
- **exits as the real run would**: 0, 1 or 42. The outcome is part of the preview. A dry
  run of a refused action that exits 0 hides the Refusal (ADR-0003) from the script that
  asked.
- **is proven.** Each command's tests assert that a dry run left every file it would
  have written byte-identical, and that a fake Server received nothing. The `verify`
  skill's evidence shows `brama.yaml (unchanged)`.

A read-only twin was the alternative, and `review` already had one. It was rejected
because a twin answers a different question. `check` says whether the file is valid;
`review --dry-run` says what `review` would write. A user or an agent who wants to see an
effect before it happens should be able to type the same flag on any command, without
first learning which other command previews it.

## Consequences

- `anonymize review` gains `--dry-run`. `init` adds `dry_run` to its Result.
  `anonymize init --dry-run` exits 42 when the real run would leave columns
  Unclassified; it used to exit 0.
- `db pull`, `files pull` and every deployment command ship with `--dry-run` from their
  first version, not as a later addition.
- A read-only command (`anonymize check`, `version`) takes no `--dry-run`, because it has
  nothing to skip.
- `--dry-run` is a preview, not a way past a guardrail. It skips no check, and a dry run
  refuses whatever the real run would refuse.
