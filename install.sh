#!/usr/bin/env bash
#
# Alfred installer.
#
# Installs the skills and protocols once per machine and registers an orchestrator with
# every agent found, so the orchestrator exists before any repository does. Repositories
# are set up from the orchestrator itself, with init.

set -euo pipefail

SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ALFRED_HOME="${ALFRED_HOME:-$HOME/.config/alfred}"
VERSION="0.2.0"

PHASES=(init explore refine research spec diagnose design tasks apply verify review archive)
PAYLOAD=(skills memory notify tracker templates defaults triggers bin alfred.config.yaml)

info() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

require_python() {
  command -v python3 >/dev/null 2>&1 || die "python3 is required"
}

# The helper that does the installer's bookkeeping and writes the agent definitions. The Go
# binary is preferred: it needs no interpreter and hashes the payload in parallel. The
# Python scripts remain as the fallback while both implementations are in the tree, so a
# machine without a Go toolchain still installs.
HELPER=""

setup_runtime() {
  [ -n "$HELPER" ] && return 0

  if [ -x "$SOURCE/.build/alfred" ]; then
    HELPER="$SOURCE/.build/alfred"
    return 0
  fi

  if command -v go >/dev/null 2>&1; then
    if (cd "$SOURCE" && go build -o .build/alfred ./cmd/alfred) >/dev/null 2>&1; then
      HELPER="$SOURCE/.build/alfred"
      return 0
    fi
    warn "the Go helper did not build; falling back to python3"
  fi

  require_python
}

helper_state() {
  if [ -n "$HELPER" ]; then "$HELPER" state "$@"
  else
    local op="$1"; shift
    python3 "$SOURCE/scripts/manage_state.py" "$op" "$@"
  fi
}

helper_agents() {
  if [ -n "$HELPER" ]; then "$HELPER" agents "$@"
  else python3 "$SOURCE/scripts/generate_agents.py" "$@"
  fi
}

helper_json_get() {
  if [ -n "$HELPER" ]; then "$HELPER" json-get "$1" "$2"
  else python3 -c 'import json,sys
src = sys.stdin if sys.argv[1] == "-" else open(sys.argv[1])
value = json.load(src)[sys.argv[2]]
print("\n".join(value) if isinstance(value, list) else value)' "$1" "$2"
  fi
}

helper_json_valid() {
  if [ -n "$HELPER" ]; then "$HELPER" json-valid "$1"
  else python3 -c 'import json,sys;json.load(open(sys.argv[1]))' "$1"
  fi
}

helper_state_apply() {
  if [ -n "$HELPER" ]; then "$HELPER" state apply "$1" "$2"
  else python3 -c '
import json, shutil, sys
from pathlib import Path

report = json.load(sys.stdin)
home, source = Path(sys.argv[1]), Path(sys.argv[2])
count = 0

for rel in report["new"] + report["updatable"]:
    target = home / rel
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source / rel, target)
    count += 1

print("%d files updated, %d left alone" % (count, len(report["modified"])))
' "$1" "$2"
  fi
}

# Agents are detected rather than asked about: an agent that is not installed has nowhere
# to register an orchestrator.
detect_agents() {
  local found=()
  command -v opencode >/dev/null 2>&1 && found+=(opencode)
  command -v claude   >/dev/null 2>&1 && found+=(claude)
  [ -d "$HOME/.claude" ] && [[ ! " ${found[*]} " =~ " claude " ]] && found+=(claude)
  printf '%s\n' "${found[@]:-}"
}

install_payload() {
  mkdir -p "$ALFRED_HOME"
  local item
  for item in "${PAYLOAD[@]}"; do
    [ -e "$SOURCE/$item" ] || die "missing from package: $item"
    cp -R "$SOURCE/$item" "$ALFRED_HOME/"
  done
  chmod +x "$ALFRED_HOME"/bin/*.sh
  info "installed to $ALFRED_HOME"
}

ask_model() {
  local label="$1" default="$2" answer
  read -r -p "  $label [${default}]: " answer </dev/tty
  printf '%s' "${answer:-$default}"
}

# Alfred ships with no model assignment. A silently chosen model is a cost and quality
# decision made without the user, so setup asks and a run without a profile stops.
setup_profile() {
  local target="$ALFRED_HOME/profile.json"

  if [ -f "$target" ] && [ "${1:-}" != "--force" ]; then
    info "profile already set, keeping it (use: $0 models to change it)"
    return
  fi

  info ""
  info "Model assignment. Use the identifier your agent expects, for example"
  info "anthropic/claude-opus-5 or ollama/qwen3.8-exec."
  info ""

  local orchestrator
  orchestrator="$(ask_model "orchestrator" "anthropic/claude-opus-5")"

  # The worktree coordinator routes messages between the user and one session per change.
  # It plans nothing and reads no document, so it is the cheapest reliable model rather
  # than the orchestrator's - asked rather than assumed, like every other assignment here.
  info ""
  info "  The worktree coordinator only relays between you and one session per change."
  info "  It decides nothing, so it runs smaller than the orchestrator."
  info ""
  local coordinator
  coordinator="$(ask_model "worktree coordinator" "anthropic/claude-haiku-4-5-20251001")"

  local uniform
  read -r -p "  use the same model for every phase? [Y/n]: " uniform </dev/tty
  uniform="${uniform:-y}"

  local phase_models=()
  if [[ "$uniform" =~ ^[Yy] ]]; then
    local all
    all="$(ask_model "all phases" "$orchestrator")"
    for phase in "${PHASES[@]}"; do
      phase_models+=("\"$phase\": \"$all\"")
    done
  else
    info ""
    info "  Judgement phases (refine, spec, design, diagnose, review) want the strongest"
    info "  model available. Execution phases (apply, verify, tasks) can run smaller."
    info ""
    for phase in "${PHASES[@]}"; do
      phase_models+=("\"$phase\": \"$(ask_model "$phase" "$orchestrator")\"")
    done
  fi

  local joined
  joined="$(IFS=,; printf '%s' "${phase_models[*]}")"
  printf '{"orchestrator": "%s", "coordinator": "%s", "phases": {%s}, "memory_tool_prefix": "mcp__engram__", "effort": {}, "extra_tools": {}}\n' \
    "$orchestrator" "$coordinator" "$joined" > "$target"

  setup_runtime
  helper_json_valid "$target" || die "could not write a valid profile"

  info ""
  info "profile written to $target"
  info ""
  info "Optional, by editing that file:"
  info "  effort       per phase, for example {\"spec\": \"high\", \"archive\": \"low\"}"
  info "  extra_tools  per phase, for tools outside the base set"
  info "               for example {\"refine\": [\"mcp__atlassian__getJiraIssue\"]}"
}

generate_agents() {
  setup_runtime
  local targets=() agent

  while read -r agent; do
    [ -n "$agent" ] || continue
    case "$agent" in
      opencode) targets+=("opencode=$HOME/.config/opencode/opencode.json") ;;
      claude)   targets+=("claude=$HOME/.claude") ;;
    esac
  done < <(detect_agents)

  if [ ${#targets[@]} -eq 0 ]; then
    warn "no supported agent found; skills are installed but no orchestrator was registered"
    return
  fi

  helper_agents "$ALFRED_HOME/profile.json" "$ALFRED_HOME/skills" "${targets[@]}"
}

record_state() {
  setup_runtime
  local payload; payload="$(IFS=,; printf '%s' "${PAYLOAD[*]}")"
  helper_state write "$ALFRED_HOME" "$VERSION" "$payload"
}

cmd_install() {
  info "Alfred $VERSION"
  install_payload
  setup_profile
  generate_agents
  record_state
  info ""
  info "Done. Open your agent, select the Alfred orchestrator, and run init in a repository."
}

# Never overwrite a file the user changed. That is the whole point of recording the hashes.
cmd_update() {
  setup_runtime
  [ -d "$ALFRED_HOME" ] || die "not installed; run: $0 install"

  local report
  local payload; payload="$(IFS=,; printf '%s' "${PAYLOAD[*]}")"
  report="$(helper_state compare "$ALFRED_HOME" "$SOURCE" "$payload")"

  local modified
  modified="$(printf '%s' "$report" | helper_json_get - modified)"

  if [ -n "$modified" ]; then
    warn "these files were modified locally and will not be touched:"
    printf '  %s\n' $modified >&2
    warn "compare them against $SOURCE and merge by hand if you want the new version"
  fi

  printf '%s' "$report" | helper_state_apply "$ALFRED_HOME" "$SOURCE"

  chmod +x "$ALFRED_HOME"/bin/*.sh
  generate_agents
  record_state
}

# `/alfred-worktree` starts a session per change by running the agent's CLI, and an agent
# does not run a command it has not been permitted. Checked here because the alternative is
# discovering it mid-run, with the worktrees already open and nothing able to work in them.
#
# This reads the settings rather than trying the command: starting a session to find out
# would cost one, and a permission that is written down is the thing being asked about. A
# permissive mode can let the command through with no rule present, so a failure here means
# "this may stop you", not "this will" - which is the safe direction for a check whose
# remedy is one line and harmless.
session_permission_granted() {
  if [ -n "$HELPER" ]; then "$HELPER" session-permission "$HOME"
  else python3 - "$HOME" <<'PY'
import json, sys
from pathlib import Path

home = Path(sys.argv[1])
for f in (home / ".claude/settings.json", home / ".claude/settings.local.json",
          Path(".claude/settings.json"), Path(".claude/settings.local.json")):
    try:
        rules = json.loads(f.read_text()).get("permissions", {}).get("allow") or []
    except Exception:
        continue
    if any(isinstance(r, str) and r.startswith("Bash(claude") for r in rules):
        sys.exit(0)
sys.exit(1)
PY
  fi
}

cmd_doctor() {
  setup_runtime
  local failures=0

  check() {
    if eval "$2" >/dev/null 2>&1; then
      info "  ok    $1"
    else
      info "  FAIL  $1"
      info "        $3"
      failures=$((failures + 1))
    fi
  }

  info "Alfred doctor"
  check "a helper runtime"       "[ -n \"$HELPER\" ] || command -v python3" \
                                 "install go (preferred) or python3"
  check "installed"              "[ -d '$ALFRED_HOME/skills' ]" "run: $0 install"
  check "model profile set"      "[ -s '$ALFRED_HOME/profile.json' ]" "run: $0 models"
  check "state recorded"         "[ -f '$ALFRED_HOME/state.json' ]"   "run: $0 update"
  check "an agent is registered" "[ -f '$HOME/.config/opencode/opencode.json' ] || [ -f '$HOME/.claude/commands/alfred.md' ]" \
                                 "install opencode or claude code, then run: $0 install"

  local phase missing=0
  for phase in "${PHASES[@]}"; do
    [ -f "$ALFRED_HOME/skills/$phase/SKILL.md" ] || { missing=$((missing + 1)); }
  done
  check "all ${#PHASES[@]} skills resolvable" "[ $missing -eq 0 ]" "reinstall: $0 install"
  check "worktree script installed"  "[ -x '$ALFRED_HOME/bin/worktree.sh' ]" "run: $0 update"
  check "sessions may be started"    "session_permission_granted" \
                                     "allow the start command once: /permissions in Claude Code, or add \"Bash(claude --bg:*)\" to permissions.allow in ~/.claude/settings.json"

  info ""
  [ $failures -eq 0 ] && info "no problems found" || info "$failures problem(s) found"
  return $failures
}

cmd_status() {
  [ -d "$ALFRED_HOME" ] || die "not installed; run: $0 install"
  setup_runtime
  info "home      $ALFRED_HOME"
  info "version   $(helper_json_get "$ALFRED_HOME/state.json" version 2>/dev/null || echo unknown)"
  info "skills    $(find "$ALFRED_HOME/skills" -name SKILL.md 2>/dev/null | wc -l | tr -d ' ')"
  info "profile   $(helper_json_get "$ALFRED_HOME/profile.json" orchestrator 2>/dev/null || echo 'not set')"
  info "agents    $(detect_agents | tr '\n' ' ')"
}

case "${1:-install}" in
  install) cmd_install ;;
  update)  cmd_update ;;
  models)  setup_profile --force; generate_agents ;;
  doctor)  cmd_doctor ;;
  status)  cmd_status ;;
  *)       die "usage: $0 [install|update|models|doctor|status]" ;;
esac
