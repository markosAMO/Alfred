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
and relays what comes back.

## status

Report what is installed here and what is running.

```
global skills     ~/.config/alfred/skills/     13 skills, version 0.1.0
local overrides   .alfred/skills/              apply
profile           mixed
memory            engram, reachable
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

This is not the skill registry. `reindex` rebuilds the memory index from documents;
`registry` rescans the skill roots. They were conflated once because both are called
rebuilding something.

## registry

Rescan both skill roots and rewrite `.alfred/skill-registry.md`, with the tool named in
`skills.registry_tool`:

```
~/.config/alfred/bin/registry.sh write --cwd <repository>
~/.config/alfred/bin/registry.sh write --cwd <repository> --force
```

The first rewrites only when the table changed and reports each row added, removed or
changed. `--force` rewrites regardless, for when the file was edited or corrupted by hand.
Report what the tool printed, nothing else: the table is never written by hand.

The session hook runs the same tool in `sync` mode every time an agent opens the repository,
so this operation is for the moment a skill is added or overridden mid-session, or when the
hook is `off` in this repository. See `skills/_shared/skill-resolver.md`.

## doctor

Check the setup and name what is broken, with the fix.

```
is a model profile active
does the registry match what is on disk        registry.sh check --cwd <repository>
is the registry ignored by git                 .alfred/skill-registry.md in .gitignore
is the memory backend reachable
is the test command in code_conventions.md runnable
are the agent pointer files present and pointing at AGENTS.md
is any state file referencing a change directory that no longer exists
```

`check` exits 1 and lists each stale row when the registry differs from the disk; the
remedy is `registry`. A registry that is versioned rather than ignored is reported too,
with the two commands that fix it: add the line to `.gitignore`, then
`git rm --cached .alfred/skill-registry.md`.

Each failure is reported with its remedy. A diagnostic that says something is wrong without
saying what to do is a longer way of failing.

The test command check matters most: a wrong command makes every `verify` an environment
failure, and it is invisible until the first change reaches that phase.

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
