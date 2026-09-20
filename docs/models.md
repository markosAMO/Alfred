# Model assignment

Every phase runs on its own model. Alfred ships with no assignment: profiles are created
during `./install.sh install`, which asks which model runs each phase and writes the result
to `~/.config/alfred/profile.json`.

The profile lives on the machine, not in the repository, because which models are available
is a property of the machine and its credentials. A repository committed with a profile
naming models a colleague cannot reach would fail for them and not for you.

A run with no active profile stops and asks for setup rather than falling back to a
default. A silently chosen model is a cost and quality decision made without the user.

No skill names a model. A skill describes what the phase does; the profile decides what
executes it.

## Why phases differ

Phases do not ask the same thing of a model.

| Demand | Phases | Needs |
|---|---|---|
| Judgement under ambiguity | `refine`, `spec`, `design`, `diagnose` | the strongest model available |
| Adversarial reading | `review` | a model that did not write the code, and is strong enough to disagree with it |
| Bounded execution | `apply`, `verify`, `tasks` | competence and consistency, not brilliance |
| Mechanical transformation | `archive`, the worktree coordinator | the cheapest model that is reliable |

Running everything on the strongest model is expensive without being better: `archive`
merges a delta into a file, and no amount of reasoning improves that.

Running everything on the cheapest is worse than it looks: a weak `spec` produces
requirements that read fine and verify nothing, and every phase downstream inherits the
error.

## Profiles

A profile is a named set of assignments. Setup creates at least one and makes it active.
More can be added later and switched between without touching any skill.

A common split, shown as illustration rather than as a default:

```json
{
  "orchestrator": "<provider/model>",
  "coordinator": "<cheap model>",
  "phases": {
    "refine": "<strong hosted model>",
    "spec": "<strong hosted model>",
    "design": "<strong hosted model>",
    "diagnose": "<strong hosted model>",
    "review": "<strong hosted model>",
    "apply": "<local model>",
    "verify": "<local model>",
    "tasks": "<local model>",
    "archive": "<cheap model>"
  },
  "memory_tool_prefix": "mcp__engram__",
  "effort": {"spec": "high", "archive": "low"},
  "extra_tools": {"refine": ["mcp__atlassian__getJiraIssue"]}
}
```

`effort` and `extra_tools` are optional and per phase: effort is the second cost lever
after model choice, and extra tools are for whatever a phase needs beyond the base set —
a tracker's read tool in `refine`, for instance, which is stack-specific and therefore not
shipped.

Moving bounded execution onto locally hosted models and keeping judgement hosted follows
what local models are actually good at: given a precise specification and one task, they
execute well; asked to decide what should be built, they drift.

Keep `review` on a model at least as strong as the one that wrote the code. A reviewer
weaker than the implementer approves everything, and the phase becomes a rubber stamp.

## The coordinator is not an orchestrator

`orchestrator` is what runs a change: it chooses a route, delegates each phase and decides
what to do with what comes back. `coordinator` is what runs `/alfred-worktree`: it starts a
session per change and moves messages between them and the user. It proposes no route, reads
no document and decides nothing, so it belongs with `archive` rather than with the phases it
delegates to — and it is the largest single cost in a worktree run, because relaying is
cheap per turn and it takes more turns than anything else.

The sessions it starts run `orchestrator`, not `coordinator`. Each of them is an ordinary
orchestrator; the coordinator is cheap precisely because every judgement it carries was made
somewhere else. Its start command names the model for that reason: a session started in the
background never passes through the orchestrator command, so without it the session would
run on whatever the machine defaults to, which is a model nothing in the profile chose.

A profile written before this key existed falls back to `orchestrator`, which is what those
runs already did.

## Changing assignments

Run `./install.sh models`, or edit `~/.config/alfred/profile.json` and run
`./install.sh update`. Either way the agent definitions are regenerated for every agent on
the machine, so the change takes effect everywhere without editing any agent configuration
by hand.
