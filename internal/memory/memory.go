// Package memory registers the configured memory backend with every agent, and keeps it
// whole.
//
// Two layers have to agree for a phase to reach a memory tool: the agent declares the tool,
// and the MCP server exposes it. Package agents owns the first. This owns the second, and
// checks the first from the outside, because the two fail differently and only one of them
// is visible.
//
// An agent that declares a tool the server does not expose sees no tool at all, which from
// inside a phase is indistinguishable from a backend that cannot do the thing - the failure
// this whole arrangement exists to stop. So the registration is written by the installer
// rather than pasted from a document, and a narrowed one already on disk is repaired rather
// than reported.
package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/markosAMO/alfred/internal/agents"
	"github.com/markosAMO/alfred/internal/jsonobj"
)

// profile is every tool the backend exposes, never a subset. A subset is a copy of the
// backend's surface that nothing keeps in step, and it degrades silently as the backend
// grows.
const profile = "all"

// ConfiguredBackend is the `memory.backend` value, read without a YAML parser.
func ConfiguredBackend(alfredHome string) string {
	f, err := os.Open(filepath.Join(alfredHome, "alfred.config.yaml"))
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	inMemory := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "memory:") {
			inMemory = true
			continue
		}
		if !inMemory {
			continue
		}
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			break
		}
		key, value, _ := strings.Cut(strings.TrimSpace(line), ":")
		if key == "backend" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// binary is the absolute path, because an agent does not inherit the shell's PATH.
func binary(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return name
}

// server is one MCP server entry. The two agents spell a stdio server differently: Claude
// Code splits command and args, OpenCode carries both in one list, so Command is either a
// string or a list.
type server struct {
	Type    string    `json:"type"`
	Command any       `json:"command"`
	Args    []string  `json:"args,omitempty"`
	Env     *struct{} `json:"env,omitempty"`
	Enabled *bool     `json:"enabled,omitempty"`
}

// wanted is the registration for one agent: same registration, two shapes.
func wanted(kind, name string) server {
	if kind == "opencode" {
		enabled := true
		return server{
			Type:    "local",
			Command: []string{binary(name), "mcp", "--tools=" + profile},
			Enabled: &enabled,
		}
	}
	return server{
		Type:    "stdio",
		Command: binary(name),
		Args:    []string{"mcp", "--tools=" + profile},
		Env:     &struct{}{},
	}
}

// invocation is the argument list, whichever shape the agent writes it in, and false when
// the entry is not an object at all.
func invocation(raw json.RawMessage) ([]string, bool) {
	var entry map[string]json.RawMessage
	if json.Unmarshal(raw, &entry) != nil || entry == nil {
		return nil, false
	}

	var list []any
	if json.Unmarshal(entry["command"], &list) == nil && list != nil {
		return stringsOf(list), true
	}

	var parts []string
	var command string
	if json.Unmarshal(entry["command"], &command) == nil && command != "" {
		parts = append(parts, command)
	}
	var args []any
	_ = json.Unmarshal(entry["args"], &args)
	return append(parts, stringsOf(args)...), true
}

func stringsOf(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// diagnose returns "missing", "narrowed", or "" when the registration is whole.
func diagnose(raw json.RawMessage) string {
	parts, ok := invocation(raw)
	if !ok {
		return "missing"
	}

	hasMCP := false
	tools, found := "", false
	for _, part := range parts {
		if part == "mcp" {
			hasMCP = true
		}
		if value, ok := strings.CutPrefix(part, "--tools="); ok && !found {
			tools, found = value, true
		}
	}
	if !hasMCP {
		return "missing"
	}
	// No --tools at all is the backend's own default, which is every tool.
	if !found || strings.TrimSpace(tools) == profile {
		return ""
	}
	return "narrowed"
}

// load reads a config with its keys in their order, so everything this does not own is
// written back exactly where it was. A file that does not parse is treated as empty, as
// the agent itself would have to.
func load(path string) *jsonobj.Object {
	data, err := os.ReadFile(path)
	if err != nil {
		return jsonobj.New()
	}
	doc, err := jsonobj.Parse(data)
	if err != nil {
		return jsonobj.New()
	}
	return doc
}

func save(path string, doc *jsonobj.Object) error {
	data, err := jsonobj.Format(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// handle diagnoses one agent's config, repairing it when asked, and returns the status.
func handle(path, container, kind, name string, apply bool) (string, error) {
	doc := load(path)
	servers := doc.Child(container)

	current, _ := servers.Get(name)
	status := diagnose(current)
	if status == "" || !apply {
		return status, nil
	}

	entry, err := jsonobj.Raw(wanted(kind, name))
	if err != nil {
		return status, err
	}
	servers.Set(name, entry)
	doc.SetChild(container, servers)
	return status, save(path, doc)
}

// Register checks, and with apply repairs, the backend's registration with every target.
// Targets are `claude=<home>` or `opencode=<config.json>`. It returns the number of
// problems left; `check` never writes.
func Register(out io.Writer, alfredHome string, targets []string, apply bool) (int, error) {
	backend := ConfiguredBackend(alfredHome)
	if backend == "" || backend == "none" {
		if backend == "" {
			backend = "unset"
		}
		_, _ = fmt.Fprintf(out, "memory backend is %s; nothing to register\n", backend)
		return 0, nil
	}
	if _, err := exec.LookPath(backend); err != nil {
		// Configured but absent: report it, do not invent a registration for a binary that
		// is not there. `memory.required` decides whether that is fatal, at run time.
		_, _ = fmt.Fprintf(out, "FAIL  %s is the configured backend and is not on PATH\n", backend)
		_, _ = fmt.Fprintf(out, "      install it, or set memory.backend: none in %s/alfred.config.yaml\n", alfredHome)
		//nolint:nilerr // the count is the diagnosis and the error is reserved for a check
		// that could not run: an absent backend is a problem this function reports and
		// counts, not a failure of the reporting.
		return 1, nil
	}

	problems := 0
	for _, target := range targets {
		kind, where, _ := strings.Cut(target, "=")
		var path, container string
		switch kind {
		case "claude":
			path, container = filepath.Join(where, ".claude.json"), "mcpServers"
		case "opencode":
			path, container = where, "mcp"
		default:
			continue
		}

		status, err := handle(path, container, kind, backend, apply)
		if err != nil {
			return problems, err
		}
		label := kind + ": " + backend
		switch {
		case status == "":
			_, _ = fmt.Fprintf(out, "ok    %s registered with every tool\n", label)
		case apply:
			_, _ = fmt.Fprintf(out, "fixed %s was %s, now --tools=%s\n", label, status, profile)
		default:
			problems++
			_, _ = fmt.Fprintf(out, "FAIL  %s registration is %s\n", label, status)
			_, _ = fmt.Fprintln(out, "      run: ./install.sh update")
		}
	}
	return problems, nil
}

var memToolPattern = regexp.MustCompile(`mem_[a-z_]+`)

// Declared reports whether every alfred-*.md definition under agentsDir declares every
// memory tool. agents.MemoryTools is the authority on the list, so it is asked rather than
// copied: a check carrying its own copy is one more thing to fall out of step, which is the
// failure being checked for.
func Declared(agentsDir string) bool {
	files, err := filepath.Glob(filepath.Join(agentsDir, "alfred-*.md"))
	if err != nil || len(files) == 0 {
		return false
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return false
		}
		declared := map[string]bool{}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "tools:") {
				for _, tool := range memToolPattern.FindAllString(line, -1) {
					declared[tool] = true
				}
				break
			}
		}
		for _, tool := range agents.MemoryTools {
			if !declared[tool] {
				return false
			}
		}
	}
	return true
}
