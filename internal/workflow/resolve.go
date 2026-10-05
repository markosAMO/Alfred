package workflow

import (
	"fmt"
	"os"
	"path/filepath"
)

// SkillFile is what a phase is, on disk, in every root it can resolve from.
const SkillFile = "SKILL.md"

// Origin is the root a phase resolved from.
//
// The generator needs it for two reasons: a phase the workflow brought itself is
// addressable under a name that is the workflow's alone, so it never collides with the
// shared phase it shadows, and the side a phase came from is the side that assigned its
// model.
type Origin string

const (
	FromWorkflow   Origin = "workflow"
	FromRepository Origin = "repository"
	FromShared     Origin = "shared"
)

// SkillRoots are the places a phase may resolve from, tried in the order they are written
// here: the workflow's own, then the repository's, then the shared library.
//
// Repository is empty for a machine-scope workflow, and that emptiness is the only thing
// in this file that knows the two scopes apart. There is no declaration for "a repository
// will provide this": a machine workflow naming a phase no machine root has is refused
// here, where the user is looking at the workflow, rather than three steps into a change.
type SkillRoots struct {
	Own        string
	Repository string
	Shared     string
}

// Resolved is one phase of a workflow, with everything the generator needs to dispatch it
// or to report that it cannot be.
type Resolved struct {
	Name   string
	Skill  string
	Origin Origin

	// Model is empty when neither side assigned one. That is a result the caller reports
	// and a phase it generates no subagent for; it is not an error, and there is no
	// fallback to the orchestrator's model.
	Model string

	// Tools is the set an own phase declares. Empty means the default set for the phase,
	// which is the generator's to decide, and it is always empty for a shared phase: what
	// that phase runs with is the installation's, and a declaration is refused below rather
	// than carried here for the generator to ignore.
	Tools []string
}

// Resolve locates every phase of one workflow within the roots of its scope, and returns
// the first reason the workflow cannot be registered: a phase that resolves in none of
// them, or one declaring a model or a tool set where the installation owns the assignment.
//
// Rejecting the workflow is the point: a command generated for a workflow with an
// unresolvable phase works until a run reaches that phase, which is the worst moment to
// find out. installed is the model assignment the installation made, keyed by phase name.
func Resolve(d *Definition, roots SkillRoots, installed map[string]string) ([]Resolved, error) {
	resolved := make([]Resolved, 0, len(d.Phases))
	for _, phase := range d.Phases {
		one, err := resolve(phase, roots, installed)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, one)
	}
	return resolved, nil
}

func resolve(phase Phase, roots SkillRoots, installed map[string]string) (Resolved, error) {
	for _, candidate := range []struct {
		origin Origin
		root   string
	}{
		{FromWorkflow, roots.Own},
		{FromRepository, roots.Repository},
		{FromShared, roots.Shared},
	} {
		if candidate.root == "" {
			continue
		}
		skill := filepath.Join(candidate.root, phase.Name, SkillFile)
		// Any error is this root not providing the phase. A root that cannot be read is
		// the same as one that does not have it: the next root is tried, and a phase no
		// root answers for rejects the workflow below.
		if _, err := os.Stat(skill); err != nil {
			continue
		}
		assigned, err := model(phase, candidate.origin, installed)
		if err != nil {
			return Resolved{}, err
		}
		granted, err := tools(phase, candidate.origin)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{
			Name:   phase.Name,
			Skill:  skill,
			Origin: candidate.origin,
			Model:  assigned,
			Tools:  granted,
		}, nil
	}
	return Resolved{}, unresolved(phase.Name, roots)
}

// model: whichever side created the phase is the side that assigned it.
//
// A phase from the shared library takes the installation's assignment, because the
// installation is what created it. A phase the workflow or the repository brought takes
// the model on its entry in workflow.json and never consults the profile: a workflow's own
// review inheriting the shared review's assignment would be the installation choosing a
// model for a phase it has never seen.
//
// Declaring a model on a shared phase is therefore an error rather than a redundancy, and
// it rejects the workflow. Reporting it and resolving anyway was the alternative: that
// leaves a declaration the user wrote with no effect, mentioned once in a report nobody
// re-reads, which is the silent discard under another name. The check is here rather than
// in validation because where a phase resolves is not a fact the definition carries.
func model(phase Phase, origin Origin, installed map[string]string) (string, error) {
	if origin != FromShared {
		return phase.Model, nil
	}
	if phase.Model != "" {
		return "", fmt.Errorf("phase %q declares a model and resolves to the shared library, "+
			"where the model is the installation's assignment in profile.json", phase.Name)
	}
	return installed[phase.Name], nil
}

// tools is the same rule as model, about the other key, and the two are meant to read as
// one: whichever side created the phase is the side that says what it runs with.
//
// A phase from the shared library takes the installation's tool set, which is Alfred's base
// set for that phase plus whatever profile.json added to it; the generator reads it there
// and has never looked at the definition for it. So a `tools` declared on such a phase was
// validated against the tool pattern, carried this far, and then discarded with nothing
// said — the silent discard this change exists to end, in exactly the position the model
// was. It rejects the workflow for the same reason: a declaration the user wrote and the
// run ignores is worse than a rejection naming the phase.
func tools(phase Phase, origin Origin) ([]string, error) {
	if origin != FromShared {
		return phase.Tools, nil
	}
	if len(phase.Tools) > 0 {
		return nil, fmt.Errorf("phase %q declares a tool set and resolves to the shared "+
			"library, where the tool set is the installation's assignment", phase.Name)
	}
	return nil, nil
}

func unresolved(name string, roots SkillRoots) error {
	if roots.Repository == "" {
		return fmt.Errorf("phase %q resolves in no machine root; a phase only a repository "+
			"provides belongs to a repository-scope workflow", name)
	}
	return fmt.Errorf("phase %q resolves in neither the workflow, the repository nor the "+
		"shared library", name)
}
