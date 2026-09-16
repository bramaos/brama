// Package ssh runs commands on a Server over OpenSSH.
//
// brama owns no authentication. It shells out to the `ssh` binary rather than
// speaking the protocol itself, so ~/.ssh/config, ssh-agent, hardware keys and
// everything else a user has already configured work exactly as they do at the
// prompt — and brama never holds a credential it could leak.
//
// A Session multiplexes: one connection is authenticated, and every command in the
// operation rides it. Four separate `ssh` invocations would mean four handshakes,
// which with a hardware key is four touch prompts for one command.
//
// This package exports no interface. There is one implementation and, for now, one
// caller; the interface belongs to the consumer that needs to substitute it.
package ssh

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Session is an open, multiplexed connection to a Server.
type Session struct {
	host    string
	user    string
	control string
	dir     string
}

// Target is the Server to connect to. User is optional: without it, OpenSSH decides,
// exactly as `ssh <host>` would.
type Target struct {
	Host string
	User string
}

// Open authenticates once and holds the connection open for the caller's lifetime.
//
// The control socket lives in a directory of its own, removed on Close — including
// on the error paths. A predictable path shared between runs would leave a live,
// authenticated channel to production sitting around after the command returns.
func Open(ctx context.Context, t Target) (*Session, error) {
	if t.Host == "" {
		return nil, fmt.Errorf("no host to connect to")
	}

	dir, err := os.MkdirTemp("", "brama-")
	if err != nil {
		return nil, fmt.Errorf("creating control directory: %w", err)
	}

	s := &Session{
		host:    t.Host,
		user:    t.User,
		control: filepath.Join(dir, "control"),
		dir:     dir,
	}

	args := append(s.baseArgs(),
		"-o", "ControlMaster=yes",
		// The master must not outlive the operation that opened it.
		"-o", "ControlPersist=no",
		"-N", "-f",
		s.host,
	)

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("cannot reach %s: %w", t.Host, withStderr(err, stderr.String()))
	}
	return s, nil
}

// Close ends the multiplexed connection and removes the socket.
func (s *Session) Close() error {
	args := append(s.baseArgs(), "-O", "exit", s.host)
	// The master exits with the socket either way; a failure here means it was
	// already gone, which is the state Close is trying to reach.
	_ = exec.Command("ssh", args...).Run()
	return os.RemoveAll(s.dir)
}

// Run executes a command on the Server and returns its standard output.
func (s *Session) Run(ctx context.Context, command string) (string, error) {
	args := append(s.baseArgs(), s.host, command)

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), withStderr(err, stderr.String())
	}
	return stdout.String(), nil
}

// Send streams data to a command's standard input on the Server. It is how a binary
// crosses: `cat > path` on the far end, with the bytes going straight down the
// connection rather than through a second tool with its own flags and failure modes.
func (s *Session) Send(ctx context.Context, command string, data []byte) error {
	args := append(s.baseArgs(), s.host, command)

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return withStderr(err, stderr.String())
	}
	return nil
}

// baseArgs are the options every invocation shares: batch mode so a command never
// blocks on an interactive password prompt, and the control socket that makes all of
// them one connection.
func (s *Session) baseArgs() []string {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ControlPath=" + s.control,
	}
	if s.user != "" {
		args = append(args, "-l", s.user)
	}
	return args
}

// withStderr folds what ssh wrote to stderr into the error, because "exit status 255"
// on its own tells a user nothing about which of the many reasons it was.
func withStderr(err error, stderr string) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, lastLine(detail))
}

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
