// Package parity checks the Go implementation against the Python scripts it replaces.
//
// Both implementations are installed while the migration is in progress, and state.json
// written by one is read by the other, so "equivalent" is not enough: the outputs have to
// be identical byte for byte. Every test here runs the real scripts under scripts/ and
// diffs what they produce against what the binary produces from the same input.
//
// The suite skips itself when python3 is gone. That is the intended end state - the
// scripts are deleted once this has passed - and a skipped parity test is honest, where a
// failing one after the deletion would be noise.
package parity

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var (
	repoRoot string
	binary   string
	python   string
)

func TestMain(m *testing.M) {
	os.Exit(runSuite(m))
}

// runSuite builds the binary the tests compare against and returns the exit code, so the
// temporary directory is removed on the way out rather than left behind by os.Exit.
func runSuite(m *testing.M) int {
	root, err := filepath.Abs("../..")
	if err != nil {
		panic(err)
	}
	repoRoot = root

	python, _ = exec.LookPath("python3")

	dir, err := os.MkdirTemp("", "alfred-parity")
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

func requirePython(t *testing.T) {
	t.Helper()
	if python == "" {
		t.Skip("python3 is not installed; the scripts it runs are already gone")
	}
}

// run executes a command and fails the test with its output when it errors.
func run(t *testing.T, stdin string, name string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), name, args...)
	cmd.Dir = repoRoot
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s: %v\nstderr: %s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String()
}

// snapshot reads a directory tree into relative path -> contents.
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
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// assertSameTree compares two snapshots and reports the first difference in each file.
func assertSameTree(t *testing.T, what string, want, got map[string]string) {
	t.Helper()

	names := map[string]bool{}
	for name := range want {
		names[name] = true
	}
	for name := range got {
		names[name] = true
	}

	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	if len(sorted) == 0 {
		t.Fatalf("%s: neither implementation wrote anything", what)
	}

	for _, name := range sorted {
		w, inWant := want[name]
		g, inGot := got[name]

		switch {
		case !inWant:
			t.Errorf("%s: the Go version wrote %s and the Python version did not", what, name)
		case !inGot:
			t.Errorf("%s: the Python version wrote %s and the Go version did not", what, name)
		case w != g:
			t.Errorf("%s: %s differs\n%s", what, name, firstDifference(w, g))
		}
	}
}

// firstDifference reports the first line that differs, which is enough to place a
// divergence in files that are otherwise thousands of identical bytes.
func firstDifference(want, got string) string {
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
			return fmt.Sprintf("  line %d\n  python: %q\n  go:     %q", i+1, w, g)
		}
	}
	return "  (identical lines but different bytes: check the trailing newline)"
}

// fixture builds a payload tree that exercises the awkward cases: a name that orders
// differently as a path than as a string, a nested tree, a .git directory, the files the
// installation writes itself, and a non-ASCII name.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"pay/a/b.txt":            "b",
		"pay/a-c/d.txt":          "d",
		"pay/a.b/c.txt":          "c",
		"pay/deep/er/still/x.md": "x",
		"pay/ñandu.md":           "non-ascii name",
		"pay/.hidden":            "hidden",
		"pay/.git/config":        "git",
		"pay/state.json":         "{}",
		"pay/profile.json":       "{}",
		"pay/bin/run.sh":         "#!/bin/sh\n",
		"top.yaml":               "top: true\n",
		"README.md":              "not part of the payload",
	}

	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	if out, err := exec.CommandContext(t.Context(), "cp", "-R", src+"/.", dst).CombinedOutput(); err != nil {
		t.Fatalf("copying the fixture: %v\n%s", err, out)
	}
	return dst
}

const payload = "pay,top.yaml"

func TestStateWriteIsIdentical(t *testing.T) {
	requirePython(t)

	source := fixture(t)
	pyHome, goHome := copyTree(t, source), copyTree(t, source)

	pyOut := run(t, "", python, "scripts/manage_state.py", "write", pyHome, "0.2.0", payload)
	goOut := run(t, "", binary, "state", "write", goHome, "0.2.0", payload)

	if pyOut != goOut {
		t.Errorf("stdout differs\n  python: %q\n  go:     %q", pyOut, goOut)
	}

	pyState, err := os.ReadFile(filepath.Join(pyHome, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	goState, err := os.ReadFile(filepath.Join(goHome, "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(pyState, goState) {
		t.Errorf("state.json differs\n%s", firstDifference(string(pyState), string(goState)))
	}
}

func TestStateCompareIsIdenticalInEveryClassification(t *testing.T) {
	requirePython(t)

	source := fixture(t)
	home := copyTree(t, source)

	// Record the install, then produce one file of every classification.
	run(t, "", binary, "state", "write", home, "0.2.0", payload)

	edit := func(root, rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	edit(home, "pay/a/b.txt", "the user edited this") // modified
	edit(source, "pay/a-c/d.txt", "a new release")    // updatable
	edit(source, "pay/added.txt", "added upstream")   // new
	// everything else stays unchanged

	pyOut := run(t, "", python, "scripts/manage_state.py", "compare", home, source, payload)
	goOut := run(t, "", binary, "state", "compare", home, source, payload)

	if pyOut != goOut {
		t.Errorf("compare report differs\n%s", firstDifference(pyOut, goOut))
	}
	for _, class := range []string{"modified", "updatable", "new", "unchanged"} {
		if !strings.Contains(goOut, `"`+class+`": [`) {
			t.Errorf("the report is missing the %s section: %s", class, goOut)
		}
	}
	if !strings.Contains(goOut, "pay/a/b.txt") {
		t.Error("the edited file should appear in the report")
	}
}

func TestStateCompareWithoutAStateFileIsIdentical(t *testing.T) {
	requirePython(t)

	source := fixture(t)
	home := t.TempDir()

	pyOut := run(t, "", python, "scripts/manage_state.py", "compare", home, source, payload)
	goOut := run(t, "", binary, "state", "compare", home, source, payload)

	if pyOut != goOut {
		t.Errorf("compare without a state file differs\n%s", firstDifference(pyOut, goOut))
	}
}

// applySnippet is the inline snippet install.sh used to copy the files an update brings.
const applySnippet = `
import json, shutil, sys
from pathlib import Path

report = json.load(sys.stdin)
home, source = Path(sys.argv[1]), Path(sys.argv[2])
count = 0

for rel in report["new"] + report["updatable"]:
    target = home / rel
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source / rel, target)
    count += 1

print(f"{count} files updated, {len(report['modified'])} left alone")
`

func TestStateApplyMatchesTheInlineSnippet(t *testing.T) {
	requirePython(t)

	source := fixture(t)
	pyHome, goHome := t.TempDir(), t.TempDir()

	report := run(t, "", binary, "state", "compare", pyHome, source, payload)

	pyOut := run(t, report, python, "-c", applySnippet, pyHome, source)
	goOut := run(t, report, binary, "state", "apply", goHome, source)

	if pyOut != goOut {
		t.Errorf("stdout differs\n  python: %q\n  go:     %q", pyOut, goOut)
	}
	assertSameTree(t, "state apply", snapshot(t, pyHome), snapshot(t, goHome))
}

// profiles cover the shapes the generator has to handle: the uniform one the installer
// writes, one that uses every optional key, and one that omits the optional keys entirely.
var profiles = map[string]string{
	"uniform": `{"orchestrator": "anthropic/claude-opus-5",
  "coordinator": "anthropic/claude-haiku-4-5-20251001",
  "phases": {"init": "anthropic/claude-opus-5", "explore": "anthropic/claude-opus-5",
    "refine": "anthropic/claude-opus-5", "research": "anthropic/claude-opus-5",
    "spec": "anthropic/claude-opus-5", "diagnose": "anthropic/claude-opus-5",
    "design": "anthropic/claude-opus-5", "tasks": "anthropic/claude-opus-5",
    "apply": "anthropic/claude-opus-5", "verify": "anthropic/claude-opus-5",
    "review": "anthropic/claude-opus-5", "archive": "anthropic/claude-opus-5"},
  "memory_tool_prefix": "mcp__engram__", "effort": {}, "extra_tools": {}}`,

	// Every optional key set, the phases deliberately out of alphabetical order, and a
	// non-ASCII extra tool so the JSON escaping is compared too.
	"every-option": `{"orchestrator": "anthropic/claude-opus-5",
  "coordinator": "vendor/cheap", "manage": "vendor/mid",
  "phases": {"spec": "vendor/big", "apply": "vendor/small", "init": "vendor/mid",
    "archive": "vendor/small"},
  "memory_tool_prefix": "mcp__custom__",
  "effort": {"spec": "high", "archive": "low"},
  "extra_tools": {"spec": ["mcp__atlassian__getJiraIssue", "mcp__a__niño"],
    "apply": ["mcp__b__tool"]}}`,

	// No coordinator, no manage, no effort, no extras, and the memory prefix switched off.
	"minimal": `{"orchestrator": "solo-model",
  "phases": {"spec": "solo-model", "apply": "solo-model"},
  "memory_tool_prefix": ""}`,
}

func TestGenerateAgentsIsIdentical(t *testing.T) {
	requirePython(t)

	for name, body := range profiles {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			profile := filepath.Join(dir, "profile.json")
			if err := os.WriteFile(profile, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}

			pyDir, goDir := filepath.Join(dir, "py"), filepath.Join(dir, "go")
			for _, d := range []string{pyDir, goDir} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			const skillsRoot = "/home/someone/.config/alfred/skills"

			pyOut := run(t, "", python, "scripts/generate_agents.py", profile, skillsRoot,
				"claude="+pyDir, "opencode="+filepath.Join(pyDir, "opencode.json"))
			goOut := run(t, "", binary, "agents", profile, skillsRoot,
				"claude="+goDir, "opencode="+filepath.Join(goDir, "opencode.json"))

			if pyOut != goOut {
				t.Errorf("stdout differs\n  python: %q\n  go:     %q", pyOut, goOut)
			}
			assertSameTree(t, "generate agents", snapshot(t, pyDir), snapshot(t, goDir))
		})
	}
}

func TestGenerateAgentsMergeIsIdentical(t *testing.T) {
	requirePython(t)

	dir := t.TempDir()
	profile := filepath.Join(dir, "profile.json")
	if err := os.WriteFile(profile, []byte(profiles["every-option"]), 0o644); err != nil {
		t.Fatal(err)
	}

	// A configuration that already holds a foreign agent and a stale Alfred one. The merge
	// has to keep the first, replace the second, and leave the key order alone.
	existing := `{
  "theme": "opencode-dark",
  "agent": {
    "mine": {"model": "keep-me", "mode": "primary"},
    "alfred": {"model": "stale"},
    "alfred-spec": {"model": "stale"},
    "other": {"model": "keep-me-too"}
  },
  "$schema": "https://opencode.ai/config.json"
}
`
	pyDir, goDir := filepath.Join(dir, "py"), filepath.Join(dir, "go")
	for _, d := range []string{pyDir, goDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "opencode.json"), []byte(existing), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	const skillsRoot = "/opt/alfred/skills"
	run(t, "", python, "scripts/generate_agents.py", profile, skillsRoot,
		"opencode="+filepath.Join(pyDir, "opencode.json"))
	run(t, "", binary, "agents", profile, skillsRoot,
		"opencode="+filepath.Join(goDir, "opencode.json"))

	assertSameTree(t, "opencode merge", snapshot(t, pyDir), snapshot(t, goDir))

	merged, err := os.ReadFile(filepath.Join(goDir, "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(merged), "keep-me-too") {
		t.Error("a foreign agent was dropped by the merge")
	}
	if strings.Contains(string(merged), `"stale"`) {
		t.Error("a stale alfred agent survived the merge")
	}
}

// jsonGetSnippets are the inline one-liners install.sh used to read a single value.
func TestJSONGetMatchesTheInlineOneLiners(t *testing.T) {
	requirePython(t)

	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	if err := os.WriteFile(state, []byte(`{"version": "0.2.0", "files": {}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	pyOut := run(t, "", python, "-c",
		"import json,sys;print(json.load(open(sys.argv[1]))['version'])", state)
	goOut := run(t, "", binary, "json-get", state, "version")

	if pyOut != goOut {
		t.Errorf("version differs\n  python: %q\n  go:     %q", pyOut, goOut)
	}

	// The modified list, read from a report on stdin, one path per line.
	report := `{"new": [], "unchanged": [], "modified": ["a/b.txt", "c.md"], "updatable": []}`
	pyList := run(t, report, python, "-c",
		`import json,sys; print("\n".join(json.load(sys.stdin)["modified"]))`)
	goList := run(t, report, binary, "json-get", "-", "modified")

	if pyList != goList {
		t.Errorf("modified list differs\n  python: %q\n  go:     %q", pyList, goList)
	}
}
