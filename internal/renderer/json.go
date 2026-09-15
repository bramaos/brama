package renderer

import (
	"encoding/json"
	"io"

	"github.com/bramaos/brama/internal/refusal"
)

// JSON writes the documented machine contract.
//
// It marshals an explicit map built from the Result's Fields rather than the Result
// struct itself, so the JSON keys are the contract and cannot drift when a struct
// field is renamed.
type JSON struct {
	Out io.Writer
}

// NewJSON returns a renderer writing the machine contract to w.
func NewJSON(w io.Writer) *JSON { return &JSON{Out: w} }

func (j *JSON) Result(r Result) error {
	payload := map[string]any{
		"action": r.Action(),
		"status": string(r.Status()),
	}
	for _, f := range r.Fields() {
		payload[f.Key] = contractValue(f.Value)
	}
	return j.write(payload)
}

// Refused writes the refusal shape: a declined operation, with the command that
// resolves it. Distinct from Error so an agent can tell the two apart structurally
// rather than by reading the message.
func (j *JSON) Refused(action string, r *refusal.Refusal) error {
	payload := map[string]any{
		"action": action,
		"status": "refused",
		"reason": string(r.Reason),
	}
	if r.Detail != "" {
		payload["detail"] = r.Detail
	}
	if r.Fix != "" {
		payload["fix"] = r.Fix
	}
	return j.write(payload)
}

func (j *JSON) Error(action string, err error) error {
	return j.write(map[string]any{
		"action": action,
		"status": "error",
		"detail": err.Error(),
	})
}

// contractValue maps a value brama could not determine to null. An empty string
// would read as "the answer is blank"; null reads as "there is no answer", which is
// what an unresolved key actually means.
func contractValue(v any) any {
	if s, ok := v.(string); ok && s == "" {
		return nil
	}
	return v
}

func (j *JSON) write(payload map[string]any) error {
	enc := json.NewEncoder(j.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}
