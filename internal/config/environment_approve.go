package config

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNoEnvironmentsBlock reports that there is nowhere to record an Approval: the file
// declares no Environments. An Approval names a destination, and a file with none has no
// destination to name.
var ErrNoEnvironmentsBlock = errors.New("no environments block to record an approval in")

// Approve returns doc with each ColumnRef recorded under
// `environments.<environment>.anonymize.approved`.
//
// Approval is per destination and only a human grants one, so this is only ever called
// with what somebody answered at a keyboard. It is the one write in brama that widens
// what leaves production, which is why it does exactly what it was told and nothing
// besides: a reference already in the list is left alone rather than duplicated, and
// nothing is ever removed — withdrawing an Approval is an edit a person makes in the
// file, where the diff shows it.
//
// The edit is surgical, like AmendAnonymize's and for the same reason: brama.yaml is a
// review surface a human also writes in, so everything outside the inserted lines comes
// through byte for byte. See docs/adr/0005-one-committed-config-file.md.
//
// An Environment the file does not declare is an error and never a block this creates.
// Writing an Approval for a destination that does not exist would record an exposure
// against a name nothing will ever read.
func Approve(doc []byte, environment string, refs []ColumnRef) ([]byte, error) {
	if len(refs) == 0 {
		return doc, nil
	}

	lines := splitLines(doc)
	for _, ref := range refs {
		if ref.Table == "" || ref.Column == "" {
			return nil, fmt.Errorf("an approval is a table.column reference, and %q names only half of one", ref.String())
		}
		updated, err := approve(lines, environment, ref)
		if err != nil {
			return nil, err
		}
		lines = updated
	}
	return joinLines(lines), nil
}

// approve records one reference, creating whatever part of the path the file is missing.
//
// Each pass creates at most one level and then starts again, because inserting lines
// moves every index after them — the same shape, and for the same reason, as amend.
func approve(lines []string, environment string, ref ColumnRef) ([]string, error) {
	// environments, the environment, anonymize, approved: four levels, so four passes is
	// one more than any file can need.
	for range 4 {
		updated, done, err := approveStep(lines, environment, ref)
		if err != nil {
			return nil, err
		}
		lines = updated
		if done {
			return lines, nil
		}
	}
	return nil, fmt.Errorf("could not record the approval of %s for %s in %s", ref, environment, Filename)
}

func approveStep(lines []string, environment string, ref ColumnRef) ([]string, bool, error) {
	environments, ok := topLevel(lines, "environments")
	if !ok {
		return nil, false, ErrNoEnvironmentsBlock
	}
	if err := environments.writable(lines); err != nil {
		return nil, false, err
	}
	step := environments.step(lines)

	env, ok := environments.child(lines, environment)
	if !ok {
		return nil, false, fmt.Errorf("%s declares no environment named %q, so there is no destination to approve %s for",
			Filename, environment, ref)
	}
	if err := env.writable(lines); err != nil {
		return nil, false, err
	}

	anonymize, ok := env.child(lines, "anonymize")
	if !ok {
		return insertAt(lines, anonymize.end, []string{pad(anonymize.indent) + "anonymize:"}), false, nil
	}
	if err := anonymize.writable(lines); err != nil {
		return nil, false, err
	}

	approved, ok := anonymize.child(lines, "approved")
	if !ok {
		return insertAt(lines, approved.end, []string{pad(approved.indent) + "approved:"}), false, nil
	}
	if err := approved.writable(lines); err != nil {
		return nil, false, err
	}

	end, indent, listed := sequence(lines, approved, step)
	for _, item := range listed {
		if item == ref.String() {
			// Already approved. Recording it twice would put the same decision in the
			// diff a second time, and say nothing the first one did not.
			return lines, true, nil
		}
	}
	return insertAt(lines, end, []string{pad(indent) + "- " + ref.String()}), true, nil
}

// sequence is the list written under a key: where it ends, what its items are indented
// to, and what is already in it.
//
// It is not span.child, because a sequence item is not a key. YAML lets the dashes sit
// at the key's own indentation or past it, and both are the same list — so the items are
// found by their dash rather than by an indentation the file was assumed to use.
func sequence(lines []string, key span, step int) (end, indent int, items []string) {
	indent, end = key.indent+step, key.key+1
	found := false

	for i := key.key + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || isComment(lines[i]) {
			continue
		}
		item, dashed := strings.CutPrefix(trimmed, "- ")
		if !dashed || indentOf(lines[i]) < key.indent {
			break
		}
		if !found {
			indent, found = indentOf(lines[i]), true
		}
		items = append(items, strings.TrimSpace(item))
		end = i + 1
	}
	return end, indent, items
}
