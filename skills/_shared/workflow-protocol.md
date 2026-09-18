# Workflow protocol

A workflow is a recipe: which phases exist, in which order they run for each route, and
the question that chooses the route. Phases are the steps, and they are shared: a workflow
reuses the library under `skills/` or brings its own.

Alfred ships one workflow, `sdd`. Every workflow, shipped or custom, becomes a command of
its own, `/alfred-<name>`, generated from the same definition and running the same
orchestrator. `/alfred` is the default workflow, `workflows.default` in the configuration.

## Where workflows live

```
~/.config/alfred/workflows/<name>/            shipped with Alfred, managed by the installer
~/.config/alfred/custom/workflows/<name>/     the user's, never touched by the installer
```

The installer copies `workflows/` from the package and manages it by hash like every other
installed file. It never lists `custom/` in what it installs, so `install` and `update`
neither read, write nor remove anything there. That is the whole preservation mechanism:
a custom workflow survives every update because no update knows it exists.

A name present in both places is an error, not a shadow. A custom workflow that silently
replaced a shipped one would change `/alfred-sdd` on one machine and nowhere else.

## The definition

```
<name>/
  workflow.yaml     the recipe
  rules.md          how to choose between its routes
  skills/           the phases this workflow brings itself, one SKILL.md each
```

`workflow.yaml`:

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

| Key | Meaning |
|---|---|
| `name` | lowercase letters, digits and dashes; equal to the directory name; becomes the command |
| `title`, `description` | shown as the command's help |
| `phases` | every phase the workflow may run, in the order they may run |
| `routes` | at least one; each an ordered list of declared phases |
| `default_route` | the one proposed when the rules do not decide otherwise |
| `routing` | the rules file, relative to the directory |
| `entry_points` | which phase a kind of request starts at |

The file uses a subset of YAML: scalars, flow lists in brackets, and one level of nested
mapping. Nothing else is needed, and nothing else is parsed.

Reserved names: the shared phase names, `alfred`, `manage`, `worktree`, `add-workflow`,
`workflows-scanner`. They are already agents or commands.

## Resolving a phase

```
resolve(prospectar, workflow ventas)
  1  <workflow>/skills/prospectar/SKILL.md     the workflow's own phase
  2  .alfred/skills/prospectar/SKILL.md        repository override of a shared phase
  3  ~/.config/alfred/skills/prospectar/SKILL.md   the shared library
```

A phase named in `workflow.yaml` and found in none of them fails registration and names
the phase. A workflow that registered with a missing step would fail on the first run that
reached it, in a repository, with less context to explain why.

Own phases follow `skills/_shared/phase-protocol.md` like any other, and declare `mode` in
their frontmatter. They may also declare `tools:`; without it they get Read, Write, Glob
and Grep, and the memory operations every phase has.

## Registration

`bin/register.sh` reads both roots and generates, for every agent on the machine:

```
/alfred-<name>                   Claude Code command; primary agent alfred-<name> in OpenCode
alfred-<phase>                   one subagent per shared phase, as before
alfred-<name>-<phase>            one subagent per own phase, so names never collide
/alfred                          the default workflow, unchanged
/alfred-add-workflow             the interview that creates a custom workflow
/alfred-workflows-scanner        rescan both roots and bring the commands up to date
```

It runs from `install`, `update`, `./install.sh workflows`, at the end of `add-workflow`,
and from `/alfred-workflows-scanner`, which exists so the scan can be run alone from inside
an agent. Every run compares what it would generate with what is on disk and reports, per
agent, what was added, updated, removed and left unchanged; `--dry-run` reports without
writing. A second run reports that nothing changed. Files it generated for a workflow that
no longer exists are removed, so a deleted workflow does not keep a command. `doctor`
reports a workflow without a command.

The same tool serves both agents. In Claude Code the scanner is the slash command
`/alfred-workflows-scanner`; in OpenCode it is the command of the same name under
`~/.config/opencode/commands/`, run by the `alfred-manage` agent. Both delegate to the tool
and relay its report.

The model of an own phase is `<name>-<phase>` in the profile, or the orchestrator's when
not assigned.

## The command

Each command is the orchestrator with a workflow section: the phases, the routes as the
only routes, the default, the path of the rules, the entry points, and which subagent runs
each phase. The orchestrator reads the rules before proposing a route, and that is the
only file it reads beyond configuration, state and registry.

The section also names the other installed workflows. A request that belongs to one of
them is answered by saying so, not by running it in the wrong recipe.

## Adding a workflow

`/alfred-add-workflow` interviews: name, title, phases reused and new, the mode of each new
phase, routes and default, the routing question. It shows the resulting `workflow.yaml`,
waits for a yes, and delegates the writing to `alfred-manage`, which creates the directory
under `custom/workflows/` from `templates/workflow/`, one `SKILL.md` per new phase with what
was said as its starting point, and runs the register tool.

The interview writes nothing itself: it has no tools for it, and the files come out of the
templates the same way every time.

## Updating Alfred

`update` replaces shipped workflows whose hash still matches what was installed, reports
the ones edited by hand, and regenerates every command from both roots. Custom workflows
are not looked at.

The one thing an update can break is a shared phase a custom workflow reuses: renamed or
removed, it is no longer resolvable. Registration fails and names the phase and the
workflow, and `doctor` reports the same. It does not fail silently on the first run.
