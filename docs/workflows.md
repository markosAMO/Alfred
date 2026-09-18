# Workflows

A workflow is a recipe: which phases run, in which order, for which route. Alfred ships
one, spec-driven development, and lets you add your own. Every workflow is a command.

```
/alfred                 the default workflow, sdd
/alfred-sdd             the same, by name
/alfred-ventas          a workflow you defined
/alfred-add-workflow    define a new one
```

## The shipped workflow

`sdd` lives in `workflows/sdd/` in the package and in `~/.config/alfred/workflows/sdd/` once
installed. Its recipe is eleven lines:

```yaml
name: sdd
title: Spec-driven development
phases: [refine, research, spec, diagnose, design, tasks, apply, verify, review, archive]
routes:
  direct: [apply, verify, review]
  pipeline: [spec, design, tasks, apply, verify, review, archive]
  full: [refine, research, spec, design, tasks, apply, verify, review, archive]
default_route: pipeline
routing: rules.md
entry_points:
  feature: refine
  bug: diagnose
```

The phases are the skills under `skills/`, shared by every workflow. The routes and the
rules for choosing between them are the workflow's own.

## Defining your own

Ask for it:

```
/alfred-add-workflow
```

Alfred asks, one step at a time: a name, a title and description, which shared phases you
reuse and which new phases you need (what each does, and whether it runs on its own, asks
and waits, or asks permission first), the routes and the default, and the one question
that decides between routes. It shows the resulting definition, waits for your yes, writes
it, and registers the command. Restart your agent and `/alfred-<name>` is there.

Or write it yourself. A workflow is a directory:

```
~/.config/alfred/custom/workflows/ventas/
  workflow.yaml
  rules.md
  skills/
    prospectar/SKILL.md
    propuesta/SKILL.md
```

```yaml
name: ventas
title: Sales follow-up
description: Prospect, propose, follow up and close
phases: [prospectar, propuesta, seguimiento, review]
routes:
  rapida: [propuesta, review]
  completa: [prospectar, propuesta, seguimiento, review]
default_route: completa
routing: rules.md
entry_points:
  default: prospectar
```

A phase named here is looked for in the workflow's own `skills/` first, then in the shared
library. `review` above is the shared one; `prospectar` is the workflow's. A new phase is a
`SKILL.md` like any other, with `mode` in its frontmatter and, optionally, `tools`.

Then register it:

```bash
~/.config/alfred/bin/register.sh
```

or `./install.sh workflows` from the Alfred clone. Restart the agent.

## Where they live, and why they survive updates

```
~/.config/alfred/workflows/          shipped: the installer writes here
~/.config/alfred/custom/workflows/   yours: the installer never writes here
```

`./install.sh update` refreshes the shipped files and regenerates every command from both
directories. It does not read, write or delete anything under `custom/`. Your workflows
are still there, and their commands are regenerated with the rest.

The one thing an update can affect is a shared phase your workflow reuses. If Alfred renames
or removes it, registration stops and names the phase and the workflow, and `doctor`
reports the same. Nothing fails silently on the first run.

## Naming

Lowercase letters, digits and dashes. The name is the directory, the `name` field and the
command. Names already taken: the shared phase names, `alfred`, `manage`, `worktree`,
`add-workflow`. A custom workflow with the name of a shipped one is an error, not an
override.

## Models

Shared phases use the model assigned in the profile, as always. A workflow's own phase uses
the entry `<workflow>-<phase>` in the profile, for example `ventas-prospectar`, or the
orchestrator's model when there is none. Edit `~/.config/alfred/profile.json` and run
`./install.sh workflows`.

## In OpenCode

Each workflow is a primary agent, `alfred-<name>`, next to `alfred` for the default one.
Press Tab to pick it.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `/alfred-<name>` does not appear | not registered yet, or the agent was not restarted; run `./install.sh workflows` and restart |
| `name 'x' does not match its directory 'y'` | the `name` field and the directory disagree; make them equal |
| `name 'x' is reserved` | the name is a shared phase or an existing command; pick another |
| `phases not found in the workflow's skills/ nor in the shared library` | a phase has no `SKILL.md` anywhere; add it or fix the name |
| `route 'r' uses phases not declared` | a route names a phase missing from `phases`; add it there |
| `workflow 'x' is defined twice` | a custom workflow has a shipped name; rename the custom one |
