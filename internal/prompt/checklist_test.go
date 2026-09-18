package prompt

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// keys drives one run of the question. The reader is a string because that is what a
// terminal sends: a stream of bytes, escape sequences and all.
func keys(t *testing.T, typed string, items []Item) ([]bool, string, error) {
	t.Helper()
	var out bytes.Buffer
	answers, err := Run(strings.NewReader(typed), &out, "Approve exposing real data to `local`?", items)
	return answers, out.String(), err
}

func columns() []Item {
	return []Item{
		{Label: "users.email", Detail: "fake.email"},
		{Label: "users.created_at"},
		{Label: "users.display_name", Detail: "fake.full_name"},
	}
}

// The whole question: tick one line, leave the rest, confirm.
func TestRunReturnsOneAnswerPerItem(t *testing.T) {
	answers, _, err := keys(t, " \r", columns())
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if len(answers) != 3 {
		t.Fatalf("answers = %v, want one per item", answers)
	}
	if !answers[0] || answers[1] || answers[2] {
		t.Errorf("answers = %v, want only the ticked line approved", answers)
	}
}

// The cursor moves, and space ticks whatever it is on. Approval is per line, and a
// question that ticked the wrong one would grant an exposure nobody chose.
func TestRunTicksTheLineTheCursorIsOn(t *testing.T) {
	answers, _, err := keys(t, "\x1b[B\x1b[B \r", columns())
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if answers[0] || answers[1] || !answers[2] {
		t.Errorf("answers = %v, want the third line approved", answers)
	}
}

// Nothing is ticked by default. A question whose safe answer needs a keystroke is one
// that grants an exposure to whoever was in a hurry.
func TestRunStartsWithNothingApproved(t *testing.T) {
	answers, _, err := keys(t, "\r", columns())
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}

	for i, approved := range answers {
		if approved {
			t.Errorf("answers[%d] = true, want nothing approved until somebody says so", i)
		}
	}
}

// Each line says what stands if it is left unticked, before the decision is made.
// Declining is then never a leap.
func TestRunShowsTheFallbackBesideTheLine(t *testing.T) {
	_, frames, err := keys(t, "\r", columns())
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if !strings.Contains(frames, "users.email") || !strings.Contains(frames, "→ fake.email") {
		t.Errorf("the question does not show what declining writes:\n%s", frames)
	}
}

// A group is one decision — and one that can be opened before it is made, so that
// accepting it is not the same as not having looked.
func TestRunExpandsAGroupToItsMembers(t *testing.T) {
	items := []Item{{
		Label:   "wordpress preset — 104 columns kept as real data",
		Members: []string{"wp_users.user_login → fake.username", "wp_users.user_url → fake.url"},
	}}

	_, frames, err := keys(t, "e\r", items)
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if !strings.Contains(frames, "(expand)") {
		t.Errorf("a group does not say it can be opened:\n%s", frames)
	}
	if !strings.Contains(frames, "wp_users.user_login → fake.username") {
		t.Errorf("expanding did not list the group's columns:\n%s", frames)
	}
}

// Ticking a group is one answer for all of it, which is the point: a hundred ticks
// before anybody has pulled anything makes accept-all the only realistic answer.
func TestRunAnswersAGroupOnce(t *testing.T) {
	items := []Item{{Label: "wordpress preset — 104 columns", Members: []string{"wp_users.user_login"}}}

	answers, _, err := keys(t, " \r", items)
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(answers) != 1 || !answers[0] {
		t.Errorf("answers = %v, want the group approved as one decision", answers)
	}
}

// Walking away is not an answer. A caller must not be able to read "they declined all of
// these" off somebody who pressed q.
func TestRunCancelsWithoutAnswering(t *testing.T) {
	for _, typed := range []string{"q", "\x1b", "\x03", ""} {
		answers, _, err := keys(t, " "+typed, columns())
		if !errors.Is(err, ErrCancelled) {
			t.Errorf("Run(%q) = %v, want it cancelled", typed, err)
		}
		if answers != nil {
			t.Errorf("Run(%q) answers = %v, want none — nobody answered", typed, answers)
		}
	}
}

// The decision stays on the screen once it is made. Whoever is about to read the diff or
// write the commit message is the next reader of this frame.
func TestRunLeavesTheAnswerOnTheScreen(t *testing.T) {
	_, frames, err := keys(t, " \r", columns())
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}

	last := frames[strings.LastIndex(frames, "\x1b[0J"):]
	if !strings.Contains(last, "[x] users.email") {
		t.Errorf("the last frame does not show what was decided:\n%s", last)
	}
	if strings.Contains(last, "enter confirm") {
		t.Errorf("the last frame still asks a question that has been answered:\n%s", last)
	}
}

// Everything above the question survives it. Whatever the person was reading when they
// decided to run brama is theirs, and is not a thing to clear away to ask them something.
func TestRunErasesOnlyItsOwnFrame(t *testing.T) {
	_, frames, err := keys(t, "\x1b[B\r", columns())
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if strings.Contains(frames, "\x1b[2J") || strings.Contains(frames, "\x1b[H") {
		t.Errorf("the question cleared the screen:\n%q", frames)
	}
	if !strings.Contains(frames, "\x1b[0J") {
		t.Errorf("the question never erased its own frame:\n%q", frames)
	}
}
