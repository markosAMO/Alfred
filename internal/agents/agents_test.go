package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalProfile = `{
  "orchestrator": "anthropic/claude-opus-5",
  "coordinator": "anthropic/claude-haiku-4-5-20251001",
  "phases": {"spec": "anthropic/claude-opus-5", "apply": "vendor/small", "init": "vendor/mid"},
  "memory_tool_prefix": "mcp__engram__",
  "effort": {"spec": "high"},
  "extra_tools": {"spec": ["mcp__atlassian__getJiraIssue"]}
}`

func profileFrom(t *testing.T, body string) *Profile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPhaseNamesAreSorted(t *testing.T) {
	p := profileFrom(t, minimalProfile)

	if got := strings.Join(p.PhaseNames(), ","); got != "apply,init,spec" {
		t.Errorf("PhaseNames = %s", got)
	}
}

func TestToolsComposeBaseMemoryAndExtras(t *testing.T) {
	p := profileFrom(t, minimalProfile)

	got := strings.Join(p.Tools("spec"), ", ")
	memory := make([]string, len(MemoryTools))
	for i, name := range MemoryTools {
		memory[i] = "mcp__engram__" + name
	}
	want := "Read, Write, Edit, Glob, Grep, " + strings.Join(memory, ", ") +
		", mcp__atlassian__getJiraIssue"
	if got != want {
		t.Errorf("Tools(spec) =\n  %s\nwant\n  %s", got, want)
	}
}

// Every phase reaches every memory tool: a phase cannot tell a tool it was not given from a
// backend that cannot do the thing.
func TestEveryPhaseGetsEveryMemoryTool(t *testing.T) {
	p := profileFrom(t, minimalProfile)

	for _, phase := range []string{"init", "apply", "spec", "alfred"} {
		got := map[string]bool{}
		for _, tool := range p.Tools(phase) {
			got[tool] = true
		}
		for _, name := range MemoryTools {
			if !got["mcp__engram__"+name] {
				t.Errorf("Tools(%s) is missing %s", phase, name)
			}
		}
	}
}

func TestToolsWithoutMemoryPrefixOmitsMemoryTools(t *testing.T) {
	p := profileFrom(t, `{"orchestrator": "m", "phases": {"spec": "m"}, "memory_tool_prefix": ""}`)

	for _, tool := range p.Tools("spec") {
		if strings.Contains(tool, "mem_") {
			t.Errorf("an empty prefix should drop the memory tools, got %s", tool)
		}
	}
}

// Memory is opt-in: a profile that never answered the installer's question has none.
func TestToolsWithoutAMemoryKeyHaveNoMemoryTools(t *testing.T) {
	p := profileFrom(t, `{"orchestrator": "m", "phases": {"spec": "m"}}`)

	for _, tool := range p.Tools("spec") {
		if strings.Contains(tool, "mem_") {
			t.Errorf("a profile without memory_tool_prefix should get no memory tools, got %s", tool)
		}
	}
}

func TestToolsForUnknownPhaseFallsBackToTheDefaultSet(t *testing.T) {
	p := profileFrom(t, `{"orchestrator": "m", "phases": {"invented": "m"}}`)

	if got := strings.Join(p.Tools("invented"), ", "); got != "Read, Write, Glob, Grep" {
		t.Errorf("Tools(invented) = %s", got)
	}
}

func TestModelFallbacks(t *testing.T) {
	full := profileFrom(t, minimalProfile)
	if got := full.Coordinator(); got != "anthropic/claude-haiku-4-5-20251001" {
		t.Errorf("Coordinator = %s", got)
	}
	// manage is unset, so it falls back to init's model before the orchestrator's.
	if got := full.ManageModel(); got != "vendor/mid" {
		t.Errorf("ManageModel = %s, want init's model", got)
	}

	bare := profileFrom(t, `{"orchestrator": "only/model", "phases": {"spec": "only/model"}}`)
	if got := bare.Coordinator(); got != "only/model" {
		t.Errorf("a profile without a coordinator should fall back to the orchestrator, got %s", got)
	}
	if got := bare.ManageModel(); got != "only/model" {
		t.Errorf("without init, manage should fall back to the orchestrator, got %s", got)
	}

	explicit := profileFrom(t, `{"orchestrator": "o", "manage": "m/explicit", "phases": {"init": "i"}}`)
	if got := explicit.ManageModel(); got != "m/explicit" {
		t.Errorf("an explicit manage model should win, got %s", got)
	}
}

func TestBareModelDropsOnlyTheFirstVendorSegment(t *testing.T) {
	cases := map[string]string{
		"anthropic/claude-opus-5": "claude-opus-5",
		"no-vendor":               "no-vendor",
		"a/b/c":                   "b/c",
	}
	for in, want := range cases {
		if got := bareModel(in); got != want {
			t.Errorf("bareModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadProfileRejectsAProfileWithoutPhases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.json")
	if err := os.WriteFile(path, []byte(`{"orchestrator": "m"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfile(path); err == nil {
		t.Error("expected an error for a profile with no phases")
	}
}

func TestWorktreeIsRejectedAsAPhase(t *testing.T) {
	p := profileFrom(t, `{"orchestrator": "m", "phases": {"worktree": "m"}}`)

	if _, err := Opencode(p, "/skills"); err == nil {
		t.Error("'worktree' is the fleet orchestrator and must not be accepted as a phase")
	}
}

func TestEveryPlaceholderIsResolved(t *testing.T) {
	p := profileFrom(t, minimalProfile)

	prompts := map[string]string{
		"orchestrator": OrchestratorPrompt("/skills", p.PhaseNames(), "`/alfred-worktree`"),
		"fleet":        FleetPrompt("/skills", p.PhaseNames()),
		"coordinator":  CoordinatorPrompt("/skills", "claude-opus-5"),
		"subagent":     SubagentPrompt("spec", "/skills/spec/SKILL.md"),
		"manage":       ManagePrompt("/skills/alfred/SKILL.md"),
	}

	for name, text := range prompts {
		if strings.Contains(text, "{{") {
			t.Errorf("%s prompt still holds an unresolved placeholder", name)
		}
		if strings.Contains(text, "ORCHESTRATOR_MODEL") {
			t.Errorf("%s prompt still holds the model sentinel", name)
		}
	}

	if !strings.Contains(prompts["coordinator"], "claude-opus-5") {
		t.Error("the coordinator prompt should name the session model")
	}
	// bin/ sits next to skills/, so the tool path is derived rather than passed.
	if !strings.Contains(prompts["fleet"], "/bin/worktree.sh") {
		t.Error("the fleet prompt should name the worktree tool")
	}
}

func TestOpencodeToolsAreExhaustive(t *testing.T) {
	entry := asBooleans([]string{"Read", "Bash"}, false)

	// Every tool OpenCode knows about is named, so none is inherited by omission.
	if len(entry) != len(opencodeNames) {
		t.Errorf("got %d tool flags, want %d", len(entry), len(opencodeNames))
	}
	if !entry["read"] || !entry["bash"] || entry["write"] {
		t.Error("declared tools should be true and the rest false")
	}
	if entry["task"] {
		t.Error("task should follow allowTask")
	}
	if !asBooleans(nil, true)["task"] {
		t.Error("allowTask should enable task")
	}
}

func TestMergeOpencodeKeepsForeignAgentsAndReplacesAlfredOnes(t *testing.T) {
	p := profileFrom(t, minimalProfile)
	target := filepath.Join(t.TempDir(), "opencode.json")

	existing := `{"theme": "dark", "$schema": "https://opencode.ai/config.json",` +
		` "agent": {"mine": {"model": "keep me"}, "alfred": {"model": "stale"},` +
		` "alfred-gone": {"model": "stale"}, "alfred-spec": {"model": "stale"}, "zz": {}}}`
	if err := os.WriteFile(target, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	config, err := Opencode(p, "/skills")
	if err != nil {
		t.Fatal(err)
	}
	if err := MergeOpencode(target, config); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	if !strings.Contains(text, `"keep me"`) {
		t.Error("an agent that is not Alfred's should survive the merge")
	}
	if strings.Contains(text, `"stale"`) {
		t.Error("the alfred-* agents should have been replaced")
	}
	if !strings.Contains(text, `"theme": "dark"`) {
		t.Error("unrelated top-level keys should survive")
	}
	if !strings.HasSuffix(text, "}\n") {
		t.Error("the file should end with a newline")
	}

	// The file is OpenCode's: keys stay where they were, an Alfred agent is replaced in
	// place, and one the profile no longer has is gone.
	order := func(keys ...string) bool {
		last := -1
		for _, k := range keys {
			i := strings.Index(text, k)
			if i < last {
				return false
			}
			last = i
		}
		return true
	}
	if !order(`"theme"`, `"$schema"`, `"agent"`) {
		t.Error("top-level keys were reordered")
	}
	if !order(`"mine"`, `"alfred":`, `"alfred-spec"`, `"zz"`) {
		t.Errorf("agents were reordered:\n%s", text)
	}
	if strings.Contains(text, `"alfred-gone"`) {
		t.Error("an Alfred agent the profile no longer has should be removed")
	}
}

func TestClaudeAgentsWritesOneFilePerPhasePlusCommands(t *testing.T) {
	p := profileFrom(t, minimalProfile)
	home := t.TempDir()

	count, err := ClaudeAgents(p, "/skills", home)
	if err != nil {
		t.Fatal(err)
	}
	// three phases, alfred-manage, and the two slash commands
	if count != 6 {
		t.Errorf("wrote %d files, want 6", count)
	}

	spec, err := os.ReadFile(filepath.Join(home, "agents/alfred-spec.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(spec)

	for _, want := range []string{
		"name: alfred-spec\n",
		"model: claude-opus-5\n", // the vendor prefix is dropped
		"effort: high\n",         // only the phases that set it carry the line
		"mcp__atlassian__getJiraIssue\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("alfred-spec.md is missing %q", want)
		}
	}

	apply, err := os.ReadFile(filepath.Join(home, "agents/alfred-apply.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(apply), "effort:") {
		t.Error("a phase with no effort set should carry no effort line")
	}

	command, err := os.ReadFile(filepath.Join(home, "commands/alfred-worktree.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(command), "model: claude-haiku-4-5-20251001\n") {
		t.Error("the worktree command should run the coordinator's model")
	}
	if !strings.HasSuffix(string(command), "$ARGUMENTS\n") {
		t.Error("a command should end with the argument placeholder")
	}
}
