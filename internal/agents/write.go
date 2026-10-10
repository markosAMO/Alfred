package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/markosAMO/alfred/internal/generated"
	"github.com/markosAMO/alfred/internal/jsonobj"
)

// Targets is where one scope's registration writes. It is the only thing that differs
// between machine and project scope; nothing below branches on scope. An empty field is a
// target this installation does not have.
type Targets struct {
	Claude   string // the agent's directory; commands/ and agents/ sit under it
	Opencode string // the agent's configuration file, which it also owns
	Manifest string // the scope's generated.json
	Exclude  string // the repository's local exclude file; empty appends nothing
	Root     string // what an excluded path is written relative to, which git reads it against
}

// Entry is one generated item, named as its agent names it: "/name" for a Claude Code
// command, the bare key for an OpenCode agent.
type Entry struct {
	Name    string
	Command bool
}

// Change is what one agent received in one scope. Added, Updated, Unchanged and Removed are
// Alfred's own output; Modified, Orphans and Collisions are things on disk that are not,
// listed by path or key so the user can find them.
type Change struct {
	Agent      string
	Target     string
	Added      []Entry
	Updated    []Entry
	Unchanged  []Entry
	Removed    []Entry
	Modified   []string
	Orphans    []string
	Collisions []string
}

// Touched reports whether this run added, updated or removed anything for this agent.
func (c Change) Touched() bool {
	return len(c.Added) > 0 || len(c.Updated) > 0 || len(c.Removed) > 0
}

// Override is a repository workflow that shares a machine workflow's name, reported with
// the renamed command to use in the repository.
type Override struct {
	Workflow string
	Command  string // the command to use in this repository
	Machine  string // the command the machine's workflow keeps
}

// Adoption lists the files and keys that a scope with no manifest proved were written by
// Alfred, and so may be refreshed or removed.
type Adoption struct {
	Files []string // Claude Code paths
	Keys  []string // OpenCode agent keys
}

// Written is everything one registration run did to one scope. Manifest reports whether
// the run claims generated.json; the caller relies on it to avoid adding a permanent
// exclude line for a manifest that is never written.
type Written struct {
	Agents   []Change
	Adopted  Adoption
	Excluded []string
	Manifest bool
}

// Bookkeeping is the manifest state an agent's writing is checked against: the previous
// manifest, the set being recorded, and Apply, false in report-only mode. Report and Write
// share one code path gated by Apply so their reports cannot drift.
type Bookkeeping struct {
	Previous *generated.Manifest
	Record   *generated.Set
	Apply    bool
}

// Write registers one scope's generated set into that scope's targets.
func Write(set *Set, targets Targets) (*Written, error) { return register(set, targets, true) }

// Report produces exactly what Write would report, and writes nothing.
func Report(set *Set, targets Targets) (*Written, error) { return register(set, targets, false) }

// register writes (or, with write false, only plans) one scope's set into its targets,
// updates the manifest and the exclude file, and reports what it did.
func register(set *Set, targets Targets, write bool) (*Written, error) {
	// No supported agent detected: write nothing and let the caller report it.
	if targets.Claude == "" && targets.Opencode == "" {
		return &Written{}, nil
	}

	previous, err := generated.Read(targets.Manifest)
	if err != nil {
		return nil, err
	}

	written := &Written{}

	// No manifest yet (an upgrade from a version that wrote none): reconstruct one from the
	// files Alfred demonstrably wrote, or they would all be reported as collisions.
	if previous == nil {
		reconstructed, adopted, err := adopt(targets, set)
		if err != nil {
			return nil, err
		}
		previous, written.Adopted = reconstructed, adopted
	}

	plan := Bookkeeping{Previous: previous, Record: generated.NewSet(), Apply: write}

	var owned []string

	if targets.Claude != "" {
		change, paths, err := ClaudeAgents(targets.Claude, set, plan)
		if err != nil {
			return nil, err
		}
		written.Agents = append(written.Agents, change)
		owned = append(owned, paths...)
	}

	if targets.Opencode != "" {
		config := Opencode(set)
		change, err := MergeOpencode(targets.Opencode, config, plan)
		if err != nil {
			return nil, err
		}
		written.Agents = append(written.Agents, change)
		// opencode.json is only ours to exclude when this scope generates agents into it.
		if len(config.Agent) > 0 {
			owned = append(owned, targets.Opencode)
		}
	}

	// Write a manifest only when this run claims or removed something, so a repository
	// with no workflow is left exactly as it was found.
	if targets.Manifest != "" && (len(owned) > 0 || removedAnything(written)) {
		written.Manifest = true
		owned = append(owned, targets.Manifest)
		current := plan.Record.Manifest()
		// Skip rewriting an identical manifest, so a second run changes nothing.
		if write && !reflect.DeepEqual(previous, current) {
			if err := current.Write(targets.Manifest); err != nil {
				return nil, err
			}
		}
	}

	// The manifest holds machine model identifiers, so it is always excluded when an
	// exclude file is given; the caller decides whether one is.
	excluded, err := exclude(targets, owned, write)
	if err != nil {
		return nil, err
	}
	written.Excluded = excluded

	return written, nil
}

// removedAnything reports whether any agent had an entry removed in this run.
func removedAnything(written *Written) bool {
	for _, change := range written.Agents {
		if len(change.Removed) > 0 {
			return true
		}
	}
	return false
}

// exclude adds the owned paths, relative to the repository root and sorted, to the
// exclude file (only when write is true) and returns them.
func exclude(targets Targets, owned []string, write bool) ([]string, error) {
	if targets.Exclude == "" || len(owned) == 0 {
		return nil, nil
	}

	paths := make([]string, 0, len(owned))
	for _, path := range owned {
		paths = append(paths, relativeTo(targets.Root, path))
	}
	sort.Strings(paths)
	if !write {
		return paths, nil
	}
	if _, err := generated.Exclude(targets.Exclude, paths); err != nil {
		return nil, err
	}
	return paths, nil
}

// relativeTo makes a path relative to the repository root, as git reads exclude patterns.
// A path outside the root is returned unchanged, since a `..` pattern would match nothing.
func relativeTo(root, path string) string {
	if root == "" {
		return path
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return path
	}
	return relative
}

// generatedMark is the first line of every generated prompt body; it lets a run without a
// manifest tell Alfred's output from the user's. It carries no version on purpose, or every
// file would be rewritten on each release.
const generatedMark = "<!-- ALFRED:GENERATED -->"

// legacyOpening is how every prompt from the generator before the mark begins.
const legacyOpening = "You are the Alfred "

// markPrompts prepends generatedMark to every prompt in the set. Done once over the finished
// set so a template added later cannot ship unmarked.
func markPrompts(set *Set) {
	for i := range set.Commands {
		set.Commands[i].Prompt = generatedMark + "\n" + set.Commands[i].Prompt
	}
	for i := range set.Subagents {
		set.Subagents[i].Prompt = generatedMark + "\n" + set.Subagents[i].Prompt
	}
}

// Legacy names the commands and agents the generator before the mark produced in a scope.
// It is empty at project scope, since that generator only wrote machine-level files.
type Legacy struct {
	Commands []string
	Agents   []string
}

// legacyNames returns the names the pre-mark generator used at machine scope: `alfred`,
// `alfred-worktree`, `alfred-manage` and alfred-<phase> for each profile phase.
func legacyNames(profile *Profile, scope Scope) Legacy {
	if scope != Machine {
		return Legacy{}
	}
	subagents := []string{"alfred-manage"}
	for _, phase := range profile.PhaseNames() {
		subagents = append(subagents, "alfred-"+phase)
	}
	return Legacy{Commands: []string{"alfred", "alfred-worktree"}, Agents: subagents}
}

// adopt reconstructs a manifest for a scope that has none, claiming every file and key
// that ours recognises. It is a one-time migration: the same run then writes a manifest.
// Claimed names no longer generated are removed by the ordinary removal pass.
func adopt(targets Targets, set *Set) (*generated.Manifest, Adoption, error) {
	claimed := generated.NewSet()
	var adopted Adoption

	if targets.Claude != "" {
		for _, directory := range []struct {
			path  string
			files map[string]bool
		}{
			{filepath.Join(targets.Claude, "commands"), markdownNames(set.Legacy.Commands)},
			{filepath.Join(targets.Claude, "agents"), markdownNames(set.Legacy.Agents)},
		} {
			present, err := generated.Scan(directory.path)
			if err != nil {
				return nil, Adoption{}, err
			}
			for _, path := range present {
				content, err := os.ReadFile(path)
				if err != nil {
					return nil, Adoption{}, fmt.Errorf("reading %s: %w", path, err)
				}
				if !ours(filepath.Base(path), promptBody(content), directory.files) {
					continue
				}
				claimed.AddFile(AgentClaude, path, content)
				adopted.Files = append(adopted.Files, path)
			}
		}
	}

	if targets.Opencode != "" {
		keys, err := adoptOpencode(targets.Opencode, claimed, set.Legacy)
		if err != nil {
			return nil, Adoption{}, err
		}
		adopted.Keys = keys
	}

	sort.Strings(adopted.Files)
	sort.Strings(adopted.Keys)
	return claimed.Manifest(), adopted, nil
}

// adoptOpencode claims the OpenCode agent keys that ours recognises. Values are compacted
// exactly as MergeOpencode does, or the recorded hash would never match on the next run.
func adoptOpencode(target string, claimed *generated.Set, legacy Legacy) ([]string, error) {
	data, err := os.ReadFile(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", target, err)
	}

	existing, err := jsonobj.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", target, err)
	}

	names := asSet(append(append([]string(nil), legacy.Commands...), legacy.Agents...))
	agents := existing.Child("agent")

	var adopted []string
	for _, key := range agents.Keys() {
		if !generated.Named(key) {
			continue
		}
		raw, _ := agents.Get(key)
		value, err := compact(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", target, key, err)
		}
		if !ours(key, opencodePrompt(value), names) {
			continue
		}
		claimed.AddKey(AgentOpencode, key, value)
		adopted = append(adopted, key)
	}
	return adopted, nil
}

// ours reports whether the content at a generated name was written by Alfred: it carries
// the mark, or has both a legacy name and the legacy opening. Anything else is the user's.
func ours(name, body string, legacy map[string]bool) bool {
	if firstLine(body) == generatedMark {
		return true
	}
	// Both checks are required: a name alone would adopt a user's replacement file, and the
	// opening alone would adopt a user's renamed copy of an Alfred file.
	return legacy[name] && strings.HasPrefix(body, legacyOpening)
}

// promptBody returns the prompt of a Claude Code command or agent file: everything after
// the frontmatter block, where the mark lives.
func promptBody(file []byte) string {
	const fence = "---\n"
	text := string(file)
	if !strings.HasPrefix(text, fence) {
		return text
	}
	_, rest, found := strings.Cut(text[len(fence):], "\n"+fence)
	if !found {
		return text
	}
	return strings.TrimPrefix(rest, "\n")
}

// opencodePrompt returns the prompt of one entry under `agent`, or "" when the value is not
// an agent object (which is then simply not claimed, not an error).
func opencodePrompt(value []byte) string {
	var entry struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(value, &entry); err != nil {
		return ""
	}
	return entry.Prompt
}

// firstLine returns text up to its first newline.
func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}

// markdownNames returns a lookup of "<name>.md" for each name.
func markdownNames(names []string) map[string]bool {
	lookup := make(map[string]bool, len(names))
	for _, name := range names {
		lookup[name+".md"] = true
	}
	return lookup
}

// asSet returns a lookup containing each name.
func asSet(names []string) map[string]bool {
	lookup := make(map[string]bool, len(names))
	for _, name := range names {
		lookup[name] = true
	}
	return lookup
}

// ClaudeAgents writes one scope's commands and subagents into a Claude Code directory,
// removes what earlier runs generated and this one does not, and returns the change and
// every path it owns there.
func ClaudeAgents(dir string, set *Set, plan Bookkeeping) (Change, []string, error) {
	commands := filepath.Join(dir, "commands")
	subagents := filepath.Join(dir, "agents")

	type generatedFile struct {
		path  string
		body  []byte
		entry Entry
	}
	var files []generatedFile

	for _, command := range set.Commands {
		if !command.For(AgentClaude) {
			continue
		}
		files = append(files, generatedFile{
			path:  filepath.Join(commands, command.Name+".md"),
			body:  []byte(claudeCommand(command)),
			entry: Entry{Name: "/" + command.Name, Command: true},
		})
	}
	for _, subagent := range set.Subagents {
		files = append(files, generatedFile{
			path:  filepath.Join(subagents, subagent.Name+".md"),
			body:  []byte(claudeSubagent(subagent)),
			entry: Entry{Name: subagent.Name},
		})
	}

	change := Change{Agent: AgentClaude, Target: dir}
	for _, file := range files {
		plan.Record.AddFile(AgentClaude, file.path, file.body)
	}

	// Scan disk before writing, so unclaimed pre-existing files are detected as the user's.
	var present []string
	for _, root := range []string{commands, subagents} {
		found, err := generated.Scan(root)
		if err != nil {
			return Change{}, nil, err
		}
		present = append(present, found...)
	}
	sweep, err := generated.PlanFiles(plan.Previous, plan.Record, AgentClaude, present)
	if err != nil {
		return Change{}, nil, err
	}

	// A colliding name belongs to the user: never write it, claim it, or report it as ours.
	colliding := asSet(sweep.Collisions)
	plan.Record.ForgetFiles(AgentClaude, sweep.Collisions)

	owned := make([]string, 0, len(files))
	for _, file := range files {
		if colliding[file.path] {
			continue
		}
		owned = append(owned, file.path)

		existing, err := os.ReadFile(file.path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			change.Added = append(change.Added, file.entry)
		case err != nil:
			return Change{}, nil, fmt.Errorf("reading %s: %w", file.path, err)
		case bytes.Equal(existing, file.body):
			change.Unchanged = append(change.Unchanged, file.entry)
			continue
		default:
			change.Updated = append(change.Updated, file.entry)
		}
		if !plan.Apply {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
			return Change{}, nil, err
		}
		if err := os.WriteFile(file.path, file.body, 0o644); err != nil {
			return Change{}, nil, err
		}
	}

	for _, path := range sweep.Remove {
		change.Removed = append(change.Removed, claudeEntry(path))
	}
	if plan.Apply {
		if err := generated.Remove(sweep.Remove); err != nil {
			return Change{}, nil, err
		}
	}
	change.Modified, change.Orphans, change.Collisions = sweep.Modified, sweep.Orphans, sweep.Collisions

	return change, owned, nil
}

// claudeEntry turns a generated Claude Code path back into the Entry it was reported as.
func claudeEntry(path string) Entry {
	name := strings.TrimSuffix(filepath.Base(path), ".md")
	if filepath.Base(filepath.Dir(path)) == "commands" {
		return Entry{Name: "/" + name, Command: true}
	}
	return Entry{Name: name}
}

// yamlScalar renders a value as a YAML double-quoted scalar. This prevents injection: an
// unquoted value with a newline (e.g. from an untrusted repository workflow) could add
// frontmatter keys such as `allowed-tools`. Go's quoting matches YAML's escapes.
func yamlScalar(value string) string { return strconv.Quote(value) }

// claudeCommand renders a Claude Code slash command file: frontmatter, prompt, and the
// $ARGUMENTS section.
func claudeCommand(command Command) string {
	return "---\n" +
		"description: " + yamlScalar(command.Description) + "\n" +
		"argument-hint: " + command.ArgumentHint + "\n" +
		"model: " + bareModel(command.Model) + "\n" +
		"tools: " + strings.Join(command.Tools, ", ") + "\n" +
		"---\n\n" +
		command.Prompt + "\n\n" +
		"## The request\n\n" +
		"$ARGUMENTS\n"
}

// claudeSubagent renders a Claude Code subagent file. Free-text values are quoted with
// yamlScalar to prevent frontmatter injection; `name` (validated to [a-z0-9-]) and `tools`
// (a comma list, validated by workflow.toolPattern) stay bare on purpose.
func claudeSubagent(subagent Subagent) string {
	effort := ""
	if subagent.Effort != "" {
		effort = "effort: " + yamlScalar(subagent.Effort) + "\n"
	}
	return "---\n" +
		"name: " + subagent.Name + "\n" +
		"description: " + yamlScalar(subagent.Description) + "\n" +
		"model: " + yamlScalar(bareModel(subagent.Model)) + "\n" +
		effort +
		"tools: " + strings.Join(subagent.Tools, ", ") + "\n" +
		"---\n\n" +
		subagent.Prompt + "\n"
}

// asBooleans renders a tool list as OpenCode's per-tool flags. Every known tool is listed,
// enabled or disabled, so none is inherited by omission.
func asBooleans(tools []string, allowTask bool) map[string]bool {
	flags := make(map[string]bool, len(opencodeNames))
	for _, name := range opencodeNames {
		flags[name] = false
	}
	for _, tool := range tools {
		if name, ok := opencodeNames[tool]; ok {
			flags[name] = true
		}
	}
	flags["task"] = allowTask
	return flags
}

// opencodePermission is the `permission` block of an OpenCode agent.
type opencodePermission struct {
	Task map[string]string `json:"task"`
}

// opencodeAgent is one entry under `agent` in opencode.json.
type opencodeAgent struct {
	Model       string              `json:"model"`
	Mode        string              `json:"mode"`
	Hidden      bool                `json:"hidden,omitempty"`
	Description string              `json:"description"`
	Prompt      string              `json:"prompt"`
	Permission  *opencodePermission `json:"permission,omitempty"`
	Tools       map[string]bool     `json:"tools"`
}

// OpencodeConfig is the generated half of opencode.json.
type OpencodeConfig struct {
	Schema string                   `json:"$schema"`
	Agent  map[string]opencodeAgent `json:"agent"`
}

// opencodeSchema is the `$schema` URL written into a new opencode.json.
const opencodeSchema = "https://opencode.ai/config.json"

// Opencode builds the generated part of opencode.json: each command becomes a primary
// agent and each subagent a hidden one, all in one `agent` map.
func Opencode(set *Set) *OpencodeConfig {
	agent := map[string]opencodeAgent{}

	for _, command := range set.Commands {
		if !command.For(AgentOpencode) {
			continue
		}
		entry := opencodeAgent{
			Model:       command.Model,
			Mode:        "primary",
			Description: command.Description,
			Prompt:      command.Prompt,
			Tools:       asBooleans(command.Tools, command.Delegates),
		}
		// Only a delegating command gets a task permission; otherwise it would grant Task.
		if command.Delegates {
			entry.Permission = &opencodePermission{
				Task: map[string]string{"*": "deny", "alfred-*": "allow"},
			}
		}
		agent[command.Name] = entry
	}

	for _, subagent := range set.Subagents {
		agent[subagent.Name] = opencodeAgent{
			Model:       subagent.Model,
			Mode:        "subagent",
			Hidden:      true,
			Description: subagent.Description,
			Prompt:      subagent.Prompt,
			Tools:       asBooleans(subagent.Tools, false),
		}
	}

	return &OpencodeConfig{Schema: opencodeSchema, Agent: agent}
}

// MergeOpencode writes one scope's agents into opencode.json, replacing only keys the
// manifest claims. Unclaimed keys, even alfred-* ones, are the user's: reported and kept.
func MergeOpencode(target string, config *OpencodeConfig, plan Bookkeeping) (Change, error) {
	existing := jsonobj.New()
	if data, err := os.ReadFile(target); err == nil {
		if existing, err = jsonobj.Parse(data); err != nil {
			return Change{}, fmt.Errorf("%s: %w", target, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Change{}, fmt.Errorf("reading %s: %w", target, err)
	}

	agents := existing.Child("agent")

	// Compact both sides before comparing or hashing, or formatting differences would make
	// every key look changed on every run.
	present := map[string][]byte{}
	for _, key := range agents.Keys() {
		raw, _ := agents.Get(key)
		compacted, err := compact(raw)
		if err != nil {
			return Change{}, fmt.Errorf("%s: %s: %w", target, key, err)
		}
		present[key] = compacted
	}

	change := Change{Agent: AgentOpencode, Target: target}
	generatedValues := make(map[string]json.RawMessage, len(config.Agent))
	for _, name := range sortedAgentNames(config.Agent) {
		raw, err := jsonobj.Raw(config.Agent[name])
		if err != nil {
			return Change{}, err
		}
		generatedValues[name] = raw
		plan.Record.AddKey(AgentOpencode, name, raw)
	}

	// Plan before classifying, so keys the user holds are skipped and reported as collisions.
	sweep := generated.PlanKeys(plan.Previous, plan.Record, AgentOpencode, present)
	colliding := asSet(sweep.Collisions)
	plan.Record.ForgetKeys(AgentOpencode, sweep.Collisions)

	for _, name := range sortedAgentNames(config.Agent) {
		if colliding[name] {
			continue
		}
		entry := Entry{Name: name, Command: config.Agent[name].Mode == "primary"}
		switch before, there := present[name]; {
		case !there:
			change.Added = append(change.Added, entry)
		case bytes.Equal(before, generatedValues[name]):
			change.Unchanged = append(change.Unchanged, entry)
		default:
			change.Updated = append(change.Updated, entry)
		}
	}

	for _, key := range sweep.Remove {
		change.Removed = append(change.Removed, Entry{Name: key})
	}
	change.Modified, change.Orphans, change.Collisions = sweep.Modified, sweep.Orphans, sweep.Collisions

	// Nothing to change: leave the file untouched (and never create one for no workflows).
	if !change.Touched() || !plan.Apply {
		return change, nil
	}

	if _, ok := existing.Get("$schema"); !ok && existing.Len() == 0 {
		// A new file starts with the schema, as OpenCode writes its own.
		schema, err := jsonobj.Raw(config.Schema)
		if err != nil {
			return Change{}, err
		}
		existing.Set("$schema", schema)
	}

	for _, key := range sweep.Remove {
		agents.Delete(key)
	}
	for _, name := range sortedAgentNames(config.Agent) {
		if colliding[name] {
			continue
		}
		agents.Set(name, generatedValues[name])
	}
	existing.SetChild("agent", agents)

	if _, ok := existing.Get("$schema"); !ok {
		schema, err := jsonobj.Raw(config.Schema)
		if err != nil {
			return Change{}, err
		}
		existing.Set("$schema", schema)
	}

	data, err := jsonobj.Format(existing)
	if err != nil {
		return Change{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return Change{}, err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return Change{}, fmt.Errorf("writing %s: %w", target, err)
	}
	return change, nil
}

// compact returns raw JSON with insignificant whitespace removed.
func compact(raw []byte) ([]byte, error) {
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// sortedAgentNames returns the OpenCode agent keys sorted, so output is deterministic.
func sortedAgentNames(agent map[string]opencodeAgent) []string {
	names := make([]string, 0, len(agent))
	for name := range agent {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Localise renames a repository workflow that shares a machine workflow's name (and its
// own-phase subagents) with a `-local` suffix, since agents' precedence between scopes is
// unknown. Shared-phase subagents keep their names. It returns one Override per workflow.
func Localise(set *Set, machine []string) []Override {
	colliding := map[string]bool{}
	for _, name := range machine {
		colliding[name] = true
	}

	var overrides []Override
	workflows := make([]string, 0, len(set.Commands))
	rename := map[string]string{}
	// A workflow may have one command per agent; report its override only once.
	reported := map[string]bool{}

	for i := range set.Commands {
		name := strings.TrimPrefix(set.Commands[i].Name, "alfred-")
		workflows = append(workflows, name)
		if !colliding[name] {
			continue
		}
		set.Commands[i].Name = localName(set.Commands[i].Name)
		if reported[name] {
			continue
		}
		reported[name] = true
		overrides = append(overrides, Override{
			Workflow: name,
			Command:  "/" + localName("alfred-"+name),
			Machine:  "/alfred-" + name,
		})
	}
	if len(overrides) == 0 {
		return nil
	}

	for i := range set.Subagents {
		// The longest matching workflow prefix wins, so "a" and "a-b" do not both claim it.
		owner := ""
		for _, name := range workflows {
			if strings.HasPrefix(set.Subagents[i].Name, "alfred-"+name+"-") && len(name) > len(owner) {
				owner = name
			}
		}
		if owner == "" || !colliding[owner] {
			continue
		}
		oldName := set.Subagents[i].Name
		set.Subagents[i].Name = "alfred-" + owner + "-local" + strings.TrimPrefix(oldName, "alfred-"+owner)
		rename[oldName] = set.Subagents[i].Name
	}

	// Rewrite subagent names in command prompts in one pass, longest first: each new name
	// contains the old one, so repeated passes would rename twice.
	if len(rename) > 0 {
		pairs := make([]string, 0, len(rename)*2)
		for _, oldName := range longestFirst(rename) {
			pairs = append(pairs, oldName, rename[oldName])
		}
		replacer := strings.NewReplacer(pairs...)
		for i := range set.Commands {
			set.Commands[i].Prompt = replacer.Replace(set.Commands[i].Prompt)
		}
	}

	sort.Slice(overrides, func(i, j int) bool { return overrides[i].Workflow < overrides[j].Workflow })
	return overrides
}

// localName returns the repository-local name of a command.
func localName(command string) string { return command + "-local" }

// longestFirst returns the keys of rename, longest first and then alphabetically.
func longestFirst(rename map[string]string) []string {
	keys := make([]string, 0, len(rename))
	for key := range rename {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	return keys
}
