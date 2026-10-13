// Package golden pins the exact bytes the helper produces. To re-record, see testdata/README.md.
package golden

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

var (
	repoRoot string
	binary   string
)

// spec is the input every golden file was produced from.
type spec struct {
	Payload     string            `json:"payload"`
	Version     string            `json:"version"`
	Files       map[string]string `json:"files"`
	HomeEdits   map[string]string `json:"home_edits"`
	SourceEdits map[string]string `json:"source_edits"`
	Workflows   roots             `json:"workflows"`
	Project     repository        `json:"project"`
}

// repository names the project-scope fixture and the workflow the removal case deletes.
type repository struct {
	Repo    string `json:"repo"`
	Removes string `json:"removes"`
}

// roots names the machine-scope fixture roots. Absent is a missing root: no workflows.
type roots struct {
	Shipped        string `json:"shipped"`
	Custom         string `json:"custom"`
	Rejected       string `json:"rejected"`
	RejectedCustom string `json:"rejected_custom"`
	Injection      string `json:"injection"`
	Absent         string `json:"absent"`
}

func TestMain(m *testing.M) {
	os.Exit(runSuite(m))
}

func runSuite(m *testing.M) int {
	root, err := filepath.Abs("../..")
	if err != nil {
		panic(err)
	}
	repoRoot = root

	dir, err := os.MkdirTemp("", "alfred-golden")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	binary = filepath.Join(dir, "alfred")
	build := exec.CommandContext(context.Background(), "go", "build", "-o", binary, "./cmd/alfred")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building the binary failed: %v\n%s", err, out)
		return 1
	}

	return m.Run()
}

func loadSpec(t *testing.T) spec {
	t.Helper()

	data, err := os.ReadFile("testdata/fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	var s spec
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// build writes a set of files under root, creating directories as needed.
func build(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func run(t *testing.T, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), binary, args...)
	cmd.Dir = repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("alfred %s: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String()
}

// outcome is everything a run leaves behind, including how it ended.
type outcome struct {
	stdout string
	stderr string
	code   int
}

// attempt runs the helper and returns its exit code instead of failing the test.
func attempt(t *testing.T, args ...string) outcome {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), binary, args...)
	cmd.Dir = repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// A process that never started has no exit code to record.
	var exit *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exit) {
		t.Fatalf("alfred %s could not be run: %v\nstderr: %s",
			strings.Join(args, " "), err, stderr.String())
	}

	return outcome{
		stdout: stdout.String(),
		stderr: stderr.String(),
		code:   cmd.ProcessState.ExitCode(),
	}
}

func golden(t *testing.T, rel string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", rel))
	if err != nil {
		t.Fatalf("missing golden file %s: %v", rel, err)
	}
	return string(data)
}

// assertMatches compares against a recorded file and points at the first differing line.
func assertMatches(t *testing.T, what, want, got string) {
	t.Helper()

	if want == got {
		return
	}

	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			t.Errorf("%s differs from the recorded output\n  line %d\n  recorded: %q\n  now:      %q",
				what, i+1, w, g)
			return
		}
	}
	t.Errorf("%s differs only in trailing bytes", what)
}

func TestStateWriteMatchesGolden(t *testing.T) {
	s := loadSpec(t)

	home := t.TempDir()
	build(t, home, s.Files)

	out := run(t, "state", "write", home, s.Version, s.Payload)
	assertMatches(t, "state write stdout", golden(t, "state/write.stdout"), out)

	data, err := os.ReadFile(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	assertMatches(t, "state.json", golden(t, "state/write.json"), string(data))
}

func TestStateCompareMatchesGolden(t *testing.T) {
	s := loadSpec(t)

	source, home := t.TempDir(), t.TempDir()
	build(t, source, s.Files)
	build(t, home, s.Files)

	run(t, "state", "write", home, s.Version, s.Payload)

	// One file of every classification: edited by the user, changed upstream, added.
	build(t, home, s.HomeEdits)
	build(t, source, s.SourceEdits)

	out := run(t, "state", "compare", home, source, s.Payload)
	assertMatches(t, "compare report", golden(t, "state/compare.json"), out)

	// Name the classifications too, so a recording made from a wrong report still fails.
	var report map[string][]string
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	for class, want := range map[string]string{
		"modified":  "pay/a/b.txt",
		"updatable": "pay/a-c/d.txt",
		"new":       "pay/added.txt",
	} {
		if len(report[class]) != 1 || report[class][0] != want {
			t.Errorf("%s = %v, want [%s]", class, report[class], want)
		}
	}

	// The same source against a home with no state file.
	out = run(t, "state", "compare", t.TempDir(), source, s.Payload)
	assertMatches(t, "compare without a state file", golden(t, "state/compare-no-state.json"), out)
}

// Placeholders for what differs per machine: the staging tree, this checkout and hashes.
const (
	workToken   = "<work>"
	repoToken   = "<alfred>"
	digestToken = "<sha256>"
)

// digest matches manifest hashes, which depend on machine paths.
var digest = regexp.MustCompile(`[0-9a-f]{64}`)

// registration is one machine-scope run over fixture roots staged in a temp tree.
type registration struct {
	work    string
	profile string
	shipped string
	custom  string
}

func newRegistration(t *testing.T, profile, shipped, custom string) *registration {
	t.Helper()

	r := &registration{
		work:    t.TempDir(),
		profile: filepath.Join(repoRoot, "internal/golden/testdata/profiles", profile+".json"),
	}
	r.shipped = stage(t, shipped, filepath.Join(r.work, "roots/shipped"))
	r.custom = stage(t, custom, filepath.Join(r.work, "roots/custom"))
	return r
}

// stage copies a fixture root into the run's tree; a missing root stages as nothing.
func stage(t *testing.T, from, to string) string {
	t.Helper()

	source := filepath.Join("testdata", from)
	if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
		return to
	}
	if err := os.CopyFS(to, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	return to
}

// out holds everything a run writes: both agents' targets and the manifest.
func (r *registration) out() string { return filepath.Join(r.work, "out") }

// run is one machine-scope registration in the given mode.
func (r *registration) run(t *testing.T, mode string) outcome {
	t.Helper()

	return attempt(t, "workflows", "machine", mode,
		r.profile,
		filepath.Join(repoRoot, "skills"),
		r.shipped,
		r.custom,
		filepath.Join(repoRoot, "templates/workflow"),
		"manifest="+filepath.Join(r.out(), "generated.json"),
		"claude="+filepath.Join(r.out(), "claude"),
		"opencode="+filepath.Join(r.out(), "opencode.json"))
}

func (r *registration) normalise(text string) string { return normalise(r.work, text) }

// normalise replaces what differs per machine: the staging tree, this checkout and hashes.
func normalise(work, text string) string {
	text = strings.ReplaceAll(text, work, workToken)
	text = strings.ReplaceAll(text, repoRoot, repoToken)
	return digest.ReplaceAllString(text, digestToken)
}

// recorded formats a run as stored: exit code, stdout, and stderr if any.
func recorded(o outcome) string {
	text := fmt.Sprintf("exit %d\n\n--- stdout\n%s", o.code, o.stdout)
	if o.stderr != "" {
		text += "\n--- stderr\n" + o.stderr
	}
	return text
}

// assertReport compares a run against agents/<name>.report.
func (r *registration) assertReport(t *testing.T, name string, o outcome) {
	t.Helper()
	assertRecorded(t, "agents/"+name+".report", r.work, o)
}

// assertRecorded compares a run against the recording at rel.
func assertRecorded(t *testing.T, rel, work string, o outcome) {
	t.Helper()
	assertMatches(t, rel, golden(t, rel), normalise(work, recorded(o)))
}

// assertTree compares every file a run wrote, including added or missing ones.
func (r *registration) assertTree(t *testing.T, name string) {
	t.Helper()

	got := map[string]string{}
	for path, body := range snapshot(t, r.out()) {
		got[path] = r.normalise(body)
	}
	assertFiles(t, name, snapshot(t, filepath.Join("testdata/agents", name)), got)
}

// assertRecordedFiles compares only the files recorded for a case.
func (r *registration) assertRecordedFiles(t *testing.T, name string) {
	t.Helper()

	for path, want := range snapshot(t, filepath.Join("testdata/agents", name)) {
		assertMatches(t, name+"/"+path, want, r.normalise(readFile(t, filepath.Join(r.out(), path))))
	}
}

// assertFiles compares a set of files against the set that was recorded for it.
func assertFiles(t *testing.T, name string, want, got map[string]string) {
	t.Helper()

	names := map[string]bool{}
	for n := range want {
		names[n] = true
	}
	for n := range got {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	if len(sorted) == 0 {
		t.Fatal("nothing was generated and nothing was recorded")
	}

	for _, n := range sorted {
		w, isRecorded := want[n]
		g, produced := got[n]
		switch {
		case !isRecorded:
			t.Errorf("%s is generated now but was not recorded", n)
		case !produced:
			t.Errorf("%s was recorded but is no longer generated", n)
		default:
			assertMatches(t, name+"/"+n, w, g)
		}
	}
}

// TestRegisterMachineMatchesGolden records each profile's report and its generated files.
func TestRegisterMachineMatchesGolden(t *testing.T) {
	s := loadSpec(t)

	for _, name := range []string{"every-option", "minimal", "uniform"} {
		t.Run(name, func(t *testing.T) {
			r := newRegistration(t, name, s.Workflows.Shipped, s.Workflows.Custom)

			r.assertReport(t, name, r.run(t, "apply"))
			switch name {
			case "every-option":
				r.assertTree(t, name)
			case "minimal":
				r.assertRecordedFiles(t, name)
			}
		})
	}
}

// TestRegisterWithNoWorkflowMatchesGolden: with no workflows, the base commands are still written.
func TestRegisterWithNoWorkflowMatchesGolden(t *testing.T) {
	s := loadSpec(t)

	r := newRegistration(t, "uniform", s.Workflows.Absent, s.Workflows.Absent)

	r.assertReport(t, "no-workflow", r.run(t, "apply"))
	if _, err := os.Stat(filepath.Join(r.out(), "claude/commands/alfred.md")); err != nil {
		t.Errorf("the /alfred stub was not written: %v", err)
	}
}

// TestRejectedWorkflowsMatchGolden pins how each stage's refusal reaches the report.
func TestRejectedWorkflowsMatchGolden(t *testing.T) {
	s := loadSpec(t)

	r := newRegistration(t, "uniform", s.Workflows.Rejected, s.Workflows.RejectedCustom)

	o := r.run(t, "report")
	r.assertReport(t, "rejected", o)

	// The forged name must appear quoted, and never raw.
	if !strings.Contains(o.stdout, strconv.Quote(forgedName)) {
		t.Errorf("the report does not name the workflow at all:\n%s", o.stdout)
	}
	if bare := strings.ReplaceAll(o.stdout, strconv.Quote(forgedName), ""); strings.Contains(bare, forgedName) {
		t.Errorf("the report prints a directory name carrying the column separator as it stands:\n%s", o.stdout)
	}

	if wrote := snapshot(t, r.out()); len(wrote) > 0 {
		t.Errorf("report-only wrote %d files", len(wrote))
	}
}

// forgedName holds the report's column separator (two spaces) and must be printed quoted.
const forgedName = "forged  row"

// TestADescriptionCarryingANewlineStaysOneScalar: a newline must not add frontmatter keys.
func TestADescriptionCarryingANewlineStaysOneScalar(t *testing.T) {
	s := loadSpec(t)

	r := newRegistration(t, "uniform", s.Workflows.Injection, s.Workflows.Absent)

	if o := r.run(t, "apply"); o.code != 0 {
		t.Fatalf("the run exited %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}

	command := filepath.Join(r.out(), "claude/commands/alfred-injection.md")
	assertMatches(t, "agents/injection.command",
		golden(t, "agents/injection.command"), r.normalise(readFile(t, command)))

	// A recording made from a wrong output would still match itself; check the keys too.
	keys := frontmatterKeys(t, readFile(t, command))
	want := []string{"description", "argument-hint", "model", "tools"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("the frontmatter declares %v, want %v", keys, want)
	}
}

// frontmatterKeys returns the frontmatter keys in order and fails on any non-key line.
var frontmatterKey = regexp.MustCompile(`^[a-z][a-z-]*$`)

func frontmatterKeys(t *testing.T, text string) []string {
	t.Helper()

	lines := strings.Split(text, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		t.Fatalf("the file does not open with a frontmatter fence:\n%s", text)
	}

	var keys []string
	for _, line := range lines[1:] {
		if line == "---" {
			return keys
		}
		key, _, found := strings.Cut(line, ": ")
		if !found || !frontmatterKey.MatchString(key) {
			t.Errorf("frontmatter line %q is not one key", line)
			continue
		}
		keys = append(keys, key)
	}
	t.Fatalf("the frontmatter is never closed:\n%s", text)
	return nil
}

// agentKey returns one agent entry of opencode.json as raw JSON.
func agentKey(t *testing.T, path, key string) string {
	t.Helper()

	var config struct {
		Agent map[string]json.RawMessage `json:"agent"`
	}
	if err := json.Unmarshal([]byte(readFile(t, path)), &config); err != nil {
		t.Fatal(err)
	}
	value, there := config.Agent[key]
	if !there {
		t.Fatalf("%s holds no %s agent", path, key)
	}
	return string(value)
}

// TestReportModeWritesNothing: report mode prints the pending changes and writes nothing.
func TestReportModeWritesNothing(t *testing.T) {
	s := loadSpec(t)

	r := newRegistration(t, "uniform", s.Workflows.Shipped, s.Workflows.Custom)

	r.assertReport(t, "report-pending", r.run(t, "report"))

	if wrote := snapshot(t, r.out()); len(wrote) > 0 {
		t.Errorf("report-only wrote %d files", len(wrote))
	}
}

// TestASecondRunWritesNothing: rerunning over an unchanged tree changes no byte.
func TestASecondRunWritesNothing(t *testing.T) {
	s := loadSpec(t)

	r := newRegistration(t, "uniform", s.Workflows.Shipped, s.Workflows.Custom)
	if first := r.run(t, "apply"); first.code != 1 {
		t.Fatalf("the first run exited %d, want 1\n%s", first.code, first.stderr)
	}
	before := snapshot(t, r.out())

	r.assertReport(t, "unchanged", r.run(t, "apply"))

	after := snapshot(t, r.out())
	if !reflect.DeepEqual(before, after) {
		t.Error("the second run changed what the first one wrote")
	}
}

// TestCheckMatchesGolden records both answers of check: commands missing, then registered.
func TestCheckMatchesGolden(t *testing.T) {
	s := loadSpec(t)

	r := newRegistration(t, "uniform", s.Workflows.Shipped, s.Workflows.Absent)

	r.assertReport(t, "check-missing", r.run(t, "check"))

	if applied := r.run(t, "apply"); applied.code != 0 {
		t.Fatalf("the apply exited %d, want 0\n%s", applied.code, applied.stderr)
	}
	r.assertReport(t, "check-registered", r.run(t, "check"))
}

// Exclude targets: excludeAll keeps everything out of git, excludeManifest only the manifest.
const (
	excludeAll      = "exclude"
	excludeManifest = "exclude-manifest"
)

// repoFixture is one project-scope run, with repository and machine roots staged in a temp tree.
type repoFixture struct {
	work    string
	repo    string
	machine string
	exclude string
}

// newRepoFixture stages the repository fixture and the machine roots beside it.
func newRepoFixture(t *testing.T, s spec, exclude string) *repoFixture {
	t.Helper()

	p := &repoFixture{work: t.TempDir(), exclude: exclude}
	p.repo = stage(t, s.Project.Repo, filepath.Join(p.work, "repo"))
	p.machine = filepath.Join(p.work, "machine")
	stage(t, s.Workflows.Shipped, filepath.Join(p.machine, "workflows"))
	stage(t, s.Workflows.Custom, filepath.Join(p.machine, "custom"))
	return p
}

func (p *repoFixture) apply(t *testing.T) outcome {
	t.Helper()

	return attempt(t, "workflows", "project", "apply",
		filepath.Join(repoRoot, "internal/golden/testdata/profiles/uniform.json"),
		filepath.Join(repoRoot, "skills"),
		filepath.Join(p.repo, ".alfred/skills"),
		filepath.Join(p.repo, ".alfred/workflows"),
		filepath.Join(p.machine, "workflows"),
		filepath.Join(p.machine, "custom"),
		"manifest="+filepath.Join(p.repo, ".alfred/generated.json"),
		"claude="+filepath.Join(p.repo, ".claude"),
		"opencode="+filepath.Join(p.repo, "opencode.json"),
		"root="+p.repo,
		p.exclude+"="+filepath.Join(p.repo, excludeFile))
}

// excludeFile is recorded flat: git never tracks a .git path.
const excludeFile = ".git/info/exclude"

// wrote is what registration wrote in the repository, without fixture inputs or the exclude file.
func (p *repoFixture) wrote(t *testing.T) map[string]string {
	t.Helper()

	files := map[string]string{}
	for path, body := range snapshot(t, p.repo) {
		if strings.HasPrefix(path, ".alfred/workflows/") || strings.HasPrefix(path, ".alfred/skills/") {
			continue
		}
		if path == excludeFile {
			continue
		}
		files[path] = normalise(p.work, body)
	}
	return files
}

// assertExclude compares the local exclude file against its flat recording.
func (p *repoFixture) assertExclude(t *testing.T, rel string) {
	t.Helper()

	assertMatches(t, rel, golden(t, rel), readFile(t, filepath.Join(p.repo, excludeFile)))
}

// TestRegisterProjectMatchesGolden: writes and excludes inside the repo, never the machine roots.
func TestRegisterProjectMatchesGolden(t *testing.T) {
	s := loadSpec(t)

	p := newRepoFixture(t, s, excludeAll)
	before := snapshot(t, p.machine)
	fixture := snapshot(t, filepath.Join("testdata", s.Project.Repo))
	stale := agentKey(t, filepath.Join(p.repo, "opencode.json"), "alfred-stale")

	assertRecorded(t, "project/excluded.report", p.work, p.apply(t))
	p.assertExclude(t, "project/excluded.exclude")

	// The bytes are the machine recordings' templates; check where they land and what stays.
	for _, path := range []string{
		".alfred/generated.json",
		".claude/commands/alfred-almacen.md",
		".claude/commands/alfred-ventas-local.md",
		".claude/agents/alfred-almacen-spec.md",
		".claude/agents/alfred-ventas-local-propuesta.md",
	} {
		if _, err := os.Stat(filepath.Join(p.repo, path)); err != nil {
			t.Errorf("%s was not written: %v", path, err)
		}
	}
	for _, path := range []string{".gitignore", ".claude/commands/alfred-stale.md"} {
		if got := readFile(t, filepath.Join(p.repo, path)); got != fixture[path] {
			t.Errorf("%s was written over:\n%s", path, got)
		}
	}
	if got := agentKey(t, filepath.Join(p.repo, "opencode.json"), "alfred-stale"); got != stale {
		t.Errorf("the orphan alfred-stale key was written over:\n%s", got)
	}

	if after := snapshot(t, p.machine); !reflect.DeepEqual(before, after) {
		t.Error("a project-scope run changed the machine's roots")
	}
}

// TestProjectUnderCommittedArtifacts: artifacts.committed true only changes what is excluded.
func TestProjectUnderCommittedArtifacts(t *testing.T) {
	s := loadSpec(t)

	committed := newRepoFixture(t, s, excludeManifest)
	assertRecorded(t, "project/committed.report", committed.work, committed.apply(t))

	excluded := newRepoFixture(t, s, excludeAll)
	if o := excluded.apply(t); o.code != 0 {
		t.Fatalf("the run under artifacts.committed false exited %d\n%s", o.code, o.stderr)
	}

	// wrote() leaves out the exclude file, the one thing the setting may change.
	if was, now := excluded.wrote(t), committed.wrote(t); !reflect.DeepEqual(was, now) {
		t.Error("artifacts.committed changed what was written, and it may only change what is excluded")
	}

	// Only the manifest, which holds this machine's absolute paths, is kept out of git.
	committed.assertExclude(t, "project/committed.exclude")

	// Hard rule 18: .gitignore is the repository's and Alfred never writes it.
	if got := readFile(t, filepath.Join(committed.repo, ".gitignore")); got != "node_modules/\n" {
		t.Errorf(".gitignore = %q, want it exactly as the fixture left it", got)
	}
}

// TestProjectRemovalIsDrivenByTheManifest: a deleted workflow loses only its own files.
func TestProjectRemovalIsDrivenByTheManifest(t *testing.T) {
	s := loadSpec(t)

	p := newRepoFixture(t, s, excludeAll)
	if first := p.apply(t); first.code != 0 {
		t.Fatalf("the first run exited %d, want 0\n%s", first.code, first.stderr)
	}
	if err := os.RemoveAll(filepath.Join(p.repo, ".alfred/workflows", s.Project.Removes)); err != nil {
		t.Fatal(err)
	}

	assertRecorded(t, "project/removed.report", p.work, p.apply(t))

	// Removal follows the manifest: the other workflow, the orphan and .gitignore stay.
	for path, want := range map[string]bool{
		".claude/commands/alfred-almacen.md":      false,
		".claude/agents/alfred-almacen-spec.md":   false,
		".claude/commands/alfred-ventas-local.md": true,
		".claude/commands/alfred-stale.md":        true,
		".gitignore":                              true,
	} {
		_, err := os.Stat(filepath.Join(p.repo, path))
		if there := err == nil; there != want {
			t.Errorf("%s is present: %v, want %v", path, there, want)
		}
	}

	// An exclude line is never taken back, so the file is still the first run's.
	p.assertExclude(t, "project/excluded.exclude")
}

// TestNoRecordingLivesAtAPathACheckoutCannotHold: every fixture path must survive any checkout.
func TestNoRecordingLivesAtAPathACheckoutCannotHold(t *testing.T) {
	err := filepath.Walk("testdata", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("testdata", path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		for _, name := range strings.Split(rel, string(filepath.Separator)) {
			reason := unportable(name)
			if reason == "" {
				continue
			}
			// Quoted: the offending path may break the line it is printed on.
			t.Errorf("%q holds the component %q, which %s, so not every checkout has it",
				path, name, reason)
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// windowsReserved are the characters Windows refuses in a name.
const windowsReserved = `<>:"\|?*`

// windowsDevices are the names Windows treats as devices, with any extension or case.
var windowsDevices = regexp.MustCompile(`^(?i:con|prn|aux|nul|com[1-9]|lpt[1-9])(\.|$)`)

// unportable says why a path component does not reach every checkout, or "" when it does.
func unportable(name string) string {
	switch {
	case strings.EqualFold(name, ".git"):
		return "git refuses to track, declining in silence"
	case !utf8.ValidString(name):
		return "is not valid UTF-8"
	case strings.ContainsAny(name, windowsReserved):
		return "holds a character Windows refuses in a name"
	case strings.HasPrefix(name, " ") || strings.HasSuffix(name, " ") ||
		strings.HasSuffix(name, "."):
		return "begins or ends with a space or a dot, both of which Windows strips"
	case windowsDevices.MatchString(name):
		return "is a Windows device name"
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return fmt.Sprintf("holds the non-printable rune %q, a byte git archive does not preserve", r)
		}
	}
	return ""
}

// readFile reads a file or fails the test.
func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// snapshot reads every file under root. A missing root holds nothing.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = string(data)
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return files
}
