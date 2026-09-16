package renderer

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bramaos/brama/internal/refusal"
)

// Human writes for a person.
//
// Styling goes through lipgloss, which detects the colour profile and strips ANSI
// when output is not a terminal — so a piped run is plain text without the caller
// branching, and NO_COLOR is honoured.
type Human struct {
	Out io.Writer
	Err io.Writer
}

// NewHuman returns a renderer writing results to out and problems to err.
func NewHuman(out, err io.Writer) *Human { return &Human{Out: out, Err: err} }

var (
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	noteStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	refusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	fixStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

func (h *Human) Result(r Result) error {
	fields := r.Fields()
	width := 0
	for _, f := range fields {
		if len(f.Label) > width {
			width = len(f.Label)
		}
	}

	lipgloss.Fprintln(h.Out)
	for _, f := range fields {
		label := labelStyle.Render(pad(f.Label, width))
		lipgloss.Fprintf(h.Out, "  %s  %s\n", label, humanValue(f))
	}

	if noted, ok := r.(Noted); ok {
		for _, note := range noted.Notes() {
			lipgloss.Fprintf(h.Out, "\n  %s\n", noteStyle.Render(note))
		}
	}

	glyph := successStyle.Render("✓")
	if r.Status() == StatusPartial {
		glyph = refusedStyle.Render("!")
	}
	lipgloss.Fprintf(h.Out, "\n  %s %s\n\n", glyph, r.Headline())
	return nil
}

// Refused renders a declined operation. It is deliberately not styled as a failure:
// nothing went wrong, a guardrail held, and the output says what would clear it.
func (h *Human) Refused(_ string, r *refusal.Refusal) error {
	lipgloss.Fprintf(h.Err, "\n  %s %s\n", refusedStyle.Render("✗ refused —"), r.Detail)
	if r.Fix != "" {
		lipgloss.Fprintf(h.Err, "    %s %s\n", labelStyle.Render("run:"), fixStyle.Render(r.Fix))
	}
	lipgloss.Fprintln(h.Err)
	return nil
}

func (h *Human) Error(_ string, err error) error {
	lipgloss.Fprintf(h.Err, "\n  %s %s\n\n", errorStyle.Render("✗"), err.Error())
	return nil
}

// humanValue renders a field value for a person. A list is joined rather than shown
// in Go's bracket syntax, and an empty one says "none" — the machine contract keeps
// the empty array, because the absence of unresolved keys is itself a fact.
func humanValue(f Field) string {
	switch value := f.Value.(type) {
	case []string:
		if len(value) == 0 {
			return "none"
		}
		return strings.Join(value, ", ")
	case string:
		if value == "" {
			if f.Absent != "" {
				return f.Absent
			}
			return "not determined"
		}
		return value
	default:
		return fmt.Sprintf("%v", f.Value)
	}
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
