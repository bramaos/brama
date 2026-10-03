package prompt

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// choose drives one run of the choice, the way keys drives a checklist.
func choose(t *testing.T, typed string) (int, string, error) {
	t.Helper()
	var out bytes.Buffer
	picked, err := RunChoice(strings.NewReader(typed), &out, "How should users.retry_count reach a copy?", answers())
	return picked, out.String(), err
}

func answers() []string {
	return []string{"leave it unclassified", "drop", "fake.email", "keep"}
}

// Enter on its own takes the first option. It is the one a caller puts first because it
// is the answer that stands when somebody is in a hurry.
func TestRunChoiceTakesTheFirstOptionByDefault(t *testing.T) {
	picked, _, err := choose(t, "\r")
	if err != nil {
		t.Fatalf("RunChoice() = %v", err)
	}
	if picked != 0 {
		t.Errorf("RunChoice(enter) = %d, want 0", picked)
	}
}

func TestRunChoiceReturnsTheOptionTheCursorIsOn(t *testing.T) {
	tests := []struct {
		name  string
		typed string
		want  int
	}{
		{"arrow down", "\x1b[B\x1b[B\r", 2},
		{"j and k", "jjjk\r", 2},
		{"wraps upward", "\x1b[A\r", 3},
		{"wraps downward", "jjjj\r", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			picked, _, err := choose(t, tt.typed)
			if err != nil {
				t.Fatalf("RunChoice(%q) = %v", tt.typed, err)
			}
			if picked != tt.want {
				t.Errorf("RunChoice(%q) = %d, want %d", tt.typed, picked, tt.want)
			}
		})
	}
}

// Leaving is never an answer: a caller must not read "they chose the first option" off
// somebody walking away from the keyboard.
func TestRunChoiceCancels(t *testing.T) {
	tests := []struct {
		name  string
		typed string
	}{
		{"q", "jq"},
		{"escape", "j\x1b"},
		{"ctrl-c", "j\x03"},
		{"closed input", "j"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := choose(t, tt.typed)
			if !errors.Is(err, ErrCancelled) {
				t.Errorf("RunChoice(%q) = %v, want ErrCancelled", tt.typed, err)
			}
		})
	}
}

// The last frame keeps the question and the answer, and drops the cursor and the legend.
func TestRunChoiceLeavesTheAnswerOnScreen(t *testing.T) {
	_, out, err := choose(t, "jj\r")
	if err != nil {
		t.Fatalf("RunChoice() = %v", err)
	}

	last := out[strings.LastIndex(out, "How should"):]
	if !strings.Contains(last, "→ fake.email") {
		t.Errorf("last frame does not show the answer:\n%s", last)
	}
	if strings.Contains(last, "enter confirm") || strings.Contains(last, "drop") {
		t.Errorf("last frame still shows the options or the legend:\n%s", last)
	}
}

func TestRunChoiceShowsEveryOptionWithTheCursor(t *testing.T) {
	_, out, _ := choose(t, "j")

	for _, want := range append(answers(), "> drop", "enter confirm") {
		if !strings.Contains(out, want) {
			t.Errorf("output does not show %q:\n%s", want, out)
		}
	}
}

// A choice with nothing to pick is a caller's bug, reported rather than drawn.
func TestRunChoiceRefusesNoOptions(t *testing.T) {
	var out bytes.Buffer
	if _, err := RunChoice(strings.NewReader("\r"), &out, "How?", nil); !errors.Is(err, ErrNoOptions) {
		t.Errorf("RunChoice(no options) = %v, want ErrNoOptions", err)
	}
}
