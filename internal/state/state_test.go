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
		"pay/a-c/d.txt",
		"pay/a.b/c.txt",
		"pay/a/b.txt",
		"pay/deep/er/still/x.md",
		"pay/ñandu.md",
		"top.yaml",
	}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ManagedFiles =\n  %v\nwant\n  %v", got, want)
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

func TestWriteRecordsHashes(t *testing.T) {
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
	if !strings.Contains(text, `"pay/ñandu.md"`) {
		t.Error("a non-ASCII name should be recorded as it is")
	}
	// The two files the installation writes itself are never recorded.
	if strings.Contains(text, "profile.json") {
		t.Error("profile.json should not be recorded")
	}
}

// The helper the installation builds is copied to bin/alfred, so it is written by the
// installation rather than shipped with it and is never recorded. It is matched by its
// relative path and not by its base name, because a name rule would exclude any payload
// file that ever carried the name.
func TestManagedFilesExcludesTheInstalledHelperByPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "bin/alfred"), "a binary")
	writeFile(t, filepath.Join(root, "bin/worktree.sh"), "#!/usr/bin/env bash")
	writeFile(t, filepath.Join(root, "workflows/alfred"), "a payload file of the same name")

	files, err := ManagedFiles(root, []string{"bin", "workflows"})
	if err != nil {
		t.Fatal(err)
	}
	got := rels(t, root, files)

	want := []string{"bin/worktree.sh", "workflows/alfred"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ManagedFiles =\n  %v\nwant\n  %v", got, want)
	}
}

func TestWriteNeverHashesTheInstalledHelper(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "bin/alfred"), "a binary")
	writeFile(t, filepath.Join(root, "bin/worktree.sh"), "#!/usr/bin/env bash")

	count, err := Write(root, "9.9.9", []string{"bin"})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("recorded %d files, want only bin/worktree.sh", count)
	}

	data, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"bin/alfred"`) {
		t.Error("bin/alfred should not be recorded")
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
	want := `{"new":[],"unchanged":[],"modified":[],"updatable":[]}`
	if empty != want {
		t.Errorf("empty report = %s, want %s", empty, want)
	}

	filled := (&Report{New: []string{"a"}, Modified: []string{"b", "c"}}).JSON()
	wantFilled := `{"new":["a"],"unchanged":[],"modified":["b","c"],"updatable":[]}`
	if filled != wantFilled {
		t.Errorf("report = %s, want %s", filled, wantFilled)
	}
}

// The clone is gone: nothing but the installation directory is left, and the version has
// to come out of it. Write is the only thing that ever put the number there, so the test
// asks Write for it and then asks Version back.
func TestVersionReportsTheVersionThatWasInstalled(t *testing.T) {
	home := tree(t)
	if _, err := Write(home, "0.4.0", []string{"pay"}); err != nil {
		t.Fatal(err)
	}

	got, err := Version(home)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.4.0" {
		t.Errorf("Version = %q, want %q", got, "0.4.0")
	}
}

// An update refreshes the installed copy, and the number the helper reports has to be the
// one that run installed rather than the one before it.
func TestVersionMatchesTheVersionThatWasJustInstalled(t *testing.T) {
	home := tree(t)
	if _, err := Write(home, "0.3.0", []string{"pay"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(home, "0.4.0", []string{"pay"}); err != nil {
		t.Fatal(err)
	}

	got, err := Version(home)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.4.0" {
		t.Errorf("Version = %q, want the version the second install recorded, %q", got, "0.4.0")
	}
}

func TestVersionWithNoInstallationReportsAndNamesTheOperation(t *testing.T) {
	home := t.TempDir()

	got, err := Version(home)
	if err == nil {
		t.Fatalf("Version = %q, want an error where nothing is installed", got)
	}
	if got != "" {
		t.Errorf("Version = %q, want no version at all", got)
	}
	for _, want := range []string{"no installation", home, "install.sh install"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// A state.json with no version is the same answer as no state.json: there is no number to
// trust here. Reporting "unknown" would be worse than failing, because the only reason to
// ask is to compare the answer with something.
func TestVersionWithAStateFileCarryingNoVersion(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "state.json"), `{"files":{}}`)

	got, err := Version(home)
	if err == nil {
		t.Fatalf("Version = %q, want an error where no version was recorded", got)
	}
	if got != "" {
		t.Errorf("Version = %q, want no version at all", got)
	}
	for _, want := range []string{"no installation", home, "install.sh install"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// A state.json that cannot be read for any reason other than being absent is a failure
// rather than "no installation here": something is installed and the read went wrong, and
// saying so is the difference between "run install" and "look at this file".
func TestVersionWithAnUnreadableStateFile(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "state.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Version(home)
	if err == nil {
		t.Fatal("an unreadable state.json should be an error")
	}
	if !strings.Contains(err.Error(), "state.json") {
		t.Errorf("error %q does not name the file it failed on", err)
	}
	if strings.Contains(err.Error(), "no installation") {
		t.Errorf("error %q should not claim nothing is installed", err)
	}
}

func TestVersionWithAMalformedStateFile(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "state.json"), "{not json")

	if _, err := Version(home); err == nil {
		t.Fatal("a malformed state.json should be an error, not an empty version")
	} else if !strings.Contains(err.Error(), "state.json") {
		t.Errorf("error %q does not name the file it failed on", err)
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
