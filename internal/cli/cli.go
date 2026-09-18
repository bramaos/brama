// Package cli assembles brama's commands and maps outcomes to exit codes.
//
// The exit code is part of brama's contract with agents, so it is decided in exactly
// one place: Main. No command calls os.Exit, which is what lets deferred cleanup run
// on the way out.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"

	"github.com/bramaos/brama/internal/refusal"
	"github.com/bramaos/brama/internal/renderer"
)

// Exit codes. A Refusal is not an error, and gets its own code so a caller can tell
// "I am not allowed" from "something broke" without reading the message.
const (
	ExitOK      = 0
	ExitError   = 1
	ExitRefused = 42
)

// ErrAlreadyReported marks an error whose outcome the command has already rendered.
// Wrapping it sets the exit code without printing a second time — which matters most
// for a partial result, where the fields are the useful output and a trailing error
// line would contradict them.
var ErrAlreadyReported = errors.New("already reported")

// console is what commands render through. It exists so a test can swap the renderer
// and the streams without touching a global.
//
// Deliberately not called `environment`: CONTEXT.md reserves that word for a named
// target brama acts on, and reusing it here for a render context would put two
// meanings on the project's most load-bearing noun.
type console struct {
	Renderer renderer.Renderer
	Out      io.Writer
	Err      io.Writer
	JSON     bool
}

// writeSkeletonPreview prints a generated file for --dry-run. Human output only:
// the machine contract reports what would be written as fields, not as a blob.
//
// The write error is returned rather than dropped: under --dry-run this body is the
// entire output, so failing to print it means the command produced nothing.
func (e *console) writeSkeletonPreview(body []byte) error {
	if e.JSON {
		return nil
	}
	if _, err := fmt.Fprintln(e.Out, strings.TrimRight(string(body), "\n")); err != nil {
		return fmt.Errorf("writing the preview: %w", err)
	}
	if _, err := fmt.Fprintln(e.Out); err != nil {
		return fmt.Errorf("writing the preview: %w", err)
	}
	return nil
}

// Main runs brama and returns the process exit code.
func Main(version string) int {
	var jsonOut bool

	root := &cobra.Command{
		Use:   "brama",
		Short: "A real copy of production to develop against — with none of the real data",
		Long: "brama moves data downward — production to staging to local — anonymizing it on\n" +
			"the server before it is transferred. Code moves up, data moves down, and raw\n" +
			"production data never leaves the production server.",
		SilenceUsage:      true,
		SilenceErrors:     true,
		CompletionOptions: cobra.CompletionOptions{HiddenDefaultCmd: true},
	}

	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "emit the machine-readable result")

	// No --non-interactive yet. Nothing in this release prompts, and a flag that
	// silently does nothing is the "warning nobody reads" failure ADR-0003 names.
	// It arrives with the first command that asks a question — `anonymize init`.

	env := &console{Out: os.Stdout, Err: os.Stderr}
	// Flags are not parsed when the command tree is built, so the renderer is chosen
	// once parsing is done and before any command runs.
	root.PersistentPreRun = func(*cobra.Command, []string) {
		env.JSON = jsonOut
		if jsonOut {
			env.Renderer = renderer.NewJSON(env.Out)
			return
		}
		env.Renderer = renderer.NewHuman(env.Out, env.Err)
	}

	root.AddCommand(
		newInitCmd(env),
		newAnonymizeCmd(env),
		newServerCmd(env, version),
		newVersionCmd(env, version),
	)

	err := fang.Execute(
		context.Background(),
		root,
		fang.WithVersion(version),
		fang.WithErrorHandler(func(io.Writer, fang.Styles, error) {
			// Rendering belongs to the renderer, which knows whether this run is
			// for a person or a machine. Fang's styled default would print an
			// error a second time, and would print a Refusal as a failure.
		}),
	)
	if err == nil {
		return ExitOK
	}

	// A renderer only exists once flags have parsed; a usage error fails before that.
	// Nothing to do if even this write fails — the exit code still carries the news.
	if env.Renderer == nil {
		_, _ = fmt.Fprintln(env.Err, "brama:", err)
		return ExitError
	}

	if r, ok := refusal.As(err); ok {
		_ = env.Renderer.Refused(commandPath(root), r)
		return ExitRefused
	}
	if errors.Is(err, ErrAlreadyReported) {
		return ExitError
	}
	_ = env.Renderer.Error(commandPath(root), err)
	return ExitError
}

// commandPath names the command that refused, for the JSON `action` field.
func commandPath(root *cobra.Command) string {
	cmd, _, err := root.Find(os.Args[1:])
	if err != nil || cmd == nil {
		return root.Name()
	}
	return strings.ReplaceAll(strings.TrimPrefix(cmd.CommandPath(), root.Name()+" "), " ", "_")
}

func join(items []string) string { return strings.Join(items, ", ") }
