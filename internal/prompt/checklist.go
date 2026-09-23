// Package prompt asks a question a person answers with the keyboard.
//
// It holds one question: a checklist. Everything brama asks interactively is the same
// question asked about a list — "which of these do you approve?" — and a package that
// grew a second widget for every command would be a place where two commands ask the
// same thing in two different shapes.
//
// The model is separated from the terminal on purpose. Run reads keystrokes from any
// reader and writes frames to any writer, so the whole of the behaviour is testable
// without a pseudo-terminal; Ask is the thin part that puts a real terminal into raw
// mode and hands it to Run.
package prompt

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
)

// ErrCancelled is the answer of somebody who left the question rather than answering it.
//
// It is not an error in the sense that something broke. A caller gets it when a person
// pressed q, escape or ctrl-c, and the right response is to carry on as though nobody
// had been at the keyboard at all — which for `anonymize review` is the non-interactive
// run it would have done anyway.
var ErrCancelled = errors.New("the question was cancelled")

// Item is one line of a Checklist.
type Item struct {
	// Label is what the line is about — `users.email`.
	Label string
	// Detail is what follows the arrow: the answer that stands if this line is left
	// unticked. Empty where there is none to show.
	Detail string
	// Note is a parenthetical after the line, for the one thing a reader has to know
	// before deciding that the label and the detail do not say.
	Note string
	// Members is the full list this line stands for, revealed on expand and empty on a
	// line that stands only for itself.
	//
	// A hundred lines nobody can read is a hundred lines everybody accepts at once, so a
	// group is one decision — and one that can be opened before it is made.
	Members []string
	// Checked is whether the line starts ticked.
	Checked bool
}

// Ask puts tty into raw mode and asks the question on it.
//
// The terminal is restored on every way out, including a cancel and a read that failed
// half way through a frame: a command that leaves a shell in raw mode has broken the
// thing the person has to type the fix into.
func Ask(tty *os.File, out io.Writer, title string, items []Item) ([]bool, error) {
	state, err := term.MakeRaw(tty.Fd())
	if err != nil {
		return nil, fmt.Errorf("putting the terminal in raw mode: %w", err)
	}
	defer func() { _ = term.Restore(tty.Fd(), state) }()

	return Run(tty, out, title, items)
}

// Interactive reports whether both ends of a conversation are a terminal.
//
// Both, not either. Output redirected to a file with a keyboard still attached is a run
// whose frames nobody sees, and input from a pipe with a terminal on the other end is a
// question nobody is there to answer.
func Interactive(in, out *os.File) bool {
	return term.IsTerminal(in.Fd()) && term.IsTerminal(out.Fd())
}

// Run asks the question, and returns one answer per Item in the order they were given.
//
// A cancel is ErrCancelled and never a half-filled answer: a caller must not be able to
// read "they said no to all of these" off somebody walking away from the keyboard.
func Run(in io.Reader, out io.Writer, title string, items []Item) ([]bool, error) {
	c := &checklist{title: title, items: items}
	c.checked = make([]bool, len(items))
	c.expanded = make([]bool, len(items))
	for i, item := range items {
		c.checked[i] = item.Checked
	}

	if err := c.draw(out); err != nil {
		return nil, err
	}

	// One read can carry several keystrokes — an escape sequence is three bytes, and a
	// held key arrives in a batch — so the whole of what arrived is decoded before the
	// next frame is drawn. Drawing per byte would redraw the middle of an arrow key.
	buf := make([]byte, 256)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			done, cancelled := c.handle(buf[:n])
			c.finished = done || cancelled
			if drawErr := c.draw(out); drawErr != nil {
				return nil, drawErr
			}
			if cancelled {
				return nil, ErrCancelled
			}
			if done {
				return c.checked, nil
			}
		}
		if err != nil {
			// A closed input is somebody who left, not a failure to report: the terminal
			// went away, and there is no answer to be had either way.
			if errors.Is(err, io.EOF) {
				c.finished = true
				_ = c.draw(out)
				return nil, ErrCancelled
			}
			return nil, fmt.Errorf("reading the answer: %w", err)
		}
	}
}

// checklist is the question and everything the person has done to it so far.
type checklist struct {
	title string
	items []Item

	checked  []bool
	expanded []bool
	cursor   int
	// finished says the answer is in, which drops the cursor and the key legend from the
	// last frame. What stays on the screen afterwards is the decision itself, which is
	// what the next reader — often the person writing the commit message — needs.
	finished bool
	// drawn is how many lines the last frame took, so the next one can erase exactly it.
	drawn int
}

// draw replaces the last frame with the current one.
//
// The cursor is walked back over the previous frame and everything below it cleared,
// rather than the screen being wiped: whatever the person had in their scrollback before
// running brama is theirs, and a command that clears the terminal to ask a question has
// thrown away the output they were reading when they decided to run it.
func (c *checklist) draw(out io.Writer) error {
	var b strings.Builder
	if c.drawn > 0 {
		fmt.Fprintf(&b, "\x1b[%dA\r\x1b[0J", c.drawn)
	}

	lines := c.view()
	// Raw mode turns off the translation of \n into a carriage return, so every line
	// ends with both or the frame walks off to the right.
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\r\n")
	}
	c.drawn = len(lines)

	if _, err := io.WriteString(out, b.String()); err != nil {
		return fmt.Errorf("drawing the question: %w", err)
	}
	return nil
}

// view is the frame: the question, the lines, and how to answer it.
func (c *checklist) view() []string {
	width := 0
	for i, item := range c.items {
		// Only a line that has something after the arrow sets the column the arrows line
		// up in. A long label with nothing to its right would indent every arrow past it.
		if c.items[i].Detail != "" && len(item.Label) > width {
			width = len(item.Label)
		}
	}

	lines := []string{c.title, ""}
	for i, item := range c.items {
		lines = append(lines, c.line(i, item, width))
		if len(item.Members) > 0 && c.expanded[i] {
			for _, member := range item.Members {
				lines = append(lines, "        "+member)
			}
		}
	}
	if c.finished {
		return lines
	}
	return append(lines, "", "  space toggle · e expand · ↑↓ move · enter confirm · q cancel")
}

func (c *checklist) line(i int, item Item, width int) string {
	var b strings.Builder
	switch {
	case c.finished:
		b.WriteString("  ")
	case i == c.cursor:
		b.WriteString("> ")
	default:
		b.WriteString("  ")
	}

	if c.checked[i] {
		b.WriteString("[x] ")
	} else {
		b.WriteString("[ ] ")
	}

	b.WriteString(item.Label)
	if item.Detail != "" {
		b.WriteString(strings.Repeat(" ", width-len(item.Label)))
		b.WriteString(" → ")
		b.WriteString(item.Detail)
	}
	if item.Note != "" {
		b.WriteString("   (" + item.Note + ")")
	}
	if len(item.Members) > 0 && !c.finished {
		if c.expanded[i] {
			b.WriteString("   (collapse)")
		} else {
			b.WriteString("   (expand)")
		}
	}
	return b.String()
}

// handle applies everything one read carried, and says whether the question is over.
func (c *checklist) handle(keys []byte) (done, cancelled bool) {
	for i := 0; i < len(keys); i++ {
		switch key := keys[i]; key {
		case 0x03, 'q', 'Q':
			return false, true
		case '\r', '\n':
			return true, false
		case ' ':
			c.toggle()
		case 'e', 'E':
			c.expand(!c.expanded[c.cursor])
		case 'j':
			c.move(1)
		case 'k':
			c.move(-1)
		case 0x1b:
			// An escape with a bracket and a letter behind it is an arrow key. An escape
			// on its own is the escape key, which is a way out of the question.
			if i+2 < len(keys) && keys[i+1] == '[' {
				c.arrow(keys[i+2])
				i += 2
				continue
			}
			return false, true
		}
	}
	return false, false
}

func (c *checklist) arrow(code byte) {
	switch code {
	case 'A':
		c.move(-1)
	case 'B':
		c.move(1)
	case 'C':
		c.expand(true)
	case 'D':
		c.expand(false)
	}
}

// move walks the cursor, wrapping at both ends. A list long enough to need the cursor is
// one where getting back to the top is worth a keystroke rather than a dozen.
func (c *checklist) move(by int) {
	if len(c.items) == 0 {
		return
	}
	c.cursor = (c.cursor + by + len(c.items)) % len(c.items)
}

func (c *checklist) toggle() {
	if len(c.items) == 0 {
		return
	}
	c.checked[c.cursor] = !c.checked[c.cursor]
}

func (c *checklist) expand(open bool) {
	if len(c.items) == 0 || len(c.items[c.cursor].Members) == 0 {
		return
	}
	c.expanded[c.cursor] = open
}
