package cli

import (
	"github.com/spf13/cobra"

	"github.com/bramaos/brama/internal/renderer"
)

// VersionResult is what `brama version` produces.
type VersionResult struct {
	Version string
}

func (r *VersionResult) Action() string          { return "version" }
func (r *VersionResult) Status() renderer.Status { return renderer.StatusSuccess }
func (r *VersionResult) Headline() string        { return "brama " + r.Version }

func (r *VersionResult) Fields() []renderer.Field {
	return renderer.Fields{}.Add("version", "Version", r.Version)
}

// newVersionCmd keeps `brama version` as a subcommand alongside the `--version` flag
// fang supplies. Both exist because scripts written against either should work, and
// because a subcommand is what `brama version --json` needs to hang off.
func newVersionCmd(env *console, version string) *cobra.Command {
	return &cobra.Command{
		Use:          "version",
		Short:        "Print the version this binary was built from",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(*cobra.Command, []string) error {
			return env.Renderer.Result(&VersionResult{Version: version})
		},
	}
}
