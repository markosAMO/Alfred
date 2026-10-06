---
name: alfred
mode: interactive
pipeline_phase: false
reads: [config, skill_registry, state]
writes: [skills, workflows, config, skill_registry]
---

# alfred

Manage Alfred itself: inspect a repository's setup, update it, diagnose it, register its
workflows, and author new skills and workflows.

This is not a pipeline phase. It never appears in a route. Its operations run in the
`alfred-manage` subagent, which has a shell: the orchestrator delegates them like any phase
and relays what comes back. The same subagent opens and closes worktrees when the
orchestrator asks, running `git.worktrees.tool` and returning its output as printed.

## status

Report what is installed here and what is running.

```
global skills     ~/.config/alfred/skills/     13 skills, version 0.4.0
local overrides   .alfred/skills/              apply
profile           mixed
memory            engram, reachable
documents         ephemeral (default)          artifacts: committed, retain final_only
tracker           none
open changes      login-google (design, waiting_for_input)
```

Overrides are listed explicitly. A phase behaving differently in one repository is almost
always an override nobody remembers making.

`documents` says whether the mode was chosen or inherited, for the same reason. `ephemeral`
became the default after `keep` was, so a repository still on `keep` is either one that
decided to be or one that predates the change, and those read identically without the
annotation.

## update

Report whether the installed skills are current, and how to refresh them.

Refreshing is `./install.sh update`, run from the Alfred clone. This skill cannot do it:
copying files into `~/.config/alfred/` is the installer's job, and a skill is a document,
not a program.

```
for each managed file
  stored hash matches the file?
    yes  -> update it
    no   -> the user changed it: report the difference, change nothing
```

Never overwrite a modified file. Report it and let the user decide, which is the whole
point of storing the hashes.

## reindex

Rebuild the memory index from the files, per `memory/CONTRACT.md`.

```
walk docs/ for artifacts
derive each key from its path, per the contract's naming rules
remember() each one
report what was indexed
```

Run it after cloning a repository, after switching memory backend, or when files and index
may have diverged. Existing entries are replaced by key, so it is safe to repeat.

With no memory backend configured this is a no-op, and says so rather than appearing to
have done something.

### Migrating a repository to pointer

`reindex` reads `memory.documents`. Under `keep` and `ephemeral` it stops at the report
above: indexing is all there is to do and nothing on disk changes. `ephemeral` deletes at
close, in `archive`, and never here — a change still open keeps every document it has
written so far.

Under `pointer` the change documents belong in memory and the repository keeps only their
addresses, so a `docs/changes/` written while the repository was on `keep` is carrying
documents the configuration says should not be there. This is the operation that moves
them, and the only one in Alfred that deletes a document.

```
for each directory under docs/changes/
  index every document in it, as above
  read each one back with fetch() and check the document is in what comes back
  write docs/changes/{change}/README.md from templates/docs/addresses.md,
    one row per document, under the key it was indexed as
  delete the documents that file now names
```

Four conditions are refused rather than worked around:

```
a read-back without the document in it   stop, name the document, delete nothing
a file not tracked by git                stop: deleting it would be unrecoverable
a dirty working tree                     stop: the resulting diff would be unreadable
a backend that does not answer           stop, per memory.required under pointer
```

The read-back is the whole safety of the operation. An index that reports success and
stored nothing is indistinguishable from one that worked, right up to the moment the files
are gone.

Confirm before deleting, with the counts and with what is *not* being touched:

```
confirm("Move 47 documents from 9 changes into memory and delete them",
        "recoverable from git history; docs/specs/, architecture.md and
         code_conventions.md stay as files")
```

`paths.master_specs`, `docs/architecture.md` and `docs/code_conventions.md` are never
moved, in this operation or any other: `pointer` is about the change directory, per
`memory/CONTRACT.md`.

Run it once per repository. From then on the phases write in `pointer` shape themselves and
there is nothing left to migrate.

This is not the skill registry. `reindex` rebuilds the memory index from documents;
`registry` rescans the skill roots. They were conflated once because both are called
rebuilding something.

## registry

Rescan both skill roots and rewrite `.alfred/skill-registry.md`.

```
resolve every skill: .alfred/skills/ first, then the global installation
write name, path and source for each
report what changed
```

Run it after adding, removing or overriding a skill. A skill added under `.alfred/skills/`
that is not in the registry is not resolved, and the phase silently uses the global one.

## doctor

Check the setup and name what is broken, with the fix.

```
is a model profile active
is every skill in the registry resolvable
is the memory backend reachable, and does the registry match what is on disk
is the test command in code_conventions.md runnable
are the agent pointer files present and pointing at AGENTS.md
is every workflow this repository defines registered on every agent installed here
is any project-level Alfred command left by a workflow this repository no longer has
under artifacts.committed: true: do the skill paths its commands carry exist on this machine
is any state file referencing a change directory that no longer exists
under pointer: does every address file row still resolve, and does state agree with it
under ephemeral: does every closed change have a record, and is the state file gone
does the memory backend report a project name matching this repository's git remote
is any worktree listed whose directory no longer exists   worktree.sh list, exists: false
does every open worktree have architecture, conventions and a configuration
under artifacts.committed: false: does any worktree hold documents the main checkout lacks
```

Two of those are conditions rather than faults, and are reported as such.

`memory.documents: ephemeral` with `backend: none` discards the working documents at close
with no second copy anywhere. It is supported and it is a choice; `doctor` names it so it is
not discovered after the fact.

A repository with no git remote makes the memory backend fall back to the directory name, so
two clones in differently named directories accumulate two memories that never see each
other and both runs succeed. `doctor` reports the missing remote; `mem_merge_projects`
merges what already diverged. See `memory/adapters/engram.md`.

The three workflow checks are this repository's half of rule 22 in the package `AGENTS.md`,
and the installation's own `doctor` checks the machine's half. Both halves or neither: a
workflow with no command is indistinguishable from a workflow nobody wanted, and a command
whose workflow is gone runs a recipe that no longer exists. The first is answered by the
registration tool in its checking mode:

```
~/.config/alfred/bin/register.sh --project <repository> --check
```

It names every workflow of this repository with no command, and the agent it is missing on,
and exits non-zero when it finds one. The remedy for all three is the `workflows` operation
below.

The last of them is a consequence of committing the generated commands rather than a fault in
them: they carry this machine's absolute skill paths and its model identifiers, which resolve
to nothing in another clone, and they cannot be made machine-neutral while a command
carries a model. Registering in that clone rewrites them. See `docs/artifacts.md`.

Each failure is reported with its remedy. A diagnostic that says something is wrong without
saying what to do is a longer way of failing.

The test command check matters most: a wrong command makes every `verify` an environment
failure, and it is invisible until the first change reaches that phase.

The two worktree checks report what `worktree.sh open` prints when a worktree is created,
for the worktrees that are already open. A missing `conventions` is not an error — a
repository may genuinely not have written them — but a phase that is told costs nothing and
a phase that discovers it costs a rediscovery per subagent. The document check is the one
that prevents loss: under `artifacts.committed: false` nothing in git is holding those
files, so a worktree removed before `archive` copied them back takes the only copy with it.

## add-workflow

Write a workflow the user has already accepted, and register it.

The interview is a command of its own and happened before this: it settled the name, the
phases, the routes, the default route, the entry points, the groups dispatched together, the
closing phase, the rules and the model of every phase the workflow brings, showed the whole
thing and waited. This operation writes it and decides nothing. It is given what it needs:

```
Operation:    add-workflow
Templates:    the workflow templates directory
Destination:  the workflow's directory, under the machine's custom root or a repository's
Workflow:     the facts the user accepted, as they were accepted
```

```
1  refuse a name that already exists in the destination's scope, or that is reserved
2  create the destination directory
3  write workflow.json from the template, as accepted, carrying the model of every phase
   the workflow brings on that phase's entry
4  write rules.md from the template, in the words the interview showed
5  write skills/<phase>/SKILL.md from the template for every phase the workflow brings
6  register that scope and return the report as printed
```

The templates are filled, never improvised. Two workflows written by hand in different shapes
are two documents to keep in step with one validator, and the definition is checked key for
key when registration reads it — see `docs/workflows.md`.

**No model is chosen here.** A phase the user left without one is written without one, and
registration names it as a phase with no model; nothing is carried over from another phase,
from the shared library or from this operation's judgement. It is the one decision that costs
money on every run, and it is the user's, taken when the workflow is created.

The destination is passed in and never inferred. A workflow that belongs to one repository is
the same files written under that repository's `.alfred/workflows/<name>` and registered for
that repository; a workflow that belongs to the machine is written under its custom root and
registered for the machine. Nothing is written under the shipped root, which the installation
owns, and nothing in the Alfred package changes.

```
created: custom/workflows/billing, 2 own phases (draft, invoice)
registered: /alfred-billing on claude code, alfred-billing on opencode
```

A workflow the report rejects was written and is not registered. Show the reason as printed,
name the path the definition is at, and say that registration runs again through the
operation below once it is fixed. A rejection is not a reason to delete what the user
accepted.

## workflows

Register the workflows of one scope, in any of its three modes. Nothing is reimplemented
here: the operation is the registration tool, and the report is the tool's.

```
~/.config/alfred/bin/register.sh                          the machine's workflows
~/.config/alfred/bin/register.sh --project <repository>   the repository's own
```

```
(no option)   validate every workflow of that scope, regenerate, report what changed
--dry-run     the identical report, writing nothing
--check       every workflow of that scope with no command; non-zero when any is found
```

Run `--dry-run` when the request asks what would change, and `--check` when it asks whether
anything is missing. Return the output as printed rather than summarised: it names each
workflow with its source and its routes, what was added, updated, removed and left unchanged
per agent, and every phase that has no model. A run over a tree that changed in no way writes
nothing and says so.

Exit code 1 means a workflow was rejected, a phase has no model, a checked command is
missing, or no supported agent was found. It is an answer, not a crash — every valid
workflow still registered — and the report says which of them it was.

**The scope is the user's request, never a guess.** A request about this repository registers
this repository and leaves the machine alone; a request about the machine never reaches
into a repository. Registering the wrong one writes commands where nobody asked for them.

Run it after a workflow directory is created, edited or removed by hand, after a repository
starts overriding one of the machine's, and whenever a command is missing or stale.
Installing and updating Alfred already register the machine's workflows; this operation
exists for the times when nothing is being installed, and `init` is what registers a
repository's for the first time.

## worktrees

List every worktree of this repository with its change, phase and status, with the tool
named in `git.worktrees.tool`:

```
~/.config/alfred/bin/worktree.sh list --cwd <repository>
```

Run it from any checkout, the main one or a worktree: the tool finds the repository. It
reads each worktree's state file, so a worktree whose change never started shows
`not_started`, and one whose directory is gone shows `exists: false` and should be closed.
`status` includes this list when the repository has worktrees. See
`skills/_shared/worktree-protocol.md`.

## abandon

Remove a worktree without archiving its change. The branch is kept.

```
confirm("Remove the worktree for feature/login", "uncommitted changes are discarded; the branch is kept")
~/.config/alfred/bin/worktree.sh abandon --branch <branch> --cwd <main_checkout> --yes
```

Without `--yes` the tool prints what it would remove, including whether there are
uncommitted changes, and stops. Run it that way first, show the answer, and pass `--yes`
only after the user confirmed. The state file goes with the worktree; a change abandoned
this way is not `failed`, it is gone, and `status` stops listing it.

## new skill

Author a skill and register it.

```
skills/<name>/SKILL.md with frontmatter: name, mode, skippable, next
shared rules referenced from skills/_shared/, never repeated
a template in templates/docs/ when it produces a document
an entry in the workflow's workflow.json, with what it reads and recalls
```

A skill states what the phase does and why the constraints exist. It names no model, no
concrete memory or notification tool, and no path that the registry should resolve.

## What it does not do

It does not install or update Alfred on the machine. That is `./install.sh`, run from the
clone, which has to exist before any repository does, per `docs/installation.md`.

It does not edit the package itself. Modifying Alfred happens in the Alfred repository,
under its own `AGENTS.md`.
