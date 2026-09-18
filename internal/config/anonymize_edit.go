package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
)

// ErrAnonymizeExists reports that the file already carries an anonymize block.
//
// Writing one is bootstrapping an empty file, not editing a full one: what is in
// there has been reviewed and committed, and a second `init` that merged into it
// would silently reopen decisions a human already made. Amending is `anonymize
// review`'s verb.
var ErrAnonymizeExists = errors.New("anonymize block already exists")

// TableClassification is the Classification of one table, ready to be written.
//
// It is ordered where Anonymize is mapped, because this side of the model is a diff
// somebody reads: columns come out in the order the table declares them, which is the
// order that reader sees in their own schema, and two runs against the same database
// produce the same bytes.
type TableClassification struct {
	Name    string
	Columns []ColumnClassification
}

// ColumnClassification is one column's Classification, and the type it was chosen
// against — written as a trailing comment, because "why fake.email here?" is answered
// by the column being a varchar(100) called email.
type ColumnClassification struct {
	Name     string
	Action   Classification
	Declared string
}

// AddAnonymize returns doc with an anonymize block written into it.
//
// The edit is surgical, like AddServer's and for the same reason: brama.yaml is a
// review surface a human also writes in, and everything outside the inserted lines
// comes through byte for byte. Where `brama init` left its commented note saying there
// is no classification yet, the block takes its place; otherwise it goes at the end.
//
// tables is written as given and nothing is sorted here. What order a Classification
// is reviewed in is the caller's decision, and it has the Schema this one came from.
func AddAnonymize(doc []byte, tables []TableClassification) ([]byte, error) {
	if len(tables) == 0 {
		// An `anonymize:` with nothing under it classifies nothing, which is the state
		// the file is already in — and `anonymize check` refuses it by name. Writing it
		// would turn "brama recognised none of your columns" into a file that looks
		// decided and is not.
		return nil, errors.New("nothing to write — no column was claimed by a generator")
	}
	if err := noAnonymizeBlock(doc); err != nil {
		return nil, err
	}

	lines := splitLines(doc)
	block := renderAnonymize(tables)

	if start, end, ok := anonymizeNote(lines); ok {
		return joinLines(replaceRange(lines, start, end, block)), nil
	}
	return joinLines(appendBlock(lines, block)), nil
}

// noAnonymizeBlock reports that the file has no anonymize block, and in doing so proves
// the file parses. A caller writing into a file brama cannot read would be appending to
// a broken tree.
//
// The line scan is not redundant with the parse. A bare `anonymize:` with nothing under
// it decodes to no block at all, and appending a second `anonymize:` key to a file that
// already has one produces a document neither brama nor YAML can read.
func noAnonymizeBlock(doc []byte) error {
	if _, err := parser.ParseBytes(doc, parser.ParseComments); err != nil {
		return fmt.Errorf("cannot read %s: %w", Filename, err)
	}

	var probe struct {
		Anonymize *Anonymize `yaml:"anonymize"`
	}
	if err := yaml.Unmarshal(doc, &probe); err != nil {
		return fmt.Errorf("cannot read %s: %w", Filename, err)
	}
	if probe.Anonymize != nil {
		return ErrAnonymizeExists
	}
	for _, line := range splitLines(doc) {
		if strings.HasPrefix(line, "anonymize:") {
			return ErrAnonymizeExists
		}
	}
	return nil
}

// renderAnonymize writes the block. Only `columns` is emitted: a key/value table's
// discriminator is a fact about rows, and `init` reads no rows.
func renderAnonymize(tables []TableClassification) []string {
	out := []string{"anonymize:", "  tables:"}
	for _, t := range tables {
		out = append(out, "    "+t.Name+":", "      columns:")
		for _, c := range t.Columns {
			out = append(out, "        "+c.Name+":")
			action := "          action: " + string(c.Action)
			if c.Declared != "" {
				action = padTo(action, actionCommentColumn) + "  # " + c.Declared
			}
			out = append(out, action)
		}
	}
	return out
}

// actionCommentColumn is where the trailing type comments line up. A longer action
// simply pushes its own comment out; the file stays valid either way.
const actionCommentColumn = 40

func padTo(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// anonymizeNote finds the commented note `brama init` writes in place of the block —
// the one saying every column is unclassified until this command runs — so the
// Classification can take its place rather than sitting underneath a sentence that is
// no longer true.
func anonymizeNote(lines []string) (start, end int, ok bool) {
	start = -1
	for i, line := range lines {
		if body, isComment := commentBody(line); isComment && strings.Contains(body, "No anonymize block yet") {
			start = i
			break
		}
	}
	if start < 0 {
		return 0, 0, false
	}

	end = start + 1
	for end < len(lines) {
		if _, isComment := commentBody(lines[end]); !isComment {
			break
		}
		end++
	}
	return start, end, true
}
