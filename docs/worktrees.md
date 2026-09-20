# Several changes at once

Alfred can work on several changes in the same repository at the same time. Each one gets
its own git worktree and its own branch, so the pipelines never write the same files or
commit to the same `HEAD`.

```
one change, one worktree, one branch
```

## Starting

In Claude Code, `/alfred-worktree` takes the list of changes, one per line:

```
/alfred-worktree
feature/login-google from main: add google sign-in
fix/payments-timeout: bug: payments time out after 30 seconds
```

Each line is `branch [from base]: request`. The branch name is required. Without `from`,
the branch starts from the branch the repository is currently on.

In OpenCode it is the `alfred-worktree` agent, picked from the agent list, and the same
list of changes is the first message to it.

For each line, Alfred creates the worktree, proposes a route for the request the way it
always does, and runs the pipeline inside that worktree. Questions and reports carry the
change name, so you can tell which change is asking.

`git.worktrees.max_parallel` caps how many changes run at once; the rest wait.

Each change runs in its own agent session, started in the background with its working
directory set to that change's worktree. The session you typed into becomes the
coordinator: it opens the worktrees, starts a session per change, and from then on carries
questions up to you and your answers back down. You keep one place to look, and every
question tells you which change is asking. To pick up where you left off:

```
/alfred-worktree continue
```

A single change in the current checkout is still `/alfred`, or the `alfred` agent.

## Before the first run

There is one thing to set up, once per machine, and nothing at all per repository.

Starting a session is a command, and your agent will not run a command you have not
permitted. Permit it once in your own agent settings. In Claude Code that is
`/permissions`, or an entry in `~/.claude/settings.json`:

```json
{ "permissions": { "allow": ["Bash(claude --bg:*)"] } }
```

Without it the coordinator stops at the first change and tells you which command it needs,
rather than starting anything. It cannot grant this itself: an agent that widens its own
permissions has thrown away the decision the permission was there to record.

The sessions need nothing else granted. Each one inherits the permissions you already have,
which is why `sessions.allowed_tools` is left unset — it narrows a session rather than
enabling it, and a list that forgets a tool produces a change that fails on it later.

## Where the worktrees are

```
~/.alfred/worktrees/<repository>/<branch>/
```

Outside the repository, grouped by repository name, with slashes in the branch name turned
into dashes. `git.worktrees.root` changes the template; `{repo}`, `{branch}` and `{change}`
are replaced.

The repository you ran `init` in is the main checkout. It is where the configuration lives
and where `status` reports from. Nothing is worked on there while worktrees are open.

## Preparing a worktree

A fresh worktree has only the tracked files. Two configuration values make it usable:

```yaml
git:
  worktrees:
    setup_command: npm ci
    copy_files: [.env, config/local.yml]
```

`copy_files` are copied from the main checkout. `setup_command` runs inside the new
worktree. With the default, `none`, nothing is installed, and Alfred says so when it opens
the worktree rather than when `verify` cannot run the tests.

## While changes run

`status` lists every open change across all worktrees, with its branch, phase and state.

The memory backend is shared, and that is intended: entries are keyed by change name, and
a postmortem written from one worktree is found from the next.

## Finishing

When a change is archived, Alfred commits on its branch, pushes it, and removes the
worktree. The branch is deleted only if it is already merged into its base; otherwise it
stays for the pull request.

A pull request is opened only with `git.worktrees.pull_request: true`, and only if the
tool for it is installed and logged in. It is off by default.

To drop a change instead, ask for `abandon <branch>`. Alfred shows what would be lost and
asks before removing the worktree. The branch is kept.

## The tool

Everything above that touches git worktrees is done by one installed script, so it
happens the same way every time and never depends on an agent typing git commands:

```
~/.config/alfred/bin/worktree.sh open    --branch <name> [--base <ref>] [--change <slug>]
~/.config/alfred/bin/worktree.sh close   --branch <name> [--force]
~/.config/alfred/bin/worktree.sh abandon --branch <name> --yes
~/.config/alfred/bin/worktree.sh list
~/.config/alfred/bin/worktree.sh path    --branch <name>
```

All of them take `--cwd <dir>`, any directory inside the repository or a worktree. You can
run them yourself; Alfred runs them for you.

## Configuration

```yaml
git:
  worktrees:
    tool: ~/.config/alfred/bin/worktree.sh
    root: ~/.alfred/worktrees/{repo}/{branch}
    setup_command: none
    copy_files: []
    max_parallel: 3
    remove_on_archive: true
    pull_request: false
```

## Not yet

A change across several repositories still runs at workspace level with one checkout per
repository. Running each task of `apply` in its own worktree, which would let tasks touching
the same file run together, needs a merge step the pipeline does not have yet.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `the main checkout is not on a branch; pass --base explicitly` | the repository is on a detached `HEAD`; name the base with `from` |
| `<path> exists and is not a worktree of this repository` | something else occupies the target directory; move it or change `git.worktrees.root` |
| `has uncommitted changes to tracked files` on close | a file was changed and not committed by `archive`; look at what it lists before forcing |
| `setup: failed` when opening | the setup command failed; the log path is printed, the worktree is kept |
| `exists: false` in the list | the worktree directory was deleted by hand; ask for `abandon <branch>` to clean the record |
