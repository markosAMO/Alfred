# Phase protocol

Every phase follows the same cycle. A phase that skips a step breaks the phases downstream
that depend on it.

## The cycle

```
1  load state          read .alfred/state/{change}.yaml
2  read inputs         every locator the orchestrator passed, all at once, Handoff first
3  recall              each memory type the orchestrator passed, for earlier changes
4  do the work         the part that differs between phases
5  write the artifact  at this phase's own locator, Handoff included, below
6  update state        mark the phase completed in .alfred/state/{change}.yaml
7  notify              phase_completed, or error
```

These rules hold for every phase of every workflow, shipped or written by hand. A skill says
how to do its work; it never says what to read or where to write, because that is the
workflow's declaration and the orchestrator passes it.

## Artifacts

Every phase leaves exactly one artifact, named after the phase: `design` leaves `design`, a
workflow's own `cotizar` leaves `cotizar`. The name is not configurable, so any phase can
address another's output without being told what it is called.

What a phase reads is configurable, and it is the only thing that is. The running workflow
declares it per phase in its `workflow.json`, as `reads` (the artifacts of other phases, by
phase name, and the project artifacts `architecture`, `conventions` and `specs`) and
`recall` (memory types to search for what earlier changes concluded). Registration validates
both and renders them into the command, and the orchestrator passes one locator per read.

A phase reads what it was handed and nothing else. Reaching for an artifact the workflow did
not declare is a hidden dependency that breaks under the next workflow that reuses the
phase.

## Starting from what the last phase left

A phase does not rediscover what the phases before it already found. Step 2 reads the
`## Handoff` section of every artifact it was handed before anything else, and treats it as
given: the files and lines named there are where to look, the commands are how to check,
and what is ruled out stays ruled out. The phase explores only what the handoffs leave
open.

Every locator is read in one parallel batch. Reading them one at a time costs a round trip
per artifact to learn nothing the batch would not have.

Step 3 recalls each type the orchestrator passed, with the area of the change as the query,
and fetches the full entry of every candidate it acts on: a preview is for choosing, never
for acting. A recall that returns nothing is a normal result.

## The handoff

Every artifact ends with a `## Handoff` section, written for the phases that read it. It is
what this phase learned that the next one would otherwise pay to learn again:

```markdown
## Handoff
code       app/auth/session.rb:40-88       where the token is issued; the change goes here
           spec/auth/session_spec.rb       existing scenarios, reusable
verified   bundle exec rspec spec/auth     31 examples, 0 failures (baseline)
ruled out  the OmniAuth middleware         it never touches the token
open       whether the migration is its own task
```

Paths with lines, commands with what they printed, and the dead ends, so nobody walks them
twice. Fifteen lines at most: a handoff that restates the artifact above it is one nobody
reads. A phase that found nothing worth handing on writes `none`, which tells the next
phase that the artifact is the whole story.

Steps 1 and 6 name the file because a phase has no other way to know it. There is one state
file per change, named for the change and never for the phase, and when the change runs in a
worktree it is that worktree's copy. Its shape, and the fields the first phase of a change
records from what the orchestrator passed it, are in `skills/_shared/state-contract.md`.

A phase that invents its own name breaks the only thing state exists for: `continue` looks
the change up by name, and a file named after a phase is a file it will never find.

Step 2 and step 5 both work from **locators**, which the orchestrator resolves and passes
in. A locator is either a path or a memory key, already decided:

```
keep, ephemeral    .alfred/changes/login-google/{phase}.md    a path
pointer            alfred/login-google/{phase}             a key
```

The phase reads the file when the locator is a path and the entry when it is a key, and
writes back the same way. It does not read `memory.documents`, does not detect the mode and
does not branch on it. See `Locators` in `memory/CONTRACT.md`.

Step 5 is two writes, in an order the locator's shape decides:

```
a path   write the file from the template, then remember() it under its key
a key    remember() it under its key, then append its row to the address file
```

A phase does not choose between them, so a change is never half in memory and half on disk.
A locator the orchestrator reports as `<unresolved>` is an artifact that does not exist: the
phase reports a blocker and stops, rather than looking for the other mode's copy.

## Ordering rules

**The durable side is written before the thing that references it.** Under `keep` the file
comes first and memory follows: a run that dies between them leaves the artifact on disk
and `reindex` recovers the copy, where the reverse loses the artifact. Under `pointer` the
entry comes first and its address follows: an entry nothing names is unreachable and
harmless, where an address naming nothing fails in the phase downstream as though memory
had lost it. One rule, stated twice: never write a reference to something that is not
stored yet. See `memory/CONTRACT.md`.

**The address file is committed by whatever commits the state file.** Under `pointer` they
answer the same question from two sides, and state that outlives its address file resumes a
phase whose input cannot be found.

**State is updated after the document exists.** State claiming a phase finished when its
document is missing is worse than no state: `continue` would skip the phase.

**Notification is last.** The user is told a phase finished only once it actually finished.

## What a completion carries

A phase reports what it did in terms someone can check without asking it again.

```
{phase}: 7 of 7 scenarios covered, 47 tests passing, coverage 100%
commands: bundle exec rspec, bundle exec rubocop
state: .alfred/state/login-google.yaml
context: 91k tokens
```

Numbers as the tool printed them, never rounded into prose. A phase that says the tests pass
has said something nobody can verify; a phase that says `47 examples, 0 failures` has quoted
something that either is or is not in the output.

The state file path is there because it is the one thing that outlives the report. Whoever
is reading — an orchestrator, a coordinator, the user — can look rather than believe, and in
a fleet of changes running in their own sessions that is the only check there is: a
coordinator reads reports it has no way to audit, and its whole picture of the run rests on
them being true.

`context` is the phase's own token cost. It belongs in the line because it is the number
that decides whether a phase is worth what it does, and asking for it afterwards means
asking every phase separately, out of band, for something each one already knew.

## Skipping

A route may leave a phase out. The orchestrator then passes its artifact as `<not
produced>`, and a phase handles that by falling back to what exists, never by failing.

```
a phase whose upstream artifact was not produced
    work from the user's request directly
a phase whose investigation phase did not run
    proceed with what architecture.md already states
```

`<unresolved>` is different: the phase that writes it completed and the artifact cannot be
found. That is a blocker, reported and never worked around.

## Phases that run together

Two phases may be dispatched at once when both conditions hold:

```
neither reads what the other writes
their outputs are different documents
```

Which pairs those are is a fact of the running workflow, declared in its definition as a
group and carried by the command, per `skills/_shared/workflow-protocol.md`. No pair is
named here, and a phase never infers that it runs alongside another from what the same two
names do under some other workflow: the conditions are about what each phase reads and
writes, and a workflow that gives one of them a different job breaks them without renaming
anything.

Every other pair is sequential, because each one reads the document the previous one
wrote. A workflow that declares no group runs every phase on its own, in route order.

Concurrency inside a phase is a different question, decided per task by file overlap. See
`skills/apply/SKILL.md`.

## Failure

A phase that cannot complete leaves state marked `failed` with the reason, writes no
partial document, and raises `error`. A partial document indexed as complete poisons every
phase that reads it afterwards.

## Interaction

A phase in `interactive` mode uses `ask()` and waits. A phase in `confirm` mode uses
`confirm()` before step 4 and stops if the answer is no. A phase in `auto` mode calls
neither, and must not block for input under any circumstance.

`interactive` means a phase **may** ask, not that it must. A phase that reads its inputs and
finds every decision already made — by an earlier phase, by the architecture, by the
conventions — reports what it decided and proceeds. Manufacturing a question to satisfy
the mode costs a round trip and teaches the user that the gates are ceremony, which is what
makes them skip the one that mattered.

The same applies to `confirm`. It exists for the moment work becomes expensive to undo, and
a confirmation of something already confirmed is noise wearing the costume of a safeguard.

A phase reporting that it asked nothing is a phase reporting that nothing was ambiguous.
That is information, and it is cheaper than the question.

A phase asks for everything it needs at once. It reads its inputs, finds every decision it
cannot make, and puts them in one round rather than discovering them one at a time — the
second question of a phase was almost always answerable alongside the first, and asking it
separately spends another round trip to learn nothing new. This is not a reason to guess: a
decision that is the user's stays the user's, and a phase short of one still stops.

A round trip is the expensive unit, not the question. It costs the user's attention, and in
a change running in its own session it also costs three conversations a turn each. Two
questions in one round cost one of those; the same two, asked in sequence, cost two.

Most of them should not be here at all. A workflow whose route opens with a phase that
settles what is being asked for has already bought the round that settles it, so a decision
a later phase discovers is usually one that phase could have levied — and a run that leaks
decisions out one at a time through the phases after it has turned a phase that batches by
design into three that do not.

Waiting assumes the session the phase runs in is the one the user is looking at. When it is
not — a change running in its own session, per `skills/_shared/worktree-protocol.md` — the
phase does not wait. It records the question in state, returns it, and ends; the answer
arrives as a fresh dispatch. A phase never reaches past its own session to find the user:
a subagent's message is sent under its session's address and the reply is delivered there,
so a phase that asks directly waits for something that cannot come back to it.
