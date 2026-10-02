// Package agents generates agent definitions from a model profile.
//
// One orchestrator that only delegates, and one subagent per phase that only executes.
// Written for whichever agents are installed, from the same profile, so a model change is
// made in one place.
//
// The prompts live in prompts/*.tmpl rather than in string literals: they are prose, they
// are the part most often edited, and a Go literal would have to escape the backticks that
// run through all of them. They are embedded, so the binary still ships alone.
package agents

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// MemoryTools is every memory tool the backend exposes, and every phase gets all of them.
//
// This used to be carved up per phase, from memory/CONTRACT.md, on the reasoning that a
// phase which never supersedes an entry has no use for mem_update. The economy was real -
// each declared tool is schema the agent carries before it reads a line - and it was the
// wrong trade, for a reason an end-to-end run made plain.
//
// A phase cannot tell a tool it was not given from a backend that cannot do the thing. Both
// read as absence from inside the phase. An archive run hit `judgment_required` on every
// save and had no `mem_judge` to settle it, so it recorded six open conflicts and moved on -
// correct behaviour under the contract, and a worse outcome than settling them, caused
// entirely by a list written before `mem_judge` existed.
//
// That failure mode repeats every time the backend grows a tool: the carve-up is a copy of
// the backend's surface that nothing keeps in step, and it degrades silently. The contract
// in memory/CONTRACT.md governs which operations a phase *calls*; the tool list governs what
// is *reachable*. Those are different questions and only the first belongs in a skill.
var MemoryTools = []string{
	"mem_search", "mem_get_observation", "mem_save", "mem_update", "mem_context",
	"mem_save_prompt", "mem_suggest_topic_key", "mem_judge", "mem_review",
	"mem_compare", "mem_capture_passive", "mem_session_start", "mem_session_end",
	"mem_session_summary", "mem_current_project", "mem_list_projects",
	"mem_merge_projects", "mem_pin", "mem_unpin", "mem_doctor",
	"mem_stats", "mem_timeline", "mem_delete",
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

// Profile is the model assignment.
type Profile struct {
	OrchestratorModel string              `json:"orchestrator"`
	CoordinatorModel  string              `json:"coordinator"`
	Manage            string              `json:"manage"`
	Phases            map[string]string   `json:"phases"`
	MemoryToolPrefix  string              `json:"memory_tool_prefix"`
	Efforts           map[string]string   `json:"effort"`
	ExtraTools        map[string][]string `json:"extra_tools"`
}

func LoadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if p.Phases == nil {
		return nil, fmt.Errorf("%s: no phases", path)
	}
	return &p, nil
}

func (p *Profile) Orchestrator() string { return p.OrchestratorModel }

// PhaseNames returns the phases in name order, so every run writes them the same way.
func (p *Profile) PhaseNames() []string {
	names := make([]string, 0, len(p.Phases))
	for name := range p.Phases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (p *Profile) PhaseModel(phase string) string { return p.Phases[phase] }

// Coordinator is assigned separately: it relays and decides nothing.
//
// A profile written before the key existed falls back to the orchestrator's model rather
// than to a guess: the previous behaviour, which is wrong only in being expensive.
func (p *Profile) Coordinator() string {
	if p.CoordinatorModel != "" {
		return p.CoordinatorModel
	}
	return p.Orchestrator()
}

// ManageModel is used for bookkeeping operations: the init model is a sensible default.
func (p *Profile) ManageModel() string {
	if p.Manage != "" {
		return p.Manage
	}
	if m := p.Phases["init"]; m != "" {
		return m
	}
	return p.Orchestrator()
}

func (p *Profile) Effort(phase string) string { return p.Efforts[phase] }

// Tools returns the base tools, the memory tools with the configured prefix, then the
// per-project extras.
//
// Memory is opt-in: the installer asks, and writes the prefix only when the answer is yes.
// A profile without the key gets no memory tools, since a tool with no server behind it is
// schema carried for nothing.
func (p *Profile) Tools(phase string) []string {
	base, ok := phaseTools[phase]
	if !ok {
		base = defaultTools
	}
	tools := append([]string(nil), base...)

	if p.MemoryToolPrefix != "" {
		for _, name := range MemoryTools {
			tools = append(tools, p.MemoryToolPrefix+name)
		}
	}

	return append(tools, p.ExtraTools[phase]...)
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
