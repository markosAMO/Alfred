package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/markosAMO/alfred/internal/contract"
)

// SkillFile is the file that defines a phase inside its skill directory.
const SkillFile = "SKILL.md"

// Origin is the kind of root a phase resolved from: the workflow, the repository or the
// shared library. It decides the phase's command name and who assigns its model.
type Origin string

// The three roots a phase can resolve from.
const (
	FromWorkflow   Origin = "workflow"
	FromRepository Origin = "repository"
	FromShared     Origin = "shared"
)

// SkillRoots are the directories a phase may resolve from, tried in field order. Repository
// is empty for a machine-scope workflow.
type SkillRoots struct {
	Own        string
	Repository string
	Shared     string
}

// Resolved is one phase of a workflow with the skill file, origin, model and tools the
// generator needs to dispatch it.
type Resolved struct {
	Name   string
	Skill  string
	Origin Origin

	// Model is empty when nobody assigned one; that is reported, not an error, and no
	// fallback to the orchestrator's model is applied.
	Model string

	// Tools is the set a workflow or repository phase declares; empty means the default
	// set. It is always empty for a shared phase.
	Tools []string

	// Contract is what the phase's contract.json declares, read here so that nothing reads
	// it during a run; nil when the skill has none.
	Contract *contract.Contract
}

// Resolve locates every phase of a workflow in its skill roots and returns the first reason
// the workflow cannot be registered. installed maps phase names to installed models.
func Resolve(definition *Definition, roots SkillRoots, installed map[string]string) ([]Resolved, error) {
	resolved := make([]Resolved, 0, len(definition.Phases))
	for _, phase := range definition.Phases {
		one, err := resolve(phase, roots, installed)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, one)
	}
	return resolved, nil
}

// resolve finds the first root that holds the phase's skill file and returns the phase
// with its model and tools, or an error when no root provides it.
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
		// Any stat error means this root does not provide the phase; try the next one.
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
		declared, err := phaseContract(filepath.Dir(skill), candidate.origin, phase.Name, roots)
		if err != nil {
			return Resolved{}, fmt.Errorf("phase %q has an invalid contract: %w", phase.Name, err)
		}
		return Resolved{
			Name:     phase.Name,
			Skill:    skill,
			Origin:   candidate.origin,
			Model:    assigned,
			Tools:    granted,
			Contract: declared,
		}, nil
	}
	return Resolved{}, unresolved(phase.Name, roots)
}

// phaseContract reads the contract next to the skill a phase resolved to. A repository
// override with no contract.json inherits the shared skill's, so overriding the prose never
// lifts a rule by omission; an override that means to drop the rules says so with {}.
func phaseContract(dir string, origin Origin, phase string, roots SkillRoots) (*contract.Contract, error) {
	declared, err := contract.Load(dir)
	if err != nil || declared != nil || origin != FromRepository || roots.Shared == "" {
		return declared, err
	}
	return contract.Load(filepath.Join(roots.Shared, phase))
}

// model returns the phase's model: the installation's for a shared phase, the definition's
// otherwise. A model declared on a shared phase is an error, never silently ignored.
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

// tools returns the phase's declared tool set, or nil for a shared phase, whose tools the
// installation decides. Tools declared on a shared phase are an error, never ignored.
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

// unresolved builds the error for a phase that no root provides, worded for the scope.
func unresolved(name string, roots SkillRoots) error {
	if roots.Repository == "" {
		return fmt.Errorf("phase %q resolves in no machine root; a phase only a repository "+
			"provides belongs to a repository-scope workflow", name)
	}
	return fmt.Errorf("phase %q resolves in neither the workflow, the repository nor the "+
		"shared library", name)
}
