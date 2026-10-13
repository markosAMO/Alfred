// Command alfred is the installer's helper: install bookkeeping, workflow and agent
// generation, memory registration, and the JSON reads the shell script needs. It exists so
// an installation needs nothing beyond the shell (which has no JSON or sha256).
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/markosAMO/alfred/internal/agents"
	"github.com/markosAMO/alfred/internal/contract"
	"github.com/markosAMO/alfred/internal/generated"
	"github.com/markosAMO/alfred/internal/hook"
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
  hook pre-tool [--write <p>]... [--deny <p>]...   judge one write a phase's agent is about to make
  hook stop [--output <p>]... [--result [--require <key>]... [--status <v>]...]
                                             check what a phase leaves when it stops

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
exits 1 when there is no installation there.

hook reads the harness's event on stdin; its arguments are the phase's contract, rendered
at registration. pre-tool refuses Alfred's own files whatever the contract says. stop
refuses once, naming what is missing, and lets a phase already continuing from a refusal
through. A refusal prints the decision and exits 0. An event it cannot judge, or wrong
arguments, exits 2 for pre-tool, which refuses the write, and 1 for stop, which lets the
phase end rather than loop on a hook it cannot repair.`

// errUsage makes main print the usage text on stderr and exit 2, without the "error:"
// prefix a real failure carries.
var errUsage = errors.New("usage")

// main runs the command. Exit codes: 0 success, 1 failure, 2 called wrongly.
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

// run dispatches to the subcommand named by the first argument.
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
	case "hook":
		runHook(args[1:])
		return nil
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

// runState handles `state write|compare|apply`.
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

// applyReport reads a state report from stdin, copies every New and Updatable file from
// source to home and deletes every Removed one, with any directory that leaves empty.
// Modified files are never touched, so user edits survive.
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
			sourcePath, err := under(source, rel)
			if err != nil {
				return err
			}
			destPath, err := under(home, rel)
			if err != nil {
				return err
			}
			if err := copyFile(sourcePath, destPath); err != nil {
				return err
			}
			count++
		}
	}

	for _, rel := range report.Removed {
		path, err := under(home, rel)
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		pruneEmpty(home, filepath.Dir(path))
	}

	fmt.Printf("%d files updated, %d removed, %d left alone\n", count, len(report.Removed), len(report.Modified))
	return nil
}

// pruneEmpty removes dir and each parent below home that a removal left empty, so a skill or
// workflow the source dropped leaves no directory a later scan would find.
func pruneEmpty(home, dir string) {
	for dir != home && strings.HasPrefix(dir, home+string(filepath.Separator)) {
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// under joins a relative path to root and rejects any path that escapes it, because paths
// read from the report on stdin are not trusted.
func under(root, rel string) (string, error) {
	joined := filepath.Join(root, rel)

	clean, err := filepath.Rel(root, joined)
	if err != nil || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("report: %q points outside %s", rel, root)
	}
	return joined, nil
}

// copyFile copies sourcePath to destPath, creating directories and keeping the permissions.
func copyFile(sourcePath, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(destPath, data, info.Mode().Perm()); err != nil {
		return err
	}
	// Chmod as well, since WriteFile keeps an existing file's mode; bin/*.sh must be runnable.
	return os.Chmod(destPath, info.Mode().Perm())
}

// runWorkflows registers the workflows of one scope: validate the definitions, generate
// commands and subagents, write them (or only report) and print the report. It only
// translates between internal/workflow and internal/agents; it makes no decisions itself.
func runWorkflows(args []string) error {
	request, err := workflow.Parse(args)
	if err != nil {
		// Called wrongly: print the reason, then exit 2 with the usage text.
		fmt.Fprintln(os.Stderr, "workflows: "+err.Error())
		return errUsage
	}

	profile, err := agents.LoadProfile(request.Profile)
	if err != nil {
		return err
	}

	previous, err := generated.Read(request.Targets.Manifest)
	if err != nil {
		return err
	}
	var registered map[string]json.RawMessage
	if previous != nil {
		registered = previous.Registered
	}

	accepted, lines, err := request.Prepare(profile.Phases, registered)
	if err != nil {
		return err
	}

	set, err := agents.Generate(profile, generation(request, accepted))
	if err != nil {
		return err
	}
	if set.Registered, err = snapshots(accepted); err != nil {
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

// snapshots returns the validated form of every accepted workflow, by name, for the manifest
// to record; nil when there is none, so an empty scope's manifest gains no empty key.
func snapshots(accepted []workflow.Accepted) (map[string]json.RawMessage, error) {
	if len(accepted) == 0 {
		return nil, nil
	}
	recorded := make(map[string]json.RawMessage, len(accepted))
	for _, one := range accepted {
		raw, err := one.Snapshot()
		if err != nil {
			return nil, err
		}
		recorded[one.Name] = raw
	}
	return recorded, nil
}

// generation converts the parsed request and accepted workflows into agents' input.
func generation(request *workflow.Request, accepted []workflow.Accepted) agents.Generation {
	input := agents.Generation{
		Scope:     agents.Machine,
		Skills:    request.Skills,
		Templates: request.Templates,
		Custom:    request.Custom,
	}
	if request.Scope == workflow.ScopeProject {
		input.Scope = agents.Project
	}
	for _, definition := range accepted {
		input.Workflows = append(input.Workflows, agents.Registered{
			Dir: definition.Dir, Definition: definition.Definition, Phases: definition.Phases,
		})
	}
	return input
}

// registration writes the generated set in apply mode and only reports otherwise. Both go
// through the same code, so a report always describes what apply would do.
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

// agentLine converts one agent's change summary into a report line.
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

// entryNames returns the names of the entries, in order.
func entryNames(entries []agents.Entry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names
}

// runMemory handles `memory check|apply|declared`. check and apply exit with the number of
// problems found, so the installer's doctor can use the exit code directly.
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

// runJSONGet prints a top-level value of a JSON object: a string as is, a string list one
// item per line, anything else as raw JSON.
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

	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}

	raw, ok := object[args[1]]
	if !ok {
		return fmt.Errorf("json-get: no key %q", args[1])
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		fmt.Println(text)
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

// runJSONValid returns an error when the file does not parse as JSON.
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

// runSessionPermission exits 1 when no settings file permits starting a session.
func runSessionPermission(args []string) error {
	if len(args) != 1 {
		return errors.New("session-permission: expected <home>")
	}
	if !settings.SessionPermissionGranted(args[0]) {
		os.Exit(1)
	}
	return nil
}

// runVersion prints the installed version. It is meant for users, not the installer, so it
// resolves the home itself when none is given.
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

// runHook runs one of a phase's hooks: pre-tool judges a write, stop checks what the phase
// leaves. A pre-tool failure exits 2, which refuses the write: a guard that cannot judge
// must not let it through. A stop failure exits 1, which lets the phase end: exit 2 would
// send it back to work on a hook it cannot repair, until the harness gives up.
func runHook(args []string) {
	if err := hookCommand(args); err != nil {
		fmt.Fprintln(os.Stderr, "alfred hook: "+err.Error())
		if len(args) > 0 && args[0] == "stop" {
			os.Exit(1)
		}
		os.Exit(2)
	}
}

// hookCommand parses the hook's arguments and runs it on stdin.
func hookCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("expected pre-tool or stop")
	}
	flags, err := hookFlags(args[1:])
	if err != nil {
		return err
	}

	switch args[0] {
	case "pre-tool":
		if err := onlyFlags(flags, "--write", "--deny"); err != nil {
			return err
		}
		protected, err := protectedSet()
		if err != nil {
			return err
		}
		rules := contract.Filesystem{Write: flags["--write"], DenyWrite: flags["--deny"]}
		return hook.PreTool(os.Stdin, os.Stdout, rules, protected)
	case "stop":
		if err := onlyFlags(flags, "--output", "--require", "--status", "--result"); err != nil {
			return err
		}
		expected := hook.Expected{Outputs: flags["--output"]}
		if _, checked := flags["--result"]; checked {
			expected.Result = &contract.Result{Required: flags["--require"], Status: flags["--status"]}
		}
		return hook.StopCheck(os.Stdin, os.Stdout, expected)
	default:
		return fmt.Errorf("unknown hook %q: expected pre-tool or stop", args[0])
	}
}

// hookFlags reads `--name value` pairs, each name repeatable, except --result, which takes
// no value and marks that the contract declares a result block.
func hookFlags(args []string) (map[string][]string, error) {
	flags := map[string][]string{}
	for len(args) > 0 {
		if args[0] == "--result" {
			flags["--result"] = nil
			args = args[1:]
			continue
		}
		if len(args) < 2 {
			return nil, fmt.Errorf("%s takes a value", args[0])
		}
		flags[args[0]] = append(flags[args[0]], args[1])
		args = args[2:]
	}
	return flags, nil
}

// onlyFlags refuses a flag the hook does not take, so a misrendered command fails loudly.
func onlyFlags(flags map[string][]string, allowed ...string) error {
	for name := range flags {
		if !slices.Contains(allowed, name) {
			return fmt.Errorf("unknown argument %q", name)
		}
	}
	return nil
}

// protectedSet returns Alfred's fixed protected files for the installation this helper
// belongs to, which is the directory above the bin/ it was installed in.
func protectedSet() (hook.Protected, error) {
	executable, err := os.Executable()
	if err != nil {
		return hook.Protected{}, fmt.Errorf("locating the installation: %w", err)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return hook.Protected{}, fmt.Errorf("resolving the home directory: %w", err)
	}
	return hook.Fixed(filepath.Dir(filepath.Dir(executable)), userHome), nil
}

// alfredHome resolves the default installation directory the way install.sh does:
// $ALFRED_HOME, else ~/.config/alfred.
func alfredHome() (string, error) {
	if home := os.Getenv("ALFRED_HOME"); home != "" {
		return home, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving the default home: %w", err)
	}
	return filepath.Join(userHome, ".config", "alfred"), nil
}
