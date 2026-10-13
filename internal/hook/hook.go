// Package hook is what a phase's agent runs before a tool call: it reads the call the harness
// describes on stdin and refuses a write the phase may not make. Its rules arrive as
// arguments rendered at registration, so it never reads a skill or a contract itself.
package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/markosAMO/alfred/internal/contract"
)

// Call is the part of a harness's pre-tool event the guard reads: where the session runs,
// which tool is called and the file it would write.
type Call struct {
	Cwd       string `json:"cwd"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
}

// Protected is what no phase may write whatever its contract says: Alfred's own files in
// the repository, as patterns relative to its root, and Alfred's directories on the machine.
type Protected struct {
	Patterns    []string
	Directories []string
}

// Fixed returns the protected set for an installation at alfredHome on a machine whose home
// directory is userHome. A phase that rewrote any of these could change the rules it runs
// under, or those of the phases after it.
func Fixed(alfredHome, userHome string) Protected {
	return Protected{
		Patterns: []string{
			".alfred/skills/**",
			".alfred/workflows/**",
			".alfred/config.yaml",
			".alfred/generated.json",
			".alfred/skill-registry.md",
			".claude/agents/**",
			".claude/commands/**",
			"opencode.json",
		},
		Directories: []string{
			alfredHome,
			filepath.Join(userHome, ".claude", "agents"),
			filepath.Join(userHome, ".claude", "commands"),
			filepath.Join(userHome, ".config", "opencode"),
		},
	}
}

// decision is the pre-tool answer that refuses a call; the reason is shown to the agent.
type decision struct {
	HookSpecificOutput struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	} `json:"hookSpecificOutput"`
}

// PreTool reads one call from input and, when the write it would make is refused, writes
// the refusal to output. An allowed call writes nothing, so the harness's own permission
// checks still apply. An error means the call could not be judged, which the caller must
// treat as a refusal.
func PreTool(input io.Reader, output io.Writer, rules contract.Filesystem, protected Protected) error {
	var call Call
	if err := json.NewDecoder(input).Decode(&call); err != nil {
		return fmt.Errorf("reading the tool call: %w", err)
	}

	target := call.ToolInput.FilePath
	if target == "" {
		target = call.ToolInput.NotebookPath
	}
	if target == "" {
		return nil
	}
	if call.Cwd == "" {
		return fmt.Errorf("the %s call carries no cwd to resolve %s against", call.ToolName, target)
	}

	allowed, reason := Judge(call.Cwd, target, rules, protected)
	if allowed {
		return nil
	}

	var refusal decision
	refusal.HookSpecificOutput.HookEventName = "PreToolUse"
	refusal.HookSpecificOutput.PermissionDecision = "deny"
	refusal.HookSpecificOutput.PermissionDecisionReason = fmt.Sprintf(
		"Alfred refused writing %s: %s. Leave this file as it is and report it in your result.",
		target, reason)
	return json.NewEncoder(output).Encode(refusal)
}

// Judge decides whether a write to target, from a session running in cwd, is allowed, and
// when it is not, why.
func Judge(cwd, target string, rules contract.Filesystem, protected Protected) (bool, string) {
	if !filepath.IsAbs(target) {
		target = filepath.Join(cwd, target)
	}
	target = filepath.Clean(target)

	for _, directory := range protected.Directories {
		if within(directory, target) {
			return false, "it is part of Alfred's installation"
		}
	}

	root := repositoryRoot(cwd)
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		if len(rules.Write) > 0 {
			return false, "it is outside the repository, and the phase's contract allows writing only " +
				strings.Join(rules.Write, ", ")
		}
		return true, ""
	}
	relative = filepath.ToSlash(relative)

	for _, pattern := range protected.Patterns {
		if contract.Match(pattern, relative) {
			return false, "it is one of Alfred's own files in this repository"
		}
	}
	return rules.Allows(relative)
}

// within reports whether path is directory itself or lies under it.
func within(directory, path string) bool {
	if directory == "" {
		return false
	}
	directory = filepath.Clean(directory)
	return path == directory || strings.HasPrefix(path, directory+string(filepath.Separator))
}

// repositoryRoot returns the nearest directory at or above cwd that holds .alfred/, which
// is the root every pattern is relative to, or cwd itself when there is none.
func repositoryRoot(cwd string) string {
	for directory := filepath.Clean(cwd); ; {
		if info, err := os.Stat(filepath.Join(directory, ".alfred")); err == nil && info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return filepath.Clean(cwd)
		}
		directory = parent
	}
}

// Stop is the part of a harness's stop event the check reads.
type Stop struct {
	Cwd                  string `json:"cwd"`
	StopHookActive       bool   `json:"stop_hook_active"`
	LastAssistantMessage string `json:"last_assistant_message"`
}

// Expected is what a phase must have done when it stops: the files it leaves and what its
// result envelope holds.
type Expected struct {
	Outputs []string
	Result  *contract.Result
}

// stopRefusal is the stop answer that keeps the phase running; the reason is its next
// instruction.
type stopRefusal struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// StopCheck reads one stop event from input and, when the phase has not done what its
// contract expects, writes a refusal that keeps it running to fix it. It refuses once: a
// phase already continuing because of a refusal is let through, so a phase that cannot
// comply ends with its result as it is rather than looping.
func StopCheck(input io.Reader, output io.Writer, expected Expected) error {
	var event Stop
	if err := json.NewDecoder(input).Decode(&event); err != nil {
		return fmt.Errorf("reading the stop event: %w", err)
	}
	if event.StopHookActive {
		return nil
	}

	problems := Missing(event.Cwd, event.LastAssistantMessage, expected)
	if len(problems) == 0 {
		return nil
	}
	return json.NewEncoder(output).Encode(stopRefusal{
		Decision: "block",
		Reason: "Alfred checked what this phase leaves and found: " + strings.Join(problems, "; ") +
			". Fix these, then end with your result envelope again.",
	})
}

// Missing returns, in a fixed order, everything the phase was expected to do and did not.
// The envelope is checked whatever the status; outputs unless the phase reports, with a
// valid status, that it stopped short.
func Missing(cwd, message string, expected Expected) []string {
	envelope := Envelope(message)
	var problems []string

	if expected.Result != nil {
		for _, key := range expected.Result.Required {
			if _, found := envelope[key]; !found {
				problems = append(problems, "the result envelope has no "+key)
			}
		}
		if status, found := envelope["status"]; found && len(expected.Result.Status) > 0 &&
			!slices.Contains(expected.Result.Status, status) {
			problems = append(problems, fmt.Sprintf("status %q is not one of %s",
				status, strings.Join(expected.Result.Status, ", ")))
		}
	}

	if len(expected.Outputs) == 0 || stoppedShort(envelope["status"], expected.Result) {
		return problems
	}
	change, found := envelope["change"]
	if !found || !oneSegment(change) {
		return append(problems, "the result envelope names no change directory, so the "+
			"outputs cannot be checked; add change: <the change's directory name>")
	}
	root := repositoryRoot(cwd)
	for _, output := range expected.Outputs {
		relative := strings.ReplaceAll(output, contract.ChangeSegment, change)
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil || info.IsDir() {
			problems = append(problems, "the output "+relative+" does not exist")
		}
	}
	return problems
}

// stoppedShort reports whether a status says the phase did not finish, which is the one
// case that owes no outputs. It must be a status the phase may return: an invalid one is
// refused alongside any missing output, because the phase is refused only once and the
// outputs would otherwise go unchecked after it corrects the status. With no list declared,
// the protocol's own failed and blocked are the ones that count.
func stoppedShort(status string, result *contract.Result) bool {
	if status == "" || status == "completed" {
		return false
	}
	if result != nil && len(result.Status) > 0 {
		return slices.Contains(result.Status, status)
	}
	return status == "failed" || status == "blocked"
}

// Envelope reads the top-level keys of the result envelope from a phase's final message: the
// last fenced block when there is one, the whole message otherwise. Only unindented
// `key: value` lines are read; the envelope is flat, so nothing below a key is needed.
func Envelope(message string) map[string]string {
	lines := strings.Split(lastFencedBlock(message), "\n")
	envelope := map[string]string{}
	for _, line := range lines {
		key, value, found := strings.Cut(strings.TrimRight(line, " \t\r"), ":")
		if !found || !contract.EnvelopeKey(key) {
			continue
		}
		envelope[key] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return envelope
}

// lastFencedBlock returns the body of the last ``` block in text, or text when it has none.
func lastFencedBlock(text string) string {
	lines := strings.Split(text, "\n")
	end := -1
	for index := len(lines) - 1; index >= 0; index-- {
		if !strings.HasPrefix(strings.TrimSpace(lines[index]), "```") {
			continue
		}
		if end == -1 {
			end = index
			continue
		}
		return strings.Join(lines[index+1:end], "\n")
	}
	return text
}

// oneSegment reports whether a change name is a single directory name, so it cannot point
// the check outside the changes directory.
func oneSegment(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}
