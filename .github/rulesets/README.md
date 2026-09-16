# Rulesets

`main.json` is the ruleset for `main`. **It is not applied.** Until v1, `main` takes
direct pushes on purpose: the cost of a broken commit on a branch nobody depends on
yet is lower than the cost of a pull request for every change.

That trade stops holding the moment someone can install a release. Apply it then.

## What it does

| Rule | Effect |
|------|--------|
| `deletion`, `non_fast_forward` | `main` cannot be deleted or force-pushed |
| `required_linear_history` | No merge bubbles; history stays bisectable |
| `pull_request` | Changes arrive by pull request; stale reviews are dismissed on push |
| `required_status_checks` | The six checks below must be green, on a branch that is up to date with `main` |

The required checks are the job names CI reports under. They have to match exactly:

- `lint / golangci-lint (ubuntu-latest)` — the `lint` job in `ci.yml` calling `lint.yml`
- `check (ubuntu-latest)`, `check (macos-latest)`
- `govulncheck`
- `codeql`, `dependency-review` — from `security.yml`

Renaming a job in a workflow renames its check, and a required check that no longer
reports blocks every pull request. Change both together.

`required_approving_review_count` is `0`, which still requires a pull request but
lets a sole maintainer merge their own. Raise it to `1` when there is a second
person to ask.

## Applying it

```sh
gh api --method POST repos/bramaos/brama/rulesets \
  --input .github/rulesets/main.json
```

Check what is in force:

```sh
gh api repos/bramaos/brama/rulesets --jq '.[] | {id, name, enforcement}'
```

Update an existing one — rulesets are addressed by id, not by name, so a second POST
creates a duplicate rather than replacing:

```sh
gh api --method PUT repos/bramaos/brama/rulesets/<id> \
  --input .github/rulesets/main.json
```

To rehearse without blocking anything, change `"enforcement"` to `"evaluate"`. The
ruleset then records what it *would* have blocked, visible under
Settings → Rules → Rulesets → Insights.
