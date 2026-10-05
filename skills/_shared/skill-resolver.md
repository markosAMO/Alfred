# Skill resolver

A skill is referenced by name and resolved to a path. Nothing in the pipeline holds a
hardcoded path to a `SKILL.md`.

## Resolution order

A phase is resolved within the scope of the workflow running it, per
`skills/_shared/workflow-protocol.md`. A repository's workflow has three roots:

```
resolve(invoice, under the repository's workflow billing)
  1  .alfred/workflows/billing/skills/invoice/SKILL.md   the workflow's own phase
  2  .alfred/skills/invoice/SKILL.md                     repository override
  3  ~/.config/alfred/skills/invoice/SKILL.md            global installation
```

A machine's workflow has two, its own `skills/` and the global installation. Root 2 is not
among them, and that is deliberate: a machine workflow is available in every repository, so
resolving one of its phases against one repository's overrides would register a workflow
that works there and is missing a phase everywhere else. A phase only a repository can
provide belongs to a repository-scope workflow.

The first match wins. A repository that defines none of its own uses the global set, and
`./install.sh update` keeps every such repository current by updating one place.

Root 1 is how a workflow brings a phase the library does not have, and how it brings its
own version of one the library does have without changing it for anything else: a
workflow's own `review` is resolved from the workflow and is a different phase from the
shared `review`, which is why they never collide as subagents.

An override exists for the cases where one repository genuinely needs different rules.
A Rails API and a React front end can disagree about what `apply` or `review` should
enforce, and one global skill cannot serve both without being changed for one and broken
for the other.

A phase that matches in no root of its scope is not a run-time failure. It rejects the
workflow at registration, per `skills/_shared/workflow-protocol.md`, which is the one
moment the user is looking at the workflow rather than at a change.

## The registry

Resolution runs once, at the start of a run, and is written to `paths.skill_registry`.

```markdown
| skill   | path                                              | source   |
|---------|---------------------------------------------------|----------|
| refine  | ~/.config/alfred/skills/refine/SKILL.md           | global   |
| spec    | ~/.config/alfred/skills/spec/SKILL.md             | global   |
| apply   | .alfred/skills/apply/SKILL.md                     | local    |
| invoice | .alfred/workflows/billing/skills/invoice/SKILL.md | workflow |
```

`source` is the column that matters six months later, when a phase behaves differently in
one repository and nobody remembers that it was overridden. It carries which root the
phase came from — `workflow`, `local` or `global` — because the three answer different
questions: a `workflow` phase belongs to one recipe and exists nowhere else, a `local` one
is this repository disagreeing with the installation about a shared phase, and a `global`
one is the installation as it shipped.

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

A skill referenced but present in no root of its scope fails the run and reports every
path that was searched. A phase silently skipped because its skill was missing produces a
pipeline that appears to succeed while doing less than it claims.
