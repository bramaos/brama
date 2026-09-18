package cli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/preset"
	"github.com/bramaos/brama/internal/refusal"
	"github.com/bramaos/brama/internal/renderer"
	"github.com/bramaos/brama/internal/schema"
)

// AnonymizeCheckResult is what `brama anonymize check` produces when the file holds
// together. The counts are the part worth printing: they are how a reader tells "this
// validated my project" from "this validated an empty block and said yes".
type AnonymizeCheckResult struct {
	Path         string
	Environments []string
	Summary      anonymize.Summary
	// Preset names the Preset the file references, and is empty when it references
	// none. The Summary counts what that Preset classifies, so saying where those
	// columns came from is what stops the counts reading as a file nobody can find.
	Preset string
	// Drift is every column the Preset and the file disagree about, and which of the two
	// brama acts on. It is empty on a project that names no Preset, and on one whose
	// record agrees with the one it names.
	Drift preset.Drifts
	// Fallbacks is what Classification and Approval resolve to together, at each of the
	// Environments this run checked: the `keep` columns none of them approves, and what
	// each would receive instead. It is derived on every run and written nowhere.
	Fallbacks anonymize.Fallbacks
	// SchemaFrom names the Environment whose Schema the classification was compared
	// against, and is empty when none could be reached.
	SchemaFrom string
	// Coverage is that comparison, and nil when SchemaFrom is empty. The two are
	// separate so that "nothing is unclassified" can never be read off a run that
	// never saw a column list.
	Coverage *anonymize.Coverage
}

func (r *AnonymizeCheckResult) Action() string { return "anonymize_check" }

// coverageState is how much of the column-coverage question one run settled.
//
// It is derived once and rendered four ways. The status, the headline, the notes and
// the fields are four accounts of the same run, and deriving the answer separately in
// each is how a headline ends up saying something the status denies.
type coverageState int

const (
	// coverageUnread is a run that reached no Environment and read no Schema.
	coverageUnread coverageState = iota
	// coverageIncomplete compared the two and found columns nothing classifies.
	coverageIncomplete
	// coverageComplete compared the two and every column is answered for.
	coverageComplete
)

// verified reports whether the comparison was a whole answer. Only then does a count
// of unclassified columns mean anything.
func (s coverageState) verified() bool {
	return s == coverageIncomplete || s == coverageComplete
}

func (r *AnonymizeCheckResult) coverage() coverageState {
	switch {
	case r.Coverage == nil:
		return coverageUnread
	case r.Coverage.Complete():
		return coverageComplete
	default:
		return coverageIncomplete
	}
}

// Status separates a check that verified column coverage from one that could not.
//
// A run with no Schema did what it could, and what it could do is less than the whole
// job: the columns a database has and the file does not are still Unclassified and a
// Pull still refuses. Reporting that as success is the one answer nobody can act on,
// so it is partial — in the machine contract as well as in the prose.
// Drift is partial for the same reason. A Preset that disagrees with the file leaves
// something for a person to do either way round — accept a loosening, or write an
// applied tightening back into the file — and a run that reported success would be
// telling a caller there was nothing left.
// So is a `keep` with no approval and nothing to fall back to: the file holds together,
// and a pull to that destination still refuses until someone decides. A substitution is
// not, because nothing is left over from one — it is the model working.
func (r *AnonymizeCheckResult) Status() renderer.Status {
	if r.coverage() == coverageComplete && len(r.Drift) == 0 && len(r.Fallbacks.NoFallback()) == 0 {
		return renderer.StatusSuccess
	}
	return renderer.StatusPartial
}

func (r *AnonymizeCheckResult) Headline() string {
	classifies := fmt.Sprintf("%s in %s",
		plural(r.Summary.Columns, "column"), plural(r.Summary.Tables, "table"))
	if r.Preset != "" {
		classifies += fmt.Sprintf(" with the %s preset", r.Preset)
	}

	switch r.coverage() {
	case coverageComplete:
		return fmt.Sprintf("%s is consistent — %s, covering every column %s has",
			filepath.Base(r.Path), classifies, r.SchemaFrom)
	case coverageIncomplete:
		return fmt.Sprintf("%s holds together, but %s of %s %s unclassified",
			filepath.Base(r.Path), plural(len(r.Coverage.Unclassified), "column"), r.SchemaFrom,
			isAre(len(r.Coverage.Unclassified)))
	default:
		return fmt.Sprintf("%s holds together — %s, against no schema",
			filepath.Base(r.Path), classifies)
	}
}

// Notes says what this run could not settle, and names what it found that the file
// does not. A clean check is not a clean bill of health unless it read a schema.
func (r *AnonymizeCheckResult) Notes() []string {
	notes := append(r.driftNotes(), r.approvalNotes()...)
	return append(notes, r.coverageNotes()...)
}

// approvalNotes says what each destination would actually receive, wherever that is not
// what the file says on its own.
//
// Kept in two blocks for the same reason drift is. A substitution is brama carrying the
// model out and needs nothing from anybody; a column with nothing to fall back to stops
// a pull that has not been written yet, and is the one thing here a person has to act
// on before it can run.
func (r *AnonymizeCheckResult) approvalNotes() []string {
	var notes []string
	if substituted := r.Fallbacks.Substituted(); len(substituted) > 0 {
		notes = append(notes, fmt.Sprintf(
			"%s classified keep %s not approved %s, so a generator stands in and "+
				"a pull sends fabricated values:",
			plural(len(substituted), "column"), isAre(len(substituted)), whereItLands(len(substituted))))
		notes = append(notes, indentAll(fallbackNames(substituted))...)
	}
	if stranded := r.Fallbacks.NoFallback(); len(stranded) > 0 {
		notes = append(notes, fmt.Sprintf(
			"%s classified keep %s neither approved nor claimed by a generator, and brama will not "+
				"empty a column on its own authority — so a pull refuses until each is approved, "+
				"or classified fake.<generator> or drop:",
			plural(len(stranded), "column"), isAre(len(stranded))))
		notes = append(notes, indentAll(fallbackNames(stranded))...)
	}
	return notes
}

// fallbackNames is the resolution as both audiences read it — one line a column, naming
// the destination it is about, because the same column resolves two ways at two of them.
func fallbackNames(fallbacks anonymize.Fallbacks) []string {
	out := make([]string, 0, len(fallbacks))
	for _, f := range fallbacks {
		out = append(out, f.String())
	}
	return out
}

func indentAll(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, "  "+line)
	}
	return out
}

func whereItLands(n int) string {
	if n == 1 {
		return "where it lands"
	}
	return "where they land"
}

// driftNotes says where the Preset and the file disagree, in two blocks that never run
// together into one list.
//
// Applied and held are opposite instructions. One says brama is already doing something
// the file does not say; the other says brama is refusing to do something the Preset
// does say. A reader scanning a single list of column names would act on the wrong half.
func (r *AnonymizeCheckResult) driftNotes() []string {
	var notes []string
	if applied := r.Drift.Applied(); len(applied) > 0 {
		notes = append(notes, fmt.Sprintf(
			"the %s preset is stricter than %s on %s — brama anonymizes %s already, and "+
				"`brama anonymize review` writes the file back into agreement:",
			r.Preset, filepath.Base(r.Path), plural(len(applied), "column"), itThem(len(applied))))
		notes = append(notes, indent(applied)...)
		// Said here rather than reported against the approval itself. An approval of a
		// column the file keeps is not a mistake — it was written beside a classification
		// that agreed with it — and refusing the run over one would stop the pull that
		// the tightening exists to make safe.
		notes = append(notes, "  an approval of one of these sends nothing while it is applied")
	}
	if held := r.Drift.Held(); len(held) > 0 {
		notes = append(notes, fmt.Sprintf(
			"the %s preset is looser than %s on %s — sending real values is a decision only "+
				"a human makes, so this is held and %s stands until `brama anonymize review` accepts it:",
			r.Preset, filepath.Base(r.Path), plural(len(held), "column"), theFilesAnswer(len(held))))
		notes = append(notes, indent(held)...)
	}
	return notes
}

// driftNames is the Drift as the contract carries it — one string a column, never null.
// No drift is an answer, and an empty list is how a caller reads one.
func driftNames(drift preset.Drifts) []string {
	out := make([]string, 0, len(drift))
	for _, d := range drift {
		out = append(out, d.String())
	}
	return out
}

func indent(drift preset.Drifts) []string {
	out := make([]string, 0, len(drift))
	for _, d := range drift {
		out = append(out, "  "+d.String())
	}
	return out
}

func itThem(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

func theFilesAnswer(n int) string {
	if n == 1 {
		return "the file's answer"
	}
	return "the file's answers"
}

func (r *AnonymizeCheckResult) coverageNotes() []string {
	switch r.coverage() {
	case coverageUnread:
		return []string{"column coverage was not verified — this run read no schema, " +
			"and a column the database has and this file does not is still unclassified"}
	case coverageIncomplete:
		notes := make([]string, 0, len(r.Coverage.Unclassified)+1)
		notes = append(notes, fmt.Sprintf("unclassified in %s — a pull refuses until each one is decided:", r.SchemaFrom))
		for _, u := range r.Coverage.Unclassified {
			notes = append(notes, "  "+u.String()+" "+u.Column.Declared)
		}
		return notes
	case coverageComplete:
		// The schema was read and the file answers for all of it. The one run with
		// nothing left to say.
	}
	return nil
}

func (r *AnonymizeCheckResult) Fields() []renderer.Field {
	fields := renderer.Fields{}.
		Add("file", "File", r.Path).
		Add("environments", "Environments", r.Environments).
		AddOptional("preset", "Preset", r.Preset, "none").
		Add("tables", "Tables", r.Summary.Tables).
		Add("columns", "Columns", r.Summary.Columns).
		Add("correlation_groups", "Correlation groups", r.Summary.Groups).
		AddOptional("schema", "Schema read from", r.SchemaFrom, "no environment was reachable").
		// Both lists are always present and always separate. A caller acting on one key
		// for "the preset and the file differ" would be acting on a tightening brama has
		// already carried out and a loosening it has refused to.
		//
		// They name the columns rather than counting them, because a caller that can only
		// count has to send a person to the repo to find out which column it was. They
		// are contract-only: the same columns reach a person through the Notes, in prose
		// that does not wrap off the screen at ten of them.
		AddContractOnly("preset_drift_applied", driftNames(r.Drift.Applied())).
		AddContractOnly("preset_drift_held", driftNames(r.Drift.Held())).
		// The resolution, split the same way and for the same reason: one of these is a
		// pull that runs and says what it substituted, the other is a pull that refuses.
		// A caller acting on one key for both would gate a release on the wrong half.
		AddContractOnly("keep_substituted", fallbackNames(r.Fallbacks.Substituted())).
		AddContractOnly("keep_no_fallback", fallbackNames(r.Fallbacks.NoFallback()))

	// The coverage keys are always present, so a caller reads the same shape from
	// every run. Which one it is reading is what `schema` and the notes answer: where
	// the comparison was not a whole answer these are zero because nothing was
	// counted, not because nothing was found.
	var columns, unclassified int
	if r.coverage().verified() {
		columns, unclassified = r.Coverage.Columns, len(r.Coverage.Unclassified)
	}
	return fields.
		Add("schema_columns", "Columns in the schema", columns).
		Add("unclassified_columns", "Unclassified", unclassified)
}

// schemaSource reads the Schema of one Environment.
//
// It is a seam rather than a call because reaching a database is the part of `check`
// that cannot happen everywhere `check` runs, and the command has to behave the same
// either way: a source that answers errUnreachable is the ordinary case on a CI
// runner, not a failure.
type schemaSource func(ctx context.Context, cfg *config.Config, environment string) (schema.Schema, error)

// errUnreachable is a schemaSource saying there is no route to this Environment's
// database. It is not an error the command fails on — it is the answer that makes
// column coverage unverified.
var errUnreachable = errors.New("no route to this environment's database")

// unreachable is the schemaSource brama ships today.
//
// Nothing here can open a database yet: resolving an Environment's credentials out of
// the project's own config is its own piece of work (#17), and reaching a remote one
// goes through the Shim. Until then every Environment answers errUnreachable and
// `check` reports column coverage as unverified — which is a state it says out loud,
// and the reason Status is partial rather than success.
func unreachable(context.Context, *config.Config, string) (schema.Schema, error) {
	return schema.Schema{}, errUnreachable
}

func newAnonymizeCheckCmd(env *console) *cobra.Command {
	var only string

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate the classification in brama.yaml",
		Long: "Read brama.yaml and report every way its classification contradicts itself:\n" +
			"a generator brama does not have, a correlate beside a classification it means\n" +
			"nothing on, a correlation group with one member, an approval of a column that\n" +
			"is not kept.\n\n" +
			"Where the file names a preset it also reports the columns the two disagree\n" +
			"about: the ones the preset now classifies more strictly, which brama applies\n" +
			"on its own, and the ones it classifies less strictly, which are held until\n" +
			"`brama anonymize review` accepts them.\n\n" +
			"It then reads classification and approval together, per environment, and says\n" +
			"what each would actually receive: the kept columns it has not approved, which\n" +
			"a generator stands in for, and the ones no generator claims — where a pull\n" +
			"refuses rather than emptying a column nobody asked it to empty.\n\n" +
			"It writes nothing, and it runs on a CI runner with no route to production.\n" +
			"Where an environment is reachable it also reads that database's schema and\n" +
			"compares the two: which columns nothing classifies, and whether a generator\n" +
			"can fill the column it was given. Where none is, it says column coverage went\n" +
			"unverified rather than reporting a clean bill of health it could not earn.\n\n" +
			"Exits 42 when the file is inconsistent — nothing went wrong, a guardrail held.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("finding the working directory: %w", err)
			}
			return runAnonymizeCheck(cmd.Context(), env, dir, only, unreachable)
		},
	}

	// No --ci. CI wants a different renderer, not a different command, and --json is
	// already that. A flag meaning "be terse somewhere else" is a second contract to
	// keep in step with the first.
	cmd.Flags().StringVar(&only, "env", "", "check only this environment's approvals (default: every environment)")

	return cmd
}

// runAnonymizeCheck validates the project rooted at or above dir.
//
// A file that will not parse is an error and not a Refusal: a Refusal means brama
// understood the file and declined to act on what it says. Exit 1 there, 42 here.
func runAnonymizeCheck(ctx context.Context, env *console, dir, only string, source schemaSource) error {
	cfg, path, err := config.Load(dir)
	if err != nil {
		return err
	}

	environments, err := environmentsFor(cfg, only)
	if err != nil {
		return err
	}

	// No block at all is not a file that holds together — it is a file that decides
	// nothing, leaving every column Unclassified and the first Pull refused (ADR
	// 0003). Reporting success on it would be the one answer nobody can act on.
	if cfg.Anonymize == nil || (cfg.Anonymize.Preset == "" && len(cfg.Anonymize.Tables) == 0) {
		return refusal.New(refusal.Unclassified,
			fmt.Sprintf("%s classifies nothing — with no anonymize block every column is unclassified, and a pull refuses", path),
			"brama anonymize init")
	}

	// The Preset is read in before anything is checked, and never written back. What
	// the file says plus what the Preset ships is what a Pull would act on, so it is
	// what `check` has to hold up — a column the Preset classifies is not a column
	// anybody left undecided.
	//
	// A name that did not resolve stops the run here rather than joining the report.
	// Everything after this point reads the Preset's answers as classification, so
	// carrying on without them would call every column it was holding unclassified and
	// refuse every approval of one — a hundred lines of consequence stacked on top of
	// the one typo that caused them.
	resolved, drift, unresolved := anonymize.Resolve(cfg)
	if len(unresolved) > 0 {
		return refusal.New(refusal.Invalid, problemDetail(unresolved), "")
	}

	summary, problems := anonymize.Check(resolved, environments, drift)

	// What each destination would receive, which is Classification and Approval read
	// together. It is reported rather than refused: `check` validates the file, and a
	// `keep` nothing claims is not a file that contradicts itself — it is a pull that
	// will refuse, named here before anyone runs one. The Refusal itself belongs to the
	// operation that moves data, and is anonymize.Refuse.
	result := &AnonymizeCheckResult{
		Path:         path,
		Environments: environments,
		Summary:      summary,
		Preset:       resolved.Anonymize.Preset,
		Drift:        drift,
		Fallbacks:    anonymize.Effective(resolved, environments),
	}
	read, from, err := readSchema(ctx, cfg, environments, source)
	if err != nil {
		return err
	}
	if from != "" {
		coverage, uncoverable := anonymize.Cover(resolved.Anonymize, read)
		result.SchemaFrom, result.Coverage = from, &coverage
		problems = append(problems, uncoverable...)
	}

	// The refusal comes after the comparison so that one run says everything wrong
	// with the file, whether it needed a database to see it or not.
	if len(problems) > 0 {
		return refusal.New(refusal.Invalid, problemDetail(problems), "")
	}

	// Unclassified columns are reported, not refused. Whether `check` stops on them is
	// its own decision, made once for the command rather than twice for the two ways
	// of finding them — see #10.
	if err := env.Renderer.Result(result); err != nil {
		return fmt.Errorf("rendering the check result: %w", err)
	}
	return nil
}

// readSchema returns the first Schema any checked Environment answers with, and the
// name of the Environment that answered. An empty name means none did.
//
// One Schema is enough. `anonymize.tables` is project-wide, so asking two Environments
// that agree is the same question twice; and where they disagree — staging a migration
// behind production — the difference is drift between environments, which is not a
// contradiction in the file and must not be reported as one.
//
// Anything other than being out of reach is a real failure and stops the command. A
// database that answered and then could not be read is not the same as no database,
// and quietly carrying on would report column coverage as unverified when it was in
// fact unread for a reason someone can fix.
func readSchema(ctx context.Context, cfg *config.Config, environments []string, source schemaSource) (schema.Schema, string, error) {
	for _, name := range environments {
		read, err := source(ctx, cfg, name)
		if errors.Is(err, errUnreachable) {
			continue
		}
		if err != nil {
			return schema.Schema{}, "", fmt.Errorf("reading the schema of %s: %w", name, err)
		}
		return read, name, nil
	}
	return schema.Schema{}, "", nil
}

// environmentsFor is every Environment, or the one --env named. Both anonymize
// commands narrow the same way, from the same flag, and share this.
//
// An unknown name is an error rather than a Refusal or an empty run: the command was
// asked about something that does not exist, and quietly checking nothing would exit 0
// on a typo in a CI pipeline.
func environmentsFor(cfg *config.Config, only string) ([]string, error) {
	declared := slices.Sorted(maps.Keys(cfg.Environments))
	if only == "" {
		return declared, nil
	}
	if _, ok := cfg.Environments[only]; !ok {
		return nil, fmt.Errorf("no environment named %q — %s declares %s", only, config.Filename, join(declared))
	}
	return []string{only}, nil
}

// problemDetail is the Refusal's detail — one sentence for one problem, a numbered
// block for several, in the shape config's own validation already reports in.
//
// Every problem is listed rather than the first. Whoever is fixing this has the block
// open in front of them, and one problem per run is how a ten-minute edit becomes ten
// CI runs.
func problemDetail(problems []anonymize.Problem) string {
	if len(problems) == 1 {
		return problems[0].String()
	}
	lines := make([]string, 0, len(problems))
	for _, p := range problems {
		lines = append(lines, p.String())
	}
	return fmt.Sprintf("%d problems:\n  - %s", len(problems), strings.Join(lines, "\n  - "))
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
