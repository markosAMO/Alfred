# Worktree protocol

Several changes worked on at the same time never share a checkout. Each runs in its own
git worktree, on its own branch, and the pipeline inside it is the ordinary pipeline.

```
one change, one worktree, one branch
```

Two subagents writing the same working tree leave one silent winner; that is why `apply`
only parallelises tasks with disjoint files. Two *changes* in one working tree have the
same problem with no file list to check, and they also share one `HEAD`: a commit made for
one lands on the branch of the other. A worktree per change removes both.

## Starting

Several changes start together through `/alfred-worktree`, one change per line, each on a
named branch:

```
feature/login-google from main: add google sign-in
fix/payments-timeout: bug: payments time out after 30 seconds
```

One session orchestrates all of them. The orchestrator opens every worktree through the
tool, proposes a route for every request in one message, and then runs the pipelines side
by side: phases of different changes are dispatched at the same time, and inside one
change the ordinary order holds. Every delegation carries the worktree as its first
address. Questions and reports carry the change name, and an answer applies to the change
it names.

`git.worktrees.max_parallel` bounds how many changes run at once. The orchestrator holds
one state file per change for the whole run, and the bound is what keeps its working set
at two pages rather than growing with the list.

`/alfred-worktree continue` resumes every worktree with an open change, from the state file
inside each.

One session rather than one per worktree because the interactive phases need someone to
answer: a run split across headless sessions cannot ask, and a user following three
changes wants one place to look. A change in the current checkout, alone, is still
`/alfred`.

## Layout

```
~/Work/api/                                     main checkout: .alfred/, config, init
~/.alfred/worktrees/api/feature-login-google/   worktree, branch feature/login-google
~/.alfred/worktrees/api/fix-payments-timeout/   worktree, branch fix/payments-timeout
```

Worktrees live outside the repository, under `git.worktrees.root`. The default keeps them
in one place per machine, grouped by repository, so a worktree is never nested in another
checkout and never shows up as an untracked directory. `{repo}`, `{branch}` and `{change}`
are replaced; slashes in the branch name become dashes.

The main checkout is where `init` ran and where `.alfred/config.yaml` lives. It is not
worked in while worktrees are open, and it is the one `status` and `worktrees` report from.

## Branches

Every worktree needs a branch name; it is given, never derived. The base defaults to the
branch the main checkout is on, and can be named explicitly.

```
feature/login-google              from the current branch
fix/payments-timeout from main    from main
```

A branch that already exists, locally or on `origin`, is checked out rather than recreated.
A branch that already has a worktree is resumed: `open` returns the existing path and says
`status: existing`. Starting the same change twice is therefore the same as `continue`.

## The tool

Worktrees are created and removed by `git.worktrees.tool`, never by an agent running
`git worktree` by hand. The steps are deterministic and the failure modes are known; a
script does them the same way every time and prints fields the orchestrator records.

```
worktree.sh open    --branch <name> [--base <ref>] [--change <slug>]
worktree.sh close   --branch <name> [--force]
worktree.sh abandon --branch <name> --yes
worktree.sh list
worktree.sh path    --branch <name>
```

Every mode takes `--cwd <dir>`, any directory inside the repository or one of its
worktrees; the tool finds the main checkout from there. Output is YAML.

`open` prints what the state file needs:

```yaml
branch: feature/login-google
base: main
path: /home/me/.alfred/worktrees/api/feature-login-google
main_checkout: /home/me/Work/api
change: login-google
status: created
setup: ok
copied: [.env]
```

## Preparing a worktree

A new worktree has the tracked files and nothing else: no installed dependencies, no
`.env`, nothing the project's tests need that git does not hold. The tool prepares it from
two configuration values, and an agent never improvises the preparation:

```yaml
git:
  worktrees:
    setup_command: npm ci
    copy_files: [.env, config/local.yml]
```

`copy_files` are copied from the main checkout when present there and absent in the
worktree. `setup_command` runs inside the worktree; its output goes to a log in the
worktree's git directory, and a failure is reported as `setup: failed` without removing
the worktree. `none`, the default, does nothing and is reported as `setup: none`, so the
orchestrator can say that dependencies were not installed rather than discover it when
`verify` cannot run the tests.

## State

A change running in a worktree records where, per `state-contract.md`:

```yaml
worktree: /home/me/.alfred/worktrees/api/feature-login-google
branch: feature/login-google
base: main
main_checkout: /home/me/Work/api
```

The state file lives inside the worktree, under its `.alfred/state/`, and is committed
with the change like any other. `list` reads it from each worktree to report the phase and
status of every change at once.

## Subagents

A phase working on a change in a worktree receives the worktree as an address, like
everything else it receives:

```
Worktree: /home/me/.alfred/worktrees/api/feature-login-google
Skill:    .alfred/skills/apply/SKILL.md
Task:     3 of alfred/login-google/tasks
```

Every path in the task is relative to that directory, and every command runs there. A
subagent that runs the tests in the main checkout tests the wrong code and reports a
result about a branch it did not touch; the address exists so that it does not have to
guess.

## What is shared

Memory is shared across worktrees, and that is correct: keys carry the change name, so
`alfred/login-google/spec` and `alfred/payments-timeout/spec` do not collide, and a
postmortem written from one worktree is recalled from the next.

Notifications carry the change name too, per `notify/CONTRACT.md`, so a user following
several changes from a phone can tell which one is asking.

The skill registry is per worktree, since each has its own `.alfred/`; the session hook
writes it. Configuration is read from the main checkout.

## Closing

`archive` commits on the worktree's branch and pushes it. Then, with
`git.worktrees.remove_on_archive`, it runs `close`, which removes the worktree and deletes
the branch only if it is already merged into `base`. Otherwise the branch stays, for the
pull request.

`close` refuses a worktree with uncommitted changes to tracked files. After a correct
`archive` there are none, so the refusal means a file `apply` changed and did not report,
and it is reported rather than forced. Untracked files are discarded: they are the
copied `.env`, the setup output, the things a commit never included.

A pull request is opened only with `git.worktrees.pull_request: true`, and only when the
tool for it is available. Off by default: opening one is a decision about the team's
review flow, not about the change.

## Abandoning

A change that will not be finished is removed with `abandon`, which discards the worktree
and keeps the branch. The tool refuses without `--yes`, and prints whether uncommitted
changes would be lost, so the orchestrator asks with `confirm()` and passes `--yes` only
after the answer. The state file goes with the worktree; `status` stops listing the change.

## Listing

`list` reports every worktree of the repository from any checkout: branch, base, path,
change, current phase and status, and whether the directory still exists. `status`
includes it; `doctor` reports a worktree whose directory is gone as something to `close`.

## Not in this version

A change spanning several repositories still runs at workspace level with one checkout
per repository; a worktree per repository per change is a later step. So is a worktree per
task inside `apply`, which would lift the disjoint-files rule: it needs a merge step the
pipeline does not have yet.
