package agents

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/markosAMO/alfred/internal/workflow"
)

// Agent keys used to restrict a command to one agent and to tag generated output.
const (
	AgentClaude   = "claude"
	AgentOpencode = "opencode"
)

// Scope says which set a run generates: machine scope also owns `/alfred` and the
// workflow-independent commands. It is passed in, never derived, so a repository never gets
// copies carrying one machine's model identifiers.
type Scope string

// The two scopes a registration run can target.
const (
	Machine Scope = "machine"
	Project Scope = "project"
)

// Registered is a validated workflow with its resolved phases. Dir is the directory it was
// read from, used to make its rules path absolute.
type Registered struct {
	Dir        string
	Definition *workflow.Definition
	Phases     []workflow.Resolved
}

// Generation is the input to Generate for one scope: the scope, the roots it renders paths
// from, and its registered workflows.
type Generation struct {
	Scope     Scope
	Skills    string // the shared skill library
	Templates string // templates/workflow/, which the interview writes a new workflow from
	Custom    string // the root the interview writes a new machine-scope workflow under
	Workflows []Registered
}

// Command is an entry point the user starts a run from: a slash command on Claude Code, a
// primary agent on OpenCode.
type Command struct {
	Name         string
	Description  string
	ArgumentHint string
	Model        string
	Tools        []string
	Prompt       string

	// Delegates is false for a command that does the work itself; such a command gets no
	// Task tool, so it cannot reach a subagent.
	Delegates bool

	// Agents restricts the command to the named agents; empty means every agent. Used when
	// the same command name needs a different prompt per agent.
	Agents []string
}

// For reports whether this command is generated for the given agent.
func (c Command) For(agent string) bool {
	if len(c.Agents) == 0 {
		return true
	}
	for _, name := range c.Agents {
		if name == agent {
			return true
		}
	}
	return false
}

// Subagent is one phase executor, started by a command and never by the user.
type Subagent struct {
	Name        string
	Description string
	Model       string
	Effort      string
	Tools       []string
	Prompt      string
}

// Unavailable is a resolved phase with no model assigned. It is reported, not an error:
// routes that never reach it still work.
type Unavailable struct {
	Workflow string
	Phase    string
	Reason   string
}

// Set is everything one scope's registration generates.
type Set struct {
	Commands    []Command
	Subagents   []Subagent
	Unavailable []Unavailable

	// Legacy names what the pre-mark generator produced in this scope, so the writer can
	// adopt those files on a scope with no manifest.
	Legacy Legacy
}

// Generate turns one scope's registered workflows into its commands and subagents: one
// command per workflow per agent, and one subagent per phase that has a model.
// Definitions are not read from disk here; they arrive already validated and resolved.
func Generate(profile *Profile, generation Generation) (*Set, error) {
	if err := declaredOnce(generation.Workflows); err != nil {
		return nil, err
	}

	set := &Set{}
	sections := make(map[string]string, len(generation.Workflows))
	subagents := map[string]Subagent{}

	for _, registered := range generation.Workflows {
		name := registered.Definition.Name
		missing := map[string]string{}

		for _, phase := range registered.Phases {
			if phase.Model == "" {
				reason := noModel(phase)
				missing[phase.Name] = reason
				set.Unavailable = append(set.Unavailable, Unavailable{Workflow: name, Phase: phase.Name, Reason: reason})
				continue
			}
			// Keyed by subagent name: a shared phase used by several workflows is one subagent.
			agent := subagentFor(profile, name, phase)
			subagents[agent.Name] = agent
		}

		sections[name] = workflowSection(registered, generation.Workflows, missing)
		set.Commands = append(set.Commands,
			workflowCommand(profile, generation.Skills, "alfred-"+name, registered.Definition.Description, sections[name])...)
	}

	if generation.Scope == Machine {
		set.Commands = append(set.Commands, machineCommands(profile, generation, sections)...)
		subagents["alfred-manage"] = manageSubagent(profile, generation.Skills)
	}

	for _, name := range sortedNames(subagents) {
		set.Subagents = append(set.Subagents, subagents[name])
	}
	sortCommands(set.Commands)
	set.Legacy = legacyNames(profile, generation.Scope)
	markPrompts(set)
	return set, nil
}

// declaredOnce rejects duplicate workflow names and duplicate own-phase subagent names.
// Registration already refuses these; keep this guard anyway, since a duplicate would
// otherwise silently overwrite one workflow's subagent with another's in Generate.
func declaredOnce(registered []Registered) error {
	seen := make(map[string]bool, len(registered))
	workflowBySubagent := map[string]string{}

	for _, entry := range registered {
		if entry.Definition == nil {
			return fmt.Errorf("the workflow at %s was registered with no definition", entry.Dir)
		}
		name := entry.Definition.Name
		if seen[name] {
			return fmt.Errorf("workflow %q is registered twice in one scope", name)
		}
		seen[name] = true

		for _, phase := range entry.Phases {
			// Shared phases are meant to be one subagent across workflows; only own phases collide.
			if phase.Origin == workflow.FromShared {
				continue
			}
			agent := workflow.SubagentName(name, phase)
			if other, taken := workflowBySubagent[agent]; taken {
				return fmt.Errorf(
					"the subagent name %q is generated by both %q and %q; neither workflow is registered",
					agent, other, name)
			}
			workflowBySubagent[agent] = name
		}
	}
	return nil
}

// sortCommands orders commands by name, then by agent list, so output is deterministic.
func sortCommands(commands []Command) {
	sort.SliceStable(commands, func(i, j int) bool {
		if commands[i].Name != commands[j].Name {
			return commands[i].Name < commands[j].Name
		}
		return strings.Join(commands[i].Agents, ",") < strings.Join(commands[j].Agents, ",")
	})
}

// workflowCommand builds one workflow's entry point once per agent; the two differ only in
// how the prompt names the fleet entry point.
func workflowCommand(profile *Profile, skillsRoot, name, description, section string) []Command {
	base := Command{
		Name:         name,
		Description:  description,
		ArgumentHint: "[what you want done, or: continue | status]",
		Model:        profile.Orchestrator(),
		Tools:        orchestratorTools,
		Delegates:    true,
	}

	claude, opencode := base, base
	claude.Agents = []string{AgentClaude}
	claude.Prompt = OrchestratorPrompt(skillsRoot, section, "`/alfred-worktree`")
	opencode.Agents = []string{AgentOpencode}
	opencode.Prompt = OrchestratorPrompt(skillsRoot, section, "the `alfred-worktree` agent")
	return []Command{claude, opencode}
}

// machineCommands builds the commands only machine scope generates: `/alfred`, the worktree
// entry point, and four workflow-independent commands that exist even with no workflow.
func machineCommands(profile *Profile, generation Generation, sections map[string]string) []Command {
	wanted := profile.DefaultWorkflow()
	section, registered := sections[wanted]

	var commands []Command
	if registered {
		commands = append(commands,
			workflowCommand(profile, generation.Skills, "alfred", wanted+": the default workflow", section)...)
		commands = append(commands,
			worktreeCommand(profile, FleetPrompt(generation.Skills, section)),
			coordinatorCommand(profile, CoordinatorPrompt(generation.Skills, bareModel(profile.Orchestrator()), section)))
	} else {
		// No default workflow: both halves of the worktree entry point become stubs that stop.
		unavailable := stub(profile, "alfred-worktree", "/alfred-worktree", wanted, generation.Workflows)
		commands = append(commands, stub(profile, "alfred", "/alfred", wanted, generation.Workflows))
		commands = append(commands,
			stopped(worktreeCommand(profile, unavailable.Prompt), unavailable),
			stopped(coordinatorCommand(profile, unavailable.Prompt), unavailable))
	}

	commands = append(commands,
		standalone(profile, "alfred-init", "init", generation.Skills,
			"Alfred repository setup - the init phase itself",
			"set this repository up to run Alfred"),
		standalone(profile, "alfred-explore", "explore", generation.Skills,
			"Alfred exploration - derives the specification of an area",
			"derive the specification of the area of this repository the request names"),
		interviewCommand(profile, generation),
		scannerCommand(profile, generation.Skills))
	return commands
}

// worktreeCommand is the OpenCode version of `alfred-worktree`: it runs several changes
// inline under the default workflow.
func worktreeCommand(profile *Profile, promptText string) Command {
	return Command{
		Name:         "alfred-worktree",
		Description:  "Alfred orchestrator for several changes at once, one git worktree each",
		ArgumentHint: "[branch [from base]: request, one per line, or: continue]",
		Model:        profile.Orchestrator(),
		Tools:        orchestratorTools,
		Prompt:       promptText,
		Delegates:    true,
		Agents:       []string{AgentOpencode},
	}
}

// coordinatorCommand is the Claude Code version of `alfred-worktree`: a coordinator that
// starts one background session per change.
func coordinatorCommand(profile *Profile, promptText string) Command {
	return Command{
		Name:         "alfred-worktree",
		Description:  "Alfred worktree coordinator - a session per change, one git worktree each",
		ArgumentHint: "[branch [from base]: request, one per line, or: continue]",
		Model:        profile.Coordinator(),
		Tools:        coordinatorTools,
		Prompt:       promptText,
		Delegates:    true,
		Agents:       []string{AgentClaude},
	}
}

// stub builds a command that says the default workflow is not installed, lists the
// installed ones, and stops. It gets only Read, so it cannot run another workflow instead.
func stub(profile *Profile, name, command, wanted string, available []Registered) Command {
	return Command{
		Name:         name,
		Description:  "Alfred - the configured default workflow " + wanted + " is not installed",
		ArgumentHint: "[what you want done]",
		Model:        profile.Orchestrator(),
		Tools:        []string{"Read"},
		Prompt: fill(prompt("unavailable.tmpl"),
			"{{COMMAND}}", command,
			"{{WORKFLOW_NAME}}", wanted,
			"{{AVAILABLE}}", availableLines(available)),
	}
}

// stopped gives a command the stub's tools and disables delegation, so it cannot start
// subagents or sessions; the prompt alone would not enforce that.
func stopped(command, stubCommand Command) Command {
	command.Tools = stubCommand.Tools
	command.Delegates = false
	return command
}

// standalone builds a workflow-independent command that runs a phase's skill directly,
// with no orchestrator or subagent in between.
func standalone(profile *Profile, name, phase, skillsRoot, description, task string) Command {
	return Command{
		Name:         name,
		Description:  description,
		ArgumentHint: "[what you want done]",
		Model:        profile.PhaseModel(phase),
		Tools:        profile.Tools(phase),
		Prompt: fill(prompt("standalone.tmpl"),
			"{{COMMAND}}", "/"+name,
			"{{SKILL_PATH}}", skillPath(skillsRoot, phase),
			"{{TASK}}", task),
	}
}

// interviewCommand builds `alfred-add-workflow`, the interview that creates a new workflow.
func interviewCommand(profile *Profile, generation Generation) Command {
	return Command{
		Name:         "alfred-add-workflow",
		Description:  "Alfred - the interview that creates a workflow",
		ArgumentHint: "[what the workflow is for]",
		Model:        profile.Orchestrator(),
		Tools:        orchestratorTools,
		Delegates:    true,
		Prompt: fill(prompt("add-workflow.tmpl"),
			"{{SHARED_PHASES}}", sharedPhases(),
			"{{TEMPLATES_ROOT}}", generation.Templates,
			"{{CUSTOM_ROOT}}", generation.Custom),
	}
}

// scannerCommand builds `alfred-workflows-scanner`, which reruns registration through the
// alfred skill using the manage model and tools.
func scannerCommand(profile *Profile, skillsRoot string) Command {
	return Command{
		Name:         "alfred-workflows-scanner",
		Description:  "Alfred - rescan the workflow roots and bring the commands up to date",
		ArgumentHint: "[machine | project, and: apply | report | check]",
		Model:        profile.ManageModel(),
		Tools:        profile.Tools("alfred"),
		Prompt: fill(prompt("standalone.tmpl"),
			"{{COMMAND}}", "/alfred-workflows-scanner",
			"{{SKILL_PATH}}", skillPath(skillsRoot, "alfred"),
			"{{TASK}}", "run the workflows operation of that skill and report what it prints"),
	}
}

// manageSubagent builds the alfred-manage subagent used for bookkeeping operations.
func manageSubagent(profile *Profile, skillsRoot string) Subagent {
	return Subagent{
		Name:        "alfred-manage",
		Description: "Alfred management: status, registry, doctor, reindex",
		Model:       profile.ManageModel(),
		Tools:       profile.Tools("alfred"),
		Prompt:      ManagePrompt(skillPath(skillsRoot, "alfred")),
	}
}

// subagentFor builds one phase's executor. A shared phase is named alfred-<phase> and uses
// the profile's settings; an own phase is named under its workflow and ignores them.
// Uniqueness of own-phase names is enforced by declaredOnce, not here.
func subagentFor(profile *Profile, name string, phase workflow.Resolved) Subagent {
	if phase.Origin == workflow.FromShared {
		return Subagent{
			Name:        "alfred-" + phase.Name,
			Description: "Alfred " + phase.Name + " phase executor",
			Model:       phase.Model,
			Effort:      profile.Effort(phase.Name),
			Tools:       profile.Tools(phase.Name),
			Prompt:      SubagentPrompt(phase.Name, phase.Skill),
		}
	}
	return Subagent{
		Name:        workflow.SubagentName(name, phase),
		Description: "Alfred " + phase.Name + " phase executor for the " + name + " workflow",
		Model:       phase.Model,
		Tools:       profile.OwnTools(phase.Tools),
		Prompt:      SubagentPrompt(phase.Name, phase.Skill),
	}
}

// noModel returns the sentence explaining why a phase has no model, shared by the report
// and the generated command so the wording is identical in both.
func noModel(phase workflow.Resolved) string {
	switch phase.Origin {
	case workflow.FromWorkflow:
		return fmt.Sprintf("the workflow's own %s declares no model and will not be dispatched", phase.Name)
	case workflow.FromRepository:
		return fmt.Sprintf("the repository's own %s declares no model and will not be dispatched", phase.Name)
	default:
		return fmt.Sprintf("the shared phase %s has no model in this installation's profile and will not be dispatched", phase.Name)
	}
}

// workflowSection renders one workflow's structural facts into the section each command
// carries. User-written free text (other workflows, title, description) is substituted
// last so a `{{` inside it is never rewritten by a later placeholder.
func workflowSection(registered Registered, all []Registered, missing map[string]string) string {
	definition := registered.Definition
	return fill(prompt("workflow.tmpl"),
		"{{WORKFLOW_NAME}}", definition.Name,
		"{{ROUTES}}", routeLines(definition),
		"{{DEFAULT_ROUTE}}", definition.DefaultRoute,
		"{{ENTRY_POINTS}}", entryLines(definition),
		"{{PARALLEL_GROUPS}}", groupLines(definition),
		"{{CLOSING_PHASE}}", definition.Closes,
		"{{PHASE_SUBAGENTS}}", phaseLines(definition, registered.Phases, missing),
		"{{PHASE_READS}}", readLines(definition),
		"{{RULES_PATH}}", filepath.Join(registered.Dir, definition.Rules),
		"{{OTHER_WORKFLOWS}}", otherLines(definition.Name, all),
		"{{WORKFLOW_TITLE}}", definition.Title+"\n\n"+definition.Description)
}

// routeLines renders one aligned line per route, sorted by route name.
func routeLines(definition *workflow.Definition) string {
	names := make([]string, 0, len(definition.Routes))
	for name := range definition.Routes {
		names = append(names, name)
	}
	sort.Strings(names)

	lines := make([]string, 0, len(names))
	for _, name := range names {
		lines = append(lines, "  "+pad(name, width(names))+"  "+strings.Join(definition.Routes[name], " -> "))
	}
	return strings.Join(lines, "\n")
}

// entryLines renders one aligned line per entry point, sorted by label.
func entryLines(definition *workflow.Definition) string {
	labels := make([]string, 0, len(definition.EntryPoints))
	for label := range definition.EntryPoints {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	lines := make([]string, 0, len(labels))
	for _, label := range labels {
		lines = append(lines, "  `"+label+"`"+strings.Repeat(" ", width(labels)-len(label))+"  enters at `"+definition.EntryPoints[label]+"`")
	}
	return strings.Join(lines, "\n")
}

// groupLines renders one line per group of phases dispatched together. The empty case is
// a sentence, since the template cannot branch and a blank line would read as missing data.
func groupLines(definition *workflow.Definition) string {
	if len(definition.Parallel) == 0 {
		return "This workflow declares no group dispatched together."
	}
	lines := make([]string, 0, len(definition.Parallel))
	for _, group := range definition.Parallel {
		quoted := make([]string, len(group))
		for i, phase := range group {
			quoted[i] = "`" + phase + "`"
		}
		lines = append(lines, "  "+strings.Join(quoted[:len(quoted)-1], ", ")+" and "+quoted[len(quoted)-1])
	}
	return strings.Join(lines, "\n")
}

// phaseLines renders one aligned line per resolved phase: its subagent and origin, or the
// reason it is unavailable.
func phaseLines(definition *workflow.Definition, resolved []workflow.Resolved, missing map[string]string) string {
	names := make([]string, len(definition.Phases))
	for i, phase := range definition.Phases {
		names[i] = phase.Name
	}

	lines := make([]string, 0, len(resolved))
	for _, phase := range resolved {
		line := "  " + pad(phase.Name, width(names)) + "  "
		if reason, absent := missing[phase.Name]; absent {
			lines = append(lines, line+"unavailable - "+reason)
			continue
		}
		lines = append(lines, line+workflow.SubagentName(definition.Name, phase)+"  ("+origin(phase.Origin)+")")
	}
	return strings.Join(lines, "\n")
}

// readLines renders one line per phase, in declaration order: the artifacts it reads and
// the memory types it recalls.
func readLines(definition *workflow.Definition) string {
	names := make([]string, len(definition.Phases))
	for i, phase := range definition.Phases {
		names[i] = phase.Name
	}

	lines := make([]string, 0, len(definition.Phases))
	for _, phase := range definition.Phases {
		reads := "nothing"
		if len(phase.Reads) > 0 {
			reads = strings.Join(phase.Reads, ", ")
		}
		line := "  " + pad(phase.Name, width(names)) + "  reads " + reads
		if len(phase.Recall) > 0 {
			line += "; recalls " + strings.Join(phase.Recall, ", ")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// origin returns the human label for where a phase came from.
func origin(from workflow.Origin) string {
	switch from {
	case workflow.FromWorkflow:
		return "the workflow's own"
	case workflow.FromRepository:
		return "the repository's own"
	default:
		return "shared"
	}
}

// otherLines lists every registered workflow except the named one, sorted.
func otherLines(name string, all []Registered) string {
	lines := make([]string, 0, len(all))
	for _, other := range all {
		if other.Definition.Name == name {
			continue
		}
		lines = append(lines, "  `"+other.Definition.Name+"`  "+other.Definition.Description)
	}
	if len(lines) == 0 {
		return "This is the only workflow registered here."
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// availableLines lists the commands of every registered workflow, sorted, for the stub.
func availableLines(available []Registered) string {
	lines := make([]string, 0, len(available))
	for _, registered := range available {
		lines = append(lines, "  `/alfred-"+registered.Definition.Name+"`  "+registered.Definition.Description)
	}
	if len(lines) == 0 {
		return "  none: no workflow registered on this machine"
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// sharedPhases lists the shared phases a new workflow may reuse, excluding `init` and
// `explore`, which are standalone commands and are refused as phases.
func sharedPhases() string {
	names := make([]string, 0, len(workflow.SharedPhases))
	for _, name := range workflow.SharedPhases {
		if name == "init" || name == "explore" {
			continue
		}
		names = append(names, "`"+name+"`")
	}
	return strings.Join(names, ", ")
}

// skillPath returns the SKILL.md path of a phase under the skills root.
func skillPath(skillsRoot, phase string) string {
	return fmt.Sprintf("%s/%s/SKILL.md", skillsRoot, phase)
}

// sortedNames returns the keys of the subagent map, sorted.
func sortedNames(subagents map[string]Subagent) []string {
	names := make([]string, 0, len(subagents))
	for name := range subagents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// width returns the length of the longest name, for column alignment.
func width(names []string) int {
	longest := 0
	for _, name := range names {
		if len(name) > longest {
			longest = len(name)
		}
	}
	return longest
}

// pad right-pads name with spaces to the given width.
func pad(name string, to int) string {
	return name + strings.Repeat(" ", to-len(name))
}
