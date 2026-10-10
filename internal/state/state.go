// Package state tracks the files Alfred installed (state.json), by content hash. A file
// whose hash still matches was not touched by the user and may be replaced; one whose hash
// differs was modified and is reported instead of overwritten.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// notManagedName (any file with that base name) and notManagedPath (one relative path) are
// written by the installation, not shipped, so they are never recorded or replaced.
var (
	notManagedName = map[string]bool{"state.json": true, "profile.json": true}
	notManagedPath = map[string]bool{"bin/alfred": true}
)

// ManagedFiles lists, sorted, the files under the payload items of root (or all of root when
// payload is empty), skipping .git and unmanaged files. The payload filter keeps the
// repository's own README, docs and installer out of the installation.
func ManagedFiles(root string, payload []string) ([]string, error) {
	roots := []string{root}
	if len(payload) > 0 {
		roots = make([]string, 0, len(payload))
		for _, item := range payload {
			roots = append(roots, filepath.Join(root, item))
		}
	}

	var found []string
	for _, entry := range roots {
		info, err := os.Stat(entry)
		if err != nil {
			// Missing payload items are skipped; the installer reports them earlier.
			continue
		}

		if !info.IsDir() {
			found = append(found, entry)
			continue
		}

		err = filepath.WalkDir(entry, func(path string, dirEntry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !dirEntry.IsDir() {
				found = append(found, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", entry, err)
		}
	}

	kept := found[:0]
	for _, path := range found {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		if hasGitPart(path) || notManagedName[filepath.Base(path)] || notManagedPath[filepath.ToSlash(rel)] {
			continue
		}
		kept = append(kept, path)
	}

	sort.Strings(kept)
	return kept, nil
}

// hasGitPart reports whether any component of path is a .git directory.
func hasGitPart(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".git" {
			return true
		}
	}
	return false
}

// SHA256 returns the hex digest of a file's contents.
func SHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// hashAll returns the SHA-256 of every path, hashed concurrently across the machine's cores,
// and the first error seen (if any).
func hashAll(paths []string) (map[string]string, error) {
	sums := make(map[string]string, len(paths))
	if len(paths) == 0 {
		return sums, nil
	}

	workers := min(runtime.GOMAXPROCS(0), len(paths))

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
	)
	jobs := make(chan string)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				sum, err := SHA256(path)
				mu.Lock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
				} else {
					sums[path] = sum
				}
				mu.Unlock()
			}
		}()
	}

	for _, path := range paths {
		jobs <- path
	}
	close(jobs)
	wg.Wait()

	return sums, firstErr
}

// File is state.json: the installed version and a hash per managed file.
type File struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
}

// Write saves state.json at root with the version and a hash per managed file, and returns
// how many files were recorded.
func Write(root, version string, payload []string) (int, error) {
	files, err := ManagedFiles(root, payload)
	if err != nil {
		return 0, err
	}

	sums, err := hashAll(files)
	if err != nil {
		return 0, err
	}

	stateFile := File{Version: version, Files: make(map[string]string, len(files))}
	for _, path := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return 0, err
		}
		stateFile.Files[rel] = sums[path]
	}

	data, err := json.MarshalIndent(stateFile, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(root, "state.json"), append(data, '\n'), 0o644); err != nil {
		return 0, err
	}
	return len(files), nil
}

// Version returns the version recorded in home's state.json, not one baked into the binary,
// because a stale helper binary may sit beside a newer payload. A missing version is an
// error rather than "unknown", since callers compare the answer.
func Version(home string) (string, error) {
	data, err := os.ReadFile(filepath.Join(home, "state.json"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", notInstalled(home)
	case err != nil:
		return "", fmt.Errorf("reading state.json: %w", err)
	}

	var recorded File
	if err := json.Unmarshal(data, &recorded); err != nil {
		return "", fmt.Errorf("reading state.json: %w", err)
	}
	if recorded.Version == "" {
		return "", notInstalled(home)
	}
	return recorded.Version, nil
}

// notInstalled is the error for a home with no usable installation record.
func notInstalled(home string) error {
	return fmt.Errorf("no installation at %s; run: install.sh install", home)
}

// Report lists source files by status (relative paths): not installed yet, identical,
// edited by the user since install, or safe to update.
type Report struct {
	New       []string `json:"new"`
	Unchanged []string `json:"unchanged"`
	Modified  []string `json:"modified"`
	Updatable []string `json:"updatable"`
}

// Compare classifies every source file against what is installed at root. A file whose
// installed hash differs from the recorded one is Modified, so user edits are not overwritten.
func Compare(root, source string, payload []string) (*Report, error) {
	var recorded File
	if data, err := os.ReadFile(filepath.Join(root, "state.json")); err == nil {
		if err := json.Unmarshal(data, &recorded); err != nil {
			return nil, fmt.Errorf("reading state.json: %w", err)
		}
	}

	sources, err := ManagedFiles(source, payload)
	if err != nil {
		return nil, err
	}

	// Hash both sides up front, in parallel, so each file is read only once.
	sourceSums, err := hashAll(sources)
	if err != nil {
		return nil, err
	}

	relPaths := make([]string, len(sources))
	installed := make([]string, 0, len(sources))
	for i, sourcePath := range sources {
		rel, err := filepath.Rel(source, sourcePath)
		if err != nil {
			return nil, err
		}
		relPaths[i] = rel

		target := filepath.Join(root, rel)
		if _, err := os.Stat(target); err == nil {
			installed = append(installed, target)
		}
	}

	installedSums, err := hashAll(installed)
	if err != nil {
		return nil, err
	}

	report := &Report{}
	for i, sourcePath := range sources {
		rel := relPaths[i]
		target := filepath.Join(root, rel)

		installedSum, present := installedSums[target]
		recordedSum, wasRecorded := recorded.Files[rel]
		switch {
		case !present:
			report.New = append(report.New, rel)
		case wasRecorded && installedSum != recordedSum:
			report.Modified = append(report.Modified, rel)
		case installedSum == sourceSums[sourcePath]:
			report.Unchanged = append(report.Unchanged, rel)
		default:
			report.Updatable = append(report.Updatable, rel)
		}
	}

	return report, nil
}

// JSON renders the report on one line with every section present (empty as []), so the
// installer can read any of them without checking first.
func (r *Report) JSON() string {
	complete := Report{New: r.New, Unchanged: r.Unchanged, Modified: r.Modified, Updatable: r.Updatable}
	for _, section := range []*[]string{&complete.New, &complete.Unchanged, &complete.Modified, &complete.Updatable} {
		if *section == nil {
			*section = []string{}
		}
	}
	data, err := json.Marshal(complete)
	if err != nil {
		panic(err)
	}
	return string(data)
}
