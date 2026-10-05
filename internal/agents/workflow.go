package agents

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/markosAMO/alfred/internal/workflow"
)

// The keys an entry point is generated under when its prompt differs per agent.
const (
	AgentClaude   = "claude"
	AgentOpencode = "opencode"
)

// Scope says which set a run generates. It is given, never derived from the roots: machine
// scope owns `/alfred` and the commands that belong to no workflow, and a repository that
// generated its own copies of those would carry one machine's model identifiers into
// somebody else's checkout.
type Scope string

const (
	Machine Scope = "machine"
	Project Scope = "project"
)

// Registered is one workflow that validated and whose phases all resolved. Dir is the
// directory it was read from, which is what makes the rules path in its command absolute.
type Registered struct {
	Dir        string
	Definition *workflow.Definition
	Phases     []workflow.Resolved
}

// Generation is everything one scope's generation is given.
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

	// Delegates is false for a command that is the work rather than a plan for it. It gets
	// no Task tool, so a command that is supposed to stop cannot reach a subagent at all.
	Delegates bool

	// Agents restricts the command to the agents named here; empty is every agent. Two
	// entry points need it, because their prompt differs rather than their name: a workflow
	// command addresses the fleet entry point the way that agent names it, and
	// `alfred-worktree` starts a background session per change on Claude Code and runs the
	// same fleet inline on OpenCode.
	Agents []string
}

// For says whether this command is generated for one agent.
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

// Unavailable is a phase that resolved and that nobody assigned a model to. It is a result
// to report rather than an error: the routes that never reach it work, and the ones that do
// stop there.
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

	// Legacy is what the generator before the mark produced in this scope. It travels with
	// the set because the writer is where a manifest-less scope is reconstructed and the
	// profile is the only thing that knows this installation's phases.
	Legacy Legacy
}

// Generate turns the registered workflows of one scope into the commands and subagents that
// scope writes.
//
// One command per workflow, which is the shared orchestrator prompt with that workflow's
// structural facts rendered into it, and one subagent per phase that has a model. Nothing
// here reads a definition from disk: the facts arrive already validated and resolved,
// because they are read once, at registration, and never again while a change runs.
func Generate(p *Profile, g Generation) (*Set, error) {
	if err := declaredOnce(g.Workflows); err != nil {
		return nil, err
	}

	set := &Set{}
	sections := make(map[string]string, len(g.Workflows))
	subagents := map[string]Subagent{}

	for _, registered := range g.Workflows {
		name := registered.Definition.Name
		missing := map[string]string{}

		for _, phase := range registered.Phases {
			if phase.Model == "" {
				reason := noModel(phase)
				missing[phase.Name] = reason
				set.Unavailable = append(set.Unavailable, Unavailable{Workflow: name, Phase: phase.Name, Reason: reason})
				continue
			}
			// A shared phase is one subagent however many workflows run it; an own phase is
			// addressed under a name that is its workflow's alone.
			agent := subagentFor(p, name, phase)
			subagents[agent.Name] = agent
		}

		sections[name] = workflowSection(registered, g.Workflows, missing)
		set.Commands = append(set.Commands,
			workflowCommand(p, g.Skills, "alfred-"+name, registered.Definition.Description, sections[name])...)
	}

	if g.Scope == Machine {
		set.Commands = append(set.Commands, machineCommands(p, g, sections)...)
		subagents["alfred-manage"] = manageSubagent(p, g.Skills)
	}

	for _, name := range sortedNames(subagents) {
		set.Subagents = append(set.Subagents, subagents[name])
	}
	sortCommands(set.Commands)
	set.Legacy = legacyNames(p, g.Scope)
	markPrompts(set)
	return set, nil
}

// declaredOnce is a guard against a caller that skipped registration, not the policy that
// decides what registers.
//
// A duplicated workflow name and a duplicated subagent name are both decided in
// internal/workflow, where every other per-workflow refusal is decided and where refusing a
// pair of definitions costs the rest of the scope nothing. By the time Generate runs they
// are impossible, so an error from here is one caller handing this package what the prepare
// path would have rejected — and the two maps below would otherwise silently take the second
// assignment, leaving one workflow's command dispatching a subagent carrying the other's
// prompt and model.
//
// It therefore stays cheap and total rather than being dropped: a guard that never fires
// costs one pass over the registered workflows, and the alternative is a corrupt set that
// nothing downstream can tell apart from a correct one.
func declaredOnce(registered []Registered) error {
	seen := make(map[string]bool, len(registered))
	by := map[string]string{}

	for _, one := range registered {
		if one.Definition == nil {
			return fmt.Errorf("the workflow at %s was registered with no definition", one.Dir)
		}
		name := one.Definition.Name
		if seen[name] {
			return fmt.Errorf("workflow %q is registered twice in one scope", name)
		}
		seen[name] = true

		for _, phase := range one.Phases {
			// A shared phase is deliberately one subagent however many workflows run it, so
			// only the names a workflow makes its own can collide.
			if phase.Origin == workflow.FromShared {
				continue
			}
			agent := workflow.SubagentName(name, phase)
			if other, taken := by[agent]; taken {
				return fmt.Errorf(
					"the subagent name %q is generated by both %q and %q; neither workflow is registered",
					agent, other, name)
			}
			by[agent] = name
		}
	}
	return nil
}

// sortCommands orders by name and then by agent, so two runs over the same roots write the
// same bytes in the same order.
func sortCommands(commands []Command) {
	sort.SliceStable(commands, func(i, j int) bool {
		if commands[i].Name != commands[j].Name {
			return commands[i].Name < commands[j].Name
		}
		return strings.Join(commands[i].Agents, ",") < strings.Join(commands[j].Agents, ",")
	})
}

// workflowCommand is one workflow's entry point, once per agent. A command is a whole
// prompt rather than a prompt with holes left in it, and the shared part names the fleet
// entry point the way the agent it is written for names it.
func workflowCommand(p *Profile, skillsRoot, name, description, section string) []Command {
	base := Command{
		Name:         name,
		Description:  description,
		ArgumentHint: "[what you want done, or: continue | status]",
		Model:        p.Orchestrator(),
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

// machineCommands are the ones a repository never generates: `/alfred`, the worktree entry
// point, and the four that belong to no workflow.
//
// The last four are generated whether or not any workflow registered. Setting a repository
// up, exploring it, creating a workflow and rescanning the roots are what a machine with no
// workflow needs most, and they are commands rather than phases precisely so that removing
// a workflow can never remove them.
func machineCommands(p *Profile, g Generation, sections map[string]string) []Command {
	wanted := p.DefaultWorkflow()
	section, registered := sections[wanted]

	var commands []Command
	if registered {
		commands = append(commands,
			workflowCommand(p, g.Skills, "alfred", wanted+": the default workflow", section)...)
		commands = append(commands,
			worktreeCommand(p, FleetPrompt(g.Skills, section)),
			coordinatorCommand(p, CoordinatorPrompt(g.Skills, bareModel(p.Orchestrator()), section)))
	} else {
		// Both halves of the fleet entry point stop, and for the same reason: with no
		// default workflow there are no structural facts to run on, and the coordinator
		// would otherwise start sessions that have none from any source.
		unavailable := stub(p, "alfred-worktree", "/alfred-worktree", wanted, g.Workflows)
		commands = append(commands, stub(p, "alfred", "/alfred", wanted, g.Workflows))
		commands = append(commands,
			stopped(worktreeCommand(p, unavailable.Prompt), unavailable),
			stopped(coordinatorCommand(p, unavailable.Prompt), unavailable))
	}

	commands = append(commands,
		standalone(p, "alfred-init", "init", g.Skills,
			"Alfred repository setup - the init phase itself",
			"set this repository up to run Alfred"),
		standalone(p, "alfred-explore", "explore", g.Skills,
			"Alfred exploration - derives the specification of an area",
			"derive the specification of the area of this repository the request names"),
		interviewCommand(p, g),
		scannerCommand(p, g.Skills))
	return commands
}

// worktreeCommand is the OpenCode half of the fleet entry point: several changes at once,
// run inline, under the default workflow.
func worktreeCommand(p *Profile, promptText string) Command {
	return Command{
		Name:         "alfred-worktree",
		Description:  "Alfred orchestrator for several changes at once, one git worktree each",
		ArgumentHint: "[branch [from base]: request, one per line, or: continue]",
		Model:        p.Orchestrator(),
		Tools:        orchestratorTools,
		Prompt:       promptText,
		Delegates:    true,
		Agents:       []string{AgentOpencode},
	}
}

// coordinatorCommand is the Claude Code half of the same entry point. It starts a
// background session per change, which is a capability only that agent has, so the two
// agents are given different prompts under one name rather than two names for one thing.
//
// It is given a prompt rather than the facts to build one, like the OpenCode half, because
// the workflow section it carries is the same rendering that half receives and is not
// derived a second time here.
func coordinatorCommand(p *Profile, promptText string) Command {
	return Command{
		Name:         "alfred-worktree",
		Description:  "Alfred worktree coordinator - a session per change, one git worktree each",
		ArgumentHint: "[branch [from base]: request, one per line, or: continue]",
		Model:        p.Coordinator(),
		Tools:        coordinatorTools,
		Prompt:       promptText,
		Delegates:    true,
		Agents:       []string{AgentClaude},
	}
}

// stub is `/alfred` on a machine where the configured default workflow did not register.
//
// A command that simply does not exist cannot report anything, and the user who asked for
// the default would get whatever their agent does with an unknown command. This one names
// the workflow, says it is not installed, lists the ones that are, and stops. It is given
// no Task tool, so it cannot run one of them in its place even by accident.
func stub(p *Profile, name, command, wanted string, available []Registered) Command {
	return Command{
		Name:         name,
		Description:  "Alfred - the configured default workflow " + wanted + " is not installed",
		ArgumentHint: "[what you want done]",
		Model:        p.Orchestrator(),
		Tools:        []string{"Read"},
		Prompt: fill(prompt("unavailable.tmpl"),
			"{{COMMAND}}", command,
			"{{WORKFLOW_NAME}}", wanted,
			"{{AVAILABLE}}", availableLines(available)),
	}
}

// stopped gives a command the stub's guarantee as well as its prompt.
//
// The prompt is an instruction and the tool set is what makes stopping the only thing left
// to do. A fleet entry point keeping its own tools holds Task, and on Claude Code also
// Bash, which is what starts the background sessions the prompt is telling itself not to
// start; the stub's own tools are what the comment above it claims for every use of it.
func stopped(c, s Command) Command {
	c.Tools = s.Tools
	c.Delegates = false
	return c
}

// standalone is a command that belongs to no workflow and is the phase itself. There is no
// orchestrator in front of it, so there is no clean context for a subagent to protect.
func standalone(p *Profile, name, phase, skillsRoot, description, task string) Command {
	return Command{
		Name:         name,
		Description:  description,
		ArgumentHint: "[what you want done]",
		Model:        p.PhaseModel(phase),
		Tools:        p.Tools(phase),
		Prompt: fill(prompt("standalone.tmpl"),
			"{{COMMAND}}", "/"+name,
			"{{SKILL_PATH}}", skillPath(skillsRoot, phase),
			"{{TASK}}", task),
	}
}

func interviewCommand(p *Profile, g Generation) Command {
	return Command{
		Name:         "alfred-add-workflow",
		Description:  "Alfred - the interview that creates a workflow",
		ArgumentHint: "[what the workflow is for]",
		Model:        p.Orchestrator(),
		Tools:        orchestratorTools,
		Delegates:    true,
		Prompt: fill(prompt("add-workflow.tmpl"),
			"{{SHARED_PHASES}}", sharedPhases(),
			"{{TEMPLATES_ROOT}}", g.Templates,
			"{{CUSTOM_ROOT}}", g.Custom),
	}
}

// scannerCommand is the rescan, rendered onto alfred-manage: it runs the registration the
// alfred skill describes, and that needs a shell rather than a model that plans.
func scannerCommand(p *Profile, skillsRoot string) Command {
	return Command{
		Name:         "alfred-workflows-scanner",
		Description:  "Alfred - rescan the workflow roots and bring the commands up to date",
		ArgumentHint: "[machine | project, and: apply | report | check]",
		Model:        p.ManageModel(),
		Tools:        p.Tools("alfred"),
		Prompt: fill(prompt("standalone.tmpl"),
			"{{COMMAND}}", "/alfred-workflows-scanner",
			"{{SKILL_PATH}}", skillPath(skillsRoot, "alfred"),
			"{{TASK}}", "run the workflows operation of that skill and report what it prints"),
	}
}

func manageSubagent(p *Profile, skillsRoot string) Subagent {
	return Subagent{
		Name:        "alfred-manage",
		Description: "Alfred management: status, registry, doctor, reindex",
		Model:       p.ManageModel(),
		Tools:       p.Tools("alfred"),
		Prompt:      ManagePrompt(skillPath(skillsRoot, "alfred")),
	}
}

// subagentFor is one phase's executor. A shared phase is addressed by its own name and
// takes everything the installation assigned it; a phase the workflow or the repository
// brought is addressed under its workflow's name, so it never collides with the shared
// phase it shadows, and takes nothing from the profile's per-phase entries.
//
// That the name is then the workflow's alone is declaredOnce's guarantee and not this
// function's: the form it builds is not injective, and two workflows can reach it.
func subagentFor(p *Profile, name string, phase workflow.Resolved) Subagent {
	if phase.Origin == workflow.FromShared {
		return Subagent{
			Name:        "alfred-" + phase.Name,
			Description: "Alfred " + phase.Name + " phase executor",
			Model:       phase.Model,
			Effort:      p.Effort(phase.Name),
			Tools:       p.Tools(phase.Name),
			Prompt:      SubagentPrompt(phase.Name, phase.Skill),
		}
	}
	return Subagent{
		Name:        workflow.SubagentName(name, phase),
		Description: "Alfred " + phase.Name + " phase executor for the " + name + " workflow",
		Model:       phase.Model,
		Tools:       p.OwnTools(phase.Tools),
		Prompt:      SubagentPrompt(phase.Name, phase.Skill),
	}
}

// noModel is the sentence both the report and the command print. There is one wording, so a
// phase reads the same way wherever the user meets it.
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

// workflowSection renders one workflow's structural facts into the section every command
// carries.
//
// The free text a user wrote - the title, the description, and the description of every
// other workflow - is substituted after the structural values, for the reason the whole
// section is substituted into the shared prompt last: fill is ReplaceAll applied in order.
func workflowSection(r Registered, all []Registered, missing map[string]string) string {
	d := r.Definition
	return fill(prompt("workflow.tmpl"),
		"{{WORKFLOW_NAME}}", d.Name,
		"{{ROUTES}}", routeLines(d),
		"{{DEFAULT_ROUTE}}", d.DefaultRoute,
		"{{ENTRY_POINTS}}", entryLines(d),
		"{{PARALLEL_GROUPS}}", groupLines(d),
		"{{CLOSING_PHASE}}", d.Closes,
		"{{PHASE_SUBAGENTS}}", phaseLines(d, r.Phases, missing),
		"{{RULES_PATH}}", filepath.Join(r.Dir, d.Rules),
		"{{OTHER_WORKFLOWS}}", otherLines(d.Name, all),
		"{{WORKFLOW_TITLE}}", d.Title+"\n\n"+d.Description)
}

func routeLines(d *workflow.Definition) string {
	names := make([]string, 0, len(d.Routes))
	for name := range d.Routes {
		names = append(names, name)
	}
	sort.Strings(names)

	lines := make([]string, 0, len(names))
	for _, name := range names {
		lines = append(lines, "  "+pad(name, width(names))+"  "+strings.Join(d.Routes[name], " -> "))
	}
	return strings.Join(lines, "\n")
}

func entryLines(d *workflow.Definition) string {
	labels := make([]string, 0, len(d.EntryPoints))
	for label := range d.EntryPoints {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	lines := make([]string, 0, len(labels))
	for _, label := range labels {
		lines = append(lines, "  `"+label+"`"+strings.Repeat(" ", width(labels)-len(label))+"  enters at `"+d.EntryPoints[label]+"`")
	}
	return strings.Join(lines, "\n")
}

// groupLines renders the empty case as a sentence rather than as nothing: a template cannot
// branch, and a blank line under a heading reads as a fact nobody wrote down.
func groupLines(d *workflow.Definition) string {
	if len(d.Parallel) == 0 {
		return "This workflow declares no group dispatched together."
	}
	lines := make([]string, 0, len(d.Parallel))
	for _, group := range d.Parallel {
		quoted := make([]string, len(group))
		for i, phase := range group {
			quoted[i] = "`" + phase + "`"
		}
		lines = append(lines, "  "+strings.Join(quoted[:len(quoted)-1], ", ")+" and "+quoted[len(quoted)-1])
	}
	return strings.Join(lines, "\n")
}

func phaseLines(d *workflow.Definition, resolved []workflow.Resolved, missing map[string]string) string {
	names := make([]string, len(d.Phases))
	for i, phase := range d.Phases {
		names[i] = phase.Name
	}

	lines := make([]string, 0, len(resolved))
	for _, phase := range resolved {
		line := "  " + pad(phase.Name, width(names)) + "  "
		if reason, absent := missing[phase.Name]; absent {
			lines = append(lines, line+"unavailable - "+reason)
			continue
		}
		lines = append(lines, line+workflow.SubagentName(d.Name, phase)+"  ("+origin(phase.Origin)+")")
	}
	return strings.Join(lines, "\n")
}

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

func otherLines(name string, all []Registered) string {
	lines := make([]string, 0, len(all))
	for _, one := range all {
		if one.Definition.Name == name {
			continue
		}
		lines = append(lines, "  `"+one.Definition.Name+"`  "+one.Definition.Description)
	}
	if len(lines) == 0 {
		return "This is the only workflow registered here."
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func availableLines(available []Registered) string {
	lines := make([]string, 0, len(available))
	for _, one := range available {
		lines = append(lines, "  `/alfred-"+one.Definition.Name+"`  "+one.Definition.Description)
	}
	if len(lines) == 0 {
		return "  none: no workflow registered on this machine"
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// sharedPhases is what the interview offers as a phase a workflow may reuse. `init` and
// `explore` are shared skills and are not on it: they are commands of their own, and a
// definition that declares either as a phase is refused.
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

func skillPath(skillsRoot, phase string) string {
	return fmt.Sprintf("%s/%s/SKILL.md", skillsRoot, phase)
}

func sortedNames(agents map[string]Subagent) []string {
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func width(names []string) int {
	longest := 0
	for _, name := range names {
		if len(name) > longest {
			longest = len(name)
		}
	}
	return longest
}

func pad(name string, to int) string {
	return name + strings.Repeat(" ", to-len(name))
}
