# init leaves what it cannot name unclassified

`brama anonymize init` writes a Classification only where a Generator declares a match, and
leaves every other column out of the file entirely. The obvious alternative — default the
remainder to `drop`, since dropping is safe — is safe only about privacy. Zeroing
`orders.total_amount`, emptying `settings.retry_count` or blanking a feature flag produces
a technically anonymous database that is useless to develop against, and it makes that
choice silently, on Brama's authority, about data whose meaning Brama could not determine.

Deciding what a column means when nothing claims it is a judgement, and judgements belong
to a human.

**Amended (#9):** in a terminal, `init` puts that judgement to the person running it, one
column or key at a time: leave it, `drop`, a `fake.<generator>` that can fill it, or
`keep`. Leaving it is the first option, so enter alone decides nothing. A key the file
cannot name, or one whose value column the Schema lacks, is not asked about. What they
answer is written; what they leave, and everything a run with nobody at the keyboard
reaches, stays omitted as below. Such a run writes what Generators claim and exits 42 (`unclassified`),
because a Pull of that file still refuses.

## Consequences

- Brama itself has exactly two outcomes per column: a declared Generator match is written,
  and anything else is omitted unless a person answers for it. There is no "unusable, so
  drop it" bucket.
- An omitted column is Unclassified, which is already a defined state with a defined
  consequence, so `brama.yaml` never needs a fourth token meaning "pending".
- `init` produces a config that can Pull only where somebody answered for everything no
  Adapter and no Generator recognises. Otherwise it exits 42.
- Brama never writes `keep` on its own. `keep` is a decision to preserve production data,
  and it enters the file only where a human put it — by hand, or by picking it in `init`.
- Matching is binary: a Generator's declared name patterns and types either claim a column
  or they do not. No similarity score, no threshold. "Why did Brama choose `fake.email`?"
  has the answer "because `fake.email` declares it matches this column", which survives
  being asked in an incident review.
