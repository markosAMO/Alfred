package workflow

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Root is one directory holding workflow directories, with the label the registration
// report prints beside everything found in it.
type Root struct {
	Label string
	Dir   string
}

// Found is one workflow directory the scan met, registered or not. Refused workflows stay
// in the list with their reason, so the report names every workflow the user created.
type Found struct {
	Name   string
	Dir    string
	Source string

	// Exactly one is set: Definition when the workflow registered, Reason when it did not.
	Definition *Definition
	Reason     string
}

// Scan reads every root of one scope and validates every workflow directory it finds,
// returning them sorted by name. A missing root means no workflows; an unreadable one is
// an error, since treating it as empty would orphan every command generated from it.
func Scan(roots []Root) ([]Found, error) {
	holders := make(map[string][]Root)
	for _, root := range roots {
		entries, err := os.ReadDir(root.Dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s cannot be read: %w", root.Dir, err)
		}

		for _, entry := range entries {
			// Only directories are workflows; dot directories belong to tools.
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			holders[entry.Name()] = append(holders[entry.Name()], root)
		}
	}

	found := make([]Found, 0, len(holders))
	for _, name := range sorted(holders) {
		found = append(found, read(name, holders[name]))
	}
	return found, nil
}

// read loads and validates the workflow called name from the roots that hold it, and
// returns its entry with either the definition or the reason it was refused.
func read(name string, roots []Root) Found {
	labels := make([]string, len(roots))
	for i, root := range roots {
		labels[i] = root.Label
	}

	// A name present in several roots registers from none: picking one would shadow the other.
	if len(roots) > 1 {
		return Found{Name: name, Source: strings.Join(labels, ", "), Reason: duplicated(labels)}
	}

	entry := Found{Name: name, Dir: filepath.Join(roots[0].Dir, name), Source: labels[0]}
	definition, err := Load(entry.Dir)
	if err != nil {
		entry.Reason = err.Error()
		return entry
	}

	// The rules file is checked here, not in Decode, because it needs the directory path.
	if err := definition.validateRules(entry.Dir); err != nil {
		entry.Reason = err.Error()
		return entry
	}

	entry.Definition = definition
	return entry
}

// duplicated builds the refusal reason for a workflow name found in several roots.
func duplicated(labels []string) string {
	if len(labels) == 2 {
		return fmt.Sprintf("exists in both roots, %s and %s; neither copy is registered",
			labels[0], labels[1])
	}
	return fmt.Sprintf("exists in %d roots, %s; no copy is registered",
		len(labels), strings.Join(labels, ", "))
}
