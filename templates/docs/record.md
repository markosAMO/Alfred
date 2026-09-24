---
change: {change}
type: {feature|bug}
closed_at: {ISO8601}
commit: {sha}
outcome: {completed|closed_incomplete}
---

# {change} — {one line, what this change delivers}

## What changed

One paragraph, in the present tense, for someone who was not here. What the system does now
that it did not do before, in the terms a user of the system would use.

## Why

The problem this solved, from the proposal or the diagnosis. Not the process that found it.

## How it was decided

The approach that was taken, and the alternatives that were rejected with the reason each
was rejected.

This section is why the record exists. It is the only part of the design that survives the
change, and without it the next change reopens a decision that was already settled.

## Requirements

What the delta added, modified or removed, as one line each. The full text is in the delta
spec beside this record, and merged into the master specifications.

```
ADDED     Google sign-in
MODIFIED  Session expiry, 7 days -> 30 days
REMOVED   Password reset by security question
```

## Result

```
verify   {n}/{n} scenarios, {n} tests
review   {n} findings, {n} blocking
```

Anything unresolved at close is named here, not omitted. A record that reports a clean
close for work that was not finished sends the next reader to redo it or to trust something
that is still open.

## Where the rest is

The working documents of this change — proposal, research, design, tasks and the two
reports — were removed at close under `artifacts.retain: final_only`.

```
memory   alfred/{change}/{artifact}
```

They were removed before the closing commit was staged, so git never held them. Memory is
the only copy, and where no backend was configured there is none: what a later reader needs
was lifted into this document first, which is the trade the mode makes.

{omit this section entirely under artifacts.retain: all}
