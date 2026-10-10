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
VERSION="0.4.0"

PHASES=(init explore refine research spec diagnose design tasks apply verify review archive)
PAYLOAD=(skills memory notify tracker templates defaults triggers workflows bin alfred.config.yaml)

# The workflow /alfred runs when the profile does not say. Only sdd ships, so there is
# nothing to choose at installation; a profile without the key is read as sdd too, which is
# what reproduces today's /alfred on every installation that predates workflows.
DEFAULT_WORKFLOW="sdd"

info() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

# The helper that does the installer's bookkeeping and writes the agent definitions.
# Bash has neither JSON nor sha256, so something has to; it is Go, so an installation needs
# no interpreter and hashing the payload uses every core.
#
# Built from source on every run rather than shipped as a binary: a binary would have to be
# built per platform and trusted, where the source is already here and `go build` is one
# command. Nothing else in the payload needs a toolchain, so it stays out of PAYLOAD and
# is built into .build/. The result is then installed, by install_helper, because the
# registration tool has to reach it on a machine where the clone is gone.
HELPER=""
BUILD_ERROR=""
WORKFLOW_ARGS=()

# setup_runtime builds the helper from the current source, once per run.
# It always rebuilds when go is available: reusing an existing .build/alfred would install
# a helper compiled from older source after a pull. go build is incremental, so an unchanged
# tree costs almost nothing. Without go, an existing build is the only helper there is.
setup_runtime() {
  [ -n "$HELPER" ] && return 0

  if ! command -v go >/dev/null 2>&1; then
    if [ -x "$SOURCE/.build/alfred" ]; then
      warn "go not found; using the existing $SOURCE/.build/alfred, which may predate this source"
      HELPER="$SOURCE/.build/alfred"
      return 0
    fi
    BUILD_ERROR="go is required; the installer builds its helper from cmd/alfred"
    return 1
  fi

  # The compiler's own output is kept: "the build failed" names nothing a user can act on,
  # and the installation stops here, so there is no later message to carry the reason.
  if ! BUILD_ERROR="$( (cd "$SOURCE" && go build -o .build/alfred ./cmd/alfred) 2>&1 )"; then
    BUILD_ERROR="could not build the helper from cmd/alfred:
$BUILD_ERROR"
    return 1
  fi

  BUILD_ERROR=""
  HELPER="$SOURCE/.build/alfred"
}

# Every command but doctor stops without a helper: it is the thing that does the work.
# doctor reports instead, because reporting is what doctor is for.
require_runtime() {
  setup_runtime || die "$BUILD_ERROR"
}

# Agents are detected rather than asked about: an agent that is not installed has nowhere
# to register an orchestrator.
detect_agents() {
  local found=()
  command -v opencode >/dev/null 2>&1 && found+=(opencode)
  command -v claude   >/dev/null 2>&1 && found+=(claude)
  # ${found[*]:-} rather than ${found[*]}: set -u treats an empty array as unset, and a
  # machine with ~/.claude and no binary on PATH reaches here with nothing found yet.
  [ -d "$HOME/.claude" ] && [[ ! " ${found[*]:-} " =~ " claude " ]] && found+=(claude)
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

# The one thing installed that the payload does not carry. bin/register.sh runs on machines
# with no clone of Alfred, and it shells to this helper, so the helper has to be where the
# installation is rather than in the .build/ of a directory the user may have deleted.
#
# It is the binary built from the source in this run. Nothing is fetched: a downloaded
# binary would have to be built per platform and trusted, and the source is already here.
install_helper() {
  require_runtime
  mkdir -p "$ALFRED_HOME/bin"

  # Written beside the target and moved into place, because the copy being replaced may be
  # the one currently running this registration.
  local staged="$ALFRED_HOME/bin/.alfred.incoming"
  cp "$HELPER" "$staged"
  chmod +x "$staged"
  mv -f "$staged" "$ALFRED_HOME/bin/alfred"
}

# Engram is installed with Go, which the installer already requires for its helper, so the
# memory backend adds no package manager. The version is pinned: the memory tools the agents
# declare are Engram v3's, and an older server exposes fewer of them, which reads from inside
# a phase as a backend that cannot do the thing.
ENGRAM_MODULE="github.com/Gentleman-Programming/engram/v3/cmd/engram"
ENGRAM_VERSION="v3.0.0"
ENGRAM_MAJOR=3

# go install writes to GOBIN, or GOPATH/bin, which a shell often does not have on PATH. It
# goes first for this run, so the copy just installed is the one found and registered; the
# registration records the absolute path, so the agents do not depend on PATH either.
USER_PATH="$PATH"
GO_BIN=""
if command -v go >/dev/null 2>&1; then
  GO_BIN="$(go env GOBIN 2>/dev/null)"
  [ -n "$GO_BIN" ] || GO_BIN="$(go env GOPATH 2>/dev/null)/bin"
  PATH="$GO_BIN:$PATH"
fi

engram_on_path() { command -v engram >/dev/null 2>&1; }

# "engram 3.0.0" -> "3.0.0"; empty when it is absent or does not say.
engram_version() {
  engram_on_path || return 0
  engram version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -n 1
}

engram_supported() {
  local v; v="$(engram_version)"
  [ -n "$v" ] && [ "${v%%.*}" -ge "$ENGRAM_MAJOR" ]
}

# Asked rather than done: it installs a program on the machine, outside Alfred's directory.
offer_engram() {
  local current answer
  current="$(engram_version)"
  info ""
  if [ -n "$current" ]; then
    info "  engram $current is installed; Alfred needs v$ENGRAM_MAJOR."
  else
    info "  engram is not installed."
  fi
  read -r -p "  install engram $ENGRAM_VERSION with go install? [Y/n]: " answer </dev/tty
  if [[ ! "${answer:-y}" =~ ^[Yy] ]]; then
    warn "memory is enabled without a usable engram; install it, then run: $0 update"
    return 0
  fi
  install_engram
}

install_engram() {
  command -v go >/dev/null 2>&1 || { warn "go is required to install engram"; return 0; }
  info "installing engram $ENGRAM_VERSION"
  if ! go install "$ENGRAM_MODULE@$ENGRAM_VERSION"; then
    warn "could not install engram; run: go install $ENGRAM_MODULE@$ENGRAM_VERSION"
    return 0
  fi
  # The shell remembers where it last found engram; the copy just installed is elsewhere.
  hash -r

  # Another engram earlier on the user's own PATH keeps answering in their shell. Alfred
  # registers the one it installed, by absolute path, and says which one that is.
  local shell_engram
  shell_engram="$(PATH="$USER_PATH"; command -v engram 2>/dev/null || true)"
  info "engram $(engram_version) installed at $GO_BIN/engram"
  if [ -n "$shell_engram" ] && [ "$shell_engram" != "$GO_BIN/engram" ]; then
    warn "your shell finds another engram first: $shell_engram; Alfred uses $GO_BIN/engram"
  elif [ -z "$shell_engram" ]; then
    info "add $GO_BIN to PATH to use the engram command yourself"
  fi
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

  # Carried over rather than asked for. Which workflow /alfred runs is a choice there is
  # nothing to make at installation, because only sdd ships; once the user has edited it,
  # re-running this to change a model must not quietly put it back.
  require_runtime
  local default_workflow=""
  [ -f "$target" ] && default_workflow="$("$HELPER" json-get "$target" default_workflow 2>/dev/null || true)"
  [ -n "$default_workflow" ] || default_workflow="$DEFAULT_WORKFLOW"

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

  # Memory is a choice, not a default: every memory tool is schema each agent carries
  # before it reads a line, which is worth paying only when a backend is there to answer.
  info ""
  info "  Memory. Engram is a local memory server, a single binary over SQLite, that lets"
  info "  the phases recall past decisions, specifications and postmortems across changes."
  info "  Alfred works without it, reading the documents directly at a higher token cost."
  info "  With it, every agent is given Engram's memory tools and the server is registered."
  info ""
  # The default follows what is installed: yes when engram is on PATH, no otherwise.
  local hint="y/N" use_memory="n" answer prefix=""
  engram_on_path && { hint="Y/n"; use_memory="y"; }
  read -r -p "  use Engram for memory? [$hint]: " answer </dev/tty
  use_memory="${answer:-$use_memory}"
  if [[ "$use_memory" =~ ^[Yy] ]]; then
    prefix="mcp__engram__"
    engram_supported || offer_engram
  fi

  local joined
  joined="$(IFS=,; printf '%s' "${phase_models[*]}")"
  printf '{"orchestrator": "%s", "coordinator": "%s", "phases": {%s}, "memory_tool_prefix": "%s", "default_workflow": "%s", "effort": {}, "extra_tools": {}}\n' \
    "$orchestrator" "$coordinator" "$joined" "$prefix" "$default_workflow" > "$target"

  "$HELPER" json-valid "$target" || die "could not write a valid profile"

  info ""
  info "profile written to $target"
  info ""
  info "Optional, by editing that file:"
  info "  effort            per phase, for example {\"spec\": \"high\", \"archive\": \"low\"}"
  info "  extra_tools       per phase, for tools outside the base set"
  info "                    for example {\"refine\": [\"mcp__atlassian__getJiraIssue\"]}"
  info "  default_workflow  the workflow /alfred runs; currently $default_workflow"
  info "                    then run: $0 workflows"
}

# Where each detected agent keeps what the installer writes. Both the orchestrator
# definitions and the memory server registration go to the same places, so they are built
# once: an agent that gets one and not the other is the half-configured state that reads,
# from inside a phase, as a backend that cannot do the thing.
agent_targets() {
  local agent
  while read -r agent; do
    [ -n "$agent" ] || continue
    case "$agent" in
      opencode) printf '%s\n' "opencode=$HOME/.config/opencode/opencode.json" ;;
      claude)   printf '%s\n' "claude=$HOME" ;;
    esac
  done < <(detect_agents)
}

# The roots and write targets of a machine-scope registration, one per line, in the order
# the helper takes them. Machine scope only: a machine-level operation that registered a
# repository's own workflows would have to go looking for repositories first.
#
# The custom root is passed although the installer never creates it. An absent root means
# no custom workflows, which is how an update leaves them alone and still reports one that
# stopped validating.
workflow_args() {
  local targets=() t
  while read -r t; do [ -n "$t" ] && targets+=("$t"); done < <(agent_targets)
  [ ${#targets[@]} -eq 0 ] && return 1

  # The helper takes the Claude home itself, not the parent.
  local resolved=() x
  for x in "${targets[@]}"; do
    case "$x" in claude=*) resolved+=("claude=${x#claude=}/.claude") ;; *) resolved+=("$x") ;; esac
  done

  printf '%s\n' \
    "$ALFRED_HOME/profile.json" \
    "$ALFRED_HOME/skills" \
    "$ALFRED_HOME/workflows" \
    "$ALFRED_HOME/custom/workflows" \
    "$ALFRED_HOME/templates/workflow" \
    "manifest=$ALFRED_HOME/generated.json" \
    "${resolved[@]}"
}

read_workflow_args() {
  local a
  WORKFLOW_ARGS=()
  while read -r a; do [ -n "$a" ] && WORKFLOW_ARGS+=("$a"); done < <(workflow_args)
  [ ${#WORKFLOW_ARGS[@]} -gt 0 ]
}

register_workflows() {
  require_runtime

  if ! read_workflow_args; then
    warn "no supported agent found; skills are installed but no command was registered"
    return 0
  fi

  # A rejected workflow exits 1 with every other workflow registered, and that is reported
  # rather than fatal: an update must not stop because a workflow the user wrote stopped
  # validating. Being called wrongly is a different thing and does stop the run.
  local status=0
  "$HELPER" workflows machine apply "${WORKFLOW_ARGS[@]}" || status=$?
  case "$status" in
    0) ;;
    1) warn "some workflows were not registered; the report above says which, and why" ;;
    *) die "registration was called wrongly" ;;
  esac
}

# Whether the profile gives the agents memory tools. A profile without the key predates the
# question and has none, which is what the helper reads it as too.
memory_enabled() {
  [ -n "$("$HELPER" json-get "$ALFRED_HOME/profile.json" memory_tool_prefix 2>/dev/null)" ]
}

# The agents declare every memory tool; this makes the server expose them. Both halves are
# written by the installer because a registration pasted from a document is a step someone
# skips, and the failure it causes is invisible: the phase sees no tool, which is exactly
# what a backend that cannot do the thing looks like.
register_memory() {
  require_runtime
  memory_enabled || return 0
  local targets=() t
  while read -r t; do [ -n "$t" ] && targets+=("$t"); done < <(agent_targets)
  [ ${#targets[@]} -eq 0 ] && return 0

  "$HELPER" memory apply "$ALFRED_HOME" "${targets[@]}" || true
}

record_state() {
  require_runtime
  local payload; payload="$(IFS=,; printf '%s' "${PAYLOAD[*]}")"
  "$HELPER" state write "$ALFRED_HOME" "$VERSION" "$payload"
}

cmd_install() {
  # Before anything is copied: a run that installs the payload and then discovers it
  # cannot generate the agents leaves a half-installed directory behind.
  require_runtime

  info "Alfred $VERSION"
  install_payload
  install_helper
  setup_profile
  register_workflows
  register_memory
  record_state
  info ""
  info "Done. Open your agent, select the Alfred orchestrator, and run init in a repository."
}

# Never overwrite a file the user changed. That is the whole point of recording the hashes.
cmd_update() {
  require_runtime
  [ -d "$ALFRED_HOME" ] || die "not installed; run: $0 install"

  local report
  local payload; payload="$(IFS=,; printf '%s' "${PAYLOAD[*]}")"
  report="$("$HELPER" state compare "$ALFRED_HOME" "$SOURCE" "$payload")"

  local modified
  modified="$(printf '%s' "$report" | "$HELPER" json-get - modified)"

  if [ -n "$modified" ]; then
    warn "these files were modified locally and will not be touched:"
    printf '  %s\n' $modified >&2
    warn "compare them against $SOURCE and merge by hand if you want the new version"
  fi

  printf '%s' "$report" | "$HELPER" state apply "$ALFRED_HOME" "$SOURCE"

  chmod +x "$ALFRED_HOME"/bin/*.sh
  install_helper
  register_workflows
  register_memory
  record_state
}

cmd_workflows() {
  [ -d "$ALFRED_HOME" ] || die "not installed; run: $0 install"
  register_workflows
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
  "$HELPER" session-permission "$HOME"
}

# The agents' half of the memory wiring. The helper is the authority on the list, so it is
# asked rather than copied: a check carrying its own copy is one more thing to fall out of
# step, which is the failure being checked for.
agents_declare_every_memory_tool() {
  "$HELPER" memory declared "$HOME/.claude/agents"
}

# Both halves of workflow registration, for the same reason the memory wiring has two
# checks: a workflow installed with no command cannot be reached, a command whose workflow
# is gone dispatches phases that resolve nowhere, and neither is visible from inside a run.
#
# The first half has an exit code of its own, which is what `check` is for. The second has
# none, so the report is read: a command the next run would remove is a command nothing
# claims any more.
every_workflow_has_a_command() {
  read_workflow_args || return 1
  "$HELPER" workflows machine check "${WORKFLOW_ARGS[@]}"
}

no_command_outlives_its_workflow() {
  read_workflow_args || return 1
  local report
  report="$("$HELPER" workflows machine report "${WORKFLOW_ARGS[@]}" || true)"
  ! printf '%s\n' "$report" | grep -q '^  removed  '
}

cmd_doctor() {
  setup_runtime || true
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
  check "installer helper built" "[ -n '$HELPER' ]" "install go, then run: $0 install"
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
  check "helper installed"           "[ -x '$ALFRED_HOME/bin/alfred' ]" "run: $0 update"

  # Both halves of workflow registration, skipped rather than failed when there is nothing
  # to ask or nowhere to register: the two checks above already name those, and asking again
  # here reports one fault as three.
  if [ -z "$HELPER" ]; then
    info "  skip  workflow registration (no helper to ask)"
  elif ! read_workflow_args; then
    info "  skip  workflow registration (no agent detected)"
  else
    check "every workflow has a command" "every_workflow_has_a_command" \
          "run: $0 workflows, which registers them and names any it rejects"
    check "no command outlives its workflow" "no_command_outlives_its_workflow" \
          "run: $0 workflows, which removes the commands of workflows that are gone"
  fi

  # Both halves of the memory wiring, checked separately because they fail separately and
  # only one of them is visible from inside a run.
  if memory_enabled; then
    check "engram v$ENGRAM_MAJOR or newer" "engram_supported" \
          "run: go install $ENGRAM_MODULE@$ENGRAM_VERSION, then: $0 update"
    local mt=() m
    while read -r m; do [ -n "$m" ] && mt+=("$m"); done < <(agent_targets)
    check "memory server exposes every tool" \
          "'$HELPER' memory check '$ALFRED_HOME' ${mt[*]}" \
          "run: $0 update, which rewrites the registration with --tools=all"
    check "agents declare every memory tool" "agents_declare_every_memory_tool" \
          "run: $0 update, which regenerates the agent definitions"
  else
    info "  skip  memory not enabled (run: $0 models to enable it)"
  fi
  check "sessions may be started"    "session_permission_granted" \
                                     "allow the start command once: /permissions in Claude Code, or add \"Bash(claude --bg:*)\" to permissions.allow in ~/.claude/settings.json"

  info ""
  [ $failures -eq 0 ] && info "no problems found" || info "$failures problem(s) found"
  return $failures
}

cmd_status() {
  [ -d "$ALFRED_HOME" ] || die "not installed; run: $0 install"
  require_runtime
  info "home      $ALFRED_HOME"
  info "version   $("$HELPER" json-get "$ALFRED_HOME/state.json" version 2>/dev/null || echo unknown)"
  info "skills    $(find "$ALFRED_HOME/skills" -name SKILL.md 2>/dev/null | wc -l | tr -d ' ')"
  info "profile   $("$HELPER" json-get "$ALFRED_HOME/profile.json" orchestrator 2>/dev/null || echo 'not set')"
  info "agents    $(detect_agents | tr '\n' ' ')"
}

case "${1:-install}" in
  install) cmd_install ;;
  update)  cmd_update ;;
  models)  setup_profile --force; register_workflows; register_memory ;;
  workflows) cmd_workflows ;;
  doctor)  cmd_doctor ;;
  status)  cmd_status ;;
  *)       die "usage: $0 [install|update|models|workflows|doctor|status]" ;;
esac
