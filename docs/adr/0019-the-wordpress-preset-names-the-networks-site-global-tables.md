# The WordPress Preset names the network's site-global tables

A WordPress multisite network adds six tables to the twelve every install has: `wp_blogs`,
`wp_site`, `wp_sitemeta`, `wp_blogmeta`, `wp_signups` and `wp_registration_log`. The
`wordpress` Preset named none of them, so every one of their columns was Unclassified and
`anonymize check` refused. ADR 0018 recorded this as the second reason multisite still
refused; the first, per-site usermeta keys, was closed there.

The Preset now names all six, on every install, whatever `anonymize.tables` says.

That is consistent with ADR 0014. What it refuses is naming a table Brama has not seen and
cannot count: a per-site table like `wp_2_posts` exists once per site, and naming it from
the Preset would be guessing how many sites there are. The network's tables are not per
site. Core creates them with the plain `$table_prefix` and no site id, and there is one set
per install, so they are named from the project's prefix exactly as `{prefix}users` is.
Per-site tables stay the project's own to classify.

It is consistent with ADR 0018 too, for the same reason in the other direction: `{n}`
matches a site's copy of a key without Brama learning the site ids, and naming the
site-global tables needs no id at all.

On a single-site install the six tables are absent, and naming a table the database lacks
has no effect. `schema.Read` reads no Discriminator for it, `Cover` walks only the tables
the Schema has, and the check summary counts only the tables that are present. No code
outside the Preset changed.

The Classification is argued column by column in `internal/preset/wordpress.go`. Two
decisions cost the copy something, and they are recorded here.

`wp_sitemeta.meta_key=site_admins` is `drop`. The value is a serialized list of the super
admins' real `user_login` values. Logins are fabricated, so the real list would match no
account on the copy, and keeping it leaks the logins for nothing. Rewriting it to the
fabricated logins would need a Generator that reads serialized PHP arrays, and one row is
not worth one. Without the row WordPress falls back to its default, `['admin']`, so **the
copy has no super admin**. Each site's administrators still work, because the per-site
`wp_{n}_capabilities` keys are kept. A developer who needs the network admin runs
`wp super-admin add <login>` on the copy. Brama does not do that for them: creating a
super admin is a write to the destination that no Classification can express.

Multisite adds `spam` and `deleted` to `wp_users` as well. The Preset names them `keep`,
because they are account state and identify nobody. Naming a column the table lacks has
no effect on coverage, but the check summary counts what the Preset classifies in each
table present, so a single-site install now reports two more classified columns in
`wp_users` than before. Its table count, coverage and correlation groups do not change.

## Consequences

- A multisite install whose tables and keys are core's own passes `anonymize check`.
  A plugin's key in `wp_sitemeta` stays Unclassified and refuses, as it does in options.
- The `sitemeta` key list is core's own and closed. Core adding a key in a later release
  is a key that refuses until the Preset names it, which is the refusal working.
- The salts `wp_salt()` saves in `wp_sitemeta` when `wp-config.php` defines none are
  named one by one and dropped; WordPress regenerates them. A `*_key` entry would have
  classified a plugin's keys as core's. The same salts in a single site's options are not
  named, and stay the gap they were.
- `wp_registration_log.IP` is `drop`, not `fake.ip`. Core declares it `varchar(30)`, and
  a fabricated IPv6 address needs 45 characters. The Generator is not widened for one
  column that is worth nothing on a copy.
- Offline, with no Schema to narrow it, `anonymize check` counts eighteen Preset tables
  rather than twelve.
