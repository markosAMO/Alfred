package agents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/markosAMO/alfred/internal/pyjson"
)

// asBooleans renders a tool list as OpenCode's per-tool flags: every tool it knows about
// appears, enabled or disabled, so a tool is never inherited by omission.
func asBooleans(tools []string, allowTask bool) *pyjson.Value {
	entry := pyjson.NewObject()
	enabled := map[string]bool{}

	for _, tool := range tools {
		if name, ok := opencodeNames[tool]; ok {
			entry.Set(name, pyjson.NewBool(true))
			enabled[name] = true
		}
	}

	// Sorted: map iteration order varies per process, and an unsorted difference rewrites
	// every agent's tool block on each run for no change at all.
	rest := make([]string, 0, len(opencodeNames))
	for _, name := range opencodeNames {
		if !enabled[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	for _, name := range dedupe(rest) {
		entry.Set(name, pyjson.NewBool(false))
	}

	entry.Set("task", pyjson.NewBool(allowTask))
	return entry
}

func dedupe(values []string) []string {
	out := values[:0]
	for i, v := range values {
		if i == 0 || v != values[i-1] {
			out = append(out, v)
		}
	}
	return out
}

func taskPermission() *pyjson.Value {
	task := pyjson.NewObject()
	task.Set("*", pyjson.NewString("deny"))
	task.Set("alfred-*", pyjson.NewString("allow"))

	permission := pyjson.NewObject()
	permission.Set("task", task)
	return permission
}

// OpencodeConfig builds the generated half of opencode.json.
func OpencodeConfig(p *Profile, skillsRoot string) (*pyjson.Value, error) {
	sorted := p.SortedPhaseNames()
	agent := pyjson.NewObject()

	// Two primary agents, the same pair Claude Code gets as two slash commands: one change
	// in this checkout, or several at once with a worktree each. Both appear in the picker,
	// so the choice is made by starting the run rather than by an argument to it.
	primary := func(description, promptText string) *pyjson.Value {
		entry := pyjson.NewObject()
		entry.Set("model", pyjson.NewString(p.Orchestrator()))
		entry.Set("mode", pyjson.NewString("primary"))
		entry.Set("description", pyjson.NewString(description))
		entry.Set("prompt", pyjson.NewString(promptText))
		entry.Set("permission", taskPermission())
		entry.Set("tools", asBooleans(orchestratorTools, true))
		return entry
	}

	agent.Set("alfred", primary(
		"Alfred orchestrator: plans, routes and delegates. Never writes code.",
		OrchestratorPrompt(skillsRoot, sorted, "the `alfred-worktree` agent")))
	agent.Set("alfred-worktree", primary(
		"Alfred orchestrator for several changes at once, one git worktree each.",
		FleetPrompt(skillsRoot, sorted)))

	subagent := func(model, description, promptText string, tools []string) *pyjson.Value {
		entry := pyjson.NewObject()
		entry.Set("model", pyjson.NewString(model))
		entry.Set("mode", pyjson.NewString("subagent"))
		entry.Set("hidden", pyjson.NewBool(true))
		entry.Set("description", pyjson.NewString(description))
		entry.Set("prompt", pyjson.NewString(promptText))
		entry.Set("tools", asBooleans(tools, false))
		return entry
	}

	for _, phase := range p.PhaseNames() {
		if phase == "worktree" {
			return nil, errors.New("profile: 'worktree' is the fleet orchestrator, not a phase")
		}
		skillPath := fmt.Sprintf("%s/%s/SKILL.md", skillsRoot, phase)
		agent.Set("alfred-"+phase, subagent(
			p.PhaseModel(phase),
			"Alfred "+phase+" phase executor",
			SubagentPrompt(phase, skillPath),
			p.Tools(phase)))
	}

	agent.Set("alfred-manage", subagent(
		p.ManageModel(),
		"Alfred management: status, registry, doctor, reindex",
		ManagePrompt(skillsRoot+"/alfred/SKILL.md"),
		p.Tools("alfred")))

	config := pyjson.NewObject()
	config.Set("$schema", pyjson.NewString("https://opencode.ai/config.json"))
	config.Set("agent", agent)
	return config, nil
}

// MergeOpencode replaces only the alfred-* agents, leaving any other agent untouched.
func MergeOpencode(target string, generated *pyjson.Value) error {
	existing := pyjson.NewObject()
	if data, err := os.ReadFile(target); err == nil {
		existing, err = pyjson.Decode(data)
		if err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
	}

	agents := pyjson.NewObject()
	for _, m := range existing.Get("agent").Members() {
		if m.Key != "alfred" && !strings.HasPrefix(m.Key, "alfred-") {
			agents.Set(m.Key, m.Val)
		}
	}
	for _, m := range generated.Get("agent").Members() {
		agents.Set(m.Key, m.Val)
	}

	existing.Set("agent", agents)
	existing.SetDefault("$schema", generated.Get("$schema"))

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(pyjson.Encode(existing, 2)+"\n"), 0o644)
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

	sorted := p.SortedPhaseNames()
	if err := os.WriteFile(filepath.Join(commandsDir, "alfred.md"),
		[]byte(claudeCommand(p, skillsRoot, sorted)), 0o644); err != nil {
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
