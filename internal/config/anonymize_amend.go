package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
)

// ErrNoAnonymizeBlock reports that there is nothing to amend: the file classifies
// nothing at all. Amending is editing decisions that exist, and a file with no block
// has none — that is `brama anonymize init`'s job, and it is a different one.
var ErrNoAnonymizeBlock = errors.New("no anonymize block to amend")

// Amendment is one column's Classification, as `brama anonymize review` writes it.
//
// It names where the answer goes rather than what to call it in a sentence. A review
// writes into `anonymize.tables.<table>.columns.<column>` or, for a Discriminator value,
// `.keys.<key>`, and those are different places in the file with the same shape inside.
type Amendment struct {
	// Table is the table the column belongs to, unqualified.
	Table string
	// Column is an ordinary column. Exactly one of Column and Key is set.
	Column string
	// Key is a Discriminator value — a row selector, not a column of its own.
	Key string
	// Discriminator and Value are the table's own two facts, carried so that a keyed
	// Amendment can be recorded in a file that classifies the table nowhere yet. They
	// are written only when the table has to be created, and neither is read back out
	// of a key: a `keys` block under a table with no `discriminator` classifies nothing,
	// and inventing one would be brama deciding which column selects the rows.
	//
	// Both are empty for an ordinary column, and a keyed Amendment without them can
	// still amend a table the file already declares a discriminator for.
	Discriminator string
	Value         string
	// Action is the Classification to record.
	Action Classification
	// Correlate is the Correlation group to record beside it, and is empty where the
	// amendment is about the action alone.
	//
	// An empty Correlate leaves whatever the entry has, rather than removing it. The one
	// answer that could carry a group and does not is `keep` or `drop`, and a `correlate`
	// beside either is a file `anonymize check` already refuses — so there is no entry
	// this could be clearing, and treating empty as "remove it" would only ever throw
	// away a decision on a file nobody could have written.
	Correlate string
	// Declared is the column's type, written as a trailing comment on an entry this
	// amendment creates. It answers "why fake.email here?" in the diff, the way `init`
	// does, and it is left alone on an entry that already exists: the comment there is
	// whatever the last writer — possibly a person — put in it.
	Declared string
}

// entry is the key an Amendment writes under, and group is the mapping that key lives
// in. A Discriminator value is keyed by its value and an ordinary column by its name.
func (a Amendment) entry() string { return a.Column + a.Key }

func (a Amendment) group() string {
	if a.Key != "" {
		return "keys"
	}
	return "columns"
}

func (a Amendment) validate() error {
	switch {
	case a.Table == "":
		return errors.New("an amendment names the table it is about")
	case a.Column == "" && a.Key == "":
		return errors.New("an amendment names a column or a discriminator key")
	case a.Column != "" && a.Key != "":
		return fmt.Errorf("%s.%s is written as both a column and a discriminator key", a.Table, a.entry())
	}
	return a.Action.Validate()
}

// AmendAnonymize returns doc with each Amendment recorded in its anonymize block.
//
// The edit is surgical, like AddServer's and AddAnonymize's and for the same reason:
// brama.yaml is a review surface a human also writes in, so everything outside the lines
// this touches comes through byte for byte — the comments, the key order, the blank line
// somebody left between two tables. Re-marshalling the document, or round-tripping it
// through the AST, reflows lines this write has no business touching.
// See docs/adr/0005-one-committed-config-file.md.
//
// An entry that exists has its `action` replaced and everything beside it — a
// `correlate`, a trailing comment — left as it was. An entry that does not is inserted,
// creating the `columns` mapping, the table, or the whole `tables` block on the way if
// the file has none. Nothing is ever removed: a review narrows what a column exposes or
// adds a column nobody had classified, and neither is a reason to drop a line somebody
// wrote.
//
// The parse still happens. It is what proves brama can read the file it is about to
// write into, and what refuses a file whose anonymize block is written inline.
func AmendAnonymize(doc []byte, amendments []Amendment) ([]byte, error) {
	if len(amendments) == 0 {
		// Not an error and not a no-op worth reporting: a review with nothing automatic
		// in it is the ordinary case on a project nobody has changed.
		return doc, nil
	}
	if err := anonymizeBlockExists(doc); err != nil {
		return nil, err
	}

	lines := splitLines(doc)
	for _, a := range amendments {
		if err := a.validate(); err != nil {
			return nil, err
		}
		updated, err := amend(lines, a)
		if err != nil {
			return nil, err
		}
		lines = updated
	}
	return joinLines(lines), nil
}

// anonymizeBlockExists is AddAnonymize's guard read the other way round, and proves the
// file parses in the same breath.
func anonymizeBlockExists(doc []byte) error {
	if _, err := parser.ParseBytes(doc, parser.ParseComments); err != nil {
		return fmt.Errorf("cannot read %s: %w", Filename, err)
	}

	var probe struct {
		Anonymize *Anonymize `yaml:"anonymize"`
	}
	if err := yaml.Unmarshal(doc, &probe); err != nil {
		return fmt.Errorf("cannot read %s: %w", Filename, err)
	}
	if probe.Anonymize == nil {
		return ErrNoAnonymizeBlock
	}
	return nil
}

// amend records one Amendment, creating whatever part of the path the file is missing.
//
// Each pass creates at most one level and then starts again, because inserting lines
// moves every index after them. Re-finding the path costs a scan of a file measured in
// hundreds of lines, and buys back the whole class of bug where an insertion invalidates
// an offset computed before it.
func amend(lines []string, a Amendment) ([]string, error) {
	// anonymize, tables, the table, the group, the entry: five levels, so five passes
	// is one more than any file can need. The bound is a guard against a step that
	// reports a change and makes none, not a budget anything is expected to spend.
	for range 5 {
		updated, done, err := amendStep(lines, a)
		if err != nil {
			return nil, err
		}
		lines = updated
		if done {
			return lines, nil
		}
	}
	return nil, fmt.Errorf("could not record %s.%s in %s", a.Table, a.entry(), Filename)
}

// amendStep writes the Amendment, or creates the one level of the path that is missing
// and reports that there is more to do.
func amendStep(lines []string, a Amendment) ([]string, bool, error) {
	anonymize, ok := topLevel(lines, "anonymize")
	if !ok {
		// Unreachable: anonymizeBlockExists decoded one. A file whose block the parser
		// sees and this scan does not would be one brama must not write into blind.
		return nil, false, fmt.Errorf("cannot find the anonymize block in %s", Filename)
	}
	if err := anonymize.writable(lines); err != nil {
		return nil, false, err
	}

	// The step the file indents by, read off the block rather than assumed, so a file
	// written with four spaces keeps being written with four.
	step := anonymize.step(lines)

	tables, ok := anonymize.child(lines, "tables")
	if !ok {
		// A project whose Preset covered everything, until a migration added a column.
		return insertAt(lines, anonymize.end, []string{pad(tables.indent) + "tables:"}), false, nil
	}
	if err := tables.writable(lines); err != nil {
		return nil, false, err
	}

	table, ok := tables.child(lines, a.Table)
	if !ok {
		if a.Key != "" && (a.Discriminator == "" || a.Value == "") {
			// A discriminated table brama would have to invent a `discriminator` and a
			// `value` for, which are facts about the table and not about this key.
			return nil, false, fmt.Errorf(
				"%s does not classify %s, and a discriminator key cannot be recorded without one", Filename, a.Table)
		}
		block := []string{pad(table.indent) + a.Table + ":"}
		if a.Key != "" {
			block = append(block,
				pad(table.indent+step)+"discriminator: "+a.Discriminator,
				pad(table.indent+step)+"value: "+a.Value)
		}
		block = append(block, pad(table.indent+step)+a.group()+":")
		block = append(block, renderEntry(table.indent+2*step, step, a)...)
		return insertAt(lines, table.end, block), true, nil
	}
	if err := table.writable(lines); err != nil {
		return nil, false, err
	}

	// A `keys` block under a table with no `discriminator` classifies nothing, and
	// validation refuses the file it would leave behind. The table may be here because
	// somebody wrote its ordinary columns and never its discriminated half.
	if a.Key != "" {
		if _, named := table.child(lines, "discriminator"); !named {
			if a.Discriminator == "" || a.Value == "" {
				return nil, false, fmt.Errorf(
					"%s does not classify %s, and a discriminator key cannot be recorded without one", Filename, a.Table)
			}
			return insertAt(lines, table.key+1, []string{
				pad(table.indent+step) + "discriminator: " + a.Discriminator,
				pad(table.indent+step) + "value: " + a.Value,
			}), false, nil
		}
	}

	group, ok := table.child(lines, a.group())
	if !ok {
		block := append(
			[]string{pad(group.indent) + a.group() + ":"},
			renderEntry(group.indent+step, step, a)...)
		return insertAt(lines, group.end, block), true, nil
	}
	if err := group.writable(lines); err != nil {
		return nil, false, err
	}

	entry, ok := group.child(lines, a.entry())
	if !ok {
		return insertAt(lines, entry.end, renderEntry(entry.indent, step, a)), true, nil
	}
	if err := entry.writable(lines); err != nil {
		return nil, false, err
	}

	action, ok := entry.child(lines, "action")
	if !ok {
		// An entry with no action classifies nothing, which config validation refuses.
		// Writing the missing line is the only reading of "record this" that leaves the
		// file better than it found it.
		return insertAt(lines, entry.end, renderColumn(action.indent, a)), true, nil
	}

	out := append([]string{}, lines...)
	out[action.key] = setValue(lines[action.key], "action", string(a.Action))
	return correlate(out, entry, action, a), true, nil
}

// correlate records the Correlation group beside the action it belongs to.
//
// A group is part of an answer rather than a decoration on it: a column written out of
// its group still fabricates a value, and the joins that used to survive the pull stop
// surviving it. So the group goes in wherever the answer did, and it goes in next to the
// action, where a reader looking at one finds the other.
func correlate(lines []string, entry, action span, a Amendment) []string {
	if a.Correlate == "" {
		return lines
	}
	if existing, ok := entry.child(lines, "correlate"); ok {
		lines[existing.key] = setValue(lines[existing.key], "correlate", a.Correlate)
		return lines
	}
	return insertAt(lines, action.key+1, []string{pad(action.indent) + "correlate: " + a.Correlate})
}

// span is one key and the lines that belong to it.
type span struct {
	// key is the index of the line declaring the key.
	key int
	// end is one past the last line belonging to it, trailing blank lines excluded —
	// they read as separating this block from whatever follows, and an insertion that
	// went after them would drift away from the block it belongs to.
	end int
	// indent is the key line's indentation.
	indent int
	// rest is whatever followed the colon on the key line.
	rest string
}

// writable refuses a key whose value is written on the same line — a flow mapping, an
// anchor, a scalar where a block was expected.
//
// brama edits lines, so a block it cannot find lines for is one it must not guess at.
// Refusing says which key and leaves the file alone, which is the outcome a person can
// act on; editing around it is the outcome that silently loses a decision.
func (s span) writable(lines []string) error {
	value := strings.TrimSpace(s.rest)
	if value == "" || strings.HasPrefix(value, "#") {
		return nil
	}
	return fmt.Errorf("cannot amend %s: %s is written inline, and brama edits it a line at a time",
		Filename, strings.TrimSpace(lines[s.key]))
}

// step is how far one level of this block is indented past the last, taken from the file
// rather than assumed, so a file written with four spaces keeps being written with four.
func (s span) step(lines []string) int {
	if indent, ok := childIndent(lines, s); ok {
		return indent - s.indent
	}
	return 2
}

// child finds a direct child of this span. Direct is what makes it a search and not a
// grep: `action` under one column is not `action` under the next one.
func (s span) child(lines []string, key string) (span, bool) {
	indent, ok := childIndent(lines, s)
	if !ok {
		return span{key: s.end, end: s.end, indent: s.indent + 2}, false
	}
	for i := s.key + 1; i < s.end; i++ {
		if indentOf(lines[i]) != indent || isComment(lines[i]) {
			continue
		}
		rest, named := declares(lines[i], key)
		if !named {
			continue
		}
		return span{key: i, end: blockEnd(lines, i, s.end), indent: indent, rest: rest}, true
	}
	return span{key: s.end, end: s.end, indent: indent}, false
}

// topLevel finds a key at column zero.
func topLevel(lines []string, key string) (span, bool) {
	for i, line := range lines {
		if indentOf(line) != 0 || isComment(line) {
			continue
		}
		if rest, named := declares(line, key); named {
			return span{key: i, end: blockEnd(lines, i, len(lines)), indent: 0, rest: rest}, true
		}
	}
	return span{}, false
}

// declares reports whether a line declares this key, and returns what follows the colon.
func declares(line, key string) (string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	rest, ok := strings.CutPrefix(trimmed, key+":")
	return rest, ok
}

// blockEnd is one past the last line belonging to the key at start.
//
// A blank line or a line indented further belongs to the block; a line at or left of the
// key's own indentation ends it. Trailing blanks are handed back to whatever follows.
func blockEnd(lines []string, start, limit int) int {
	indent := indentOf(lines[start])
	end := limit
	for i := start + 1; i < limit; i++ {
		if strings.TrimSpace(lines[i]) == "" || indentOf(lines[i]) > indent {
			continue
		}
		end = i
		break
	}
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return end
}

// childIndent is the indentation this block's direct children sit at, and false when it
// has none yet.
func childIndent(lines []string, s span) (int, bool) {
	for i := s.key + 1; i < s.end; i++ {
		if strings.TrimSpace(lines[i]) == "" || isComment(lines[i]) {
			continue
		}
		return indentOf(lines[i]), true
	}
	return 0, false
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

func isComment(line string) bool {
	_, ok := commentBody(line)
	return ok
}

func pad(n int) string { return strings.Repeat(" ", n) }

// renderEntry writes one column entry — the key, and what is written under it.
func renderEntry(indent, step int, a Amendment) []string {
	return append([]string{pad(indent) + a.entry() + ":"}, renderColumn(indent+step, a)...)
}

// renderColumn writes what goes under the key: the action, with its type as a trailing
// comment where the Schema supplied one — the same line `anonymize init` would have
// written — and the Correlation group where the answer belongs to one.
func renderColumn(indent int, a Amendment) []string {
	action := pad(indent) + "action: " + string(a.Action)
	if a.Declared != "" {
		action = padTo(action, actionCommentColumn) + "  # " + a.Declared
	}

	out := []string{action}
	if a.Correlate != "" {
		out = append(out, pad(indent)+"correlate: "+a.Correlate)
	}
	return out
}

// setValue replaces the value on one key's line and leaves the rest of it alone.
//
// The trailing comment stays where it was, so a column whose type was written beside it
// keeps saying what the column is. Where the new value is long enough to reach the
// comment, the comment moves right rather than being dropped: alignment is cosmetic and
// the type is not.
func setValue(line, key, value string) string {
	written := pad(indentOf(line)) + key + ": " + value

	// Searched from after the key, so that a `#` is only read as a comment where a
	// comment is the only thing it could be. What follows one of these keys is a single
	// unquoted word — `keep`, `fake.email`, a group name — and none of them can carry one.
	after := strings.Index(line, ":") + 1
	comment := strings.Index(line[after:], " #")
	if comment < 0 {
		return written
	}

	at := after + comment
	if len(written) >= at {
		// A longer value pushes its own comment right rather than dropping it.
		// Alignment is cosmetic; the type the comment names is not.
		return written + "  " + strings.TrimLeft(line[at:], " ")
	}
	return padTo(written, at) + strings.TrimLeft(line[at:], " ")
}
