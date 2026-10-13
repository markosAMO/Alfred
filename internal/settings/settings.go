// Package settings reads Claude Code's settings files to check whether starting a session
// is permitted. It reads the settings instead of trying the command, because trying it
// would start (and spend) a session.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// SessionPermissionGranted reports whether any user or project settings file allows a
// `Bash(claude...` rule. false means "this may block you", not "this will".
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
		var doc struct {
			Permissions struct {
				Allow []any `json:"allow"`
			} `json:"permissions"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			continue
		}
		for _, item := range doc.Permissions.Allow {
			rule, _ := item.(string)
			if strings.HasPrefix(rule, "Bash(claude") {
				return true
			}
		}
	}
	return false
}
