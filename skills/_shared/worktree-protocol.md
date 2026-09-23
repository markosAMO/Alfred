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
asked from. So are two changes that are not independent.

## Coupling, which a worktree does not isolate

A worktree isolates file writes. It does nothing about decisions, and two changes that need
to agree on one are worse off apart than together: each decides alone, they decide
differently, and the disagreement surfaces later as work that has to be undone in both.

The coordinator reads the list for that before the user confirms it, and says which changes
will need to know what another decided. Shared files are not the test — two changes are
coupled when they turn on the same setup command, the same entry point, the same dependency
list or the same document, whoever writes it. Coupled changes are recommended into one
session, because an orchestrator holding both knows what each decided for free, and the
coordinator holding both only learns it by carrying a correction between them.

It is a recommendation, not a refusal. The user may want the worktrees anyway, and a change
list is theirs to decide.

## Files two changes both touch

Coupling is about decisions and is judged before the run. Shared *files* are knowable later
and exactly: each change's `tasks` declares the files of every task, so once two changes
have both reached `tasks`, the coordinator can intersect the lists.

It is worth doing at that moment rather than discovering it in `apply`.

```
both changes touch app/models/concerns/channel_sender_helper.rb
  -> the change that reaches apply first owns it
  -> the other is told, before its subagent is dispatched
```

The change that owns the file writes it. The other is told which file, who owns it, and what
to do when its own task needs the same file: stop and report, per `skills/apply/SKILL.md`,
which is what a task did in one run and was right to do. The difference is that it knew
beforehand rather than discovering it mid-task, and that it cost a message instead of a
round trip through the user.

**A fix that is idempotent needs no round trip at all.** A task that finds a shared file
already corrected the way its own change requires leaves it alone and says so. A task whose
correction is the same correction applies it, whether or not the other change got there
first, and the second writer changes nothing.

```
channel.to_s.camelize applied by either change, in either order, is one result
```

That is not a licence to write a shared file whenever the result looks the same. The test is
whether the two changes want the same final state of that file, and it is answered by
reading what the other change's design says about it — which is in memory, under that
change's key, where one run's `design` read another's without anyone relaying it.

Where the answer is no, or where reading it does not settle the question, the task stops and
reports. Guessing about a file another change owns is the failure the whole layout exists to
prevent.

Where worktrees pay is the opposite case: changes independent in decisions as well as in
files, each long enough to be worth a session, each needing few answers. The bound that
matters there is not `max_parallel` but how many questions the user is answering at once —
three changes that each ask constantly do not run in parallel no matter how many worktrees
they have, because the user is the thing they are queueing for.

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

`.alfred/config.yaml` is in `copy_files` by default. A worktree that does not have it is a
change whose every phase reads the configuration of a repository it is not standing in, or
fails to find one at all.

## What the tool reports before a phase can ask

A worktree is created from a ref, so anything the main checkout has and git does not is
simply absent from it. The tool is standing in the worktree it just made and knows this for
free; every phase that has to find it out instead finds it out separately.

```yaml
architecture: present
conventions: missing
config: present
```

This is the whole of a friction that is otherwise invisible: a run where
`docs/code_conventions.md` was never committed had five subagents discover its absence
independently, and the orchestrator could not have told them, because it reads the
configuration, the state and the registry and nothing else.

The orchestrator records these three fields with the rest of the record, and carries
`conventions: missing` into every dispatch, so a subagent is told rather than left to infer
it from a failed read. A phase that knows the file is absent follows the patterns already in
the code; a phase that expected it and did not find it spends its first minutes deciding
whether it looked in the wrong place.

## Repositories that do not accept Alfred

`artifacts.committed: false` is for a repository where Alfred's own documents cannot be
committed: a team that has not adopted it, a checkout whose review process would reject the
directory, a fork nobody owns. Alfred still runs there. What changes is who is holding the
documents.

Under `committed: false` they are kept out of git deliberately, through
`artifacts.local_exclude` — `.git/info/exclude` by default, which is per-clone and untracked,
so excluding them is not itself a change to the repository. `.gitignore` would be.

Two consequences follow, and the tool handles both:

**A worktree cannot inherit them.** Git has no copy to hand over. The tool copies
`paths.architecture`, `paths.conventions`, `paths.master_specs` and `paths.changes` from the
main checkout when it creates the worktree, and reports `artifacts: copied` with what it
carried. Under `committed: true` it reports `artifacts: tracked` and copies nothing, because
the branch already has them.

**Closing a worktree would destroy them.** `close` discards untracked files, which is correct
when they are a copied `.env` and wrong when they are the specification of the change that
just finished. So `close` refuses when the worktree holds Alfred documents the main checkout
does not have byte for byte, names them, and stops. `archive` copies them back before asking
for the close; the refusal means it did not.

The documents are worth more in this mode, not less: git is not holding them, so the main
checkout and the memory backend are the only two copies there are. A repository that cannot
commit them is the one where a memory backend stops being an optimisation.

## Sessions

A session is started in the background, named for its change, with its working directory
set to the worktree. Which command does that is a property of the machine and the agent, not
of the repository, so each agent's coordinator carries its own — the same reason the model
profile lives on the machine. A repository configures nothing to run changes this way.

`git.worktrees.sessions.start` exists to override it, with `{change}` replaced, for a
machine whose CLI is not on the path or is wrapped. It is absent from a normal
configuration, and a repository that carries it has pinned every machine that clones it to
one spelling of a command that is not the repository's business.

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

**The first line of that message loads the skill, before anything else is said.** A session
that reads the request first is a general-purpose agent reading source files to orient
itself, and it has already spent context and made decisions by the time it learns there was
a pipeline for this. Observed: an orchestrator opened two source files before loading the
skill that tells it never to open one.

What follows the skill line is one block, not instructions arriving as the run needs them:
the addresses, the preflight fields above, what the staging scope is, whether documentation
is committed with the change, and which resources are shared with other worktrees. Every one
of those is a property of the repository or of this fleet, known before the change starts. A
standing instruction that arrives in the middle of a run has already been violated once by
definition, and whoever sends it pays a round trip to say something that was true from the
beginning.

The coordinator does not announce its own address. A message arrives wrapped with the
sender in it, and a reply is that sender copied back, so a session learns where to report
from the first message it receives.

The coordinator does not subscribe to a session going idle, and does not ask one whether it
is done. Every phase reports when it completes, so a session that owes an answer sends one;
a session that is quiet is working. An idle subscription on top of that fires on turns the
session has already reported, and each notice costs the coordinator a full turn — its whole
conversation re-read — to conclude that nothing happened.

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

## What a relay costs

The coordinator passes text through. It does not restate a session's question in its own
words, and it does not restate the user's answer; it adds the change name and nothing else.

This is a cost rule and a correctness rule at once. Everything the coordinator composes is
written once and then re-read on every later turn of two conversations — a measured run put
sixty million tokens of cache read against three quarters of a million of output, which is
every written token read back about eighty times. Restating is the one cost in this topology
that buys nothing, because the text already existed. And a coordinator that paraphrases an
argument it is not equipped to judge — it reads no specification, no design and no diff — is
a channel that loses the detail the question turned on.

The compact half of this belongs to the session, not to the coordinator. A question travels
as the question, the options, and the phase's own recommendation; a session that sends its
reasoning in full is asking the user to read what the phase was supposed to weigh. Where the
user wants the reasoning, they ask for it, and the coordinator relays that too.

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
writes it. Configuration is copied into each worktree at `open`, per `copy_files`, so a
phase reads the configuration of the repository it is standing in.

The master specifications are the one document two changes both write, and they are **not**
shared. Each worktree has its own copy on its own branch, `archive` merges the delta there,
and the merge lands in the change's commit. Two changes touching the same requirement then
meet as a git conflict when the branches do, which is a question for a person.

A worktree that writes the master specifications in the main checkout instead produces a
commit whose specification is missing, a file two changes can overwrite with no branch
between them, and a dirty main checkout nobody expected. All three were observed in one
run, and the third is what made it visible.

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
