// Command alfred is the installer's helper: install bookkeeping, agent generation, and the
// few JSON reads the shell script needs.
//
// Bash has neither JSON nor sha256, so everything the installer needs of either is done
// here, and an installation needs no interpreter beyond the shell.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/markosAMO/alfred/internal/agents"
	"github.com/markosAMO/alfred/internal/memory"
	"github.com/markosAMO/alfred/internal/settings"
	"github.com/markosAMO/alfred/internal/state"
	"github.com/markosAMO/alfred/internal/workflow"
)

const usage = `usage: alfred <command> [arguments]

  state write <home> <version> <payload>     record a hash per installed file
  state compare <home> <source> <payload>    classify source files against the install
  state apply <home> <source>                copy new and updatable files, report from stdin
  workflows machine <mode> <roots> <kind=path>...   register the machine's workflows
  workflows project <mode> <roots> <kind=path>...   register one repository's own
  memory check <home> <kind=path>...         exit 0 when the backend exposes every tool
  memory apply <home> <kind=path>...         register the backend, repairing a narrowed one
  memory declared <agents-dir>               exit 0 when every agent declares every memory tool
  json-get <file|-> <key>                    print a top-level value, one line per item
  json-valid <file>                          exit 0 when the file parses as JSON
  session-permission <home>                  exit 0 when starting a session is permitted
  version [<home>]                           print the version the installation recorded

<payload> is a comma-separated list of paths relative to the root.

workflows takes the roots of one scope, positionally and in this order:

  machine  <profile> <skills> <workflows> <custom> <templates>
  project  <profile> <skills> <local-skills> <local-workflows> <machine-workflows> <machine-custom>

  mode   apply    write the commands and the agents, and report what changed
         report   the identical report, writing nothing
         check    report every workflow of this scope with no registered command

  kind   claude=<dir>              the agent's directory; commands/ and agents/ sit under it
         opencode=<file>           the agent's configuration file
         manifest=<file>           this scope's generated.json; required in every mode
         root=<dir>                the repository an excluded path is written relative to
         exclude=<file>            the local exclude file; every generated path is recorded
         exclude-manifest=<file>   the same file, for the manifest's path alone

root=, exclude= and exclude-manifest= are project scope only. exclude= and
exclude-manifest= are alternatives, and either one needs root=: the first is
artifacts.committed false, where nothing registration wrote is committed, and the second is
artifacts.committed true, where only the manifest is kept out of git because it holds this
machine's absolute paths.

The machine roots given at project scope are read and never written. They are what tells a
repository workflow that this machine already has a command of its name.

workflows exits 0 when every workflow registered, every phase has a model and, under check,
every command is present; 1 when a workflow was rejected, a phase has no model, a checked
command is missing, or no supported agent was detected; 2 when it was called wrongly.

version takes the installation directory; with none it reads ALFRED_HOME, falling back to
~/.config/alfred. It prints what state.json records, on one line and with nothing else, and
exits 1 when there is no installation there.`

// errUsage asks for the usage text on stderr and a non-zero exit, without the "error:"
// prefix a real failure carries.
var errUsage = errors.New("usage")

func main() {
	switch err := run(os.Args[1:]); {
	case err == nil:
	case errors.Is(err, errUsage):
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	default:
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errUsage
	}

	switch args[0] {
	case "state":
		return runState(args[1:])
	case "workflows":
		return runWorkflows(args[1:])
	case "memory":
		return runMemory(args[1:])
	case "json-get":
		return runJSONGet(args[1:])
	case "json-valid":
		return runJSONValid(args[1:])
	case "session-permission":
		return runSessionPermission(args[1:])
	case "version":
		return runVersion(args[1:])
	case "-h", "--help", "help":
		fmt.Println(usage)
		return nil
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

// splitPayload splits the comma-separated payload; an empty one selects the whole root.
func splitPayload(raw string) []string {
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func runState(args []string) error {
	if len(args) == 0 {
		return errors.New("state: expected write, compare or apply")
	}

	switch args[0] {
	case "write":
		if len(args) != 4 {
			return errors.New("state write: expected <home> <version> <payload>")
		}
		count, err := state.Write(args[1], args[2], splitPayload(args[3]))
		if err != nil {
			return err
		}
		fmt.Printf("%d files recorded\n", count)
		return nil

	case "compare":
		if len(args) != 4 {
			return errors.New("state compare: expected <home> <source> <payload>")
		}
		report, err := state.Compare(args[1], args[2], splitPayload(args[3]))
		if err != nil {
			return err
		}
		fmt.Println(report.JSON())
		return nil

	case "apply":
		if len(args) != 3 {
			return errors.New("state apply: expected <home> <source>")
		}
		return applyReport(args[1], args[2])

	default:
		return fmt.Errorf("state: unknown operation %s", args[0])
	}
}

// applyReport copies every file the report calls new or updatable. A modified file is
// never touched: that is the whole point of recording the hashes.
func applyReport(home, source string) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	var report state.Report
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("reading report: %w", err)
	}

	count := 0
	for _, section := range [][]string{report.New, report.Updatable} {
		for _, rel := range section {
			src, err := under(source, rel)
			if err != nil {
				return err
			}
			dst, err := under(home, rel)
			if err != nil {
				return err
			}
			if err := copyFile(src, dst); err != nil {
				return err
			}
			count++
		}
	}

	fmt.Printf("%d files updated, %d left alone\n", count, len(report.Modified))
	return nil
}

// under joins a relative path to a root and refuses anything that climbs out of it. The
// report is a document rather than an argument, so a path inside it is not trusted to stay
// where it belongs.
func under(root, rel string) (string, error) {
	joined := filepath.Join(root, rel)

	clean, err := filepath.Rel(root, joined)
	if err != nil || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("report: %q points outside %s", rel, root)
	}
	return joined, nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return err
	}
	// The mode is what makes bin/*.sh runnable.
	return os.Chmod(dst, info.Mode().Perm())
}

// runWorkflows registers the workflows of one scope: scan the roots, validate and resolve
// every definition, generate the commands and the subagents, write them into the targets of
// that scope, and report.
//
// This is the only place that sees both internal/workflow and internal/agents. The second
// imports the first, so the first cannot name the second's result, and translating one
// into the other is therefore here. Every decision either way stays in the package it
// belongs to: this function picks nothing and only passes values along.
func runWorkflows(args []string) error {
	request, err := workflow.Parse(args)
	if err != nil {
		// Called wrongly is exit 2, and the reason is worth more than the usage text alone.
		fmt.Fprintln(os.Stderr, "workflows: "+err.Error())
		return errUsage
	}

	profile, err := agents.LoadProfile(request.Profile)
	if err != nil {
		return err
	}

	accepted, lines, err := request.Prepare(profile.Phases)
	if err != nil {
		return err
	}

	set, err := agents.Generate(profile, generation(request, accepted))
	if err != nil {
		return err
	}

	report := &workflow.Report{
		Scope:       request.Scope,
		Mode:        request.Mode,
		Workflows:   lines,
		ExcludeFile: request.ExcludeFile(),
	}

	machine, err := request.MachineNames()
	if err != nil {
		return err
	}
	for _, override := range agents.Localise(set, machine) {
		report.Override(override.Workflow,
			strings.TrimPrefix(override.Command, "/"), strings.TrimPrefix(override.Machine, "/"))
	}

	written, err := registration(set, request)
	if err != nil {
		return err
	}
	for _, change := range written.Agents {
		report.Agents = append(report.Agents, agentLine(change))
	}
	report.Adopted = workflow.Adopted{Files: written.Adopted.Files, Keys: written.Adopted.Keys}
	report.Excluded = written.Excluded

	recorded, err := request.RecordManifest(written.Manifest, request.Mode == workflow.ModeApply)
	if err != nil {
		return err
	}
	report.Excluded = append(report.Excluded, recorded...)

	for _, phase := range set.Unavailable {
		report.Unassigned = append(report.Unassigned, workflow.Unassigned{
			Workflow: phase.Workflow, Phase: phase.Phase, Reason: phase.Reason,
		})
	}

	fmt.Print(report.String())
	if report.Failed() {
		os.Exit(1)
	}
	return nil
}

func generation(request *workflow.Request, accepted []workflow.Accepted) agents.Generation {
	g := agents.Generation{
		Scope:     agents.Machine,
		Skills:    request.Skills,
		Templates: request.Templates,
		Custom:    request.Custom,
	}
	if request.Scope == workflow.ScopeProject {
		g.Scope = agents.Project
	}
	for _, one := range accepted {
		g.Workflows = append(g.Workflows, agents.Registered{
			Dir: one.Dir, Definition: one.Definition, Phases: one.Phases,
		})
	}
	return g
}

// registration is the one place the mode decides anything: apply writes, and both of the
// others go through the same code with the writes gated off, so a report can never describe
// a run that would not have happened.
func registration(set *agents.Set, request *workflow.Request) (*agents.Written, error) {
	targets := agents.Targets{
		Claude:   request.Targets.Claude,
		Opencode: request.Targets.Opencode,
		Manifest: request.Targets.Manifest,
		Exclude:  request.Targets.Exclude,
		Root:     request.Targets.Root,
	}
	if request.Mode == workflow.ModeApply {
		return agents.Write(set, targets)
	}
	return agents.Report(set, targets)
}

func agentLine(change agents.Change) workflow.AgentLine {
	line := workflow.AgentLine{
		Agent:      change.Agent,
		Target:     change.Target,
		Added:      entryNames(change.Added),
		Updated:    entryNames(change.Updated),
		Removed:    entryNames(change.Removed),
		Modified:   change.Modified,
		Orphans:    change.Orphans,
		Collisions: change.Collisions,
	}
	for _, entry := range change.Unchanged {
		if entry.Command {
			line.Unchanged.Commands++
			continue
		}
		line.Unchanged.Agents++
	}
	return line
}

func entryNames(entries []agents.Entry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names
}

// runMemory exits with the number of problems found, so the installer's doctor can use it
// as a check directly.
func runMemory(args []string) error {
	if len(args) == 0 {
		return errors.New("memory: expected check, apply or declared")
	}

	switch args[0] {
	case "check", "apply":
		if len(args) < 2 {
			return fmt.Errorf("memory %s: expected <home> and kind=path targets", args[0])
		}
		problems, err := memory.Register(os.Stdout, args[1], args[2:], args[0] == "apply")
		if err != nil {
			return err
		}
		if problems > 0 {
			os.Exit(problems)
		}
		return nil

	case "declared":
		if len(args) != 2 {
			return errors.New("memory declared: expected <agents-dir>")
		}
		if !memory.Declared(args[1]) {
			os.Exit(1)
		}
		return nil

	default:
		return fmt.Errorf("memory: unknown operation %s", args[0])
	}
}

func runJSONGet(args []string) error {
	if len(args) != 2 {
		return errors.New("json-get: expected <file|-> <key>")
	}

	var (
		data []byte
		err  error
	)
	if args[0] == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(args[0])
	}
	if err != nil {
		return err
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}

	raw, ok := doc[args[1]]
	if !ok {
		return fmt.Errorf("json-get: no key %q", args[1])
	}

	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		fmt.Println(str)
		return nil
	}
	var items []string
	if err := json.Unmarshal(raw, &items); err == nil {
		for _, item := range items {
			fmt.Println(item)
		}
		return nil
	}
	fmt.Println(string(raw))
	return nil
}

func runJSONValid(args []string) error {
	if len(args) != 1 {
		return errors.New("json-valid: expected <file>")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	if !json.Valid(data) {
		return fmt.Errorf("%s: not valid JSON", args[0])
	}
	return nil
}

func runSessionPermission(args []string) error {
	if len(args) != 1 {
		return errors.New("session-permission: expected <home>")
	}
	if !settings.SessionPermissionGranted(args[0]) {
		os.Exit(1)
	}
	return nil
}

// runVersion is the only command here the installer does not call, so it is the only one
// that has to work out its own home. Every other command is handed one by the script that
// knows it; this one answers a user on a machine with no clone, with nothing to hand.
func runVersion(args []string) error {
	if len(args) > 1 {
		return errors.New("version: expected an optional <home>")
	}

	var home string
	if len(args) == 1 {
		home = args[0]
	} else {
		resolved, err := alfredHome()
		if err != nil {
			return err
		}
		home = resolved
	}

	version, err := state.Version(home)
	if err != nil {
		return err
	}
	fmt.Println(version)
	return nil
}

// alfredHome resolves the default installation directory the way install.sh does, which is
// the only reason this binary reads an environment variable at all.
func alfredHome() (string, error) {
	if home := os.Getenv("ALFRED_HOME"); home != "" {
		return home, nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving the default home: %w", err)
	}
	return filepath.Join(dir, ".config", "alfred"), nil
}
