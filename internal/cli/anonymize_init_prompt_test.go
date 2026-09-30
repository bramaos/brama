package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/prompt"
	"github.com/bramaos/brama/internal/refusal"
)

// chosen is somebody at the keyboard for init's walk. It answers a question about a
// column or key named in picks with the option starting with that answer, and leaves
// everything else unclassified by taking the first option, as enter alone does.
type chosen struct {
	picks map[string]string
	// asked is every title put to them, and offered the options of each, by title.
	asked   []string
	offered map[string][]string
	// cancelAt cancels on the question with this index, as q would.
	cancelAt int
}

func (c *chosen) choose(title string, options []string) (int, error) {
	if c.offered == nil {
		c.offered = map[string][]string{}
	}
	if c.cancelAt > 0 && len(c.asked) == c.cancelAt {
		return 0, prompt.ErrCancelled
	}
	c.asked = append(c.asked, title)
	c.offered[title] = options
	for name, answer := range c.picks {
		if !strings.Contains(title, name+" ") {
			continue
		}
		for i, option := range options {
			if strings.HasPrefix(option, answer) {
				return i, nil
			}
		}
	}
	return 0, nil
}

// offeredFor is the options put in the question about name.
func (c *chosen) offeredFor(t *testing.T, name string) []string {
	t.Helper()
	for title, options := range c.offered {
		if strings.Contains(title, name+" ") {
			return options
		}
	}
	t.Fatalf("nobody was asked about %s; asked: %v", name, c.asked)
	return nil
}

// The walk: what nothing claims is asked about, one at a time, and every answer lands in
// the file beside what a generator claimed.
func TestAnonymizeInitWritesWhatSomebodyAnswered(t *testing.T) {
	root := presetlessProject(t)
	env, _, _ := testEnv()
	c := &chosen{picks: map[string]string{
		"users.internal_note": "drop",
		"orders.total_amount": "keep",
		"users.id":            "keep",
	}}
	env.Choose = c.choose

	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())); err != nil {
		t.Fatalf("runAnonymizeInit() = %v, want every column decided", err)
	}

	tables := written(t, root).Anonymize.Tables
	for column, want := range map[string]config.Classification{
		"users.internal_note": config.Drop,
		"orders.total_amount": config.Keep,
		"users.id":            config.Keep,
		"users.user_login":    "fake.username",
		"users.user_email":    "fake.email",
	} {
		table, name, _ := strings.Cut(column, ".")
		if got := tables[table].Columns[name].Action; got != want {
			t.Errorf("%s.action = %q, want %q", column, got, want)
		}
	}
}

// Only what nothing claims is asked about. A column a generator claimed is decided, and
// asking about it again would be a second, contradicting answer to the same question.
func TestAnonymizeInitAsksOnlyAboutWhatNothingClaims(t *testing.T) {
	root := presetlessProject(t)
	env, _, _ := testEnv()
	c := &chosen{}
	env.Choose = c.choose

	bootstrapped(t, runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())))

	if len(c.asked) != 3 {
		t.Fatalf("asked %d questions, want one per unclaimed column: %v", len(c.asked), c.asked)
	}
	for _, title := range c.asked {
		if strings.Contains(title, "_email") || strings.Contains(title, "user_login") {
			t.Errorf("asked %q, want no question about a claimed column", title)
		}
	}
}

// Leaving it is the first option, so enter alone never decides anything. Only generators
// that can fill the column are offered — a choice check would refuse is no choice — and
// keep comes last, as the answer that sends real data.
func TestAnonymizeInitOffersWhatCanFillTheColumn(t *testing.T) {
	root := presetlessProject(t)
	env, _, _ := testEnv()
	c := &chosen{}
	env.Choose = c.choose

	bootstrapped(t, runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())))

	options := c.offeredFor(t, "orders.total_amount")
	if !strings.HasPrefix(options[0], "leave it unclassified") {
		t.Errorf("first option = %q, want leaving it unclassified", options[0])
	}
	if !strings.HasPrefix(options[len(options)-1], "keep") {
		t.Errorf("last option = %q, want keep", options[len(options)-1])
	}
	if slices.Contains(options, "fake.email") {
		t.Errorf("options for a decimal = %v, want no fake.email, which cannot fill it", options)
	}
	if !slices.ContainsFunc(options, func(o string) bool { return strings.HasPrefix(o, "drop") }) {
		t.Errorf("options = %v, want drop among them", options)
	}

	// fake.password is the Adapter's to fill, and no Adapter can yet: offering it would
	// write a file check refuses.
	text := c.offeredFor(t, "users.internal_note")
	if !slices.Contains(text, "fake.email") || slices.Contains(text, "fake.password") {
		t.Errorf("options for a text column = %v, want fake.email and no fake.password", text)
	}
}

// Left unclassified, it is out of the file, and the run exits 42: a pull still refuses,
// and a CI runner has to be able to tell without reading prose.
func TestAnonymizeInitRefusesWhileSomethingIsLeft(t *testing.T) {
	root := presetlessProject(t)
	env, _, _ := testEnv()
	env.Choose = (&chosen{picks: map[string]string{"users.internal_note": "drop"}}).choose

	r := refused(t, runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())))

	if r.Reason != refusal.Unclassified {
		t.Errorf("reason = %q, want %q", r.Reason, refusal.Unclassified)
	}
	users := written(t, root).Anonymize.Tables["users"].Columns
	if got := users["internal_note"].Action; got != config.Drop {
		t.Errorf("users.internal_note.action = %q, want the answer written despite the refusal", got)
	}
	if _, ok := written(t, root).Anonymize.Tables["orders"].Columns["total_amount"]; ok {
		t.Error("orders.total_amount is in the file, want it left out — nobody decided it")
	}
}

// Nobody at the keyboard: nothing is asked, what generators claim is still written, and
// the rest is a refusal rather than a guess (ADR 0003).
func TestAnonymizeInitRefusesWhenNobodyIsThereToAsk(t *testing.T) {
	root := presetlessProject(t)
	env, _, _ := testEnv()

	r := refused(t, runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())))

	if r.Reason != refusal.Unclassified {
		t.Errorf("reason = %q, want %q", r.Reason, refusal.Unclassified)
	}
	if !strings.Contains(r.Detail, "human decision") {
		t.Errorf("detail = %q, want it to say why brama will not decide", r.Detail)
	}
	if got := written(t, root).Anonymize.Tables["users"].Columns["user_email"].Action; got != "fake.email" {
		t.Errorf("users.user_email.action = %q, want the generator's claim written", got)
	}
}

// Walking away keeps what was answered — each was a decision — and leaves the rest.
func TestAnonymizeInitKeepsTheAnswersGivenBeforeACancel(t *testing.T) {
	root := presetlessProject(t)
	env, _, _ := testEnv()
	c := &chosen{picks: map[string]string{
		"users.id":            "drop",
		"users.internal_note": "drop",
		"orders.total_amount": "drop",
	}, cancelAt: 1}
	env.Choose = c.choose

	r := refused(t, runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())))

	if r.Reason != refusal.Unclassified {
		t.Errorf("reason = %q, want %q", r.Reason, refusal.Unclassified)
	}
	users := written(t, root).Anonymize.Tables["users"].Columns
	if got := users["id"].Action; got != config.Drop {
		t.Errorf("users.id.action = %q, want drop — answered before the cancel", got)
	}
	if col, ok := users["internal_note"]; ok {
		t.Errorf("users.internal_note = %v, want it left out — asked after the cancel", col)
	}
}

// A key nothing claims is asked about the way a column is, and written under `keys` with
// the table's discriminator.
func TestAnonymizeInitAsksAboutTheKeysNothingClaims(t *testing.T) {
	root := unclassifiedProject(t)
	env, _, _ := testEnv()
	c := &chosen{picks: map[string]string{
		"wp_usermeta.meta_key='stripe_customer_id'": "drop",
		`wp_usermeta.meta_key=""`:                   "drop",
	}}
	env.Choose = c.choose

	if err := runAnonymizeInit(t.Context(), env, root, "", false, pluginKeys()); err != nil {
		t.Fatalf("runAnonymizeInit() = %v, want every key decided", err)
	}

	usermeta := written(t, root).Anonymize.Tables["wp_usermeta"]
	for _, key := range []string{"stripe_customer_id", ""} {
		if got := usermeta.Keys[key].Action; got != config.Drop {
			t.Errorf("wp_usermeta keys %q = %q, want drop", key, got)
		}
	}
}

// A dry run writes nothing, so it asks nothing: an answer it would throw away is a
// question wasted.
func TestAnonymizeInitDryRunAsksNothing(t *testing.T) {
	root := presetlessProject(t)
	env, _, _ := testEnv()
	c := &chosen{}
	env.Choose = c.choose

	if err := runAnonymizeInit(t.Context(), env, root, "", true, reachable("staging", wordpressish())); err != nil {
		t.Fatalf("runAnonymizeInit(--dry-run) = %v, want the preview", err)
	}
	if len(c.asked) > 0 {
		t.Errorf("asked %v, want nothing asked on a dry run", c.asked)
	}
}
