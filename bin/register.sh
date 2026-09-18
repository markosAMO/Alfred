#!/usr/bin/env bash
#
# Register Alfred's commands and agents from the installed workflows.
#
# Generates one /alfred-<workflow> command per workflow found under the Alfred workflows
# root and the user's custom root, plus the phase subagents, for every agent detected on
# the machine. The installer calls this; so does the add-workflow operation; so can you,
# after creating a workflow directory by hand.
#
#   register.sh            regenerate everything
#   register.sh --check    report workflows without a registered command; exit 1 if any

set -euo pipefail

ALFRED_HOME="${ALFRED_HOME:-$HOME/.config/alfred}"
SCRIPTS="${ALFRED_SCRIPTS:-$ALFRED_HOME/scripts}"
PROFILE="$ALFRED_HOME/profile.json"

fail() { printf 'register: %s\n' "$1" >&2; exit "${2:-2}"; }

detect_agents() {
  local found=()
  command -v opencode >/dev/null 2>&1 && found+=(opencode)
  command -v claude   >/dev/null 2>&1 && found+=(claude)
  [ -d "$HOME/.claude" ] && [[ ! " ${found[*]} " =~ " claude " ]] && found+=(claude)
  printf '%s\n' "${found[@]:-}"
}

workflow_names() {
  local dir
  for dir in "$ALFRED_HOME"/workflows/*/ "$ALFRED_HOME"/custom/workflows/*/; do
    [ -f "${dir}workflow.yaml" ] || continue
    basename "$dir"
  done
}

cmd_check() {
  local name missing=0
  while read -r name; do
    [ -n "$name" ] || continue
    if detect_agents | grep -qx claude && [ ! -f "$HOME/.claude/commands/alfred-$name.md" ]; then
      printf 'workflow %s has no /alfred-%s command in Claude Code\n' "$name" "$name"
      missing=$((missing + 1))
    fi
    if detect_agents | grep -qx opencode && ! grep -q "\"alfred-$name\"" "$HOME/.config/opencode/opencode.json" 2>/dev/null; then
      printf 'workflow %s has no alfred-%s agent in OpenCode\n' "$name" "$name"
      missing=$((missing + 1))
    fi
  done < <(workflow_names)
  [ $missing -eq 0 ] && printf 'every workflow is registered\n' || { printf 'fix: run %s\n' "$0"; exit 1; }
}

cmd_register() {
  command -v python3 >/dev/null 2>&1 || fail "python3 is required"
  [ -s "$PROFILE" ] || fail "no model profile at $PROFILE; run the installer first"
  [ -f "$SCRIPTS/generate_agents.py" ] || fail "generator not found at $SCRIPTS/generate_agents.py; run the installer"

  local targets=() agent
  while read -r agent; do
    [ -n "$agent" ] || continue
    case "$agent" in
      opencode) targets+=("opencode=$HOME/.config/opencode/opencode.json") ;;
      claude)   targets+=("claude=$HOME/.claude") ;;
    esac
  done < <(detect_agents)
  [ ${#targets[@]} -gt 0 ] || fail "no supported agent found; nothing to register" 3

  python3 "$SCRIPTS/generate_agents.py" "$PROFILE" "$ALFRED_HOME/skills" "${targets[@]}"
}

case "${1:-}" in
  "")        cmd_register ;;
  --check)   cmd_check ;;
  -h|--help) sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//' ;;
  *)         fail "unknown argument: $1" ;;
esac
