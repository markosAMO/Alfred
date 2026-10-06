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

// Targets is where one scope's registration writes, and it is the only thing that knows
// the two scopes apart.
//
// A machine run is handed the machine's agent directories and a project run the
// repository's, and nothing below branches on which it got: that is what makes an update
// incapable of touching a repository and a repository incapable of writing outside itself.
// An empty field is a target this installation does not have, and a run with neither agent
// writes nothing at all.
type Targets struct {
	Claude   string // the agent's directory; commands/ and agents/ sit under it
	Opencode string // the agent's configuration file, which it also owns
	Manifest string // the scope's generated.json
	Exclude  string // the repository's local exclude file; empty appends nothing
	Root     string // what an excluded path is written relative to, which git reads it against
}

// Entry is one generated thing, named the way the agent it was written for names it: a
// slash command on Claude Code, a bare agent key on OpenCode.
type Entry struct {
	Name    string
	Command bool
}

// Change is what one agent received in one scope.
//
// Added, Updated, Unchanged and Removed are Alfred's own output. Modified, Orphans and
// Collisions are the three ways something on disk is not, and they carry the path or the
// key rather than a display name, because the user has to go and look at them.
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

// Touched reports whether this run changed anything for this agent. A run whose roots
// changed in no way writes nothing, and the report says so per agent rather than listing
// everything it left alone.
func (c Change) Touched() bool {
	return len(c.Added) > 0 || len(c.Updated) > 0 || len(c.Removed) > 0
}

// Override is one repository workflow carrying the name of a machine workflow.
//
// It is reported because it is otherwise invisible: a user who types the machine's command
// in a repository that overrides that workflow gets the machine's, and the only thing that
// told them otherwise is this line.
type Override struct {
	Workflow string
	Command  string // the command to use in this repository
	Machine  string // the command the machine's workflow keeps
}

// Adoption is what a scope with no manifest proved was Alfred's own output.
//
// It is provenance rather than a fourth action: it says whose these are, which is the claim
// that licenses the additions and removals reported beside it. The same names appear there
// too, and that is one line saying whose they are and another saying what was done.
type Adoption struct {
	Files []string // Claude Code paths
	Keys  []string // OpenCode agent keys
}

// Written is the whole of what one registration run did to one scope.
//
// Manifest reports whether this run claims the scope's generated.json. It is false for a
// repository defining no workflow, which claims nothing and leaves no file, and the caller
// needs it because under `artifacts.committed: true` the manifest is excluded from outside
// this package: an exclude line is permanent, so one written for a file that is never
// written is never taken back.
type Written struct {
	Agents   []Change
	Adopted  Adoption
	Excluded []string
	Manifest bool
}

// Bookkeeping is the manifest state one agent's writing is done against.
//
// Apply is false for the report-only mode, which has to produce the identical report. One
// code path with a write gate rather than two that can drift: a second implementation of
// "what would have happened" is a second answer to the same question.
type Bookkeeping struct {
	Previous *generated.Manifest
	Record   *generated.Set
	Apply    bool
}

// Write registers one scope's generated set into that scope's targets.
func Write(set *Set, t Targets) (*Written, error) { return register(set, t, true) }

// Report produces exactly what Write would report, and writes nothing.
func Report(set *Set, t Targets) (*Written, error) { return register(set, t, false) }

func register(set *Set, t Targets, write bool) (*Written, error) {
	// No supported agent was detected. Nothing is written, and the caller reports that
	// rather than reporting success over an empty list.
	if t.Claude == "" && t.Opencode == "" {
		return &Written{}, nil
	}

	previous, err := generated.Read(t.Manifest)
	if err != nil {
		return nil, err
	}

	out := &Written{}

	// A scope with no manifest is every installation upgraded into the version that started
	// writing one. Nothing there is claimed, so without this every file Alfred itself wrote
	// is a collision, nothing is refreshed and the upgrade fails for doing its job.
	if previous == nil {
		reconstructed, adopted, err := adopt(t, set)
		if err != nil {
			return nil, err
		}
		previous, out.Adopted = reconstructed, adopted
	}

	plan := Bookkeeping{Previous: previous, Record: generated.NewSet(), Apply: write}

	var owned []string

	if t.Claude != "" {
		change, paths, err := ClaudeAgents(t.Claude, set, plan)
		if err != nil {
			return nil, err
		}
		out.Agents = append(out.Agents, change)
		owned = append(owned, paths...)
	}

	if t.Opencode != "" {
		config := Opencode(set)
		change, err := MergeOpencode(t.Opencode, config, plan)
		if err != nil {
			return nil, err
		}
		out.Agents = append(out.Agents, change)
		// The file is OpenCode's, and a scope that generates nothing into it owns no part
		// of it: a repository defining no workflow has nothing to exclude there either.
		if len(config.Agent) > 0 {
			owned = append(owned, t.Opencode)
		}
	}

	// The manifest records what this run claims, so the next one can take it back. A run
	// that generated nothing and took nothing back claims nothing and leaves no file: a
	// repository defining no workflow is left exactly as it was found.
	if t.Manifest != "" && (len(owned) > 0 || removedAnything(out)) {
		out.Manifest = true
		owned = append(owned, t.Manifest)
		current := plan.Record.Manifest()
		// A run that claims exactly what the last one claimed rewrites nothing, which is
		// what makes "a second run changes nothing" true of the bookkeeping too.
		if write && !reflect.DeepEqual(previous, current) {
			if err := current.Write(t.Manifest); err != nil {
				return nil, err
			}
		}
	}

	// The manifest records the machine's model identifiers, so it is never committed
	// whatever the repository decided about the rest; the caller is what decides whether
	// an exclude file was passed at all.
	excluded, err := exclude(t, owned, write)
	if err != nil {
		return nil, err
	}
	out.Excluded = excluded

	return out, nil
}

func removedAnything(w *Written) bool {
	for _, change := range w.Agents {
		if len(change.Removed) > 0 {
			return true
		}
	}
	return false
}

func exclude(t Targets, owned []string, write bool) ([]string, error) {
	if t.Exclude == "" || len(owned) == 0 {
		return nil, nil
	}

	paths := make([]string, 0, len(owned))
	for _, path := range owned {
		paths = append(paths, relativeTo(t.Root, path))
	}
	sort.Strings(paths)
	if !write {
		return paths, nil
	}
	if _, err := generated.Exclude(t.Exclude, paths); err != nil {
		return nil, err
	}
	return paths, nil
}

// relativeTo makes a path relative to the repository root, which is what git reads an
// exclude pattern against. A path that is not under the root is left as it is rather than
// turned into a chain of `..`, which git would not match anything with.
func relativeTo(root, path string) string {
	if root == "" {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}

// generatedMark is the first line of the prompt body of everything Alfred generates: the
// first line after the frontmatter of a Claude Code command or agent, and the first line of
// the prompt string of an OpenCode agent. It is in the body and in no frontmatter and no
// JSON schema, because whether either agent tolerates an unknown key there is agent
// behaviour this project cannot observe.
//
// It is what lets a later run tell its own output from the user's on a scope whose manifest
// was never written. It carries no version deliberately: a version in it would rewrite every
// file on every bump, report everything as updated on an upgrade that changed no template,
// and break "a second run changes nothing" across versions.
const generatedMark = "<!-- ALFRED:GENERATED -->"

// legacyOpening is how every template the generator before the mark could render begins -
// subagent, manage, orchestrator, fleet and coordinator alike.
const legacyOpening = "You are the Alfred "

// markPrompts prepends the mark to every prompt one scope generates.
//
// One pass over the finished set rather than a line in each renderer or at each write site:
// every generated prompt is a Command or a Subagent before it is bytes on either agent, so
// a template added later cannot ship unmarked.
func markPrompts(set *Set) {
	for i := range set.Commands {
		set.Commands[i].Prompt = generatedMark + "\n" + set.Commands[i].Prompt
	}
	for i := range set.Subagents {
		set.Subagents[i].Prompt = generatedMark + "\n" + set.Subagents[i].Prompt
	}
}

// Legacy is what the generator before the mark produced in one scope, under the names it
// produced them: `alfred` and `alfred-worktree` as commands, one agent per phase of the
// profile and `alfred-manage`, and the same names as keys on OpenCode.
//
// It is carried on the set because adoption happens in the writer and the profile is the
// only thing that knows which phases this installation has. It is empty at project scope:
// that generator only ever wrote into the machine's own agent directories, so a repository
// holding one of these names never got it from Alfred.
type Legacy struct {
	Commands []string
	Agents   []string
}

func legacyNames(p *Profile, scope Scope) Legacy {
	if scope != Machine {
		return Legacy{}
	}
	subagents := []string{"alfred-manage"}
	for _, phase := range p.PhaseNames() {
		subagents = append(subagents, "alfred-"+phase)
	}
	return Legacy{Commands: []string{"alfred", "alfred-worktree"}, Agents: subagents}
}

// adopt reconstructs the manifest an earlier version never wrote.
//
// It runs only on a scope that has no manifest, and the run that uses it writes one, so it
// is a one-time migration that deletes itself. Without it the collision rule breaks every
// upgrade: on an existing installation nothing is claimed, so every file Alfred itself wrote
// is a name the user holds, none is refreshed and the upgrade exits non-zero for doing
// exactly what it was meant to do.
//
// Expressing it as a reconstructed manifest rather than as a special case in the writer buys
// two things. Adopted files are refreshed by the ordinary path, and the names this version no
// longer generates - `alfred-init` and `alfred-explore` as subagents - are claimed here and
// taken back by the ordinary removal pass on that same run.
func adopt(t Targets, set *Set) (*generated.Manifest, Adoption, error) {
	claimed := generated.NewSet()
	var adopted Adoption

	if t.Claude != "" {
		for _, dir := range []struct {
			path  string
			files map[string]bool
		}{
			{filepath.Join(t.Claude, "commands"), markdownNames(set.Legacy.Commands)},
			{filepath.Join(t.Claude, "agents"), markdownNames(set.Legacy.Agents)},
		} {
			present, err := generated.Scan(dir.path)
			if err != nil {
				return nil, Adoption{}, err
			}
			for _, path := range present {
				content, err := os.ReadFile(path)
				if err != nil {
					return nil, Adoption{}, fmt.Errorf("reading %s: %w", path, err)
				}
				if !ours(filepath.Base(path), promptBody(content), dir.files) {
					continue
				}
				claimed.AddFile(AgentClaude, path, content)
				adopted.Files = append(adopted.Files, path)
			}
		}
	}

	if t.Opencode != "" {
		keys, err := adoptOpencode(t.Opencode, claimed, set.Legacy)
		if err != nil {
			return nil, Adoption{}, err
		}
		adopted.Keys = keys
	}

	sort.Strings(adopted.Files)
	sort.Strings(adopted.Keys)
	return claimed.Manifest(), adopted, nil
}

// adoptOpencode is the same claim over the keys of the file OpenCode owns. The value is
// compacted before it is hashed, exactly as MergeOpencode compacts it, or the reconstructed
// hash would never match the one the next comparison computes.
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

// ours reports whether what sits at a generated name was written by Alfred rather than by
// the user. Anything it says no about is a collision: not written, reported.
func ours(name, body string, legacy map[string]bool) bool {
	if firstLine(body) == generatedMark {
		return true
	}
	// The migration, and both halves of it are required. The name alone would adopt a file
	// the user replaced wholesale at a name Alfred happens to generate. The opening alone
	// would adopt a file the user wrote by copying one of Alfred's and giving it a name of
	// their own, which is the likeliest way a user comes to own a file full of Alfred's
	// prose, and which the name half excludes outright.
	return legacy[name] && strings.HasPrefix(body, legacyOpening)
}

// promptBody is the prompt of a Claude Code command or agent: everything after the
// frontmatter block, which is where the mark lives.
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

// opencodePrompt is the prompt of one entry under `agent`. A value that is not an agent
// object is not Alfred's and is not claimed: somebody else's JSON in a file OpenCode owns is
// not this run's business and is not a failure either.
func opencodePrompt(value []byte) string {
	var entry struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(value, &entry); err != nil {
		return ""
	}
	return entry.Prompt
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}

func markdownNames(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name+".md"] = true
	}
	return out
}

func asSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}

// ClaudeAgents writes one scope's commands and subagents into a Claude Code directory, and
// returns what it did along with every path it owns there.
//
// Removal is this side's too, which it has never had: before this change nothing removed a
// Claude agent at all, so a workflow that was deleted kept a working command.
func ClaudeAgents(dir string, set *Set, plan Bookkeeping) (Change, []string, error) {
	commands := filepath.Join(dir, "commands")
	subagents := filepath.Join(dir, "agents")

	type file struct {
		path  string
		body  []byte
		entry Entry
	}
	var files []file

	for _, c := range set.Commands {
		if !c.For(AgentClaude) {
			continue
		}
		files = append(files, file{
			path:  filepath.Join(commands, c.Name+".md"),
			body:  []byte(claudeCommand(c)),
			entry: Entry{Name: "/" + c.Name, Command: true},
		})
	}
	for _, s := range set.Subagents {
		files = append(files, file{
			path:  filepath.Join(subagents, s.Name+".md"),
			body:  []byte(claudeSubagent(s)),
			entry: Entry{Name: s.Name},
		})
	}

	change := Change{Agent: AgentClaude, Target: dir}
	for _, f := range files {
		plan.Record.AddFile(AgentClaude, f.path, f.body)
	}

	// What is on disk is read before anything is written: a file this run generates that
	// was already there and is claimed by nobody is the user's, and saying so afterwards
	// would be saying it about Alfred's own output.
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

	// A name the user already holds is the user's. The generated content for it is not
	// written, the manifest does not claim it, and it is not Added, Updated or Unchanged:
	// those four describe Alfred's own output, and calling an overwrite a refresh is what
	// let a file be destroyed and reported as a success.
	colliding := asSet(sweep.Collisions)
	plan.Record.ForgetFiles(AgentClaude, sweep.Collisions)

	owned := make([]string, 0, len(files))
	for _, f := range files {
		if colliding[f.path] {
			continue
		}
		owned = append(owned, f.path)

		existing, err := os.ReadFile(f.path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			change.Added = append(change.Added, f.entry)
		case err != nil:
			return Change{}, nil, fmt.Errorf("reading %s: %w", f.path, err)
		case bytes.Equal(existing, f.body):
			change.Unchanged = append(change.Unchanged, f.entry)
			continue
		default:
			change.Updated = append(change.Updated, f.entry)
		}
		if !plan.Apply {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			return Change{}, nil, err
		}
		if err := os.WriteFile(f.path, f.body, 0o644); err != nil {
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

// claudeEntry names a path the way the agent does, which is how it was added and so how a
// user recognises it going.
func claudeEntry(path string) Entry {
	name := strings.TrimSuffix(filepath.Base(path), ".md")
	if filepath.Base(filepath.Dir(path)) == "commands" {
		return Entry{Name: "/" + name, Command: true}
	}
	return Entry{Name: name}
}

// yamlScalar renders a value as a YAML double-quoted scalar.
//
// A command's description is the `description` of a workflow definition, and at project
// scope a definition arrives with a clone and is registered by `init` without anyone
// reading it. Interpolated plain, a value holding a newline ends the scalar and every line
// after it becomes a further frontmatter key: `verify` watched a definition add
// `allowed-tools`, a key Alfred never writes, and so choose the command's permissions.
//
// Go's quoting escapes what YAML's double-quoted style escapes — the quote, the backslash
// and every control character, as `\n`, `\t` and `\u` forms YAML reads the same way — so
// the result is one scalar whatever the value holds. Quoting rather than refusing keeps a
// description that is merely awkward, a colon or a leading `-`, from costing a workflow its
// registration.
func yamlScalar(v string) string { return strconv.Quote(v) }

func claudeCommand(c Command) string {
	return "---\n" +
		"description: " + yamlScalar(c.Description) + "\n" +
		"argument-hint: " + c.ArgumentHint + "\n" +
		"model: " + bareModel(c.Model) + "\n" +
		"tools: " + strings.Join(c.Tools, ", ") + "\n" +
		"---\n\n" +
		c.Prompt + "\n\n" +
		"## The request\n\n" +
		"$ARGUMENTS\n"
}

// claudeSubagent writes one phase executor. Every free-text value on the block is quoted,
// for the reason yamlScalar gives: `model` is `phases[].model` from a definition, `effort`
// comes from the profile, and `description` is a constant that carries a colon and a space
// and so is not a plain scalar either — one defect class reaching three lines.
//
// Two values stay bare, and each for its own reason. `name` is the file's own basename and
// every component of it answers to `^[a-z][a-z0-9-]*$`, so there is no free text in it to
// quote. `tools` is a list rather than a scalar: Claude Code splits the line on commas,
// and quoting the join would hand it one string where it has always been given a list. A
// tool name is kept safe where it is read instead, by workflow.toolPattern, which admits
// no character that could end the line or split it.
func claudeSubagent(s Subagent) string {
	effort := ""
	if s.Effort != "" {
		effort = "effort: " + yamlScalar(s.Effort) + "\n"
	}
	return "---\n" +
		"name: " + s.Name + "\n" +
		"description: " + yamlScalar(s.Description) + "\n" +
		"model: " + yamlScalar(bareModel(s.Model)) + "\n" +
		effort +
		"tools: " + strings.Join(s.Tools, ", ") + "\n" +
		"---\n\n" +
		s.Prompt + "\n"
}

// asBooleans renders a tool list as OpenCode's per-tool flags: every tool it knows about
// appears, enabled or disabled, so a tool is never inherited by omission.
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

const opencodeSchema = "https://opencode.ai/config.json"

// Opencode builds the generated half of opencode.json from one scope's set. A command is a
// primary agent there and a subagent a hidden one, which is the whole of the difference:
// both live in one `agent` map, which is why a standalone command and a phase executor can
// never share a name.
func Opencode(set *Set) *OpencodeConfig {
	agent := map[string]opencodeAgent{}

	for _, c := range set.Commands {
		if !c.For(AgentOpencode) {
			continue
		}
		entry := opencodeAgent{
			Model:       c.Model,
			Mode:        "primary",
			Description: c.Description,
			Prompt:      c.Prompt,
			Tools:       asBooleans(c.Tools, c.Delegates),
		}
		// A command that delegates nothing needs no permission to: the block would grant
		// a tool it was not given.
		if c.Delegates {
			entry.Permission = &opencodePermission{
				Task: map[string]string{"*": "deny", "alfred-*": "allow"},
			}
		}
		agent[c.Name] = entry
	}

	for _, s := range set.Subagents {
		agent[s.Name] = opencodeAgent{
			Model:       s.Model,
			Mode:        "subagent",
			Hidden:      true,
			Description: s.Description,
			Prompt:      s.Prompt,
			Tools:       asBooleans(s.Tools, false),
		}
	}

	return &OpencodeConfig{Schema: opencodeSchema, Agent: agent}
}

// MergeOpencode writes one scope's agents into the file OpenCode owns, replacing only what
// this scope's manifest claims and leaving every other key where it was.
//
// What it no longer does is delete every alfred-* key it did not generate. That took an
// agent the user wrote and happened to call alfred-notes, so an unclaimed key is now
// reported and kept.
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

	// The values in the file carry the indentation Format gave them, and the ones this run
	// produces are compact. Both sides are compacted before they are compared or hashed, or
	// every key would read as changed on every run and nothing would ever be removable.
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

	// The plan is asked for before anything is classified, because a key the user already
	// holds is not one of the four things Alfred did: it is skipped here and reported as a
	// collision, exactly as the colliding file is on the other agent.
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

	// Nothing to add, update or take back leaves the file exactly as it was found, which
	// is also how a repository defining no workflow never gets an opencode.json created
	// for it.
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

func compact(raw []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func sortedAgentNames(agent map[string]opencodeAgent) []string {
	names := make([]string, 0, len(agent))
	for name := range agent {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Localise gives a repository's copy of a colliding workflow a distinguishing name, per
// docs/changes/custom-workflows/inputs/command-precedence.md.
//
// Whether an agent resolves a project-scope command before a machine-scope one of the same
// name was never observed on either agent, so a generated name that relies on it would
// rely on something nobody measured. A distinct name collides with nothing, whichever way
// precedence resolves, and the report says which one to use.
//
// Only a collision triggers it: a repository workflow the machine does not have keeps the
// plain command, because there is no second command of that name. A phase resolved from
// the shared library keeps alfred-<phase> in both scopes - it encodes no workflow, and on
// one machine it carries the same skill path and the same model whichever scope wrote it.
func Localise(set *Set, machine []string) []Override {
	colliding := map[string]bool{}
	for _, name := range machine {
		colliding[name] = true
	}

	var overrides []Override
	workflows := make([]string, 0, len(set.Commands))
	rename := map[string]string{}
	// A workflow is one command per agent wherever its prompt differs, so the override is
	// the workflow's and is reported once however many agents carry it.
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
		// A subagent belongs to the longest workflow name it is prefixed by: two workflows
		// where one name extends the other would otherwise both claim the same agent.
		owner := ""
		for _, name := range workflows {
			if strings.HasPrefix(set.Subagents[i].Name, "alfred-"+name+"-") && len(name) > len(owner) {
				owner = name
			}
		}
		if owner == "" || !colliding[owner] {
			continue
		}
		was := set.Subagents[i].Name
		set.Subagents[i].Name = "alfred-" + owner + "-local" + strings.TrimPrefix(was, "alfred-"+owner)
		rename[was] = set.Subagents[i].Name
	}

	// The command dispatches its phases by name, so the prompt has to name what was
	// written. One pass over each prompt, longest name first: every new name contains the
	// old one, and a second pass would rename what the first just produced.
	if len(rename) > 0 {
		pairs := make([]string, 0, len(rename)*2)
		for _, was := range longestFirst(rename) {
			pairs = append(pairs, was, rename[was])
		}
		replacer := strings.NewReplacer(pairs...)
		for i := range set.Commands {
			set.Commands[i].Prompt = replacer.Replace(set.Commands[i].Prompt)
		}
	}

	sort.Slice(overrides, func(i, j int) bool { return overrides[i].Workflow < overrides[j].Workflow })
	return overrides
}

func localName(command string) string { return command + "-local" }

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
