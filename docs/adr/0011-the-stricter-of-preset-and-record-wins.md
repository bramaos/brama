# The stricter of the Preset and the record wins

Presets are referenced by name rather than expanded into `brama.yaml`, so upgrading Brama
can change what a Preset says about a column. We apply the change when it tightens and
refuse to apply it when it loosens: a Preset moving a column from `keep` to `fake` takes
effect on the next Pull, a Preset moving one from `fake` to `keep` waits for
`brama anonymize review`. The alternative — pinning a Preset version — means a security fix
we ship sits inert in every project until someone thinks to bump a number.

The recorded Classification is itself the baseline the comparison runs against, so this
needs no version, no hash, and no server-side history.

## Consequences

- A Preset improvement reaches existing projects without ceremony; a Preset regression
  cannot widen exposure without a human.
- Between a tightening and the next `brama anonymize review`, `brama.yaml` says `keep`
  while Brama does `fake`. The Pull reports this, and `review` writes the file back into
  agreement.
- `review` is the only command that writes. A Pull never mutates a tracked file, so it
  cannot dirty a CI checkout or turn a data operation into a source-control event.
- Because the record is the baseline, a project that has never run `review` has no baseline
  and every Preset answer is applied as-is. That is correct: nothing has been decided yet,
  so nothing is being overridden.
