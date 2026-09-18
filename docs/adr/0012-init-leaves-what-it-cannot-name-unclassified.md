# init leaves what it cannot name unclassified

`brama anonymize init` writes a Classification only where a Generator declares a match, and
leaves every other column out of the file entirely. The obvious alternative — default the
remainder to `drop`, since dropping is safe — is safe only about privacy. Zeroing
`orders.total_amount`, emptying `settings.retry_count` or blanking a feature flag produces
a technically anonymous database that is useless to develop against, and it makes that
choice silently, on Brama's authority, about data whose meaning Brama could not determine.

Deciding what a column means when nothing claims it is a judgement, and judgements belong
to `brama anonymize review`.

## Consequences

- `init` has exactly two outcomes: a declared Generator match is written, and anything else
  is omitted. There is no "unusable, so drop it" bucket.
- An omitted column is Unclassified, which is already a defined state with a defined
  consequence, so `brama.yaml` never needs a fourth token meaning "pending".
- `init` does not produce a config that can Pull. The first `review` has real work in it,
  proportional to how much of the schema no Adapter and no Generator recognises.
- `init` never writes `keep`. `keep` is a decision to preserve production data, and it
  enters the file only where a human put it.
- Matching is binary: a Generator's declared name patterns and types either claim a column
  or they do not. No similarity score, no threshold. "Why did Brama choose `fake.email`?"
  has the answer "because `fake.email` declares it matches this column", which survives
  being asked in an incident review.
