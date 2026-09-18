---
name: diagnose
mode: interactive
entry_point: bug
reads: [memory, architecture, master_specs, code, tests]
writes: [diagnosis]
document: docs/changes/{change}/diagnosis.md
routes_to: {behaviour_changed: spec, default: design}
---

# diagnose

Find why something fails. Not what fails, and not how to fix it.

This phase writes no fix. Understanding the cause and choosing the remedy are separate, and
merging them produces a change that makes the symptom disappear.

## Materialise the inputs first

A defect usually arrives with material attached: a ticket, an error report, a log, a
conversation. Read each one once and write it to `docs/changes/{change}/inputs/` in full,
per `skills/_shared/external-inputs.md`.

Logs and stack traces are materialised verbatim. A summarised trace is a trace with the
line that mattered removed.

## Search memory first

```
recall the symptom, in the terms someone would search for
```

Postmortems are written by `archive` for exactly this moment. A class of failure is
diagnosed once: a timeout in payments and a timeout in notifications caused by the same
missing backoff is one investigation, not two.

A match is a lead, not a conclusion. It is confirmed against this failure before being
acted on.

## Reproduce

This phase is `interactive` because reproduction usually needs the user.

```
ask("How do you reproduce it?",
    ["exact steps", "a failing request or payload", "when it started", "who it affects"])
```

A failure that cannot be reproduced cannot be confirmed fixed. Where reproduction is
impossible, say so and record what evidence exists instead: logs, traces, reports. A
diagnosis built on an unreproduced failure is a hypothesis, and is labelled as one.

Write the failing test before the cause is known, when possible. It is the reproduction,
and `apply` will require it anyway per `skills/_shared/testing-protocol.md`.

## Root cause, not symptom

Keep asking why until the answer stops being a consequence of something else.

```
symptom        the payment endpoint times out
because        the retry loop never exits
because        there is no backoff and no attempt limit
because        the retry helper was written for a service that could not fail
root cause     the retry helper has no termination condition
```

Stopping at the first line produces a timeout raise. Stopping at the last one fixes every
caller of that helper.

Where a root cause would take longer to reach than the failure allows, record the symptom
fix as deliberate, with the cause still open. A workaround recorded as a workaround can be
revisited. One recorded as a fix cannot.

## Routing

```
does the correct behaviour differ from what the master specs describe?
  yes  -> spec, as a delta, then design
  no   -> design directly
```

Behaviour was always meant to work this way and did not: the code was wrong, the
specification was right, nothing to write. Behaviour is now meant to differ: that is a
requirement, and it is written down.

The decision is stated and confirmed, per `workflows/sdd/rules.md`.

## What it writes

```markdown
## Symptom
## Reproduction
## Investigation
## Root cause
## Affected scope
## Correct behaviour
## Confidence
```

`Affected scope` names everything else reaching the same cause. A root cause found in one
caller usually has others, and they fail next week.

`Correct behaviour` is what should happen instead, stated concretely enough for `spec` or
`design` to work from.

`Confidence` is `confirmed` when reproduced and proven, `probable` when the evidence is
consistent but unreproduced. It carries into the postmortem, so a fix that was a good guess
is not remembered as a certainty.

## Completion

Follow `skills/_shared/phase-protocol.md`: write the document, index it under
`alfred/{change}/diagnosis`, update state with the chosen route, notify `phase_completed`.
