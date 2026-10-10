package workflow

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/markosAMO/alfred/internal/generated"
)

// reportWidth is the column the registration report wraps to.
const reportWidth = 90

// Scope is the side of the installation a run registers: the whole machine or one
// repository. It is always given on the command line, never derived.
type Scope string

// The two registration scopes.
const (
	ScopeMachine Scope = "machine"
	ScopeProject Scope = "project"
)

// Mode is what a run does with what it found: apply writes, report prints the same report
// without writing, and check lists the workflows whose command is missing.
type Mode string

// The three run modes.
const (
	ModeApply  Mode = "apply"
	ModeReport Mode = "report"
	ModeCheck  Mode = "check"
)

// Targets is where one scope's registration writes, as given on the command line.
// Exclude (keep every generated path out of git) and ExcludeManifest (keep only the
// manifest out, since it holds machine-specific paths and hashes) are mutually exclusive.
type Targets struct {
	Claude          string
	Opencode        string
	Manifest        string
	Root            string
	Exclude         string
	ExcludeManifest string
}

// Request is one parsed invocation of `alfred workflows`: scope, mode, every root it reads
// and every target it writes.
type Request struct {
	Scope   Scope
	Mode    Mode
	Profile string
	Skills  string

	// Machine scope only. Templates and Custom are also rendered into /alfred-add-workflow.
	Workflows string
	Custom    string
	Templates string

	// Project scope only. The machine roots are read, never written, to detect name clashes.
	LocalSkills      string
	LocalWorkflows   string
	MachineWorkflows string
	MachineCustom    string

	Targets Targets
}

// machineRootCount and projectRootCount are the root arguments each scope takes.
const (
	machineRootCount = 5
	projectRootCount = 6
)

// Parse reads the arguments of one `alfred workflows` invocation into a Request.
// Every error means the command was called wrongly (exit 2), never that the run failed.
func Parse(args []string) (*Request, error) {
	if len(args) == 0 {
		return nil, errors.New("expected machine or project")
	}

	request := &Request{Scope: Scope(args[0])}
	if request.Scope != ScopeMachine && request.Scope != ScopeProject {
		return nil, fmt.Errorf("unknown scope %q: expected machine or project", args[0])
	}

	rest := args[1:]
	if len(rest) == 0 {
		return nil, errors.New("expected apply, report or check")
	}
	request.Mode = Mode(rest[0])
	if request.Mode != ModeApply && request.Mode != ModeReport && request.Mode != ModeCheck {
		return nil, fmt.Errorf("unknown mode %q: expected apply, report or check", rest[0])
	}

	wanted := machineRootCount
	if request.Scope == ScopeProject {
		wanted = projectRootCount
	}
	roots := rest[1:]
	if len(roots) < wanted {
		return nil, fmt.Errorf("%s scope takes %d roots before the targets, and %d were given",
			request.Scope, wanted, len(roots))
	}

	request.Profile, request.Skills = roots[0], roots[1]
	if request.Scope == ScopeMachine {
		request.Workflows, request.Custom, request.Templates = roots[2], roots[3], roots[4]
	} else {
		request.LocalSkills, request.LocalWorkflows = roots[2], roots[3]
		request.MachineWorkflows, request.MachineCustom = roots[4], roots[5]
	}

	if err := request.targets(roots[wanted:]); err != nil {
		return nil, err
	}
	return request, nil
}

// targets parses the `kind=path` target arguments into r.Targets and checks that the
// combination is consistent.
func (r *Request) targets(raw []string) error {
	fields := map[string]*string{
		"claude":           &r.Targets.Claude,
		"opencode":         &r.Targets.Opencode,
		"manifest":         &r.Targets.Manifest,
		"root":             &r.Targets.Root,
		"exclude":          &r.Targets.Exclude,
		"exclude-manifest": &r.Targets.ExcludeManifest,
	}

	for _, target := range raw {
		kind, path, found := strings.Cut(target, "=")
		if !found {
			return fmt.Errorf("target %q is not kind=path", target)
		}
		field, known := fields[kind]
		if !known {
			return fmt.Errorf("unknown target kind %q", kind)
		}
		if path == "" {
			return fmt.Errorf("target %q names no path", kind)
		}
		if *field != "" {
			return fmt.Errorf("target kind %q was given twice", kind)
		}
		*field = path
	}

	return r.consistent()
}

// consistent refuses target combinations that would write outside the scope, or that ask
// for an exclusion without the repository root it is written against.
func (r *Request) consistent() error {
	if r.Targets.Manifest == "" {
		return errors.New("manifest= is required: without it nothing can be taken back and " +
			"removal never works")
	}

	if r.Scope == ScopeMachine {
		for kind, value := range map[string]string{
			"root":             r.Targets.Root,
			"exclude":          r.Targets.Exclude,
			"exclude-manifest": r.Targets.ExcludeManifest,
		} {
			if value != "" {
				return fmt.Errorf("%s= is project scope only; machine scope has no repository", kind)
			}
		}
		return nil
	}

	if r.Targets.Exclude != "" && r.Targets.ExcludeManifest != "" {
		return errors.New("exclude= and exclude-manifest= are alternatives: the first records " +
			"every generated path, the second the manifest alone")
	}
	if (r.Targets.Exclude != "" || r.Targets.ExcludeManifest != "") && r.Targets.Root == "" {
		return errors.New("excluding needs root=: git reads an exclude pattern against the " +
			"repository root")
	}
	return nil
}

// Roots returns the workflow roots of this scope, each with the label the report prints.
func (r *Request) Roots() []Root {
	if r.Scope == ScopeMachine {
		return []Root{
			{Label: "machine/shipped", Dir: r.Workflows},
			{Label: "machine/custom", Dir: r.Custom},
		}
	}
	return []Root{{Label: "project", Dir: r.LocalWorkflows}}
}

// Accepted is one workflow that validated and whose phases all resolved; a command is
// generated from it.
type Accepted struct {
	Name       string
	Dir        string
	Definition *Definition
	Phases     []Resolved
}

// Line is one workflow row in the report, registered or not. A refused workflow keeps its
// row with the reason, so the report names every workflow the user created.
type Line struct {
	Name   string
	Source string
	Phases int
	Routes []string

	// Command is the registered command name without the agent's prefix; empty if refused.
	Command string

	// Overrides marks a repository workflow that shares a machine workflow's name; Command
	// is then the distinct name it got, and Machine the command the machine's copy keeps.
	Overrides bool
	Machine   string

	Reason string
}

// Prepare scans this scope's roots, validates and resolves every workflow, and returns the
// accepted ones together with a report line for every workflow found.
func (r *Request) Prepare(installed map[string]string) ([]Accepted, []Line, error) {
	return collect(r.Roots(), r.LocalSkills, r.Skills, installed)
}

// MachineNames returns the names of the machine workflows that register, so a project run
// can detect its own names clashing with them. It returns nothing at machine scope.
func (r *Request) MachineNames() ([]string, error) {
	if r.Scope == ScopeMachine {
		return nil, nil
	}

	roots := []Root{
		{Label: "machine/shipped", Dir: r.MachineWorkflows},
		{Label: "machine/custom", Dir: r.MachineCustom},
	}
	accepted, _, err := collect(roots, "", r.Skills, nil)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(accepted))
	for _, one := range accepted {
		names = append(names, one.Name)
	}
	return names, nil
}

// collect scans the roots, resolves every valid workflow's phases against the skill roots,
// and refuses workflows whose subagent names collide. local is empty at machine scope.
func collect(roots []Root, local, shared string, installed map[string]string) ([]Accepted, []Line, error) {
	found, err := Scan(roots)
	if err != nil {
		return nil, nil, err
	}

	var accepted []Accepted
	lines := make([]Line, 0, len(found))
	for _, one := range found {
		if one.Reason != "" {
			lines = append(lines, Line{Name: one.Name, Source: one.Source, Reason: one.Reason})
			continue
		}

		// Lookup order: the workflow's own skills, the repository's, then the shared library.
		phases, err := Resolve(one.Definition, SkillRoots{
			Own:        filepath.Join(one.Dir, "skills"),
			Repository: local,
			Shared:     shared,
		}, installed)
		if err != nil {
			lines = append(lines, Line{Name: one.Name, Source: one.Source, Reason: err.Error()})
			continue
		}

		accepted = append(accepted, Accepted{
			Name: one.Name, Dir: one.Dir, Definition: one.Definition, Phases: phases,
		})
		lines = append(lines, Line{
			Name:    one.Name,
			Source:  one.Source,
			Phases:  len(one.Definition.Phases),
			Routes:  sorted(one.Definition.Routes),
			Command: "alfred-" + one.Name,
		})
	}

	accepted, lines = refuse(accepted, lines, collisions(accepted))
	return accepted, lines, nil
}

// SubagentName returns the name a workflow's phase is dispatched under:
// `alfred-<phase>` for a shared phase, `alfred-<workflow>-<phase>` for one the workflow
// or repository brought.
func SubagentName(workflow string, phase Resolved) string {
	if phase.Origin == FromShared {
		return "alfred-" + phase.Name
	}
	return "alfred-" + workflow + "-" + phase.Name
}

// collisions returns, keyed by workflow name, the refusal reason for every workflow whose
// generated subagent name is also generated by another workflow of the scope.
// Subagent names are not unique per workflow (`a-b` + `c` vs `a` + `b-c`), so both
// colliders are refused rather than silently overwriting each other.
func collisions(accepted []Accepted) map[string]string {
	owners, refused := map[string]string{}, map[string]string{}

	for _, one := range accepted {
		for _, phase := range one.Phases {
			// Shared phases are one subagent by design; only workflow-owned names can collide.
			if phase.Origin == FromShared {
				continue
			}
			name := SubagentName(one.Name, phase)
			other, taken := owners[name]
			if !taken {
				owners[name] = one.Name
				continue
			}
			if other == one.Name {
				continue
			}
			reason := fmt.Sprintf(
				"the subagent name %q is generated by both %q and %q; neither workflow is registered",
				name, other, one.Name)
			refused[other], refused[one.Name] = reason, reason
		}
	}
	return refused
}

// refuse drops the workflows named in refused from the accepted list and turns their
// report lines into rejections; every other workflow is left untouched.
func refuse(accepted []Accepted, lines []Line, refused map[string]string) ([]Accepted, []Line) {
	if len(refused) == 0 {
		return accepted, lines
	}

	kept := make([]Accepted, 0, len(accepted))
	for _, one := range accepted {
		if _, isRefused := refused[one.Name]; !isRefused {
			kept = append(kept, one)
		}
	}
	for i, line := range lines {
		if reason, isRefused := refused[line.Name]; isRefused {
			lines[i] = Line{Name: line.Name, Source: line.Source, Reason: reason}
		}
	}
	return kept, lines
}

// RecordManifest adds the manifest to the local git exclude file when only the manifest is
// excluded, returning the recorded paths; it writes only when apply is true.
// written is false when no manifest was produced: exclude lines are never removed, so none
// is added for a manifest that will not exist.
func (r *Request) RecordManifest(written, apply bool) ([]string, error) {
	if r.Targets.ExcludeManifest == "" || !written {
		return nil, nil
	}

	paths := []string{relativeTo(r.Targets.Root, r.Targets.Manifest)}
	if !apply {
		return paths, nil
	}
	if _, err := generated.Exclude(r.Targets.ExcludeManifest, paths); err != nil {
		return nil, err
	}
	return paths, nil
}

// ExcludeFile returns the local exclude file path relative to the repository root, or ""
// when nothing is excluded.
func (r *Request) ExcludeFile() string {
	file := r.Targets.Exclude
	if file == "" {
		file = r.Targets.ExcludeManifest
	}
	if file == "" {
		return ""
	}
	return relativeTo(r.Targets.Root, file)
}

// relativeTo makes path relative to root, as git exclude patterns require. A path outside
// root (or an unusable root) is returned unchanged, since a `..` pattern matches nothing.
func relativeTo(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return path
	}
	return relative
}

// Counts is how many commands and agents a run left unchanged.
type Counts struct {
	Commands int
	Agents   int
}

// AgentLine is what one agent received in one scope. Modified, Orphans and Collisions are
// files Alfred does not own; they are reported, never touched.
type AgentLine struct {
	Agent      string
	Target     string
	Added      []string
	Updated    []string
	Removed    []string
	Unchanged  Counts
	Modified   []string
	Orphans    []string
	Collisions []string
}

// Unassigned is one phase that has no model assigned, with the reason reported for it.
type Unassigned struct {
	Workflow string
	Phase    string
	Reason   string
}

// Adopted lists what an earlier Alfred version wrote and this run records in the manifest
// for the first time: Claude Code file paths and OpenCode agent keys.
type Adopted struct {
	Files []string
	Keys  []string
}

// empty reports whether nothing was adopted.
func (a Adopted) empty() bool { return len(a.Files)+len(a.Keys) == 0 }

// Report is everything one registration run prints, and what decides its exit code.
type Report struct {
	Scope       Scope
	Mode        Mode
	Workflows   []Line
	Agents      []AgentLine
	Adopted     Adopted
	Unassigned  []Unassigned
	Excluded    []string
	ExcludeFile string
}

// Override marks a workflow as a repository copy of a machine workflow's name, recording
// the distinct command it got and the command the machine's copy keeps.
func (r *Report) Override(workflow, command, machine string) {
	for i := range r.Workflows {
		if r.Workflows[i].Name == workflow {
			r.Workflows[i].Command, r.Workflows[i].Overrides = command, true
			r.Workflows[i].Machine = machine
		}
	}
}

// missingCommand is one workflow whose command an agent does not have yet.
type missingCommand struct {
	workflow string
	agent    string
	command  string
}

// missing returns the commands an agent would have to be given (what the write reported
// as added). Refused workflows are skipped: their refusal is already reported.
func (r *Report) missing() []missingCommand {
	var commands []missingCommand
	for _, line := range r.Workflows {
		if line.Reason != "" || line.Command == "" {
			continue
		}
		for _, agent := range r.Agents {
			named := asNamed(agent.Agent, line.Command)
			if contains(agent.Added, named) {
				commands = append(commands, missingCommand{
					workflow: line.Name, agent: label(agent.Agent), command: named,
				})
			}
		}
	}
	return commands
}

// Failed reports whether the run exits non-zero: no agent, a collision, a refused workflow,
// an unassigned model, or (in check mode) a missing command. Modified files, orphans and
// adoptions do not fail the run; a collision does, because it leaves a command missing.
func (r *Report) Failed() bool {
	if len(r.Agents) == 0 {
		return true
	}
	for _, agent := range r.Agents {
		if len(agent.Collisions) > 0 {
			return true
		}
	}
	for _, line := range r.Workflows {
		if line.Reason != "" {
			return true
		}
	}
	if len(r.Unassigned) > 0 {
		return true
	}
	return r.Mode == ModeCheck && len(r.missing()) > 0
}

// String renders the full report: workflows, adoptions, per-agent results (or the missing
// commands in check mode), exclusions and phases without a model.
func (r *Report) String() string {
	var sections []string

	sections = append(sections, r.workflowSection())
	if section := r.adoptedSection(); section != "" {
		sections = append(sections, section)
	}
	switch {
	case len(r.Agents) == 0:
		sections = append(sections, "no supported agent was detected; nothing was written")
	case r.Mode == ModeCheck:
		sections = append(sections, r.missingSection())
	default:
		for _, agent := range r.Agents {
			sections = append(sections, r.agentSection(agent))
		}
		if section := r.excludedSection(); section != "" {
			sections = append(sections, section)
		}
	}
	if section := r.unassignedSection(); section != "" {
		sections = append(sections, section)
	}

	return strings.Join(sections, "\n\n") + "\n"
}

// workflowSection renders the aligned table of workflows, with each refusal reason or each
// workflow's phase count, routes and any machine override.
func (r *Report) workflowSection() string {
	lines := []string{"workflows"}
	if len(r.Workflows) == 0 {
		return strings.Join(append(lines, "  this scope defines no workflow; nothing to register"), "\n")
	}

	// Escape values before measuring, so column widths match what is printed.
	names, sources := make([]string, 0, len(r.Workflows)), make([]string, 0, len(r.Workflows))
	for _, line := range r.Workflows {
		names = append(names, inline(line.Name))
		sources = append(sources, inline(line.Source))
	}
	nameWidth, sourceWidth := width(names), width(sources)
	indent := 2 + nameWidth + 2 + sourceWidth + 2

	for i, line := range r.Workflows {
		head := "  " + pad(names[i], nameWidth) + "  " + pad(sources[i], sourceWidth) + "  "
		if line.Reason != "" {
			lines = append(lines, reason(head, indent, line.Reason)...)
			continue
		}

		routes := make([]string, 0, len(line.Routes))
		for _, route := range line.Routes {
			routes = append(routes, inline(route))
		}
		lines = append(lines, head+fmt.Sprintf("%d phases  routes: %s",
			line.Phases, strings.Join(routes, ", ")))

		if line.Overrides {
			lines = append(lines, fmt.Sprintf("%soverrides the machine's %s in this repository; use /%s here, /%s for the machine's",
				strings.Repeat(" ", 2+nameWidth+2), names[i], inline(line.Command), inline(line.Machine)))
		}
	}
	return strings.Join(lines, "\n")
}

// inline prints a definition value as-is when safe, or Go-quoted otherwise, so a value with
// a newline or a double space cannot forge a report row or column (injection guard).
func inline(value string) string {
	if printableColumn(value) {
		return value
	}
	return strconv.Quote(value)
}

// printableColumn reports whether value can be printed in a column unquoted: all runes
// printable, no leading/trailing space, and no double space (the column separator).
func printableColumn(value string) bool {
	if strings.Contains(value, "  ") ||
		strings.HasPrefix(value, " ") || strings.HasSuffix(value, " ") {
		return false
	}
	for _, character := range value {
		if !unicode.IsPrint(character) {
			return false
		}
	}
	return true
}

// reason renders a rejection after the row's head, wrapped so continuation lines align
// under the "rejected: " marker.
func reason(head string, indent int, text string) []string {
	const marker = "rejected: "
	wrapped := wrap(text, reportWidth-indent-len(marker), reportWidth-indent)

	lines := make([]string, 0, len(wrapped))
	lines = append(lines, head+marker+wrapped[0])
	for _, rest := range wrapped[1:] {
		lines = append(lines, strings.Repeat(" ", indent)+rest)
	}
	return lines
}

// agentSection renders what one agent received: added, updated, removed, unchanged counts,
// and the modified, orphan and collision lists left alone.
func (r *Report) agentSection(agent AgentLine) string {
	lines := []string{fmt.Sprintf("%s  (%s: %s)", label(agent.Agent), r.Scope, agent.Target)}

	lines = append(lines, field("added", strings.Join(agent.Added, ", "))...)
	lines = append(lines, field("updated", strings.Join(agent.Updated, ", "))...)
	lines = append(lines, field("removed", strings.Join(agent.Removed, ", "))...)
	if count := agent.Unchanged.Commands + agent.Unchanged.Agents; count > 0 {
		lines = append(lines, field("unchanged", fmt.Sprintf("%s, %s",
			plural(agent.Unchanged.Commands, "command"), plural(agent.Unchanged.Agents, "agent")))...)
	}
	if len(agent.Added)+len(agent.Updated)+len(agent.Removed) == 0 {
		lines = append(lines, "  nothing added, updated or removed")
	}

	lines = append(lines, kept("modified", agent.Modified,
		"claimed by this scope's manifest and edited since; left alone")...)
	lines = append(lines, kept("orphans", agent.Orphans,
		"the generated naming, claimed by nothing; this run will not remove them")...)
	lines = append(lines, collided(agent)...)

	return strings.Join(lines, "\n")
}

// collided renders each collision path with its own note naming the command or subagent
// that is now missing on that agent.
func collided(agent AgentLine) []string {
	// Capacity hint: a path line plus a note that usually wraps onto two lines.
	lines := make([]string, 0, 3*len(agent.Collisions))
	for i, path := range agent.Collisions {
		key := ""
		if i == 0 {
			key = "collisions"
		}
		lines = append(lines, field(key, path)...)
		lines = append(lines, field("", fmt.Sprintf(
			"not written: %s Alfred never generated already carries this name, so %s is "+
				"missing on %s; rename or remove it",
			carrier(agent.Agent), missingName(agent.Agent, path), label(agent.Agent)))...)
	}
	return lines
}

// carrier names what holds a colliding name: a file on Claude Code, an agent (a key in a
// generated file) on OpenCode.
func carrier(agent string) string {
	if agent == "claude" {
		return "a file"
	}
	return "an agent"
}

// missingName converts a collision entry (a path on Claude Code, a key on OpenCode) into
// the name the agent uses: `/name` for a Claude Code command, the bare name otherwise.
func missingName(agent, entry string) string {
	if agent != "claude" {
		return entry
	}
	name := strings.TrimSuffix(filepath.Base(entry), ".md")
	if filepath.Base(filepath.Dir(entry)) == "commands" {
		return "/" + name
	}
	return name
}

// kept renders a list of paths the run left alone, followed by the note explaining why.
func kept(key string, paths []string, note string) []string {
	if len(paths) == 0 {
		return nil
	}
	lines := field(key, strings.Join(paths, ", "))
	return append(lines, field("", note)...)
}

// adoptedSection renders the files and agent keys adopted from an earlier Alfred version,
// or "" when nothing was adopted.
func (r *Report) adoptedSection() string {
	if r.Adopted.empty() {
		return ""
	}

	key, sentence := "adopted", "recorded in the manifest for the first time. From here on "+
		"they are refreshed and removed like anything else Alfred writes. This happens once."
	if r.Mode != ModeApply {
		key, sentence = "would adopt", "which applying would record in the manifest for the "+
			"first time. Nothing was written. From there they are refreshed and removed like "+
			"anything else Alfred writes. This happens once."
	}

	lines := marginal(key, adoptedCount(r.Adopted)+" written by an earlier version, "+sentence)
	for _, name := range append(append([]string{}, r.Adopted.Files...), r.Adopted.Keys...) {
		lines = append(lines, marginal("", name)...)
	}
	return strings.Join(lines, "\n")
}

// adoptedCount describes how many files and agents were adopted, omitting a zero side.
func adoptedCount(adopted Adopted) string {
	var parts []string
	if len(adopted.Files) > 0 {
		parts = append(parts, plural(len(adopted.Files), "file"))
	}
	if len(adopted.Keys) > 0 {
		parts = append(parts, plural(len(adopted.Keys), "agent"))
	}
	return strings.Join(parts, " and ")
}

// marginal renders a `key  value` line at the left margin, wrapped under the value's
// column. An empty key continues the line above.
func marginal(key, value string) []string {
	const keyWidth = 11

	wrapped := wrap(value, reportWidth-keyWidth-2, reportWidth-keyWidth-2)
	lines := make([]string, 0, len(wrapped))
	for i, text := range wrapped {
		head := strings.Repeat(" ", keyWidth+2)
		if i == 0 && key != "" {
			head = pad(key, keyWidth) + "  "
		}
		lines = append(lines, head+text)
	}
	return lines
}

// excludedSection renders how many paths were recorded in the exclude file, or "".
func (r *Report) excludedSection() string {
	if len(r.Excluded) == 0 {
		return ""
	}
	return pad("excluded", 11) + "  " + fmt.Sprintf("%s recorded in %s",
		plural(len(r.Excluded), "path"), r.ExcludeFile)
}

// missingSection renders the check-mode table of workflows whose command an agent lacks.
func (r *Report) missingSection() string {
	found := r.missing()
	lines := []string{"missing"}
	if len(found) == 0 {
		return strings.Join(append(lines,
			"  every workflow of this scope has its command on every detected agent"), "\n")
	}

	workflows, agents := make([]string, 0, len(found)), make([]string, 0, len(found))
	for _, one := range found {
		workflows = append(workflows, inline(one.workflow))
		agents = append(agents, one.agent)
	}
	for i, one := range found {
		lines = append(lines, "  "+pad(workflows[i], width(workflows))+"  "+
			pad(one.agent, width(agents))+"  "+inline(one.command))
	}
	return strings.Join(append(lines, fmt.Sprintf(
		"  run `alfred workflows %s apply` with the same roots and targets", r.Scope)), "\n")
}

// unassignedSection renders the phases without a model, each with its wrapped reason.
func (r *Report) unassignedSection() string {
	if len(r.Unassigned) == 0 {
		return ""
	}

	phases := make([]string, 0, len(r.Unassigned))
	for _, one := range r.Unassigned {
		phases = append(phases, inline(one.Workflow+"/"+one.Phase))
	}

	lines := []string{"without a model"}
	indent := 2 + width(phases) + 2
	for i, one := range r.Unassigned {
		wrapped := wrap(one.Reason, reportWidth-indent, reportWidth-indent)
		lines = append(lines, "  "+pad(phases[i], width(phases))+"  "+wrapped[0])
		for _, rest := range wrapped[1:] {
			lines = append(lines, strings.Repeat(" ", indent)+rest)
		}
	}
	return strings.Join(lines, "\n")
}

// field renders an indented `key  value` line inside an agent section, wrapped under the
// value's column. An empty key continues the line above; an empty value renders nothing.
func field(key, value string) []string {
	const keyWidth = 11
	if value == "" {
		return nil
	}

	wrapped := wrap(value, reportWidth-2-keyWidth, reportWidth-2-keyWidth)
	lines := make([]string, 0, len(wrapped))
	for i, text := range wrapped {
		head := strings.Repeat(" ", keyWidth)
		if i == 0 && key != "" {
			head = pad(key, keyWidth)
		}
		lines = append(lines, "  "+head+text)
	}
	return lines
}

// wrap breaks text on whitespace, with one width for the first line and another for the
// rest (minimum 20). Longer words are kept whole so paths stay usable.
func wrap(text string, first, rest int) []string {
	if first < 20 {
		first = 20
	}
	if rest < 20 {
		rest = 20
	}

	var lines []string
	line, limit := "", first
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= limit:
			line += " " + word
		default:
			lines = append(lines, line)
			line, limit = word, rest
		}
	}
	return append(lines, line)
}

// label returns the agent's user-facing name ("claude" becomes "claude code").
func label(agent string) string {
	if agent == "claude" {
		return "claude code"
	}
	return agent
}

// asNamed returns a command as the agent names it: `/command` on Claude Code, else bare.
func asNamed(agent, command string) string {
	if agent == "claude" {
		return "/" + command
	}
	return command
}

// contains reports whether list holds want.
func contains(list []string, want string) bool {
	for _, one := range list {
		if one == want {
			return true
		}
	}
	return false
}

// plural formats count with noun, adding "s" unless count is 1.
func plural(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

// width returns the length of the longest value.
func width(values []string) int {
	longest := 0
	for _, value := range values {
		if len(value) > longest {
			longest = len(value)
		}
	}
	return longest
}

// pad right-pads value with spaces to targetWidth.
func pad(value string, targetWidth int) string {
	if len(value) >= targetWidth {
		return value
	}
	return value + strings.Repeat(" ", targetWidth-len(value))
}
