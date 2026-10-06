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

// reportWidth is what a registration report wraps to. It is the width the rest of this
// repository's prose is written to, and a report is read in a terminal beside everything
// else the installer prints.
const reportWidth = 90

// Scope is the side of the installation a run registers. It is given on the command line
// and never derived: a run is handed the roots and the targets of one scope, and nothing
// below asks which it got.
type Scope string

const (
	ScopeMachine Scope = "machine"
	ScopeProject Scope = "project"
)

// Mode is what a run does with what it found.
//
// Report produces the identical report to Apply and writes nothing, which is one code path
// with a write gate rather than two that can drift. Check answers a different question —
// which workflows of this scope have no command — and so prints a different section.
type Mode string

const (
	ModeApply  Mode = "apply"
	ModeReport Mode = "report"
	ModeCheck  Mode = "check"
)

// Targets is where one scope's registration writes, as the command line spells it.
//
// Exclude and ExcludeManifest are alternatives rather than degrees of one setting. Under
// `artifacts.committed: false` every generated path is kept out of git, which is Exclude.
// Under `true` the generated commands and agents are ordinary files and only the manifest
// is kept out, because it records this machine's absolute paths and the hashes of files
// rendered from this machine's model profile, so it is wrong in every other clone.
type Targets struct {
	Claude          string
	Opencode        string
	Manifest        string
	Root            string
	Exclude         string
	ExcludeManifest string
}

// Request is one invocation of `alfred workflows`, with every root and every target it was
// given.
//
// The roots are positional and explicit. Deriving them from the skills root would make the
// helper construct a path into the payload and then read it, and the write targets are
// named because a caller that swaps two of eight positionals does so silently.
type Request struct {
	Scope   Scope
	Mode    Mode
	Profile string
	Skills  string

	// Machine scope. Templates is where /alfred-add-workflow copies a new workflow from and
	// Custom is where it writes it, and both are rendered into that command as text.
	Workflows string
	Custom    string
	Templates string

	// Project scope. The machine roots are read, never written: they are what tells a
	// repository workflow that this machine already has a command of its name.
	LocalSkills      string
	LocalWorkflows   string
	MachineWorkflows string
	MachineCustom    string

	Targets Targets
}

// machineRootCount and projectRootCount are the positionals each scope takes after the mode.
const (
	machineRootCount = 5
	projectRootCount = 6
)

// Parse reads one `alfred workflows` invocation. Every error it returns is the caller
// having been called wrongly, which is exit 2 and never a failure of the run.
func Parse(args []string) (*Request, error) {
	if len(args) == 0 {
		return nil, errors.New("expected machine or project")
	}

	r := &Request{Scope: Scope(args[0])}
	if r.Scope != ScopeMachine && r.Scope != ScopeProject {
		return nil, fmt.Errorf("unknown scope %q: expected machine or project", args[0])
	}

	rest := args[1:]
	if len(rest) == 0 {
		return nil, errors.New("expected apply, report or check")
	}
	r.Mode = Mode(rest[0])
	if r.Mode != ModeApply && r.Mode != ModeReport && r.Mode != ModeCheck {
		return nil, fmt.Errorf("unknown mode %q: expected apply, report or check", rest[0])
	}

	wanted := machineRootCount
	if r.Scope == ScopeProject {
		wanted = projectRootCount
	}
	roots := rest[1:]
	if len(roots) < wanted {
		return nil, fmt.Errorf("%s scope takes %d roots before the targets, and %d were given",
			r.Scope, wanted, len(roots))
	}

	r.Profile, r.Skills = roots[0], roots[1]
	if r.Scope == ScopeMachine {
		r.Workflows, r.Custom, r.Templates = roots[2], roots[3], roots[4]
	} else {
		r.LocalSkills, r.LocalWorkflows = roots[2], roots[3]
		r.MachineWorkflows, r.MachineCustom = roots[4], roots[5]
	}

	if err := r.targets(roots[wanted:]); err != nil {
		return nil, err
	}
	return r, nil
}

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

// consistent refuses the combinations that would write somewhere the scope does not own, or
// that ask for an exclusion with nothing to write it against.
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

// Roots are the workflow roots of this scope, with the label the report prints beside
// everything found in each.
func (r *Request) Roots() []Root {
	if r.Scope == ScopeMachine {
		return []Root{
			{Label: "machine/shipped", Dir: r.Workflows},
			{Label: "machine/custom", Dir: r.Custom},
		}
	}
	return []Root{{Label: "project", Dir: r.LocalWorkflows}}
}

// Accepted is one workflow that validated and whose every phase resolved. It is what a
// command is generated from.
type Accepted struct {
	Name       string
	Dir        string
	Definition *Definition
	Phases     []Resolved
}

// Line is one workflow in the report, registered or not.
//
// A workflow that was refused keeps its place here with the reason, rather than vanishing
// from the output: the report names every workflow the user created, and one that is not
// listed reads as one that was never there.
type Line struct {
	Name   string
	Source string
	Phases int
	Routes []string

	// Command is the command this workflow registered under, without the leading marker the
	// agent puts on it. It is empty for a workflow that was refused.
	Command string

	// Overrides is a repository workflow carrying a machine workflow's name. The command
	// above is then the distinguishing one, and saying so is the only thing that tells a
	// user why the name they expected reaches the machine's copy.
	//
	// Machine is the command the machine's copy keeps. Both are named in the line, because
	// a user holding one of two commands that differ by a suffix has to be told which
	// reaches which copy rather than left to infer the other from the one they were given.
	Overrides bool
	Machine   string

	Reason string
}

// Prepare scans the roots of this scope, validates every definition and resolves every
// phase, and returns what registered alongside the report line for everything it met.
func (r *Request) Prepare(installed map[string]string) ([]Accepted, []Line, error) {
	return collect(r.Roots(), r.LocalSkills, r.Skills, installed)
}

// MachineNames are the machine workflows that have a command, which is what a project run
// needs to tell whether one of its own names collides with one.
//
// A machine workflow that was refused has no command, so nothing can collide with it. At
// machine scope there is nothing above to collide with and the answer is empty.
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

		// The workflow's own skills first, then the repository's when it has one, then the
		// shared library. local is empty at machine scope, which is what selects the
		// rejection that names repository scope.
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

// SubagentName is the name the phase of one workflow is dispatched under. A shared phase is
// one subagent however many workflows run it; a phase a workflow brought itself is addressed
// under a name that is that workflow's alone.
//
// It lives beside registration rather than beside the generator because the ambiguity it can
// produce is decided here, before anything is generated, and one rule written in two places
// is one rule that drifts.
func SubagentName(workflow string, phase Resolved) string {
	if phase.Origin == FromShared {
		return "alfred-" + phase.Name
	}
	return "alfred-" + workflow + "-" + phase.Name
}

// collisions names the workflows of one scope that generate a subagent name another workflow
// of that scope also generates, with the reason each of them is refused by.
//
// `alfred-<workflow>-<phase>` is not injective: `ventas-extra` with a phase `propuesta` and
// `ventas` with a phase `extra-propuesta` reach one name however distinct the two workflow
// names are. Whatever keys its subagents by that name takes the second assignment, so one
// workflow's command would dispatch a subagent carrying the other's prompt and model with
// nothing said anywhere, and the longest-prefix match that localises a name cannot resolve
// the ambiguity either.
//
// Both colliders are refused rather than disambiguated. A generated name the user cannot
// predict from the definition they wrote is worse than a rejection that names the two
// workflows to rename.
//
// It is decided here, among every other per-workflow refusal, because a collision is a
// property of a pair of definitions, like a name present in two roots. Deciding it where the
// generating happens would make it a refusal of the whole scope, and an invalid workflow
// does not block the valid ones.
func collisions(accepted []Accepted) map[string]string {
	by, refused := map[string]string{}, map[string]string{}

	for _, one := range accepted {
		for _, phase := range one.Phases {
			// A shared phase is deliberately one subagent however many workflows run it, so
			// only the names a workflow makes its own can collide. A phase nobody assigned a
			// model to is checked all the same: the name comes from the two definitions and
			// not from the profile, and deciding it by the models would let a registration
			// pass today and fail the day the model is filled in.
			if phase.Origin == FromShared {
				continue
			}
			name := SubagentName(one.Name, phase)
			other, taken := by[name]
			if !taken {
				by[name] = one.Name
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

// refuse turns the workflows named in refused into ordinary rejection lines and drops them
// from what registers. Every other workflow of the scope is left exactly as it was, which is
// the whole difference between rejecting a pair of definitions and rejecting a run.
func refuse(accepted []Accepted, lines []Line, refused map[string]string) ([]Accepted, []Line) {
	if len(refused) == 0 {
		return accepted, lines
	}

	kept := make([]Accepted, 0, len(accepted))
	for _, one := range accepted {
		if _, out := refused[one.Name]; !out {
			kept = append(kept, one)
		}
	}
	for i, line := range lines {
		if reason, out := refused[line.Name]; out {
			lines[i] = Line{Name: line.Name, Source: line.Source, Reason: reason}
		}
	}
	return kept, lines
}

// RecordManifest keeps this scope's manifest out of git under `artifacts.committed: true`,
// where nothing else registration writes is excluded.
//
// Under `false` the manifest is one of the paths the write itself records, so doing it here
// as well would report one path in two places. It returns what it recorded so the report
// can count it, and writes only when the run is applying.
//
// written is whether registration claimed a manifest at all. A repository defining no
// workflow claims none and is left exactly as it was found, which includes its exclude
// file: the line is append-only and nothing ever takes it back, so writing one for a
// generated.json that will never exist is permanent and wrong in the same breath.
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

// ExcludeFile is the local exclude file as the report names it: relative to the repository
// root, which is where the user goes to look at it.
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

// relativeTo makes a path relative to the repository root, which is what git reads an
// exclude pattern against. A path that is not under the root is left as it is rather than
// turned into a chain of `..`, which git would match nothing with. A root that is empty or
// not absolute is one of those: there is nothing to subtract, so nothing is.
func relativeTo(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}

// Counts is how much of one agent's set a run left exactly as it found it. The bulk of a
// steady machine is unchanged, so it is counted rather than listed.
type Counts struct {
	Commands int
	Agents   int
}

// AgentLine is what one agent received in one scope.
//
// Added, Updated, Removed and Unchanged are Alfred's own output. Modified, Orphans and
// Collisions are the three ways something on disk is not Alfred's to take back, and they
// are reported rather than acted on.
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

// Unassigned is one phase nobody gave a model to. It is reported rather than fatal: the
// routes that never reach it work, and the ones that do stop there and name it.
type Unassigned struct {
	Workflow string
	Phase    string
	Reason   string
}

// Adopted is what an earlier version of Alfred wrote and this run recorded as its own for
// the first time: Claude Code paths, and the keys of the agents OpenCode holds.
//
// It is provenance rather than a fourth action column. The same names appear again under
// `updated` or `removed`, as the ordinary algorithm classified them, and that is not double
// reporting: one line says whose they are, the other says what was done to them.
type Adopted struct {
	Files []string
	Keys  []string
}

func (a Adopted) empty() bool { return len(a.Files)+len(a.Keys) == 0 }

// Report is everything one registration run has to say, and the only thing that decides
// whether it exits zero.
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

// Override marks one workflow as a repository's copy of a machine workflow's name, under
// the distinguishing command it was registered as and beside the one the machine keeps.
func (r *Report) Override(workflow, command, machine string) {
	for i := range r.Workflows {
		if r.Workflows[i].Name == workflow {
			r.Workflows[i].Command, r.Workflows[i].Overrides = command, true
			r.Workflows[i].Machine = machine
		}
	}
}

// missingCommand is one workflow whose command an agent does not have.
type missingCommand struct {
	workflow string
	agent    string
	command  string
}

// missing is what check answers. A command an agent would have to be given is one the
// agent does not have, which is exactly what the write reported as an addition.
//
// A workflow that was refused is skipped: it has no command and will not have one until it
// is fixed, and reporting it here as well would be one fault reported as two.
func (r *Report) missing() []missingCommand {
	var out []missingCommand
	for _, line := range r.Workflows {
		if line.Reason != "" || line.Command == "" {
			continue
		}
		for _, agent := range r.Agents {
			named := asNamed(agent.Agent, line.Command)
			if contains(agent.Added, named) {
				out = append(out, missingCommand{
					workflow: line.Name, agent: label(agent.Agent), command: named,
				})
			}
		}
	}
	return out
}

// Failed reports whether this run exits non-zero. Every reason is in the specification's
// exit table and nothing else is: a modified file and an orphan are things to look at rather
// than things that stopped the run, and an adoption is a migration that worked.
//
// A collision is the exception among the three a run reports and does not act on. The
// generated content for that name was not written, so a workflow of this scope has no
// command on that agent — which is exactly what check exits non-zero for, and reporting the
// same hole as a success today and a failure tomorrow is one answer too many.
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

func (r *Report) String() string {
	var out []string

	out = append(out, r.workflowSection())
	if section := r.adoptedSection(); section != "" {
		out = append(out, section)
	}
	switch {
	case len(r.Agents) == 0:
		out = append(out, "no supported agent was detected; nothing was written")
	case r.Mode == ModeCheck:
		out = append(out, r.missingSection())
	default:
		for _, agent := range r.Agents {
			out = append(out, r.agentSection(agent))
		}
		if section := r.excludedSection(); section != "" {
			out = append(out, section)
		}
	}
	if section := r.unassignedSection(); section != "" {
		out = append(out, section)
	}

	return strings.Join(out, "\n\n") + "\n"
}

func (r *Report) workflowSection() string {
	lines := []string{"workflows"}
	if len(r.Workflows) == 0 {
		return strings.Join(append(lines, "  this scope defines no workflow; nothing to register"), "\n")
	}

	// Rendered before the columns are measured, because the width of a value is the width
	// of what is printed and not of what the definition holds.
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

// inline renders a value that came from a definition so it cannot open a line of the report
// or a column inside one.
//
// The report is a block of aligned lines a user reads to decide whether their machine is in
// the state they think it is, and its whole structure is two characters: a newline between
// rows and two spaces between columns. A value carrying either rewrites that block — a
// workflow directory named with a newline forges a row of the author's choosing, and git
// stores such a name, so a clone creates it.
//
// Rejection reasons are not the exposure and are not rendered here. They quote every value
// they name with %q, and they are laid out through wrap, which rebuilds the text out of
// strings.Fields and so collapses any run of whitespace a value smuggled in. The columns
// are the one place a value is printed as it stands.
//
// A value carrying neither character is printed exactly as it was written, which is every
// value anybody actually has: quoting them all would put quotation marks around every name
// in the report, which is the same defect read from the other side — a block that does not
// say what registered.
func inline(value string) string {
	if printableColumn(value) {
		return value
	}
	return strconv.Quote(value)
}

// printableColumn answers whether a value can stand in a column as it is: every rune
// printable, so no newline, tab or other control character, and no run of two spaces, which
// is what separates one column from the next.
func printableColumn(value string) bool {
	if strings.Contains(value, "  ") ||
		strings.HasPrefix(value, " ") || strings.HasSuffix(value, " ") {
		return false
	}
	for _, r := range value {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// reason lays a rejection out under the column it starts in. The first line is shorter by
// the width of the word that introduces it, and every line after it begins where that word
// did.
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

// collided lays out the one list a run reports and does not act on whose consequence has a
// name: the generated content was not written, so the command or subagent that content was
// going to be is missing on that agent until the user moves their file out of the way.
//
// Each path carries its own sentence rather than the list sharing one, because what is
// missing differs per entry and naming it is the whole point: `collisions` alone says a name
// clashed, and a user reading it still has to work out what they lost.
func collided(agent AgentLine) []string {
	// A collision is a path on a line of its own and a sentence that wraps onto two.
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

// carrier is what holds a colliding name on one agent. Claude Code's is a file, which is
// what the design's report says; OpenCode's is a key inside a file Alfred does generate, so
// calling it a file there would be the one word in the sentence that is untrue.
func carrier(agent string) string {
	if agent == "claude" {
		return "a file"
	}
	return "an agent"
}

// missingName is what the collision cost, named the way the agent names it: a slash command
// on Claude Code, and the bare name for a subagent and for everything OpenCode holds.
//
// The entry is a path on Claude Code and a key on OpenCode, because the user has to go and
// look at the thing, and that is also what tells the two apart here.
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

// kept is one of the lists a run reports and does not act on, with the sentence that says
// why it did not.
func kept(key string, paths []string, note string) []string {
	if len(paths) == 0 {
		return nil
	}
	lines := field(key, strings.Join(paths, ", "))
	return append(lines, field("", note)...)
}

// adoptedSection says that what a run is about to refresh and remove is Alfred's own, on a
// scope where nothing recorded it. It sits at the left margin rather than under an agent:
// whose those files are is one fact about the scope, and the two agents hold halves of it.
//
// It is stated once, on the one run that can state it at all — the run that writes the
// manifest an earlier version never did. On the next run of that scope the manifest exists,
// nothing is adopted and the section is absent.
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

// adoptedCount names only the side that has something, because a machine with one agent has
// nothing to say about the other and `0 agents` reads as a fault rather than as an absence.
func adoptedCount(a Adopted) string {
	var parts []string
	if len(a.Files) > 0 {
		parts = append(parts, plural(len(a.Files), "file"))
	}
	if len(a.Keys) > 0 {
		parts = append(parts, plural(len(a.Keys), "agent"))
	}
	return strings.Join(parts, " and ")
}

// marginal is one `key  value` line of a section that stands at the left margin rather than
// under an agent, wrapped under the value's column. An empty key is a continuation of the
// line above.
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

// excludedSection sits at the left margin rather than under an agent: what registration
// kept out of git is one fact about the repository, not something one agent received.
func (r *Report) excludedSection() string {
	if len(r.Excluded) == 0 {
		return ""
	}
	return pad("excluded", 11) + "  " + fmt.Sprintf("%s recorded in %s",
		plural(len(r.Excluded), "path"), r.ExcludeFile)
}

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

// field is one `key  value` line of a section, wrapped under the value's column. An empty
// key is a continuation of the line above, which is how a note is attached to a list.
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

// wrap breaks text on spaces, giving the first line one width and every line after it
// another. A word longer than the width is left whole: breaking a path in half makes it
// unusable, and an over-long line is only ugly.
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

// label names an agent the way the user does. The manifest and the targets key it as
// `claude`, which is the directory's name and not the product's.
func label(agent string) string {
	if agent == "claude" {
		return "claude code"
	}
	return agent
}

// asNamed is a command written the way one agent names it: a slash command on Claude Code,
// a bare primary agent on OpenCode.
func asNamed(agent, command string) string {
	if agent == "claude" {
		return "/" + command
	}
	return command
}

func contains(list []string, want string) bool {
	for _, one := range list {
		if one == want {
			return true
		}
	}
	return false
}

func plural(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

func width(values []string) int {
	longest := 0
	for _, value := range values {
		if len(value) > longest {
			longest = len(value)
		}
	}
	return longest
}

func pad(value string, to int) string {
	if len(value) >= to {
		return value
	}
	return value + strings.Repeat(" ", to-len(value))
}
