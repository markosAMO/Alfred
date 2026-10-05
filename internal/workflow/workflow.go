// Package workflow reads the one file that says what a workflow is.
//
// The structural facts of a run — the phases it may run, its routes, which of them are
// dispatched together and which one closes a change — are read here, once, at
// registration, and rendered into the command that runs them. Nothing reads a definition
// again while a change is running, which is why a definition edited afterwards has no
// effect until registration runs again, and why a corrupted one cannot break a run in
// progress.
//
// That is also why the format is JSON decoded into a typed struct with unknown keys
// rejected. The alternative was a hand-scanned YAML subset, which is a parser whose only
// specification is the code that reads it, and which accepts documents YAML rejects, so
// the user's editor and Alfred would disagree about one file. Here a misspelled key is the
// decoder's rejection and a syntax error carries its position.
//
// A key the format reserves for a capability that does not exist yet is defined and
// refused by validation, never left unknown: an unknown key is reported as a typo, which
// is the wrong message for something that will be honoured later.
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

// DefinitionFile is the name every workflow directory carries.
const DefinitionFile = "workflow.json"

// SharedPhases are the phases the shared skill library provides. They are reserved as
// workflow names, because a workflow named after one would generate a command and a
// subagent that collide with the shared phase's own.
var SharedPhases = []string{
	"apply", "archive", "design", "diagnose", "explore", "init",
	"refine", "research", "review", "spec", "tasks", "verify",
}

// reservedPhases may not be declared as a phase of any workflow. Repository setup and
// exploration are commands of their own and belong to no workflow, so removing a workflow
// can never remove either command; a worktree is not a phase at all.
var reservedPhases = map[string]bool{"init": true, "explore": true, "worktree": true}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// modelPattern and toolPattern are what a phase may name as its own model and its own
// tools. Both values are written straight onto a generated Claude Code subagent's
// frontmatter, and at project scope a definition arrives with a clone and is registered by
// `init` or the scanner without anyone reading it, so both are held to a shape rather than
// taken at their word.
//
// A model identifier is a vendor-qualified name: letters and digits to open, then the
// punctuation vendors actually use — `.`, `_`, `:`, `/` and `-`. That admits
// `anthropic/claude-haiku-4-5-20251001` and `solo-model` and refuses the whole class the
// threat is: a newline, a tab, any other control character, a space, a comma and a quote.
// The writer quotes the `model:` line as well, so a value that is merely awkward would
// survive there; this is the half that says a value which can never be a model identifier
// is a mistake worth reporting rather than a scalar worth rendering.
//
// A tool name is held tighter, because `tools:` is the one frontmatter value the writer
// cannot quote: Claude Code reads it as a bare comma-separated list, and quoting the join
// would hand it one scalar where it has always been given a list. A tool name is therefore
// a letter followed by letters, digits, underscores and hyphens — every name Claude Code
// and OpenCode have, `Read`, `WebFetch`, `mcp__engram__mem_search` — and nothing that can
// change the line's shape: no comma to split one name into two, no colon to end the
// scalar, no newline to start a second key. Claude Code's scoped form, `Bash(git diff:*)`,
// is refused with them; Alfred has never generated one, OpenCode cannot read one, and
// admitting it would mean admitting the punctuation the rule exists to keep out.
var (
	modelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]*$`)
	toolPattern  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
)

// Phase is one step a workflow may run. It is an object rather than a bare name so that
// every per-step fact — the model, the tool set, the sub-workflow reference the format
// reserves — is a key here rather than a parallel structure keyed by phase name, which
// could disagree with the phase list.
type Phase struct {
	Name  string   `json:"name"`
	Model string   `json:"model,omitempty"`
	Tools []string `json:"tools,omitempty"`

	// Workflow is reserved for a step that triggers another workflow. It is refused by
	// validation with that reason, so a definition written today — which carries no such
	// key — is unaffected by the key ever being honoured.
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

// Load reads the definition of one workflow directory. The directory name is the name the
// definition has to agree with: it is what the user sees and what the command is named
// after.
func Load(dir string) (*Definition, error) {
	file, err := os.Open(filepath.Join(dir, DefinitionFile))
	if err != nil {
		return nil, fmt.Errorf("%s cannot be read: %w", DefinitionFile, err)
	}
	defer func() { _ = file.Close() }()

	return Decode(file, filepath.Base(dir))
}

// Decode reads one definition and returns it only when every structural fact holds. The
// error is the reason the registration report prints, so it is a sentence about this
// workflow and carries no prefix naming it.
func Decode(r io.Reader, dirName string) (*Definition, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("cannot be read: %w", err)
	}

	d := &Definition{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(d); err != nil {
		return nil, readingError(err)
	}
	if decoder.More() {
		return nil, errors.New("malformed JSON: unexpected content after the definition")
	}

	// A second pass over the same bytes, for presence alone. `parallel` is required and may
	// be empty, and the decoded value cannot tell an empty list from a key nobody wrote.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, readingError(err)
	}

	if err := d.validate(raw, dirName); err != nil {
		return nil, err
	}
	return d, nil
}

// readingError turns what the decoder says into what the report prints. A syntax error
// carries its position, because a position is what sends the user to the right line.
func readingError(err error) error {
	if errors.Is(err, io.EOF) {
		return errors.New("the definition is empty")
	}
	// A truncated document has no position to report: the decoder ran out of input rather
	// than meeting a character it could not use.
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

	// encoding/json reports an unrecognised key as a plain error, so its wording is the
	// only thing to match on. Unmatched, the decoder's own sentence is still the truth.
	if key, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return fmt.Errorf("unknown key %s", key)
	}
	return err
}

// required is every key a definition has to declare, in the order a reader of the file
// would miss them.
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
	return d.validateReferences()
}

// validateDeclared is the one check that reads presence rather than value, and a value
// that is empty counts as a declaration nobody made — except for `parallel`, where an
// empty list is the declaration that nothing is grouped.
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

	for _, key := range required {
		if _, declared := raw[key.key]; !declared || empty[key.key] {
			return errors.New(key.reason)
		}
	}
	return nil
}

// validateRules checks the rules file against the directory the definition was found in.
// It is the one value a definition carries that becomes an absolute path and is handed to
// a model, so it is validated the way `docs/code_conventions.md` requires of exactly that
// case, with the `under()` pattern `cmd/alfred` uses.
//
// It is not part of validate: the decode reads one file and knows only the directory's
// name, and containment and existence are questions about a directory. The scan is what
// asks them, because the scan is what knows where the workflow is.
//
// Existence is checked here rather than left to the run because a workflow that registers
// and cannot then run is the first-use failure registration exists to prevent: the report
// exits 0 and every run under that command stops at a rules file it cannot read.
func (d *Definition) validateRules(dir string) error {
	// A control character is refused before the path is resolved at all, because the value
	// does not stop at the filesystem: the generated command names the rules file inside a
	// fence the orchestrator reads, so a newline in it adds a line of the definition
	// author's choosing to an instruction. A file of that name can be created and the
	// workflow then registers, which makes this the one key whose checks all passing is
	// not enough.
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

	// Containment again, with the symlinks resolved. The check above is lexical, and a
	// repository's workflows arrive with a clone, which can carry a link that is inside the
	// directory by name and outside it by target. Both ends are resolved because a
	// temporary directory is itself reached through a link on macOS.
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

// under answers whether path stays inside root. Both are already clean, so the comparison
// is the relative one: a path reaching the root's parent escapes it.
func under(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

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

// reservedNames are the names a workflow may not take: every shared phase, every command
// that belongs to no workflow, and the two agents.
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

func (d *Definition) validatePhases() error {
	seen := make(map[string]bool, len(d.Phases))
	for _, phase := range d.Phases {
		switch {
		case phase.Name == "":
			return errors.New("declares a phase with no name")
		// A phase name is a filename component before it is anything else: resolution joins
		// it onto a skill root and the generator joins it onto an agent directory. Held to
		// nothing, a name carrying `..` leaves both, and at project scope the definition
		// arrived with a clone. It answers to the pattern the workflow's own name does.
		case !namePattern.MatchString(phase.Name):
			return fmt.Errorf("declares a phase named %q, which does not match %s", phase.Name, namePattern)
		case reservedPhases[phase.Name]:
			return fmt.Errorf("declares a phase named %q, which is reserved", phase.Name)
		case seen[phase.Name]:
			return fmt.Errorf("declares phase %q twice", phase.Name)
		// A phase that assigns no model takes the installation's, and the empty string is
		// how it says so; a phase that names one has it written onto a frontmatter line.
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
		seen[phase.Name] = true
	}
	return nil
}

// validateReferences checks every name that addresses a phase or a route against what the
// definition declares. Maps are walked in sorted order: a reason whose wording depends on
// map iteration is a reason the golden recordings cannot pin.
func (d *Definition) validateReferences() error {
	declared := make(map[string]bool, len(d.Phases))
	for _, phase := range d.Phases {
		declared[phase.Name] = true
	}

	// A route's name is the third name a definition carries, and it answers to the pattern
	// the other two do. It is addressed by the user and by the orchestrator, printed in the
	// registration report and rendered into the generated command's prompt body, so holding
	// it more loosely than the workflow it belongs to buys nothing and costs a value that
	// can rewrite either. Every route's shape is settled before any of them is read as a
	// reference, so a definition carrying both faults is rejected for the one to fix first.
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

func sorted[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
