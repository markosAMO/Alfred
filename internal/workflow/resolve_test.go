package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkill(t *testing.T, root, phase string) string {
	t.Helper()

	dir := filepath.Join(root, phase)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, SkillFile)
	if err := os.WriteFile(path, []byte("# "+phase+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// definitionOf is the part of a definition resolution reads. The rest is validated in
// workflow_test.go and says nothing about where a phase comes from.
func definitionOf(phases ...Phase) *Definition {
	return &Definition{Name: "ventas", Phases: phases}
}

// The scope is in the roots: a machine workflow is handed no repository root, and that is
// the whole difference between the two.
func machineSkills(home, workflow string) SkillRoots {
	return SkillRoots{
		Own:    filepath.Join(home, "workflows", workflow, "skills"),
		Shared: filepath.Join(home, "skills"),
	}
}

func projectSkills(home, repo, workflow string) SkillRoots {
	return SkillRoots{
		Own:        filepath.Join(repo, ".alfred", "workflows", workflow, "skills"),
		Repository: filepath.Join(repo, ".alfred", "skills"),
		Shared:     filepath.Join(home, "skills"),
	}
}

func TestAPhaseTheWorkflowBringsResolvesBeforeEveryOtherRoot(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	roots := projectSkills(home, repo, "ventas")
	own := writeSkill(t, roots.Own, "review")
	writeSkill(t, roots.Repository, "review")
	writeSkill(t, roots.Shared, "review")

	installed := map[string]string{"review": "anthropic/claude-sonnet-4-5"}
	got, err := Resolve(definitionOf(Phase{Name: "review", Model: "anthropic/claude-opus-5"}), roots, installed)
	if err != nil {
		t.Fatalf("Resolve = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Resolve = %+v, want one phase", got)
	}
	if got[0].Skill != own {
		t.Errorf("skill = %q, want the workflow's own %q", got[0].Skill, own)
	}
	if got[0].Origin != FromWorkflow {
		t.Errorf("origin = %q, want %q", got[0].Origin, FromWorkflow)
	}
	// Its own review must not inherit the shared review's assignment.
	if got[0].Model != "anthropic/claude-opus-5" {
		t.Errorf("model = %q, want the one the workflow declares", got[0].Model)
	}
}

// The same phase name, in a workflow that brings no version of it, still reaches the
// shared library — which is what keeps one workflow's review out of another's run.
func TestAWorkflowThatBringsNoneReachesTheSharedPhase(t *testing.T) {
	home := t.TempDir()
	roots := machineSkills(home, "compras")
	shared := writeSkill(t, roots.Shared, "review")

	installed := map[string]string{"review": "anthropic/claude-sonnet-4-5"}
	got, err := Resolve(definitionOf(Phase{Name: "review"}), roots, installed)
	if err != nil {
		t.Fatalf("Resolve = %v", err)
	}
	if got[0].Skill != shared || got[0].Origin != FromShared {
		t.Errorf("resolved to %q from %q, want the shared %q", got[0].Skill, got[0].Origin, shared)
	}
	// A shared phase runs on the model the installation assigned it.
	if got[0].Model != "anthropic/claude-sonnet-4-5" {
		t.Errorf("model = %q, want the installation's assignment", got[0].Model)
	}
}

func TestARepositoryWorkflowResolvesAPhaseTheRepositoryOverrides(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	roots := projectSkills(home, repo, "ventas")
	override := writeSkill(t, roots.Repository, "review")
	writeSkill(t, roots.Shared, "review")
	writeSkill(t, roots.Shared, "design")

	installed := map[string]string{"review": "anthropic/claude-sonnet-4-5", "design": "anthropic/claude-opus-5"}
	got, err := Resolve(definitionOf(Phase{Name: "review"}, Phase{Name: "design"}), roots, installed)
	if err != nil {
		t.Fatalf("a repository workflow was rejected where it runs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Resolve = %+v, want both phases", got)
	}

	if got[0].Skill != override || got[0].Origin != FromRepository {
		t.Errorf("review resolved to %q from %q, want the repository's %q", got[0].Skill, got[0].Origin, override)
	}
	// The repository created this one, so the profile is not consulted for it: a phase
	// the repository overrides and the workflow assigns no model to has none.
	if got[0].Model != "" {
		t.Errorf("model = %q, want none rather than the installation's", got[0].Model)
	}
	if got[1].Origin != FromShared || got[1].Model != "anthropic/claude-opus-5" {
		t.Errorf("design resolved from %q on %q, want the shared library and its assignment", got[1].Origin, got[1].Model)
	}
}

// A model on a phase the installation created is refused rather than ignored. The key is
// meaningless there — `profile.json` is the authority for a shared phase — and a
// declaration with no effect, reported once and overruled, is the silent discard under
// another name.
func TestAModelDeclaredOnASharedPhaseIsRejected(t *testing.T) {
	home := t.TempDir()
	roots := machineSkills(home, "ventas")
	writeSkill(t, roots.Shared, "review")

	installed := map[string]string{"review": "anthropic/claude-sonnet-4-5"}
	got, err := Resolve(definitionOf(Phase{Name: "review", Model: "anthropic/claude-opus-5"}),
		roots, installed)
	if got != nil {
		t.Errorf("Resolve = %+v, want nothing resolved", got)
	}
	want := `phase "review" declares a model and resolves to the shared library, where the ` +
		`model is the installation's assignment in profile.json`
	if err == nil || err.Error() != want {
		t.Errorf("Resolve = %v, want %q", err, want)
	}
}

// Exactly the class the model is. What a shared phase runs with is the installation's, and
// a declaration with no effect is the silent discard this change exists to end.
func TestAToolSetDeclaredOnASharedPhaseIsRejected(t *testing.T) {
	home := t.TempDir()
	roots := machineSkills(home, "ventas")
	writeSkill(t, roots.Shared, "review")

	got, err := Resolve(definitionOf(Phase{Name: "review", Tools: []string{"Read", "WebFetch"}}),
		roots, map[string]string{"review": "anthropic/claude-sonnet-4-5"})
	if got != nil {
		t.Errorf("Resolve = %+v, want nothing resolved", got)
	}
	want := `phase "review" declares a tool set and resolves to the shared library, where the ` +
		`tool set is the installation's assignment`
	if err == nil || err.Error() != want {
		t.Errorf("Resolve = %v, want %q", err, want)
	}
}

// A shared phase that declares neither resolves, and carries no tool set of its own: the
// generator reads the installation's for it, and a set left on the result would be a second
// answer to a question that has one.
func TestASharedPhaseCarriesNoToolSetOfItsOwn(t *testing.T) {
	home := t.TempDir()
	roots := machineSkills(home, "ventas")
	writeSkill(t, roots.Shared, "review")

	got, err := Resolve(definitionOf(Phase{Name: "review"}), roots,
		map[string]string{"review": "anthropic/claude-sonnet-4-5"})
	if err != nil {
		t.Fatalf("Resolve = %v, want the shared phase accepted", err)
	}
	if got[0].Origin != FromShared || len(got[0].Tools) != 0 {
		t.Errorf("Resolve = %+v, want the shared phase with no tool set", got[0])
	}
}

// The rejection is about where the phase resolved and not about its name: the same name,
// brought by the workflow, is a phase the workflow created and may assign.
func TestAToolSetOnAPhaseTheWorkflowBringsIsNotRejectedForItsName(t *testing.T) {
	home := t.TempDir()
	roots := machineSkills(home, "ventas")
	writeSkill(t, roots.Own, "review")
	writeSkill(t, roots.Shared, "review")

	got, err := Resolve(definitionOf(Phase{Name: "review", Tools: []string{"Read", "WebFetch"}}),
		roots, map[string]string{"review": "anthropic/claude-sonnet-4-5"})
	if err != nil {
		t.Fatalf("Resolve = %v, want the workflow's own review accepted with its tools", err)
	}
	if got[0].Origin != FromWorkflow || len(got[0].Tools) != 2 {
		t.Errorf("Resolve = %+v, want the workflow's own phase on the tools it declares", got[0])
	}
}

// The rejection is about where the phase resolved and not about its name: the same name,
// brought by the workflow, is a phase the workflow created and may assign.
func TestAModelOnAPhaseTheWorkflowBringsIsNotRejectedForItsName(t *testing.T) {
	home := t.TempDir()
	roots := machineSkills(home, "ventas")
	writeSkill(t, roots.Own, "review")
	writeSkill(t, roots.Shared, "review")

	got, err := Resolve(definitionOf(Phase{Name: "review", Model: "anthropic/claude-opus-5"}),
		roots, map[string]string{"review": "anthropic/claude-sonnet-4-5"})
	if err != nil {
		t.Fatalf("Resolve = %v, want the workflow's own review accepted with its model", err)
	}
	if got[0].Origin != FromWorkflow || got[0].Model != "anthropic/claude-opus-5" {
		t.Errorf("Resolve = %+v, want the workflow's own phase on the model it declares", got[0])
	}
}

func TestAMachineWorkflowNamingAPhaseOnlyARepositoryHasIsRejected(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	writeSkill(t, filepath.Join(repo, ".alfred", "skills"), "cotizar")
	roots := machineSkills(home, "antiguo")
	writeSkill(t, roots.Shared, "review")

	got, err := Resolve(definitionOf(Phase{Name: "cotizar"}, Phase{Name: "review"}), roots, nil)
	if got != nil {
		t.Errorf("Resolve = %+v, want nothing resolved", got)
	}
	want := `phase "cotizar" resolves in no machine root; a phase only a repository provides ` +
		`belongs to a repository-scope workflow`
	if err == nil || err.Error() != want {
		t.Errorf("Resolve = %v, want %q", err, want)
	}
}

func TestAPhaseThatResolvesInNoRootOfItsScopeIsRejected(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	roots := projectSkills(home, repo, "ventas")
	writeSkill(t, roots.Shared, "review")

	_, err := Resolve(definitionOf(Phase{Name: "review"}, Phase{Name: "cotizar"}), roots, nil)
	want := `phase "cotizar" resolves in neither the workflow, the repository nor the shared library`
	if err == nil || err.Error() != want {
		t.Errorf("Resolve = %v, want %q", err, want)
	}
}

// A phase nobody assigned a model to is a result the report names, not a failure: the
// workflow is coherent and its other routes are not denied for it.
func TestAPhaseWithNoModelResolvesAndCarriesNone(t *testing.T) {
	home := t.TempDir()
	roots := machineSkills(home, "ventas")
	own := writeSkill(t, roots.Own, "propuesta")

	got, err := Resolve(definitionOf(Phase{Name: "propuesta", Tools: []string{"Read", "WebFetch"}}),
		roots, map[string]string{"propuesta": "anthropic/claude-opus-5"})
	if err != nil {
		t.Fatalf("a phase with no model stopped the resolution: %v", err)
	}
	if got[0].Skill != own || got[0].Model != "" {
		t.Errorf("Resolve = %+v, want the workflow's own phase with no model", got[0])
	}
	// There is no fallback to the orchestrator's model and none to the profile's entry of
	// the same name, which is what a workflow's own phase being its own means.
	if len(got[0].Tools) != 2 || got[0].Tools[0] != "Read" {
		t.Errorf("tools = %v, want the set the definition declares", got[0].Tools)
	}
}

func TestResolveReportsTheFirstPhaseThatResolvesNowhere(t *testing.T) {
	home := t.TempDir()
	roots := machineSkills(home, "ventas")

	_, err := Resolve(definitionOf(Phase{Name: "uno"}, Phase{Name: "dos"}), roots, nil)
	if err == nil {
		t.Fatal("a workflow whose phases resolve nowhere was accepted")
	}
	for range 20 {
		_, again := Resolve(definitionOf(Phase{Name: "uno"}, Phase{Name: "dos"}), roots, nil)
		if again.Error() != err.Error() {
			t.Fatalf("two runs disagree: %q and %q", err, again)
		}
	}
}
