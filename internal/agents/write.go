package agents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/markosAMO/alfred/internal/jsonobj"
)

// asBooleans renders a tool list as OpenCode's per-tool flags: every tool it knows about
// appears, enabled or disabled, so a tool is never inherited by omission.
func asBooleans(tools []string, allowTask bool) map[string]bool {
	flags := make(map[string]bool, len(opencodeNames))
	for _, name := range opencodeNames {
		flags[name] = false
	}
	for _, tool := range tools {
		if name, ok := opencodeNames[tool]; ok {
			flags[name] = true
		}
	}
	flags["task"] = allowTask
	return flags
}

type opencodePermission struct {
	Task map[string]string `json:"task"`
}

// opencodeAgent is one entry under `agent` in opencode.json.
type opencodeAgent struct {
	Model       string              `json:"model"`
	Mode        string              `json:"mode"`
	Hidden      bool                `json:"hidden,omitempty"`
	Description string              `json:"description"`
	Prompt      string              `json:"prompt"`
	Permission  *opencodePermission `json:"permission,omitempty"`
	Tools       map[string]bool     `json:"tools"`
}

// OpencodeConfig is the generated half of opencode.json.
type OpencodeConfig struct {
	Schema string                   `json:"$schema"`
	Agent  map[string]opencodeAgent `json:"agent"`
}

const opencodeSchema = "https://opencode.ai/config.json"

// Opencode builds the generated half of opencode.json.
func Opencode(p *Profile, skillsRoot string) (*OpencodeConfig, error) {
	phases := p.PhaseNames()
	agent := map[string]opencodeAgent{}

	// Two primary agents, the same pair Claude Code gets as two slash commands: one change
	// in this checkout, or several at once with a worktree each. Both appear in the picker,
	// so the choice is made by starting the run rather than by an argument to it.
	primary := func(description, promptText string) opencodeAgent {
		return opencodeAgent{
			Model:       p.Orchestrator(),
			Mode:        "primary",
			Description: description,
			Prompt:      promptText,
			Permission: &opencodePermission{
				Task: map[string]string{"*": "deny", "alfred-*": "allow"},
			},
			Tools: asBooleans(orchestratorTools, true),
		}
	}

	agent["alfred"] = primary(
		"Alfred orchestrator: plans, routes and delegates. Never writes code.",
		OrchestratorPrompt(skillsRoot, phases, "the `alfred-worktree` agent"))
	agent["alfred-worktree"] = primary(
		"Alfred orchestrator for several changes at once, one git worktree each.",
		FleetPrompt(skillsRoot, phases))

	subagent := func(model, description, promptText string, tools []string) opencodeAgent {
		return opencodeAgent{
			Model:       model,
			Mode:        "subagent",
			Hidden:      true,
			Description: description,
			Prompt:      promptText,
			Tools:       asBooleans(tools, false),
		}
	}

	for _, phase := range phases {
		if phase == "worktree" {
			return nil, errors.New("profile: 'worktree' is the fleet orchestrator, not a phase")
		}
		skillPath := fmt.Sprintf("%s/%s/SKILL.md", skillsRoot, phase)
		agent["alfred-"+phase] = subagent(
			p.PhaseModel(phase),
			"Alfred "+phase+" phase executor",
			SubagentPrompt(phase, skillPath),
			p.Tools(phase))
	}

	agent["alfred-manage"] = subagent(
		p.ManageModel(),
		"Alfred management: status, registry, doctor, reindex",
		ManagePrompt(skillsRoot+"/alfred/SKILL.md"),
		p.Tools("alfred"))

	return &OpencodeConfig{Schema: opencodeSchema, Agent: agent}, nil
}

// MergeOpencode replaces only the alfred-* agents, leaving any other agent and any other
// key untouched and where it was. An Alfred agent already in the file is replaced in place;
// a new one is appended, and one the profile no longer has is removed.
func MergeOpencode(target string, generated *OpencodeConfig) error {
	existing := jsonobj.New()
	if data, err := os.ReadFile(target); err == nil {
		if existing, err = jsonobj.Parse(data); err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
	}

	if _, ok := existing.Get("$schema"); !ok && existing.Len() == 0 {
		// A new file starts with the schema, as OpenCode writes its own.
		schema, err := jsonobj.Raw(generated.Schema)
		if err != nil {
			return err
		}
		existing.Set("$schema", schema)
	}

	agents := existing.Child("agent")
	for _, name := range agents.Keys() {
		_, kept := generated.Agent[name]
		if !kept && (name == "alfred" || strings.HasPrefix(name, "alfred-")) {
			agents.Delete(name)
		}
	}

	names := make([]string, 0, len(generated.Agent))
	for name := range generated.Agent {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err := jsonobj.Raw(generated.Agent[name])
		if err != nil {
			return err
		}
		agents.Set(name, raw)
	}
	existing.SetChild("agent", agents)

	if _, ok := existing.Get("$schema"); !ok {
		schema, err := jsonobj.Raw(generated.Schema)
		if err != nil {
			return err
		}
		existing.Set("$schema", schema)
	}

	data, err := jsonobj.Format(existing)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func effortLine(p *Profile, phase string) string {
	if effort := p.Effort(phase); effort != "" {
		return "effort: " + effort + "\n"
	}
	return ""
}

// ClaudeAgents writes one subagent per phase under agents/, plus the two orchestrators as
// slash commands, and returns how many files were written.
func ClaudeAgents(p *Profile, skillsRoot, claudeHome string) (int, error) {
	agentsDir := filepath.Join(claudeHome, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return 0, err
	}
	written := 0

	for _, phase := range p.PhaseNames() {
		skillPath := fmt.Sprintf("%s/%s/SKILL.md", skillsRoot, phase)
		body := "---\n" +
			"name: alfred-" + phase + "\n" +
			"description: Alfred " + phase + " phase executor\n" +
			"model: " + bareModel(p.PhaseModel(phase)) + "\n" +
			effortLine(p, phase) +
			"tools: " + strings.Join(p.Tools(phase), ", ") + "\n" +
			"---\n\n" +
			SubagentPrompt(phase, skillPath) + "\n"

		if err := os.WriteFile(filepath.Join(agentsDir, "alfred-"+phase+".md"), []byte(body), 0o644); err != nil {
			return 0, err
		}
		written++
	}

	manage := "---\n" +
		"name: alfred-manage\n" +
		"description: Alfred management - status, registry, doctor, reindex\n" +
		"model: " + bareModel(p.ManageModel()) + "\n" +
		"tools: " + strings.Join(p.Tools("alfred"), ", ") + "\n" +
		"---\n\n" +
		ManagePrompt(skillsRoot+"/alfred/SKILL.md") + "\n"

	if err := os.WriteFile(filepath.Join(agentsDir, "alfred-manage.md"), []byte(manage), 0o644); err != nil {
		return 0, err
	}
	written++

	commandsDir := filepath.Join(claudeHome, "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		return 0, err
	}

	if err := os.WriteFile(filepath.Join(commandsDir, "alfred.md"),
		[]byte(claudeCommand(p, skillsRoot, p.PhaseNames())), 0o644); err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(commandsDir, "alfred-worktree.md"),
		[]byte(claudeWorktreeCommand(p, skillsRoot)), 0o644); err != nil {
		return 0, err
	}

	return written + 2, nil
}

func claudeCommand(p *Profile, skillsRoot string, phases []string) string {
	return "---\n" +
		"description: Alfred orchestrator - plan, route and delegate a change\n" +
		"argument-hint: [what you want done, or: init | continue | status]\n" +
		"model: " + bareModel(p.Orchestrator()) + "\n" +
		"tools: " + strings.Join(orchestratorTools, ", ") + "\n" +
		"---\n\n" +
		OrchestratorPrompt(skillsRoot, phases, "`/alfred-worktree`") + "\n\n" +
		"## The request\n\n" +
		"$ARGUMENTS\n"
}

// claudeWorktreeCommand is the worktree coordinator as a slash command.
//
// A separate command rather than a mode of /alfred, for the reason FleetPrompt gives, and
// now for a second: this one starts sessions and routes between them, where /alfred runs a
// change in the session it was asked from.
func claudeWorktreeCommand(p *Profile, skillsRoot string) string {
	return "---\n" +
		"description: Alfred worktree coordinator - a session per change, one git worktree each\n" +
		"argument-hint: [branch [from base]: request, one per line, or: continue]\n" +
		"model: " + bareModel(p.Coordinator()) + "\n" +
		"tools: " + strings.Join(coordinatorTools, ", ") + "\n" +
		"---\n\n" +
		CoordinatorPrompt(skillsRoot, bareModel(p.Orchestrator())) + "\n\n" +
		"## The request\n\n" +
		"$ARGUMENTS\n"
}
