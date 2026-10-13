package hook

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/markosAMO/alfred/internal/contract"
)

// repository returns a temporary repository root holding .alfred/, and the protected set of
// an installation under a separate temporary home.
func repository(t *testing.T) (string, Protected) {
	t.Helper()
	root, home := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".alfred"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, Fixed(filepath.Join(home, ".config", "alfred"), home)
}

func TestAlfredsOwnFilesAreRefusedWhateverTheContractAllows(t *testing.T) {
	root, protected := repository(t)
	for _, target := range []string{
		filepath.Join(root, ".alfred", "skills", "spec", "contract.json"),
		filepath.Join(root, ".alfred", "config.yaml"),
		filepath.Join(root, ".claude", "agents", "alfred-spec.md"),
		filepath.Join(protected.Directories[0], "skills", "spec", "SKILL.md"),
	} {
		if ok, _ := Judge(root, target, contract.Filesystem{}, protected); ok {
			t.Errorf("writing %s was allowed", target)
		}
	}
	if ok, _ := Judge(root, filepath.Join(root, ".alfred", "changes", "a", "spec.md"), contract.Filesystem{}, protected); !ok {
		t.Error("writing a change's document was refused")
	}
}

func TestPathsAreJudgedFromTheRepositoryRootEvenFromASubdirectory(t *testing.T) {
	root, protected := repository(t)
	sub := filepath.Join(root, "pkg", "inner")
	rules := contract.Filesystem{Write: []string{"pkg/**"}}

	if ok, _ := Judge(sub, "file.go", rules, protected); !ok {
		t.Error("a relative path under write, from a subdirectory, was refused")
	}
	if ok, _ := Judge(sub, "../../src/a.go", rules, protected); ok {
		t.Error("a relative path outside write was allowed")
	}
	if ok, _ := Judge(root, filepath.Join(t.TempDir(), "x"), rules, protected); ok {
		t.Error("a path outside the repository was allowed under a write list")
	}
}

func TestPreToolPrintsARefusalOnlyWhenTheWriteIsRefused(t *testing.T) {
	root, protected := repository(t)
	rules := contract.Filesystem{DenyWrite: []string{"src/**"}}

	var refused bytes.Buffer
	call := `{"cwd":"` + root + `","tool_name":"Write","tool_input":{"file_path":"src/a.go"}}`
	if err := PreTool(strings.NewReader(call), &refused, rules, protected); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(refused.String(), `"permissionDecision":"deny"`) {
		t.Errorf("refused write printed %q", refused.String())
	}

	var allowed bytes.Buffer
	call = `{"cwd":"` + root + `","tool_name":"NotebookEdit","tool_input":{"notebook_path":"docs/a.ipynb"}}`
	if err := PreTool(strings.NewReader(call), &allowed, rules, protected); err != nil {
		t.Fatal(err)
	}
	if allowed.Len() != 0 {
		t.Errorf("allowed write printed %q; an allow would skip the harness's own checks", allowed.String())
	}

	if err := PreTool(strings.NewReader("not json"), &allowed, rules, protected); err == nil {
		t.Error("an unreadable call was judged instead of refused")
	}
}

func TestTheEnvelopeIsReadFromTheLastFencedBlock(t *testing.T) {
	message := "Done.\n\n```yaml\nstatus: failed\n```\n\nResult:\n\n```yaml\n" +
		"change: login-google\nstatus: completed\nfiles_changed: [a.go]\n  nested: ignored\n" +
		"summary: \"one: paragraph\"\n```\n"
	got := Envelope(message)
	want := map[string]string{
		"change": "login-google", "status": "completed",
		"files_changed": "[a.go]", "summary": "one: paragraph",
	}
	if len(got) != len(want) {
		t.Fatalf("Envelope = %v, want %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %q, want %q", key, got[key], value)
		}
	}
}

func TestMissingNamesEveryUnmetExpectation(t *testing.T) {
	root, _ := repository(t)
	expected := Expected{
		Outputs: []string{".alfred/changes/{change}/spec.md"},
		Result:  &contract.Result{Required: []string{"status", "summary"}, Status: []string{"completed", "blocked"}},
	}

	got := Missing(root, "change: login\nstatus: done\n", expected)
	for _, want := range []string{"no summary", `status "done"`, ".alfred/changes/login/spec.md does not exist"} {
		if !strings.Contains(strings.Join(got, "; "), want) {
			t.Errorf("Missing = %v, want it to mention %q", got, want)
		}
	}

	if got := Missing(root, "change: ../../etc\n", Expected{Outputs: expected.Outputs}); len(got) != 1 ||
		!strings.Contains(got[0], "names no change") {
		t.Errorf("a change escaping the changes directory = %v, want it refused", got)
	}

	spec := filepath.Join(root, ".alfred", "changes", "login", "spec.md")
	if err := os.MkdirAll(filepath.Dir(spec), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte("# spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Missing(root, "change: login\nstatus: completed\nsummary: done\n", expected); len(got) != 0 {
		t.Errorf("a phase that did everything = %v, want nothing missing", got)
	}
}

func TestStopRefusesOnceThenLetsThePhaseEnd(t *testing.T) {
	root, _ := repository(t)
	expected := Expected{Result: &contract.Result{Required: []string{"status"}}}

	var first bytes.Buffer
	event := `{"cwd":"` + root + `","stop_hook_active":false,"last_assistant_message":"no envelope"}`
	if err := StopCheck(strings.NewReader(event), &first, expected); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.String(), `"decision":"block"`) || !strings.Contains(first.String(), "no status") {
		t.Errorf("first stop printed %q, want a refusal naming the missing key", first.String())
	}

	var second bytes.Buffer
	event = `{"cwd":"` + root + `","stop_hook_active":true,"last_assistant_message":"no envelope"}`
	if err := StopCheck(strings.NewReader(event), &second, expected); err != nil {
		t.Fatal(err)
	}
	if second.Len() != 0 {
		t.Errorf("a stop already continuing from a refusal printed %q, want it let through", second.String())
	}
}
