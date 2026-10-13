package generated

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Exclude appends each repository-relative path, anchored to the root, to the local exclude
// file and returns how many lines it added. An empty file means "do not exclude".
// It is idempotent: a path already listed, with or without a leading slash, is skipped.
func Exclude(file string, paths []string) (int, error) {
	if file == "" || len(paths) == 0 {
		return 0, nil
	}
	// Never write to .gitignore: the repository owns and commits it.
	if filepath.Base(file) == ".gitignore" {
		return 0, fmt.Errorf("%s: Alfred excludes through the local exclude file, never .gitignore", file)
	}

	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, fmt.Errorf("reading %s: %w", file, err)
	}

	// Keyed without the leading slash so "/x" and "x" count as the same exclusion.
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

	content := string(data)
	// Finish a last line with no newline, so appending does not extend the user's pattern.
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += strings.Join(adding, "\n") + "\n"

	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return 0, fmt.Errorf("writing %s: %w", file, err)
	}
	return len(adding), nil
}

// anchored ties a pattern to the repository root by adding a leading slash, but only when
// it has no inner separator; without it, `opencode.json` would match at any depth.
func anchored(path string) string {
	if strings.Contains(strings.TrimSuffix(path, "/"), "/") {
		return path
	}
	return "/" + path
}
