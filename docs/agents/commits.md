# Commits

The shared rules are in `~/www/home/docs/agents/commits.md`. This file adds what is different in `brama`.

`brama` follows [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html), and the commit type decides the next version bump. A commit message is also read by the tooling that cuts releases.

## Types

| Type       | Use when                                                   | Bump  |
| ---------- | ---------------------------------------------------------- | ----- |
| `feat`     | A new command, flag, adapter, or capability                | MINOR |
| `fix`      | A bug a user could have hit                                | PATCH |
| `docs`     | Documentation only: README, ADRs, `docs/`, code comments   | none  |
| `refactor` | The behaviour is the same; the code is not                 | none  |
| `perf`     | A change for speed or memory, the behaviour is the same    | none  |
| `test`     | Adding or correcting tests, nothing else                   | none  |
| `build`    | `go.mod`, build scripts, the `Makefile`, release packaging | none  |
| `ci`       | Workflows in `.github/`, CI config                         | none  |
| `chore`    | Housekeeping that fits no other type: `.gitignore`         | none  |
| `revert`   | Undoing an earlier commit                                  | none  |

## Scope

Use a term from `GLOSSARY.md` (`adapter`, `shim`, `sync`, `policy`, `classification`, `preset`, `discriminator`) or a package name (`mysql`, `postgres`, `ssh`, `config`).

## Breaking changes

While `brama` is below 1.0, a breaking change can land in a MINOR bump. Still mark it, and still give it the `**BREAKING:**` prefix in `CHANGELOG.md`.

## The changelog is not the commit message

Write both in the same commit, for different readers. `CHANGELOG.md` is for people who **run** `brama`; the commit message is for people who **change** it. A `refactor` or `ci` commit needs no changelog entry; a `feat` almost always does. See `docs/agents/changelog.md`.

Never paste a commit SHA or a commit subject into the changelog.

The repo's first commit (`init: first commit`) is older than these rules. It stays as it is.
