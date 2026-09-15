# Changelog

`CHANGELOG.md` follows [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/).
`brama` follows [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

The changelog is written **for the people who run `brama`**, not for the people who
wrote it. A reader should be able to answer "what changed for me, and do I need to do
anything about it?" without opening the repo.

## When to add an entry

Add an entry in the **same commit as the change**, under `## [Unreleased]`.
Don't batch entries up and reconstruct them at release time.

An entry is required when a user could observe the change without reading the source:

| Needs an entry                              | No entry                                    |
| ------------------------------------------- | ------------------------------------------- |
| New command, subcommand, or flag            | Internal refactor with identical behaviour  |
| Changed output, exit code, or error message | Tests added or changed                      |
| Changed `brama.yaml` schema or defaults     | Renaming unexported identifiers             |
| Changed deploy, rollback, or backup behaviour | CI / tooling config with no user effect   |
| Any bug fix a user could have hit           | Comments, typos, internal docs              |
| Security fix                                | Dependency bump with no behaviour change    |

When unsure, add the entry. A slightly noisy changelog beats a missing breaking change.

## How to write an entry

Group under the six canonical headings, in this order. Omit headings with no entries.

| Heading      | Meaning                        |
| ------------ | ------------------------------ |
| `Added`      | New features                   |
| `Changed`    | Changes in existing behaviour  |
| `Deprecated` | Soon-to-be-removed features    |
| `Removed`    | Now-removed features           |
| `Fixed`      | Bug fixes                      |
| `Security`   | Vulnerabilities                |

Rules for the text itself:

- **One line, one change.** Start with a verb in the present tense: "Add `brama status
  --json`", not "Added" or "This change adds".
- **Name the user-visible thing** — the command, flag, or config key — in backticks.
  `brama deploy --dry-run`, `brama.yaml: targets[].policy`.
- **Say the effect, not the implementation.** "Fix rollback restoring the wrong release
  when two deploys share a SHA", not "Fix off-by-one in releaseIndex()".
- **Breaking changes are prefixed `**BREAKING:**`** and say what the user must do:
  > - **BREAKING:** `brama push` is now `brama deploy`. Update scripts and CI.
- **Link the issue** when one exists: `(#42)`. Never paste a commit SHA or a commit
  message — a changelog is not a git log.
- **No internal jargon.** If the term isn't in `CONTEXT.md` or the CLI's own help
  output, rephrase it.

## Cutting a release

1. Rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD`. ISO 8601 dates only.
2. Add a fresh empty `## [Unreleased]` above it. Latest version always comes first.
3. Add the link reference at the bottom:
   ```
   [unreleased]: https://github.com/bramaos/brama/compare/vX.Y.Z...HEAD
   [X.Y.Z]: https://github.com/bramaos/brama/compare/vW.V.U...vX.Y.Z
   ```
4. Tag the commit `vX.Y.Z`.

Every version gets an entry, including patch releases. While pre-1.0, breaking changes
may land in minor bumps — they still get the `**BREAKING:**` prefix.

A release pulled after publishing is marked in place, not deleted:

```
## [0.2.0] - 2026-10-01 [YANKED]
```
