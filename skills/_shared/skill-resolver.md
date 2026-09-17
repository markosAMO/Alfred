# Skill resolver

A skill is referenced by name and resolved to a path. Nothing in the pipeline holds a
hardcoded path to a `SKILL.md`.

## Resolution order

```
resolve(spec)
  1  .alfred/skills/spec/SKILL.md            repository override
  2  ~/.config/alfred/skills/spec/SKILL.md   global installation
```

The first match wins. A repository that defines none of its own uses the global set, and
`./install.sh update` keeps every such repository current by updating one place.

An override exists for the cases where one repository genuinely needs different rules.
A Rails API and a React front end can disagree about what `apply` or `review` should
enforce, and one global skill cannot serve both without being changed for one and broken
for the other.

## The registry

Resolution is written to `paths.skill_registry`, one row per skill.

```markdown
| skill   | path                                     | source |
|---------|------------------------------------------|--------|
| refine  | ~/.config/alfred/skills/refine/SKILL.md  | global |
| spec    | ~/.config/alfred/skills/spec/SKILL.md    | global |
| apply   | .alfred/skills/apply/SKILL.md            | local  |
```

`source` is the column that matters six months later, when a phase behaves differently in
one repository and nobody remembers that it was overridden.

## Generation

The registry is produced by `skills.registry_tool`, a script installed with Alfred, never
by hand. It scans both roots, applies the order above, and writes the table.

```
registry.sh write   resolve and rewrite the registry; --force rewrites even when unchanged
registry.sh check   compare the registry with the disk; exit 1 when they differ
registry.sh list    print the table, write nothing
registry.sh sync    write only when missing or stale; never fails
```

`write` runs in `init` and in the `registry` operation of the alfred skill. `check` runs in
`doctor`. `sync` runs from the session hook.

## Kept current by a hook

Every time an agent opens a repository, a session hook runs `registry.sh sync` there.
Claude Code runs it from a `SessionStart` hook the installer registers; OpenCode runs it
from a plugin the installer copies into place. The script decides whether the directory is
an Alfred repository and does nothing otherwise, so the hook applies to every project the
agent opens without configuring anything per repository.

`sync` rewrites the registry only when the table it computes differs from the file, so a
session that changed nothing writes nothing. When it does rewrite, it prints one line
saying what changed. In Claude Code that line reaches the conversation; in OpenCode it
reaches the log.

`skills.registry_hook` in the repository configuration sets what the hook does there:

| Value | Behaviour |
|---|---|
| `sync` | rewrite when stale, say so in one line |
| `check` | never write; one line when stale |
| `off` | nothing |

The hook never commits.

## Not committed

The registry is derived from what is on disk, and what is on disk differs per machine:
the global root is wherever Alfred is installed there. So it is ignored, not versioned.
`init` adds `.alfred/skill-registry.md` to the repository's `.gitignore`, and the script
adds the line when it is missing (`--no-gitignore` prevents that).

A clone therefore has no registry until an agent opens it, at which point the hook writes
one. The orchestrator handles the interval: when the registry does not exist, skills are
read from the global root.

`.alfred/skills/`, the overrides, remain versioned. They are decisions the team made about
this repository, and the registry is only a record of where those decisions lead.

## Dispatching

The orchestrator passes the resolved path to the subagent. The subagent reads the skill
itself.

```
Skill: .alfred/skills/apply/SKILL.md
```

Never the contents. A subagent that receives a skill pasted into its prompt spends context
on it before its task begins, and reads whatever the orchestrator happened to have loaded
rather than what is installed in that repository.

## Missing skills

A skill referenced but present in neither root fails the run and reports both paths that
were searched. A phase silently skipped because its skill was missing produces a pipeline
that appears to succeed while doing less than it claims.
