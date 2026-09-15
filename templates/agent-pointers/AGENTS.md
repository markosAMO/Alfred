<!-- ALFRED:BEGIN — managed by alfred, do not edit by hand -->
## Workflow

This repository uses Alfred. Software work follows the pipeline; it does not start with
code.

### Before anything

Read `.alfred/config.yaml`, then `docs/architecture.md` and `docs/code_conventions.md`.
Resolve skills through `.alfred/skill-registry.md`; never read a skill by guessing its path.

### Routing

Choose a route and state the signals it was based on, then wait for the user to accept it.

```
behaviour unchanged        direct     apply · verify · review
behaviour changes          pipeline   spec · design · tasks · apply · verify · review · archive
request underspecified     full       refine · research · then the pipeline
defect reported            diagnose   then spec or design
```

Never lengthen a route without saying so. The user may always shorten it.

### Delegation

Each phase runs as a subagent with an empty context, receiving paths rather than content.
The orchestrator holds only the request, this configuration, the pipeline state and the
registry, and does no work inline.

### State

`.alfred/state/{change}.yaml` records the phase, the route and the channel the run started
from. `continue` resumes from it. State is updated only after a phase's document exists.

### Language

Neutral English, in documents and in messages alike, regardless of the language the
request was written in. No persona and no regional voice.

### Commits

Conventional commits, no AI attribution of any kind, one commit per change once `verify` and
`review` pass, staging only the files the work reported.
<!-- ALFRED:END -->
