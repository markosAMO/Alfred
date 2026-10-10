// Package agents generates the commands and subagents Alfred installs for each agent
// (Claude Code, OpenCode) from one model profile, so a model change is made in one place.
// Prompts are embedded templates under prompts/, so the binary still ships alone.
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

// promptFS holds the prompt templates embedded into the binary.
//
//go:embed prompts/*.tmpl
var promptFS embed.FS

// prompt returns the embedded template with the given file name, panicking if it is missing.
func prompt(name string) string {
	data, err := promptFS.ReadFile("prompts/" + name)
	if err != nil {
		panic("missing embedded prompt: " + name)
	}
	return string(data)
}

// fill replaces each placeholder with its value, taking pairs in order (placeholder, value).
// Order matters: a later pair also rewrites text substituted by an earlier one.
func fill(text string, pairs ...string) string {
	for i := 0; i+1 < len(pairs); i += 2 {
		text = strings.ReplaceAll(text, pairs[i], pairs[i+1])
	}
	return text
}

// phaseTools lists the base tools of each shared phase: only phases that edit code get Edit,
// only phases that run commands get Bash.
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

// MemoryTools is every memory tool the backend exposes; every phase gets all of them.
// Do not trim it per phase: a phase cannot tell a missing tool from a backend that lacks the
// operation, and a per-phase list silently falls behind as the backend grows.
var MemoryTools = []string{
	"mem_search", "mem_get_observation", "mem_save", "mem_update", "mem_context",
	"mem_save_prompt", "mem_suggest_topic_key", "mem_judge", "mem_review",
	"mem_compare", "mem_capture_passive", "mem_session_start", "mem_session_end",
	"mem_session_summary", "mem_current_project", "mem_list_projects",
	"mem_merge_projects", "mem_pin", "mem_unpin", "mem_doctor",
	"mem_stats", "mem_timeline", "mem_delete",
}

// defaultTools is the base tool set of a phase with no entry in phaseTools.
var defaultTools = []string{"Read", "Write", "Glob", "Grep"}

// orchestratorTools is the tool set of a command that only delegates to subagents.
var orchestratorTools = []string{"Task", "Read"}

// coordinatorTools is the worktree coordinator's tool set: Task for alfred-manage, Bash to
// start sessions, Write for .alfred/coordinator.yaml, and the message tools to talk to them.
var coordinatorTools = []string{"Task", "Read", "Write", "Bash", "SendMessage", "ListAgents"}

// opencodeNames maps a Claude Code tool name to the name OpenCode uses for it.
var opencodeNames = map[string]string{
	"Read": "read", "Write": "write", "Edit": "edit", "Bash": "bash",
	"Glob": "glob", "Grep": "grep", "Task": "task",
	"WebFetch": "webfetch", "WebSearch": "websearch",
}

// Profile is the installation's model assignment: which model runs each phase and command,
// plus effort, memory tool prefix and extra tools, as read from the profile JSON.
type Profile struct {
	OrchestratorModel string              `json:"orchestrator"`
	CoordinatorModel  string              `json:"coordinator"`
	Manage            string              `json:"manage"`
	Default           string              `json:"default_workflow"`
	Phases            map[string]string   `json:"phases"`
	MemoryToolPrefix  string              `json:"memory_tool_prefix"`
	Efforts           map[string]string   `json:"effort"`
	ExtraTools        map[string][]string `json:"extra_tools"`
}

// LoadProfile reads a profile JSON file and rejects one that assigns no phases.
func LoadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if profile.Phases == nil {
		return nil, fmt.Errorf("%s: no phases", path)
	}
	return &profile, nil
}

// Orchestrator returns the model that runs workflow commands.
func (p *Profile) Orchestrator() string { return p.OrchestratorModel }

// PhaseNames returns the profile's phases sorted by name, so output is deterministic.
func (p *Profile) PhaseNames() []string {
	names := make([]string, 0, len(p.Phases))
	for name := range p.Phases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// PhaseModel returns the model assigned to a phase, or "" if none is.
func (p *Profile) PhaseModel(phase string) string { return p.Phases[phase] }

// Coordinator returns the worktree coordinator's model, falling back to the orchestrator's
// model when the profile does not set one.
func (p *Profile) Coordinator() string {
	if p.CoordinatorModel != "" {
		return p.CoordinatorModel
	}
	return p.Orchestrator()
}

// ManageModel returns the model for bookkeeping commands: the manage key, else the init
// phase's model, else the orchestrator's.
func (p *Profile) ManageModel() string {
	if p.Manage != "" {
		return p.Manage
	}
	if initModel := p.Phases["init"]; initModel != "" {
		return initModel
	}
	return p.Orchestrator()
}

// Effort returns the reasoning effort configured for a phase, or "" if none is.
func (p *Profile) Effort(phase string) string { return p.Efforts[phase] }

// DefaultWorkflow returns the workflow `/alfred` runs, or "sdd" when the profile sets none.
func (p *Profile) DefaultWorkflow() string {
	if p.Default != "" {
		return p.Default
	}
	return "sdd"
}

// Tools returns a shared phase's tools: its base set, the memory tools (only when a memory
// prefix is configured), then the profile's extras for that phase.
func (p *Profile) Tools(phase string) []string {
	base, ok := phaseTools[phase]
	if !ok {
		base = defaultTools
	}
	return append(p.withMemory(base), p.ExtraTools[phase]...)
}

// OwnTools returns the tools of a phase a workflow defines itself: the declared set (or the
// default set) plus memory tools. Profile extras are not added, since they are keyed by
// shared phase names and a workflow's own phase may reuse such a name.
func (p *Profile) OwnTools(declared []string) []string {
	if len(declared) == 0 {
		declared = defaultTools
	}
	return p.withMemory(declared)
}

// withMemory returns a copy of base with every memory tool appended under the configured
// prefix; with no prefix it returns base unchanged.
func (p *Profile) withMemory(base []string) []string {
	tools := append([]string(nil), base...)
	if p.MemoryToolPrefix != "" {
		for _, name := range MemoryTools {
			tools = append(tools, p.MemoryToolPrefix+name)
		}
	}
	return tools
}

// bareModel drops the vendor prefix of "vendor/model"; Claude Code names models without it.
func bareModel(model string) string {
	if _, rest, found := strings.Cut(model, "/"); found {
		return rest
	}
	return model
}

// worktreeTool returns the path of worktree.sh, which sits in bin/ next to the skills root.
func worktreeTool(skillsRoot string) string {
	return filepath.Join(filepath.Dir(skillsRoot), "bin", "worktree.sh")
}

// OrchestratorPrompt renders the single-change orchestrator prompt for one workflow section
// and the agent-specific name of the fleet entry point.
// The workflow section is substituted last so user text containing `{{` is never rewritten.
func OrchestratorPrompt(skillsRoot, workflowSection, fleetEntry string) string {
	text := fill(prompt("orchestrator.tmpl"),
		"{{SKILLS_ROOT}}", skillsRoot,
		"{{FLEET_ENTRY}}", fleetEntry)
	return fill(text, "{{WORKFLOW}}", workflowSection)
}

// FleetPrompt renders the orchestrator that runs several changes at once, one worktree each.
// The workflow section is substituted last, as in OrchestratorPrompt.
func FleetPrompt(skillsRoot, workflowSection string) string {
	text := fill(prompt("fleet.tmpl"),
		"{{SKILLS_ROOT}}", skillsRoot,
		"{{TOOL}}", worktreeTool(skillsRoot))
	return fill(text, "{{WORKFLOW}}", workflowSection)
}

// CoordinatorPrompt renders the worktree coordinator, which starts one session per change
// (running sessionModel) and relays the workflow section to those sessions.
// The workflow section is substituted last, as in OrchestratorPrompt.
func CoordinatorPrompt(skillsRoot, sessionModel, workflowSection string) string {
	text := fill(prompt("coordinator.tmpl"),
		"{{SESSION_MODEL}}", sessionModel,
		"{{SKILLS_ROOT}}", skillsRoot,
		"{{TOOL}}", worktreeTool(skillsRoot))
	return fill(text, "{{WORKFLOW}}", workflowSection)
}

// SubagentPrompt renders the prompt of a phase executor that follows the given skill file.
func SubagentPrompt(phase, skillPath string) string {
	return fill(prompt("subagent.tmpl"), "{{PHASE}}", phase, "{{SKILL_PATH}}", skillPath)
}

// ManagePrompt renders the prompt of the alfred-manage subagent.
func ManagePrompt(skillPath string) string {
	return fill(prompt("manage.tmpl"), "{{SKILL_PATH}}", skillPath)
}
