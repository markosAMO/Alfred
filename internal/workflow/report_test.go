package workflow

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// machineArgs is the machine form with the roots a tree built by writeWorkflow has.
func machineArgs(home, mode string, targets ...string) []string {
	args := []string{
		"machine", mode,
		filepath.Join(home, "profile.json"),
		filepath.Join(home, "skills"),
		filepath.Join(home, "workflows"),
		filepath.Join(home, "custom", "workflows"),
		filepath.Join(home, "templates", "workflow"),
	}
	return append(args, targets...)
}

func projectArgs(home, repo, mode string, targets ...string) []string {
	args := []string{
		"project", mode,
		filepath.Join(home, "profile.json"),
		filepath.Join(home, "skills"),
		filepath.Join(repo, ".alfred", "skills"),
		filepath.Join(repo, ".alfred", "workflows"),
		filepath.Join(home, "workflows"),
		filepath.Join(home, "custom", "workflows"),
	}
	return append(args, targets...)
}

func parsed(t *testing.T, args []string) *Request {
	t.Helper()
	request, err := Parse(args)
	if err != nil {
		t.Fatalf("Parse(%v) = %v", args, err)
	}
	return request
}

// ventasPhases writes the one shared phase the valid definition names, so a workflow that
// is about something else resolves rather than failing for a reason the test is not about.
//
// The other three are the workflow's own and are written beside the definition by
// ventasWorkflow. They are not here because the valid definition assigns them a model,
// which is the workflow's to assign only because it brought them itself: resolving them
// from the shared library would make every test in this file a rejection.
func ventasPhases(t *testing.T, shared string) {
	t.Helper()
	writeSkill(t, shared, "review")
}

// ventasWorkflow is one copy of the valid definition with the three phases it brings
// itself, under the workflow's own skills/ root. It is what a registered workflow looks
// like on disk; writeWorkflow alone is the definition with no phases of its own.
func ventasWorkflow(t *testing.T, root, name string, edit func(map[string]any)) {
	t.Helper()

	dir := writeWorkflow(t, root, name, edit)
	for _, phase := range []string{"prospectar", "propuesta", "seguimiento"} {
		writeSkill(t, filepath.Join(dir, "skills"), phase)
	}
}

// ownPhaseWorkflow is a workflow every one of whose phases it brings itself, which is the
// only side a generated subagent name takes the workflow's name from. Every phase carries a
// model, so the rejection under test is the one the test is about.
func ownPhaseWorkflow(t *testing.T, root, name string, phases ...string) {
	t.Helper()

	route := make([]any, 0, len(phases))
	declared := make([]any, 0, len(phases))
	for _, phase := range phases {
		route = append(route, phase)
		declared = append(declared, map[string]any{
			"name": phase, "model": "anthropic/claude-haiku-4-5",
		})
	}

	dir := writeWorkflow(t, root, name, func(d map[string]any) {
		d["phases"] = declared
		d["routes"] = map[string]any{"todo": route}
		d["default_route"] = "todo"
		d["entry_points"] = map[string]any{"default": phases[0]}
		d["closes"] = phases[len(phases)-1]
		d["parallel"] = []any{}
	})
	for _, phase := range phases {
		writeSkill(t, filepath.Join(dir, "skills"), phase)
	}
}

// lineFor finds one workflow's line by name, because the report lists every workflow of the
// scope and a test about one of them should not depend on where it lands.
func lineFor(t *testing.T, lines []Line, name string) Line {
	t.Helper()
	for _, line := range lines {
		if line.Name == name {
			return line
		}
	}
	t.Fatalf("no line for %q in %+v", name, lines)
	return Line{}
}

func installed() map[string]string {
	return map[string]string{"review": "anthropic/claude-opus-5"}
}

func TestParseReadsTheMachineForm(t *testing.T) {
	home := t.TempDir()
	request := parsed(t, machineArgs(home, "apply",
		"manifest="+filepath.Join(home, "generated.json"),
		"claude="+filepath.Join(home, ".claude"),
		"opencode="+filepath.Join(home, "opencode.json")))

	if request.Scope != ScopeMachine || request.Mode != ModeApply {
		t.Fatalf("Parse scope/mode = %q/%q", request.Scope, request.Mode)
	}
	if request.Templates != filepath.Join(home, "templates", "workflow") {
		t.Fatalf("Parse templates = %q", request.Templates)
	}
	if request.Targets.Claude != filepath.Join(home, ".claude") {
		t.Fatalf("Parse claude = %q", request.Targets.Claude)
	}
	if request.Targets.Opencode != filepath.Join(home, "opencode.json") {
		t.Fatalf("Parse opencode = %q", request.Targets.Opencode)
	}

	want := machineRoots(home)
	if got := request.Roots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Roots = %+v, want %+v", got, want)
	}
}

func TestParseReadsTheProjectFormAndItsMachineRoots(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	request := parsed(t, projectArgs(home, repo, "report",
		"manifest="+filepath.Join(repo, ".alfred", "generated.json"),
		"root="+repo,
		"exclude="+filepath.Join(repo, ".git", "info", "exclude"),
		"claude="+filepath.Join(repo, ".claude")))

	if request.Scope != ScopeProject || request.Mode != ModeReport {
		t.Fatalf("Parse scope/mode = %q/%q", request.Scope, request.Mode)
	}
	// The machine roots are read-only here, and they are what tells a repository workflow
	// that its name is already a command on this machine.
	if request.MachineWorkflows != filepath.Join(home, "workflows") {
		t.Fatalf("Parse machine workflows = %q", request.MachineWorkflows)
	}
	if request.MachineCustom != filepath.Join(home, "custom", "workflows") {
		t.Fatalf("Parse machine custom = %q", request.MachineCustom)
	}
	want := []Root{{Label: "project", Dir: filepath.Join(repo, ".alfred", "workflows")}}
	if got := request.Roots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Roots = %+v, want %+v", got, want)
	}
}

func TestParseRefusesWhatIsCalledWrongly(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	manifest := "manifest=" + filepath.Join(home, "generated.json")

	cases := map[string][]string{
		"no arguments":          {},
		"no mode":               {"machine"},
		"unknown scope":         {"everything", "apply", "a", "b", "c", "d", "e", manifest},
		"unknown mode":          machineArgs(home, "write", manifest),
		"too few roots":         {"machine", "apply", "a", "b", "c"},
		"no manifest":           machineArgs(home, "apply", "claude="+repo),
		"target without a kind": machineArgs(home, "apply", manifest, repo),
		"unknown kind":          machineArgs(home, "apply", manifest, "cursor="+repo),
		"a kind with no path":   machineArgs(home, "apply", manifest, "claude="),
		"exclude at machine scope": machineArgs(home, "apply", manifest,
			"root="+repo, "exclude="+filepath.Join(repo, "exclude")),
		"root at machine scope": machineArgs(home, "apply", manifest, "root="+repo),
		"exclude with no root": projectArgs(home, repo, "apply", manifest,
			"exclude="+filepath.Join(repo, "exclude")),
		"both exclude forms": projectArgs(home, repo, "apply", manifest, "root="+repo,
			"exclude="+filepath.Join(repo, "exclude"),
			"exclude-manifest="+filepath.Join(repo, "exclude")),
		"the same kind twice": machineArgs(home, "apply", manifest, "claude="+repo, "claude="+repo),
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if request, err := Parse(args); err == nil {
				t.Fatalf("Parse(%v) = %+v, want an error", args, request)
			}
		})
	}
}

// A workflow declares its own recipe: the report names it with its source, its phase count
// and its routes, which is what the specification asks the registration report to carry.
func TestPrepareRegistersAWorkflowAndLinesItUpWithItsFacts(t *testing.T) {
	home := t.TempDir()
	ventasPhases(t, filepath.Join(home, "skills"))
	ventasWorkflow(t, filepath.Join(home, "workflows"), "ventas", nil)

	request := parsed(t, machineArgs(home, "apply", "manifest="+filepath.Join(home, "generated.json")))
	accepted, lines, err := request.Prepare(installed())
	if err != nil {
		t.Fatalf("Prepare = %v", err)
	}

	if len(accepted) != 1 || accepted[0].Name != "ventas" {
		t.Fatalf("Prepare accepted %+v", accepted)
	}
	if len(accepted[0].Phases) != 4 {
		t.Fatalf("Prepare resolved %d phases, want 4", len(accepted[0].Phases))
	}
	want := Line{
		Name: "ventas", Source: "machine/shipped", Phases: 4,
		Routes: []string{"completa", "rapida"}, Command: "alfred-ventas",
	}
	if !reflect.DeepEqual(lines, []Line{want}) {
		t.Fatalf("Prepare lines = %+v, want %+v", lines, []Line{want})
	}
}

// One broken custom workflow: the valid ones register and the invalid one keeps its place
// in the report with the reason it was refused.
func TestPrepareKeepsARejectedWorkflowInTheReport(t *testing.T) {
	home := t.TempDir()
	ventasPhases(t, filepath.Join(home, "skills"))
	custom := filepath.Join(home, "custom", "workflows")
	ventasWorkflow(t, filepath.Join(home, "workflows"), "ventas", nil)
	ventasWorkflow(t, custom, "compras", nil)
	writeWorkflow(t, custom, "roto", func(d map[string]any) {
		d["routes"] = map[string]any{"rapida": []any{"propuest"}}
		d["default_route"] = "rapida"
	})

	request := parsed(t, machineArgs(home, "apply", "manifest="+filepath.Join(home, "generated.json")))
	accepted, lines, err := request.Prepare(installed())
	if err != nil {
		t.Fatalf("Prepare = %v", err)
	}

	if len(accepted) != 2 {
		t.Fatalf("Prepare accepted %d workflows, want the two valid ones: %+v", len(accepted), accepted)
	}
	if len(lines) != 3 {
		t.Fatalf("Prepare lines = %+v, want one per workflow found", lines)
	}
	// Name order, so a report over the same roots reads the same way twice.
	broken := lines[1]
	if broken.Name != "roto" || broken.Source != "machine/custom" {
		t.Fatalf("Prepare line = %+v, want the rejected one in name order", broken)
	}
	if !strings.Contains(broken.Reason, `route "rapida" names phase "propuest"`) {
		t.Fatalf("Prepare reason = %q", broken.Reason)
	}
	if broken.Command != "" {
		t.Fatalf("a rejected workflow carries command %q, want none", broken.Command)
	}
}

// Two workflows of one scope generating the same subagent name: neither of the two
// registers, the report names the generated name and both workflows, and every other
// workflow of the scope registers. The collision is a property of a pair of definitions, so
// it is an ordinary rejection and never a refusal of the whole run.
func TestPrepareRejectsBothWorkflowsThatGenerateOneSubagentName(t *testing.T) {
	home := t.TempDir()
	ventasPhases(t, filepath.Join(home, "skills"))
	custom := filepath.Join(home, "custom", "workflows")
	ownPhaseWorkflow(t, custom, "ventas", "extra-propuesta")
	ownPhaseWorkflow(t, custom, "ventas-extra", "propuesta")
	ventasWorkflow(t, filepath.Join(home, "workflows"), "compras", nil)

	request := parsed(t, machineArgs(home, "apply", "manifest="+filepath.Join(home, "generated.json")))
	accepted, lines, err := request.Prepare(installed())
	if err != nil {
		t.Fatalf("Prepare = %v, want the two colliders rejected and the rest registered", err)
	}

	if len(accepted) != 1 || accepted[0].Name != "compras" {
		t.Fatalf("Prepare accepted %+v, want only the workflow that collides with nothing", accepted)
	}
	if len(lines) != 3 {
		t.Fatalf("Prepare lines = %+v, want one per workflow found", lines)
	}
	if valid := lineFor(t, lines, "compras"); valid.Command != "alfred-compras" || valid.Reason != "" {
		t.Fatalf("the valid workflow lost its command to another workflow's collision: %+v", valid)
	}

	want := `the subagent name "alfred-ventas-extra-propuesta" is generated by both "ventas" ` +
		`and "ventas-extra"; neither workflow is registered`
	for _, name := range []string{"ventas", "ventas-extra"} {
		line := lineFor(t, lines, name)
		if line.Reason != want {
			t.Errorf("%s was rejected with %q, want %q", name, line.Reason, want)
		}
		if line.Command != "" {
			t.Errorf("%s was rejected and still carries command %q", name, line.Command)
		}
	}
}

// The same run through the report: two ordinary `rejected:` lines and the exit code a
// rejection already produces, rather than one scope-wide error that costs the valid
// workflows their commands.
func TestACollidingSubagentNameIsAnOrdinaryRejection(t *testing.T) {
	home := t.TempDir()
	ventasPhases(t, filepath.Join(home, "skills"))
	custom := filepath.Join(home, "custom", "workflows")
	ownPhaseWorkflow(t, custom, "ventas", "extra-propuesta")
	ownPhaseWorkflow(t, custom, "ventas-extra", "propuesta")

	request := parsed(t, machineArgs(home, "apply", "manifest="+filepath.Join(home, "generated.json")))
	_, lines, err := request.Prepare(installed())
	if err != nil {
		t.Fatalf("Prepare = %v", err)
	}

	report := &Report{Scope: ScopeMachine, Mode: ModeApply, Workflows: lines, Agents: []AgentLine{{
		Agent: "claude", Target: "/home/x/.claude",
	}}}
	text := report.String()
	if strings.Count(text, "rejected: ") != 2 {
		t.Fatalf("want one rejection line per colliding workflow:\n%s", text)
	}
	if !strings.Contains(text, "alfred-ventas-extra-propuesta") {
		t.Fatalf("the report does not name the generated name:\n%s", text)
	}
	if !report.Failed() {
		t.Fatalf("a run with two rejected workflows exits zero:\n%s", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if len(line) > reportWidth {
			t.Fatalf("a report line is %d columns wide:\n%s", len(line), line)
		}
	}
}

// A shared phase is deliberately one subagent however many workflows run it, so two
// workflows naming the same shared phase are not a collision.
func TestTwoWorkflowsSharingALibraryPhaseAreBothRegistered(t *testing.T) {
	home := t.TempDir()
	ventasPhases(t, filepath.Join(home, "skills"))
	ventasWorkflow(t, filepath.Join(home, "workflows"), "ventas", nil)
	ventasWorkflow(t, filepath.Join(home, "custom", "workflows"), "compras", nil)

	request := parsed(t, machineArgs(home, "apply", "manifest="+filepath.Join(home, "generated.json")))
	accepted, lines, err := request.Prepare(installed())
	if err != nil {
		t.Fatalf("Prepare = %v", err)
	}
	if len(accepted) != 2 {
		t.Fatalf("Prepare accepted %+v, want both: the shared review is one subagent", accepted)
	}
	for _, line := range lines {
		if line.Reason != "" {
			t.Fatalf("%s was rejected for sharing a library phase: %q", line.Name, line.Reason)
		}
	}
}

// A phase that resolves in no root of the scope rejects the workflow where the user is
// looking at it, rather than three steps into a change.
func TestPrepareRejectsAWorkflowWhosePhaseResolvesNowhere(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, "skills"), "review")
	// The definition with none of the phases it brings itself, so `prospectar` reaches no
	// root at all.
	writeWorkflow(t, filepath.Join(home, "workflows"), "ventas", nil)

	request := parsed(t, machineArgs(home, "apply", "manifest="+filepath.Join(home, "generated.json")))
	accepted, lines, err := request.Prepare(installed())
	if err != nil {
		t.Fatalf("Prepare = %v", err)
	}
	if len(accepted) != 0 {
		t.Fatalf("Prepare accepted %+v, want none", accepted)
	}
	if !strings.Contains(lines[0].Reason, "resolves in no machine root") {
		t.Fatalf("Prepare reason = %q", lines[0].Reason)
	}
}

// A model on a phase the installation owns is refused, and the refusal arrives in the
// report beside every other reason: the user is told which phase and which model is in
// force, rather than having their declaration quietly overruled.
func TestPrepareRejectsAModelDeclaredOnASharedPhase(t *testing.T) {
	home := t.TempDir()
	ventasPhases(t, filepath.Join(home, "skills"))
	ventasWorkflow(t, filepath.Join(home, "workflows"), "ventas", func(d map[string]any) {
		phases := d["phases"].([]any)
		phases[len(phases)-1] = map[string]any{
			"name": "review", "model": "anthropic/claude-opus-5",
		}
	})

	request := parsed(t, machineArgs(home, "apply", "manifest="+filepath.Join(home, "generated.json")))
	accepted, lines, err := request.Prepare(installed())
	if err != nil {
		t.Fatalf("Prepare = %v", err)
	}
	if len(accepted) != 0 {
		t.Fatalf("Prepare accepted %+v, want none", accepted)
	}

	want := `phase "review" declares a model and resolves to the shared library, where the ` +
		`model is the installation's assignment in profile.json`
	if len(lines) != 1 || lines[0].Reason != want {
		t.Fatalf("Prepare lines = %+v, want the rejection %q", lines, want)
	}
	if lines[0].Command != "" {
		t.Fatalf("a rejected workflow carries command %q, want none", lines[0].Command)
	}

	report := &Report{Scope: ScopeMachine, Mode: ModeApply, Workflows: lines}
	if !strings.Contains(report.String(), "rejected: ") || !report.Failed() {
		t.Fatalf("the report does not refuse the workflow:\n%s", report.String())
	}
}

// The other half of the same rule, and it is tested here rather than against resolve alone
// because `Prepare` is where a user can observe the resolution at all: a test that asks the
// resolver whether it refuses the declaration does not say that nothing downstream resolved
// anyway, which is how the model half of this went unnoticed for two rounds.
func TestPrepareRejectsAToolSetDeclaredOnASharedPhase(t *testing.T) {
	home := t.TempDir()
	ventasPhases(t, filepath.Join(home, "skills"))
	ventasWorkflow(t, filepath.Join(home, "workflows"), "ventas", func(d map[string]any) {
		phases := d["phases"].([]any)
		phases[len(phases)-1] = map[string]any{
			"name": "review", "tools": []any{"Read", "WebFetch"},
		}
	})

	request := parsed(t, machineArgs(home, "apply", "manifest="+filepath.Join(home, "generated.json")))
	accepted, lines, err := request.Prepare(installed())
	if err != nil {
		t.Fatalf("Prepare = %v", err)
	}
	if len(accepted) != 0 {
		t.Fatalf("Prepare accepted %+v, want none", accepted)
	}

	want := `phase "review" declares a tool set and resolves to the shared library, where the ` +
		`tool set is the installation's assignment`
	if len(lines) != 1 || lines[0].Reason != want {
		t.Fatalf("Prepare lines = %+v, want the rejection %q", lines, want)
	}
	if lines[0].Command != "" {
		t.Fatalf("a rejected workflow carries command %q, want none", lines[0].Command)
	}

	report := &Report{Scope: ScopeMachine, Mode: ModeApply, Workflows: lines}
	if !strings.Contains(report.String(), "rejected: ") || !report.Failed() {
		t.Fatalf("the report does not refuse the workflow:\n%s", report.String())
	}
}

// And the accepting side, through the same call: a tool set on a phase the workflow brought
// itself reaches the result the generator reads, so the refusal above is about where the
// phase resolved and not about the key.
func TestPrepareKeepsAToolSetDeclaredOnAPhaseTheWorkflowBrings(t *testing.T) {
	home := t.TempDir()
	ventasPhases(t, filepath.Join(home, "skills"))
	ventasWorkflow(t, filepath.Join(home, "workflows"), "ventas", nil)

	request := parsed(t, machineArgs(home, "apply", "manifest="+filepath.Join(home, "generated.json")))
	accepted, _, err := request.Prepare(installed())
	if err != nil {
		t.Fatalf("Prepare = %v", err)
	}
	if len(accepted) != 1 {
		t.Fatalf("Prepare accepted %+v, want the workflow", accepted)
	}

	for _, phase := range accepted[0].Phases {
		if phase.Name != "propuesta" {
			continue
		}
		want := []string{"Read", "Write", "WebFetch"}
		if phase.Origin != FromWorkflow || !reflect.DeepEqual(phase.Tools, want) {
			t.Fatalf("propuesta = %+v, want its own tools %v", phase, want)
		}
		return
	}
	t.Fatalf("Prepare returned no propuesta: %+v", accepted[0].Phases)
}

func TestMachineNamesAreTheOnesAProjectRunCanCollideWith(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	ventasPhases(t, filepath.Join(home, "skills"))
	ventasWorkflow(t, filepath.Join(home, "workflows"), "ventas", nil)
	ventasWorkflow(t, filepath.Join(home, "custom", "workflows"), "compras", nil)
	// Rejected, so it has no command on this machine and nothing can collide with it.
	writeWorkflow(t, filepath.Join(home, "custom", "workflows"), "roto", func(d map[string]any) {
		d["closes"] = "nadie"
	})

	request := parsed(t, projectArgs(home, repo, "apply",
		"manifest="+filepath.Join(repo, ".alfred", "generated.json")))
	names, err := request.MachineNames()
	if err != nil {
		t.Fatalf("MachineNames = %v", err)
	}
	if want := []string{"compras", "ventas"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("MachineNames = %v, want %v", names, want)
	}
}

func TestMachineNamesAreEmptyAtMachineScope(t *testing.T) {
	home := t.TempDir()
	ventasPhases(t, filepath.Join(home, "skills"))
	ventasWorkflow(t, filepath.Join(home, "workflows"), "ventas", nil)

	request := parsed(t, machineArgs(home, "apply", "manifest="+filepath.Join(home, "generated.json")))
	names, err := request.MachineNames()
	if err != nil {
		t.Fatalf("MachineNames = %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("MachineNames = %v at machine scope, want none", names)
	}
}

// The manifest holds this machine's absolute paths, so it is excluded whatever the
// repository decided about the rest of what registration writes.
func TestRecordManifestExcludesTheManifestAlone(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	exclude := filepath.Join(repo, ".git", "info", "exclude")
	request := parsed(t, projectArgs(home, repo, "apply",
		"manifest="+filepath.Join(repo, ".alfred", "generated.json"),
		"root="+repo, "exclude-manifest="+exclude))

	paths, err := request.RecordManifest(true, true)
	if err != nil {
		t.Fatalf("RecordManifest = %v", err)
	}
	want := []string{filepath.Join(".alfred", "generated.json")}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("RecordManifest = %v, want %v", paths, want)
	}

	data, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatalf("reading the exclude file: %v", err)
	}
	if strings.TrimSpace(string(data)) != want[0] {
		t.Fatalf("the exclude file holds %q, want only the manifest", data)
	}
}

func TestRecordManifestWritesNothingWhenItIsNotApplying(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	exclude := filepath.Join(repo, ".git", "info", "exclude")
	request := parsed(t, projectArgs(home, repo, "report",
		"manifest="+filepath.Join(repo, ".alfred", "generated.json"),
		"root="+repo, "exclude-manifest="+exclude))

	paths, err := request.RecordManifest(true, false)
	if err != nil {
		t.Fatalf("RecordManifest = %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("RecordManifest = %v, want the path it would have recorded", paths)
	}
	if _, err := os.Stat(exclude); !os.IsNotExist(err) {
		t.Fatalf("the exclude file exists after a run that writes nothing")
	}
}

func TestRecordManifestDoesNothingWithoutTheManifestOnlyForm(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	request := parsed(t, projectArgs(home, repo, "apply",
		"manifest="+filepath.Join(repo, ".alfred", "generated.json"),
		"root="+repo, "exclude="+filepath.Join(repo, ".git", "info", "exclude")))

	paths, err := request.RecordManifest(true, true)
	if err != nil {
		t.Fatalf("RecordManifest = %v", err)
	}
	// The full form records every owned path, the manifest among them, and doing it twice
	// here would report the same path in two places.
	if len(paths) != 0 {
		t.Fatalf("RecordManifest = %v, want none under exclude=", paths)
	}
}

// A repository defining no workflow is left exactly as it was found, and that includes its
// exclude file. Registration claims no manifest there, so there is nothing to keep out of
// git, and a line written anyway is permanent: nothing Alfred does ever takes it back.
func TestRecordManifestRecordsNothingWhenNoManifestIsWritten(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	exclude := filepath.Join(repo, ".git", "info", "exclude")
	request := parsed(t, projectArgs(home, repo, "apply",
		"manifest="+filepath.Join(repo, ".alfred", "generated.json"),
		"root="+repo, "exclude-manifest="+exclude))

	paths, err := request.RecordManifest(false, true)
	if err != nil {
		t.Fatalf("RecordManifest = %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("RecordManifest = %v, want none when no manifest was written", paths)
	}
	if _, err := os.Stat(exclude); !os.IsNotExist(err) {
		t.Fatal("the exclude file gained a line for a manifest that was never written")
	}
}

// A root that exists and cannot be read is an error rather than no workflows, in the scan
// of this scope and in the read-only scan of the machine's.
func TestPrepareAndMachineNamesReportARootTheyCannotRead(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	shipped := filepath.Join(home, "workflows")
	if err := os.MkdirAll(shipped, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(shipped, 0o755) })
	if _, err := os.ReadDir(shipped); err == nil {
		t.Skip("this user can read a directory with no permissions")
	}

	machine := parsed(t, machineArgs(home, "apply", "manifest="+filepath.Join(home, "generated.json")))
	if _, _, err := machine.Prepare(nil); err == nil {
		t.Error("Prepare read an unreadable root as no workflows")
	}

	project := parsed(t, projectArgs(home, repo, "apply",
		"manifest="+filepath.Join(repo, ".alfred", "generated.json")))
	if _, err := project.MachineNames(); err == nil {
		t.Error("MachineNames read an unreadable root as no workflows")
	}
}

// Hard rule 18 is enforced in internal/generated, and the manifest-only form goes through
// the same door: a caller that points it at .gitignore is refused rather than obeyed.
func TestRecordManifestRefusesToWriteGitignore(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	request := parsed(t, projectArgs(home, repo, "apply",
		"manifest="+filepath.Join(repo, ".alfred", "generated.json"),
		"root="+repo, "exclude-manifest="+filepath.Join(repo, ".gitignore")))

	if _, err := request.RecordManifest(true, true); err == nil {
		t.Fatal("RecordManifest wrote to .gitignore")
	}
}

// A path outside the repository is left absolute: git matches nothing against a chain of
// `..`, so turning it into one would quietly exclude nothing.
func TestExcludeFileOutsideTheRepositoryKeepsItsPath(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	elsewhere := filepath.Join(home, "exclude")
	request := parsed(t, projectArgs(home, repo, "apply",
		"manifest="+filepath.Join(repo, ".alfred", "generated.json"),
		"root="+repo, "exclude="+elsewhere))

	if got := request.ExcludeFile(); got != elsewhere {
		t.Fatalf("ExcludeFile = %q, want the path as it was given", got)
	}

	// A root that is not absolute cannot be subtracted from one that is, and the answer is
	// the same: leave the path alone rather than invent a relation.
	relative := parsed(t, projectArgs(home, repo, "apply",
		"manifest="+filepath.Join(repo, ".alfred", "generated.json"),
		"root=somewhere", "exclude="+elsewhere))
	if got := relative.ExcludeFile(); got != elsewhere {
		t.Fatalf("ExcludeFile = %q, want the path as it was given", got)
	}
}

func TestExcludeFileIsNamedRelativeToTheRepository(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	manifest := "manifest=" + filepath.Join(repo, ".alfred", "generated.json")

	full := parsed(t, projectArgs(home, repo, "apply", manifest, "root="+repo,
		"exclude="+filepath.Join(repo, ".git", "info", "exclude")))
	if got, want := full.ExcludeFile(), filepath.Join(".git", "info", "exclude"); got != want {
		t.Fatalf("ExcludeFile = %q, want %q", got, want)
	}

	only := parsed(t, projectArgs(home, repo, "apply", manifest, "root="+repo,
		"exclude-manifest="+filepath.Join(repo, ".git", "info", "exclude")))
	if got, want := only.ExcludeFile(), filepath.Join(".git", "info", "exclude"); got != want {
		t.Fatalf("ExcludeFile = %q, want %q", got, want)
	}

	none := parsed(t, machineArgs(home, "apply", "manifest="+filepath.Join(home, "generated.json")))
	if got := none.ExcludeFile(); got != "" {
		t.Fatalf("ExcludeFile = %q at machine scope, want none", got)
	}
}

func registeredReport() *Report {
	return &Report{
		Scope: ScopeMachine,
		Mode:  ModeApply,
		Workflows: []Line{{
			Name: "sdd", Source: "machine/shipped", Phases: 10, Command: "alfred-sdd",
			Routes: []string{"direct", "full", "pipeline"},
		}},
		Agents: []AgentLine{{
			Agent: "claude", Target: "/home/x/.claude",
			Added:     []string{"/alfred-sdd", "alfred-spec"},
			Unchanged: Counts{Commands: 1, Agents: 2},
		}},
	}
}

func TestReportNamesEveryWorkflowWithItsSourceAndItsRoutes(t *testing.T) {
	text := registeredReport().String()

	for _, want := range []string{
		"workflows",
		"sdd",
		"machine/shipped",
		"10 phases",
		"routes: direct, full, pipeline",
		"claude code",
		"(machine: /home/x/.claude)",
		"added",
		"/alfred-sdd, alfred-spec",
		"unchanged  1 command, 2 agents",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("the report does not carry %q:\n%s", want, text)
		}
	}
	if registeredReport().Failed() {
		t.Fatalf("a run where everything registered failed:\n%s", text)
	}
}

// Nothing a definition carries can open a line of the report. The report is a block of
// aligned lines a user reads to decide whether their machine is in the state they think it
// is, and its whole structure is two characters: a newline between rows and two spaces
// between columns. A directory named with a newline — git stores one, so a clone creates
// one — would otherwise write a row of the author's choosing into that block.
func TestReportRendersAValueThatCannotOpenALineOfItsOwn(t *testing.T) {
	report := registeredReport()
	report.Workflows = append(report.Workflows, Line{
		Name:    "evil\n  forged  machine/shipped  9 phases  routes: mine",
		Source:  "machine/custom",
		Phases:  1,
		Routes:  []string{"ok", "bad\n  second  machine/custom  1 phases  routes: no"},
		Command: "alfred-evil",
	})
	report.Unassigned = []Unassigned{{
		Workflow: "evil", Phase: "step\n  third  no model at all", Reason: "nobody assigned one",
	}}

	text := report.String()
	for _, forged := range []string{"  forged", "  second", "  third"} {
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, forged) {
				t.Fatalf("a value forged the line %q:\n%s", line, text)
			}
		}
	}
	for _, want := range []string{`"evil\n`, `"bad\n`, `"evil/step\n`} {
		if !strings.Contains(text, want) {
			t.Fatalf("the report does not render %s as one quoted value:\n%s", want, text)
		}
	}
}

// The column separator is the other half of the block's structure, and a value carrying one
// rewrites the columns without ever starting a line. A value that merely begins or ends
// with a space carries one too, because the separator is appended to it: `"sdd "` followed
// by the two the layout adds is three, which is no longer one column.
func TestReportQuotesAValueCarryingTheColumnSeparator(t *testing.T) {
	for _, name := range []string{"sdd  machine/custom", " sdd", "sdd ", "sdd\tmachine/custom"} {
		report := registeredReport()
		report.Workflows[0].Name = name

		text := report.String()
		if !strings.Contains(text, strconv.Quote(name)) {
			t.Fatalf("the name %q is not quoted in the report:\n%s", name, text)
		}
	}
}

// And the other direction, because quoting everything would be the same defect read from
// the other side: an ordinary value is printed exactly as it was written, with no quotes
// and no escaping, which is every value anybody actually has.
func TestReportLeavesAnOrdinaryValueExactlyAsItWasWritten(t *testing.T) {
	report := registeredReport()
	report.Workflows = append(report.Workflows, Line{
		Name: "ventas", Source: "machine/shipped, machine/custom",
		Reason: "exists in both roots",
	})

	text := report.String()
	for _, want := range []string{
		"  sdd  machine/shipped",
		"routes: direct, full, pipeline",
		"machine/shipped, machine/custom",
	} {
		if !strings.Contains(collapse(text), collapse(want)) {
			t.Fatalf("the report does not carry %q unchanged:\n%s", want, text)
		}
	}
	if strings.Contains(text, `"`) {
		t.Fatalf("an ordinary value was quoted:\n%s", text)
	}
}

// A second run changes nothing: nothing is written and the report says so, per agent.
func TestReportSaysPerAgentThatNothingChanged(t *testing.T) {
	report := registeredReport()
	report.Agents[0].Added = nil
	report.Agents[0].Unchanged = Counts{Commands: 2, Agents: 11}

	text := report.String()
	if !strings.Contains(text, "nothing added, updated or removed") {
		t.Fatalf("an untouched agent is not reported as untouched:\n%s", text)
	}
	if !strings.Contains(text, "unchanged  2 commands, 11 agents") {
		t.Fatalf("the unchanged count is wrong:\n%s", text)
	}
}

// A workflow was edited: its command is listed as updated and the rest as unchanged.
func TestReportSeparatesWhatWasUpdatedFromWhatWasNot(t *testing.T) {
	report := registeredReport()
	report.Agents[0].Added = nil
	report.Agents[0].Updated = []string{"/alfred-sdd"}
	report.Agents[0].Removed = []string{"alfred-ventas-propuesta"}

	text := report.String()
	if !strings.Contains(text, "updated    /alfred-sdd") {
		t.Fatalf("the edited command is not reported as updated:\n%s", text)
	}
	if !strings.Contains(text, "removed    alfred-ventas-propuesta") {
		t.Fatalf("the removal is not reported:\n%s", text)
	}
	if strings.Contains(text, "nothing added, updated or removed") {
		t.Fatalf("a run that changed something says it changed nothing:\n%s", text)
	}
}

// Report-only does not hide a rejection, and it exits as apply would have.
func TestReportFailsOnARejectionInEveryMode(t *testing.T) {
	for _, mode := range []Mode{ModeApply, ModeReport, ModeCheck} {
		report := registeredReport()
		report.Mode = mode
		report.Workflows = append(report.Workflows, Line{
			Name: "roto", Source: "machine/custom",
			Reason: `route "rapida" names phase "propuest", which it does not declare`,
		})

		text := report.String()
		if !strings.Contains(text, "rejected: ") || !strings.Contains(text, "propuest") {
			t.Fatalf("mode %s hides the rejection:\n%s", mode, text)
		}
		if !report.Failed() {
			t.Fatalf("mode %s exits zero with a rejected workflow", mode)
		}
	}
}

// A long reason is wrapped under the column it starts in, so a report stays readable at the
// width the rest of this repository's output is written to.
func TestReportWrapsALongRejectionReason(t *testing.T) {
	report := registeredReport()
	report.Workflows = append(report.Workflows, Line{
		Name: "antiguo", Source: "machine/custom",
		Reason: `phase "cotizar" resolves in no machine root; a phase only a repository ` +
			`provides belongs to a repository-scope workflow`,
	})

	for _, line := range strings.Split(report.String(), "\n") {
		if len(line) > 90 {
			t.Fatalf("a report line is %d columns wide:\n%s", len(line), line)
		}
	}
}

// Report-only on a machine with pending changes states that the command would be added, and
// exits as apply would have, which for a tree with nothing wrong is zero.
func TestReportOnlyExitsAsApplyWouldHave(t *testing.T) {
	report := registeredReport()
	report.Mode = ModeReport

	if !strings.Contains(report.String(), "/alfred-sdd") {
		t.Fatalf("report-only does not name the command that would be added")
	}
	if report.Failed() {
		t.Fatalf("report-only on a sound tree exits non-zero")
	}
}

// No supported agent was detected: nothing was written, and that is a failure rather than a
// success over an empty list.
func TestReportFailsWhenNoAgentWasDetected(t *testing.T) {
	report := registeredReport()
	report.Agents = nil

	text := report.String()
	if !strings.Contains(text, "no supported agent was detected") {
		t.Fatalf("the report does not say no agent was detected:\n%s", text)
	}
	if !report.Failed() {
		t.Fatalf("a run that wrote nothing at all exits zero")
	}
}

// A hand-written workflow with no models: every phase without one is named, and the run
// exits non-zero because a phase nobody assigned a model to will not be dispatched.
func TestReportNamesEveryPhaseWithoutAModel(t *testing.T) {
	report := registeredReport()
	report.Unassigned = []Unassigned{{
		Workflow: "ventas", Phase: "review",
		Reason: "the workflow's own review declares no model and will not be dispatched",
	}}

	text := report.String()
	if !strings.Contains(text, "without a model") {
		t.Fatalf("the report has no section for a phase with no model:\n%s", text)
	}
	if !strings.Contains(text, "ventas/review") {
		t.Fatalf("the phase is not named:\n%s", text)
	}
	if !strings.Contains(text, "declares no model and will not be dispatched") {
		t.Fatalf("the reason is not printed verbatim:\n%s", text)
	}
	if !report.Failed() {
		t.Fatalf("a phase with no model exits zero")
	}
}

func TestReportCountsTheExcludedPaths(t *testing.T) {
	report := registeredReport()
	report.Scope = ScopeProject
	report.ExcludeFile = ".git/info/exclude"
	report.Excluded = []string{".alfred/generated.json", ".claude/commands/alfred-ventas.md"}

	text := report.String()
	if !strings.Contains(text, "2 paths recorded in .git/info/exclude") {
		t.Fatalf("the excluded count is missing:\n%s", text)
	}
	if !strings.Contains(text, "(project: /home/x/.claude)") {
		t.Fatalf("the agent header does not name the scope:\n%s", text)
	}
}

// A workflow created by hand and never registered: check names the workflow and the agent
// the command is missing on, says what fixes it, and exits non-zero.
func TestCheckNamesEveryWorkflowWithoutACommand(t *testing.T) {
	report := registeredReport()
	report.Mode = ModeCheck
	report.Scope = ScopeProject
	report.Agents = []AgentLine{
		{Agent: "claude", Target: "/repo/.claude", Added: []string{"/alfred-sdd"}},
		{Agent: "opencode", Target: "/repo/opencode.json", Unchanged: Counts{Commands: 1}},
	}

	text := report.String()
	for _, want := range []string{
		"missing",
		"sdd",
		"claude code",
		"/alfred-sdd",
		"alfred workflows project apply",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("check does not carry %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "added") {
		t.Fatalf("check reports an addition it did not make:\n%s", text)
	}
	if !report.Failed() {
		t.Fatalf("check exits zero with a command missing")
	}
}

// Everything registered: check reports that every workflow has its command and exits zero.
func TestCheckPassesWhenEveryCommandIsThere(t *testing.T) {
	report := registeredReport()
	report.Mode = ModeCheck
	report.Agents = []AgentLine{{
		Agent: "claude", Target: "/home/x/.claude", Unchanged: Counts{Commands: 1, Agents: 2},
	}}

	text := report.String()
	if !strings.Contains(text, "every workflow of this scope has its command") {
		t.Fatalf("check does not report a sound machine:\n%s", text)
	}
	if report.Failed() {
		t.Fatalf("check exits non-zero with nothing missing:\n%s", text)
	}
}

// A rejected workflow has no command and never will until it is fixed, so check does not
// also report it as missing: that would be one fault reported as two.
func TestCheckIgnoresAWorkflowThatWasRejected(t *testing.T) {
	report := registeredReport()
	report.Mode = ModeCheck
	report.Workflows = []Line{{Name: "roto", Source: "machine/custom", Reason: "declares no routes"}}
	report.Agents = []AgentLine{{Agent: "claude", Target: "/home/x/.claude"}}

	if strings.Contains(report.String(), "missing\n  roto") {
		t.Fatalf("check reports a rejected workflow as a missing command:\n%s", report.String())
	}
}

func TestReportMarksARepositoryWorkflowThatOverridesAMachineOne(t *testing.T) {
	report := registeredReport()
	report.Scope = ScopeProject
	report.Workflows = []Line{{
		Name: "ventas", Source: "project", Phases: 4, Command: "alfred-ventas",
		Routes: []string{"completa", "rapida"},
	}}
	report.Override("ventas", "alfred-ventas-local", "alfred-ventas")

	text := report.String()
	if !strings.Contains(text, "overrides the machine's ventas in this repository") {
		t.Fatalf("the override is not reported:\n%s", text)
	}
	// Both commands, each said to reach a copy. The reader is told which is which rather
	// than given one and left to infer the other.
	want := "use /alfred-ventas-local here, /alfred-ventas for the machine's"
	if !strings.Contains(text, want) {
		t.Fatalf("the report does not name both commands:\n%s", text)
	}
	if report.Workflows[0].Command != "alfred-ventas-local" {
		t.Fatalf("Override left the command at %q", report.Workflows[0].Command)
	}
	if report.Workflows[0].Machine != "alfred-ventas" {
		t.Fatalf("Override left the machine's command at %q", report.Workflows[0].Machine)
	}
}

func TestReportStatesWhenAScopeDefinesNoWorkflow(t *testing.T) {
	report := registeredReport()
	report.Scope = ScopeProject
	report.Workflows = nil

	text := report.String()
	if !strings.Contains(text, "no workflow") {
		t.Fatalf("a scope with nothing to register says nothing:\n%s", text)
	}
	if report.Failed() {
		t.Fatalf("a repository that defines no workflow exits non-zero:\n%s", text)
	}
}

func TestReportNamesEachAgentTheWayTheUserDoes(t *testing.T) {
	report := registeredReport()
	report.Agents = append(report.Agents, AgentLine{
		Agent: "opencode", Target: "/home/x/.config/opencode/opencode.json",
		Added: []string{"alfred-sdd"},
	})

	text := report.String()
	if !strings.Contains(text, "opencode  (machine: /home/x/.config/opencode/opencode.json)") {
		t.Fatalf("the opencode section is missing or mis-headed:\n%s", text)
	}
}

// A long reason under `without a model` is wrapped under its own column, the same way a
// rejection is.
func TestReportWrapsALongReasonForAPhaseWithNoModel(t *testing.T) {
	report := registeredReport()
	report.Unassigned = []Unassigned{{
		Workflow: "ventas", Phase: "seguimiento",
		Reason: "the workflow's own seguimiento declares no model and will not be " +
			"dispatched, so every route that reaches it stops there and names it",
	}}

	for _, line := range strings.Split(report.String(), "\n") {
		if len(line) > reportWidth {
			t.Fatalf("a report line is %d columns wide:\n%s", len(line), line)
		}
	}
}

// A word longer than the width is left whole: half a path is unusable, and an over-long
// line is only ugly.
func TestWrapKeepsALongWordWhole(t *testing.T) {
	path := "/Users/someone/.claude/commands/alfred-a-very-long-workflow-name-indeed.md"
	lines := wrap("the file "+path+" was left alone", 1, 1)

	// A width below the floor is read as the floor, so the short words still group.
	if len(lines) != 3 {
		t.Fatalf("wrap = %q, want the long word on a line of its own", lines)
	}
	if !contains(lines, path) {
		t.Fatalf("wrap broke the path: %q", lines)
	}
}

func TestReportKeepsWhatItWillNotRemove(t *testing.T) {
	report := registeredReport()
	report.Agents[0].Modified = []string{"/home/x/.claude/commands/alfred-sdd.md"}
	report.Agents[0].Orphans = []string{"/home/x/.claude/agents/alfred-init.md"}
	report.Agents[0].Collisions = []string{"/home/x/.claude/agents/alfred-spec.md"}

	text := report.String()
	for _, want := range []string{"modified", "orphans", "collisions", "will not remove"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the report does not carry %q:\n%s", want, text)
		}
	}
}

// A collision is the one of the three whose consequence has a name. Saying only that a name
// clashed leaves the user to work out what they lost, so the line names the command or
// subagent that is now missing, on which agent, and what to do about it.
func TestACollisionNamesWhatIsMissingAndOnWhichAgent(t *testing.T) {
	report := registeredReport()
	report.Agents[0].Collisions = []string{
		"/home/x/.claude/commands/alfred-cobranzas.md",
		"/home/x/.claude/agents/alfred-spec.md",
	}
	report.Agents = append(report.Agents, AgentLine{
		Agent: "opencode", Target: "/home/x/.config/opencode/opencode.json",
		Collisions: []string{"alfred-ventas"},
	})

	text := collapse(report.String())
	for _, want := range []string{
		"collisions /home/x/.claude/commands/alfred-cobranzas.md",
		"not written: a file Alfred never generated already carries this name, " +
			"so /alfred-cobranzas is missing on claude code; rename or remove it",
		"/home/x/.claude/agents/alfred-spec.md",
		"not written: a file Alfred never generated already carries this name, " +
			"so alfred-spec is missing on claude code; rename or remove it",
		// A key in opencode.json is not a file, and Alfred does generate that file.
		"not written: an agent Alfred never generated already carries this name, " +
			"so alfred-ventas is missing on opencode; rename or remove it",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("the report does not carry %q:\n%s", want, report.String())
		}
	}

	// One sentence per collision, not one shared by the list.
	if got := strings.Count(text, "rename or remove it"); got != 3 {
		t.Fatalf("the report carries %d collision sentences, want one per collision:\n%s",
			got, report.String())
	}
	for _, line := range strings.Split(report.String(), "\n") {
		if len(line) > reportWidth {
			t.Fatalf("a report line is %d columns wide:\n%s", len(line), line)
		}
	}
}

// collapse folds a wrapped report back into single-spaced text, so a sentence can be
// asserted whole instead of being re-split the way the renderer happened to wrap it.
func collapse(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// A modified file and an orphan are things to look at. A collision is not: the generated
// content for that name was not written, so a workflow of this scope has no command on that
// agent, which is the fault check would report as missing on the next run.
func TestACollisionFailsTheRunAndTheOtherTwoDoNot(t *testing.T) {
	looked := registeredReport()
	looked.Agents[0].Modified = []string{"/home/x/.claude/commands/alfred-sdd.md"}
	looked.Agents[0].Orphans = []string{"/home/x/.claude/agents/alfred-init.md"}
	if looked.Failed() {
		t.Fatalf("a report with a modified file and an orphan exits non-zero:\n%s", looked.String())
	}

	collided := registeredReport()
	collided.Agents[0].Collisions = []string{"/home/x/.claude/commands/alfred-ventas.md"}
	if !collided.Failed() {
		t.Fatalf("a run that skipped a command for a collision exits zero:\n%s", collided.String())
	}
}

// An upgrade from a version that recorded nothing: the report names what it adopted once,
// as provenance beside the actions taken on the same files, and the run does not fail for
// having done what it was meant to do.
func TestReportNamesWhatAnEarlierVersionWroteOnce(t *testing.T) {
	report := registeredReport()
	report.Agents[0].Updated = []string{"/alfred", "alfred-spec"}
	report.Adopted = Adopted{
		Files: []string{
			"/home/x/.claude/agents/alfred-spec.md",
			"/home/x/.claude/commands/alfred.md",
		},
		Keys: []string{"alfred-manage"},
	}

	text := report.String()
	for _, want := range []string{
		"adopted      2 files and 1 agent written by an earlier version",
		"recorded in the manifest",
		"This happens once.",
		"/home/x/.claude/commands/alfred.md",
		"alfred-manage",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("the report does not carry %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "written by an earlier version") != 1 {
		t.Fatalf("the adoption is stated more than once:\n%s", text)
	}
	if report.Failed() {
		t.Fatalf("an upgrade exits non-zero for adopting its own output:\n%s", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if len(line) > reportWidth {
			t.Fatalf("a report line is %d columns wide:\n%s", len(line), line)
		}
	}
}

// Report and check write no manifest, so they say what applying would adopt rather than
// claiming an adoption that did not happen.
func TestReportSaysWhatItWouldAdoptWhenItWritesNothing(t *testing.T) {
	for _, mode := range []Mode{ModeReport, ModeCheck} {
		report := registeredReport()
		report.Mode = mode
		report.Adopted = Adopted{Files: []string{"/home/x/.claude/commands/alfred.md"}}

		text := report.String()
		if !strings.Contains(text, "would adopt  1 file written by an earlier version") {
			t.Fatalf("mode %s does not say what it would adopt:\n%s", mode, text)
		}
		if strings.Contains(text, "adopted") {
			t.Fatalf("mode %s claims an adoption it did not make:\n%s", mode, text)
		}
	}
}

// On the next run of that scope the manifest exists, nothing is adopted and the section is
// absent: a one-time migration that keeps announcing itself is not one.
func TestReportSaysNothingAboutAdoptionOnAScopeThatHasAManifest(t *testing.T) {
	if text := registeredReport().String(); strings.Contains(text, "adopt") {
		t.Fatalf("a scope with a manifest reports an adoption:\n%s", text)
	}
}
