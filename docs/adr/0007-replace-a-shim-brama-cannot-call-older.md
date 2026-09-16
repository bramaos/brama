# Replace a Shim brama cannot call older

The Shim upgrades when the CLI is newer. That leaves the other two directions
unanswered: a Server whose Shim is *newer* than the CLI reaching it, and a pair of
versions brama cannot rank at all. We install this brama's build in both cases, and
call it a replacement rather than an upgrade.

Both directions are ordinary. Two developers on one project rarely run the same
`brama`, and ADR-0006 gives them one Shim per SSH user, not one per person — so the
one who upgrades first leaves a newer Shim behind for everyone sharing that account.
Unrankable versions are more ordinary still: `git describe --tags --always --dirty`
stamps `dev` outside a git tree and a bare commit in a tree with no tags, and neither
sits anywhere in an ordering.

The alternative was to stop, and tell the user to upgrade `brama`. It reads safer
than it is. The Shim and the CLI must come from one build — that is what makes an
embedded binary better than a fetched one, and in v0.2 it is what makes structured
operations a contract rather than a hope. A CLI that cannot drive the Shim in front of
it has no operation to protect, so stopping does not preserve anything; it only moves
the failure earlier and hands the user a fix that may not be theirs to apply. Stopping
on unrankable versions is worse: it would make every `dev` build unable to reach a
Server a tagged build had touched, which is the developer's own machine, on the day
they are working on brama itself.

Replacing is cheap because of what the Shim already is. It holds no state, it is the
same build for every Environment on the Server, and ADR-0006 replaces it wholesale by
rename with the previous build kept beside it. There is nothing to migrate and nothing
to lose.

## Consequences

- Two `brama` versions sharing one SSH account on one Server will swap the Shim back
  and forth, one install per run. It is a wasted upload, not a broken operation, and
  the visible step says plainly which way it went every time. If it becomes a real
  irritation the answer is separate Identities, which is where ADR-0006 already points.
- `replace` is a distinct outcome from `upgrade` in the result, the `--json` contract,
  and `state.json` — so "brama moved this Server backwards" is a fact someone can find
  afterwards rather than an inference from two timestamps.
- Version ordering is brama's own, in `internal/version`, not a SemVer library. A
  general one reads `v0.1.0-3-gabc1234` as a *prerelease* of `v0.1.0` and ranks a
  build three commits past the tag below the tag itself, which would turn every
  untagged build into a silent downgrade.
- brama never refuses an operation over a version difference. The version check can
  fail — a Server brama has no build for, a Shim that does not run once installed —
  but it has no Refusal of its own, and `--dry-run` is the way to see what it will do
  before it does it.
