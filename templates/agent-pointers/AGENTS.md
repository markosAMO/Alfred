<!-- ALFRED:BEGIN — managed by alfred, do not edit by hand -->
## Workflow

This repository uses Alfred. Software work follows the route of the workflow it runs under;
it does not start with code.

### Before anything

Read `.alfred/config.yaml`, then `docs/architecture.md` and `docs/code_conventions.md`.
Resolve skills through `.alfred/skill-registry.md`; never read a skill by guessing its path.

### Routing

The routes are the running workflow's, not this repository's: its command carries them and
its rules file says which one a request deserves. This file declares none.

Choose one of the routes the command carries, state the signals it was based on, and wait
for the user to accept it. Never lengthen a route without saying so. The user may always
shorten it. Asked for a route the workflow does not declare, say so and name the ones it
does.

### Delegation

Each phase runs as a subagent with an empty context, receiving paths rather than content.
The orchestrator holds five things — the request, this configuration, the pipeline state,
the skill registry and the running workflow's rules file — and does no work inline.

### State

`.alfred/state/{change}.yaml` records the phase, the route and the channel the run started
from. `continue` resumes from it. State is updated only after a phase's document exists.

### Language

Neutral English, in documents and in messages alike, regardless of the language the
request was written in. No persona and no regional voice.

### Commits

Conventional commits, no AI attribution of any kind, one commit per change once the
checks the running workflow declares for it have passed — its command names them, this
file does not — staging only the files the work reported.
<!-- ALFRED:END -->
