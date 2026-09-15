# Refuse to pull when anything is unclassified

Every comparable tool — Greenmask, PostgreSQL Anonymizer, Snaplet, Percona — passes columns
it has no rule for straight through, unmasked. We refuse the operation instead: an
Unclassified column, or an unrecognised Discriminator value, stops the Pull with the
Refusal exit code. A column added by a migration six months from now is exactly how a leak
happens, and a warning nobody reads is not a control.

## Consequences

- Classification covers every column in the source, answered `fake`, `keep`, or `drop`.
  There is no implicit third state.
- A migration that adds a column breaks the next Pull until someone classifies it. This is
  the intended cost.
- `keep` on a column Brama believes holds personal data is permitted, but it is warned at
  the time and recorded in `brama.yaml`, where review can see it.
- Adapters ship Presets so the one-time classification cost lands on project-specific
  tables rather than the framework's own.
