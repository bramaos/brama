# Output rules

What brama decided about how a command reports. The reason is in
`docs/product-description.md` §9, "The rendering rule", and §10, unless a rule points
elsewhere. A rule with no citation is uncited on purpose: its reason is the code it
describes.

- Report an outcome by returning a `renderer.Result` and passing it to `env.Renderer.Result`; `Main` renders Refusals and errors.
- Never write to `os.Stdout` or call `fmt.Print*` from a command; write to `env.Out` or `env.Err` only for human-only extras that return early on `env.JSON`, like `writeSkeletonPreview`.
- Put every fact in `Fields`; `Headline` is prose for a person and holds nothing the JSON lacks. (`renderer.Result` doc)
- Let the Result decide its `Status`; glyphs, colour, layout and JSON shape live in `internal/renderer`, never in a command.
- Emit one JSON object per run: a command that rendered a Result and still fails returns `reported(err)`. (`reported` doc)
