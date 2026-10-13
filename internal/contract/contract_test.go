package contract

import (
	"strings"
	"testing"
)

func TestMatchTreatsDoubleStarAsAnyDepthAndChangeAsOneSegment(t *testing.T) {
	for _, tc := range []struct {
		pattern, path string
		want          bool
	}{
		{"src/**", "src/a.go", true},
		{"src/**", "src/a/b/c.go", true},
		{"src/**", "src", true},
		{"src/**", "srcs/a.go", false},
		{"**/*.md", "a/b/c.md", true},
		{"**/*.md", "c.md", true},
		{"docs/*.md", "docs/a/b.md", false},
		{".alfred/changes/{change}/**", ".alfred/changes/login/spec.md", true},
		{".alfred/changes/{change}/spec.md", ".alfred/changes/a/b/spec.md", false},
		{"opencode.json", "opencode.json", true},
	} {
		if got := Match(tc.pattern, tc.path); got != tc.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestDenyWinsOverWriteAndAnEmptyWriteAllowsEverything(t *testing.T) {
	rules := Filesystem{Write: []string{"docs/**"}, DenyWrite: []string{"docs/private/**"}}
	if ok, _ := rules.Allows("docs/a.md"); !ok {
		t.Error("a path under write was refused")
	}
	if ok, reason := rules.Allows("docs/private/a.md"); ok || !strings.Contains(reason, "docs/private/**") {
		t.Errorf("a denied path = %v, %q; want refused naming the rule", ok, reason)
	}
	if ok, _ := rules.Allows("src/a.go"); ok {
		t.Error("a path outside write was allowed")
	}
	if ok, _ := (Filesystem{}).Allows("anything/at/all"); !ok {
		t.Error("no rules refused a write")
	}
}

func TestDecodeRefusesWhatCouldNeverApply(t *testing.T) {
	for name, body := range map[string]string{
		"unknown key":       `{"permissions":{"filesystem":{"writes":["a"]}}}`,
		"absolute":          `{"permissions":{"filesystem":{"write":["/etc/**"]}}}`,
		"parent":            `{"permissions":{"filesystem":{"write":["../a"]}}}`,
		"empty":             `{"permissions":{"filesystem":{"deny_write":[""]}}}`,
		"other placeholder": `{"permissions":{"filesystem":{"write":["{task}/a"]}}}`,
		"bad glob":          `{"permissions":{"filesystem":{"write":["a/[b"]}}}`,
		"two values":        `{} {}`,
		"output pattern":    `{"outputs":[".alfred/changes/{change}/*.md"]}`,
		"output escapes":    `{"outputs":["../spec.md"]}`,
		"key not readable":  `{"result":{"required":["Status"]}}`,
		"status colon":      `{"result":{"status":["done: yes"]}}`,
	} {
		if _, err := Decode([]byte(body)); err == nil {
			t.Errorf("%s: Decode accepted %s", name, body)
		}
	}
	valid := `{"permissions":{"filesystem":{"write":[".alfred/changes/{change}/**"]}},
		"outputs":[".alfred/changes/{change}/spec.md"],
		"result":{"required":["change","status"],"status":["completed","blocked"]}}`
	declared, err := Decode([]byte(valid))
	if err != nil {
		t.Fatalf("Decode refused a valid contract: %v", err)
	}
	if !declared.Checked() {
		t.Error("a contract with outputs and result asks for no check")
	}
	if guardOnly, _ := Decode([]byte(`{"permissions":{}}`)); guardOnly.Checked() {
		t.Error("a contract with only permissions asks for a check")
	}
}
