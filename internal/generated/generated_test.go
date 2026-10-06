package generated

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func sum(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

func write(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func equal(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

// TestADeletedWorkflowLosesEveryFileTheManifestClaims is the scenario "a deleted
// workflow", on the agent whose generated things are files: the command and the subagents
// of a workflow that no longer exists are removed, and the sweep names them so the report
// can list them.
func TestADeletedWorkflowLosesEveryFileTheManifestClaims(t *testing.T) {
	dir := t.TempDir()
	gone := write(t, filepath.Join(dir, "alfred-ventas.md"), "ventas")
	kept := write(t, filepath.Join(dir, "alfred-sdd.md"), "sdd")

	previous := &Manifest{Agents: map[string]Agent{"claude": {
		Files: map[string]string{gone: sum("ventas"), kept: sum("sdd")},
	}}}

	set := NewSet()
	set.AddFile("claude", kept, []byte("sdd"))

	sweep, err := PlanFiles(previous, set, "claude", []string{gone, kept})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "Remove", sweep.Remove, []string{gone})
	equal(t, "Modified", sweep.Modified, nil)
	equal(t, "Orphans", sweep.Orphans, nil)
	equal(t, "Collisions", sweep.Collisions, nil)

	if err := Remove(sweep.Remove); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Errorf("the deleted workflow's file is still on disk: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("the surviving workflow's file was removed: %v", err)
	}
}

// The same scenario on the agent whose generated things are keys inside a file it owns.
func TestADeletedWorkflowLosesEveryKeyTheManifestClaims(t *testing.T) {
	previous := &Manifest{Agents: map[string]Agent{"opencode": {
		Keys: map[string]string{"alfred-ventas": sum(`{"a":1}`), "alfred-sdd": sum(`{"b":2}`)},
	}}}

	set := NewSet()
	set.AddKey("opencode", "alfred-sdd", []byte(`{"b":2}`))

	sweep := PlanKeys(previous, set, "opencode", map[string][]byte{
		"alfred-ventas": []byte(`{"a":1}`),
		"alfred-sdd":    []byte(`{"b":2}`),
	})
	equal(t, "Remove", sweep.Remove, []string{"alfred-ventas"})
	equal(t, "Modified", sweep.Modified, nil)
	equal(t, "Orphans", sweep.Orphans, nil)
	equal(t, "Collisions", sweep.Collisions, nil)
}

// TestAFileTheUserWroteIsNeverRemoved is the scenario "a file the user wrote": a file the
// manifest does not claim is never removed, however it is called, and it is reported as a
// collision when registration would generate that same name.
func TestAFileTheUserWroteIsNeverRemoved(t *testing.T) {
	dir := t.TempDir()
	notes := write(t, filepath.Join(dir, "alfred-notes.md"), "the user's")
	collides := write(t, filepath.Join(dir, "alfred-ventas.md"), "the user's too")

	previous := &Manifest{Agents: map[string]Agent{"claude": {}}}

	set := NewSet()
	set.AddFile("claude", collides, []byte("generated"))

	sweep, err := PlanFiles(previous, set, "claude", []string{notes, collides})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "Remove", sweep.Remove, nil)
	equal(t, "Orphans", sweep.Orphans, []string{notes})
	equal(t, "Collisions", sweep.Collisions, []string{collides})

	if err := Remove(sweep.Remove); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{notes, collides} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("a file the user wrote was removed: %v", err)
		}
	}
}

// An agent key the user added is the same case, and a key that carries no generated
// naming is not Alfred's business at all.
func TestAKeyTheUserWroteIsNeverRemoved(t *testing.T) {
	previous := &Manifest{Agents: map[string]Agent{"opencode": {}}}

	set := NewSet()
	set.AddKey("opencode", "alfred-ventas", []byte(`{"a":1}`))

	sweep := PlanKeys(previous, set, "opencode", map[string][]byte{
		"alfred-notes":  []byte(`{"user":true}`),
		"alfred-ventas": []byte(`{"user":true}`),
		"reviewer":      []byte(`{"user":true}`),
	})
	equal(t, "Remove", sweep.Remove, nil)
	equal(t, "Orphans", sweep.Orphans, []string{"alfred-notes"})
	equal(t, "Collisions", sweep.Collisions, []string{"alfred-ventas"})
}

// A claimed file whose hash changed was edited after it was generated, so it is reported
// and left alone, which is the rule every managed file already has.
func TestAClaimEditedSinceIsReportedAndLeftAlone(t *testing.T) {
	dir := t.TempDir()
	edited := write(t, filepath.Join(dir, "alfred-ventas.md"), "edited by hand")

	previous := &Manifest{Agents: map[string]Agent{"claude": {
		Files: map[string]string{edited: sum("as generated")},
		Keys:  map[string]string{"alfred-ventas": sum(`{"a":1}`)},
	}}}

	files, err := PlanFiles(previous, NewSet(), "claude", []string{edited})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "Remove", files.Remove, nil)
	equal(t, "Modified", files.Modified, []string{edited})
	if _, err := os.Stat(edited); err != nil {
		t.Errorf("a modified file was removed: %v", err)
	}

	keys := PlanKeys(previous, NewSet(), "claude", map[string][]byte{"alfred-ventas": []byte(`{"a":2}`)})
	equal(t, "Remove", keys.Remove, nil)
	equal(t, "Modified", keys.Modified, []string{"alfred-ventas"})
}

// The first run after this change has no manifest: with nothing claimed, nothing is
// removed, and what is on disk is reported instead.
func TestTheFirstRunHasNoManifestSoNothingIsRemoved(t *testing.T) {
	dir := t.TempDir()
	stale := write(t, filepath.Join(dir, "alfred-init.md"), "obsolete subagent")

	set := NewSet()
	set.AddKey("opencode", "alfred-sdd", []byte(`{"a":1}`))

	files, err := PlanFiles(nil, set, "opencode", []string{stale})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "Remove", files.Remove, nil)
	equal(t, "Modified", files.Modified, nil)
	equal(t, "Orphans", files.Orphans, []string{stale})

	keys := PlanKeys(nil, set, "opencode", map[string][]byte{"alfred-sdd": []byte(`{"a":0}`)})
	equal(t, "Remove", keys.Remove, nil)
	equal(t, "Collisions", keys.Collisions, []string{"alfred-sdd"})
}

// A claim whose file is already gone is nothing to do and nothing to report.
func TestAClaimAlreadyGoneFromDiskIsNotReported(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "alfred-ventas.md")

	previous := &Manifest{Agents: map[string]Agent{"claude": {
		Files: map[string]string{missing: sum("ventas")},
		Keys:  map[string]string{"alfred-ventas": sum(`{"a":1}`)},
	}}}

	files, err := PlanFiles(previous, NewSet(), "claude", nil)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "Remove", files.Remove, nil)
	equal(t, "Modified", files.Modified, nil)
	equal(t, "Orphans", files.Orphans, nil)

	keys := PlanKeys(previous, NewSet(), "claude", nil)
	equal(t, "Remove", keys.Remove, nil)
	equal(t, "Modified", keys.Modified, nil)
}

// One scope's manifest holds every agent, and a run of one agent never removes or reports
// what another agent's section records.
func TestOneAgentsSweepIgnoresAnothersClaims(t *testing.T) {
	dir := t.TempDir()
	claude := write(t, filepath.Join(dir, "alfred-ventas.md"), "ventas")

	previous := &Manifest{Agents: map[string]Agent{
		"claude":   {Files: map[string]string{claude: sum("ventas")}},
		"opencode": {Keys: map[string]string{"alfred-ventas": sum(`{"a":1}`)}},
	}}

	sweep, err := PlanFiles(previous, NewSet(), "opencode", []string{claude})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "Remove", sweep.Remove, nil)
	equal(t, "Orphans", sweep.Orphans, []string{claude})
}

func TestAManifestRoundTripsAndIsStable(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "generated.json")

	set := NewSet()
	set.AddFile("claude", "/home/x/.claude/commands/alfred-ventas.md", []byte("command"))
	set.AddFile("claude", "/home/x/.claude/agents/alfred-ventas-propuesta.md", []byte("agent"))
	set.AddKey("opencode", "alfred-ventas", []byte(`{"a":1}`))

	if err := set.Manifest().Write(path); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(first), "}\n") {
		t.Errorf("manifest does not end in a newline: %q", string(first))
	}

	read, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := read.Agents["claude"].Files["/home/x/.claude/commands/alfred-ventas.md"]; got != sum("command") {
		t.Errorf("file hash = %q, want %q", got, sum("command"))
	}
	if got := read.Agents["opencode"].Keys["alfred-ventas"]; got != sum(`{"a":1}`) {
		t.Errorf("key hash = %q, want %q", got, sum(`{"a":1}`))
	}

	if err := read.Write(path); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("rewriting the same manifest changed its bytes:\n%s\n%s", first, second)
	}
}

func TestReadWithoutAManifestIsNoClaimsAndNoError(t *testing.T) {
	got, err := Read(filepath.Join(t.TempDir(), "generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("Read = %v, want nil", got)
	}
}

func TestReadRejectsAManifestItCannotDecode(t *testing.T) {
	path := write(t, filepath.Join(t.TempDir(), "generated.json"), "{not json")
	if _, err := Read(path); err == nil {
		t.Fatal("a corrupt manifest was accepted")
	}
}

// An agent this run generated nothing for may be planned with no set at all, which is
// what report mode and an agent with no workflows both are.
func TestAnAgentThatGeneratedNothingNeedsNoSet(t *testing.T) {
	previous := &Manifest{Agents: map[string]Agent{"opencode": {
		Keys: map[string]string{"alfred-ventas": sum(`{"a":1}`)},
	}}}

	sweep := PlanKeys(previous, nil, "opencode", map[string][]byte{"alfred-ventas": []byte(`{"a":1}`)})
	equal(t, "Remove", sweep.Remove, []string{"alfred-ventas"})
}

// Nothing here swallows an I/O failure: a path that is not the kind of thing it was taken
// for is reported rather than read as nothing.
func TestEveryReadAndWriteReportsItsFailure(t *testing.T) {
	dir := t.TempDir()
	notAFile := filepath.Join(dir, "directory")
	if err := os.MkdirAll(filepath.Join(notAFile, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	aFile := write(t, filepath.Join(dir, "file"), "x")

	if _, err := Read(notAFile); err == nil {
		t.Error("Read accepted a directory")
	}
	if err := (&Manifest{}).Write(filepath.Join(aFile, "generated.json")); err == nil {
		t.Error("Write accepted a parent that is a file")
	}
	if _, err := Scan(aFile); err == nil {
		t.Error("Scan accepted a file")
	}
	if err := Remove([]string{notAFile}); err == nil {
		t.Error("Remove accepted a directory it could not remove")
	}

	claims := &Manifest{Agents: map[string]Agent{"claude": {
		Files: map[string]string{notAFile: sum("x")},
	}}}
	if _, err := PlanFiles(claims, NewSet(), "claude", nil); err == nil {
		t.Error("PlanFiles accepted a claim on a directory")
	}
}

func TestScanFindsTheGeneratedNamingAndNothingElse(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "alfred.md"), "x")
	write(t, filepath.Join(dir, "alfred-ventas.md"), "x")
	write(t, filepath.Join(dir, "alfredo.md"), "x")
	write(t, filepath.Join(dir, "notes.md"), "x")
	write(t, filepath.Join(dir, "alfred-nested", "alfred-deep.md"), "x")

	found, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "Scan", found, []string{
		filepath.Join(dir, "alfred-ventas.md"),
		filepath.Join(dir, "alfred.md"),
	})

	absent, err := Scan(filepath.Join(dir, "no-such-directory"))
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "Scan of an absent directory", absent, nil)
}
