// Package agents generates agent definitions from a model profile.
//
// One orchestrator that only delegates, and one subagent per phase that only executes.
// Written for whichever agents are installed, from the same profile, so a model change is
// made in one place. This is the port of scripts/generate_agents.py.
//
// The prompts live in prompts/*.tmpl rather than in string literals: they are prose, they
// are the part most often edited, and a Go literal would have to escape the backticks that
// run through all of them. They are embedded, so the binary still ships alone.
package agents

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/markosAMO/alfred/internal/pyjson"
)

//go:embed prompts/*.tmpl
var promptFS embed.FS

func prompt(name string) string {
	data, err := promptFS.ReadFile("prompts/" + name)
	if err != nil {
		panic("missing embedded prompt: " + name)
	}
	return string(data)
}

func fill(text string, pairs ...string) string {
	for i := 0; i+1 < len(pairs); i += 2 {
		text = strings.ReplaceAll(text, pairs[i], pairs[i+1])
	}
	return text
}

// phaseTools are the tools per phase. A phase that does not write code does not get Edit;
// a phase that does not run anything does not get Bash.
var phaseTools = map[string][]string{
	"init":     {"Read", "Write", "Edit", "Glob", "Grep", "Bash"},
	"explore":  {"Read", "Write", "Glob", "Grep", "Bash"},
	"refine":   {"Read", "Write", "Glob", "Grep", "WebFetch"},
	"research": {"Read", "Write", "Glob", "Grep", "WebSearch", "WebFetch"},
	"spec":     {"Read", "Write", "Edit", "Glob", "Grep"},
	"diagnose": {"Read", "Write", "Glob", "Grep", "Bash", "WebFetch"},
	"design":   {"Read", "Write", "Glob", "Grep"},
	"tasks":    {"Read", "Write", "Glob", "Grep"},
	"apply":    {"Read", "Write", "Edit", "Glob", "Grep", "Bash"},
	"verify":   {"Read", "Write", "Glob", "Grep", "Bash"},
	"review":   {"Read", "Write", "Glob", "Grep", "Bash"},
	"archive":  {"Read", "Write", "Edit", "Glob", "Grep", "Bash"},
	"alfred":   {"Read", "Write", "Glob", "Grep", "Bash"},
}

// phaseMemory are the memory operations per phase, from memory/CONTRACT.md. Only what each
// phase actually calls: a phase that never supersedes an entry has no use for mem_update.
var phaseMemory = map[string][]string{
	"init":     {"mem_save"},
	"explore":  {"mem_save"},
	"refine":   {"mem_search", "mem_get_observation", "mem_save", "mem_context"},
	"research": {"mem_search", "mem_get_observation", "mem_save"},
	"spec":     {"mem_search", "mem_get_observation", "mem_save"},
	"diagnose": {"mem_search", "mem_get_observation", "mem_save", "mem_context"},
	"design":   {"mem_search", "mem_get_observation", "mem_save"},
	"tasks":    {"mem_search", "mem_get_observation", "mem_save"},
	"apply":    {"mem_search", "mem_get_observation", "mem_save", "mem_update"},
	"verify":   {"mem_search", "mem_get_observation", "mem_save"},
	"review":   {"mem_search", "mem_get_observation", "mem_save"},
	"archive":  {"mem_search", "mem_get_observation", "mem_save", "mem_update"},
	"alfred":   {"mem_search", "mem_get_observation", "mem_save"},
}

var defaultTools = []string{"Read", "Write", "Glob", "Grep"}

var orchestratorTools = []string{"Task", "Read"}

// coordinatorTools: the coordinator runs no phase, so it has no Task for them; it keeps
// Task for alfred-manage, which opens and closes the worktrees. Bash starts the sessions,
// Write keeps .alfred/coordinator.yaml, and the two message tools are the whole of its
// conversation with the sessions it started.
var coordinatorTools = []string{"Task", "Read", "Write", "Bash", "SendMessage", "ListAgents"}

var opencodeNames = map[string]string{
	"Read": "read", "Write": "write", "Edit": "edit", "Bash": "bash",
	"Glob": "glob", "Grep": "grep", "Task": "task",
	"WebFetch": "webfetch", "WebSearch": "websearch",
}

// Profile is the model assignment, read with its key order intact because the order of
// `phases` decides the order agents are written in.
type Profile struct{ doc *pyjson.Value }

func LoadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := pyjson.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Get("phases") == nil {
		return nil, fmt.Errorf("%s: no phases", path)
	}
	return &Profile{doc: doc}, nil
}

func (p *Profile) Orchestrator() string { return p.doc.Get("orchestrator").StringOr("") }

// PhaseNames returns the phases in profile order.
func (p *Profile) PhaseNames() []string { return p.doc.Get("phases").Keys() }

// SortedPhaseNames returns the phases in name order, as the prompts list them.
func (p *Profile) SortedPhaseNames() []string {
	names := append([]string(nil), p.PhaseNames()...)
	sort.Strings(names)
	return names
}

func (p *Profile) PhaseModel(phase string) string {
	return p.doc.Get("phases").Get(phase).StringOr("")
}

// Coordinator is assigned separately: it relays and decides nothing.
//
// A profile written before the key existed falls back to the orchestrator's model rather
// than to a guess: the previous behaviour, which is wrong only in being expensive.
func (p *Profile) Coordinator() string {
	if m := p.doc.Get("coordinator").StringOr(""); m != "" {
		return m
	}
	return p.Orchestrator()
}

// ManageModel is used for bookkeeping operations: the init model is a sensible default.
func (p *Profile) ManageModel() string {
	if m := p.doc.Get("manage").StringOr(""); m != "" {
		return m
	}
	if m := p.doc.Get("phases").Get("init").StringOr(""); m != "" {
		return m
	}
	return p.Orchestrator()
}

func (p *Profile) Effort(phase string) string {
	return p.doc.Get("effort").Get(phase).StringOr("")
}

// Tools returns the base tools, the memory tools with the configured prefix, then the
// per-project extras.
func (p *Profile) Tools(phase string) []string {
	base, ok := phaseTools[phase]
	if !ok {
		base = defaultTools
	}
	tools := append([]string(nil), base...)

	prefix := "mcp__engram__"
	if v := p.doc.Get("memory_tool_prefix"); v != nil {
		prefix = v.StringOr("")
	}
	if prefix != "" {
		for _, name := range phaseMemory[phase] {
			tools = append(tools, prefix+name)
		}
	}

	return append(tools, p.doc.Get("extra_tools").Get(phase).Strings()...)
}

// bareModel drops a vendor prefix: Claude Code names the model without one.
func bareModel(model string) string {
	if _, rest, found := strings.Cut(model, "/"); found {
		return rest
	}
	return model
}

func subagentList(phases []string) string {
	names := make([]string, len(phases))
	for i, phase := range phases {
		names[i] = "alfred-" + phase
	}
	return strings.Join(names, ", ")
}

// worktreeTool: bin/ sits next to skills/ in the installation, so the tool path is derived
// from the skills root rather than passed separately.
func worktreeTool(skillsRoot string) string {
	return filepath.Join(filepath.Dir(skillsRoot), "bin", "worktree.sh")
}

// OrchestratorPrompt is the orchestrator for one change, in the checkout it was started
// from. Written once for both agents: what differs is how the fleet orchestrator is named,
// so that is the one thing passed in.
func OrchestratorPrompt(skillsRoot string, phases []string, fleetEntry string) string {
	return fill(prompt("orchestrator.tmpl"),
		"{{SKILLS_ROOT}}", skillsRoot,
		"{{SUBAGENTS}}", subagentList(phases),
		"{{FLEET_ENTRY}}", fleetEntry)
}

// FleetPrompt is the same orchestrator, for several changes at once, one worktree each.
//
// Kept apart from the single-change orchestrator rather than made a mode of it: the input
// is a list with a branch per line, the working set is one state file per change, and a
// user who wants one change in the current checkout should not have to opt out of
// worktrees to get it.
func FleetPrompt(skillsRoot string, phases []string) string {
	return fill(prompt("fleet.tmpl"),
		"{{SKILLS_ROOT}}", skillsRoot,
		"{{SUBAGENTS}}", subagentList(phases),
		"{{TOOL}}", worktreeTool(skillsRoot))
}

// CoordinatorPrompt is the worktree coordinator: it starts a session per change and routes
// messages. The sessions it starts run the orchestrator's model, not its own.
func CoordinatorPrompt(skillsRoot, sessionModel string) string {
	return fill(prompt("coordinator.tmpl"),
		"{{SESSION_MODEL}}", sessionModel,
		"{{SKILLS_ROOT}}", skillsRoot,
		"{{TOOL}}", worktreeTool(skillsRoot))
}

func SubagentPrompt(phase, skillPath string) string {
	return fill(prompt("subagent.tmpl"), "{{PHASE}}", phase, "{{SKILL_PATH}}", skillPath)
}

func ManagePrompt(skillPath string) string {
	return fill(prompt("manage.tmpl"), "{{SKILL_PATH}}", skillPath)
}
