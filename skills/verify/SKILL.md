---
name: verify
mode: auto
skippable: false
reads: [spec, tasks, conventions, code]
writes: [verify_report]
document: docs/changes/{change}/verify-report.md
next: [review]
---

# verify

Answer one question: does the implementation satisfy the specification?

Not whether the code is good. That is `review`, and it runs separately for a reason.

## Runs alongside review

`verify` and `review` are dispatched together. Neither reads the other's report, and
neither writes code — each writes only its own document, so there is nothing to collide
over.

They also answer independent questions. A change can satisfy its specification and still be
badly written, or be well written and miss a scenario. Running them in sequence would make
the second wait on an answer it does not use.

One consequence worth knowing: a failing verification no longer cancels the review. Both
reports arrive, so a return to `apply` carries the review findings as well, and the same
code is not revisited twice.

## Method

Scenario by scenario, from the specification. Not file by file, and not from the task list.

```
for each scenario in alfred/{change}/spec
  is there a test that covers it?
  does that test actually assert the scenario's outcome?
  does it pass?
```

Working from the specification rather than from what was built is what makes this phase
capable of finding something missing. A pass built by reading the code confirms that the
code does what the code does.

## Tests that do not test

A test that passes proves nothing on its own. Each test claimed to cover a scenario is
checked for whether it asserts that scenario's stated outcome.

```
scenario says   THEN no session is created
test asserts    the response status is 200
                -> does not cover the scenario
```

The failure scenarios matter most here. They are the ones most often written to pass rather
than to check, because the code path they exercise is the one nobody ran by hand.

## Coverage

Coverage is measured over the lines the change introduced or modified, per
`skills/_shared/testing-protocol.md`. The requirement is 100% of those lines, not of the
repository.

An uncovered line belonging to the change fails this phase and is reported with the
scenario that should have exercised it.

## Running

Run the commands named in `docs/code_conventions.md`. Report each as it was observed:

```
bundle exec rspec: 47 examples, 0 failures
bundle exec rubocop: no offences
```

A command that cannot run is reported as such, never assumed to pass. An environment
failure is not a verification result.

## What it writes

```markdown
## Result
## Scenario coverage
## Coverage
## Commands run
## Findings
```

`Scenario coverage` lists every scenario with its verdict and the test that covers it. This
is the table the user reads, and the one `archive` checks before merging the delta.

`Findings` names what failed, with the scenario it belongs to. A finding without its
scenario cannot be routed back to the task that owns it.

## Failing

This phase does not fix anything. It reports.

A failed verification returns the change to `apply` with the failing scenarios named, and
only the tasks that claim them are re-dispatched. Re-running everything discards work that
passed.

## Completion

Follow `skills/_shared/phase-protocol.md`: write the document, index it under
`alfred/{change}/verify-report`, update state, notify `phase_completed` or `error`.

```
verify: 7 of 7 scenarios covered, 47 tests passing, coverage 100%
```
