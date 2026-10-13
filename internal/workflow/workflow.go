// Package workflow reads and validates a workflow's definition file (workflow.json).
//
// A definition is read once, at registration, and rendered into the command that runs it;
// nothing reads it again during a run. Unknown keys are rejected so a typo is reported.
package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// DefinitionFile is the name of the definition file inside every workflow directory.
const DefinitionFile = "workflow.json"

// SharedPhases are the phases the shared skill library provides. They are reserved as
// workflow names so a workflow's command and subagent never collide with a shared phase's.
var SharedPhases = []string{
	"apply", "archive", "design", "diagnose", "explore", "init",
	"refine", "research", "review", "spec", "tasks", "verify",
}

// reservedPhases may not be declared as a phase of any workflow.
var reservedPhases = map[string]bool{"init": true, "explore": true, "worktree": true}

// namePattern is the shape every workflow, phase, route and artifact name must have.
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ProjectArtifacts are the repository-wide documents a phase may read but never writes.
var ProjectArtifacts = []string{"architecture", "conventions", "specs"}

// isProjectArtifact reports whether name is one of the ProjectArtifacts.
func isProjectArtifact(name string) bool {
	for _, artifact := range ProjectArtifacts {
		if artifact == name {
			return true
		}
	}
	return false
}

// modelPattern and toolPattern are the shapes a phase's model and tool names must have.
// Both are written into generated subagent frontmatter, so they block injection: `tools:`
// cannot be quoted, so a tool name may not contain commas, colons, spaces or newlines.
var (
	modelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]*$`)
	toolPattern  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
)

// Phase is one step a workflow may run, with its optional model, tools, the artifacts it
// reads and the memory types it recalls.
type Phase struct {
	Name  string   `json:"name"`
	Model string   `json:"model,omitempty"`
	Tools []string `json:"tools,omitempty"`

	// Reads names the artifacts the phase is handed when it starts (other phases of this
	// workflow, or project artifacts); Recall names the memory types it searches.
	Reads  []string `json:"reads,omitempty"`
	Recall []string `json:"recall,omitempty"`

	// Workflow is reserved for a step that triggers another workflow; validation refuses it.
	Workflow string `json:"workflow,omitempty"`
}

// Definition is a workflow's structural facts, exactly as its workflow.json declares them.
type Definition struct {
	Name         string              `json:"name"`
	Title        string              `json:"title"`
	Description  string              `json:"description"`
	Rules        string              `json:"rules"`
	Phases       []Phase             `json:"phases"`
	Routes       map[string][]string `json:"routes"`
	DefaultRoute string              `json:"default_route"`
	EntryPoints  map[string]string   `json:"entry_points"`
	Parallel     [][]string          `json:"parallel"`
	Closes       string              `json:"closes"`
}

// Load reads and validates the definition in one workflow directory. The directory name is
// the name the definition must declare.
func Load(dir string) (*Definition, error) {
	file, err := os.Open(filepath.Join(dir, DefinitionFile))
	if err != nil {
		return nil, fmt.Errorf("%s cannot be read: %w", DefinitionFile, err)
	}
	defer func() { _ = file.Close() }()

	return Decode(file, filepath.Base(dir))
}

// Decode parses one definition and returns it only when every structural check passes.
// The error is printed as-is in the registration report, so it carries no name prefix.
func Decode(reader io.Reader, dirName string) (*Definition, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("cannot be read: %w", err)
	}

	definition := &Definition{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(definition); err != nil {
		return nil, readingError(err)
	}
	if decoder.More() {
		return nil, errors.New("malformed JSON: unexpected content after the definition")
	}

	// Second pass for key presence: an empty `parallel` list is valid, a missing one is not.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, readingError(err)
	}

	if err := definition.validate(raw, dirName); err != nil {
		return nil, err
	}
	return definition, nil
}

// readingError turns a JSON decoder error into the sentence the report prints, including
// the byte position for syntax errors.
func readingError(err error) error {
	if errors.Is(err, io.EOF) {
		return errors.New("the definition is empty")
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return errors.New("malformed JSON: the definition ends before it is closed")
	}

	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return fmt.Errorf("malformed JSON at byte %d: %s", syntax.Offset, syntax.Error())
	}

	var mismatch *json.UnmarshalTypeError
	if errors.As(err, &mismatch) {
		if mismatch.Field == "" {
			return fmt.Errorf("the definition is not a JSON object: found %s (byte %d)", mismatch.Value, mismatch.Offset)
		}
		return fmt.Errorf("key %q has the wrong type: found %s (byte %d)", mismatch.Field, mismatch.Value, mismatch.Offset)
	}

	// encoding/json reports an unknown key only as plain text, so match on its wording.
	if key, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return fmt.Errorf("unknown key %s", key)
	}
	return err
}

// required lists every key a definition must declare, with the reason reported when it is
// missing, in the order they are checked.
var required = []struct {
	key    string
	reason string
}{
	{"name", "declares no name"},
	{"title", "declares no title"},
	{"description", "declares no description"},
	{"rules", "declares no rules file"},
	{"phases", "declares no phases"},
	{"routes", "declares no routes"},
	{"default_route", "declares no default route"},
	{"entry_points", "declares no entry points"},
	{"parallel", `declares no "parallel"; a workflow with no group dispatched together declares an empty list`},
	{"closes", "declares no closing phase, which is required"},
}

// validate runs every structural check on a decoded definition and returns the first
// failure. The rules file is checked separately by the scan (see validateRules).
func (d *Definition) validate(raw map[string]json.RawMessage, dirName string) error {
	if err := d.validateDeclared(raw); err != nil {
		return err
	}
	if err := d.validateName(dirName); err != nil {
		return err
	}
	if err := d.validatePhases(); err != nil {
		return err
	}
	if err := d.validateReferences(); err != nil {
		return err
	}
	return d.validateArtifacts()
}

// validateDeclared checks that every required key is present and non-empty. `parallel` is
// the exception: an empty list there means no phases run together.
func (d *Definition) validateDeclared(raw map[string]json.RawMessage) error {
	empty := map[string]bool{
		"name":          d.Name == "",
		"title":         d.Title == "",
		"description":   d.Description == "",
		"rules":         d.Rules == "",
		"phases":        len(d.Phases) == 0,
		"routes":        len(d.Routes) == 0,
		"default_route": d.DefaultRoute == "",
		"entry_points":  len(d.EntryPoints) == 0,
		"parallel":      false,
		"closes":        d.Closes == "",
	}

	for _, requirement := range required {
		if _, declared := raw[requirement.key]; !declared || empty[requirement.key] {
			return errors.New(requirement.reason)
		}
	}
	return nil
}

// validateRules checks that the rules file is a relative path that exists inside dir, also
// after resolving symlinks. Existence is checked now so a workflow never registers and then
// fails on its first run.
func (d *Definition) validateRules(dir string) error {
	// The path is rendered into the generated command, so a newline would inject a line.
	if strings.ContainsFunc(d.Rules, unicode.IsControl) {
		return fmt.Errorf("rules file %q carries a control character, which no name may hold", d.Rules)
	}

	if filepath.IsAbs(d.Rules) {
		return fmt.Errorf("rules file %q is an absolute path; it must be relative to the workflow's directory", d.Rules)
	}

	path := filepath.Join(dir, d.Rules)
	if !under(dir, path) {
		return fmt.Errorf("rules file %q resolves outside the workflow's directory", d.Rules)
	}

	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("rules file %q does not exist", d.Rules)
	case err != nil:
		return fmt.Errorf("rules file %q cannot be read: %w", d.Rules, err)
	case info.IsDir():
		return fmt.Errorf("rules file %q is a directory", d.Rules)
	}

	// Recheck containment with symlinks resolved: a cloned repository can carry a link that
	// points outside. Both ends are resolved because temp dirs are symlinked on macOS.
	realDir, dirErr := filepath.EvalSymlinks(dir)
	realPath, pathErr := filepath.EvalSymlinks(path)
	if dirErr != nil || pathErr != nil {
		return fmt.Errorf("rules file %q cannot be read: %w", d.Rules, errors.Join(dirErr, pathErr))
	}
	if !under(realDir, realPath) {
		return fmt.Errorf("rules file %q resolves outside the workflow's directory", d.Rules)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("rules file %q cannot be read: %w", d.Rules, err)
	}
	return file.Close()
}

// under reports whether path stays inside root. Both paths must already be clean.
func under(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// validateName checks the workflow's name: its shape, that it matches its directory, and
// that it is not reserved.
func (d *Definition) validateName(dirName string) error {
	if !namePattern.MatchString(d.Name) {
		return fmt.Errorf("name %q does not match %s", d.Name, namePattern)
	}
	if d.Name != dirName {
		return fmt.Errorf("name %q disagrees with its directory name %q", d.Name, dirName)
	}
	if reservedNames()[d.Name] {
		return fmt.Errorf("name %q is reserved", d.Name)
	}
	return nil
}

// reservedNames returns the names a workflow may not take: every shared phase, every
// command that belongs to no workflow, and the two agents.
func reservedNames() map[string]bool {
	names := map[string]bool{
		"alfred": true, "manage": true, "worktree": true,
		"init": true, "explore": true, "add-workflow": true, "workflows-scanner": true,
	}
	for _, phase := range SharedPhases {
		names[phase] = true
	}
	return names
}

// validatePhases checks each phase: name shape and uniqueness, reserved names, model and
// tool shapes, the unsupported `workflow` key, and its reads/recall names.
func (d *Definition) validatePhases() error {
	seen := make(map[string]bool, len(d.Phases))
	for _, phase := range d.Phases {
		switch {
		case phase.Name == "":
			return errors.New("declares a phase with no name")
		// The name becomes a path component, so `..` or similar must never get through.
		case !namePattern.MatchString(phase.Name):
			return fmt.Errorf("declares a phase named %q, which does not match %s", phase.Name, namePattern)
		case reservedPhases[phase.Name], isProjectArtifact(phase.Name):
			return fmt.Errorf("declares a phase named %q, which is reserved", phase.Name)
		case seen[phase.Name]:
			return fmt.Errorf("declares phase %q twice", phase.Name)
		// An empty model means "use the installation's"; a named one goes into frontmatter.
		case phase.Model != "" && !modelPattern.MatchString(phase.Model):
			return fmt.Errorf("phase %q declares a model %q, which does not match %s", phase.Name, phase.Model, modelPattern)
		case phase.Workflow != "":
			return fmt.Errorf("phase %q names a workflow; a step that references a workflow is not supported yet", phase.Name)
		}
		for _, tool := range phase.Tools {
			if !toolPattern.MatchString(tool) {
				return fmt.Errorf("phase %q declares a tool %q, which does not match %s", phase.Name, tool, toolPattern)
			}
		}
		if err := validateArtifactNames(phase); err != nil {
			return err
		}
		seen[phase.Name] = true
	}
	return nil
}

// validateArtifactNames checks that every name in a phase's reads and recall matches the
// name pattern and appears once, because each becomes a path component and a memory key.
func validateArtifactNames(phase Phase) error {
	for _, key := range []string{"reads", "recall"} {
		names := phase.Reads
		if key == "recall" {
			names = phase.Recall
		}
		seen := map[string]bool{}
		for _, name := range names {
			if !namePattern.MatchString(name) {
				return fmt.Errorf("phase %q %s %q, which does not match %s", phase.Name, key, name, namePattern)
			}
			if seen[name] {
				return fmt.Errorf("phase %q %s %q twice", phase.Name, key, name)
			}
			seen[name] = true
		}
	}
	return nil
}

// validateArtifacts checks that every read names another phase of this workflow or a
// project artifact.
func (d *Definition) validateArtifacts() error {
	declared := make(map[string]bool, len(d.Phases))
	for _, phase := range d.Phases {
		declared[phase.Name] = true
	}
	for _, phase := range d.Phases {
		for _, name := range phase.Reads {
			switch {
			case isProjectArtifact(name):
			case name == phase.Name:
				return fmt.Errorf("phase %q reads its own artifact", phase.Name)
			case !declared[name]:
				return fmt.Errorf("phase %q reads %q, which is neither one of its phases nor a project artifact (%s)",
					phase.Name, name, strings.Join(ProjectArtifacts, ", "))
			}
		}
	}
	return nil
}

// validateReferences checks that routes, the default route, entry points, parallel groups
// and the closing phase only name phases and routes the definition declares.
// Maps are walked in sorted order so the reported error is deterministic (golden tests).
func (d *Definition) validateReferences() error {
	declared := make(map[string]bool, len(d.Phases))
	for _, phase := range d.Phases {
		declared[phase.Name] = true
	}

	// Route names are rendered into the generated command, so they must match namePattern.
	// All names are checked before any route's contents, so that fault is reported first.
	for _, name := range sorted(d.Routes) {
		if !namePattern.MatchString(name) {
			return fmt.Errorf("declares a route named %q, which does not match %s", name, namePattern)
		}
	}

	for _, name := range sorted(d.Routes) {
		if len(d.Routes[name]) == 0 {
			return fmt.Errorf("route %q names no phase", name)
		}
		for _, phase := range d.Routes[name] {
			if !declared[phase] {
				return fmt.Errorf("route %q names phase %q, which it does not declare", name, phase)
			}
		}
	}

	if _, ok := d.Routes[d.DefaultRoute]; !ok {
		return fmt.Errorf("default route %q is not one of its routes", d.DefaultRoute)
	}

	for _, label := range sorted(d.EntryPoints) {
		if phase := d.EntryPoints[label]; !declared[phase] {
			return fmt.Errorf("entry point %q names phase %q, which it does not declare", label, phase)
		}
	}

	for i, group := range d.Parallel {
		if len(group) < 2 {
			return fmt.Errorf("parallel group %d declares fewer than two phases", i+1)
		}
		for _, phase := range group {
			if !declared[phase] {
				return fmt.Errorf("parallel group %d names phase %q, which it does not declare", i+1, phase)
			}
		}
	}

	if !declared[d.Closes] {
		return fmt.Errorf("closing phase %q is not one of its phases", d.Closes)
	}
	return nil
}

// sorted returns the keys of a string-keyed map in ascending order, for deterministic
// iteration.
func sorted[V any](table map[string]V) []string {
	keys := make([]string, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
