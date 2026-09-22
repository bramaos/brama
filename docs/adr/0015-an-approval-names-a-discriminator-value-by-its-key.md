# An Approval names a Discriminator value by its key

`0010-classification-and-approval-are-separate-axes.md` — one of two ADRs numbered 0010,
so it is named here in full — put Classification and Approval on separate axes and left
the reference that crosses between them as `table.column`.

Classification does not stop at columns: a table
that stores many kinds of value in one column is classified one Discriminator value at a
time, under `anonymize.tables.<table>.keys.<key>`. So the two axes addressed the data at
different resolutions, and everything the finer one said was unanswerable by the coarser.

The wordpress preset classifies eighteen `wp_usermeta` keys `keep`. An Approval could only
name `wp_usermeta.meta_value`, and `anonymize check` refused that — rightly, because the
column holding `admin_color`'s value holds every other key's value too, so approving it
would approve all eighteen at once. Declining was no way out either: these values have no
Generator claim, so there was no Classification to write in place of `keep`. Every one of
them stayed pending, and `brama anonymize review` exited 42 forever on a project where
every decision had in fact been made. A code that means "only you can do this" and is on
permanently for work nobody can do is a code nobody reads.

An Approval may therefore name a Discriminator value: `wp_usermeta.meta_key=admin_color`.
This widens the reference format and nothing else. Approval is still per destination, still
granted only by a human, still written only under `environments.<name>.anonymize.approved`,
and still says nothing about what a column means. What changes is that the answer can now
be exactly as narrow as the question — which is the property that made refusing the column
form correct in the first place.

The alternatives were both worse. Reclassifying the eighteen in the Preset would have Brama
deciding, on its own authority and for every WordPress project at once, whether
`wp_capabilities` should survive the copy — the destructive policy that ADR keeps out of a
Pull and ADR 0012 keeps out of `init`. Dropping keyed keeps from what Review hands back
would have inverted "nobody can approve this" into "this is approved", which is the
reading the separation of axes exists to prevent; and it would have fixed nothing, because
the Pull still has no fallback to offer for a key no Generator claims.

## Consequences

- A reference is `table.column` or `table.column=key`, and the two are distinct. A keyed
  Approval says nothing about a column of the same name, and a column Approval says nothing
  about any key. Matching either against the other would approve by resemblance.
- The Discriminator is spelled out in the reference rather than inferred from the table,
  so the line reads as the pair it decides and a reference naming the wrong Discriminator
  is a `check` problem rather than a silent match.
- `anonymize check` still refuses an Approval of the Discriminator or its value column, and
  now names the keyed form to write instead. A refusal that only says no leaves the reader
  where it found them.
- The interactive review offers Discriminator values like every other pending keep. A
  Preset's keys arrive inside the Preset's group, because they are shipped knowledge and
  not the project's own decision — the same rule every Preset-classified column follows.
- A keyed `keep` no Environment approves still falls back to a claiming Generator, and
  still refuses where nothing claims it. Nothing about the resolution changed; only who can
  answer it did.
