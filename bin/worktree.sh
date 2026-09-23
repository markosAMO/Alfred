#!/usr/bin/env bash
#
# Alfred worktrees.
#
# One change, one worktree, one branch. This script does the deterministic part: creating
# the worktree from the right base, preparing it, and removing it when the change is
# closed. No agent runs git worktree commands by hand; the orchestrator calls this and
# records what it prints.
#
#   worktree.sh open    --branch <name> [--base <ref>] [--change <slug>]
#   worktree.sh close   --branch <name> [--force]
#   worktree.sh abandon --branch <name> --yes
#   worktree.sh list
#   worktree.sh path    --branch <name>
#
# Every mode accepts --cwd <dir>, any directory inside the repository or one of its
# worktrees. Output is YAML, one document per worktree, so the caller records fields
# rather than parsing prose.

set -euo pipefail

ALFRED_HOME="${ALFRED_HOME:-$HOME/.config/alfred}"
DEFAULT_ROOT='~/.alfred/worktrees/{repo}/{branch}'

mode=""
cwd="$PWD"
branch=""
base=""
change=""
force=false
yes=false

usage() {
  cat <<'EOF'
usage: worktree.sh <open|close|abandon|list|path> [options]

  open     create the worktree and branch for a change, prepare it, print where it is
  close    remove a worktree whose change was archived; delete the branch only if merged
  abandon  remove a worktree without archiving; the branch is kept
  list     every worktree of this repository with its change, phase and status
  path     print the worktree path for a branch

  --branch <name>   the branch, required except for list
  --base <ref>      open only: where the branch starts (default: the current branch)
  --change <slug>   open only: the change this worktree belongs to
  --cwd <dir>       a directory inside the repository (default: the current directory)
  --force           close only: remove even with uncommitted changes
  --yes             abandon only: required, since it discards uncommitted work

exit codes: 0 ok, 1 not found, 2 error, 3 not an Alfred repository, 4 confirmation required
EOF
}

fail() { printf 'worktree: %s\n' "$1" >&2; exit "${2:-2}"; }
warn() { printf 'worktree: %s\n' "$1" >&2; }

parse_args() {
  [ $# -gt 0 ] || { usage >&2; exit 2; }
  mode="$1"; shift
  case "$mode" in
    open|close|abandon|list|path) ;;
    -h|--help|help) usage; exit 0 ;;
    *) usage >&2; fail "unknown mode: $mode" ;;
  esac
  while [ $# -gt 0 ]; do
    case "$1" in
      --branch) [ $# -ge 2 ] || fail "--branch requires a value"; branch="$2"; shift 2 ;;
      --base)   [ $# -ge 2 ] || fail "--base requires a value";   base="$2";   shift 2 ;;
      --change) [ $# -ge 2 ] || fail "--change requires a value"; change="$2"; shift 2 ;;
      --cwd)    [ $# -ge 2 ] || fail "--cwd requires a value";    cwd="$2";    shift 2 ;;
      --force)  force=true; shift ;;
      --yes)    yes=true; shift ;;
      *) usage >&2; fail "unknown argument: $1" ;;
    esac
  done
  case "$mode" in
    open|close|abandon|path) [ -n "$branch" ] || fail "--branch is required for $mode" ;;
  esac
  case "$branch" in
    */|*..*|-*) fail "invalid branch name: $branch" ;;
  esac
}

# The main checkout is where .alfred/ and the configuration live, whichever worktree the
# caller is standing in.
resolve_repo() {
  cwd="$(cd "$cwd" 2>/dev/null && pwd -P)" || fail "no such directory: $cwd"
  git -C "$cwd" rev-parse --git-dir >/dev/null 2>&1 || fail "$cwd is not inside a git repository" 3
  local common; common="$(git -C "$cwd" rev-parse --path-format=absolute --git-common-dir)"
  main="$(dirname "$common")"
  [ -d "$main/.alfred" ] || fail "$main is not an Alfred repository (no .alfred/); run init first" 3
  repo_name="$(basename "$main")"
}

# One scalar from the git.worktrees block of .alfred/config.yaml. Only that block is read,
# so its key names may repeat names used elsewhere in the file (max_parallel does).
worktrees_value() {
  local file="$main/.alfred/config.yaml"
  [ -f "$file" ] || return 0
  awk -v key="$1" '
    /^  worktrees:[ \t]*$/ { inblock = 1; next }
    inblock && /^  [^ \t]/  { inblock = 0 }
    inblock && /^[^ \t]/    { inblock = 0 }
    inblock {
      line = $0; sub(/^[ \t]+/, "", line)
      if (index(line, key ":") == 1) { sub("^" key ":[ \t]*", "", line); print line; exit }
    }' "$file" | tr -d '"'"'"
}

# One scalar from a top-level block of .alfred/config.yaml: config_value paths changes.
# Same shape as worktrees_value, one nesting level up.
config_value() {
  local file="$main/.alfred/config.yaml"
  [ -f "$file" ] || return 0
  awk -v block="$1" -v key="$2" '
    $0 ~ "^" block ":[ \t]*$" { inblock = 1; next }
    inblock && /^[^ \t]/ { inblock = 0 }
    inblock {
      line = $0; sub(/^[ \t]+/, "", line)
      if (index(line, key ":") == 1) { sub("^" key ":[ \t]*", "", line); print line; exit }
    }' "$file" | tr -d '"'"'"
}

# Whether the Alfred documents are committed to this repository. Some repositories do not
# accept them — a team that has not adopted Alfred, a fork nobody owns — and Alfred still
# runs there, with its documents kept out of git deliberately. That mode changes what a
# worktree has to be given, and what closing one is allowed to throw away.
artifacts_committed() {
  local value; value="$(config_value artifacts committed)"
  [ "${value:-true}" != false ]
}

# The Alfred documents, in the order they are carried.
artifact_paths() {
  config_value paths architecture
  config_value paths conventions
  config_value paths master_specs
  config_value paths changes
}

expand_home() { printf '%s' "${1/#\~/$HOME}"; }

worktree_root() {
  local template; template="$(worktrees_value root)"
  template="${template:-$DEFAULT_ROOT}"
  local safe_branch="${branch//\//-}"
  template="${template//\{repo\}/$repo_name}"
  template="${template//\{branch\}/$safe_branch}"
  template="${template//\{change\}/${change:-$safe_branch}}"
  expand_home "$template"
}

# path of the worktree checked out on $branch, or nothing
find_worktree() {
  git -C "$main" worktree list --porcelain | awk -v want="refs/heads/$branch" '
    /^worktree / { path = substr($0, 10) }
    /^branch /   { if (substr($0, 8) == want) { print path; exit } }'
}

meta_get() { git -C "$main" config --get "branch.$branch.alfred-$1" 2>/dev/null || true; }
meta_set() { git -C "$main" config "branch.$branch.alfred-$1" "$2"; }
meta_clear() {
  local key
  for key in base change; do git -C "$main" config --unset "branch.$branch.alfred-$key" 2>/dev/null || true; done
}

current_branch() {
  git -C "$main" symbolic-ref --quiet --short HEAD 2>/dev/null \
    || fail "the main checkout is not on a branch; pass --base explicitly"
}

has_origin() { git -C "$main" remote get-url origin >/dev/null 2>&1; }

# copy_files is a YAML flow list: [.env, .env.local]. Untracked files are not in a new
# worktree, and a project that needs them cannot run its tests without them.
copy_files() {
  local list; list="$(worktrees_value copy_files)"
  list="${list#[}"; list="${list%]}"
  local item copied=()
  IFS=',' read -r -a items <<< "$list"
  for item in "${items[@]:-}"; do
    item="$(printf '%s' "$item" | sed -E 's/^[ \t]+|[ \t]+$//g')"
    [ -n "$item" ] || continue
    [ -e "$main/$item" ] || continue
    [ ! -e "$path/$item" ] || continue
    mkdir -p "$(dirname "$path/$item")"
    cp -R "$main/$item" "$path/$item"
    copied+=("$item")
  done
  printf '%s' "${copied[*]:-}"
}

# Under artifacts.committed: false the Alfred documents are deliberately absent from git,
# so a worktree created from a ref does not contain them and never will. Copying them is
# the only way they reach the worktree, and without them every phase rediscovers, one
# subagent at a time, that the repository has no architecture and no conventions.
carry_artifacts() {
  if artifacts_committed; then printf 'tracked'; return 0; fi
  local item carried=()
  while IFS= read -r item; do
    [ -n "$item" ] || continue
    [ -e "$main/$item" ] || continue
    [ ! -e "$path/$item" ] || continue
    mkdir -p "$(dirname "$path/$item")"
    cp -R "$main/$item" "$path/$item"
    carried+=("${item%/}")
  done < <(artifact_paths)
  local list="${carried[*]:-}"
  printf 'copied\ncarried: [%s]' "${list// /, }"
}

# What a phase would otherwise discover for itself, one subagent at a time. The tool knows
# it for free: it is standing in the worktree it just created.
present() { [ -e "$path/$1" ] && printf present || printf missing; }

print_preflight() {
  printf 'architecture: %s\nconventions: %s\nconfig: %s\n' \
    "$(present "$(config_value paths architecture)")" \
    "$(present "$(config_value paths conventions)")" \
    "$(present .alfred/config.yaml)"
}

# Alfred documents in the worktree that the main checkout does not already have, byte for
# byte. Under artifacts.committed: false they are untracked, which means close discards
# them and git has no copy: this is the list that would be lost. Empty once archive has
# copied them back, which is what archive is required to do before asking for a close.
unsynced_documents() {
  artifacts_committed && return 0
  local item file rel
  while IFS= read -r item; do
    item="${item%/}"
    [ -n "$item" ] && [ -d "$path/$item" ] || continue
    while IFS= read -r file; do
      rel="${file#$path/}"
      cmp -s "$file" "$main/$rel" || printf '%s\n' "$rel"
    done < <(find "$path/$item" -type f)
  done < <(artifact_paths)
}

# The log lives in the worktree's own git directory, so it is neither an untracked file in
# the checkout nor something a later commit could sweep in.
setup_log() { printf '%s/alfred-setup.log' "$(git -C "$path" rev-parse --path-format=absolute --git-dir)"; }

run_setup() {
  local command; command="$(worktrees_value setup_command)"
  case "${command:-none}" in
    none|"") printf 'none' ;;
    *)
      local log; log="$(setup_log)"
      if (cd "$path" && bash -c "$command") >"$log" 2>&1; then
        printf 'ok'
      else
        warn "setup command failed; see $log"
        printf 'failed'
      fi ;;
  esac
}

# Tracked modifications and staged changes only. Untracked files are, by design, what a
# commit never sweeps in, and a worktree always has some: copied .env files, the state the
# orchestrator wrote, whatever the setup command produced.
dirty() { [ -d "$path" ] && [ -n "$(git -C "$path" status --porcelain --untracked-files=no)" ]; }
untracked_count() { [ -d "$path" ] && git -C "$path" status --porcelain --untracked-files=all | grep -c '^??' || true; }

print_record() {
  printf 'branch: %s\nbase: %s\npath: %s\nmain_checkout: %s\nchange: %s\nstatus: %s\n' \
    "$branch" "$1" "$path" "$main" "${change:-null}" "$2"
}

cmd_open() {
  path="$(find_worktree)"
  if [ -n "$path" ]; then
    change="${change:-$(meta_get change)}"
    print_record "$(meta_get base)" existing
    printf 'setup: kept\ncopied: []\nartifacts: %s\n' "$(carry_artifacts)"
    print_preflight
    return 0
  fi

  base="${base:-$(current_branch)}"
  path="$(worktree_root)"
  [ ! -e "$path" ] || fail "$path exists and is not a worktree of this repository"
  mkdir -p "$(dirname "$path")"

  if has_origin; then
    git -C "$main" fetch --quiet origin 2>/dev/null || warn "could not fetch origin; using local refs"
  fi

  if git -C "$main" show-ref --verify --quiet "refs/heads/$branch"; then
    git -C "$main" worktree add --quiet "$path" "$branch"
  elif has_origin && git -C "$main" show-ref --verify --quiet "refs/remotes/origin/$branch"; then
    git -C "$main" worktree add --quiet --track -b "$branch" "$path" "origin/$branch"
  else
    git -C "$main" rev-parse --verify --quiet "$base^{commit}" >/dev/null \
      || fail "base ref not found: $base"
    git -C "$main" worktree add --quiet -b "$branch" "$path" "$base"
  fi

  meta_set base "$base"
  [ -z "$change" ] || meta_set change "$change"

  # The state directory is created after the copy, not before: copy_files skips any item
  # already present in the destination, so creating .alfred/state first makes .alfred look
  # present and skips the entry that carries config.yaml and the skill registry. A copied
  # .alfred brings the main checkout's state files with it, and those belong to other
  # changes, so they are cleared rather than inherited.
  local copied setup artifacts
  copied="$(copy_files)"
  artifacts="$(carry_artifacts)"
  mkdir -p "$path/.alfred/state"
  find "$path/.alfred/state" -maxdepth 1 -name '*.yaml' -delete
  setup="$(run_setup)"

  print_record "$base" created
  printf 'setup: %s\ncopied: [%s]\nartifacts: %s\n' "$setup" "${copied// /, }" "$artifacts"
  print_preflight
}

# The state file for this branch inside a worktree, if the orchestrator wrote one.
state_file_for() {
  local dir="$1" file
  for file in "$dir"/.alfred/state/*.yaml; do
    [ -f "$file" ] || continue
    if grep -qE "^branch:[[:space:]]*$branch[[:space:]]*$" "$file"; then printf '%s' "$file"; return 0; fi
  done
  local slug; slug="$(meta_get change)"
  [ -n "$slug" ] && [ -f "$dir/.alfred/state/$slug.yaml" ] && printf '%s' "$dir/.alfred/state/$slug.yaml" || true
}

state_field() { sed -n -E "s/^$2:[[:space:]]*//p" "$1" 2>/dev/null | head -n 1; }

cmd_list() {
  local wt_path wt_branch first=true
  while IFS= read -r line; do
    case "$line" in
      "worktree "*) wt_path="${line#worktree }" ;;
      "branch refs/heads/"*)
        wt_branch="${line#branch refs/heads/}"
        [ "$wt_path" != "$main" ] || continue
        branch="$wt_branch"
        local state phase status slug
        state="$(state_file_for "$wt_path")"
        slug="$(meta_get change)"
        if [ -n "$state" ]; then
          slug="${slug:-$(state_field "$state" change)}"
          phase="$(state_field "$state" current_phase)"
          status="$(state_field "$state" status)"
        fi
        $first || printf -- '---\n'
        first=false
        printf 'branch: %s\nbase: %s\npath: %s\nchange: %s\nphase: %s\nstatus: %s\nexists: %s\n' \
          "$wt_branch" "$(meta_get base)" "$wt_path" "${slug:-null}" "${phase:-null}" \
          "${status:-not_started}" "$([ -d "$wt_path" ] && echo true || echo false)" ;;
    esac
  done < <(git -C "$main" worktree list --porcelain)
  $first && printf 'worktrees: []\n' || true
}

cmd_path() {
  path="$(find_worktree)"
  [ -n "$path" ] || fail "no worktree for branch $branch" 1
  printf '%s\n' "$path"
}

# Always --force: git's own check refuses untracked files, and a worktree always has some.
# The guard that matters, no uncommitted changes to tracked files, ran before this.
remove_worktree() {
  cd "$main"
  git worktree remove --force "$path"
  git worktree prune
  local parent; parent="$(dirname "$path")"
  while [ "$parent" != "/" ] && [ "$parent" != "$HOME" ] && rmdir "$parent" 2>/dev/null; do
    parent="$(dirname "$parent")"
  done
}

cmd_close() {
  path="$(find_worktree)"
  [ -n "$path" ] || fail "no worktree for branch $branch" 1
  if dirty && ! $force; then
    git -C "$path" status --short --untracked-files=no >&2
    fail "$path has uncommitted changes to tracked files; commit them, or pass --force to discard them"
  fi
  local unsynced; unsynced="$(unsynced_documents)"
  if [ -n "$unsynced" ] && ! $force; then
    printf '%s\n' "$unsynced" >&2
    fail "$path holds Alfred documents the main checkout does not have, and they are not in git; archive copies them back before closing, or pass --force to discard them"
  fi
  local untracked; untracked="$(untracked_count)"
  base="$(meta_get base)"; base="${base:-$(current_branch)}"
  local merged=false branch_result
  if git -C "$main" rev-parse --verify --quiet "$base^{commit}" >/dev/null \
     && git -C "$main" merge-base --is-ancestor "$branch" "$base" 2>/dev/null; then merged=true; fi
  remove_worktree
  if $merged; then
    git -C "$main" branch --quiet -D "$branch"
    branch_result="deleted (merged into $base)"
  else
    branch_result="kept (not merged into $base)"
  fi
  meta_clear
  printf 'branch: %s\npath: %s\nremoved: true\nbranch_result: %s\nuntracked_discarded: %s\ndocuments_discarded: %s\n' \
    "$branch" "$path" "$branch_result" "${untracked:-0}" "$(printf '%s' "$unsynced" | grep -c . || true)"
}

cmd_abandon() {
  path="$(find_worktree)"
  [ -n "$path" ] || fail "no worktree for branch $branch" 1
  if ! $yes; then
    printf 'branch: %s\npath: %s\nwould_remove: true\nuncommitted_changes: %s\nuntracked_files: %s\nunsynced_documents: %s\nbranch_kept: true\n' \
      "$branch" "$path" "$(dirty && echo true || echo false)" "$(untracked_count)" \
      "$(unsynced_documents | grep -c . || true)"
    fail "abandon discards the worktree; pass --yes after the user confirmed" 4
  fi
  remove_worktree
  meta_clear
  printf 'branch: %s\npath: %s\nremoved: true\nbranch_result: kept\n' "$branch" "$path"
}

main() {
  parse_args "$@"
  resolve_repo
  case "$mode" in
    open)    cmd_open ;;
    close)   cmd_close ;;
    abandon) cmd_abandon ;;
    list)    cmd_list ;;
    path)    cmd_path ;;
  esac
}

main "$@"
