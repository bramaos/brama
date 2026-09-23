package cli

import (
	"fmt"

	"github.com/bramaos/brama/internal/adapter"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/preset"
	"github.com/bramaos/brama/internal/refusal"
)

// tablePrefix is what this project's own tables are named with, asked of the Adapter
// that knows where the project writes it down.
//
// Every command that resolves a Preset goes through here, so `init` and `check` cannot
// end up disagreeing about which tables the Preset covers — which is the failure this
// replaces: `init` writing `preset: wordpress` off `app.adapter` alone, and `check` then
// counting twelve tables the database does not have.
//
// named is the Preset waiting on the answer. It is almost always the Adapter's own name,
// because a Preset is named after the Adapter whose tables it knows, and it is passed
// separately because a file may reference a Preset that is not this project's Adapter —
// and the person reading the Refusal needs the name they wrote.
func tablePrefix(cfg *config.Config, root, named string) (string, error) {
	prefix, err := adapter.TablePrefix(root, cfg.App.Adapter, cfg.App.Paths.Config, detectors())
	if err != nil {
		return "", refusal.New(refusal.UnknownPrefix, fmt.Sprintf(
			"%s, so brama cannot tell which tables the %s preset is about — "+
				"declare it in the project's own config, or drop the preset and classify the tables "+
				"in %s directly", err, named, config.Filename), "")
	}
	return prefix, nil
}

// presetPrefix is the prefix the Preset a file names is waiting on, and "" where it is
// waiting on nothing. cfg is the file as loaded, before any Preset is read in.
//
// A project that references no Preset never has its config read for one. Checking a
// classification written out column by column has never needed the framework to be there,
// and that is what `brama anonymize check` on a CI runner is.
func presetPrefix(cfg *config.Config, root string) (string, error) {
	if cfg.Anonymize == nil || !preset.NeedsPrefix(cfg.Anonymize.Preset) {
		return "", nil
	}
	return tablePrefix(cfg, root, cfg.Anonymize.Preset)
}
