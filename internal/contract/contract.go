// Package contract reads a phase's contract.json, the rules a hook enforces while the phase
// runs, and decides whether one write is allowed under them. The file is read at
// registration and rendered into the agent; nothing reads it while a change is running.
package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// File is the contract's name inside a skill directory, next to SKILL.md.
const File = "contract.json"

// ChangeSegment is the one placeholder a pattern may hold. It stands for the running
// change's directory name, which is not known at registration, so it matches any single
// path segment.
const ChangeSegment = "{change}"

// Contract is what a phase declares in contract.json. Every block is optional and enforced
// on its own; an absent one leaves the phase doing what it does without a contract.
type Contract struct {
	Permissions Permissions `json:"permissions"`

	// Outputs are the files the phase must leave, relative to the repository root, each
	// naming the running change's directory as {change}.
	Outputs []string `json:"outputs,omitempty"`

	// Result is what the phase's result envelope must hold; nil checks nothing.
	Result *Result `json:"result,omitempty"`
}

// Result lists the envelope keys a phase must return, and the values its status may take.
type Result struct {
	Required []string `json:"required,omitempty"`
	Status   []string `json:"status,omitempty"`
}

// Checked reports whether the contract asks for anything to be checked when the phase
// stops.
func (c *Contract) Checked() bool {
	return c != nil && (len(c.Outputs) > 0 || c.Result != nil)
}

// Permissions is the part of a contract that a hook enforces.
type Permissions struct {
	Filesystem Filesystem `json:"filesystem"`
}

// Filesystem lists, relative to the repository root, where a phase may write. An empty
// Write allows every path; DenyWrite wins over Write.
type Filesystem struct {
	Write     []string `json:"write,omitempty"`
	DenyWrite []string `json:"deny_write,omitempty"`
}

// Load reads the contract in a skill directory. A directory with no contract.json has no
// contract, which is not an error: the phase is then bound only by the fixed rules.
func Load(dir string) (*Contract, error) {
	data, err := os.ReadFile(filepath.Join(dir, File))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return Decode(data)
}

// Decode parses and validates a contract. Unknown keys are refused, so a misspelt rule is
// an error rather than a rule that silently never applies.
func Decode(data []byte) (*Contract, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var contract Contract
	if err := decoder.Decode(&contract); err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: more than one JSON value", File)
	}

	filesystem := contract.Permissions.Filesystem
	for _, rule := range []struct {
		key      string
		patterns []string
	}{
		{"write", filesystem.Write},
		{"deny_write", filesystem.DenyWrite},
	} {
		for _, pattern := range rule.patterns {
			if err := validPattern(pattern); err != nil {
				return nil, fmt.Errorf("%s: permissions.filesystem.%s: %w", File, rule.key, err)
			}
		}
	}
	for _, output := range contract.Outputs {
		if err := validOutput(output); err != nil {
			return nil, fmt.Errorf("%s: outputs: %w", File, err)
		}
	}
	if contract.Result != nil {
		if err := contract.Result.validate(); err != nil {
			return nil, fmt.Errorf("%s: result: %w", File, err)
		}
	}
	return &contract, nil
}

// validOutput refuses an output that is not one exact file inside the repository: a
// pattern could be satisfied by any file, which proves nothing about this phase.
func validOutput(output string) error {
	if err := validPattern(output); err != nil {
		return err
	}
	if strings.ContainsAny(output, "*?[") {
		return fmt.Errorf("%q is a pattern; an output names one file", output)
	}
	return nil
}

// validate refuses an envelope key that the envelope reader could never find, and an empty
// status value.
func (r *Result) validate() error {
	for _, key := range r.Required {
		if !EnvelopeKey(key) {
			return fmt.Errorf("required key %q is not lower case letters and underscores", key)
		}
	}
	for _, status := range r.Status {
		if strings.TrimSpace(status) == "" || strings.ContainsAny(status, "\n:") {
			return fmt.Errorf("status value %q is empty or holds a newline or a colon", status)
		}
	}
	return nil
}

// EnvelopeKey reports whether a key is one the result envelope can carry: lower case
// letters and underscores, starting with a letter.
func EnvelopeKey(key string) bool {
	if key == "" || key[0] < 'a' || key[0] > 'z' {
		return false
	}
	for _, r := range key {
		if (r < 'a' || r > 'z') && r != '_' {
			return false
		}
	}
	return true
}

// validPattern refuses a pattern that could never match a path inside the repository, or
// that holds a placeholder other than {change}.
func validPattern(pattern string) error {
	switch {
	case pattern == "":
		return errors.New("a pattern is empty")
	case strings.HasPrefix(pattern, "/") || filepath.IsAbs(pattern):
		return fmt.Errorf("%q is absolute; patterns are relative to the repository root", pattern)
	case strings.Contains(pattern, `\`):
		return fmt.Errorf("%q holds a backslash; separate segments with /", pattern)
	}

	for _, segment := range strings.Split(pattern, "/") {
		switch {
		case segment == "" || segment == "." || segment == "..":
			return fmt.Errorf("%q holds an empty, . or .. segment", pattern)
		case segment == ChangeSegment || segment == "**":
			continue
		case strings.ContainsAny(segment, "{}"):
			return fmt.Errorf("%q holds a placeholder other than a whole %s segment", pattern, ChangeSegment)
		}
		if _, err := path.Match(segment, ""); err != nil {
			return fmt.Errorf("%q is not a valid pattern: %w", pattern, err)
		}
	}
	return nil
}

// Allows reports whether a phase may write the given path, relative to the repository root
// and slash-separated, and when it may not, the rule that refused it.
func (f Filesystem) Allows(relative string) (bool, string) {
	for _, pattern := range f.DenyWrite {
		if Match(pattern, relative) {
			return false, "the phase's contract denies writing " + pattern
		}
	}
	if len(f.Write) == 0 {
		return true, ""
	}
	for _, pattern := range f.Write {
		if Match(pattern, relative) {
			return true, ""
		}
	}
	return false, "the phase's contract allows writing only " + strings.Join(f.Write, ", ")
}

// Match reports whether a slash-separated relative path matches a pattern. `**` matches
// any number of segments, none included; {change} matches exactly one; every other segment
// is matched as path.Match does.
func Match(pattern, relative string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(relative, "/"))
}

// matchSegments matches a pattern against a path, one segment at a time.
func matchSegments(pattern, segments []string) bool {
	if len(pattern) == 0 {
		return len(segments) == 0
	}
	if pattern[0] == "**" {
		for skipped := 0; skipped <= len(segments); skipped++ {
			if matchSegments(pattern[1:], segments[skipped:]) {
				return true
			}
		}
		return false
	}
	if len(segments) == 0 {
		return false
	}
	if pattern[0] != ChangeSegment {
		if matched, _ := path.Match(pattern[0], segments[0]); !matched {
			return false
		}
	}
	return matchSegments(pattern[1:], segments[1:])
}
