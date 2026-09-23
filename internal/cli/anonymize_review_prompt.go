package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/preset"
	"github.com/bramaos/brama/internal/prompt"
)

// asker asks one checklist and returns one answer per item.
//
// It is a seam rather than a call for the same reason schemaSource is: the terminal is
// the part of `review` that cannot happen everywhere `review` runs, and the command has
// to behave the same either way. A nil asker is the ordinary case — a CI runner, an
// agent, a pipe — and it is not a failure.
type asker func(title string, items []prompt.Item) ([]bool, error)

// decisions is what one interactive review came back with: the Approvals somebody
// granted, and the Classifications their declines wrote.
//
// The two halves go into different blocks of brama.yaml and mean different things. An
// Approval says this destination may receive real values of a column that still says
// `keep`; an amendment says the column no longer says `keep` at all. Merging them would
// be recording a project-wide decision as a per-destination one.
type decisions struct {
	// approvals is in the order the questions were asked, so the diff reads in the order
	// the person answered.
	approvals []approval
	// amendments are the Classifications the declines wrote — the answer each line
	// showed before the decision was made.
	amendments []config.Amendment
	// kept are the `keep`s written for keys nothing classified, where somebody approved
	// one for a destination. An Approval of a key the file does not keep approves
	// nothing, so each is the approval's precondition rather than a decision of its own,
	// and recorded does not count it twice.
	kept []config.Amendment
}

// recorded is how many decisions this run wrote down. A nil decisions is nobody's
// answer — no terminal, no pending half, or somebody who left — and is zero of them.
func (d *decisions) recorded() int {
	if d == nil {
		return 0
	}
	out := len(d.amendments)
	for _, granted := range d.approvals {
		out += len(granted.refs)
	}
	return out
}

// wrote reports whether this run recorded a Classification for a column.
//
// It exists because declining a column the Preset keeps writes an answer stricter than
// the Preset's, which is by definition a Preset loosening — so the re-read finds the
// decision that was just made and reports it back as one still waiting to be made.
// Naming a person's own answer as the work left to do is the one thing this output must
// not do.
func (d *decisions) wrote(table, column, key string) bool {
	if d == nil {
		return false
	}
	for _, a := range d.amendments {
		if a.Table == table && a.Column == column && a.Key == key {
			return true
		}
	}
	return false
}

type approval struct {
	environment string
	refs        []config.ColumnRef
}

// question is one line of a checklist and the columns it decides.
//
// A line usually decides one column. A Preset's keeps are one line deciding all of them:
// a hundred checkboxes before anybody has pulled anything makes accept-all the only
// realistic answer, which authorizes exactly as blindly as trusting the Preset would
// have.
type question struct {
	item    prompt.Item
	columns anonymize.Fallbacks
	// key is the Discriminator value nothing classifies that this line decides, and nil
	// on a line about kept columns.
	key *anonymize.UncoveredKey
}

// askReview puts the pending half to whoever is at the keyboard, one checklist per
// destination Environment and one for the Preset loosenings the project holds.
//
// A nil result is nobody's answer — they cancelled, or walked away — and the caller
// carries on with the run it would have done with nobody there. It is never a decline:
// reading "they said no to all of these" off somebody leaving the terminal would write a
// hundred classifications nobody chose.
func askReview(ask asker, own *config.Config, review anonymize.Review, environments []string) (*decisions, error) {
	out := &decisions{}

	// A column is one Classification project-wide and one Approval per destination, so
	// what it is declined to depends on every destination that was asked. Counted here,
	// resolved once every question has been answered.
	counted := map[string]*count{}
	var order []string
	// A key nothing classifies is kept where any destination approves it, and left
	// Unclassified where none does. Collected in the order it was first approved.
	approvedKeys := map[string]bool{}

	for _, name := range environments {
		questions := environmentQuestions(own, review.Unapproved.For(name))
		questions = append(questions, keyQuestions(review.UnclassifiedKeys)...)
		if len(questions) == 0 {
			continue
		}

		answers, err := ask(fmt.Sprintf("Approve exposing real data to `%s`?", name), items(questions))
		if errors.Is(err, prompt.ErrCancelled) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}

		granted := approval{environment: name}
		for i, q := range questions {
			if q.key != nil {
				if !answers[i] {
					continue
				}
				granted.refs = append(granted.refs, keyRef(*q.key))
				if !approvedKeys[q.key.String()] {
					approvedKeys[q.key.String()] = true
					out.kept = append(out.kept, keep(*q.key))
				}
				continue
			}
			for _, column := range q.columns {
				// Keyed by the printable name, which is the one thing unique across both
				// forms: a table can hold a column and a Discriminator value under the
				// same word, and counting them as one would let an answer about either
				// stand in for the other.
				at := column.Column
				if _, seen := counted[at]; !seen {
					counted[at] = &count{fallback: column}
					order = append(order, at)
				}
				counted[at].offered++

				if !answers[i] {
					continue
				}
				counted[at].approved++
				granted.refs = append(granted.refs, column.Ref())
			}
		}
		if len(granted.refs) > 0 {
			out.approvals = append(out.approvals, granted)
		}
	}

	for _, at := range order {
		if amendment, declined := counted[at].declined(own); declined {
			out.amendments = append(out.amendments, amendment)
		}
	}

	held, err := askHeld(ask, own.Anonymize.Preset, review.Held)
	if err != nil {
		return nil, err
	}
	if held == nil {
		return nil, nil
	}
	out.amendments = append(out.amendments, held...)

	return out, nil
}

// count is one column across every destination it was asked about.
type count struct {
	fallback anonymize.Fallback
	offered  int
	approved int
}

// declined is the Classification to write, and whether this column was declined
// everywhere — by whoever just answered, and by the file they answered against.
//
// Everywhere, because a Classification is project-wide. A column approved for staging
// and declined for a laptop still says `keep`: writing the laptop's answer into the file
// would revoke staging's approval by a side effect of answering a different question, and
// leave an approval of a column nothing keeps. The laptop receives the fabricated value
// anyway, which is the substitution doing its job.
//
// The file is read as well as the answers, and not only the Environments this run asked
// about. A destination that already approved the column never appears in the pending set
// at all — that is Effective doing its job — so a run narrowed with `--env` would
// otherwise revoke an approval it was never shown.
func (c *count) declined(cfg *config.Config) (config.Amendment, bool) {
	// Nothing to write for a column no Generator claims: brama has no answer to offer,
	// and choosing between emptying the column and preserving it is the judgement ADR
	// 0012 keeps out of `init`.
	if c.approved > 0 || c.fallback.Action == "" || approvedAnywhere(cfg, c.fallback) {
		return config.Amendment{}, false
	}
	amendment := config.Amendment{Table: c.fallback.Table, Action: c.fallback.Action}
	if c.fallback.Keyed() {
		// The table's own two facts travel with the key, so recording it in a file that
		// classifies the table nowhere yet writes a table brama can read back.
		amendment.Key = c.fallback.Field
		amendment.Discriminator = c.fallback.Discriminator
		amendment.Value = c.fallback.Value
	} else {
		amendment.Column = c.fallback.Field
	}
	return amendment, true
}

// approvedAnywhere reports whether any destination the file declares may receive this
// column's real values — including the ones this run was never asked about.
func approvedAnywhere(cfg *config.Config, f anonymize.Fallback) bool {
	for _, env := range cfg.Environments {
		if env.ApprovesRef(f.Ref()) {
			return true
		}
	}
	return false
}

// environmentQuestions is one destination's pending keeps as lines somebody can answer:
// the columns the project classified itself, one line each, and everything the Preset
// carries as one line more.
func environmentQuestions(own *config.Config, pending anonymize.Fallbacks) []question {
	var questions []question
	var shipped anonymize.Fallbacks

	for _, f := range pending {
		// A Discriminator value is offered like any other pending keep. It is approved
		// by its key — `wp_usermeta.meta_key=admin_color` — so ticking it answers for
		// that key and for no other, which is a question somebody can answer.
		if !classifiedInFile(own.Anonymize, f) {
			shipped = append(shipped, f)
			continue
		}
		questions = append(questions, question{item: line(f), columns: anonymize.Fallbacks{f}})
	}

	if len(shipped) > 0 {
		questions = append(questions, presetQuestion(own.Anonymize.Preset, shipped))
	}
	return questions
}

// keyQuestions are the Discriminator values nothing classifies, one line each. Ticking
// one approves exposing it as real data, which is the question the checklist asks;
// declining has no answer to fall back on, so the key stays Unclassified. A key the file
// cannot name is not offered, since ticking it would write a prefix, and it stays pending.
func keyQuestions(keys []anonymize.UncoveredKey) []question {
	out := make([]question, 0, len(keys))
	for i := range keys {
		if !keys[i].Nameable() {
			continue
		}
		out = append(out, question{
			item: prompt.Item{
				Label: keys[i].String(),
				Note:  "nothing classifies it — declining leaves it unclassified",
			},
			key: &keys[i],
		})
	}
	return out
}

// keyRef is the Approval of one Discriminator value, named by its key.
func keyRef(k anonymize.UncoveredKey) config.ColumnRef {
	return config.ColumnRef{Table: k.Table, Column: k.Discriminator, Key: config.SpellKey(k.Value)}
}

// keep is the Classification an approved key nothing classified is written with, and the
// Discriminator and value beside it where the file's table does not name them yet.
func keep(k anonymize.UncoveredKey) config.Amendment {
	return config.Amendment{
		Table: k.Table, Key: k.Value, Discriminator: k.Discriminator, Value: k.Selects.Name, Action: config.Keep,
	}
}

// line is one column as the checklist shows it: what it is, and what stands if it is left
// unticked. Declining is then never a leap.
func line(f anonymize.Fallback) prompt.Item {
	item := prompt.Item{Label: f.Column, Detail: string(f.Action)}
	if !f.Substituted() {
		item.Note = "no generator claims it — declining leaves it for you"
	}
	return item
}

// presetQuestion is a Preset's keeps as one auditable decision, openable to the full
// column list before it is made.
func presetQuestion(name string, shipped anonymize.Fallbacks) question {
	members := make([]string, 0, len(shipped))
	for _, f := range shipped {
		if f.Substituted() {
			members = append(members, f.Column+" → "+string(f.Action))
			continue
		}
		members = append(members, f.Column)
	}
	return question{
		item: prompt.Item{
			Label:   fmt.Sprintf("%s preset — %s kept as real data", name, plural(len(shipped), "column")),
			Members: members,
		},
		columns: shipped,
	}
}

// classifiedInFile reports whether this column's Classification is the project's own
// rather than the Preset's.
//
// It reads the file as written and not as resolved, which is the whole distinction: a
// column the project wrote down is a decision somebody made and belongs on a line of its
// own, and a column the Preset carries is shipped knowledge and belongs in the group.
func classifiedInFile(own *config.Anonymize, f anonymize.Fallback) bool {
	if own == nil {
		return false
	}
	table, known := own.Tables[f.Table]
	if !known {
		return false
	}
	if f.Keyed() {
		_, keyed := table.Keys[f.Field]
		return keyed
	}
	_, column := table.Columns[f.Field]
	return column
}

// askHeld puts the Preset loosenings to the same person, project-wide.
//
// Separately, and not merged into a destination's list. "May this destination receive
// the real values of a column we keep?" and "should this column be kept at all?" are
// different questions with different blast radii, and a single list of ticks would have
// somebody answer one while reading the other.
//
// A nil result is a cancel, the same as everywhere else here.
func askHeld(ask asker, name string, held []preset.Drift) ([]config.Amendment, error) {
	if len(held) == 0 {
		return []config.Amendment{}, nil
	}

	items := make([]prompt.Item, 0, len(held))
	for _, d := range held {
		items = append(items, prompt.Item{
			Label:  d.Name,
			Detail: string(d.Shipped),
			Note:   "your file says " + string(d.Recorded),
		})
	}

	answers, err := ask(fmt.Sprintf("Accept the %s preset's looser classification?", name), items)
	if errors.Is(err, prompt.ErrCancelled) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	out := []config.Amendment{}
	for i, d := range held {
		if !answers[i] {
			continue
		}
		out = append(out, config.Amendment{
			Table:  d.Table,
			Column: d.Column,
			Key:    d.Key,
			Action: d.Shipped,
			// The whole of the Preset's answer, for the reason a tightening carries it:
			// a column accepted into the Preset's classification and out of the group it
			// correlates with breaks the joins the group exists to keep.
			Correlate: d.Correlate,
		})
	}
	return out, nil
}

func items(questions []question) []prompt.Item {
	out := make([]prompt.Item, 0, len(questions))
	for _, q := range questions {
		out = append(out, q.item)
	}
	return out
}

// applyDecisions writes the automatic half and what somebody just decided, in one write.
//
// One write, because brama.yaml is one file and a run that wrote the approvals and then
// failed to write the classifications would leave a project approving columns against a
// record that no longer says what it was approved against.
func applyDecisions(path string, review anonymize.Review, d *decisions) (bool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", filepath.Base(path), err)
	}

	amendments := slices.Concat(review.Amendments(), d.kept, d.amendments)
	updated, err := config.AmendAnonymize(body, amendments)
	if err != nil {
		return false, fmt.Errorf("recording the review in %s: %w", filepath.Base(path), err)
	}
	for _, granted := range d.approvals {
		updated, err = config.Approve(updated, granted.environment, granted.refs)
		if err != nil {
			return false, fmt.Errorf("recording the approval in %s: %w", filepath.Base(path), err)
		}
	}

	// A review where somebody ticked nothing and nothing had an answer to fall back on
	// decided something — it just decided nothing had to change. It must not dirty a
	// checkout to say so.
	if bytes.Equal(updated, body) {
		return false, nil
	}

	//nolint:gosec // G703: path is config.Load's result, not caller-supplied.
	if err := os.WriteFile(path, updated, config.FileMode); err != nil {
		return false, fmt.Errorf("writing %s: %w", filepath.Base(path), err)
	}
	return true, nil
}
