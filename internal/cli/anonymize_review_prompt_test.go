package cli

import (
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/prompt"
	"github.com/bramaos/brama/internal/refusal"
)

// kept is a classification the project wrote itself: one `keep` no destination approves,
// and a generator that claims the column by name.
const kept = `anonymize:
  tables:
    users:
      columns:
        # Somebody decided this on purpose.
        email:
          action: keep
        internal_note:
          action: drop
`

// answered stands in for the terminal. It records every question it was asked and ticks
// whatever tick says to tick, so a test says what somebody did rather than which bytes
// they typed.
type answered struct {
	tick   func(title string, item prompt.Item) bool
	err    error
	titles []string
	asked  [][]prompt.Item
}

func (a *answered) ask(title string, items []prompt.Item) ([]bool, error) {
	a.titles = append(a.titles, title)
	a.asked = append(a.asked, items)
	if a.err != nil {
		return nil, a.err
	}

	out := make([]bool, len(items))
	for i, item := range items {
		out[i] = a.tick != nil && a.tick(title, item)
	}
	return out, nil
}

// nobodyTicks is somebody who read every line and approved none of them.
func nobodyTicks() *answered { return &answered{} }

// everything a checklist was asked, flattened, for the assertions that are about what
// was put to the person rather than about which destination it was put for.
func (a *answered) everything() []prompt.Item {
	var out []prompt.Item
	for _, items := range a.asked {
		out = append(out, items...)
	}
	return out
}

// Ticking records an Approval for that destination and no other. Approval is per
// destination, and the same column is two questions where it lands in two places.
func TestReviewRecordsAnApprovalForTheEnvironmentThatGrantedIt(t *testing.T) {
	root := classifiedProject(t, kept)
	env, _, _ := testEnv()
	env.Ask = (&answered{tick: func(title string, _ prompt.Item) bool {
		return strings.Contains(title, "`local`")
	}}).ask

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err == nil {
		t.Fatal("runAnonymizeReview() = nil, want staging still waiting")
	}

	cfg := written(t, root)
	if !cfg.Environments["local"].Approves("users", "email") {
		t.Error("local does not approve users.email, which is what was ticked")
	}
	if cfg.Environments["staging"].Approves("users", "email") {
		t.Error("staging approves users.email, which nobody granted it")
	}
}

// Declining writes the Classification the line showed, so the decision lands in the file
// rather than being re-asked on every run.
func TestReviewWritesTheShownClassificationWhenDeclined(t *testing.T) {
	root := classifiedProject(t, kept)
	env, _, _ := testEnv()
	env.Ask = nobodyTicks().ask

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeReview() = %v, want nothing left pending", err)
	}

	if got := written(t, root).Anonymize.Tables["users"].Columns["email"].Action; got != "fake.email" {
		t.Errorf("users.email.action = %q, want the fallback the line showed", got)
	}
	if strings.Contains(readFile(t, root), "approved") {
		t.Errorf("a decline granted an approval:\n%s", readFile(t, root))
	}
}

// Each line shows what stands if it is left unticked, before the decision is made.
// Declining is then never a leap.
func TestReviewShowsTheFallbackBeforeTheDecision(t *testing.T) {
	root := classifiedProject(t, kept)
	env, _, _ := testEnv()
	a := nobodyTicks()
	env.Ask = a.ask

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeReview() = %v", err)
	}

	for _, item := range a.everything() {
		if item.Label == "users.email" && item.Detail == "fake.email" {
			return
		}
	}
	t.Errorf("no line showed what declining users.email writes: %+v", a.everything())
}

// One checklist per destination, because one approval is per destination.
func TestReviewAsksOnceForEachEnvironment(t *testing.T) {
	root := classifiedProject(t, kept)
	env, _, _ := testEnv()
	a := nobodyTicks()
	env.Ask = a.ask

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeReview() = %v", err)
	}

	if len(a.titles) != 2 {
		t.Fatalf("titles = %v, want one question for each destination", a.titles)
	}
	if !strings.Contains(a.titles[0], "`local`") || !strings.Contains(a.titles[1], "`staging`") {
		t.Errorf("titles = %v, want each to name the destination it is about", a.titles)
	}
	for _, title := range a.titles {
		if !strings.Contains(title, "Approve exposing real data") {
			t.Errorf("title = %q, want it to ask about exposure rather than about keeping", title)
		}
	}
}

// A column approved at one destination still says `keep`, so the file is not rewritten
// under staging's approval by somebody answering a question about a laptop.
func TestReviewLeavesAClassificationApprovedSomewhereAlone(t *testing.T) {
	root := classifiedProject(t, kept)
	env, _, _ := testEnv()
	env.Ask = (&answered{tick: func(title string, _ prompt.Item) bool {
		return strings.Contains(title, "`staging`")
	}}).ask

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err == nil {
		t.Fatal("runAnonymizeReview() = nil, want local still unapproved")
	}

	if got := written(t, root).Anonymize.Tables["users"].Columns["email"].Action; got != config.Keep {
		t.Errorf("users.email.action = %q, want the keep staging approved left standing", got)
	}
}

// A destination that already approved the column never appears in the pending set, so a
// run narrowed to one environment must not write the answer that environment gave over a
// standing approval it was never shown.
func TestReviewLeavesAClassificationApprovedOutOfThisRunAlone(t *testing.T) {
	root := projectFile(t, kept, map[string]string{"staging": "users.email"})
	env, _, _ := testEnv()
	env.Ask = nobodyTicks().ask

	if err := runAnonymizeReview(t.Context(), env, root, "local", unreachable); err == nil {
		t.Fatal("runAnonymizeReview() = nil, want local still unapproved")
	}

	cfg := written(t, root)
	if got := cfg.Anonymize.Tables["users"].Columns["email"].Action; got != config.Keep {
		t.Errorf("users.email.action = %q, want staging's approval left standing", got)
	}
	if !cfg.Environments["staging"].Approves("users", "email") {
		t.Error("staging's approval was revoked by a question about local")
	}
}

// A preset's keeps are one auditable decision rather than a hundred ticks, and one that
// can be opened before it is made.
func TestReviewPutsAPresetsKeepsAsOneExpandableItem(t *testing.T) {
	root := classifiedProject(t, drifted)
	env, _, _ := testEnv()
	a := nobodyTicks()
	env.Ask = a.ask

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err == nil {
		t.Fatal("runAnonymizeReview() = nil, want the held loosening still waiting")
	}

	asked := a.everything()
	var group *prompt.Item
	for i := range asked {
		if len(asked[i].Members) > 0 {
			group = &asked[i]
			break
		}
	}
	if group == nil {
		t.Fatalf("no line stood for the preset's keeps: %+v", asked)
	}
	if !strings.Contains(group.Label, "wordpress preset") || !strings.Contains(group.Label, "kept as real data") {
		t.Errorf("label = %q, want it to name the preset and what ticking it would expose", group.Label)
	}
	if len(group.Members) < 2 {
		t.Errorf("members = %v, want the full column list behind the one decision", group.Members)
	}
	if !strings.Contains(strings.Join(group.Members, "\n"), "wp_users.user_status") {
		t.Errorf("members = %v, want every column the group decides", group.Members)
	}
}

// An approval names a `table.column`, and the column holding a discriminator value holds
// every other key's value too — ticking one would approve all of them at once. There is
// no question here anybody could answer, so none is asked.
func TestReviewDoesNotOfferADiscriminatorKey(t *testing.T) {
	root := classifiedProject(t, drifted)
	env, _, _ := testEnv()
	a := nobodyTicks()
	env.Ask = a.ask

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err == nil {
		t.Fatal("runAnonymizeReview() = nil, want a refusal")
	}

	for _, item := range a.everything() {
		if strings.Contains(item.Label, "meta_key=") {
			t.Errorf("a discriminator key was offered as an approval: %q", item.Label)
		}
		for _, member := range item.Members {
			if strings.Contains(member, "meta_key=") {
				t.Errorf("a discriminator key was folded into a group approval: %q", member)
			}
		}
	}
}

// A preset loosening is a different question with a different blast radius — "should
// this column be kept at all?" — so it is asked on its own and not mixed into a
// destination's list.
func TestReviewAsksAboutAPresetLooseningSeparately(t *testing.T) {
	root := classifiedProject(t, drifted)
	env, _, _ := testEnv()
	a := &answered{tick: func(title string, _ prompt.Item) bool {
		return strings.Contains(title, "looser")
	}}
	env.Ask = a.ask

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err == nil {
		t.Fatal("runAnonymizeReview() = nil, want the accepted keep still unapproved")
	}

	last := a.titles[len(a.titles)-1]
	if !strings.Contains(last, "wordpress") || !strings.Contains(last, "looser") {
		t.Errorf("title = %q, want the loosening asked as its own question", last)
	}
	if got := written(t, root).Anonymize.Tables["wp_posts"].Columns["post_content"].Action; got != config.Keep {
		t.Errorf("wp_posts.post_content.action = %q, want the accepted loosening written", got)
	}
}

// Walking away is not an answer. What a cancelled run does is what a run with nobody at
// the keyboard does — which is the behaviour that was there before anybody was asked.
func TestReviewCancelledIsTheRunWithNobodyThere(t *testing.T) {
	root := classifiedProject(t, kept)
	env, _, _ := testEnv()
	env.Ask = (&answered{err: prompt.ErrCancelled}).ask

	r := refused(t, runAnonymizeReview(t.Context(), env, root, "", unreachable))

	if r.Reason != refusal.ReviewRequired {
		t.Errorf("reason = %q, want review_required", r.Reason)
	}
	if got := written(t, root).Anonymize.Tables["users"].Columns["email"].Action; got != config.Keep {
		t.Errorf("users.email.action = %q, want the file left as it was found", got)
	}
	if strings.Contains(readFile(t, root), "approved") {
		t.Errorf("a cancelled question granted an approval:\n%s", readFile(t, root))
	}
}

// The artefact is a diff somebody commits, so everything the decision is not about comes
// through byte for byte.
func TestReviewPreservesTheFileAroundAnAnsweredDecision(t *testing.T) {
	root := classifiedProject(t, kept)
	env, _, _ := testEnv()
	env.Ask = nobodyTicks().ask
	before := readFile(t, root)

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeReview() = %v", err)
	}

	after := readFile(t, root)
	if !strings.Contains(after, "# Somebody decided this on purpose.") {
		t.Errorf("a comment beside the decided column was lost:\n%s", after)
	}
	head, _, _ := strings.Cut(before, "anonymize:")
	if !strings.HasPrefix(after, head) {
		t.Errorf("the file above the block changed:\n%s", after)
	}
}

// Nothing pending is exit 0, whether it got that way with somebody at the keyboard or
// without one. The exit code says what is left, and nothing else.
func TestReviewExitsZeroOnceEverythingIsDecided(t *testing.T) {
	root := classifiedProject(t, kept)
	env, _, _ := testEnv()
	env.Ask = nobodyTicks().ask

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeReview() = %v, want success — everything was decided", err)
	}
	// The proof that matters: a second run finds nothing left to ask about.
	a := nobodyTicks()
	env.Ask = a.ask
	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("the second run = %v, want the decisions to have stuck", err)
	}
	if len(a.titles) != 0 {
		t.Errorf("the second run asked %v, want a decided file to ask nothing", a.titles)
	}
}

// Declining a column the preset keeps writes an answer stricter than the preset's, which
// is by definition a preset loosening — so the re-read finds the decision that was just
// made. Handing somebody their own answer back as the work left to do is the one thing
// this output must not do.
func TestReviewDoesNotHandBackTheDeclineItJustRecorded(t *testing.T) {
	root := classifiedProject(t, `anonymize:
  preset: wordpress
  tables:
    users:
      columns:
        email:
          action: fake.email
`)
	env, out, _ := testEnv()
	env.Ask = nobodyTicks().ask

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err == nil {
		t.Fatal("runAnonymizeReview() = nil, want the keeps nothing claims still pending")
	}

	// wp_links.link_url is kept by the preset and claimed by a generator, so declining it
	// wrote fake.url — and the preset now reads as looser than the file on that column.
	if got := written(t, root).Anonymize.Tables["wp_links"].Columns["link_url"].Action; got != "fake.url" {
		t.Fatalf("wp_links.link_url.action = %q, want the decline written", got)
	}
	if strings.Contains(out.String(), "less strictly") {
		t.Errorf("the run handed back the decision it had just recorded:\n%s", out.String())
	}
}

// With nobody there the behaviour is the one that was there before this existed: nothing
// is asked, nothing is approved, and the run hands the decisions back.
func TestReviewWithNobodyThereAsksNothing(t *testing.T) {
	root := classifiedProject(t, kept)
	env, _, _ := testEnv()

	r := refused(t, runAnonymizeReview(t.Context(), env, root, "", unreachable))

	if r.Reason != refusal.ReviewRequired {
		t.Errorf("reason = %q, want review_required", r.Reason)
	}
	if r.Fix != reviewInATerminal {
		t.Errorf("fix = %q, want it to point at the interactive run", r.Fix)
	}
	if got := written(t, root).Anonymize.Tables["users"].Columns["email"].Action; got != config.Keep {
		t.Errorf("users.email.action = %q, want the file left as it was found", got)
	}
}

// A machine gets the non-interactive run whatever terminal it happens to be attached to.
// A checklist drawn into a JSON contract is a frame of escapes in the middle of somebody's
// payload, and --non-interactive is a person asking for exactly that run.
func TestInteractiveIsNilForAMachine(t *testing.T) {
	if interactive(true, false) != nil {
		t.Error("--json got an asker, want the machine contract left alone")
	}
	if interactive(false, true) != nil {
		t.Error("--non-interactive got an asker, want no question asked")
	}
}
