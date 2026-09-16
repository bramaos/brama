package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/renderer"
	"github.com/bramaos/brama/internal/shim"
	"github.com/bramaos/brama/internal/ssh"
)

// session is the part of a Server connection this command needs. It is declared
// here, at the point of use, so internal/ssh can stay a concrete package with no
// interface of its own to keep in step.
type session interface {
	shim.Runner
	Close() error
}

// dialer opens a session. It is a parameter so a test can drive the command against
// a stand-in Server; production passes dialSSH.
type dialer func(ctx context.Context, t ssh.Target) (session, error)

func dialSSH(ctx context.Context, t ssh.Target) (session, error) {
	return ssh.Open(ctx, t)
}

// installer is everything the command needs to reach a Server and put a Shim on it.
// Grouped so the command signature stays about the registration, not about its
// plumbing.
type installer struct {
	version string
	dial    dialer
	source  shim.Source
	// embedded reports whether this build carries any shim to install. Checked
	// before dialling: which platform is needed depends on the Server, but having
	// nothing to send does not.
	embedded func() bool
}

func sshInstaller(version string) installer {
	return installer{
		version:  version,
		dial:     dialSSH,
		source:   shim.Binary,
		embedded: shim.Available,
	}
}

// ServerAddResult is what `brama server add` produces.
type ServerAddResult struct {
	Name     string
	Host     string
	User     string
	Platform string
	Shim     shim.Report
}

func (r *ServerAddResult) Action() string          { return "server_add" }
func (r *ServerAddResult) Status() renderer.Status { return renderer.StatusSuccess }

func (r *ServerAddResult) Headline() string {
	installed := "shim already current"
	if r.Shim.Uploaded {
		installed = "shim installed"
	}
	return "registered " + r.Name + " — " + installed +
		" — next: add an environment for it in " + config.Filename
}

// Fields carry raw values. shim_uploaded is the difference between "brama put this
// here" and "it was already here", which is what a caller registering the same
// Server from a second project needs to see.
func (r *ServerAddResult) Fields() []renderer.Field {
	return renderer.Fields{}.
		Add("server", "Server", r.Name).
		Add("host", "Host", r.Host).
		Add("user", "User", r.User).
		Add("platform", "Platform", r.Platform).
		Add("shim_version", "Shim version", r.Shim.Version).
		Add("shim_uploaded", "Shim uploaded", r.Shim.Uploaded)
}

func newServerCmd(env *console, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Register and inspect the servers hosting this project's environments",
	}
	cmd.AddCommand(newServerAddCmd(env, version))
	return cmd
}

func newServerAddCmd(env *console, version string) *cobra.Command {
	var host, user string

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a server and install the shim on it",
		Long: "Register an SSH host under a name environments can reference, and install the\n" +
			"shim on it.\n\n" +
			"Authentication is OpenSSH's: --host is passed through untouched, so it may be a\n" +
			"hostname, an IP, or a ~/.ssh/config alias, and identity comes from ssh-agent and\n" +
			"~/.ssh/config. brama stores no credentials.\n\n" +
			"The server is reached before anything is written, so a registered server is\n" +
			"always one that answered.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			srv := config.Server{Host: host, User: user}
			return runServerAdd(env, dir, args[0], srv, sshInstaller(version))
		},
	}

	cmd.Flags().StringVar(&host, "host", "",
		"the ssh target: a hostname, an IP, or a ~/.ssh/config alias")
	cmd.Flags().StringVar(&user, "user", "",
		"the ssh user (optional — without it, OpenSSH decides)")
	_ = cmd.MarkFlagRequired("host")

	return cmd
}

// runServerAdd registers a Server for the project at or above dir.
//
// The order is deliberate: everything decidable from the file happens before a
// connection is opened, and the file is written only once the Server has answered.
// A servers: entry therefore always means "reachable, and the shim runs here" —
// which is the whole reason the probe exists.
func runServerAdd(env *console, dir, name string, srv config.Server, inst installer) error {
	if srv.Host == "" {
		return errors.New("--host is required — the ssh target to register")
	}

	path, err := config.Find(dir)
	if errors.Is(err, config.ErrNotFound) {
		return fmt.Errorf("no %s in this project — run: brama init", config.Filename)
	}
	if err != nil {
		return err
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// Decided from the file alone, before anything is dialled: registering is not
	// the verb for changing a reviewed decision.
	if err := checkUnregistered(body, name, path); err != nil {
		return err
	}

	if inst.embedded != nil && !inst.embedded() {
		return fmt.Errorf("%w — build one with `make binary`", shim.ErrNotBuilt)
	}

	ctx := context.Background()
	remote, err := inst.dial(ctx, ssh.Target{Host: srv.Host, User: srv.User})
	if err != nil {
		return err
	}
	defer func() { _ = remote.Close() }()

	report, err := shim.Install(ctx, remote, inst.version, inst.source)
	if err != nil {
		return describeInstallFailure(err, report)
	}

	updated, err := config.AddServer(body, name, srv)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Base(path), err)
	}

	return env.Renderer.Result(&ServerAddResult{
		Name:     name,
		Host:     srv.Host,
		User:     srv.User,
		Platform: report.Platform.String(),
		Shim:     report,
	})
}

// checkUnregistered reports whether the name is free, without modifying anything.
func checkUnregistered(body []byte, name, path string) error {
	cfg, err := config.Parse(body)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if existing, ok := cfg.Servers[name]; ok {
		return fmt.Errorf(
			"server %q is already registered (host: %s) — edit %s to change it",
			name, existing.Host, config.Filename)
	}
	return nil
}

// describeInstallFailure names the platform brama found when it has no build for it.
// "unsupported platform" without the platform leaves the user nothing to report.
func describeInstallFailure(err error, report shim.Report) error {
	if errors.Is(err, shim.ErrUnsupportedPlatform) && report.Platform.OS != "" {
		return fmt.Errorf("%w — brama has no shim build for %s", err, report.Platform)
	}
	return err
}
