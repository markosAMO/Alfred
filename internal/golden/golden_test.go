// Package golden pins the exact bytes the installer's helper produces.
//
// These files were recorded from scripts/*.py while both implementations were in the tree
// and `make parity` was green. The scripts are gone; the guarantee they gave is not. A
// change that alters a single byte of state.json, of a compare report or of any generated
// agent definition fails here, which is what the parity suite used to do and what nothing
// else would catch: the output is consumed by other programs, not read by a human.
//
// To re-record after a deliberate change, see testdata/README.md.
package golden

import (
	"bytes"
	"context"
	"encoding/json"
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
)

// spec is the input every golden file was produced from.
type spec struct {
	Payload     string            `json:"payload"`
	Version     string            `json:"version"`
	SkillsRoot  string            `json:"skills_root"`
	Files       map[string]string `json:"files"`
	HomeEdits   map[string]string `json:"home_edits"`
	SourceEdits map[string]string `json:"source_edits"`
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

	// The recorded report is the contract, so the classifications are named here too: a
	// report that silently stopped reporting a modified file would still match a golden
	// file recorded from the same mistake.
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
}

func TestStateCompareWithoutAStateFileMatchesGolden(t *testing.T) {
	s := loadSpec(t)

	source := t.TempDir()
	build(t, source, s.Files)
	build(t, source, s.SourceEdits)

	out := run(t, "state", "compare", t.TempDir(), source, s.Payload)
	assertMatches(t, "compare without a state file", golden(t, "state/compare-no-state.json"), out)
}

func TestGenerateAgentsMatchesGolden(t *testing.T) {
	s := loadSpec(t)

	entries, err := os.ReadDir("testdata/profiles")
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".json")

		t.Run(name, func(t *testing.T) {
			work := t.TempDir()
			profile := filepath.Join("testdata/profiles", entry.Name())

			out := run(t, "agents", filepath.Join(repoRoot, "internal/golden", profile), s.SkillsRoot,
				"claude="+work, "opencode="+filepath.Join(work, "opencode.json"))
			assertMatches(t, name+" stdout", golden(t, "agents/"+name+".stdout"), out)

			want := snapshot(t, filepath.Join("testdata/agents", name))
			got := snapshot(t, work)

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
				w, recorded := want[n]
				g, produced := got[n]
				switch {
				case !recorded:
					t.Errorf("%s is generated now but was not recorded", n)
				case !produced:
					t.Errorf("%s was recorded but is no longer generated", n)
				default:
					assertMatches(t, name+"/"+n, w, g)
				}
			}
		})
	}
}

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
