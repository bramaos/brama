# A Discriminator value may hold a number placeholder

A WordPress multisite install keeps one `wp_usermeta` for the whole network and splices the
site's id into the keys that belong to a site: `wp_2_capabilities`, `wp_3_user_level`. The
preset names the site-global forms — `wp_capabilities` is `$table_prefix` + `capabilities`,
which is also what site 1 uses — so every other site's copy of a key the preset already
answers for arrived Unclassified and refused the Pull.

Refusing was right, and useless. The value `wp_2_capabilities` holds what
`wp_capabilities` holds, classified `keep` since the preset was written; the number is the
only difference and it is the one thing the preset cannot write down. The project's way
out was to list every site's keys in `brama.yaml` by hand, and to add five more lines each
time somebody created a site. That is a list that goes stale silently, which is the failure
mode `0014-a-preset-is-named-for-the-projects-table-prefix.md` exists to avoid for table
names.

The existing vocabulary could not say it. A Discriminator value is matched by its exact
name or by a prefix ending in `*`, and the prefix that covers `wp_2_capabilities` is
`wp_*`, which covers every other prefixed key in the table too — including the ones nobody
has classified, which is the exposure the Discriminator exists to prevent. A pattern that
matches too much is worse than no pattern, because it answers where it should refuse.

So a Discriminator value may hold `{n}`, standing for one run of digits:
`{prefix}{n}_capabilities` in the preset, `wp_{n}_capabilities` once the prefix is
resolved. It is matched, never substituted. Brama learns no numbers, reads no `wp_blogs`,
and enumerates no sites; it recognises a value that production already handed back under
`0017-discriminator-values-are-read-from-production.md`. That is the line this decision
draws against ADR 0014, which stands unchanged: Brama still never names a table it has not
seen, because a table it supposes into existence classifies whatever sorted into its place,
while a value it matches is one it has been shown.

The alternative was a `{blog}` placeholder resolved like `{prefix}` is, from the ids in
`wp_blogs`. It was rejected twice over. Resolution would need a live database, so
`anonymize check` would stop meaning anything on a CI runner that has no production
credentials — the property ADR 0014 was careful to keep. And it would put the count of
sites back into Brama's hands, inferred at one moment and applied at another, which is the
guessing that ADR refuses. `{n}` needs neither: the matching happens where the values
already are.

`{n}` is spelled for its shape and not for what the number means. The matching lives in
`internal/config`, which serves every preset, and a Laravel key that numbers something is
not a blog. Naming the placeholder `{blog}` would have put one framework's word into
syntax every other framework writes.

## Consequences

- A key is matched by its exact name, then by a name holding `{n}`, then by a prefix
  ending in `*`; within each the longest match wins. A `{n}` entry is literal everywhere
  but one bounded segment, so it is more specific than any open-ended prefix and outranks
  it. Ranking it below `*` would let a broad entry shadow a precise one, which is the
  loosest classification winning.
- `{n}` matches one or more digits and nothing else. `wp_admin_capabilities` does not
  match `wp_{n}_capabilities`: a key nobody has examined stays Unclassified and still
  refuses.
- An entry holds at most one `{n}`, anywhere in the key, and may end in `*` as well. Two
  placeholders in one entry, or an unrecognised `{…}`, is a broken `brama.yaml` and exits
  1 like the file's other errors — not 42, which is reserved for Brama being unable to act
  on a file it understood.
- `{n}` is legal in a hand-written classification, not only in a preset. It is key syntax,
  as `*` already is, so a project that drops `anonymize.preset` keeps what it could say.
- The site-global forms stay listed beside the numbered ones. `wp_capabilities` is a key
  in its own right, not site 1's instance of a pattern, and it is the only form a
  single-site install has.
- An Approval names the entry, so `wp_usermeta.meta_key=wp_{n}_capabilities` approves the
  meaning once, the way `wp_options.option_name=_transient_*` already does. A site created
  after the Approval carries the same kind of value the human judged. Where that is too
  broad, the narrower answer was always available: classify the keys by hand.
- Multisite still refuses. The keys are one of two reasons; the network's own site-global
  tables — `wp_blogs`, `wp_site`, `wp_sitemeta`, `wp_blogmeta`, `wp_signups`,
  `wp_registration_log` — are named by no preset and are the other.
