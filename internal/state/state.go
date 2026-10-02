// Package state tracks which installed files Alfred manages, by content hash.
//
// A file whose hash still matches the one recorded at install time was not touched by the
// user and may be replaced. A file whose hash differs was modified and is reported instead
// of overwritten.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// notManaged are written by the installation rather than shipped with it, so they are
// never recorded and never replaced.
var notManaged = map[string]bool{"state.json": true, "profile.json": true}

// ManagedFiles lists the files Alfred owns, sorted.
//
// Only the payload is installed: the repository also holds its own README, licence,
// installer and documentation, none of which belong in an installation. Without this
// filter an update copies the whole repository into the install directory.
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
			// A payload item that is not there is skipped: the installer reports a missing
			// item before it gets here.
			continue
		}

		if !info.IsDir() {
			found = append(found, entry)
			continue
		}

		err = filepath.WalkDir(entry, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				found = append(found, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", entry, err)
		}
	}

	kept := found[:0]
	for _, p := range found {
		if hasGitPart(p) || notManaged[filepath.Base(p)] {
			continue
		}
		kept = append(kept, p)
	}

	sort.Strings(kept)
	return kept, nil
}

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
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashAll digests every path concurrently. Hashing is the whole cost of both commands and
// the files are independent, so the work is spread over the machine's cores.
func hashAll(paths []string) (map[string]string, error) {
	out := make(map[string]string, len(paths))
	if len(paths) == 0 {
		return out, nil
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
					out[path] = sum
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

	return out, firstErr
}

// File is state.json: the installed version and a hash per managed file.
type File struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
}

// Write records the version and a hash per managed file, and returns how many were
// recorded.
func Write(root, version string, payload []string) (int, error) {
	files, err := ManagedFiles(root, payload)
	if err != nil {
		return 0, err
	}

	sums, err := hashAll(files)
	if err != nil {
		return 0, err
	}

	doc := File{Version: version, Files: make(map[string]string, len(files))}
	for _, path := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return 0, err
		}
		doc.Files[rel] = sums[path]
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(root, "state.json"), append(data, '\n'), 0o644); err != nil {
		return 0, err
	}
	return len(files), nil
}

// Report classifies every source file against what was installed.
type Report struct {
	New       []string `json:"new"`
	Unchanged []string `json:"unchanged"`
	Modified  []string `json:"modified"`
	Updatable []string `json:"updatable"`
}

// Compare classifies every source file against what was installed.
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

	// Both sides are hashed up front and in parallel, so each file is read once no matter
	// how many of the branches below would have asked for its digest.
	srcSums, err := hashAll(sources)
	if err != nil {
		return nil, err
	}

	rels := make([]string, len(sources))
	installed := make([]string, 0, len(sources))
	for i, src := range sources {
		rel, err := filepath.Rel(source, src)
		if err != nil {
			return nil, err
		}
		rels[i] = rel

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
	for i, src := range sources {
		rel := rels[i]
		target := filepath.Join(root, rel)

		got, present := installedSums[target]
		was, wasRecorded := recorded.Files[rel]
		switch {
		case !present:
			report.New = append(report.New, rel)
		case wasRecorded && got != was:
			report.Modified = append(report.Modified, rel)
		case got == srcSums[src]:
			report.Unchanged = append(report.Unchanged, rel)
		default:
			report.Updatable = append(report.Updatable, rel)
		}
	}

	return report, nil
}

// JSON renders the report on one line, every section present even when empty, so the
// installer can read any of them without checking first.
func (r *Report) JSON() string {
	out := Report{New: r.New, Unchanged: r.Unchanged, Modified: r.Modified, Updatable: r.Updatable}
	for _, section := range []*[]string{&out.New, &out.Unchanged, &out.Modified, &out.Updatable} {
		if *section == nil {
			*section = []string{}
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		panic(err)
	}
	return string(data)
}
