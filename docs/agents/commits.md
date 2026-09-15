# Commits

Commit messages follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/).
`brama` follows [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html), and the
commit type is what decides the next version bump.

A commit message is read by two audiences: the tooling that cuts releases, and the person
running `git log` a year from now to find out why a line exists. Write for both.

## Format

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

The type, the terminal colon, the space, and the description are required. Everything else
is optional.

```
feat(anonymize): refuse a pull when a column has no classification
```

## Types

`feat` and `fix` are the only two the spec defines. The rest of this table is the vocabulary
this repo uses — stick to it rather than inventing new ones.

| Type       | Use when                                                       | Bump    |
| ---------- | -------------------------------------------------------------- | ------- |
| `feat`     | A new command, flag, adapter, or capability                     | MINOR   |
| `fix`      | A bug a user could have hit                                     | PATCH   |
| `docs`     | Documentation only — README, ADRs, `docs/`, code comments       | none    |
| `refactor` | Behaviour is identical; the code is not                         | none    |
| `perf`     | A change made for speed or memory, behaviour unchanged          | none    |
| `test`     | Adding or correcting tests, nothing else                        | none    |
| `build`    | `go.mod`, build scripts, the `Makefile`, release packaging      | none    |
| `ci`       | Workflows in `.github/`, CI config                              | none    |
| `chore`    | Housekeeping that fits nothing above — `.gitignore`, tidying    | none    |
| `revert`   | Undoing a previous commit (see below)                           | none    |

Types are lowercase. `BREAKING CHANGE` is the one token that must be uppercase.

If a change conforms to more than one type, that is a sign it should be more than one
commit. Split it.

## Scope

Optional. A noun naming the part of the codebase that changed, in parentheses:

```
fix(mysql): quote identifiers that collide with reserved words
```

Prefer a term already in `CONTEXT.md` — `adapter`, `shim`, `sync`, `policy`,
`classification`, `preset`, `discriminator` — or a package name (`mysql`, `postgres`,
`ssh`, `config`). Omit the scope rather than inventing a vague one; `feat(core):` says
nothing.

## Description

The text after the colon, on the subject line.

- **Imperative mood, present tense.** "add", not "added" or "adds". The test: the subject
  should complete the sentence _"Applying this commit will…"_.
- **Lowercase start, no trailing period.**
- **Aim for 72 characters or fewer** including the type and scope, so `git log --oneline`
  stays readable.
- **Say the effect, not the mechanism.** `fix(pull): stop truncating utf8mb4 emoji in post
  titles`, not `fix(pull): change column type handling`.

## Body

Optional, one blank line after the description. Free-form, any number of paragraphs.

Use it for the **why**, not the what — the diff already says what. Reach for a body when the
change reverses an earlier decision, works around an external bug, or looks wrong without
context. Wrap at 72 columns.

## Footers

One blank line after the body. Each footer is `Token: value`, where the token uses `-`
instead of spaces — the git trailer convention.

| Footer                | Use                                                     |
| --------------------- | ------------------------------------------------------- |
| `Closes #42`          | This commit fully resolves the issue                    |
| `Refs #42`            | Related to the issue, does not close it                 |
| `BREAKING CHANGE: …`  | Describes an incompatible change (uppercase, spaces OK) |
| `Co-authored-by: …`   | A second author                                         |

`BREAKING CHANGE` is the one exception to the `-` rule; `BREAKING-CHANGE` is a valid
synonym. A footer value may run across several lines — parsing stops at the next token.

## Breaking changes

Mark them in the subject with `!` before the colon, in the footer, or both:

```
feat(config)!: rename `targets` to `environments` in brama.yaml
```

```
feat(config): rename `targets` to `environments` in brama.yaml

BREAKING CHANGE: `brama.yaml` key `targets` is now `environments`. Run
`brama config migrate` or rename the key by hand.
```

When `!` is used alone, the description must carry the breaking change on its own. When a
`BREAKING CHANGE:` footer is present, it must say **what the user has to do**, not only what
broke.

A breaking change means MAJOR, whatever the type. While `brama` is pre-1.0 these may land in
a MINOR bump — they are still marked, and still get the `**BREAKING:**` prefix in
`CHANGELOG.md`.

## Reverting

```
revert: feat(anonymize): spill the mapping to a temp file

Refs: a1b2c3d
```

## The changelog is not the commit message

They are written in the same commit but for different readers. `CHANGELOG.md` is for people
who **run** `brama`; the commit message is for people who **change** it. A `refactor` or
`ci` commit needs no changelog entry; a `feat` almost always does. See
`docs/agents/changelog.md`.

Never paste a commit SHA or a commit subject into the changelog.

## Writing the message

Use a heredoc so the body and footers survive quoting:

```sh
git commit -F - <<'EOF'
feat(anonymize): refuse a pull when a column has no classification

Every column must be explicitly fake, keep, or drop. An unclassified
column is more likely to be a new migration nobody classified than a
column that is safe to pass through, so the safe default is to stop.

Closes #10
EOF
```

## Fixing a bad message

Before it is pushed, `git commit --amend` or `git rebase -i` it. After it is pushed, leave
it — history rewriting on `main` costs more than a malformed subject line. A commit that
misses the convention is only invisible to tooling, not harmful.

The repo's first commit (`init: first commit`) predates this document. It stays as it is.
