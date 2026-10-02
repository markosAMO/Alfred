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
)

const usage = `usage: alfred <command> [arguments]

  state write <home> <version> <payload>     record a hash per installed file
  state compare <home> <source> <payload>    classify source files against the install
  state apply <home> <source>                copy new and updatable files, report from stdin
  agents <profile> <skills-root> <kind=path>...   write the agent definitions
  memory check <home> <kind=path>...         exit 0 when the backend exposes every tool
  memory apply <home> <kind=path>...         register the backend, repairing a narrowed one
  memory declared <agents-dir>               exit 0 when every agent declares every memory tool
  json-get <file|-> <key>                    print a top-level value, one line per item
  json-valid <file>                          exit 0 when the file parses as JSON
  session-permission <home>                  exit 0 when starting a session is permitted

<payload> is a comma-separated list of paths relative to the root.`

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
	case "agents":
		return runAgents(args[1:])
	case "memory":
		return runMemory(args[1:])
	case "json-get":
		return runJSONGet(args[1:])
	case "json-valid":
		return runJSONValid(args[1:])
	case "session-permission":
		return runSessionPermission(args[1:])
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

func runAgents(args []string) error {
	if len(args) < 2 {
		return errors.New("agents: expected a profile, a skills root and one or more kind=path targets")
	}

	profile, err := agents.LoadProfile(args[0])
	if err != nil {
		return err
	}
	skillsRoot := args[1]

	var written []string
	for _, target := range args[2:] {
		kind, path, found := strings.Cut(target, "=")
		if !found {
			return fmt.Errorf("agents: target %q is not kind=path", target)
		}

		switch kind {
		case "opencode":
			config, err := agents.Opencode(profile, skillsRoot)
			if err != nil {
				return err
			}
			if err := agents.MergeOpencode(path, config); err != nil {
				return err
			}
			written = append(written, fmt.Sprintf(
				"opencode: %d subagents + alfred and alfred-worktree agents",
				len(config.Agent)-2))

		case "claude":
			count, err := agents.ClaudeAgents(profile, skillsRoot, path)
			if err != nil {
				return err
			}
			written = append(written, fmt.Sprintf(
				"claude code: %d subagents + /alfred and /alfred-worktree commands", count-2))

		default:
			return fmt.Errorf("agents: unknown target kind %q", kind)
		}
	}

	fmt.Println(strings.Join(written, "\n"))
	return nil
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
