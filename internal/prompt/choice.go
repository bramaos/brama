package prompt

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"
)

// Choose puts tty into raw mode and asks for one of options on it, restoring the
// terminal on every way out as Ask does.
func Choose(tty *os.File, out io.Writer, title string, options []string) (int, error) {
	state, err := term.MakeRaw(tty.Fd())
	if err != nil {
		return 0, fmt.Errorf("putting the terminal in raw mode: %w", err)
	}
	defer func() { _ = term.Restore(tty.Fd(), state) }()

	return RunChoice(tty, out, title, options)
}

// RunChoice asks for one of options, and returns the index of the one picked.
//
// The cursor starts on the first option, so enter alone takes it: a caller puts first
// the answer that should stand when somebody is in a hurry. A cancel is ErrCancelled,
// never the option the cursor happened to be on.
func RunChoice(in io.Reader, out io.Writer, title string, options []string) (int, error) {
	if len(options) == 0 {
		return 0, ErrNoOptions
	}
	c := &choice{title: title, options: options}
	if err := run(in, out, c); err != nil {
		return 0, err
	}
	return c.cursor, nil
}

// ErrNoOptions is a choice with nothing to choose from, which no answer can satisfy.
var ErrNoOptions = errors.New("a choice needs at least one option")

// choice is a question with one answer out of a list.
type choice struct {
	title   string
	options []string

	cursor int
	// finished collapses the frame to the question and its answer, which is what stays
	// on the screen above the next question.
	finished bool
}

func (c *choice) finish() { c.finished = true }

func (c *choice) view() []string {
	if c.finished {
		return []string{c.title + "  → " + c.options[c.cursor]}
	}

	lines := []string{c.title, ""}
	for i, option := range c.options {
		if i == c.cursor {
			lines = append(lines, "> "+option)
			continue
		}
		lines = append(lines, "  "+option)
	}
	return append(lines, "", "  ↑↓ move · enter confirm · q cancel")
}

func (c *choice) handle(keys []byte) (done, cancelled bool) {
	for i := 0; i < len(keys); i++ {
		switch keys[i] {
		case 0x03, 'q', 'Q':
			return false, true
		case '\r', '\n':
			return true, false
		case 'j':
			c.move(1)
		case 'k':
			c.move(-1)
		case 0x1b:
			// An arrow key is an escape, a bracket and a letter; an escape alone leaves.
			if i+2 < len(keys) && keys[i+1] == '[' {
				switch keys[i+2] {
				case 'A':
					c.move(-1)
				case 'B':
					c.move(1)
				}
				i += 2
				continue
			}
			return false, true
		}
	}
	return false, false
}

// move walks the cursor, wrapping at both ends like the checklist's.
func (c *choice) move(by int) {
	c.cursor = (c.cursor + by + len(c.options)) % len(c.options)
}
