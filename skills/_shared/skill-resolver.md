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

Resolution runs once, at the start of a run, and is written to `paths.skill_registry`.

```markdown
| skill   | path                                     | source |
|---------|------------------------------------------|--------|
| refine  | ~/.config/alfred/skills/refine/SKILL.md  | global |
| spec    | ~/.config/alfred/skills/spec/SKILL.md    | global |
| apply   | .alfred/skills/apply/SKILL.md            | local  |
```

`source` is the column that matters six months later, when a phase behaves differently in
one repository and nobody remembers that it was overridden.

The registry is regenerated whenever a skill is added, removed or overridden. It is **not**
committed: the `path` column carries machine-absolute paths into the global installation,
which resolve to nothing on anyone else's machine and to the wrong thing on a machine that
installed Alfred somewhere else. It is regenerated per clone by `alfred registry`, and
`init` adds it to the repository's ignore file.

This file previously said the opposite while `skills/archive/SKILL.md` said it was ignored,
each citing the other.

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
