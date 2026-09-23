# Discriminator values are read from production

A Schema never holds rows, so that deciding Classification never needs production data. A
key/value table breaks that. `wp_usermeta`'s Schema is four columns and says nothing about
which kinds of value `meta_value` holds. Those are whatever a plugin decided to write, and
that list only exists as rows. If Brama never read the rows, it could not tell that a key is
Unclassified, and ADR-0003's refusal would have nothing to refuse for key/value tables. That
is exactly where a plugin puts a customer's billing email.

So the Shim reads a Discriminator's distinct values on the Server: in a connected
`anonymize check`, in `review` and in a Pull. It never reads the values they select. The
keys leave the Server as they are, because a Refusal has to name them and so does the
committed `brama.yaml` that classifies them. The Discriminator column is never classified. A
key names a kind of value, not a person.

`check` reads them as well as the Pull. If only the Pull read them, a new plugin key would
first show up as a Refusal on a developer's laptop and not on CI, where `check` exists to
catch it.

## Consequences

- The Schema-only rule has one exception, and it is written into the Schema glossary entry.
  The exception is a Discriminator's distinct values, and nothing else.
- Keys are compared as bytes, and discovered with a binary `DISTINCT`. A collation that
  folds `Billing_Email` into `billing_email` would hide one spelling behind the other. With
  bytes, the variant is Unclassified and refuses.
- A key that embeds personal data (`_user_42_email`) reaches the laptop in a Refusal and in
  the committed file. That is accepted: no framework we know of writes such keys, and
  classifying the Discriminator would add a third axis to answer for it.
- A `keys` entry matches a key by its exact name, or by a prefix ending in `*` with the
  longest match winning. Without that, every new transient hash in `wp_options` would refuse
  the next Pull. An Approval names the entry as written, pattern included.
- A dropped key travels as no row, not as an empty value. The row carries nothing but its
  value, and a plugin can tell `''` apart from a key that is absent.
