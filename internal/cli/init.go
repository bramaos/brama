package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/bramaos/brama/internal/adapter"
	"github.com/bramaos/brama/internal/adapter/wordpress"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/renderer"
	"github.com/bramaos/brama/internal/scaffold"
)

// detectors are the Adapters this build knows. Laravel joins the list in v0.1; the
// slice is passed explicitly rather than filled by init() so the set is visible.
func detectors() []adapter.Detector {
	return []adapter.Detector{wordpress.Detector{}}
}

// InitResult is what `brama init` produces.
type InitResult struct {
	Path       string
	Adapter    string
	Paths      config.Paths
	LocalURL   string
	DryRun     bool
	notes      []string
	unresolved []adapter.Unresolved
}

func (r *InitResult) Action() string { return "init" }

func (r *InitResult) Status() renderer.Status {
	if len(r.unresolved) > 0 {
		return renderer.StatusPartial
	}
	return renderer.StatusSuccess
}

func (r *InitResult) Headline() string {
	name := filepath.Base(r.Path)
	switch {
	case len(r.unresolved) > 0 && r.DryRun:
		return "dry run — " + name + " would be incomplete, see the commented keys above"
	case len(r.unresolved) > 0:
		return "wrote " + name + ", but it is incomplete — fill in the commented keys, then rerun"
	case r.DryRun:
		return "dry run — nothing written"
	default:
		return "wrote " + name + " — next: brama server add"
	}
}

func (r *InitResult) Notes() []string { return r.notes }

// Fields carry raw values. An undetermined path is the empty string, which the JSON
// contract renders as null and the human renderer renders as "not determined" —
// prose belongs to the renderer, never to the contract.
func (r *InitResult) Fields() []renderer.Field {
	keys := make([]string, 0, len(r.unresolved))
	for _, u := range r.unresolved {
		keys = append(keys, u.Key)
	}

	return renderer.Fields{}.
		Add("file", "File", r.Path).
		Add("adapter", "Adapter", r.Adapter).
		Add("path_config", "Credentials file", r.Paths.Config).
		Add("path_uploads", "Uploads", r.Paths.Uploads).
		Add("local_url", "Local URL", r.LocalURL).
		Add("unresolved", "Unresolved", keys)
}

// ErrIncomplete reports that the skeleton was written but something in it is unset.
// The result has already been rendered by the time this is returned, so it carries
// ErrAlreadyReported: it exists to set the exit code, not to print a second time.
var ErrIncomplete = fmt.Errorf("%w: brama.yaml is incomplete", ErrAlreadyReported)

func newInitCmd(env *console) *cobra.Command {
	var (
		adapterName string
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write a starting brama.yaml for this project",
		Long: "Detect the adapter and where its parts live, then write a starting brama.yaml.\n\n" +
			"It writes no secrets and no classification: `brama anonymize init` owns the\n" +
			"anonymize block, and until it runs, every column is unclassified and a pull\n" +
			"will refuse.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			return runInit(env, root, adapterName, dryRun)
		},
	}

	cmd.Flags().StringVar(&adapterName, "adapter", "",
		"skip detection and use this adapter ("+join(adapter.Names(detectors()))+")")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be written, and write nothing")

	return cmd
}

// runInit writes the starting brama.yaml for the project rooted at root.
func runInit(env *console, root, adapterName string, dryRun bool) error {
	target := filepath.Join(root, config.Filename)

	// brama.yaml is desired state, owned by developers through git. Overwriting it
	// destroys reviewed decisions, so there is no --force: `rm brama.yaml` is the
	// escape hatch, and it shows up in the diff.
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%s already exists — remove it first if you mean to start over", config.Filename)
	}

	result, err := detect(root, adapterName)
	if err != nil {
		return err
	}

	body := scaffold.Skeleton(result)
	if !dryRun {
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", config.Filename, err)
		}
	}

	out := &InitResult{
		Path:       target,
		Adapter:    result.Adapter,
		Paths:      result.Paths,
		LocalURL:   result.LocalURL,
		DryRun:     dryRun,
		notes:      result.Notes,
		unresolved: result.Unresolved,
	}
	if err := env.Renderer.Result(out); err != nil {
		return err
	}
	if dryRun {
		env.writeSkeletonPreview(body)
	}

	// The skeleton is written either way, so there is something to edit — but the
	// file is not usable until someone chooses the missing values, and nothing
	// downstream should run against a value nobody picked.
	if !result.Complete() {
		return ErrIncomplete
	}
	return nil
}

func detect(root, adapterName string) (adapter.Detection, error) {
	if adapterName != "" {
		return adapter.DetectAs(root, adapterName, detectors())
	}

	result, err := adapter.Detect(root, detectors())
	if errors.Is(err, adapter.ErrUnrecognised) {
		return adapter.Detection{}, fmt.Errorf(
			"no adapter recognised this project — rerun with --adapter (%s) if you know what it is",
			join(adapter.Names(detectors())))
	}
	return result, err
}
