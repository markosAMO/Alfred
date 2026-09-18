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
workflows         sdd (default), ventas (custom)
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
is every installed workflow registered           register.sh --check
does every workflow's phase resolve              register.sh, which fails naming the phase
```

Each failure is reported with its remedy. A diagnostic that says something is wrong without
saying what to do is a longer way of failing.

The test command check matters most: a wrong command makes every `verify` an environment
failure, and it is invisible until the first change reaches that phase.

## add-workflow

Create a custom workflow from a confirmed definition and register its command. The
interview happened before this, in `/alfred-add-workflow`; this operation only writes and
registers. See `skills/_shared/workflow-protocol.md`.

```
1  refuse a name that exists under workflows/ or custom/workflows/, or is reserved
2  create ~/.config/alfred/custom/workflows/<name>/
3  write workflow.yaml as confirmed
4  write rules.md from templates/workflow/rules.md, with the routing question and routes
5  write skills/<phase>/SKILL.md from templates/workflow/SKILL.md for every new phase,
   with its mode and the description given as the starting point
6  run ~/.config/alfred/bin/register.sh and return what it printed
```

Nothing is written under `workflows/`, which the installer owns, and nothing in the Alfred
package changes. The templates are filled, never improvised: two workflows written by hand
in different shapes are two formats to parse.

```
created: custom/workflows/ventas, 2 new phases (prospectar, propuesta)
registered: /alfred-ventas in Claude Code, alfred-ventas in OpenCode
```

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
