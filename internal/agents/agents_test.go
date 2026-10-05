package agents

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/markosAMO/alfred/internal/workflow"
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

// `worktree` is the fleet orchestrator, not a phase. The refusal now happens where a
// definition is validated, in internal/workflow; what this package still owes is that no
// generated subagent ever takes the name, whatever a profile happens to carry.
func TestWorktreeIsRejectedAsAPhase(t *testing.T) {
	p := profileFrom(t, `{"orchestrator": "m", "coordinator": "m", "phases": {"worktree": "m", "refine": "m", "review": "m", "archive": "m"}}`)

	set := generate(t, p, machineGeneration(sddWorkflow(), ventasWorkflow()))

	if has(subagentNames(set), "alfred-worktree") {
		t.Error("'worktree' is the fleet orchestrator and must never be a phase executor")
	}
	for _, agent := range set.Subagents {
		if strings.HasSuffix(agent.Name, "-worktree") {
			t.Errorf("%s names a phase that cannot exist", agent.Name)
		}
	}
}

func TestEveryPlaceholderIsResolved(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	section := "## The workflow: sdd\n"

	prompts := map[string]string{
		"orchestrator": OrchestratorPrompt("/skills", section, "`/alfred-worktree`"),
		"fleet":        FleetPrompt("/skills", section),
		"coordinator":  CoordinatorPrompt("/skills", "claude-opus-5", section),
		"subagent":     SubagentPrompt("spec", "/skills/spec/SKILL.md"),
		"manage":       ManagePrompt("/skills/alfred/SKILL.md"),
	}

	set, err := Generate(p, machineGeneration(sddWorkflow(), ventasWorkflow()))
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range set.Commands {
		prompts["command "+command.Name+" on "+strings.Join(command.Agents, "+")] = command.Prompt
	}
	for _, agent := range set.Subagents {
		prompts["subagent "+agent.Name] = agent.Prompt
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
	p := profileFrom(t, workflowProfile)
	home := t.TempDir()
	target := filepath.Join(home, "opencode.json")

	// The two stale entries carry the mark, which is what makes them Alfred's own rather
	// than a name the user holds: an entry this run cannot prove it wrote is a collision and
	// is left exactly where it is.
	stale := `{"model": "stale", "prompt": "` + generatedMark + `\nwhat an earlier run wrote"}`
	existing := `{"theme": "dark", "$schema": "https://opencode.ai/config.json",` +
		` "agent": {"mine": {"model": "keep me"}, "alfred": ` + stale + `,` +
		` "alfred-gone": {"model": "written by hand"}, "alfred-review": ` + stale + `, "zz": {}}}`
	if err := os.WriteFile(target, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	w := apply(t, machineSetFor(t, p, sddWorkflow()), machineTargets(home))

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	if !strings.Contains(text, `"keep me"`) {
		t.Error("an agent that is not Alfred's should survive the merge")
	}
	if strings.Contains(text, `"stale"`) {
		t.Error("the alfred-* agents this run generates should have been replaced")
	}
	if !strings.Contains(text, `"theme": "dark"`) {
		t.Error("unrelated top-level keys should survive")
	}
	if !strings.HasSuffix(text, "}\n") {
		t.Error("the file should end with a newline")
	}

	// The file is OpenCode's: keys stay where they were and an Alfred agent is replaced in
	// place.
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
	if !order(`"mine"`, `"alfred":`, `"alfred-review"`, `"zz"`) {
		t.Errorf("agents were reordered:\n%s", text)
	}

	// Deliberately changed by this change: deleting every alfred-* key this run did not
	// generate takes an agent the user wrote. alfred-gone is claimed by no manifest, so it
	// is reported and kept, and only a key an earlier run recorded is taken back.
	if !strings.Contains(text, `"alfred-gone"`) {
		t.Error("an Alfred-looking agent no manifest claims must not be removed")
	}
	if !has(change(t, w, AgentOpencode).Orphans, "alfred-gone") {
		t.Errorf("and must be reported instead: %+v", change(t, w, AgentOpencode).Orphans)
	}
}

func TestClaudeAgentsWritesOneFilePerPhasePlusCommands(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	home := t.TempDir()

	apply(t, machineSetFor(t, p, sddWorkflow()), machineTargets(home))

	got := strings.Join(tree(t, filepath.Join(home, ".claude")), "\n")
	want := strings.Join([]string{
		"agents/alfred-archive.md",
		"agents/alfred-manage.md",
		"agents/alfred-refine.md",
		"agents/alfred-review.md",
		"commands/alfred-add-workflow.md",
		"commands/alfred-explore.md",
		"commands/alfred-init.md",
		"commands/alfred-sdd.md",
		"commands/alfred-workflows-scanner.md",
		"commands/alfred-worktree.md",
		"commands/alfred.md",
	}, "\n")
	if got != want {
		t.Errorf("wrote\n%s\nwant\n%s", got, want)
	}

	body, err := os.ReadFile(filepath.Join(home, ".claude/agents/alfred-review.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: alfred-review\n",
		`model: "mid"` + "\n",   // the vendor prefix is dropped, and the value is one scalar
		`effort: "high"` + "\n", // only the phases that set it carry the line
		"mcp__atlassian__getJiraIssue\n",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("alfred-review.md is missing %q", want)
		}
	}

	archive, err := os.ReadFile(filepath.Join(home, ".claude/agents/alfred-archive.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(archive), "effort:") {
		t.Error("a phase with no effort set should carry no effort line")
	}

	command, err := os.ReadFile(filepath.Join(home, ".claude/commands/alfred-worktree.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(command), "model: coordinator\n") {
		t.Error("the worktree command should run the coordinator's model")
	}
	if !strings.Contains(string(command), "argument-hint: ") {
		t.Error("a command should carry its argument hint")
	}
	if !strings.HasSuffix(string(command), "$ARGUMENTS\n") {
		t.Error("a command should end with the argument placeholder")
	}
}

const workflowProfile = `{
  "orchestrator": "anthropic/claude-opus-5",
  "coordinator": "vendor/coordinator",
  "manage": "vendor/manage",
  "default_workflow": "sdd",
  "phases": {
    "refine": "vendor/mid", "review": "vendor/mid", "archive": "vendor/small",
    "init": "vendor/mid", "explore": "vendor/mid"
  },
  "memory_tool_prefix": "mcp__engram__",
  "effort": {"review": "high"},
  "extra_tools": {"review": ["mcp__atlassian__getJiraIssue"]}
}`

// sddWorkflow is a workflow built entirely out of shared phases, which is the shipped case.
func sddWorkflow() Registered {
	return Registered{
		Dir: "/alfred/workflows/sdd",
		Definition: &workflow.Definition{
			Name:         "sdd",
			Title:        "Spec-driven development",
			Description:  "Refine, specify, apply, review and archive",
			Rules:        "rules.md",
			Phases:       []workflow.Phase{{Name: "refine"}, {Name: "review"}, {Name: "archive"}},
			Routes:       map[string][]string{"full": {"refine", "review", "archive"}, "direct": {"review", "archive"}},
			DefaultRoute: "full",
			EntryPoints:  map[string]string{"new_feature": "refine"},
			Parallel:     [][]string{{"review", "archive"}},
			Closes:       "archive",
		},
		Phases: []workflow.Resolved{
			{Name: "refine", Skill: "/skills/refine/SKILL.md", Origin: workflow.FromShared, Model: "vendor/mid"},
			{Name: "review", Skill: "/skills/review/SKILL.md", Origin: workflow.FromShared, Model: "vendor/mid"},
			{Name: "archive", Skill: "/skills/archive/SKILL.md", Origin: workflow.FromShared, Model: "vendor/small"},
		},
	}
}

// ventasWorkflow brings two phases of its own, one of which nobody assigned a model to, and
// reuses one shared phase. It declares no parallel group.
func ventasWorkflow() Registered {
	dir := "/alfred/custom/workflows/ventas"
	return Registered{
		Dir: dir,
		Definition: &workflow.Definition{
			Name:         "ventas",
			Title:        "Sales follow-up",
			Description:  "Prospect, propose, follow up and close",
			Rules:        "rules.md",
			Phases:       []workflow.Phase{{Name: "prospectar"}, {Name: "propuesta"}, {Name: "review"}},
			Routes:       map[string][]string{"rapida": {"propuesta", "review"}},
			DefaultRoute: "rapida",
			EntryPoints:  map[string]string{"lead": "prospectar"},
			Parallel:     [][]string{},
			Closes:       "review",
		},
		Phases: []workflow.Resolved{
			{Name: "prospectar", Skill: dir + "/skills/prospectar/SKILL.md", Origin: workflow.FromWorkflow, Model: "vendor/small"},
			{Name: "propuesta", Skill: dir + "/skills/propuesta/SKILL.md", Origin: workflow.FromWorkflow, Tools: []string{"Read", "WebFetch"}},
			{Name: "review", Skill: "/skills/review/SKILL.md", Origin: workflow.FromShared, Model: "vendor/mid"},
		},
	}
}

func machineGeneration(registered ...Registered) Generation {
	return Generation{
		Scope:     Machine,
		Skills:    "/alfred/skills",
		Templates: "/alfred/templates/workflow",
		Custom:    "/alfred/custom/workflows",
		Workflows: registered,
	}
}

func generate(t *testing.T, p *Profile, g Generation) *Set {
	t.Helper()
	set, err := Generate(p, g)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func command(t *testing.T, set *Set, name, agent string) Command {
	t.Helper()
	for _, c := range set.Commands {
		if c.Name == name && c.For(agent) {
			return c
		}
	}
	t.Fatalf("no command %q for %s; got %s", name, agent, strings.Join(commandNames(set), ", "))
	return Command{}
}

func commandNames(set *Set) []string {
	names := map[string]bool{}
	for _, c := range set.Commands {
		names[c.Name] = true
	}
	return sortedKeys(names)
}

func subagentNames(set *Set) []string {
	names := map[string]bool{}
	for _, a := range set.Subagents {
		names[a.Name] = true
	}
	return sortedKeys(names)
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func has(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func TestAWorkflowBecomesACommandOnEveryAgent(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	set := generate(t, p, machineGeneration(sddWorkflow(), ventasWorkflow()))

	for _, agent := range []string{AgentClaude, AgentOpencode} {
		ventas := command(t, set, "alfred-ventas", agent)

		for _, want := range []string{
			"## The workflow: ventas",
			"Sales follow-up",
			"rapida",
			"The default route is `rapida`",
			"/alfred/custom/workflows/ventas/rules.md",
			"`lead`",
		} {
			if !strings.Contains(ventas.Prompt, want) {
				t.Errorf("/alfred-ventas on %s is missing %q", agent, want)
			}
		}
		// The routes of another workflow are not routes here.
		if strings.Contains(ventas.Prompt, "refine -> review") {
			t.Errorf("/alfred-ventas on %s carries sdd's route", agent)
		}
		// Every other workflow of the scope is named with its description, so a request
		// that belongs to one of them is reported rather than run here.
		if !strings.Contains(ventas.Prompt, "`sdd`  Refine, specify, apply, review and archive") {
			t.Errorf("/alfred-ventas on %s does not list the other workflows", agent)
		}
		if !ventas.Delegates {
			t.Errorf("/alfred-ventas on %s should dispatch subagents", agent)
		}
	}

	// The fleet entry point is addressed differently per agent, which is the whole reason a
	// command is generated once per agent.
	if !strings.Contains(command(t, set, "alfred-ventas", AgentClaude).Prompt, "`/alfred-worktree`") {
		t.Error("the Claude command should name the worktree command")
	}
	if !strings.Contains(command(t, set, "alfred-ventas", AgentOpencode).Prompt, "the `alfred-worktree` agent") {
		t.Error("the OpenCode command should name the worktree agent")
	}
}

func TestOwnPhasesGetTheirOwnSubagentAndSharedOnesAreSharedOnce(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	set := generate(t, p, machineGeneration(sddWorkflow(), ventasWorkflow()))

	names := subagentNames(set)
	for _, want := range []string{"alfred-refine", "alfred-review", "alfred-archive", "alfred-ventas-prospectar"} {
		if !has(names, want) {
			t.Errorf("no subagent %q; got %s", want, strings.Join(names, ", "))
		}
	}
	if has(names, "alfred-prospectar") {
		t.Error("a workflow's own phase must not take the shared name")
	}

	// `review` is shared by both workflows and is one subagent, not two.
	count := 0
	for _, agent := range set.Subagents {
		if agent.Name == "alfred-review" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("alfred-review was generated %d times, want 1", count)
	}

	for _, agent := range set.Subagents {
		switch agent.Name {
		case "alfred-review":
			if agent.Model != "vendor/mid" || agent.Effort != "high" {
				t.Errorf("a shared phase takes the profile's assignment, got %s / %s", agent.Model, agent.Effort)
			}
			if !has(agent.Tools, "mcp__atlassian__getJiraIssue") {
				t.Error("a shared phase keeps the profile's extra tools")
			}
		case "alfred-ventas-prospectar":
			if agent.Model != "vendor/small" {
				t.Errorf("an own phase takes the model on its entry, got %s", agent.Model)
			}
			if !strings.Contains(agent.Prompt, "/alfred/custom/workflows/ventas/skills/prospectar/SKILL.md") {
				t.Error("an own phase is dispatched against its own skill")
			}
			if !has(agent.Tools, "mcp__engram__mem_search") {
				t.Error("an own phase still reaches the memory tools")
			}
		}
	}
}

func TestAPhaseWithNoModelGetsNoSubagentAndIsReportedUnavailable(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	set := generate(t, p, machineGeneration(ventasWorkflow()))

	if has(subagentNames(set), "alfred-ventas-propuesta") {
		t.Error("a phase with no model must get no subagent")
	}
	if len(set.Unavailable) != 1 {
		t.Fatalf("got %d phases without a model, want 1", len(set.Unavailable))
	}
	got := set.Unavailable[0]
	if got.Workflow != "ventas" || got.Phase != "propuesta" {
		t.Errorf("the phase without a model is %s/%s", got.Workflow, got.Phase)
	}
	if !strings.Contains(got.Reason, "declares no model") {
		t.Errorf("the reason the report prints is %q", got.Reason)
	}

	// The command says so too, so a run that reaches the phase stops and names it rather
	// than falling back to any model.
	prompt := command(t, set, "alfred-ventas", AgentClaude).Prompt
	if !strings.Contains(prompt, "unavailable") || !strings.Contains(prompt, got.Reason) {
		t.Error("the command should mark the phase unavailable with the reason")
	}
}

// fill is ReplaceAll applied in order, so a title holding a token of the shared prompt would
// be rewritten by a later pass if the workflow section were not substituted last.
func TestTheWorkflowSectionIsSubstitutedLast(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	hostile := ventasWorkflow()
	hostile.Definition.Title = "Sales {{SKILLS_ROOT}} follow-up"
	hostile.Definition.Description = "Close {{FLEET_ENTRY}} deals"

	set := generate(t, p, machineGeneration(hostile))
	prompt := command(t, set, "alfred-ventas", AgentClaude).Prompt

	if !strings.Contains(prompt, "Sales {{SKILLS_ROOT}} follow-up") {
		t.Error("a title holding a token was rewritten by a later pass")
	}
	if !strings.Contains(prompt, "Close {{FLEET_ENTRY}} deals") {
		t.Error("a description holding a token was rewritten by a later pass")
	}
	if !strings.Contains(prompt, "/alfred/skills/<phase>/SKILL.md") {
		t.Error("the skills root should still have been substituted in the shared prompt")
	}
}

func TestAMachineWithNoWorkflowStillGetsTheCommandsThatBelongToNone(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	set := generate(t, p, machineGeneration())

	for _, want := range []string{"alfred-init", "alfred-explore", "alfred-add-workflow", "alfred-workflows-scanner"} {
		if !has(commandNames(set), want) {
			t.Errorf("no command %q on a machine with no workflow; got %s",
				want, strings.Join(commandNames(set), ", "))
		}
	}

	setup := command(t, set, "alfred-init", AgentClaude)
	if setup.Model != "vendor/mid" {
		t.Errorf("/alfred-init runs the profile's init model, got %s", setup.Model)
	}
	if setup.Delegates {
		t.Error("/alfred-init is the phase itself and dispatches nothing")
	}
	if !strings.Contains(setup.Prompt, "/alfred/skills/init/SKILL.md") {
		t.Error("/alfred-init should carry the init skill's path")
	}

	interview := command(t, set, "alfred-add-workflow", AgentClaude)
	for _, want := range []string{"/alfred/templates/workflow", "/alfred/custom/workflows", "`refine`"} {
		if !strings.Contains(interview.Prompt, want) {
			t.Errorf("/alfred-add-workflow is missing %q", want)
		}
	}
	// init and explore are commands of their own and are not offered as phases to declare.
	if strings.Contains(interview.Prompt, "`init`, `refine`") {
		t.Error("the interview should not offer init as a shared phase")
	}

	scanner := command(t, set, "alfred-workflows-scanner", AgentClaude)
	if scanner.Model != "vendor/manage" {
		t.Errorf("the scanner runs on alfred-manage's model, got %s", scanner.Model)
	}
}

func TestInitAndExploreAreNoLongerSubagents(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	set := generate(t, p, machineGeneration(sddWorkflow()))

	for _, gone := range []string{"alfred-init", "alfred-explore"} {
		if has(subagentNames(set), gone) {
			t.Errorf("%s should be a command and not a subagent", gone)
		}
	}
	if !has(subagentNames(set), "alfred-manage") {
		t.Error("alfred-manage is still generated at machine scope")
	}
}

func TestTheConfiguredDefaultThatIsNotInstalledBecomesAStub(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	set := generate(t, p, machineGeneration(ventasWorkflow()))

	stub := command(t, set, "alfred", AgentClaude)
	for _, want := range []string{"sdd", "not installed", "ventas"} {
		if !strings.Contains(stub.Prompt, want) {
			t.Errorf("the /alfred stub is missing %q", want)
		}
	}
	if stub.Delegates {
		t.Error("a stub that stops must not be able to dispatch a phase")
	}
	// No other workflow is run in its place.
	if strings.Contains(stub.Prompt, "## The workflow: ventas") {
		t.Error("the stub must not carry another workflow's section")
	}
}

func TestTheDefaultWorkflowAlsoAnswersAsAlfred(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	set := generate(t, p, machineGeneration(sddWorkflow(), ventasWorkflow()))

	alfred := command(t, set, "alfred", AgentClaude)
	sdd := command(t, set, "alfred-sdd", AgentClaude)
	if alfred.Prompt != sdd.Prompt {
		t.Error("/alfred should be the default workflow's command")
	}
	if !strings.Contains(alfred.Prompt, "## The workflow: sdd") {
		t.Error("/alfred should carry sdd's section")
	}
	if !strings.Contains(alfred.Prompt, "`review` and `archive`") {
		t.Error("sdd's parallel group should be named")
	}
}

func TestADefaultWorkflowAbsentFromTheProfileIsSdd(t *testing.T) {
	p := profileFrom(t, minimalProfile)

	if got := p.DefaultWorkflow(); got != "sdd" {
		t.Errorf("DefaultWorkflow = %q, want sdd", got)
	}
}

func TestProjectScopeGeneratesItsWorkflowsAndNothingElse(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	set := generate(t, p, Generation{Scope: Project, Skills: "/alfred/skills", Workflows: []Registered{ventasWorkflow()}})

	if got := commandNames(set); len(got) != 1 || got[0] != "alfred-ventas" {
		t.Errorf("project scope generated %s, want alfred-ventas alone", strings.Join(got, ", "))
	}
	for _, gone := range []string{"alfred-manage"} {
		if has(subagentNames(set), gone) {
			t.Errorf("%s is a machine-level agent and must not be written into a repository", gone)
		}
	}
	if !has(subagentNames(set), "alfred-ventas-prospectar") {
		t.Error("a repository's workflow still gets its own phases' subagents")
	}
}

// Nothing in a template can branch, so the empty case of every repeated block is text.
func TestTheEmptyCasesAreRenderedRatherThanLeftBlank(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	set := generate(t, p, Generation{Scope: Project, Skills: "/alfred/skills", Workflows: []Registered{ventasWorkflow()}})

	prompt := command(t, set, "alfred-ventas", AgentClaude).Prompt
	if !strings.Contains(prompt, "declares no group dispatched together") {
		t.Error("a workflow with no parallel group should say so")
	}
	if !strings.Contains(prompt, "the only workflow registered here") {
		t.Error("a scope with one workflow should say that there is no other")
	}
}

func TestGenerateRejectsTheSameWorkflowTwiceInOneScope(t *testing.T) {
	p := profileFrom(t, workflowProfile)

	if _, err := Generate(p, machineGeneration(ventasWorkflow(), ventasWorkflow())); err == nil {
		t.Error("one scope cannot register the same workflow twice")
	}
	if _, err := Generate(p, machineGeneration(Registered{Dir: "/x"})); err == nil {
		t.Error("a registered workflow with no definition is a caller error")
	}
}

// A repository-scope workflow may run a phase the repository overrides, and that phase is
// neither the shared one nor the workflow's own: both the subagent name and the reason a
// model is missing say which side provided it.
func TestAPhaseTheRepositoryProvidesIsNamedAsTheRepositorys(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	local := ventasWorkflow()
	local.Phases = []workflow.Resolved{
		{Name: "prospectar", Skill: "/repo/.alfred/skills/prospectar/SKILL.md", Origin: workflow.FromRepository, Model: "vendor/small"},
		{Name: "propuesta", Skill: "/repo/.alfred/skills/propuesta/SKILL.md", Origin: workflow.FromRepository},
		{Name: "review", Skill: "/skills/review/SKILL.md", Origin: workflow.FromShared, Model: "vendor/mid"},
	}

	set := generate(t, p, Generation{Scope: Project, Skills: "/alfred/skills", Workflows: []Registered{local}})

	if !has(subagentNames(set), "alfred-ventas-prospectar") {
		t.Errorf("a repository-provided phase is still the workflow's own subagent; got %s",
			strings.Join(subagentNames(set), ", "))
	}
	if len(set.Unavailable) != 1 || !strings.Contains(set.Unavailable[0].Reason, "the repository's own propuesta") {
		t.Errorf("the reason should name the side that provided the phase, got %v", set.Unavailable)
	}
	if !strings.Contains(command(t, set, "alfred-ventas", AgentClaude).Prompt, "(the repository's own)") {
		t.Error("the command should say which side provided the phase")
	}
}

// A shared phase the installation never assigned a model to is the other half of the same
// rule: no subagent, and a reason that points at the profile rather than at the workflow.
func TestASharedPhaseWithNoModelInTheProfileIsAlsoUnavailable(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	sdd := sddWorkflow()
	sdd.Phases[1].Model = ""

	set := generate(t, p, machineGeneration(sdd))

	if has(subagentNames(set), "alfred-review") {
		t.Error("a shared phase with no model must get no subagent either")
	}
	if len(set.Unavailable) != 1 || !strings.Contains(set.Unavailable[0].Reason, "the shared phase review") {
		t.Errorf("the reason should point at the installation's profile, got %v", set.Unavailable)
	}
}
