# Phase contracts

A skill's prose tells a phase what to do. A contract states where it may write, and a hook
refuses what it may not, so the rule does not depend on the agent obeying it.

## The file

`contract.json` sits next to the phase's `SKILL.md`. A skill with none is still guarded by
the fixed rules below.

```json
{
  "permissions": {
    "filesystem": {
      "write": [".alfred/changes/{change}/**", "docs/specs/**"],
      "deny_write": ["docs/specs/archive/**"]
    }
  }
}
```

A contract may also say what the phase must leave when it stops:

```json
{
  "outputs": [".alfred/changes/{change}/spec.md"],
  "result": {
    "required": ["change", "status", "summary"],
    "status": ["completed", "failed", "blocked"]
  }
}
```

| Key | Meaning |
|---|---|
| `permissions.filesystem.write` | the only paths the phase may write; empty or absent allows every path |
| `permissions.filesystem.deny_write` | paths the phase may never write; wins over `write` |
| `outputs` | files that must exist when the phase stops; one exact path each, no pattern |
| `result.required` | keys the result envelope must hold |
| `result.status` | the values `status` may take; empty accepts any |

Patterns are relative to the repository root and separated by `/`. `**` matches any number
of segments, `*` and `?` match within one, and `{change}` matches the one segment that is
the running change's directory. Unknown keys, absolute paths, `..` and any other
placeholder are refused.

`{change}` is always a whole segment, never part of one:

```
.alfred/changes/{change}/spec.md    accepted
quotes/{change}/quote.md            accepted
quotes/{change}.md                  refused
```

A contract is optional, and so is every block in it. Each block is validated and enforced
on its own, and none requires another. A block that is absent leaves the phase doing what
it does without a contract, so a skill that never declares one runs exactly as before.

A repository override in `.alfred/skills/<phase>/` with no `contract.json` inherits the
installed skill's, so replacing the prose never lifts a rule by omission. An override that
means to drop the rules declares an empty contract, `{}`.

## When the phase stops

A contract with `outputs` or `result` gives the phase's agent a stop hook. It reads the
result envelope from the phase's final message, per `skills/_shared/subagent-protocol.md`,
and checks every required key and the status. Outputs are checked unless the phase
reports, with a valid status, that it stopped short: a phase that says it is `blocked` or
`failed` owes none, and is never sent back to produce a file it could not write. An
invalid status is refused together with any missing output. `{change}` in an
output is read from the envelope's `change`. When something is missing it refuses the stop once, naming what, and
the phase keeps working to fix it. A phase already continuing from a refusal is let through
with its result as it is, so a phase that cannot comply ends rather than loops.

## When it is read

At registration, never during a run. Registration resolves which `SKILL.md` a phase uses,
reads the `contract.json` beside it, refuses the workflow when the contract is invalid, and
renders the patterns into the hook command of the phase's agent. A contract edited while a
change runs changes nothing until registration runs again, which is the same guarantee the
workflow definition has.

## Changing a contract

A contract takes effect when its workflow registers again:

| Where the contract is | Run |
|---|---|
| a skill Alfred ships, edited in the clone | `./install.sh update` |
| a workflow of the machine, under `custom/workflows/` | `register.sh` or `/alfred-workflows-scanner` |
| a workflow of a repository, under `.alfred/workflows/` | `register.sh --project <repo>` |
| the installed copy, edited by hand | `./install.sh workflows` or `register.sh` |

A contract deleted from the clone is removed by `update` unless it was edited where it is
installed, and the phase registers again with none.

An invalid contract refuses its workflow. A workflow that registered before keeps that
registration, contract included, until the file is fixed, and `update` refuses a release
that would leave a registered workflow refused. A typo never removes a command or loosens a
rule.

## The fixed rules

Whatever a contract says, no phase may write:

```
.alfred/skills/**  .alfred/workflows/**  .alfred/config.yaml  .alfred/generated.json
.alfred/skill-registry.md  .claude/agents/**  .claude/commands/**  opencode.json
the Alfred installation, ~/.claude/agents, ~/.claude/commands, ~/.config/opencode
```

These are the files a phase could rewrite to change the rules it, or a later phase, runs
under.

## What it does not cover

- **Writes made through a shell.** No agent intercepts a command before it runs, so a
  `Bash` call that writes a file is not refused. A diff of what a change touched belongs to
  a step after the phase.
- **OpenCode.** It has no per-agent hook, so its agents are generated without the guard
  and run as though no phase declared a contract.
- **A repository override of a shared phase.** The machine's agent for a shared phase
  carries the contract of the installed skill; a contract in `.alfred/skills/<phase>/`
  applies only to the repository's own workflows.
