#!/usr/bin/env bash
#
# Alfred workflow registration.
#
# A workflow becomes a command only when something registers it, and that has to work from
# inside an agent, on a machine where the clone Alfred was installed from is long gone.
# This script is that way in. It detects the agents, builds the roots and the write targets
# of one scope, and shells to the helper the installation left at $ALFRED_HOME/bin/alfred.
# It reads no source tree, builds nothing and needs no Go toolchain.
#
#   register.sh                    machine scope: scan both roots, validate, regenerate
#   register.sh --dry-run          the same report, writing nothing
#   register.sh --check            the workflows with no command; non-zero if any
#   register.sh --project <repo>   the same three, for one repository's own workflows
#
# The report and the exit codes are the helper's and are passed through unchanged: 0 when
# every workflow registered and every phase has a model, 1 when something was rejected or
# is missing, 2 when this script was called wrongly.

set -euo pipefail

ALFRED_HOME="${ALFRED_HOME:-$HOME/.config/alfred}"
HELPER="$ALFRED_HOME/bin/alfred"
DEFAULT_EXCLUDE=".git/info/exclude"

mode=""
repo=""

usage() {
  cat <<'EOF'
usage: register.sh [--dry-run | --check] [--project <repo>]

  (no option)       register the machine's workflows and report what changed
  --dry-run         the identical report, writing nothing
  --check           report every workflow of the scope with no registered command
  --project <repo>  the same three against one repository's own workflows

exit codes: 0 everything registered, 1 something was rejected or is missing,
            2 called wrongly
EOF
}

# There is no info here, and that is deliberate: everything a successful run prints is the
# helper's report, and a line of this script's own above it would be a second place to keep
# in step with the first. This script speaks only when it cannot reach the helper, or when
# it was called wrongly.
warn() { printf 'warning: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

# Being called wrongly is exit 2, the same code the helper uses for it, so a caller
# branching on the number does not have to know which of the two answered.
usage_error() { printf 'error: %s\n' "$*" >&2; usage >&2; exit 2; }

set_mode() {
  [ -z "$mode" ] || usage_error "--dry-run and --check are two modes; pass one"
  mode="$1"
}

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --dry-run) set_mode report; shift ;;
      --check)   set_mode check;  shift ;;
      --project)
        [ $# -ge 2 ] || usage_error "--project requires a repository"
        repo="$2"; shift 2 ;;
      -h|--help|help) usage; exit 0 ;;
      *) usage_error "unknown argument: $1" ;;
    esac
  done
  mode="${mode:-apply}"
}

# Every mode needs the helper, including the two that write nothing: the report is its
# output, not this script's. Naming the installation is the whole message — the machine
# that reaches this line has an Alfred that predates the installed helper, or none at all,
# and in both cases the fix is the same command.
require_helper() {
  [ -x "$HELPER" ] && return 0
  die "no helper at $HELPER, so nothing was registered; the installation provides it, built from source in that run: ./install.sh install, or ./install.sh update on a machine that already has Alfred"
}

# Agents are detected rather than asked about, exactly as the installer detects them: an
# agent that is not installed has nowhere to register a command.
detect_agents() {
  local found=()
  command -v opencode >/dev/null 2>&1 && found+=(opencode)
  command -v claude   >/dev/null 2>&1 && found+=(claude)
  # ${found[*]:-} rather than ${found[*]}: set -u treats an empty array as unset, and a
  # machine with ~/.claude and no binary on PATH reaches here with nothing found yet.
  [ -d "$HOME/.claude" ] && [[ ! " ${found[*]:-} " =~ " claude " ]] && found+=(claude)
  printf '%s\n' "${found[@]:-}"
}

# Where the commands and the agents go, for the scope being registered. The agent is the
# machine's either way; what changes is the directory it is written into, which is the
# whole of what scope means here.
agent_targets() {
  local agent
  while read -r agent; do
    [ -n "$agent" ] || continue
    case "$agent" in
      opencode)
        if [ -n "$repo" ]; then
          printf '%s\n' "opencode=$repo/opencode.json"
        else
          printf '%s\n' "opencode=$HOME/.config/opencode/opencode.json"
        fi ;;
      claude)
        if [ -n "$repo" ]; then
          printf '%s\n' "claude=$repo/.claude"
        else
          printf '%s\n' "claude=$HOME/.claude"
        fi ;;
    esac
  done < <(detect_agents)
}

# One scalar from a top-level block of the repository's .alfred/config.yaml. A line scan,
# not a parser: the block is entered on its own header line and left on the first later
# line that starts in column zero. The helper never reads YAML, so the two settings
# registration depends on are read here and passed in as arguments.
config_value() {
  local file="$repo/.alfred/config.yaml"
  [ -f "$file" ] || return 0
  awk -v block="$1" -v key="$2" '
    $0 ~ "^" block ":[ \t]*$" { inblock = 1; next }
    inblock && /^[^ \t]/ { inblock = 0 }
    inblock {
      line = $0; sub(/^[ \t]+/, "", line)
      if (index(line, key ":") == 1) { sub("^" key ":[ \t]*", "", line); print line; exit }
    }' "$file" | tr -d '"'"'"
}

# One of the two exclude forms is always passed, because <repo>/.alfred/generated.json
# holds this machine's absolute paths and its model identifiers and is kept out of git
# whichever way the repository treats Alfred's documents. Under artifacts.committed false
# nothing registration wrote is committed, so every generated path is excluded; under true
# the commands are the repository's and only the manifest is held back.
exclude_target() {
  local committed file
  committed="$(config_value artifacts committed)"
  file="$(config_value artifacts local_exclude)"
  file="${file:-$DEFAULT_EXCLUDE}"
  case "$file" in
    /*) ;;
    *) file="$repo/$file" ;;
  esac
  if [ "${committed:-true}" = false ]; then
    printf 'exclude=%s\n' "$file"
  else
    printf 'exclude-manifest=%s\n' "$file"
  fi
}

resolve_repo() {
  [ -d "$repo" ] || die "no such directory: $repo"
  repo="$(cd "$repo" && pwd -P)"
  [ -d "$repo/.alfred" ] || die "$repo is not an Alfred repository (no .alfred/); run init there first"
}

# The roots of one scope, positionally and in the order the helper takes them, then its
# write targets. The custom root is passed although nothing creates it: an absent root
# means no workflows there, which is how a machine that never wrote one still registers.
machine_args() {
  printf '%s\n' \
    "$ALFRED_HOME/profile.json" \
    "$ALFRED_HOME/skills" \
    "$ALFRED_HOME/workflows" \
    "$ALFRED_HOME/custom/workflows" \
    "$ALFRED_HOME/templates/workflow" \
    "manifest=$ALFRED_HOME/generated.json"
  agent_targets
}

# The machine's two workflow roots are given at project scope as well, and are read and
# never written: they are what tells a repository workflow that a command of its name
# already exists on this machine.
project_args() {
  printf '%s\n' \
    "$ALFRED_HOME/profile.json" \
    "$ALFRED_HOME/skills" \
    "$repo/.alfred/skills" \
    "$repo/.alfred/workflows" \
    "$ALFRED_HOME/workflows" \
    "$ALFRED_HOME/custom/workflows" \
    "manifest=$repo/.alfred/generated.json" \
    "root=$repo"
  exclude_target
  agent_targets
}

main() {
  parse_args "$@"
  require_helper

  local scope=machine
  local args=() a
  if [ -n "$repo" ]; then
    resolve_repo
    scope=project
    while read -r a; do [ -n "$a" ] && args+=("$a"); done < <(project_args)
  else
    while read -r a; do [ -n "$a" ] && args+=("$a"); done < <(machine_args)
  fi

  # The helper's exit code is this script's, and its report is the only report: a second
  # summary here would be a second place to keep in step with the first. Exit 1 gets one
  # line pointing at it, because this runs from inside an agent, where the number arrives
  # without the output and reads the same as a crash.
  local status=0
  "$HELPER" workflows "$scope" "$mode" "${args[@]}" || status=$?
  if [ "$status" -eq 1 ]; then
    warn "not everything is registered; the report above says what, and why"
  fi
  exit "$status"
}

main "$@"
