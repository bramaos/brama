// Package refusal carries the outcome where brama declines to act.
//
// A Refusal is not a failure. Nothing went wrong: a guardrail held. It travels as an
// error because that is how Go propagates an early exit, but it is rendered
// differently and it exits 42, so a caller can tell "I am not allowed" from
// "something broke" without parsing prose.
package refusal

import "errors"

// Reason is the machine-readable cause, the `reason` field of the JSON contract.
type Reason string

const (
	// Unclassified is a column, or a discriminator value, with no Classification.
	// `upward` and `policy` arrive with the commands that can raise them.
	Unclassified Reason = "unclassified"
	// Invalid is a Classification the file states and brama cannot carry out — a
	// Generator that does not exist, a `correlate` beside a `keep`, a correlation
	// group with one member. Distinct from Unclassified: a decision was written
	// down, and it is not one that can be acted on.
	Invalid Reason = "invalid_classification"
	// ReviewRequired is work left for a human: an unapproved `keep`, or a Preset
	// loosening held back from applying itself. Nothing went wrong and nothing was
	// skipped — `brama anonymize review` did the mechanical half and stopped at the
	// half that is a decision. It exits 42 like every other Refusal, which is what
	// makes a CI job fail on it without reading prose.
	ReviewRequired Reason = "review_required"
	// UnknownPrefix is a project whose table prefix brama could not read out of its own
	// config, on a project whose Preset is written against one. Nothing is wrong with
	// the classification — brama cannot tell which tables it is about, and a Preset
	// applied under a guessed prefix would classify whatever table sorted into the
	// accounts table's place. See ADR 0013 on resemblance-matching.
	UnknownPrefix Reason = "unknown_prefix"
	// NoFallback is a `keep` column the destination has not approved and no Generator
	// claims, so there is nothing to send it as. Brama may reduce exposure by
	// derivation — it may not invent destructive policy by emptying a column nobody
	// asked it to empty — so it stops and names the column instead.
	// See docs/adr/0010-classification-and-approval-are-separate-axes.md.
	NoFallback Reason = "no_fallback"
)

// Refusal is a declined operation.
type Refusal struct {
	// Reason is the stable, machine-readable cause.
	Reason Reason
	// Detail names the specific thing that caused it, for a human.
	Detail string
	// Fix is the command that resolves it. A Refusal without a way forward is a
	// dead end, so this should almost always be set.
	Fix string
}

// New builds a Refusal.
func New(reason Reason, detail, fix string) *Refusal {
	return &Refusal{Reason: reason, Detail: detail, Fix: fix}
}

func (r *Refusal) Error() string {
	if r.Detail == "" {
		return "refused: " + string(r.Reason)
	}
	return "refused: " + r.Detail
}

// As reports whether err is a Refusal, and returns it.
func As(err error) (*Refusal, bool) {
	var r *Refusal
	ok := errors.As(err, &r)
	return r, ok
}
