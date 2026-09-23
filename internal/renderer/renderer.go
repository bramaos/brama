// Package renderer turns a Result into output for one audience.
//
// It is the only part of brama that knows a terminal exists. The core returns Result
// values and formats nothing, which has two consequences that are the reason for the
// split: --json cannot be forgotten, because a command never had the option of
// printing prose instead; and cmd/brama-shim can link the core without linking a
// terminal UI it could never use.
//
// Nothing under core/, schema/, config/ or executor/ may import this package.
package renderer

import "github.com/bramaos/brama/internal/refusal"

// Result is what an operation returns. It carries facts, not formatting — the human
// table and the JSON object are both built from the same Fields, so they cannot
// disagree about what happened.
type Result interface {
	// Action names the operation: the `action` field of the JSON contract.
	Action() string
	// Status is how the operation ended. The Result decides it; the renderer only
	// picks a glyph, so human output can never claim success the JSON denies.
	Status() Status
	// Headline is the one-line outcome a person reads first. Human output only —
	// a machine reads the fields, not the prose.
	Headline() string
	// Fields are the facts, in the order a person should read them.
	Fields() []Field
}

// Status is how an operation ended. A Refusal is not one of these: it is not a
// Result at all, because the operation did not happen.
type Status string

const (
	// StatusSuccess is a complete operation.
	StatusSuccess Status = "success"
	// StatusPartial is an operation that did what it could. It produced something
	// usable as a starting point, but not something to act on unchanged.
	StatusPartial Status = "partial"
)

// Noted is implemented by Results carrying observations worth printing but not worth
// recording — "looks like Bedrock". They are shown to people and kept out of the
// contract, so that an observation never becomes something a caller depends on.
type Noted interface {
	Notes() []string
}

// Field is one fact about a Result.
type Field struct {
	// Key is the JSON key. Stable: it is part of the documented contract.
	Key string
	// Label is the human-facing name.
	Label string
	// Value is the fact itself.
	Value any
	// Absent is what a person is told when Value is empty, for the fields where
	// empty is a deliberate state rather than a gap — a server with no user means
	// OpenSSH decides, which is not the same as a path brama could not find. The
	// Result knows which it is; the renderer cannot guess.
	//
	// It is a rendering hint, not contract data: the JSON renderer never reads it,
	// and an empty Value is still null there.
	Absent string
	// ContractOnly keeps a Field out of the human rendering and in the JSON.
	//
	// It is for the facts a caller has to act on item by item and a person reads better
	// as prose — a list of columns is one wrapped line in a table and a paragraph in the
	// Notes. Both audiences get the same facts; neither gets them twice. A Result using
	// this owes the person a Note saying the same thing.
	ContractOnly bool
}

// Renderer writes a Result, or a Refusal, for one audience.
type Renderer interface {
	// Result writes a completed operation.
	Result(r Result) error
	// Refused writes a declined one. Separate from Error on purpose: a Refusal is
	// not a failure and must not be presented as one.
	Refused(action string, r *refusal.Refusal) error
	// Error writes a failure. It carries the action for the same reason Refused
	// does: every outcome an agent sees names the operation it came from.
	Error(action string, err error) error
}

// Fields is a small helper for building a Result's fields.
type Fields []Field

// Add appends a field whose empty value carries no special meaning.
func (f Fields) Add(key, label string, value any) Fields {
	return append(f, Field{Key: key, Label: label, Value: value})
}

// AddOptional adds a field whose empty value means something specific, and says what
// a person should be told when it is.
func (f Fields) AddOptional(key, label string, value any, absent string) Fields {
	return append(f, Field{Key: key, Label: label, Value: value, Absent: absent})
}

// AddContractOnly adds a field to the machine contract alone. It takes no label,
// because nothing renders one: the person reading the terminal is told the same thing
// in the Result's Notes, in prose.
func (f Fields) AddContractOnly(key string, value any) Fields {
	return append(f, Field{Key: key, Value: value, ContractOnly: true})
}
