package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile creates a file and every directory above it.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tree builds a payload directory with the awkward cases: a name that sorts differently as
// a path than as a string, a nested directory, a .git directory, the two files the
// installation writes itself, and a non-ASCII name.
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "pay/a/b.txt"), "b")
	writeFile(t, filepath.Join(root, "pay/a-c/d.txt"), "d")
	writeFile(t, filepath.Join(root, "pay/a.b/c.txt"), "c")
	writeFile(t, filepath.Join(root, "pay/deep/er/still/x.md"), "x")
	writeFile(t, filepath.Join(root, "pay/ñandu.md"), "n")
	writeFile(t, filepath.Join(root, "pay/.hidden"), "h")
	writeFile(t, filepath.Join(root, "pay/.git/config"), "git")
	writeFile(t, filepath.Join(root, "pay/state.json"), "{}")
	writeFile(t, filepath.Join(root, "pay/profile.json"), "{}")
	writeFile(t, filepath.Join(root, "top.yaml"), "top")
	writeFile(t, filepath.Join(root, "README.md"), "not payload")

	return root
}

func rels(t *testing.T, root string, paths []string) []string {
	t.Helper()
	out := make([]string, len(paths))
	for i, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = rel
	}
	return out
}

func TestManagedFilesFiltersAndOrders(t *testing.T) {
	root := tree(t)

	files, err := ManagedFiles(root, []string{"pay", "top.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	got := rels(t, root, files)

	want := []string{
		"pay/.hidden",
		"pay/a/b.txt",
		"pay/a-c/d.txt",
		"pay/a.b/c.txt",
		"pay/deep/er/still/x.md",
		"pay/ñandu.md",
		"top.yaml",
	}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ManagedFiles =\n  %v\nwant\n  %v", got, want)
	}
}

// The ordering is the one pathlib produces, which is not the ordering of the joined
// strings: "a/b.txt" sorts before "a-c/d.txt" as paths and after it as strings. It decides
// the key order of state.json, so it is asserted on its own.
func TestPathOrderIsComponentWiseNotStringWise(t *testing.T) {
	if !lessPath("/r/a/b.txt", "/r/a-c/d.txt") {
		t.Error("a/b.txt should sort before a-c/d.txt, as pathlib orders them")
	}
	if "/r/a/b.txt" < "/r/a-c/d.txt" {
		t.Error("precondition: as plain strings the order is the other way round")
	}
	if !lessPath("/r/a", "/r/a/b") {
		t.Error("a prefix should sort first")
	}
}

func TestManagedFilesWithoutPayloadTakesWholeRoot(t *testing.T) {
	root := tree(t)

	files, err := ManagedFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := rels(t, root, files)

	// README.md is excluded by the payload, not by the filter, so it appears here.
	if len(got) != 8 {
		t.Errorf("expected the whole root (8 files), got %d: %v", len(got), got)
	}
	for _, rel := range got {
		if strings.Contains(rel, ".git") || strings.HasSuffix(rel, "state.json") {
			t.Errorf("%s should have been filtered out", rel)
		}
	}
}

func TestManagedFilesSkipsMissingPayloadItem(t *testing.T) {
	root := tree(t)

	files, err := ManagedFiles(root, []string{"pay", "does-not-exist"})
	if err != nil {
		t.Fatalf("a missing payload item should be skipped, not fatal: %v", err)
	}
	if len(files) != 6 {
		t.Errorf("got %d files, want the 6 under pay/", len(files))
	}
}

func TestWriteRecordsHashesAndEscapesNonASCII(t *testing.T) {
	root := tree(t)

	count, err := Write(root, "9.9.9", []string{"pay"})
	if err != nil {
		t.Fatal(err)
	}
	if count != 6 {
		t.Errorf("recorded %d files, want 6", count)
	}

	data, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	if !strings.HasPrefix(text, "{\n  \"version\": \"9.9.9\",\n  \"files\": {\n") {
		t.Errorf("unexpected shape:\n%s", text[:min(len(text), 120)])
	}
	if !strings.HasSuffix(text, "}\n") {
		t.Error("state.json should end with a newline")
	}
	// json.dumps escapes non-ASCII; a raw UTF-8 name here would mean a silent divergence.
	if strings.Contains(text, "ñandu.md") {
		t.Error("non-ASCII name was not escaped")
	}
	if !strings.Contains(text, "pay/\\u00f1andu.md") {
		t.Error("expected the escaped form of the non-ASCII name")
	}
	// The two files the installation writes itself are never recorded.
	if strings.Contains(text, "profile.json") {
		t.Error("profile.json should not be recorded")
	}
}

func TestCompareClassifiesEveryOutcome(t *testing.T) {
	source := tree(t)
	home := t.TempDir()

	// Install by copying the payload, then record what was installed.
	files, err := ManagedFiles(source, []string{"pay"})
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range files {
		rel, _ := filepath.Rel(source, src)
		data, _ := os.ReadFile(src)
		writeFile(t, filepath.Join(home, rel), string(data))
	}
	if _, err := Write(home, "1.0.0", []string{"pay"}); err != nil {
		t.Fatal(err)
	}

	// The user edited an installed file: it must be reported, never overwritten.
	writeFile(t, filepath.Join(home, "pay/a/b.txt"), "edited by the user")
	// A new release changed a file the user did not touch.
	writeFile(t, filepath.Join(source, "pay/a-c/d.txt"), "new release")
	// A new release added a file.
	writeFile(t, filepath.Join(source, "pay/added.txt"), "added")

	report, err := Compare(home, source, []string{"pay"})
	if err != nil {
		t.Fatal(err)
	}

	assertOnly := func(name string, got []string, want string) {
		t.Helper()
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s = %v, want [%s]", name, got, want)
		}
	}
	assertOnly("modified", report.Modified, "pay/a/b.txt")
	assertOnly("updatable", report.Updatable, "pay/a-c/d.txt")
	assertOnly("new", report.New, "pay/added.txt")

	if len(report.Unchanged) != 4 {
		t.Errorf("unchanged = %v, want the other 4 files", report.Unchanged)
	}
}

func TestCompareWithoutStateTreatsInstalledFilesAsUpdatable(t *testing.T) {
	source := tree(t)
	home := t.TempDir()

	writeFile(t, filepath.Join(home, "pay/a/b.txt"), "different content")

	// No state.json: nothing is recorded, so nothing can be called modified.
	report, err := Compare(home, source, []string{"pay"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Modified) != 0 {
		t.Errorf("modified = %v, want none without a state file", report.Modified)
	}
	if len(report.Updatable) != 1 || report.Updatable[0] != "pay/a/b.txt" {
		t.Errorf("updatable = %v", report.Updatable)
	}
	if len(report.New) != 5 {
		t.Errorf("new = %v, want the 5 files that are not installed", report.New)
	}
}

func TestReportJSONShapeAndEmptySections(t *testing.T) {
	empty := (&Report{}).JSON()
	want := `{"new": [], "unchanged": [], "modified": [], "updatable": []}`
	if empty != want {
		t.Errorf("empty report = %s, want %s", empty, want)
	}

	filled := (&Report{New: []string{"a"}, Modified: []string{"b", "c"}}).JSON()
	wantFilled := `{"new": ["a"], "unchanged": [], "modified": ["b", "c"], "updatable": []}`
	if filled != wantFilled {
		t.Errorf("report = %s, want %s", filled, wantFilled)
	}
}

func TestSHA256KnownValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	writeFile(t, path, "abc")

	got, err := SHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Errorf("SHA256 = %s, want %s", got, want)
	}
}
