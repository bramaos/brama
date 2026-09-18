package anonymize

import (
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/preset"
)

// Resolve reads in the Preset a project's Classification names.
//
// It returns the config brama acts on: the same file, with `anonymize` standing for
// what it means rather than for what is written in it. The Preset is expanded in memory
// and never into brama.yaml — the file keeps holding the project's own half, which is
// what makes it short enough to review and what lets a Preset brama tightens reach the
// project without an edit. See ADR 0011.
//
// Everything downstream takes the resolved config, so a column the Preset classifies is
// a classified column everywhere: `check` counts it, coverage answers for it, and an
// Approval may name it. What resolution does not do is grant anything. A Preset's
// `keep` arrives here as a Classification and as nothing else, and the Environment that
// receives real values is still only the one a human wrote an Approval on. See ADR 0010.
//
// An unknown name is a Problem rather than an error, so one run reports it beside
// everything else wrong with the file, and the rest of the check still runs against
// what the file does say.
func Resolve(cfg *config.Config) (*config.Config, []Problem) {
	if cfg.Anonymize == nil || cfg.Anonymize.Preset == "" {
		return cfg, nil
	}

	p, err := preset.Lookup(cfg.Anonymize.Preset)
	if err != nil {
		return cfg, []Problem{{At: "anonymize.preset", Detail: err.Error()}}
	}

	// A shallow copy: nothing below replaces anything but the Classification, and the
	// caller's config is left saying what its file says.
	resolved := *cfg
	resolved.Anonymize = p.Apply(cfg.Anonymize)
	return &resolved, nil
}
