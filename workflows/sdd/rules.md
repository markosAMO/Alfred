# Spec-driven development

A request becomes a specification, a design, a task list, code, verification and a delta
merged into the master specifications. This file is the judgement `sdd` applies. The
orchestrator reads it before it proposes a route, and it is the only file of this workflow
it opens while a change is running: the phases, the routes, the default among them, the
entry points, the group dispatched together and the phase that closes a change were read
from `workflow.json` when the command was registered, and the command carries them.

Nothing here restates a structural fact. A route written in both places has two
authorities, and the wrong one is the one nobody re-reads.

## The question

> Does this change observable system behaviour?

Behaviour is what the system does as seen from outside: responses, stored data, messages a
user reads, side effects. Not how the code is arranged.

```
No   renaming a symbol, extracting a function, formatting,
     a dependency bump, a typo in a comment
     -> direct

Yes  a new validation, a changed default, a new endpoint,
     an error message a user reads, a changed response shape
     -> pipeline
```

Counting files is the wrong test. A refactor touching thirty files changes nothing
observable; adding one validation to one file changes behaviour. The count is a proxy for
the question and gets both cases backwards.

## Choosing a route

### direct

The answer is no. There is nothing to specify, because nothing a user can observe is
different afterwards, and writing a requirement for a rename produces a document that says
the system still does what it already did.

What it skips is planning, never verification: a rename still has to compile, pass its
tests and read well, which is why `verify` and `review` sit on it as they sit on every
other route of this workflow.

What it also skips is the delta, because there is none to merge. Master specifications
describe behaviour, and behaviour did not change.

### pipeline

The answer is yes, and the request already says enough to write a requirement from: what
should happen, what should happen when it fails, and which part of the system is affected.

### full

The answer is yes, and the request is underspecified against the signals in
`skills/refine/SKILL.md`. Between `pipeline` and `full` sits nothing structural: `refine`
and `research` are `skippable`, so the difference is which of them the routing decision
includes.

Those signals are checkable, and they are not infallible. A wrongly triggered interview
costs the user four rounds of questions they did not need, which is why the proposal names
the signals it matched and lets them answer the shorter way.

```
This looks like a full run: no failure behaviour stated, more than one
component affected, and the auth strategy is undecided.

Start with the interview, or go straight to spec?
```

## After `diagnose`

`diagnose` ends by asking the question above of the fix it proposes, rather than of the
request that reported the defect.

```
the fix changes observable behaviour    -> spec
anything else                           -> design
```

A fix that changes what a user sees is a behaviour change like any other and needs a
requirement written for it, whatever started it. A fix that does not is the code being
corrected to do what it already claimed, and the specification it is measured against is
the one that already exists.

## How a route changes mid-run

A direct route that turns out to change behaviour stops and proposes the pipeline from
`spec`. Work already done is kept: the change exists, it simply needs a requirement
written for it.

The reverse does not happen. A pipeline run is never silently downgraded, because the
planning artifacts already exist and skipping them mid-run leaves a change half-specified.

## External material

A tracker card, a URL or a document in another system arrives with the request, so the
phase that receives it is the one the request enters at: `refine` for a feature,
`diagnose` for a defect. It is read once there and written to the change's `inputs/` as
text, per `skills/_shared/external-inputs.md`, and no later phase reaches for it again.

Naming one phase per kind of request is what makes that true. Material fetched twice is
material two phases can disagree about, and the second fetch is the one nobody reads.

## Judging the work

`verify` and `review` are the two phases that judge it, and they are what this workflow
dispatches together: neither reads what the other writes, neither writes code, and they
answer independent questions. `verify` asks whether the change satisfies its
specification; `review` asks whether the code is sound. Those stay in separate agents
because an agent reviewing its own output approves its own assumptions.

The orchestrator relays what they return and does not read the diff to form an opinion of
its own. Judging the work is their job, and an orchestrator duplicating it pays for the
second opinion in context the rest of the run needs.

## What memory is for here

`diagnose` looks for prior postmortems of the area it is in, and `design` for decisions
already taken about it. A backend that is configured and unreachable leaves both finding
nothing and neither saying why, which is why it is checked once before the first phase
rather than discovered halfway.

## Closing a change

`archive` writes the record, merges the delta into the master specifications for a feature
or writes a postmortem to memory for a defect, and removes the change's working documents
under `artifacts.retain: final_only`. The record is written and read back before anything
is deleted.

Removing the state file and closing the worktree are the same under every workflow, per
`skills/_shared/workflow-protocol.md`.

## What this workflow does not handle

`sdd` is for changes to a repository's code and to the documents that describe it. A
request that is work of another kind — its own steps, its own vocabulary, its own idea
of what finishing means — belongs to a workflow written for it, and the orchestrator
names that workflow rather than running the request here.

Alfred installs no other workflow, so on a fresh installation there is nothing to name.
What the orchestrator can say is that this one does not handle the request, which is more
useful than a specification written for something that was never a change.
