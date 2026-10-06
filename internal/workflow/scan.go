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
//
// A scan is handed the roots of one scope and cannot tell which scope that is. Machine
// scope passes the installed root and the user's custom root, project scope passes the
// repository's own, and there is no branch here that asks. That is what makes a
// machine-level run incapable of reaching into a repository: it is never given one.
type Root struct {
	Label string
	Dir   string
}

// Found is one workflow name the scan met, registered or not.
//
// A workflow that cannot be registered stays in this list with the reason it was refused,
// rather than being left out of it. The report names every workflow the user created, and
// one that vanished from the output would read as one that was never there.
type Found struct {
	Name   string
	Dir    string
	Source string

	// Definition is set only when the workflow registered; Reason is set only when it
	// did not. Exactly one of the two is ever present.
	Definition *Definition
	Reason     string
}

// Scan reads every root of one scope and validates every definition it finds.
//
// An absent root is no workflows rather than an error: the custom root is never created by
// the installer, and a machine where nobody has written a custom workflow is the ordinary
// case. A root that exists and cannot be read is an error, because reading it as empty
// would tell the caller that every command generated from it is now an orphan.
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
			// A workflow is a directory. Anything else in a root is somebody's note, and a
			// dot directory is a tool's, not a workflow with an unusable name.
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

// read turns one name into its entry. The roots arrive in the order they were given, so
// what the report says about a duplicate depends on which roots hold the name and never on
// which copy was written first — a fact the scan cannot see and the user cannot act on.
func read(name string, roots []Root) Found {
	labels := make([]string, len(roots))
	for i, root := range roots {
		labels[i] = root.Label
	}

	// A duplicated name registers under neither root and addresses neither copy: choosing
	// one would be shadowing, which is the outcome this case exists to refuse.
	if len(roots) > 1 {
		return Found{Name: name, Source: strings.Join(labels, ", "), Reason: duplicated(labels)}
	}

	entry := Found{Name: name, Dir: filepath.Join(roots[0].Dir, name), Source: labels[0]}
	definition, err := Load(entry.Dir)
	if err != nil {
		entry.Reason = err.Error()
		return entry
	}

	// The rules file is checked here and not in the decode: it is a path relative to the
	// directory the definition was found in, and the decode is given a file rather than a
	// directory. A workflow whose rules file is missing or outside it is refused with the
	// reason beside every other one, so the report needs no case of its own.
	if err := definition.validateRules(entry.Dir); err != nil {
		entry.Reason = err.Error()
		return entry
	}

	entry.Definition = definition
	return entry
}

func duplicated(labels []string) string {
	if len(labels) == 2 {
		return fmt.Sprintf("exists in both roots, %s and %s; neither copy is registered",
			labels[0], labels[1])
	}
	return fmt.Sprintf("exists in %d roots, %s; no copy is registered",
		len(labels), strings.Join(labels, ", "))
}
