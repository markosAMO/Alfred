# Phase protocol

Every phase follows the same cycle. A phase that skips a step breaks the phases downstream
that depend on it.

## The cycle

```
1  load state          read .alfred/state/{change}.yaml
2  recover context     context() and recall() for what is already known
3  read inputs         fetch() the artifacts this phase depends on
4  do the work         the part that differs between phases
5  write the document  at the locator the orchestrator passed, below
6  update state        mark the phase completed in .alfred/state/{change}.yaml
7  notify              phase_completed, or error
```

Steps 1 and 6 name the file because a phase has no other way to know it. There is one state
file per change, named for the change and never for the phase, and when the change runs in a
worktree it is that worktree's copy. Its shape, and the fields the first phase of a change
records from what the orchestrator passed it, are in `skills/_shared/state-contract.md`.

A phase that invents its own name breaks the only thing state exists for: `continue` looks
the change up by name, and a file named after a phase is a file it will never find.

Step 3 and step 5 both work from **locators**, which the orchestrator resolves and passes
in. A locator is either a path or a memory key, already decided:

```
keep, ephemeral    docs/changes/login-google/spec.md      a path
pointer            alfred/login-google/spec               a key
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
verify: 7 of 7 scenarios covered, 47 tests passing, coverage 100%
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

## Inputs and outputs

A phase declares what it reads and what it writes in its `SKILL.md` frontmatter. It reads
nothing else.

`document:` names the artifact, in every mode. It is what the orchestrator resolves the
locator from: a path under `keep` and `ephemeral`, a key under `pointer`, per the naming
rules in `memory/CONTRACT.md`. A phase never has two identities depending on configuration,
and never resolves its own.

A phase that reaches for an artifact it did not declare creates a hidden dependency that
breaks when phases are skipped.

## Skipping

A phase marked `skippable` may be absent. Downstream phases handle a missing input by
falling back to what exists, never by failing.

```
spec with no proposal      the requirement comes from the user's request directly
design with no research    proceed with what architecture.md already states
```

## Phases that run together

Two phases may be dispatched at once when neither reads what the other writes and their
outputs are different files. `verify` and `review` are the pair this applies to: both read
the code, neither writes it, and each writes its own report.

Every other pair in the pipeline is sequential, because each one reads the document the
previous one wrote.

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
finds every decision already made — by `refine`, by the architecture, by the conventions —
reports what it decided and proceeds. Manufacturing a question to satisfy the mode costs a
round trip and teaches the user that the gates are ceremony, which is what makes them skip
the one that mattered.

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

Most of them should not be here at all. `refine` exists to settle what is being asked for
before the pipeline commits to it, so a decision a later phase discovers is usually one
`refine` could have levied — and a run that leaks decisions out through `research`, `spec`
and `design` one at a time has turned a phase that batches by design into three that do not.

Waiting assumes the session the phase runs in is the one the user is looking at. When it is
not — a change running in its own session, per `skills/_shared/worktree-protocol.md` — the
phase does not wait. It records the question in state, returns it, and ends; the answer
arrives as a fresh dispatch. A phase never reaches past its own session to find the user:
a subagent's message is sent under its session's address and the reply is delivered there,
so a phase that asks directly waits for something that cannot come back to it.
