# Model assignment

Every phase runs on its own model. Assignments live under `models` in
`alfred.config.yaml` and are grouped into named profiles.

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

```yaml
models:
  active_profile: frontier
  profiles:
    frontier: ...
    local: ...
```

`frontier` runs everything on hosted models.

`local` moves bounded execution onto models running on the machine and keeps judgement
hosted. The split follows what local models are actually good at: given a precise
specification and a task, they execute well; asked to decide what should be built, they
drift.

`review` stays hosted in both profiles. A review that cannot disagree with the
implementation is a rubber stamp.

## Adding a profile

Add an entry under `profiles` and switch `active_profile`. The installer regenerates the
agent definitions for every agent on the machine, so the change takes effect everywhere
without editing any agent's configuration by hand.
