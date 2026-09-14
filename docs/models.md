# Model assignment

Every phase runs on its own model. Alfred ships with no assignment: profiles are created
during `alfred install`, which asks which model runs each phase and writes the result to
`models.profiles`.

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
| Mechanical transformation | `archive` | the cheapest model that is reliable |

Running everything on the strongest model is expensive without being better: `archive`
merges a delta into a file, and no amount of reasoning improves that.

Running everything on the cheapest is worse than it looks: a weak `spec` produces
requirements that read fine and verify nothing, and every phase downstream inherits the
error.

## Profiles

A profile is a named set of assignments. Setup creates at least one and makes it active.
More can be added later and switched between without touching any skill.

A common split, shown as illustration rather than as a default:

```yaml
models:
  active_profile: mixed
  profiles:
    mixed:
      orchestrator: <provider/model>
      phases:
        refine: <strong hosted model>
        spec: <strong hosted model>
        design: <strong hosted model>
        diagnose: <strong hosted model>
        review: <strong hosted model>
        apply: <local model>
        verify: <local model>
        tasks: <local model>
        archive: <cheap model>
```

Moving bounded execution onto locally hosted models and keeping judgement hosted follows
what local models are actually good at: given a precise specification and one task, they
execute well; asked to decide what should be built, they drift.

Keep `review` on a model at least as strong as the one that wrote the code. A reviewer
weaker than the implementer approves everything, and the phase becomes a rubber stamp.

## Changing assignments

Edit `models.profiles` or rerun setup, then switch `active_profile`. The installer
regenerates the agent definitions for every agent on the machine, so the change takes
effect everywhere without editing any agent configuration by hand.
