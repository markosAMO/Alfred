package generated

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Exclude appends every path to a repository's local exclude file and returns how many
// lines it added.
//
// The caller decides whether to exclude at all: under `artifacts.committed: true` it
// passes no file and nothing is appended anywhere. Reading that setting here would mean
// this package parsing a repository's configuration to find out what the caller already
// knows.
//
// The caller makes every path relative to the repository root, which is what git reads an
// exclude pattern against, and every line written is anchored to it — see anchored, which
// adds the leading slash only where git does not infer one.
//
// Registration runs again on every change to a workflow, so appending is idempotent: a
// path already excluded, by Alfred or by the user, is left as the one line it is, written
// with a leading slash or without. The two forms are the one exclusion, and a repository
// registered before this would otherwise gain a second line for a path on every run.
func Exclude(file string, paths []string) (int, error) {
	if file == "" || len(paths) == 0 {
		return 0, nil
	}
	// Hard rule 18: Alfred's files are kept out of git through the repository's local
	// exclude file, which is nobody else's, and never through .gitignore, which the
	// repository owns and commits.
	if filepath.Base(file) == ".gitignore" {
		return 0, fmt.Errorf("%s: Alfred excludes through the local exclude file, never .gitignore", file)
	}

	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, fmt.Errorf("reading %s: %w", file, err)
	}

	// Keyed without the anchor, so a line written before anchoring and the anchored form
	// of the same path count as the one exclusion they are.
	present := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		present[strings.TrimPrefix(strings.TrimSpace(line), "/")] = true
	}

	var adding []string
	for _, path := range paths {
		path = strings.TrimSpace(path)
		key := strings.TrimPrefix(path, "/")
		if path == "" || present[key] {
			continue
		}
		present[key] = true
		adding = append(adding, anchored(path))
	}
	if len(adding) == 0 {
		return 0, nil
	}

	out := string(data)
	// A file somebody edited may end mid-line, and appending to it would extend that line
	// into a pattern neither of us wrote.
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += strings.Join(adding, "\n") + "\n"

	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(file, []byte(out), 0o644); err != nil {
		return 0, fmt.Errorf("writing %s: %w", file, err)
	}
	return len(adding), nil
}

// anchored ties a pattern to the repository root, which is the directory the local exclude
// file governs.
//
// git does that itself for any pattern carrying a separator somewhere other than its end,
// which is every concrete path registration writes but one: `opencode.json` has no
// directory in it, and unanchored it excludes a file of that name at any depth — a
// vendored copy, a sub-package's own, a test fixture — which then cannot be staged and
// never appears in `git status`. The slash is added only where it is missing, so a path
// git already anchors is written the way the rest of the file writes it.
func anchored(path string) string {
	if strings.Contains(strings.TrimSuffix(path, "/"), "/") {
		return path
	}
	return "/" + path
}
