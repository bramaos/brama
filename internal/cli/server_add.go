package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

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
	shim.Remote
	Close() error
}

// dialer opens a session. It is a parameter so a test can drive the command against
// a stand-in Server; production passes dialSSH.
type dialer func(ctx context.Context, t ssh.Target) (session, error)

func dialSSH(ctx context.Context, t ssh.Target) (session, error) {
	s, err := ssh.Open(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("opening an ssh session: %w", err)
	}
	return s, nil
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

// shimStep runs the Shim's version check against a Server, or reports what it would
// do. Both read the Server; only one of them writes to it.
func (i installer) shimStep(ctx context.Context, r shim.Remote, dryRun bool) (shim.Step, error) {
	if dryRun {
		return shim.Plan(ctx, r, i.version, i.source)
	}
	return shim.Ensure(ctx, r, i.version, i.source)
}

// ServerAddResult is what `brama server add` produces.
type ServerAddResult struct {
	Name     string
	Host     string
	User     string
	Platform string
	Shim     shim.Step
	DryRun   bool
}

func (r *ServerAddResult) Action() string          { return "server_add" }
func (r *ServerAddResult) Status() renderer.Status { return renderer.StatusSuccess }

func (r *ServerAddResult) Headline() string {
	if r.DryRun {
		return "dry run — " + shimHeadline(r.Shim) + ", and " + r.Name +
			" would be registered in " + config.Filename + " — nothing was written"
	}
	return "registered " + r.Name + " — " + shimHeadline(r.Shim) +
		" — next: add an environment for it in " + config.Filename
}

// shimHeadline says what the version check did, in the words a person reading one
// line needs: which way the Shim moved, and whether it has moved yet.
func shimHeadline(step shim.Step) string {
	var verb string
	switch step.Change {
	case shim.ChangeNone:
		return "shim already current at " + step.To
	case shim.ChangeInstall:
		verb = "installed"
	case shim.ChangeUpgrade:
		verb = "upgraded"
	case shim.ChangeReplace:
		verb = "replaced"
	default:
		verb = string(step.Change)
	}
	if step.Pending {
		verb = "will be " + verb
	}

	if step.From == "" {
		return "shim " + verb + " at " + step.To
	}
	return "shim " + verb + " " + step.From + " → " + step.To
}

// Fields carry raw values. shim_uploaded is the difference between "brama put this
// here" and "it was already here", which is what a caller registering the same
// Server from a second project needs to see; shim_change is what the difference
// amounted to, and under --dry-run it is what the difference would amount to.
func (r *ServerAddResult) Fields() []renderer.Field {
	return renderer.Fields{}.
		Add("server", "Server", r.Name).
		Add("host", "Host", r.Host).
		AddOptional("user", "User", r.User, "from ~/.ssh/config").
		Add("platform", "Platform", r.Platform).
		AddOptional("shim_version", "Shim version", r.Shim.Running(), "none installed").
		AddOptional("shim_previous_version", "Shim was", r.Shim.From, "none installed").
		Add("shim_change", "Shim change", string(r.Shim.Change)).
		Add("shim_uploaded", "Shim uploaded", r.Shim.Uploaded()).
		Add("dry_run", "Dry run", r.DryRun)
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
	var (
		host, user string
		dryRun     bool
	)

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a server and install the shim on it",
		Long: "Register an SSH host under a name environments can reference, and install the\n" +
			"shim on it.\n\n" +
			"Authentication is OpenSSH's: --host is passed through untouched, so it may be a\n" +
			"hostname, an IP, or a ~/.ssh/config alias, and identity comes from ssh-agent and\n" +
			"~/.ssh/config. brama stores no credentials.\n\n" +
			"The server is reached before anything is written, so a registered server is\n" +
			"always one that answered.\n\n" +
			"--dry-run still connects — reporting which shim a server runs is the point of\n" +
			"it — but sends nothing and writes nothing.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("finding the working directory: %w", err)
			}
			srv := config.Server{Host: host, User: user}
			return runServerAdd(env, dir, args[0], srv, sshInstaller(version), dryRun)
		},
	}

	cmd.Flags().StringVar(&host, "host", "",
		"the ssh target: a hostname, an IP, or a ~/.ssh/config alias")
	cmd.Flags().StringVar(&user, "user", "",
		"the ssh user (optional — without it, OpenSSH decides)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"report what the shim step would do, and change nothing")
	_ = cmd.MarkFlagRequired("host")

	return cmd
}

// runServerAdd registers a Server for the project at or above dir.
//
// The order is deliberate: everything decidable from the file happens before a
// connection is opened, and the file is written only once the Server has answered.
// A servers: entry therefore always means "reachable, and the shim runs here" —
// which is the whole reason the probe exists.
func runServerAdd(env *console, dir, name string, srv config.Server, inst installer, dryRun bool) error {
	if srv.Host == "" {
		return errors.New("--host is required — the ssh target to register")
	}

	path, err := config.Find(dir)
	if errors.Is(err, config.ErrNotFound) {
		return fmt.Errorf("no %s in this project — run: brama init", config.Filename)
	}
	if err != nil {
		return fmt.Errorf("looking for %s: %w", config.Filename, err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if _, err := config.Parse(body); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	// The edited file is computed before anything is dialled, not after: it is what
	// decides whether the name is free, and refusing a name already registered is a
	// decision about the file that should cost nobody a handshake. Writing it is
	// what waits for the Server.
	updated, err := config.AddServer(body, name, srv)
	if errors.Is(err, config.ErrServerExists) {
		return fmt.Errorf("server %q is already registered — edit %s to change it", name, config.Filename)
	}
	if err != nil {
		return fmt.Errorf("adding server %q to %s: %w", name, config.Filename, err)
	}

	if !inst.embedded() {
		return fmt.Errorf("%w — build one with `make binary`", shim.ErrNotBuilt)
	}

	// Wired to the interrupt signals so a Ctrl-C still runs the deferred Close: the
	// ssh master is backgrounded, and one left alive is an authenticated channel to
	// production nobody is watching.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	remote, err := inst.dial(ctx, ssh.Target{Host: srv.Host, User: srv.User})
	if err != nil {
		return fmt.Errorf("reaching %s: %w", srv.Host, err)
	}
	defer func() { _ = remote.Close() }()

	// The version check is its own step, taken before any operation rather than
	// inside one. Here there is no operation after it — registering a Server is the
	// whole command — but the shape is the one every later command follows, and
	// --dry-run is what makes the step reportable without being taken.
	//
	// Nothing is recorded in state.json: that is an Environment's Observed state, and
	// `server add` names no Environment. The installed version stays readable from the
	// Server at any time, because the symlink target carries it. See
	// docs/product-description.md, "The Shim".
	step, err := inst.shimStep(ctx, remote, dryRun)
	if err != nil {
		return fmt.Errorf("installing the shim on %s: %w", name, err)
	}

	if !dryRun {
		if err := os.WriteFile(path, updated, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", filepath.Base(path), err)
		}
	}

	if err := env.Renderer.Result(&ServerAddResult{
		Name:     name,
		Host:     srv.Host,
		User:     srv.User,
		Platform: step.Platform.String(),
		Shim:     step,
		DryRun:   dryRun,
	})
}
