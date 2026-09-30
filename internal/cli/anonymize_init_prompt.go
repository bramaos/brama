package cli

import (
	"errors"
	"fmt"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/generator"
	"github.com/bramaos/brama/internal/prompt"
	"github.com/bramaos/brama/internal/schema"
)

// chooser puts one question with one answer to whoever is at the keyboard, and returns
// the index of the option picked. A test swaps it for a scripted answer.
type chooser func(title string, options []string) (int, error)

// walk asks about each column and key nothing claimed, and adds every answer to tables.
//
// One question per column, and the answers are the whole vocabulary a person can write
// by hand, narrowed to what the column can hold: a Generator that cannot fill it is not
// offered, because a choice `check` refuses is no choice. keep is last, as the answer
// that sends real production data; it enters the file here because a human put it there.
//
// A cancel ends the walk and keeps what was answered before it — each of those was a
// decision — and leaves the rest unclassified. Any other error is returned, and nothing
// is written.
func walk(choose chooser, tables []config.TableClassification, columns []anonymize.Uncovered, keys []anonymize.UncoveredKey) ([]config.TableClassification, error) {
	var askable []anonymize.UncoveredKey
	for _, k := range keys {
		// A key the file cannot name has no entry to be written in, and one whose value
		// column the Schema lacks has nothing a Generator could fill. A person decides
		// both in the file.
		if k.Nameable() && k.Selects.Name != "" {
			askable = append(askable, k)
		}
	}

	w := walker{choose: choose, tables: tables, total: len(columns) + len(askable)}
	for _, u := range columns {
		action, err := w.ask(u.String(), u.Column)
		if err != nil {
			return w.finish(err)
		}
		if action != "" {
			t := w.table(u.Table)
			t.Columns = append(t.Columns, config.ColumnClassification{
				Name: u.Column.Name, Action: action, Declared: u.Column.Declared,
			})
		}
	}
	for _, k := range askable {
		action, err := w.ask(k.String(), k.Selects)
		if err != nil {
			return w.finish(err)
		}
		if action != "" {
			t := w.table(k.Table)
			t.Discriminator, t.Value = k.Discriminator, k.Selects.Name
			t.Keys = append(t.Keys, config.ColumnClassification{Name: k.Value, Action: action})
		}
	}
	return w.tables, nil
}

// walker is one walk in progress: the classification so far, and how far through it is.
type walker struct {
	choose chooser
	tables []config.TableClassification
	asked  int
	total  int
}

// ask puts the question about one column, or one key over the column it selects, and
// returns the Classification picked — empty for leaving it.
func (w *walker) ask(name string, col schema.Column) (config.Classification, error) {
	w.asked++
	answers := answersFor(col)
	labels := make([]string, len(answers))
	for i, a := range answers {
		labels[i] = a.label
	}
	title := fmt.Sprintf("How should %s (%s) reach a copy?  %d of %d", name, col.Declared, w.asked, w.total)
	picked, err := w.choose(title, labels)
	if err != nil {
		return "", err
	}
	return answers[picked].action, nil
}

// finish ends a walk that stopped at err: a cancel keeps what was answered, anything
// else is a failure.
func (w *walker) finish(err error) ([]config.TableClassification, error) {
	if errors.Is(err, prompt.ErrCancelled) {
		return w.tables, nil
	}
	return nil, fmt.Errorf("asking how to classify: %w", err)
}

// table is the entry for name, added after the others if there is none yet — the order
// Bootstrap writes in, so an answer lands beside what a Generator claimed in that table.
func (w *walker) table(name string) *config.TableClassification {
	for i := range w.tables {
		if w.tables[i].Name == name {
			return &w.tables[i]
		}
	}
	w.tables = append(w.tables, config.TableClassification{Name: name})
	return &w.tables[len(w.tables)-1]
}

// answer is one option in the walk: what the person reads, and what it writes.
type answer struct {
	label string
	// action is empty for leaving the column unclassified.
	action config.Classification
}

// answersFor is every answer a person may give for col, in the order they are offered.
//
// Leaving it comes first, so enter alone never classifies a column: a walk somebody
// hurried through leaves what they skipped exactly as unclassified as a run nobody was at.
// fake.password is left out with every other Generator an Adapter supplies: none can
// yet, so writing it would hand `check` a Classification it refuses.
func answersFor(col schema.Column) []answer {
	answers := []answer{
		{label: "leave it unclassified — a pull refuses until it is decided"},
		{label: "drop — empty or null", action: config.Drop},
	}
	for _, g := range generator.All() {
		if g.AdapterSupplied || g.Fits(col) != nil {
			continue
		}
		answers = append(answers, answer{label: "fake." + g.Name, action: config.Classification("fake." + g.Name)})
	}
	return append(answers, answer{label: "keep — real data, sent only where a destination approves it", action: config.Keep})
}
