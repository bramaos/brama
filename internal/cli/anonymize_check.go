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
	// coverageUnexpanded read a Schema and cannot compare against it: a Preset holds
	// part of the classification, and brama cannot yet expand one.
	coverageUnexpanded
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
	case r.Coverage.Unexpanded != "":
		return coverageUnexpanded
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
func (r *AnonymizeCheckResult) Status() renderer.Status {
	if r.coverage() == coverageComplete {
		return renderer.StatusSuccess
	}
	return renderer.StatusPartial
}

func (r *AnonymizeCheckResult) Headline() string {
	classifies := fmt.Sprintf("%s in %s",
		plural(r.Summary.Columns, "column"), plural(r.Summary.Tables, "table"))

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
	switch r.coverage() {
	case coverageUnread:
		return []string{"column coverage was not verified — this run read no schema, " +
			"and a column the database has and this file does not is still unclassified"}
	case coverageUnexpanded:
		return []string{fmt.Sprintf(
			"column coverage was not verified — the %s preset is referenced and not expanded, "+
				"so brama cannot yet tell a column it classifies from one nobody did",
			r.Coverage.Unexpanded)}
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
		Add("tables", "Tables", r.Summary.Tables).
		Add("columns", "Columns", r.Summary.Columns).
		Add("correlation_groups", "Correlation groups", r.Summary.Groups).
		AddOptional("schema", "Schema read from", r.SchemaFrom, "no environment was reachable")

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

	summary, problems := anonymize.Check(cfg, environments)

	result := &AnonymizeCheckResult{Path: path, Environments: environments, Summary: summary}
	read, from, err := readSchema(ctx, cfg, environments, source)
	if err != nil {
		return err
	}
	if from != "" {
		coverage, uncoverable := anonymize.Cover(cfg.Anonymize, read)
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
