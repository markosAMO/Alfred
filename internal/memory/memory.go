// Package memory registers the configured memory backend as an MCP server with every agent
// and repairs a registration that is missing or narrowed to a subset of tools. It also checks
// that agent definitions declare every memory tool, since a mismatch fails silently.
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

// profile is the --tools value: every tool, never a subset that would fall out of step as
// the backend grows.
const profile = "all"

// ConfiguredBackend returns the `memory.backend` value from alfred.config.yaml, read line by
// line without a YAML parser, or "" when it is absent.
func ConfiguredBackend(alfredHome string) string {
	file, err := os.Open(filepath.Join(alfredHome, "alfred.config.yaml"))
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()

	inMemory := false
	scanner := bufio.NewScanner(file)
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

// binary returns the absolute path of name (or name itself when not found), because an
// agent does not inherit the shell's PATH.
func binary(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return name
}

// server is one MCP server entry. Claude Code splits command and args; OpenCode puts both
// in one list, so Command is either a string or a list.
type server struct {
	Type    string    `json:"type"`
	Command any       `json:"command"`
	Args    []string  `json:"args,omitempty"`
	Env     *struct{} `json:"env,omitempty"`
	Enabled *bool     `json:"enabled,omitempty"`
}

// wanted builds the correct registration of backend name for an agent kind
// ("opencode" or Claude Code).
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

// invocation returns an entry's full command line in either agent's shape, and false when
// the entry is not a JSON object.
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

// stringsOf keeps the string items of a list, dropping anything else.
func stringsOf(items []any) []string {
	texts := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			texts = append(texts, text)
		}
	}
	return texts
}

// diagnose returns "missing", "narrowed", or "" when the registration exposes every tool.
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

// load reads an agent config keeping key order, so keys Alfred does not own are written
// back unchanged. A missing or unparsable file is treated as empty.
func load(path string) *jsonobj.Object {
	data, err := os.ReadFile(path)
	if err != nil {
		return jsonobj.New()
	}
	config, err := jsonobj.Parse(data)
	if err != nil {
		return jsonobj.New()
	}
	return config
}

// save writes an agent config indented, creating its directory if needed.
func save(path string, config *jsonobj.Object) error {
	data, err := jsonobj.Format(config)
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
	config := load(path)
	servers := config.Child(container)

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
	config.SetChild(container, servers)
	return status, save(path, config)
}

// Register checks, and with apply repairs, the backend's registration with every target
// (`claude=<home>` or `opencode=<config.json>`). It returns the number of problems left;
// without apply it never writes.
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
		// Report a missing binary instead of registering it; `memory.required` decides at run
		// time whether that is fatal.
		_, _ = fmt.Fprintf(out, "FAIL  %s is the configured backend and is not on PATH\n", backend)
		_, _ = fmt.Fprintf(out, "      install it, or set memory.backend: none in %s/alfred.config.yaml\n", alfredHome)
		//nolint:nilerr // an absent backend is a counted problem, not a failure to check.
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

// memToolPattern matches a memory tool name such as mem_save.
var memToolPattern = regexp.MustCompile(`mem_[a-z_]+`)

// Declared reports whether every alfred-*.md definition under agentsDir declares every tool
// in agents.MemoryTools on its `tools:` line. The list is read from agents, never copied.
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
