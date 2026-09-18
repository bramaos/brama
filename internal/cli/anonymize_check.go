package cli

import (
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
)

// AnonymizeCheckResult is what `brama anonymize check` produces when the file holds
// together. The counts are the part worth printing: they are how a reader tells "this
// validated my project" from "this validated an empty block and said yes".
type AnonymizeCheckResult struct {
	Path         string
	Environments []string
	Summary      anonymize.Summary
}

func (r *AnonymizeCheckResult) Action() string { return "anonymize_check" }

func (r *AnonymizeCheckResult) Status() renderer.Status { return renderer.StatusSuccess }

func (r *AnonymizeCheckResult) Headline() string {
	return fmt.Sprintf("%s is consistent — %s in %s",
		filepath.Base(r.Path), plural(r.Summary.Columns, "column"), plural(r.Summary.Tables, "table"))
}

// Notes says what this run could not have checked. A clean offline check is not a
// clean bill of health: the columns a database has and the file does not are still
// unclassified, and nothing here read a database.
func (r *AnonymizeCheckResult) Notes() []string {
	return []string{"column coverage was not verified — this run read no schema"}
}

func (r *AnonymizeCheckResult) Fields() []renderer.Field {
	return renderer.Fields{}.
		Add("file", "File", r.Path).
		Add("environments", "Environments", r.Environments).
		Add("tables", "Tables", r.Summary.Tables).
		Add("columns", "Columns", r.Summary.Columns).
		Add("correlation_groups", "Correlation groups", r.Summary.Groups)
}

func newAnonymizeCheckCmd(env *console) *cobra.Command {
	var only string

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate the classification in brama.yaml, reaching no database",
		Long: "Read brama.yaml and report every way its classification contradicts itself:\n" +
			"a generator brama does not have, a correlate beside a classification it means\n" +
			"nothing on, a correlation group with one member, an approval of a column that\n" +
			"is not kept.\n\n" +
			"It writes nothing and reaches nothing, so it runs on a CI runner with no route\n" +
			"to production. What it cannot see there is the database: whether the\n" +
			"classification covers every column the schema has is a separate question, and\n" +
			"a clean run here does not answer it.\n\n" +
			"Exits 42 when the file is inconsistent — nothing went wrong, a guardrail held.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(*cobra.Command, []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("finding the working directory: %w", err)
			}
			return runAnonymizeCheck(env, dir, only)
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
func runAnonymizeCheck(env *console, dir, only string) error {
	cfg, path, err := config.Load(dir)
	if err != nil {
		return err
	}

	environments, err := environmentsToCheck(cfg, only)
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
	if len(problems) > 0 {
		return refusal.New(refusal.Invalid, problemDetail(problems), "")
	}

	if err := env.Renderer.Result(&AnonymizeCheckResult{
		Path:         path,
		Environments: environments,
		Summary:      summary,
	}); err != nil {
		return fmt.Errorf("rendering the check result: %w", err)
	}
	return nil
}

// environmentsToCheck is every Environment, or the one --env named.
//
// An unknown name is an error rather than a Refusal or an empty run: the command was
// asked about something that does not exist, and quietly checking nothing would exit 0
// on a typo in a CI pipeline.
func environmentsToCheck(cfg *config.Config, only string) ([]string, error) {
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
