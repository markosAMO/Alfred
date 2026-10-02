package memory

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markosAMO/alfred/internal/agents"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConfiguredBackendReadsOnlyTheMemoryBlock(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "alfred.config.yaml"),
		"notify:\n  backend: slack\nmemory:\n  documents: ephemeral\n  backend: engram\ntracker:\n  backend: jira\n")

	if got := ConfiguredBackend(home); got != "engram" {
		t.Errorf("ConfiguredBackend = %q, want engram", got)
	}
	if got := ConfiguredBackend(t.TempDir()); got != "" {
		t.Errorf("no config should read as unset, got %q", got)
	}
}

func TestDiagnose(t *testing.T) {
	for body, want := range map[string]string{
		`{"command": "engram", "args": ["mcp", "--tools=all"]}`:   "",
		`{"command": "engram", "args": ["mcp"]}`:                  "",
		`{"command": ["engram", "mcp", "--tools=all"]}`:           "",
		`{"command": "engram", "args": ["mcp", "--tools=agent"]}`: "narrowed",
		`{"command": ["engram", "mcp", "--tools=admin"]}`:         "narrowed",
		`{"command": "engram", "args": ["serve"]}`:                "missing",
		`"not an object"`: "missing",
	} {
		if got := diagnose([]byte(body)); got != want {
			t.Errorf("diagnose(%s) = %q, want %q", body, got, want)
		}
	}
	if got := diagnose(nil); got != "missing" {
		t.Errorf("diagnose(nil) = %q, want missing", got)
	}
}

func TestHandleRepairsANarrowedRegistrationAndKeepsTheRest(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	write(t, path, `{"zeta": 1, "theme": "dark", "mcpServers": {"other": {"command": "x"}, "engram": {"command": "engram", "args": ["mcp", "--tools=agent"]}}}`)

	status, err := handle(path, "mcpServers", "claude", "engram", false)
	if err != nil || status != "narrowed" {
		t.Fatalf("check: status %q, err %v", status, err)
	}

	if _, err := handle(path, "mcpServers", "claude", "engram", true); err != nil {
		t.Fatal(err)
	}
	status, err = handle(path, "mcpServers", "claude", "engram", false)
	if err != nil || status != "" {
		t.Fatalf("after apply: status %q, err %v", status, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"other"`) {
		t.Error("apply dropped content it does not own")
	}
	// zeta was written before theme and stays there: the file is the agent's, not ours.
	if strings.Index(text, `"zeta"`) > strings.Index(text, `"theme"`) ||
		strings.Index(text, `"other"`) > strings.Index(text, `"engram"`) {
		t.Errorf("apply reordered keys it does not own:\n%s", text)
	}
}

func TestRegisterWithoutABackendTouchesNothing(t *testing.T) {
	home, target := t.TempDir(), t.TempDir()
	write(t, filepath.Join(home, "alfred.config.yaml"), "memory:\n  backend: none\n")

	problems, err := Register(io.Discard, home, []string{"claude=" + target}, true)
	if err != nil || problems != 0 {
		t.Fatalf("problems %d, err %v", problems, err)
	}
	if _, err := os.Stat(filepath.Join(target, ".claude.json")); !os.IsNotExist(err) {
		t.Error("a backend of none should register nothing")
	}
}

func TestDeclared(t *testing.T) {
	dir := t.TempDir()
	if Declared(dir) {
		t.Error("no definitions should not count as declared")
	}

	every := make([]string, len(agents.MemoryTools))
	for i, name := range agents.MemoryTools {
		every[i] = "mcp__engram__" + name
	}
	write(t, filepath.Join(dir, "alfred-spec.md"), "---\ntools: Read, "+strings.Join(every, ", ")+"\n---\n")
	if !Declared(dir) {
		t.Error("a definition with every tool should count as declared")
	}

	write(t, filepath.Join(dir, "alfred-init.md"), "---\ntools: Read, mcp__engram__mem_save\n---\n")
	if Declared(dir) {
		t.Error("one short definition should fail the check")
	}
}
