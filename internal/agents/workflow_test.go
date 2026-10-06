package agents

import (
	"strings"
	"testing"

	"github.com/markosAMO/alfred/internal/workflow"
)

// outsideSection is the prompt with the workflow section taken out of it, which is what a
// test about the shared prose has to look at: the section legitimately names the phases of
// the workflow it describes, and the prose around it must name none.
func outsideSection(t *testing.T, prompt, section string) string {
	t.Helper()
	rest := strings.Replace(prompt, section, "", 1)
	if rest == prompt {
		t.Fatal("the prompt carries no workflow section")
	}
	return rest
}

// The coordinator starts bare background sessions. They run no workflow command, so the
// structural facts they work from are the ones this prompt hands them, and before this they
// had none from any source: `routing.routes` in `.alfred/config.yaml` was where they used to
// come from and it no longer exists.
func TestTheClaudeWorktreeCommandCarriesTheDefaultWorkflowsFacts(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	all := []Registered{sddWorkflow(), ventasWorkflow()}
	set := generate(t, p, machineGeneration(all...))

	section := workflowSection(sddWorkflow(), all, map[string]string{})
	coordinator := command(t, set, "alfred-worktree", AgentClaude)
	fleet := command(t, set, "alfred-worktree", AgentOpencode)

	if !strings.Contains(coordinator.Prompt, section) {
		t.Error("the Claude half of /alfred-worktree carries no workflow section")
	}
	// One rendering, two carriers: the section is not re-derived for the coordinator, so
	// the two paths have no way to drift.
	if !strings.Contains(fleet.Prompt, section) {
		t.Error("the OpenCode half carries a different rendering of the same workflow")
	}
	// The default workflow's facts, and no other workflow's.
	if strings.Contains(coordinator.Prompt, "## The workflow: ventas") {
		t.Error("the coordinator carries a workflow that is not the configured default")
	}
	if coordinator.Model != "vendor/coordinator" {
		t.Errorf("the coordinator runs on its own model, got %s", coordinator.Model)
	}
	if !strings.Contains(coordinator.Prompt, "claude-opus-5") {
		t.Error("the coordinator should still name the model the sessions it starts run on")
	}
}

// The coordinator holds the facts for the sessions and not for itself: it proposes no route
// and dispatches no phase, so the only thing it does with the section is pass it on whole.
func TestTheCoordinatorRelaysTheSectionToEachSession(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	all := []Registered{sddWorkflow()}
	set := generate(t, p, machineGeneration(all...))

	section := workflowSection(sddWorkflow(), all, map[string]string{})
	rest := outsideSection(t, command(t, set, "alfred-worktree", AgentClaude).Prompt, section)

	work := line(rest, "Give each session its work")
	if work == "" {
		t.Fatal("the step that gives each session its work is gone")
	}
	for _, want := range []string{"the workflow section", "verbatim"} {
		if !strings.Contains(work, want) {
			t.Errorf("the step that gives a session its work does not mention %q", want)
		}
	}
}

// Held to the same rule as the other two shared prompts: a workflow whose changes run under
// the coordinator would otherwise inherit a rule about a phase it does not have. The rule is
// checked over the default workflow's declared phases rather than over the whole shared
// library, because the prompt names `a specification, a design, a diff` as documents the
// coordinator must not read, and `design` is a document in that sentence rather than a phase
// anybody dispatches.
func TestTheCoordinatorPromptNamesNoPhaseOfAnyWorkflow(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	all := []Registered{sddWorkflow()}
	set := generate(t, p, machineGeneration(all...))

	section := workflowSection(sddWorkflow(), all, map[string]string{})
	rest := outsideSection(t, command(t, set, "alfred-worktree", AgentClaude).Prompt, section)

	for _, phase := range []string{"refine", "review", "archive"} {
		if strings.Contains(rest, phase) {
			t.Errorf("the shared coordinator prose names the phase %q", phase)
		}
	}
	// `init` is the exception and stays: it is a command that belongs to no workflow.
	if !strings.Contains(rest, "/alfred-init") {
		t.Error("the coordinator should still send a repository with no .alfred/ to /alfred-init")
	}
}

// fill is ReplaceAll applied in order, so a title holding a token of this prompt would be
// rewritten by a later pass if the section were not substituted after every other token.
func TestTheWorkflowSectionIsSubstitutedLastInTheCoordinator(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	hostile := sddWorkflow()
	hostile.Definition.Title = "Spec-driven {{SESSION_MODEL}} development"
	hostile.Definition.Description = "Refine in {{SKILLS_ROOT}} and archive"

	set := generate(t, p, machineGeneration(hostile))
	prompt := command(t, set, "alfred-worktree", AgentClaude).Prompt

	if !strings.Contains(prompt, "Spec-driven {{SESSION_MODEL}} development") {
		t.Error("a title holding a token was rewritten by a later pass")
	}
	if !strings.Contains(prompt, "Refine in {{SKILLS_ROOT}} and archive") {
		t.Error("a description holding a token was rewritten by a later pass")
	}
	if !strings.Contains(prompt, "claude-opus-5") {
		t.Error("the session model should still have been substituted in the shared prompt")
	}
	if !strings.Contains(prompt, "/alfred/skills") {
		t.Error("the skills root should still have been substituted in the shared prompt")
	}
}

// A coordinator with no default workflow has no facts to give the sessions it would start,
// which is the whole reason this change exists. Both halves of the fleet entry point stop
// for that reason, and neither runs another workflow in its place.
//
// The tool set is asserted beside the prompt because the prompt alone is an instruction and
// the tool set is what makes stopping the only thing the command can do: a stopped entry
// point holding Task can dispatch a phase anyway and one holding Bash can start the
// background sessions it is telling itself not to start.
func TestTheCoordinatorStopsWhenTheDefaultWorkflowIsNotInstalled(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	set := generate(t, p, machineGeneration(ventasWorkflow()))
	guard := command(t, set, "alfred", AgentClaude)

	for _, agent := range []string{AgentClaude, AgentOpencode} {
		stopped := command(t, set, "alfred-worktree", agent)
		for _, want := range []string{"/alfred-worktree", "sdd", "not installed", "ventas"} {
			if !strings.Contains(stopped.Prompt, want) {
				t.Errorf("the stopped %s entry point is missing %q", agent, want)
			}
		}
		if strings.Contains(stopped.Prompt, "## The workflow: ventas") {
			t.Errorf("the stopped %s entry point must not start sessions on a workflow nobody asked for", agent)
		}
		if strings.Contains(stopped.Prompt, "{{") {
			t.Errorf("the stopped %s entry point still holds an unresolved placeholder", agent)
		}
		if got, want := strings.Join(stopped.Tools, ", "), strings.Join(guard.Tools, ", "); got != want {
			t.Errorf("the stopped %s entry point holds %q, not the stub's %q", agent, got, want)
		}
		if stopped.Delegates {
			t.Errorf("the stopped %s entry point can still delegate", agent)
		}
	}
}

// ownWorkflow is a workflow every one of whose phases is its own, which is the only side a
// generated subagent name takes the workflow's name from. Every phase has a model, so each
// one reaches the map Generate keys its subagents by.
func ownWorkflow(name string, phases ...string) Registered {
	dir := "/alfred/custom/workflows/" + name
	definition := &workflow.Definition{
		Name:         name,
		Title:        name,
		Description:  "a workflow built out of its own phases",
		Rules:        "rules.md",
		Routes:       map[string][]string{"todo": phases},
		DefaultRoute: "todo",
		EntryPoints:  map[string]string{"lead": phases[0]},
		Closes:       phases[len(phases)-1],
	}

	registered := Registered{Dir: dir, Definition: definition}
	for _, phase := range phases {
		definition.Phases = append(definition.Phases, workflow.Phase{Name: phase})
		registered.Phases = append(registered.Phases, workflow.Resolved{
			Name:   phase,
			Skill:  dir + "/skills/" + phase + "/SKILL.md",
			Origin: workflow.FromWorkflow,
			Model:  "vendor/small",
		})
	}
	return registered
}

// `alfred-<workflow>-<phase>` is not injective. Generate keys its subagents by that name, so
// without this the second assignment wins silently and one workflow's command dispatches a
// subagent carrying the other's prompt and model.
func TestTwoWorkflowsGeneratingOneSubagentNameAreBothRejected(t *testing.T) {
	p := profileFrom(t, workflowProfile)

	set, err := Generate(p, machineGeneration(
		ownWorkflow("ventas", "extra-propuesta"),
		ownWorkflow("ventas-extra", "propuesta")))
	if err == nil {
		t.Fatal("two workflows generating one subagent name must be rejected")
	}
	if set != nil {
		t.Error("a rejected scope generates nothing")
	}

	// The name and both workflows, so the user can see which two to rename.
	for _, want := range []string{`"alfred-ventas-extra-propuesta"`, `"ventas"`, `"ventas-extra"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the rejection does not name %s: %s", want, err)
		}
	}
	if !strings.Contains(err.Error(), "neither") {
		t.Errorf("the rejection should say that neither workflow is registered: %s", err)
	}
}

// The ambiguity is a property of the two definitions and not of the profile, so a phase
// nobody assigned a model to still collides. Deciding it by the models would mean a
// registration that passes today and fails the day a model is filled in.
func TestASubagentNameCollidesEvenWhereOneSideHasNoModel(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	quiet := ownWorkflow("ventas-extra", "propuesta")
	quiet.Phases[0].Model = ""

	if _, err := Generate(p, machineGeneration(ownWorkflow("ventas", "extra-propuesta"), quiet)); err == nil {
		t.Error("a phase with no model still declares the name its workflow would generate")
	}
}

// The check rejects an ambiguity and nothing else. Two workflows whose own phases generate
// distinct names are the ordinary case and both keep their subagents.
func TestOwnPhasesOfDifferentWorkflowsKeepTheirOwnSubagents(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	set := generate(t, p, machineGeneration(
		ownWorkflow("ventas", "propuesta"),
		ownWorkflow("ventas-extra", "cotizar")))

	for _, want := range []string{"alfred-ventas-propuesta", "alfred-ventas-extra-cotizar"} {
		if !has(subagentNames(set), want) {
			t.Errorf("missing the subagent %q; got %s", want, strings.Join(subagentNames(set), ", "))
		}
	}
}

// A shared phase is one subagent however many workflows run it, which is the one case where
// two workflows are meant to produce the same name.
func TestASharedPhaseInTwoWorkflowsIsNotACollision(t *testing.T) {
	p := profileFrom(t, workflowProfile)
	set := generate(t, p, machineGeneration(sddWorkflow(), ventasWorkflow()))

	seen := 0
	for _, agent := range set.Subagents {
		if agent.Name == "alfred-review" {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("sdd and ventas both run the shared review; got %d subagents for it", seen)
	}
}

// line returns the whole line holding needle, so a test about one step reads that step and
// not the rest of the prompt.
func line(text, needle string) string {
	for _, one := range strings.Split(text, "\n") {
		if strings.Contains(one, needle) {
			return one
		}
	}
	return ""
}

// What each phase reads is the workflow's declaration, rendered once per phase, so the
// orchestrator can hand a phase its locators without reading the phase's skill.
func TestTheSectionNamesWhatEachPhaseReads(t *testing.T) {
	r := Registered{Dir: "/w/ventas", Definition: &workflow.Definition{
		Name: "ventas", Title: "Ventas", Description: "d", Rules: "rules.md",
		Phases: []workflow.Phase{
			{Name: "prospectar"},
			{Name: "propuesta", Reads: []string{"prospectar", "architecture"}, Recall: []string{"propuesta"}},
		},
		Routes: map[string][]string{"r": {"prospectar", "propuesta"}}, DefaultRoute: "r",
		EntryPoints: map[string]string{"default": "prospectar"}, Closes: "propuesta",
	}}
	section := workflowSection(r, []Registered{r}, map[string]string{})

	for _, want := range []string{
		"  prospectar  reads nothing\n",
		"  propuesta   reads prospectar, architecture; recalls propuesta\n",
		"alfred/{change}/{phase}",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("the section does not carry %q:\n%s", want, section)
		}
	}
}
