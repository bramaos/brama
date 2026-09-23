package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/refusal"
	"github.com/bramaos/brama/internal/renderer"
	"github.com/bramaos/brama/internal/schema"
)

// settled is a classification with nothing left to decide: no preset to drift from, and
// no `keep` for a destination to approve.
const settled = `anonymize:
  tables:
    users:
      columns:
        email:
          action: fake.email            # varchar(100)
        internal_note:
          action: drop
`

// drifted records a `keep` on a column the wordpress preset now fabricates, which is the
// tightening review applies, plus a `drop` on one the preset would rather keep, which is
// the loosening it holds.
const drifted = `anonymize:
  preset: wordpress
  tables:
    wp_users:
      columns:
        # Decided before the preset knew better.
        user_email:
          action: keep
    wp_posts:
      columns:
        post_content:
          action: drop
`

// settledSchema is the schema the `settled` block answers for exactly.
func settledSchema(extra ...schema.Column) schema.Schema {
	users := schema.Table{Name: "users", Columns: append([]schema.Column{
		{Name: "email", Type: "varchar", Declared: "varchar(100)", Length: 100},
		{Name: "internal_note", Type: "text", Declared: "text"},
	}, extra...)}
	return schema.Schema{Database: "acme", Tables: []schema.Table{users}}
}

func readFile(t *testing.T, root string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, config.Filename)) //nolint:gosec // G703: this test's own temp dir.
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// A preset tightening narrows what leaves production, so it needs no approval — and the
// file is written back into agreement, rather than going on saying `keep` about a column
// brama already fabricates.
func TestReviewAppliesAPresetTightening(t *testing.T) {
	root := classifiedProject(t, drifted)
	env, out, _ := testEnv()

	err := runAnonymizeReview(t.Context(), env, root, "", unreachable)

	if r := refused(t, err); r.Reason != refusal.ReviewRequired {
		t.Fatalf("reason = %q, want review_required", r.Reason)
	}
	if got := written(t, root).Anonymize.Tables["wp_users"].Columns["user_email"].Action; got != "fake.email" {
		t.Errorf("wp_users.user_email.action = %q, want the tightening written", got)
	}
	// Listed, never silent: a change nobody approved is one everybody has to see.
	if !strings.Contains(out.String(), "wp_users.user_email: keep → fake.email") {
		t.Errorf("output does not name the applied tightening:\n%s", out.String())
	}
}

// The whole of the preset's answer is written, not the action out of it. A column
// written back into agreement about what it fabricates and out of the group it
// fabricates it with leaves a group of one, which correlates nothing — and a file the
// next `anonymize check` refuses.
func TestReviewWritesTheGroupATighteningBelongsTo(t *testing.T) {
	root := classifiedProject(t, drifted)
	env, _, _ := testEnv()

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err == nil {
		t.Fatal("runAnonymizeReview() = nil, want a refusal")
	}

	if got := written(t, root).Anonymize.Tables["wp_users"].Columns["user_email"].Correlate; got != "wp_email" {
		t.Errorf("wp_users.user_email.correlate = %q, want the preset's group written with its action", got)
	}
	// The proof that matters: running it again validates the file it just wrote.
	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err != nil {
		if r, ok := refusal.As(err); !ok || r.Reason != refusal.ReviewRequired {
			t.Errorf("the second run = %v, want the file it wrote to still hold together", err)
		}
	}
}

// A preset loosening widens what leaves production. Only a human decides that, so the
// file's answer stands and nothing is written for it.
func TestReviewHoldsAPresetLoosening(t *testing.T) {
	root := classifiedProject(t, drifted)
	env, out, _ := testEnv()

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err == nil {
		t.Fatal("runAnonymizeReview() = nil, want a refusal — a loosening is waiting")
	}

	if got := written(t, root).Anonymize.Tables["wp_posts"].Columns["post_content"].Action; got != config.Drop {
		t.Errorf("wp_posts.post_content.action = %q, want the file's stricter answer standing", got)
	}
	if !strings.Contains(out.String(), "wp_posts.post_content: drop → keep") {
		t.Errorf("output does not name the held loosening:\n%s", out.String())
	}
}

// A `keep` no destination approves is the review question itself. It is named and left
// exactly as the file has it: approval is per destination, and only a human grants one.
func TestReviewNamesUnapprovedKeepsAndAppliesNothingForThem(t *testing.T) {
	root := classifiedProject(t, drifted)
	env, out, _ := testEnv()

	err := runAnonymizeReview(t.Context(), env, root, "", unreachable)

	refused(t, err)
	if strings.Contains(readFile(t, root), "approved") {
		t.Errorf("review granted an approval, which is not its to grant:\n%s", readFile(t, root))
	}
	// The preset keeps `wp_users.user_status`, and neither environment approves it.
	for _, want := range []string{"local: wp_users.user_status", "staging: wp_users.user_status"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not name %q, which needs a decision:\n%s", want, out.String())
		}
	}
}

// A column a migration added is classified by what a generator declares a claim on, in
// the same lane as a tightening: nothing is widened, and it lands in the diff.
func TestReviewClassifiesAColumnAMigrationAdded(t *testing.T) {
	root := classifiedProject(t, settled)
	env, out, _ := testEnv()
	added := schema.Column{Name: "user_url", Type: "varchar", Declared: "varchar(100)", Length: 100}

	if err := runAnonymizeReview(t.Context(), env, root, "", reachable("staging", settledSchema(added))); err != nil {
		t.Fatalf("runAnonymizeReview() = %v, want the new column classified", err)
	}

	if got := written(t, root).Anonymize.Tables["users"].Columns["user_url"].Action; got != "fake.url" {
		t.Errorf("users.user_url.action = %q, want fake.url", got)
	}
	if !strings.Contains(readFile(t, root), "# varchar(100)") {
		t.Errorf("the new entry does not say what the column is:\n%s", readFile(t, root))
	}
	if !strings.Contains(out.String(), "users.user_url → fake.url") {
		t.Errorf("output does not list the applied change:\n%s", out.String())
	}
}

// Nothing claims it, so brama has no answer to offer and writes none — the same
// judgement ADR 0012 keeps out of `init`. It is named, because a pull refuses on it.
func TestReviewLeavesAColumnNothingClaimsUnclassified(t *testing.T) {
	root := classifiedProject(t, settled)
	env, out, _ := testEnv()
	added := schema.Column{Name: "retry_count", Type: "int", Declared: "int(11)"}

	if err := runAnonymizeReview(t.Context(), env, root, "", reachable("staging", settledSchema(added))); err != nil {
		t.Fatalf("runAnonymizeReview() = %v", err)
	}

	users := written(t, root).Anonymize.Tables["users"].Columns
	if col, ok := users["retry_count"]; ok {
		t.Errorf("users.retry_count = %v, want it left out — nothing claims it", col)
	}
	if !strings.Contains(out.String(), "users.retry_count") {
		t.Errorf("output does not name the column left undecided:\n%s", out.String())
	}
}

// Nothing pending is exit 0. A CI runner branches on that and on nothing else.
func TestReviewSucceedsWhenNothingIsPending(t *testing.T) {
	root := classifiedProject(t, settled)
	env, _, _ := testEnv()

	if err := runAnonymizeReview(t.Context(), env, root, "", reachable("staging", settledSchema())); err != nil {
		t.Fatalf("runAnonymizeReview() = %v, want success — nothing is waiting on anybody", err)
	}
}

// Anything pending is `review_required` at 42. Nothing went wrong and nothing was
// skipped: the mechanical half is done, and the half that is a decision is handed back.
func TestReviewExitsReviewRequiredWhenAnythingIsPending(t *testing.T) {
	root := classifiedProject(t, drifted)
	env, _, _ := testEnv()

	r := refused(t, runAnonymizeReview(t.Context(), env, root, "", unreachable))

	if r.Reason != refusal.ReviewRequired {
		t.Errorf("reason = %q, want review_required", r.Reason)
	}
	if !strings.Contains(r.Fix, "in a terminal") {
		t.Errorf("fix = %q, want it to point at the interactive run", r.Fix)
	}
}

// The refusal is already reported — the Result said it, in both lanes and for both
// audiences. Main exits 42 on it without rendering a second account of the same run.
func TestReviewReportsItsRefusalItself(t *testing.T) {
	root := classifiedProject(t, drifted)
	env, _, _ := testEnv()

	err := runAnonymizeReview(t.Context(), env, root, "", unreachable)

	if _, ok := refusal.As(err); !ok {
		t.Fatalf("error = %v, want a refusal so the run exits 42", err)
	}
	if !errors.Is(err, ErrAlreadyReported) {
		t.Errorf("error = %v, want it marked as already reported", err)
	}
}

// A review of a project nobody has migrated must not rewrite a tracked file to say what
// it already said. A no-op commit is how a command nobody dares run in CI is made.
func TestReviewWritesNothingWhenThereIsNothingToApply(t *testing.T) {
	root := classifiedProject(t, settled)
	env, _, _ := testEnv()
	before := readFile(t, root)

	if err := runAnonymizeReview(t.Context(), env, root, "", reachable("staging", settledSchema())); err != nil {
		t.Fatalf("runAnonymizeReview() = %v", err)
	}

	if after := readFile(t, root); after != before {
		t.Errorf("the file changed with nothing to apply:\n%s", after)
	}
}

// brama.yaml is a review surface a human also writes in. Everything outside the lines
// review touches comes through byte for byte.
func TestReviewPreservesCommentsAndKeyOrder(t *testing.T) {
	root := classifiedProject(t, drifted)
	env, _, _ := testEnv()
	before := readFile(t, root)

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err == nil {
		t.Fatal("runAnonymizeReview() = nil, want a refusal")
	}

	after := readFile(t, root)
	if !strings.Contains(after, "# Decided before the preset knew better.") {
		t.Errorf("a comment beside the amended column was lost:\n%s", after)
	}
	head, _, _ := strings.Cut(before, "anonymize:")
	if !strings.HasPrefix(after, head) {
		t.Errorf("the file above the block changed:\n%s", after)
	}
	if index(after, "wp_users") > index(after, "wp_posts") {
		t.Errorf("the tables were reordered:\n%s", after)
	}
}

// A file that classifies nothing holds no decision to review. Bootstrapping one is
// `init`'s job, and doing it here under a command that only ever narrows would be a
// second way to write a classification nobody asked for.
func TestReviewRefusesAFileThatClassifiesNothing(t *testing.T) {
	root := unclassifiedProject(t)
	env, _, _ := testEnv()

	r := refused(t, runAnonymizeReview(t.Context(), env, root, "", unreachable))

	if r.Reason != refusal.Unclassified {
		t.Errorf("reason = %q, want unclassified", r.Reason)
	}
	if r.Fix != "brama anonymize init" {
		t.Errorf("fix = %q, want it to point at init", r.Fix)
	}
}

// A file that contradicts itself is not one to write into. The amendment would land
// beside the contradiction and leave a bigger file saying the same wrong thing.
func TestReviewRefusesAFileThatContradictsItself(t *testing.T) {
	root := classifiedProject(t, `anonymize:
  tables:
    users:
      columns:
        email:
          action: fake.nonexistent
`)
	env, _, _ := testEnv()
	before := readFile(t, root)

	r := refused(t, runAnonymizeReview(t.Context(), env, root, "", unreachable))

	if r.Reason != refusal.Invalid {
		t.Errorf("reason = %q, want invalid_classification", r.Reason)
	}
	if after := readFile(t, root); after != before {
		t.Errorf("review wrote into a file it could not validate:\n%s", after)
	}
}

// The machine contract carries the two lanes as four lists, never merged, and always
// present: a caller must be able to tell "the preset agreed" from "nothing looked".
func TestReviewContractNamesBothLanes(t *testing.T) {
	root := classifiedProject(t, drifted)
	env, out, _ := testEnv()
	env.Renderer, env.JSON = renderer.NewJSON(out), true

	if err := runAnonymizeReview(t.Context(), env, root, "", unreachable); err == nil {
		t.Fatal("runAnonymizeReview() = nil, want a refusal")
	}

	var payload struct {
		Status     string   `json:"status"`
		Reason     string   `json:"reason"`
		Tightened  []string `json:"applied_preset_tightenings"`
		NewColumns []string `json:"applied_new_columns"`
		Loosened   []string `json:"pending_preset_loosenings"`
		Keeps      []string `json:"pending_keeps"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("the contract does not decode: %v\n%s", err, out.String())
	}

	if payload.Reason != "review_required" {
		t.Errorf("reason = %q, want review_required", payload.Reason)
	}
	if payload.Status != string(renderer.StatusPartial) {
		t.Errorf("status = %q, want partial", payload.Status)
	}
	if len(payload.Tightened) != 1 || !strings.HasPrefix(payload.Tightened[0], "wp_users.user_email") {
		t.Errorf("applied_preset_tightenings = %v, want the one applied column", payload.Tightened)
	}
	if len(payload.Loosened) != 1 || !strings.HasPrefix(payload.Loosened[0], "wp_posts.post_content") {
		t.Errorf("pending_preset_loosenings = %v, want the one held column", payload.Loosened)
	}
	if payload.NewColumns == nil {
		t.Error("applied_new_columns = null, want an empty list — no schema is still an answer")
	}
	if len(payload.Keeps) == 0 {
		t.Error("pending_keeps is empty, want the unapproved keeps named")
	}
}

// One environment's approvals, not every environment's. A pull has one destination.
func TestReviewNarrowsToOneEnvironment(t *testing.T) {
	root := projectFile(t, drifted, map[string]string{"staging": "wp_users.user_status"})
	env, out, _ := testEnv()

	if err := runAnonymizeReview(t.Context(), env, root, "staging", unreachable); err == nil {
		t.Fatal("runAnonymizeReview() = nil, want a refusal")
	}

	if strings.Contains(out.String(), "local:") {
		t.Errorf("output reports an environment this run was not asked about:\n%s", out.String())
	}
	if strings.Contains(out.String(), "staging: wp_users.user_status") {
		t.Errorf("an approved keep is not a decision anybody still has to make:\n%s", out.String())
	}
}

// index is the 1-based line a string first appears on, for asserting key order.
func index(doc, want string) int {
	for i, line := range strings.Split(doc, "\n") {
		if strings.Contains(line, want) {
			return i + 1
		}
	}
	return -1
}
