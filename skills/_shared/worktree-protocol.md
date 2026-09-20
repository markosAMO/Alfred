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

Each change runs in its own agent session, whose working directory is its worktree. The
session the user started is the coordinator: it opens the worktrees, starts a session per
change, and from then on routes messages between those sessions and the user.

```
coordinator                  the session the user types into
  ├─ session   cwd = the worktree of change A
  └─ session   cwd = the worktree of change B
```

The coordinator is not an orchestrator. It proposes no route, reads no specification,
dispatches no phase and holds no plan. Each session is an ordinary Alfred orchestrator and
runs the ordinary pipeline; nothing about a phase changes because it runs in one.

One session per worktree, never two. The worktree exists so that two pipelines do not write
the same working tree, and two sessions in one would reintroduce exactly what it prevents.
Tasks inside `apply` remain subagents of their change's session, on disjoint files.

A phase that needs an answer has nobody in its own session to ask. It returns the question
rather than asking, its session relays it to the coordinator, and the coordinator puts it to
the user; the answer travels back the same way. Questions and reports carry the change name,
and an answer applies to the change it names. See `Asking` below.

`git.worktrees.max_parallel` bounds how many changes run at once. It used to bound the
orchestrator's working set, which grew by a state file per change; now that each change
holds its own, what it bounds is machine and spend.

`/alfred-worktree continue` resumes every worktree with an open change, from the state file
inside each, adopting a session that is still alive and starting one where it is not.

A change in the current checkout, alone, is still `/alfred`, and runs in the session it was
asked from.

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

## Sessions

A session is started in the background, named for its change, with its working directory
set to the worktree. The command is `git.worktrees.sessions.start` with `{change}`
replaced. The protocol names the setting; the setting names the agent's CLI, as
`git.worktrees.tool` does for the worktree script.

In the background, and not in a terminal. The difference is not presentation: a session
becomes addressable only once it has started a conversation, so one launched interactively
in a terminal window sits there unregistered and unreachable, while a background session
registers as it comes up. A `start` that opens a terminal produces changes the coordinator
can see running and cannot talk to, which is the one thing this topology needs. Watching one
is a separate act, and the agent's CLI has its own way to attach to a session already
running.

A session comes up idle: starting one is not handing it a task. The coordinator gives it
its work in the first message, which says it is the Alfred orchestrator for that change and
carries the change's addresses — worktree, branch, base, main checkout, and the request.
Nothing is pasted into it that a path would do.

The coordinator does not announce its own address. A message arrives wrapped with the
sender in it, and a reply is that sender copied back, so a session learns where to report
from the first message it receives.

To learn that a session has finished without asking it, a message carries
`notify_when_idle`. Polling the session list, or sending a session a message asking whether
it is done, is what that exists to replace.

Two constraints are not the coordinator's to relax:

**`--bare` is never used.** A session started with it does not register and cannot be
addressed. It is the flag that skips the machinery this runs on.

**The user has to permit the start command before the first run.** Starting a session is the
coordinator running a command, and a command it has not been permitted stops the run before
any change begins. The permission belongs to the user's own agent settings, granted once for
the command `sessions.start` names. The coordinator cannot grant it: an agent that widens its
own permissions has removed the decision the permission existed to record, so a coordinator
blocked here reports the command it needs permitted and stops.

**A session inherits the permissions the user already granted.** Started with no tool flags
at all, it can do what the user's own settings allow: a session started that way wrote a
file and queried the memory backend without either being granted to it. This is the default
and it is the point — a phase must be able to reach what it needs without anyone having
enumerated, in advance and somewhere else, what the phases use.

`sessions.allowed_tools` exists to narrow that, not to enable it, and is left unset unless
there is a reason. Setting it replaces inheritance with exactly what it lists, and a
background session cannot show a permission prompt, so anything omitted is refused rather
than deferred. A list that forgets the memory tools produces a session that reports the
backend as unreachable — a backend that is fine, and a tool that was never granted.

A permission *mode* is not the lever either. `dontAsk` means do not ask and therefore deny,
which leaves a session that plans and writes nothing; bypassing permissions wholesale is
refused when a session is started from inside another one.

One thing a session must not mistake for an absent backend: some tools arrive deferred, with
their names known and their schemas not, and have to be loaded before the first call. A
memory tool that has not been loaded yet is not a memory backend that is down.

The coordinator cannot widen a grant, and must not route around one. A session reporting
that it was refused an action is reported to the user; a coordinator that performs the
action itself launders the decision the user was supposed to make.

## Asking

A phase in `interactive` or `confirm` mode has nobody in its session to ask. The question
travels, and it travels by returning rather than by sending.

A subagent that sends a message sends it under its session's address, and the reply is
delivered to that session rather than to the subagent: a phase that asks directly waits for
something that never arrives. So the phase returns its question the way it returns any
result, and its session relays it.

```
alfred-refine  --return->  session   --message->  coordinator  ->  user
alfred-refine  <-dispatch- session   <-message--  coordinator  <-  user
```

Three consequences, none of them free:

- A question is no longer a pause inside a phase. The phase ends, state records that the
  change is waiting and what for, and the phase is dispatched again with the answer.
  `continue` and an answered question become the same operation.
- State that says `waiting_for_input` without naming the question leaves the coordinator
  with nothing to put to the user. What is being asked belongs in the state file.
- `origin_channel`, per `state-contract.md`, is where blocking questions return. For a
  change running this way it names the coordinator.

When several changes are waiting at once, an answer applies to the change it names; when it
names none and more than one is waiting, the coordinator asks which.

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

The coordinator keeps one file of its own, in the main checkout:

```yaml
# .alfred/coordinator.yaml
coordinator: alfred-3a
repo: /home/me/Work/api
updated_at: 2026-09-20T14:05:00Z

changes:
  login-google:
    worktree: /home/me/.alfred/worktrees/api/feature-login-google
    branch: feature/login-google
    session: wt-login-google
    phase: refine
    status: waiting_for_input
    waiting_on: which provider the existing accounts migrate from
```

It sits outside `.alfred/state/` deliberately: that directory is one file per change, named
for the change, and a fleet file inside it would either collide with a change of the same
name or become a silent exception to the rule `continue` depends on.

`session` is what the change's own state file has no business knowing — a session is a
property of this coordinator's run, not of the change. `phase`, `status` and `waiting_on`
are a cache, so that reporting every change does not mean reading into every worktree; the
change's own state file is authoritative and a coordinator that disagrees with it re-reads
it.

The file exists mainly for one failure. A coordinator that dies or compacts loses the map
from change to session and leaves sessions running with nobody listening. `continue` reads
it, adopts by name the sessions that are still alive, and starts one for each change whose
session is gone.

## Subagents

A phase working on a change in a worktree is dispatched by that change's session, whose
working directory is already the worktree:

```
Skill: .alfred/skills/apply/SKILL.md
Task:  3 of alfred/login-google/tasks
```

The worktree is not passed as an address because it is not somewhere else. Every path is
relative to where the phase already stands, and every command runs there. A subagent that
ran the tests in the main checkout would test the wrong code and report a result about a
branch it never touched — an address existed to prevent that, and a working directory
prevents it without anything having to be carried.

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
