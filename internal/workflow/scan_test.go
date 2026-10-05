package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeWorkflow puts one workflow directory under a root. The definition is the valid one
// renamed, so a test that is about the scan changes nothing about the document.
//
// The rules file the definition names is written beside it, because a workflow whose rules
// file is missing is now refused and a test about anything else would be refused for the
// wrong reason. A test that is about the rules file removes it or repoints it afterwards.
func writeWorkflow(t *testing.T, root, name string, edit func(map[string]any)) string {
	t.Helper()

	definition := valid()
	definition["name"] = name
	if edit != nil {
		edit(definition)
	}
	body, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, DefinitionFile), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if rules, ok := definition["rules"].(string); ok && rules != "" && !filepath.IsAbs(rules) {
		path := filepath.Join(dir, rules)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("how "+name+" decides"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func machineRoots(home string) []Root {
	return []Root{
		{Label: "machine/shipped", Dir: filepath.Join(home, "workflows")},
		{Label: "machine/custom", Dir: filepath.Join(home, "custom", "workflows")},
	}
}

func TestScanReadsEveryRootAndRecordsWhichOneEachCameFrom(t *testing.T) {
	home := t.TempDir()
	roots := machineRoots(home)
	shipped := writeWorkflow(t, roots[0].Dir, "ventas", nil)
	custom := writeWorkflow(t, roots[1].Dir, "compras", nil)

	found, err := Scan(roots)
	if err != nil {
		t.Fatalf("Scan = %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("Scan found %d workflows, want 2: %+v", len(found), found)
	}

	// Name order, so two runs over the same roots report in the same order.
	if found[0].Name != "compras" || found[1].Name != "ventas" {
		t.Fatalf("Scan = %q, %q, want them in name order", found[0].Name, found[1].Name)
	}
	for _, want := range []struct {
		at     int
		source string
		dir    string
	}{
		{0, "machine/custom", custom},
		{1, "machine/shipped", shipped},
	} {
		got := found[want.at]
		if got.Source != want.source {
			t.Errorf("%s came from %q, want %q", got.Name, got.Source, want.source)
		}
		if got.Dir != want.dir {
			t.Errorf("%s is at %q, want %q", got.Name, got.Dir, want.dir)
		}
		if got.Definition == nil || got.Reason != "" {
			t.Errorf("%s was not registered: %q", got.Name, got.Reason)
		}
	}
}

// The custom root is never created by the installer, so an absent root is the ordinary
// case and not a failure.
func TestScanTreatsAnAbsentRootAsNoWorkflows(t *testing.T) {
	home := t.TempDir()
	roots := machineRoots(home)
	writeWorkflow(t, roots[0].Dir, "ventas", nil)

	found, err := Scan(roots)
	if err != nil {
		t.Fatalf("Scan over an absent custom root = %v", err)
	}
	if len(found) != 1 || found[0].Name != "ventas" {
		t.Fatalf("Scan = %+v, want only ventas", found)
	}

	empty, err := Scan(machineRoots(t.TempDir()))
	if err != nil {
		t.Fatalf("Scan over two absent roots = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("Scan over two absent roots = %+v, want nothing", empty)
	}
}

func TestScanRegistersNeitherCopyOfANameInBothRoots(t *testing.T) {
	home := t.TempDir()
	roots := machineRoots(home)
	writeWorkflow(t, roots[0].Dir, "ventas", nil)
	writeWorkflow(t, roots[1].Dir, "ventas", nil)

	found, err := Scan(roots)
	if err != nil {
		t.Fatalf("Scan = %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("Scan = %+v, want one entry for the duplicated name", found)
	}

	got := found[0]
	if got.Name != "ventas" {
		t.Errorf("name = %q, want it to name the duplicate", got.Name)
	}
	if got.Definition != nil {
		t.Error("one of the two copies was registered")
	}
	if got.Dir != "" {
		t.Errorf("dir = %q, want neither copy to be addressed", got.Dir)
	}
	want := "exists in both roots, machine/shipped and machine/custom; neither copy is registered"
	if got.Reason != want {
		t.Errorf("reason = %q, want %q", got.Reason, want)
	}
	for _, label := range []string{"machine/shipped", "machine/custom"} {
		if !strings.Contains(got.Source, label) {
			t.Errorf("source = %q, want it to name %s", got.Source, label)
		}
	}
}

// Neither scope has three roots today, but Scan takes as many as it is handed and a
// reason reading "both roots" while naming three would be the report lying about what it
// found.
func TestScanNamesEveryRootANameWasFoundIn(t *testing.T) {
	home := t.TempDir()
	roots := append(machineRoots(home), Root{Label: "machine/extra", Dir: filepath.Join(home, "extra")})
	for _, root := range roots {
		writeWorkflow(t, root.Dir, "ventas", nil)
	}

	found, err := Scan(roots)
	if err != nil {
		t.Fatal(err)
	}
	want := "exists in 3 roots, machine/shipped, machine/custom, machine/extra; no copy is registered"
	if len(found) != 1 || found[0].Reason != want {
		t.Errorf("Scan = %+v, want one entry refused with %q", found, want)
	}
}

// Which root the second copy was created in is not a fact the scan can see, and a report
// that depended on it would change for a reason the user cannot act on.
func TestScanReportsADuplicateTheSameWayWhicheverRootItWasCreatedIn(t *testing.T) {
	first := t.TempDir()
	writeWorkflow(t, machineRoots(first)[0].Dir, "ventas", nil)
	writeWorkflow(t, machineRoots(first)[1].Dir, "ventas", nil)

	second := t.TempDir()
	writeWorkflow(t, machineRoots(second)[1].Dir, "ventas", nil)
	writeWorkflow(t, machineRoots(second)[0].Dir, "ventas", nil)

	one, err := Scan(machineRoots(first))
	if err != nil {
		t.Fatal(err)
	}
	other, err := Scan(machineRoots(second))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(one, other) {
		t.Errorf("the two orders disagree:\n%+v\n%+v", one, other)
	}
}

func TestScanRejectsADefinitionAndKeepsTheRest(t *testing.T) {
	home := t.TempDir()
	roots := machineRoots(home)
	writeWorkflow(t, roots[0].Dir, "ventas", nil)
	writeWorkflow(t, roots[1].Dir, "roto", func(d map[string]any) {
		d["routes"] = map[string]any{"rapida": []any{"propuest"}}
		d["default_route"] = "rapida"
	})

	found, err := Scan(roots)
	if err != nil {
		t.Fatalf("Scan = %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("Scan = %+v, want the rejected workflow listed beside the valid one", found)
	}

	broken, ok := byName(found, "roto")
	if !ok {
		t.Fatal("the rejected workflow is missing from the scan")
	}
	want := `route "rapida" names phase "propuest", which it does not declare`
	if broken.Reason != want {
		t.Errorf("reason = %q, want %q", broken.Reason, want)
	}
	if broken.Definition != nil {
		t.Error("a rejected workflow carries a definition")
	}
	if broken.Source != "machine/custom" {
		t.Errorf("source = %q, want the root it came from", broken.Source)
	}

	if valid, _ := byName(found, "ventas"); valid.Definition == nil {
		t.Error("a valid workflow was held back by an invalid one")
	}
}

// Registration exits 0 and every run under the command then stops at the rules file it
// cannot read, which is the first-use failure registration exists to prevent.
func TestScanRejectsAWorkflowWhoseRulesFileIsMissing(t *testing.T) {
	home := t.TempDir()
	roots := machineRoots(home)
	writeWorkflow(t, roots[0].Dir, "ventas", nil)
	dir := writeWorkflow(t, roots[1].Dir, "compras", func(d map[string]any) {
		d["rules"] = "notes.md"
	})
	if err := os.Remove(filepath.Join(dir, "notes.md")); err != nil {
		t.Fatal(err)
	}

	found, err := Scan(roots)
	if err != nil {
		t.Fatalf("Scan = %v", err)
	}

	broken, ok := byName(found, "compras")
	if !ok {
		t.Fatal("the rejected workflow is missing from the scan")
	}
	want := `rules file "notes.md" does not exist`
	if broken.Reason != want {
		t.Errorf("reason = %q, want %q", broken.Reason, want)
	}
	if broken.Definition != nil {
		t.Error("a workflow whose rules file is missing was registered")
	}
	if valid, _ := byName(found, "ventas"); valid.Definition == nil {
		t.Error("a valid workflow was held back by one with no rules file")
	}
}

// The rules path is rendered into a command that tells the orchestrator to read it, so a
// definition arriving with a clone must not be able to address a file outside its own
// directory.
func TestScanRejectsAWorkflowWhoseRulesFileLeavesItsDirectory(t *testing.T) {
	home := t.TempDir()
	roots := machineRoots(home)
	writeWorkflow(t, roots[0].Dir, "ventas", func(d map[string]any) {
		d["rules"] = "../../../../.ssh/config"
	})

	found, err := Scan(roots)
	if err != nil {
		t.Fatalf("Scan = %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("Scan = %+v, want the workflow reported", found)
	}

	want := `rules file "../../../../.ssh/config" resolves outside the workflow's directory`
	if found[0].Reason != want {
		t.Errorf("reason = %q, want %q", found[0].Reason, want)
	}
	if found[0].Definition != nil {
		t.Error("a workflow addressing a rules file outside its directory was registered")
	}
}

func TestScanRejectsADirectoryWithNoDefinition(t *testing.T) {
	home := t.TempDir()
	roots := machineRoots(home)
	if err := os.MkdirAll(filepath.Join(roots[0].Dir, "ventas"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := Scan(roots)
	if err != nil {
		t.Fatalf("Scan = %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("Scan = %+v, want the directory reported", found)
	}
	if !strings.Contains(found[0].Reason, DefinitionFile) {
		t.Errorf("reason = %q, want it to name %s", found[0].Reason, DefinitionFile)
	}
}

func TestScanLooksOnlyAtDirectories(t *testing.T) {
	home := t.TempDir()
	roots := machineRoots(home)
	writeWorkflow(t, roots[0].Dir, "ventas", nil)
	if err := os.WriteFile(filepath.Join(roots[0].Dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(roots[0].Dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := Scan(roots)
	if err != nil {
		t.Fatalf("Scan = %v", err)
	}
	if len(found) != 1 || found[0].Name != "ventas" {
		t.Fatalf("Scan = %+v, want only the workflow directory", found)
	}
}

// A root that exists and cannot be read is not the same as a root that is not there: read
// as nothing, it would tell the caller to remove every command it had generated.
func TestScanReportsARootItCannotRead(t *testing.T) {
	home := t.TempDir()
	roots := machineRoots(home)
	if err := os.MkdirAll(roots[0].Dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roots[0].Dir, 0o755) })
	if _, err := os.ReadDir(roots[0].Dir); err == nil {
		t.Skip("this user can read a directory with no permissions")
	}

	if _, err := Scan(roots); err == nil {
		t.Error("an unreadable root was read as no workflows")
	}
}

func byName(found []Found, name string) (Found, bool) {
	for _, f := range found {
		if f.Name == name {
			return f, true
		}
	}
	return Found{}, false
}
