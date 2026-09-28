// Package settings reads the agent's own settings files.
//
// It exists for one check: whether starting a session is permitted. The installer reads the
// settings rather than trying the command, because starting a session to find out would
// cost one, and a permission that is written down is the thing being asked about.
package settings

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/markosAMO/alfred/internal/pyjson"
)

// SessionPermissionGranted reports whether any settings file allows the start command.
//
// A permissive mode can let the command through with no rule present, so a false here
// means "this may stop you", not "this will" - which is the safe direction for a check
// whose remedy is one line and harmless.
func SessionPermissionGranted(home string) bool {
	candidates := []string{
		filepath.Join(home, ".claude/settings.json"),
		filepath.Join(home, ".claude/settings.local.json"),
		".claude/settings.json",
		".claude/settings.local.json",
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		doc, err := pyjson.Decode(data)
		if err != nil {
			continue
		}
		for _, rule := range doc.Get("permissions").Get("allow").Strings() {
			if strings.HasPrefix(rule, "Bash(claude") {
				return true
			}
		}
	}
	return false
}
