package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/preset"
	"github.com/bramaos/brama/internal/refusal"
	"github.com/bramaos/brama/internal/renderer"
)

// AnonymizeReviewResult is what `brama anonymize review` produces with nobody at the
// keyboard.
//
// It reports two lanes and never merges them. The automatic half is what brama applied
// and wrote down, listed so that nothing lands invisibly; the pending half is what only
// a human may decide, named and not applied. A reader acting on one list as if it were
// the other would either re-do work brama has done or wave through an exposure nobody
// approved.
type AnonymizeReviewResult struct {
	Path string
	// Environments are the destinations whose approvals this run read.
	Environments []string
	// Preset names the Preset the file references, empty when it references none.
	Preset string
	// Review is the run itself, in the two lanes.
	Review anonymize.Review
	// SchemaFrom names the Environment whose Schema was compared against the file, and
	// is empty when none could be reached. A run that read no Schema cannot have found
	// a column a migration added, which is a limit it says out loud rather than a clean
	// answer it did not earn.
	SchemaFrom string
	// Written is whether the file changed. A review with nothing automatic in it is the
	// ordinary case on a project nobody has migrated, and it must not dirty a checkout.
	Written bool
	// Decided is how many decisions somebody answered at the keyboard this run.
	//
	// Zero on every run with nobody there, which is every run in CI. It is kept apart
	// from the applied count because the two are different claims: brama decided one of
	// them and a person decided the other, and a headline that summed them would report
	// an exposure somebody approved as something brama had done.
	Decided int
}

func (r *AnonymizeReviewResult) Action() string { return "anonymize_review" }

// Status is success only where the whole job is done: nothing pending, nothing left
// unclassified, and a Schema actually read. A run that reached no database did what it
// could, and what it could do is less than the whole job.
//
// So a run can be partial and still exit 0, which is `check`'s answer to the same
// question and is not a contradiction: a column nothing claims leaves the file
// incomplete, and it is not a decision this command was ever going to make. What exits
// 42 is work handed back to a person, and the exit code says that and only that.
func (r *AnonymizeReviewResult) Status() renderer.Status {
	if r.Review.Pending() || len(r.Review.Unclassified) > 0 || r.SchemaFrom == "" {
		return renderer.StatusPartial
	}
	return renderer.StatusSuccess
}

func (r *AnonymizeReviewResult) Headline() string {
	name := filepath.Base(r.Path)
	applied := plural(r.Review.Automatic(), "change")

	switch {
	// What somebody answered leads, where they answered anything. The automatic half is
	// still counted in the fields and named in the notes, but it is not the news on a run
	// where a person made decisions — those are what they are about to read in the diff.
	case r.Decided > 0 && r.Review.Pending():
		return fmt.Sprintf("recorded %s in %s — %s left, and only you can make %s",
			plural(r.Decided, "decision"), name, plural(r.pending(), "decision"), itThem(r.pending()))
	case r.Decided > 0:
		return fmt.Sprintf("recorded %s in %s — nothing is waiting on you",
			plural(r.Decided, "decision"), name)
	case r.Review.Pending() && r.Written:
		return fmt.Sprintf("applied %s to %s — %s left, and only you can make %s",
			applied, name, plural(r.pending(), "decision"), itThem(r.pending()))
	case r.Review.Pending():
		return fmt.Sprintf("nothing to apply in %s — %s left, and only you can make %s",
			name, plural(r.pending(), "decision"), itThem(r.pending()))
	case r.Written:
		return fmt.Sprintf("applied %s to %s — nothing is waiting on you", applied, name)
	default:
		return fmt.Sprintf("%s is already in agreement — nothing to apply, nothing waiting on you", name)
	}
}

// pending is how many decisions this run handed back.
//
// Decisions, not columns. Approval is per destination, so one `keep` column that lands
// at two Environments is two questions, and neither answers the other.
func (r *AnonymizeReviewResult) pending() int {
	return len(r.Review.Held) + len(r.Review.Unapproved)
}

// Notes name every column in both lanes.
//
// All of them, not a count. Whoever reads this next is about to read a diff or open the
// file, and a number is not something anybody can act on.
func (r *AnonymizeReviewResult) Notes() []string {
	notes := r.automaticNotes()
	notes = append(notes, r.pendingNotes()...)
	return append(notes, r.coverageNotes()...)
}

// automaticNotes is what was applied without asking. It is listed rather than summed
// because a change nobody approved is one everybody has to be able to see.
func (r *AnonymizeReviewResult) automaticNotes() []string {
	var notes []string
	if applied := r.Review.Tightenings; len(applied) > 0 {
		notes = append(notes, fmt.Sprintf(
			"the %s preset classifies %s more strictly than %s did — narrowing what leaves "+
				"production needs nobody's approval, so this is applied and written down:",
			r.Preset, plural(len(applied), "column"), filepath.Base(r.Path)))
		notes = append(notes, indent(applied)...)
	}
	if added := anonymize.Claimed(r.Review.New); added > 0 {
		notes = append(notes, fmt.Sprintf(
			"%s %s has that %s did not, each classified by the generator that declares a "+
				"claim on it — written, and visible in the diff:",
			plural(added, "column"), r.SchemaFrom, filepath.Base(r.Path)))
		notes = append(notes, indentAll(newColumnNames(r.Review.New))...)
	}
	return notes
}

// pendingNotes is what was not applied, and why nobody but the reader can apply it.
func (r *AnonymizeReviewResult) pendingNotes() []string {
	var notes []string
	if held := r.Review.Held; len(held) > 0 {
		notes = append(notes, fmt.Sprintf(
			"the %s preset classifies %s less strictly than %s does — sending real values is a "+
				"decision only a human makes, so the file's answer stands and nothing was written:",
			r.Preset, plural(len(held), "column"), filepath.Base(r.Path)))
		notes = append(notes, indent(held)...)
	}
	if keeps := r.Review.Unapproved; len(keeps) > 0 {
		notes = append(notes, fmt.Sprintf(
			"%s classified keep and not approved %s — approval is per destination and only "+
				"you grant one, so each is left exactly as the file has it:",
			plural(len(keeps), "column"), whereItLands(len(keeps))))
		notes = append(notes, indentAll(fallbackNames(keeps))...)
	}
	return notes
}

func (r *AnonymizeReviewResult) coverageNotes() []string {
	if r.SchemaFrom == "" {
		return []string{"no environment was reachable, so no schema was read — a column a migration " +
			"added is still unclassified, and this run cannot have found it"}
	}
	if len(r.Review.Unclassified) == 0 {
		return nil
	}
	return append(
		[]string{fmt.Sprintf(
			"no generator claims these, so brama has no answer to offer and wrote none — "+
				"a pull refuses until each one is decided in %s:", config.Filename)},
		uncoveredNames(r.Review.Unclassified)...)
}

func (r *AnonymizeReviewResult) Fields() []renderer.Field {
	// reason is the exit as the contract carries it, and it is always present so that a
	// caller reads the same shape from every run. It is the one field that says whether
	// this run finished the job or handed half of it back.
	reason := ""
	if r.Review.Pending() {
		reason = string(refusal.ReviewRequired)
	}

	return renderer.Fields{}.
		Add("file", "File", r.Path).
		Add("environments", "Environments", r.Environments).
		AddOptional("preset", "Preset", r.Preset, "none").
		AddOptional("schema", "Schema read from", r.SchemaFrom, "no environment was reachable").
		Add("written", "File changed", r.Written).
		Add("automatic_changes", "Applied", r.Review.Automatic()).
		Add("pending_decisions", "Left for you", r.pending()).
		Add("unclassified_columns", "Unclassified", len(r.Review.Unclassified)).
		AddOptional("reason", "Exit", reason, "nothing pending").
		// Four lists, never merged. Two of them are what brama did and two are what it
		// declined to do, and a caller gating a release on the wrong key would be gating
		// it on work that is already finished.
		//
		// They name the columns rather than counting them, because a caller that can
		// only count has to send a person to the repo to find out which column it was.
		// They are contract-only: the same columns reach a person through the Notes, in
		// prose that does not wrap off the screen at ten of them.
		AddContractOnly("applied_preset_tightenings", driftNames(r.Review.Tightenings)).
		AddContractOnly("applied_new_columns", newColumnNames(r.Review.New)).
		AddContractOnly("pending_preset_loosenings", driftNames(r.Review.Held)).
		AddContractOnly("pending_keeps", fallbackNames(r.Review.Unapproved))
}

// newColumnNames is one line a column — `orders.billing_email → fake.email` — in the
// shape the drift and fallback lists are already written in.
func newColumnNames(tables []config.TableClassification) []string {
	out := make([]string, 0, anonymize.Claimed(tables))
	for _, t := range tables {
		for _, c := range t.Columns {
			out = append(out, t.Name+"."+c.Name+" → "+string(c.Action))
		}
	}
	return out
}

func newAnonymizeReviewCmd(env *console) *cobra.Command {
	var only string

	cmd := &cobra.Command{
		Use:   "review",
		Short: "Reconcile the classification in brama.yaml, and hand back what needs you",
		Long: "Bring brama.yaml back into agreement with what brama would actually do, and name\n" +
			"what it will not do on its own.\n\n" +
			"Two lanes, and they are not the same lane. A preset that classifies a column more\n" +
			"strictly than the file does, and a column a migration added that a generator\n" +
			"claims, both narrow what leaves production: they need nobody's approval, they are\n" +
			"applied, and they are listed so that nothing lands invisibly.\n\n" +
			"A preset that classifies a column less strictly, and a kept column the destination\n" +
			"has not approved, both widen it. Those are decisions, only a human makes one, and\n" +
			"this command makes none of them on its own.\n\n" +
			"In a terminal it asks. One checklist per destination, one line per column, and each\n" +
			"line shows the classification that stands if you leave it unticked — so declining is\n" +
			"never a leap. Ticking records an approval for that destination; declining writes the\n" +
			"classification the line showed. A preset's keeps are one line you can open, rather\n" +
			"than a hundred ticks nobody reads. What comes out is a diff you review and commit.\n\n" +
			"With nobody at the keyboard it asks nothing: it names what is left and exits\n" +
			"review_required at 42 — nothing went wrong, work remains. That is what a CI runner\n" +
			"or an agent gets, and what --non-interactive asks for: the mechanical half done, the\n" +
			"decisions handed back.\n\n" +
			"It is the one command that writes the classification. A pull never touches a\n" +
			"tracked file, so it cannot dirty a checkout or turn a data operation into a\n" +
			"source-control event.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("finding the working directory: %w", err)
			}
			return runAnonymizeReview(cmd.Context(), env, dir, only, unreachable)
		},
	}

	cmd.Flags().StringVar(&only, "env", "", "review only this environment's approvals (default: every environment)")

	return cmd
}

// runAnonymizeReview reconciles the project rooted at or above dir.
//
// The order is the same as every other write in brama: everything decidable is decided
// before the file is touched, and the file is written once, whole.
func runAnonymizeReview(ctx context.Context, env *console, dir, only string, source schemaSource) error {
	cfg, path, err := config.Load(dir)
	if err != nil {
		return err
	}

	environments, err := environmentsFor(cfg, only)
	if err != nil {
		return err
	}

	// Amending is changing decisions that exist. A file with none is `anonymize init`'s
	// job, and doing it here would bootstrap a classification under the name of a
	// command whose whole contract is that it only ever narrows.
	if cfg.Anonymize == nil || (cfg.Anonymize.Preset == "" && len(cfg.Anonymize.Tables) == 0) {
		return refusal.New(refusal.Unclassified,
			fmt.Sprintf("%s classifies nothing — there is no decision here to review", path),
			"brama anonymize init")
	}

	read, err := inspect(ctx, cfg, filepath.Dir(path), environments, source)
	if err != nil {
		return err
	}
	// A file that contradicts itself is not one to write into. Every amendment below
	// would land beside the contradiction and leave a bigger file saying the same wrong
	// thing, with brama's fingerprints on it.
	if len(read.Problems) > 0 {
		return refusal.New(refusal.Invalid, problemDetail(read.Problems), "brama anonymize check")
	}

	review := anonymize.Plan(read.Resolved, read.Drift, environments, read.Coverage)

	// The interactive half, where there is somebody to ask and something to ask them.
	// Everything below is unchanged by it: the questions produce decisions, the decisions
	// are written with the automatic half in one write, and what is reported is what the
	// file says afterwards — re-read and re-planned, rather than predicted.
	decided, err := askPending(env, cfg, review, environments)
	if err != nil {
		return err
	}

	written := false
	if decided == nil {
		if written, err = applyReview(path, review); err != nil {
			return err
		}
	} else if review, read, written, err = decide(ctx, dir, path, environments, source, review, decided); err != nil {
		return err
	}

	result := &AnonymizeReviewResult{
		Path:         path,
		Environments: environments,
		Preset:       read.Resolved.Anonymize.Preset,
		Review:       review,
		SchemaFrom:   read.SchemaFrom,
		Written:      written,
		Decided:      decided.recorded(),
	}
	if err := env.Renderer.Result(result); err != nil {
		return fmt.Errorf("rendering the review result: %w", err)
	}

	if !review.Pending() {
		return nil
	}
	// Reported, not raised: the Result above is this run's whole account of itself, in
	// both lanes and for both audiences, and a refusal rendered after it would say a
	// second time — in the machine contract, as a second object — what it already said.
	// What is left is the exit code, which is what a CI runner actually branches on.
	return reported(refusal.New(refusal.ReviewRequired, pendingDetail(review), nextStep(env)))
}

// askPending is the interactive half, and nil where there is nobody to ask, nothing to
// ask them, or nobody who stayed to answer.
//
// Nil is the ordinary case: it is what a CI runner, an agent and a pipe all get, and it
// is what somebody who pressed q gets too. All four then run the same command.
func askPending(env *console, cfg *config.Config, review anonymize.Review, environments []string) (*decisions, error) {
	if env.Ask == nil || !review.Pending() {
		return nil, nil
	}
	return askReview(env.Ask, cfg, review, environments)
}

// decide writes what somebody answered, with the automatic half, and reads the file back.
func decide(ctx context.Context, dir, path string, environments []string, source schemaSource, review anonymize.Review, decided *decisions) (anonymize.Review, inspection, bool, error) {
	written, err := applyDecisions(path, review, decided)
	if err != nil {
		return review, inspection{}, false, err
	}
	replanned, read, err := replan(ctx, dir, environments, source)
	if err != nil {
		return review, inspection{}, false, err
	}
	// The automatic lane is carried over rather than re-derived. Applying it is exactly
	// what makes the re-read stop reporting it, and "brama wrote these without asking" is
	// true of this run whether or not it is still true of the file.
	replanned.Tightenings, replanned.New = review.Tightenings, review.New
	replanned.Held = stillHeld(replanned.Held, decided)
	return replanned, read, written, nil
}

// stillHeld drops the loosenings this run's own declines created.
//
// Declining a column the Preset keeps writes an answer stricter than the Preset's, which
// is by definition a Preset loosening — so the re-read finds the decision that was just
// made and hands it back as one still waiting to be made. What is left here is what was
// held before anybody was asked and is held still.
func stillHeld(held preset.Drifts, decided *decisions) preset.Drifts {
	var out preset.Drifts
	for _, d := range held {
		if decided.wrote(d.Table, d.Column, d.Key) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// replan reads the file back and divides it into the two lanes again.
//
// Re-read rather than predicted. What a run reports after writing has to be what is in
// the file: a review that carried forward the answer it expected would report a decision
// the file does not carry, the first time the two disagree — and this is the output
// somebody is about to commit.
func replan(ctx context.Context, dir string, environments []string, source schemaSource) (anonymize.Review, inspection, error) {
	cfg, path, err := config.Load(dir)
	if err != nil {
		return anonymize.Review{}, inspection{}, err
	}
	read, err := inspect(ctx, cfg, filepath.Dir(path), environments, source)
	if err != nil {
		return anonymize.Review{}, inspection{}, err
	}
	if len(read.Problems) > 0 {
		return anonymize.Review{}, inspection{}, refusal.New(refusal.Invalid, problemDetail(read.Problems), "brama anonymize check")
	}
	return anonymize.Plan(read.Resolved, read.Drift, environments, read.Coverage), read, nil
}

// nextStep is the way forward out of a run that still has something pending, and it
// depends on whether anybody was asked.
func nextStep(env *console) string {
	if env.Ask != nil {
		return editTheFile
	}
	return reviewInATerminal
}

// reviewInATerminal is the way forward out of a non-interactive run: the same command,
// somewhere a person can answer it.
const reviewInATerminal = "brama anonymize review    (in a terminal)"

// editTheFile is the way forward out of an interactive one. The questions have been
// asked, so pointing at the terminal again would point at the run that just happened —
// what is left is a discriminator key nothing can approve, or a column no generator
// claims, and both are edits somebody makes in the file.
const editTheFile = "decide the rest in " + config.Filename

// applyReview writes the automatic half, and reports whether the file changed.
//
// Nothing automatic means nothing written. A review of a project nobody has migrated
// must not rewrite a tracked file to say what it already said — a no-op commit is how a
// command that "only reconciles" becomes a command nobody dares run in CI.
func applyReview(path string, review anonymize.Review) (bool, error) {
	amendments := review.Amendments()
	if len(amendments) == 0 {
		return false, nil
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", filepath.Base(path), err)
	}
	updated, err := config.AmendAnonymize(body, amendments)
	if errors.Is(err, config.ErrNoAnonymizeBlock) {
		// Unreachable: the caller refused a file with no block before reaching here.
		return false, fmt.Errorf("%s classifies nothing — bootstrap it with `brama anonymize init`", filepath.Base(path))
	}
	if err != nil {
		return false, fmt.Errorf("recording the review in %s: %w", filepath.Base(path), err)
	}

	//nolint:gosec // G703: path is config.Load's result, not caller-supplied.
	if err := os.WriteFile(path, updated, config.FileMode); err != nil {
		return false, fmt.Errorf("writing %s: %w", filepath.Base(path), err)
	}
	return true, nil
}

// pendingDetail is the Refusal's own account of what is left, for the one caller that
// reads an error string rather than the contract.
func pendingDetail(review anonymize.Review) string {
	held, keeps := len(review.Held), len(review.Unapproved)
	switch {
	case held == 0:
		return fmt.Sprintf("%s classified keep that no destination approves", plural(keeps, "column"))
	case keeps == 0:
		return fmt.Sprintf("%s the preset would loosen, held for a human", plural(held, "column"))
	default:
		return fmt.Sprintf("%s classified keep that no destination approves, and %s the preset would loosen",
			plural(keeps, "column"), plural(held, "column"))
	}
}
