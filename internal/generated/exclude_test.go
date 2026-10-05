package generated

import (
	"os"
	"path/filepath"
	"testing"
)

// TestARepositoryThatDoesNotAcceptAlfredsFilesExcludesEveryPathWritten is the scenario "a
// repository that does not accept Alfred's files": the caller passes the local exclude
// file and every generated path is recorded there.
func TestARepositoryThatDoesNotAcceptAlfredsFilesExcludesEveryPathWritten(t *testing.T) {
	repo := t.TempDir()
	file := write(t, filepath.Join(repo, ".git/info/exclude"), "# git ls-files --others\nbuild/\n")

	added, err := Exclude(file, []string{
		".claude/commands/alfred-ventas.md",
		".alfred/generated.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 {
		t.Errorf("added = %d, want 2", added)
	}

	got := read(t, file)
	want := "# git ls-files --others\nbuild/\n.claude/commands/alfred-ventas.md\n.alfred/generated.json\n"
	if got != want {
		t.Errorf("exclude file =\n%q\nwant\n%q", got, want)
	}
}

// A pattern git reads from a local exclude file is anchored to the repository root only
// when it carries a separator somewhere other than its end. `opencode.json`, which
// registration writes at project scope, carries none, so unanchored it excludes a file of
// that name at any depth — a vendored copy, a fixture, a sub-package's own. It is written
// with the slash that anchors it, and a path git anchors on its own is left as it is
// rather than anchored twice.
func TestAPathWithNoDirectoryInItIsAnchoredToTheRepositoryRoot(t *testing.T) {
	file := write(t, filepath.Join(t.TempDir(), ".git/info/exclude"), "build/\n")

	added, err := Exclude(file, []string{"opencode.json", ".alfred/generated.json"})
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 {
		t.Errorf("added = %d, want 2", added)
	}

	got := read(t, file)
	want := "build/\n/opencode.json\n.alfred/generated.json\n"
	if got != want {
		t.Errorf("exclude file =\n%q\nwant\n%q", got, want)
	}
}

// Appending is idempotent across the anchoring too: a repository registered before
// `opencode.json` was anchored holds the line without the slash, and it is the same
// exclusion. Adding the anchored form beside it would grow the file on every run.
func TestAPathAlreadyExcludedUnanchoredIsLeftAsTheOneLineItIs(t *testing.T) {
	file := write(t, filepath.Join(t.TempDir(), "exclude"), "opencode.json\n.alfred/generated.json\n")

	added, err := Exclude(file, []string{"opencode.json", ".alfred/generated.json"})
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Errorf("added = %d, want 0", added)
	}
	if got, want := read(t, file), "opencode.json\n.alfred/generated.json\n"; got != want {
		t.Errorf("exclude file = %q, want %q", got, want)
	}
}

// TestARepositoryThatAcceptsThemHasNothingExcluded is the scenario "a repository that
// accepts them": under `artifacts.committed: true` the caller passes no exclude target,
// so nothing is appended anywhere and no file is created.
func TestARepositoryThatAcceptsThemHasNothingExcluded(t *testing.T) {
	repo := t.TempDir()

	added, err := Exclude("", []string{".claude/commands/alfred-ventas.md"})
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Errorf("added = %d, want 0", added)
	}

	entries, err := os.ReadDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the repository gained %d entries, want none", len(entries))
	}
}

// Registration runs again and again, so appending the same paths twice must leave one
// line each.
func TestExcludingTheSamePathTwiceAddsOneLine(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".git/info/exclude")
	paths := []string{".claude/commands/alfred-ventas.md", ".alfred/generated.json"}

	if _, err := Exclude(file, paths); err != nil {
		t.Fatal(err)
	}
	added, err := Exclude(file, append(paths, ".claude/agents/alfred-ventas-propuesta.md"))
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 {
		t.Errorf("added = %d, want 1", added)
	}

	got := read(t, file)
	want := ".claude/commands/alfred-ventas.md\n.alfred/generated.json\n" +
		".claude/agents/alfred-ventas-propuesta.md\n"
	if got != want {
		t.Errorf("exclude file =\n%q\nwant\n%q", got, want)
	}

	// A run that excludes nothing new leaves the file byte for byte as it was.
	added, err = Exclude(file, paths)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Errorf("added = %d on a run with nothing new, want 0", added)
	}
	if again := read(t, file); again != got {
		t.Errorf("exclude file changed with nothing to add:\n%q\n%q", again, got)
	}
}

// A file somebody edited without a final newline must not have the first path appended to
// its last line.
func TestExcludingIntoAFileWithNoFinalNewline(t *testing.T) {
	file := write(t, filepath.Join(t.TempDir(), "exclude"), "build/")

	if _, err := Exclude(file, []string{".alfred/generated.json"}); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, file), "build/\n.alfred/generated.json\n"; got != want {
		t.Errorf("exclude file = %q, want %q", got, want)
	}
}

// Hard rule 18: Alfred's files are kept out of git through the repository's local exclude
// file and never through .gitignore, which belongs to the repository.
func TestExcludeRefusesToWriteGitignore(t *testing.T) {
	repo := t.TempDir()
	file := write(t, filepath.Join(repo, ".gitignore"), "node_modules/\n")

	if _, err := Exclude(file, []string{".alfred/generated.json"}); err == nil {
		t.Fatal(".gitignore was accepted as an exclude target")
	}
	if got, want := read(t, file), "node_modules/\n"; got != want {
		t.Errorf(".gitignore = %q, want %q", got, want)
	}
}

func TestExcludingNoPathsWritesNothing(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".git/info/exclude")

	added, err := Exclude(file, nil)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Errorf("added = %d, want 0", added)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("an exclude file was created for no paths: %v", err)
	}
}

// An exclude target that is not a file is reported rather than read as an empty one,
// which would append every path to a file nobody can read.
func TestExcludeReportsAnUnreadableTarget(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "exclude"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Exclude(filepath.Join(dir, "exclude"), []string{".alfred/generated.json"}); err == nil {
		t.Fatal("a directory was accepted as an exclude file")
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
