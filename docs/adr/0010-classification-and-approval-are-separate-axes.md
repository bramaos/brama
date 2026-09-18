# Classification and Approval are separate axes

Classification answers what a column means; Approval answers where its real values may go.
We keep them apart, in different parts of `brama.yaml`, because collapsing them lets a
source of knowledge become a source of authorization. An Adapter genuinely knows that
`wp_users.display_name` is a public-facing name — it does not know whether you want real
names on a developer's laptop. A Preset therefore wins the conflict-resolution question and
loses the authorization question, and nothing other than a human ever grants an Approval.

## Consequences

- `anonymize.tables` holds Classification, project-wide. `environments.<name>.anonymize.approved`
  holds Approval, per destination.
- A Preset's own `keep` is not pre-approved by virtue of not being written into
  `brama.yaml`. Presets are omitted from the file because they are reusable knowledge, not
  because they are trusted.
- A `keep` with no Approval for the destination falls back to the Classification a claiming
  Generator would have given it, and the Pull reports the substitution. A Pull can only ever
  produce less exposure than the file suggests, never more.
- Where no Generator claims the column, the Pull refuses instead of falling back to `drop`.
  A Pull may reduce exposure by derivation; it may not invent destructive policy. Choosing
  between emptying a column and preserving it is the same kind of judgement ADR 0012 keeps
  out of `init`, and it belongs to a human for the same reason.
- The fallback is recomputed on the Server rather than stored, so a column carries one
  `action` and never a second shadow answer. Nothing per-Environment needs to appear in the
  file, because every fallback that exists is derivable — and where derivation runs out, the
  system stops and asks rather than guessing.
- Adding an Environment is therefore safe by default, but not silent by default: it Pulls
  immediately unless some `keep` column has no Generator to fall back to, and then it names
  that column before anything moves.
- `brama.yaml` can be read two ways and both are true: the `tables` block is what the data
  means, the `environments` blocks are who may see it. Reviewing exposure means reading one
  short list per Environment, not auditing every column.
