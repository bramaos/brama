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
	"github.com/bramaos/brama/internal/renderer"
)

// AnonymizeInitResult is what `brama anonymize init` produces.
//
// The two counts are the whole story of a run, and they are reported side by side on
// purpose: what brama classified is what a Generator claimed, and what is left is work
// for a person. A run that wrote forty columns and left sixty is not a finished job,
// and a result that printed only the forty would read like one.
type AnonymizeInitResult struct {
	Path string
	// SchemaFrom names the Environment whose Schema was classified.
	SchemaFrom string
	// Tables is what was written, in the order it was written.
	Tables []config.TableClassification
	// Unclassified are the columns no Generator claimed, left out of the file.
	Unclassified []anonymize.Uncovered
	DryRun       bool
}

func (r *AnonymizeInitResult) Action() string { return "anonymize_init" }

// Status is partial while anything is still Unclassified, because a Pull still refuses.
// `init` bootstraps a classification; it does not finish one.
func (r *AnonymizeInitResult) Status() renderer.Status {
	if len(r.Unclassified) > 0 {
		return renderer.StatusPartial
	}
	return renderer.StatusSuccess
}

func (r *AnonymizeInitResult) Headline() string {
	wrote := fmt.Sprintf("%s in %s",
		plural(anonymize.Claimed(r.Tables), "column"), plural(len(r.Tables), "table"))

	switch {
	case r.DryRun:
		return fmt.Sprintf("dry run — %s would be classified from %s, and nothing was written",
			wrote, r.SchemaFrom)
	case len(r.Unclassified) > 0:
		return fmt.Sprintf("classified %s from %s — %s left for you to decide",
			wrote, r.SchemaFrom, plural(len(r.Unclassified), "column"))
	default:
		return fmt.Sprintf("classified %s from %s — next: brama anonymize check", wrote, r.SchemaFrom)
	}
}

// Notes names every column left out, and says what leaving it out means.
//
// All of them, not a count. Whoever runs this next has `brama.yaml` open in front of
// them, and a number is not something anyone can act on — these are the review items,
// and they are the reason a Pull still refuses.
func (r *AnonymizeInitResult) Notes() []string {
	if len(r.Unclassified) == 0 {
		return nil
	}
	notes := make([]string, 0, len(r.Unclassified)+1)
	notes = append(notes, "no generator claimed these, so they are left out of the file and "+
		"unclassified — a pull refuses until each one is decided with brama anonymize review:")
	for _, u := range r.Unclassified {
		notes = append(notes, "  "+u.String()+" "+u.Column.Declared)
	}
	return notes
}

func (r *AnonymizeInitResult) Fields() []renderer.Field {
	return renderer.Fields{}.
		Add("file", "File", r.Path).
		Add("schema", "Schema read from", r.SchemaFrom).
		Add("tables", "Tables", len(r.Tables)).
		Add("columns", "Columns classified", anonymize.Claimed(r.Tables)).
		Add("unclassified_columns", "Left unclassified", len(r.Unclassified)).
		Add("dry_run", "Dry run", r.DryRun)
}

func newAnonymizeInitCmd(env *console) *cobra.Command {
	var (
		only   string
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   initVerb,
		Short: "Bootstrap the classification in brama.yaml from a database schema",
		Long: "Read an environment's schema and write an anonymize block classifying the columns\n" +
			"brama recognises: a column a generator declares a claim on — by name and by type —\n" +
			"becomes `action: fake.<generator>`.\n\n" +
			"Everything else is left out of the file, which leaves it unclassified and a pull\n" +
			"refused. Nothing is defaulted to `drop`: dropping is safe about privacy and\n" +
			"reckless about everything else, and zeroing a column brama could not name is a\n" +
			"product decision that is not brama's to make. `keep` is never written at all —\n" +
			"it sends real production data, and it enters the file only where a human put it.\n\n" +
			"Decide the rest with: brama anonymize review",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("finding the working directory: %w", err)
			}
			return runAnonymizeInit(cmd.Context(), env, dir, only, dryRun, unreachable)
		},
	}

	cmd.Flags().StringVar(&only, "env", "", "read the schema of this environment (default: the first reachable one)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would be classified, and write nothing")

	return cmd
}

// runAnonymizeInit writes the anonymize block for the project rooted at or above dir.
//
// The order is the same as every other write in brama: everything decidable is decided
// before the file is touched, and the file is written once, whole.
func runAnonymizeInit(ctx context.Context, env *console, dir, only string, dryRun bool, source schemaSource) error {
	cfg, path, err := config.Load(dir)
	if err != nil {
		return err
	}

	environments, err := environmentsFor(cfg, only)
	if err != nil {
		return err
	}

	// Refused before a database is reached: whether the file already holds a reviewed
	// classification is a question about the file, and answering it should cost nobody
	// a connection to production.
	if cfg.Anonymize != nil {
		return fmt.Errorf("%s already classifies this project — amend it with `brama anonymize review`, "+
			"which changes decisions one at a time instead of replacing them all", filepath.Base(path))
	}

	read, from, err := readSchema(ctx, cfg, environments, source)
	if err != nil {
		return err
	}
	if from == "" {
		return fmt.Errorf("no environment is reachable — init classifies the columns a database has, "+
			"and %s could not read one from %s", filepath.Base(path), join(environments))
	}

	// Cover with nothing classified is the whole schema, column by column, which is
	// exactly the list `check` calls unclassified. init and check walk the same ground
	// on purpose: what one reports, the other offers an answer for.
	coverage, problems := anonymize.Cover(nil, read)
	if len(problems) > 0 {
		// Unreachable today — nothing is classified, so no Generator was named and none
		// can fail to fit. Reported rather than dropped, because silently writing a file
		// while holding a problem is the failure mode this command exists to avoid.
		return fmt.Errorf("reading the schema of %s: %s", from, problemDetail(problems))
	}

	tables := anonymize.Bootstrap(coverage)
	if len(tables) == 0 {
		return fmt.Errorf("no generator claimed any of the %s in %s — "+
			"classify them with `brama anonymize review`, which asks about them one at a time",
			plural(coverage.Columns, "column"), from)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	updated, err := config.AddAnonymize(body, tables)
	if errors.Is(err, config.ErrAnonymizeExists) {
		return fmt.Errorf("%s already classifies this project — amend it with `brama anonymize review`", filepath.Base(path))
	}
	if err != nil {
		return fmt.Errorf("writing the classification into %s: %w", filepath.Base(path), err)
	}

	if !dryRun {
		// gosec traces path back to the directory brama was run in and calls it
		// traversal. It is the brama.yaml config.Load located: the command's whole job
		// is to write the file back where it found it.
		//
		//nolint:gosec // G703: path is config.Load's result, not caller-supplied.
		if err := os.WriteFile(path, updated, config.FileMode); err != nil {
			return fmt.Errorf("writing %s: %w", filepath.Base(path), err)
		}
	}

	result := &AnonymizeInitResult{
		Path:         path,
		SchemaFrom:   from,
		Tables:       tables,
		Unclassified: leftUnclassified(coverage, tables),
		DryRun:       dryRun,
	}
	if err := env.Renderer.Result(result); err != nil {
		return fmt.Errorf("rendering the init result: %w", err)
	}
	if dryRun {
		return env.writeSkeletonPreview(updated)
	}
	return nil
}

// leftUnclassified is what the Schema has and the written block still does not — the
// columns no Generator claimed. It is derived from what was written rather than
// recomputed from the Generators, so the two halves of the report can never disagree
// about which columns those are.
func leftUnclassified(coverage anonymize.Coverage, tables []config.TableClassification) []anonymize.Uncovered {
	written := make(map[string]bool, len(coverage.Unclassified))
	for _, t := range tables {
		for _, c := range t.Columns {
			written[t.Name+"."+c.Name] = true
		}
	}

	var left []anonymize.Uncovered
	for _, u := range coverage.Unclassified {
		if !written[u.String()] {
			left = append(left, u)
		}
	}
	return left
}
