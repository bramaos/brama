# A Preset is named for the project's table prefix

A Preset knows which tables a framework creates and what is in them. It does not know what
this install calls them: the name is half the framework's and half the project's, and
WordPress spells the project's half `$table_prefix`. `wp_` is that value's most common
setting and not its meaning — moving off it is standard hardening advice, and plenty of
projects took it.

So a Preset's table names carry the prefix as a placeholder, and the Adapter reads the
value out of the project's own config: `wp-config.php`, or `config/application.php` and
`.env` on Bedrock. It is read every time a Preset is resolved, and never recorded in
`brama.yaml` — a prefix written down twice can disagree with itself, and the copy in
`brama.yaml` is the one nobody updates when the install moves. The prefix is Adapter
knowledge in the sense `CONTEXT.md` already uses, so it lives beside detection rather than
in `internal/preset`, and the Preset vocabulary stays closed and framework-free.

A prefix Brama cannot determine is a Refusal naming the files it read, never a fallback to
the framework's default. A Preset applied under a guessed prefix does not match nothing —
it applies the accounts table's Classification to whatever table sorted into that place,
which is the resemblance-matching ADR 0013 keeps out of the Generators, reaching the one
process that reads raw production data.

## Consequences

- `anonymize init` and `anonymize check` read the prefix the same way, through one seam,
  so the tables a Preset covers cannot be one set when the file is written and another when
  it is validated.
- `brama anonymize check` still means something on a CI runner, but a project whose Preset
  is waiting on a prefix now needs the framework's config in the checkout. A committed
  `wp-config.php` is there; a Bedrock `.env` may not be, and that run refuses rather than
  reporting a coverage it did not earn.
- Only a literal assignment counts. A prefix built from an expression — `getenv`, a
  concatenation, Bedrock's `env('DB_PREFIX') ?: 'wp_'` with nothing in `.env` — is
  undetermined, because `env()` reads a real environment Brama is not running in. Where PHP
  would take the last of two assignments, so does Brama.
- A multisite install's per-site tables are the project's own to classify. The Preset
  answers for the site-global set under the base prefix; inferring how many sites there are
  would be the guessing this decision exists to refuse.
- A project can always step outside the Preset: drop `anonymize.preset` and classify the
  tables in `brama.yaml`, which needs no prefix and reads no framework config at all.
