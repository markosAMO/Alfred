// Package generated records what a registration run put into an agent's directories, so a
// later run can take back what it no longer produces without touching anything it never
// wrote.
//
// It is deliberately not part of internal/state. state.json records the payload Alfred
// copied into the installation and `state apply` copies those files from a source tree;
// these files have no source to copy from — they are rendered from a model profile into
// directories other programs own, and a repository holds its own set of them. One file
// holding both would make `update` try to copy the generated ones.
//
// Removal is driven by this manifest rather than by a name prefix, because deleting every
// alfred-* file not produced this run deletes a file the user wrote and happened to call
// alfred-notes. A run removes what the manifest claims, is still on disk and still matches
// the hash recorded when it was written; everything else is reported and left alone.
package generated

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Agent is what one agent received: files on disk, and keys inside a file it owns, each
// with the hash of what was written.
//
// The two kinds are separate because they are taken back differently: a file is removed
// from disk, a key is dropped from a JSON document somebody else also writes into.
type Agent struct {
	Files map[string]string `json:"files,omitempty"`
	Keys  map[string]string `json:"keys,omitempty"`
}

// Manifest is the generated.json of one scope. The machine's and a repository's have the
// same shape and neither is ever read by a run of the other scope.
type Manifest struct {
	Agents map[string]Agent `json:"agents"`
}

// Read loads a scope's manifest. A scope that has never been registered has no manifest,
// which is no claims rather than an error; a manifest that cannot be decoded is an error,
// because treating it as absent would silently abandon every file it claims.
func Read(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return &m, nil
}

// Write records the manifest. Paths are absolute: at machine scope the generated files sit
// outside the directory the manifest lives in, so there is no root to make them relative
// to.
func (m *Manifest) Write(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (m *Manifest) agent(name string) Agent {
	if m == nil {
		return Agent{}
	}
	return m.Agents[name]
}

// Set is what one run generated, accumulated as it writes. It is also how a manifest is
// built out of what is already on disk: a caller that can prove some file was written by an
// earlier version records it here and takes the Manifest, which is a reconstruction rather
// than a claim about this run.
type Set struct {
	agents map[string]Agent
}

func NewSet() *Set { return &Set{agents: map[string]Agent{}} }

// AddFile records a file this run wrote, with the hash of the bytes it wrote.
func (s *Set) AddFile(agent, path string, content []byte) {
	section := s.agent(agent)
	if section.Files == nil {
		section.Files = map[string]string{}
	}
	section.Files[path] = hash(content)
	s.agents[agent] = section
}

// AddKey records an agent key this run set, with the hash of the value it set.
func (s *Set) AddKey(agent, key string, value []byte) {
	section := s.agent(agent)
	if section.Keys == nil {
		section.Keys = map[string]string{}
	}
	section.Keys[key] = hash(value)
	s.agents[agent] = section
}

// ForgetFiles and ForgetKeys drop names this run turned out not to write.
//
// A collision is only knowable once the run has said what it generates: PlanFiles decides
// it by comparing that list against what is on disk. So the caller records everything, asks
// for the plan, and then takes back the names the plan refused - and the manifest claims
// only what was actually written, which is what stops the next run believing it owns a file
// the user wrote.
func (s *Set) ForgetFiles(agent string, paths []string) {
	if len(paths) == 0 {
		return
	}
	section := s.agent(agent)
	for _, path := range paths {
		delete(section.Files, path)
	}
	s.agents[agent] = section
}

func (s *Set) ForgetKeys(agent string, keys []string) {
	if len(keys) == 0 {
		return
	}
	section := s.agent(agent)
	for _, key := range keys {
		delete(section.Keys, key)
	}
	s.agents[agent] = section
}

func (s *Set) agent(name string) Agent {
	if s == nil {
		return Agent{}
	}
	return s.agents[name]
}

// Manifest is what this run claims, to be written once removal has run.
func (s *Set) Manifest() *Manifest {
	out := &Manifest{Agents: make(map[string]Agent, len(s.agents))}
	for name, section := range s.agents {
		out.Agents[name] = Agent{
			Files: maps.Clone(section.Files),
			Keys:  maps.Clone(section.Keys),
		}
	}
	return out
}

// Sweep is what a run must do about what an earlier run left behind, for one agent and one
// kind of generated thing.
//
// Remove is safe to take back: the manifest claims it, it is still there and it is
// unchanged. Modified is claimed but was edited since, so it is reported and kept.
// Orphans and Collisions carry the generated naming and are claimed by nothing: the first
// is reported as something this run will not remove, the second as a name the user already
// uses that registration also generates.
type Sweep struct {
	Remove     []string
	Modified   []string
	Orphans    []string
	Collisions []string
}

func (s *Sweep) sort() {
	for _, list := range []*[]string{&s.Remove, &s.Modified, &s.Orphans, &s.Collisions} {
		slices.Sort(*list)
	}
}

// PlanFiles decides what to do with the files an earlier run generated for one agent.
//
// present is what is on disk now, as the caller found it before writing anything: a file
// this run generates that was already there and is claimed by nobody is the user's, and
// saying so afterwards would be saying it about Alfred's own output.
func PlanFiles(previous *Manifest, s *Set, agent string, present []string) (Sweep, error) {
	claimed := previous.agent(agent).Files
	generated := s.agent(agent).Files

	var sweep Sweep
	for path, recorded := range claimed {
		if _, kept := generated[path]; kept {
			continue
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			// Already gone: nothing to remove and nothing to report.
			continue
		}
		if err != nil {
			return Sweep{}, fmt.Errorf("reading %s: %w", path, err)
		}
		if hash(data) == recorded {
			sweep.Remove = append(sweep.Remove, path)
		} else {
			sweep.Modified = append(sweep.Modified, path)
		}
	}

	for _, path := range present {
		if _, claimed := claimed[path]; claimed || !named(filepath.Base(path)) {
			continue
		}
		if _, collides := generated[path]; collides {
			sweep.Collisions = append(sweep.Collisions, path)
		} else {
			sweep.Orphans = append(sweep.Orphans, path)
		}
	}

	sweep.sort()
	return sweep, nil
}

// PlanKeys is PlanFiles for the keys of an agent that keeps its definitions inside one
// file. present is the keys that file holds now, with the bytes of each value, which the
// caller already has parsed and so never reads twice.
func PlanKeys(previous *Manifest, s *Set, agent string, present map[string][]byte) Sweep {
	claimed := previous.agent(agent).Keys
	generated := s.agent(agent).Keys

	var sweep Sweep
	for key, recorded := range claimed {
		if _, kept := generated[key]; kept {
			continue
		}
		value, there := present[key]
		if !there {
			continue
		}
		if hash(value) == recorded {
			sweep.Remove = append(sweep.Remove, key)
		} else {
			sweep.Modified = append(sweep.Modified, key)
		}
	}

	for key := range present {
		if _, claimed := claimed[key]; claimed || !named(key) {
			continue
		}
		if _, collides := generated[key]; collides {
			sweep.Collisions = append(sweep.Collisions, key)
		} else {
			sweep.Orphans = append(sweep.Orphans, key)
		}
	}

	sweep.sort()
	return sweep
}

// Scan lists the files in dir that carry the naming registration generates, so a caller
// can hand them to PlanFiles as what is on disk. A directory that is not there holds no
// files, which is the state of every agent directory before its first run.
func Scan(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}

	var found []string
	for _, entry := range entries {
		if entry.IsDir() || !named(entry.Name()) {
			continue
		}
		found = append(found, filepath.Join(dir, entry.Name()))
	}
	return found, nil
}

// Remove takes back the paths a Sweep found safe to remove. A path that disappeared
// between the plan and the removal is already in the state the removal wanted.
func Remove(paths []string) error {
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("removing %s: %w", path, err)
		}
	}
	return nil
}

// Named is named for a caller that holds agent keys rather than file names: Scan already
// applies this to a directory, and the file OpenCode owns is one document whose keys the
// caller has parsed, so there is nothing here to scan for it.
func Named(name string) bool { return named(name) }

// named reports whether a file name or an agent key carries the naming registration
// generates. It is what decides whether something unclaimed is worth reporting at all: a
// file called notes.md is not Alfred's business, and alfredo.md is not Alfred's either.
func named(name string) bool {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	return stem == "alfred" || strings.HasPrefix(stem, "alfred-")
}

func hash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
