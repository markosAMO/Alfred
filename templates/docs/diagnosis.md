---
change: {change}
type: bug
phase: diagnose
---

# {Symptom, in the words someone would search for}

## Symptom
What was observed, by whom, since when.

## Reproduction
Exact steps, or the evidence available when it cannot be reproduced.

## Investigation
The chain of causes, each one a consequence of the next.

## Root cause
Where the chain stops being a consequence of something else.

## Affected scope
Everything else reaching the same cause. Usually more than the reported symptom.

## Correct behaviour
What should happen instead, concrete enough for `spec` or `design` to work from.

## Confidence
`confirmed` when reproduced and proven. `probable` when the evidence is consistent but the
failure was not reproduced.

## Route
`spec` when the correct behaviour differs from the master specifications, `design` when it
does not.
