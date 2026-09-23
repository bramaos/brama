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
	// Preset names the Preset written into the file as a reference, and is empty when
	// brama ships none for this project's Adapter. The columns it covers are not in
	// Tables and are not Unclassified either — they are answered for elsewhere, which
	// is the one thing a reader of these counts has to be told.
	Preset string
	// Covered is how many columns of the Schema the Preset answered for.
	Covered int
	// Tables is what was written, in the order it was written.
	Tables []config.TableClassification
	// Unclassified are the columns no Generator claimed, left out of the file.
	Unclassified []anonymize.Uncovered
	// UnclassifiedKeys are the Discriminator values no Generator claimed, left out the
	// same way.
	UnclassifiedKeys []anonymize.UncoveredKey
	DryRun           bool
}

func (r *AnonymizeInitResult) Action() string { return "anonymize_init" }

// Status is partial while anything is still Unclassified, because a Pull still refuses.
// `init` bootstraps a classification; it does not finish one.
func (r *AnonymizeInitResult) Status() renderer.Status {
	if r.left() > 0 {
		return renderer.StatusPartial
	}
	return renderer.StatusSuccess
}

func (r *AnonymizeInitResult) Headline() string {
	wrote := fmt.Sprintf("%s in %s", entries(anonymize.Claimed(r.Tables), anonymize.ClaimedKeys(r.Tables)),
		plural(len(r.Tables), "table"))
	if r.Preset != "" {
		wrote = fmt.Sprintf("%s beyond the %s preset's %s",
			wrote, r.Preset, plural(r.Covered, "column"))
	}

	switch {
	case r.DryRun:
		return fmt.Sprintf("dry run — %s would be classified from %s, and nothing was written",
			wrote, r.SchemaFrom)
	case r.left() > 0:
		return fmt.Sprintf("classified %s from %s — %s left for you to decide",
			wrote, r.SchemaFrom, entries(len(r.Unclassified), len(r.UnclassifiedKeys)))
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
	if r.left() == 0 {
		return nil
	}
	notes := []string{"no generator claimed these, so they are left out of the file and " +
		"unclassified — a pull refuses until each one is decided with brama anonymize review:"}
	notes = append(notes, uncoveredNames(r.Unclassified)...)
	return append(notes, indentAll(keyNames(r.UnclassifiedKeys))...)
}

// left is how many columns and keys this run left Unclassified.
func (r *AnonymizeInitResult) left() int { return len(r.Unclassified) + len(r.UnclassifiedKeys) }

func (r *AnonymizeInitResult) Fields() []renderer.Field {
	return renderer.Fields{}.
		Add("file", "File", r.Path).
		Add("schema", "Schema read from", r.SchemaFrom).
		AddOptional("preset", "Preset", r.Preset, "none").
		Add("preset_columns", "Columns the preset covers", r.Covered).
		Add("tables", "Tables", len(r.Tables)).
		Add("columns", "Columns classified", anonymize.Claimed(r.Tables)).
		Add("unclassified_columns", "Left unclassified", len(r.Unclassified)).
		Add("keys", "Keys classified", anonymize.ClaimedKeys(r.Tables)).
		Add("unclassified_keys", "Keys left unclassified", len(r.UnclassifiedKeys)).
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
			"becomes `action: fake.<generator>`. A key/value table's keys are read too, and a\n" +
			"key is classified exactly as a column is, always by its exact name.\n\n" +
			"Where brama ships a preset for this project's adapter, the block references it by\n" +
			"name instead of expanding it, and the tables it already knows are not written out\n" +
			"again. The file stays short, and a preset brama tightens reaches this project\n" +
			"without anyone editing it.\n\n" +
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

	// The Preset brama ships for this project's Adapter, if it ships one. It is the
	// Classification this project starts from, and it is referenced by name rather than
	// copied into the file: what it covers is decided already, and what it does not is
	// the only thing init has to write down. A Preset answer therefore beats a
	// Generator's pattern claim on the same column without either of them being
	// compared — the column never reaches the Generators at all. See ADR 0011.
	//
	// named is the one signal for whether there is a Preset: empty is "brama ships none
	// for this adapter", and it is the same empty string the block is written from.
	//
	// Settled before a database is reached, like everything else here that a file can
	// answer on its own. A Preset names its tables after the project's own prefix, so one
	// resolved without it would cover twelve tables this database does not have — and
	// `check`, resolving the same file later, would count them. A prefix brama cannot
	// read is a Refusal rather than a fallback to the framework's default, and refusing it
	// here costs nobody a connection to production. The Preset the Adapter ships is the
	// Adapter's own name twice, which is the naming ADR 0011 relies on.
	var (
		named string
		start *config.Anonymize
	)
	prefix := ""
	if preset.NeedsPrefix(cfg.App.Adapter) {
		if prefix, err = tablePrefix(cfg, filepath.Dir(path), cfg.App.Adapter); err != nil {
			return err
		}
	}
	shipped, ships, err := preset.For(cfg.App.Adapter, prefix)
	if err != nil {
		return err
	}
	if ships {
		// No Drift to collect: init runs on a file with no anonymize block, so there is
		// no recorded Classification for the Preset to disagree with.
		start, _ = shipped.Apply(nil)
		named = shipped.Name
	}

	// The keys of each table the Preset keys are read with the Schema, and classified the
	// way its columns are. They are the only rows init reads (ADR 0017).
	read, from, err := readSchema(ctx, cfg, environments, source, discriminators(start))
	if err != nil {
		return err
	}
	if from == "" {
		return fmt.Errorf("no environment is reachable — init classifies the columns a database has, "+
			"and %s could not read one from %s", filepath.Base(path), join(environments))
	}

	// Cover against that start is the part of the schema still undecided, which is
	// exactly the list `check` calls unclassified. init and check walk the same ground
	// on purpose: what one reports, the other offers an answer for.
	coverage, problems := anonymize.Cover(start, read)
	if len(problems) > 0 {
		// Unreachable today — nothing is classified, so no Generator was named and none
		// can fail to fit. Reported rather than dropped, because silently writing a file
		// while holding a problem is the failure mode this command exists to avoid.
		return fmt.Errorf("reading the schema of %s: %s", from, problemDetail(problems))
	}

	tables := anonymize.Bootstrap(coverage)
	if len(tables) == 0 && named == "" {
		return fmt.Errorf("no generator claimed any of the %s in %s — "+
			"classify them with `brama anonymize review`, which asks about them one at a time",
			plural(coverage.Columns, "column"), from)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	updated, err := config.AddAnonymize(body, named, tables)
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
		Path:       path,
		SchemaFrom: from,
		Preset:     named,
		// What the Preset answered for is what the schema has and Cover did not report
		// back — derived from the same comparison rather than counted separately, so
		// the two halves of the report cannot disagree about which columns those are.
		Covered:          coverage.Columns - len(coverage.Unclassified),
		Tables:           tables,
		Unclassified:     anonymize.Unclaimed(coverage, tables),
		UnclassifiedKeys: anonymize.UnclaimedKeys(coverage, tables),
		DryRun:           dryRun,
	}
	if err := env.Renderer.Result(result); err != nil {
		return fmt.Errorf("rendering the init result: %w", err)
	}
	if dryRun {
		return env.writeSkeletonPreview(updated)
	}
	return nil
}
