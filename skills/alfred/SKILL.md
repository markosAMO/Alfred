---
name: alfred
mode: interactive
pipeline_phase: false
reads: [config, skill_registry, state]
writes: [skills, config, skill_registry]
---

# alfred

Manage Alfred itself: inspect a repository's setup, update it, diagnose it, and author new
skills.

This is not a pipeline phase. It never appears in a route. Its operations run in the
`alfred-manage` subagent, which has a shell: the orchestrator delegates them like any phase
and relays what comes back. The same subagent opens and closes worktrees when the
orchestrator asks, running `git.worktrees.tool` and returning its output as printed.

## status

Report what is installed here and what is running.

```
global skills     ~/.config/alfred/skills/     13 skills, version 0.2.0
local overrides   .alfred/skills/              apply
profile           mixed
memory            engram, reachable, documents: pointer
tracker           none
open changes      login-google (design, waiting_for_input)
```

Overrides are listed explicitly. A phase behaving differently in one repository is almost
always an override nobody remembers making.

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

`reindex` reads `memory.documents`. Under `keep` it stops at the report above: indexing is
all there is to do and nothing on disk changes.

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
is any state file referencing a change directory that no longer exists
under pointer: does every address file row still resolve, and does state agree with it
is any worktree listed whose directory no longer exists   worktree.sh list, exists: false
does every open worktree have architecture, conventions and a configuration
under artifacts.committed: false: does any worktree hold documents the main checkout lacks
```

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
skills/<name>/SKILL.md with frontmatter: name, mode, skippable, reads, writes, document, next
shared rules referenced from skills/_shared/, never repeated
a template in templates/docs/ when it produces a document
an entry in the configuration when it is a pipeline phase
```

A skill states what the phase does and why the constraints exist. It names no model, no
concrete memory or notification tool, and no path that the registry should resolve.

## What it does not do

It does not install or update Alfred on the machine. That is `./install.sh`, run from the
clone, which has to exist before any repository does, per `docs/installation.md`.

It does not edit the package itself. Modifying Alfred happens in the Alfred repository,
under its own `AGENTS.md`.
