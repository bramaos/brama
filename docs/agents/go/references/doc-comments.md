# Doc comments

Depth behind the Doc comments section of `docs/agents/go.md`. Our prose, not the upstream
text. Every claim links the section it came from. Where this file and the code disagree,
the code wins (see the Priority section of `go.md`).

## What gets one

Every exported top-level name has a doc comment. So does any unexported type or function
whose behaviour or meaning isn't obvious.
([Decisions § Doc comments](https://google.github.io/styleguide/go/decisions#doc-comments),
[Code Review Comments § Doc comments](https://go.dev/wiki/CodeReviewComments#doc-comments))
In brama, `revive` enforces the exported half. It skips the `Result` methods in
`internal/cli`, which `renderer.Result` documents once on the interface (`.golangci.yml`).

A comment directly above a top-level declaration, with no blank line between, is that
declaration's doc comment.
([Effective Go § Commentary](https://go.dev/doc/effective_go#commentary))

## Form

A doc comment is one or more full sentences that start with the name being described. An
article may come first ("A Request represents…").
([Decisions § Doc comments](https://google.github.io/styleguide/go/decisions#doc-comments),
[Code Review Comments § Comment sentences](https://go.dev/wiki/CodeReviewComments#comment-sentences))
An unexported comment follows the same form, so exporting the name later means renaming
it in one place. ([Decisions § Doc comments](https://google.github.io/styleguide/go/decisions#doc-comments))

Full sentences are capitalised and punctuated. A short end-of-line comment on a struct
field can be a fragment with the field as its implied subject.
([Decisions § Comment sentences](https://google.github.io/styleguide/go/decisions#comment-sentences))
In brama, `godot` requires the final period.

There is no fixed width. Wrap long comments so they read in a narrow editor, and be
consistent within a file. Don't break a long URL.
([Decisions § Comment line length](https://google.github.io/styleguide/go/decisions#comment-line-length))

## What to say

Don't list every parameter. Document the ones that are error-prone or surprising, and say
why they matter. ([Best Practices § Parameters and configuration](https://google.github.io/styleguide/go/best-practices#parameters-and-configuration))

Say what the caller must clean up, and how, when that isn't obvious.
([Best Practices § Cleanup](https://google.github.io/styleguide/go/best-practices#cleanup))

Name the sentinel errors and error types a function returns, and say whether a type is
returned as a pointer. Conventions that hold across a package go in the package comment.
([Best Practices § Documentation: Errors](https://google.github.io/styleguide/go/best-practices#errors))

Leave out what the reader already assumes: that cancelling the context interrupts the call,
that read-only methods are safe to call concurrently, that mutating ones are not. Say so
only when it isn't true.
([Best Practices § Contexts](https://google.github.io/styleguide/go/best-practices#contexts),
[Best Practices § Documentation: Concurrency](https://google.github.io/styleguide/go/best-practices#concurrency))

Explain why, not what. A comment that restates the code, or contradicts it, adds only
clutter and upkeep.
([Guide § Why is the code doing what it does?](https://google.github.io/styleguide/go/guide#why-is-the-code-doing-what-it-does))

When a line looks like a common idiom but differs (`err == nil` where `err != nil` is
expected), a short comment points out the difference.
([Best Practices § Signal boosting](https://google.github.io/styleguide/go/best-practices#signal-boosting))

## Package comments

Exactly one per package, directly above `package` with no blank line, starting
"Package name …". A long one may live alone in `doc.go`. Notes for maintainers that
shouldn't appear in godoc go after the imports instead.
([Decisions § Package comments](https://google.github.io/styleguide/go/decisions#package-comments),
[Code Review Comments § Package comments](https://go.dev/wiki/CodeReviewComments#package-comments))

For a `main` package, the comment names the binary: "Command brama …" or
"Brama …", capitalised even though the command is typed in lowercase.
([Code Review Comments § Package comments](https://go.dev/wiki/CodeReviewComments#package-comments),
[Go Doc Comments § Commands](https://go.dev/doc/comment#cmd))

## Formatting

- A blank `//` line separates paragraphs.
- Indented lines render verbatim, for code and command lines. `gofmt` indents such a
  block by one tab and puts a blank line on each side.
- `[Name]`, `[pkg.Name]` and `[*pkg.Type]` link to other declarations. `[Text]` with a
  `[Text]: URL` definition at the end of the comment links to a URL.
- A list is an indented span whose first line starts with a bullet (`-`, `*`, `+`) or a
  number.
([Best Practices § Godoc formatting](https://google.github.io/styleguide/go/best-practices#godoc-formatting),
[Go Doc Comments § Code blocks](https://go.dev/doc/comment#code),
[Go Doc Comments § Doc links](https://go.dev/doc/comment#doclinks),
[Go Doc Comments § Links](https://go.dev/doc/comment#links),
[Go Doc Comments § Lists](https://go.dev/doc/comment#lists))
brama's comments don't use doc links yet. They are fine to introduce.

A `Deprecated:` paragraph marks an identifier as deprecated in tools.
([Go Doc Comments § Deprecations](https://go.dev/doc/comment#deprecations))

## Examples

Runnable `Example` functions live in test files, appear in godoc, and run as tests. Prefer
one to a code block in a comment when showing how to use an API.
([Decisions § Examples](https://google.github.io/styleguide/go/decisions#examples),
[Code Review Comments § Examples](https://go.dev/wiki/CodeReviewComments#examples),
[Best Practices § Godoc formatting](https://google.github.io/styleguide/go/best-practices#godoc-formatting))
