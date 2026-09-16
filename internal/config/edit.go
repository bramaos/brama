package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
)

// ErrServerExists reports that the file already declares a Server under that name.
// Registering is not the verb for changing one: brama.yaml holds reviewed decisions,
// and overwriting one silently is how a reviewed decision disappears.
var ErrServerExists = errors.New("server already registered")

// AddServer returns doc with one Server added under servers.
//
// The edit is surgical on purpose. brama.yaml is a review surface a human also
// writes in, so everything outside the inserted lines comes through byte for byte —
// including the column-aligned comments `brama init` wrote. Re-marshalling the
// document, or even round-tripping it through the AST, reflows lines this write has
// no business touching. The parse still happens: it is what validates the file and
// finds the block, it just is not what renders the result.
//
// See docs/adr/0005-one-committed-config-file.md.
func AddServer(doc []byte, name string, srv Server) ([]byte, error) {
	if name == "" {
		return nil, errors.New("server name is required")
	}
	if srv.Host == "" {
		return nil, errors.New("server host is required")
	}

	existing, err := existingServers(doc)
	if err != nil {
		return nil, err
	}
	if _, ok := existing[name]; ok {
		return nil, fmt.Errorf("%w: %s", ErrServerExists, name)
	}

	lines := splitLines(doc)
	entry := renderServer(name, srv)

	if at, ok := serversBlockEnd(lines); ok {
		return joinLines(insertAt(lines, at, entry)), nil
	}

	block := append([]string{"servers:"}, entry...)
	if start, end, ok := commentedServersBlock(lines); ok {
		return joinLines(replaceRange(lines, start, end, block)), nil
	}
	return joinLines(appendBlock(lines, block)), nil
}

// existingServers reads the servers block, and in doing so proves the file parses.
// A caller adding to a file brama cannot read would be writing into a broken tree.
func existingServers(doc []byte) (map[string]Server, error) {
	if _, err := parser.ParseBytes(doc, parser.ParseComments); err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", Filename, err)
	}

	var probe struct {
		Servers map[string]Server `yaml:"servers"`
	}
	if err := yaml.Unmarshal(doc, &probe); err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", Filename, err)
	}
	return probe.Servers, nil
}

// renderServer writes one servers entry. User is omitted when unset, matching the
// struct tag: an absent user means OpenSSH decides, which is not the same as an
// empty one.
func renderServer(name string, srv Server) []string {
	out := []string{
		"  " + name + ":",
		"    host: " + srv.Host,
	}
	if srv.User != "" {
		out = append(out, "    user: "+srv.User)
	}
	return out
}

// serversBlockEnd finds the line index just past the last entry of a top-level
// servers block, or reports that the file has none.
//
// The block runs from the `servers:` key until the next line at column zero that is
// not a comment; trailing blank lines belong to whatever follows, not to the block,
// so the insertion point backs up over them.
func serversBlockEnd(lines []string) (int, bool) {
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "servers:") {
			start = i
			break
		}
	}
	if start < 0 {
		return 0, false
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" || isIndented(line) {
			continue
		}
		end = i
		break
	}
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return end, true
}

func isIndented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

// commentedServersBlock finds the commented servers example `brama init` writes, so
// the first real Server can take its place.
//
// The block is its heading — a comment whose only content is `servers:` — plus the
// comment lines indented beneath it. Indentation is what separates the example from
// ordinary prose: `#   prod:` continues it, `# No anonymize block yet.` does not.
func commentedServersBlock(lines []string) (start, end int, ok bool) {
	start = -1
	for i, line := range lines {
		if body, isComment := commentBody(line); isComment && strings.TrimSpace(body) == "servers:" {
			start = i
			break
		}
	}
	if start < 0 {
		return 0, 0, false
	}

	end = start + 1
	for end < len(lines) {
		body, isComment := commentBody(lines[end])
		if !isComment || strings.TrimSpace(body) == "" || !strings.HasPrefix(body, "  ") {
			break
		}
		end++
	}
	return start, end, true
}

// commentBody returns what follows the `#` of a whole-line comment.
func commentBody(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	return strings.TrimPrefix(trimmed, "#"), true
}

func replaceRange(lines []string, start, end int, block []string) []string {
	out := make([]string, 0, len(lines)-(end-start)+len(block))
	out = append(out, lines[:start]...)
	out = append(out, block...)
	return append(out, lines[end:]...)
}

// appendBlock puts a block at the end of the file, separated by one blank line.
func appendBlock(lines, block []string) []string {
	out := append([]string{}, lines...)
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	if len(out) > 0 {
		out = append(out, "")
	}
	return append(out, block...)
}

func insertAt(lines []string, at int, block []string) []string {
	out := make([]string, 0, len(lines)+len(block))
	out = append(out, lines[:at]...)
	out = append(out, block...)
	return append(out, lines[at:]...)
}

// splitLines splits without inventing a trailing empty line for a file that ends in
// a newline, so that joinLines is its exact inverse.
func splitLines(doc []byte) []string {
	s := string(doc)
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func joinLines(lines []string) []byte {
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}
