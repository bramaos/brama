# One committed config file, classification included

A classified WordPress + WooCommerce project runs to sixty tables, and that Classification could
live in its own file — `brama.anonymize.yaml` — keeping `brama.yaml` short enough to read on one
screen. We put it inline instead, and let `brama init`, `brama server add` and `brama anonymize
init` all write to the same committed file, because §8 makes `brama.yaml` the place a reviewer
sees a `keep` on a column holding personal data. A second file is a second thing to find, and the
one that matters would be the one nobody opens.

Presets keep the cost down: an Adapter ships the Classification for the tables it already knows,
so what lands in the file is only what is specific to this project.

## Consequences

- Three commands write to a file a human also edits, so writes edit the YAML tree rather than
  re-marshalling it. Comments and key order survive, which is what makes `init`'s commented
  skeleton still meaningful after `server add` has run. This rules out any YAML library that
  only round-trips through structs.
- `brama.yaml` grows with the schema it classifies. A large project's config is long, and that is
  accepted — it is a review surface, not a settings file.
- The file is Desired state, owned by developers through git. `brama init` therefore refuses to
  overwrite an existing one rather than offering `--force`: a clobbered file is reviewed
  decisions destroyed, and `rm brama.yaml` is already the escape hatch, visible in the diff.
- Policy stays out of it. Anything that can edit `brama.yaml` must not be able to widen its own
  permissions, which is why Policy lives on the Server and why Reach is derived from whether an
  Environment names a Server rather than declared as a key.
- Unknown keys are rejected rather than ignored, so a typo cannot silently mean "no
  classification". That exits 1, not 42 — a broken file is a failure, not a Refusal.
