# Testing protocol

What deserves a test. How tests are run belongs to the target project's
`docs/code_conventions.md`, because it depends on the stack.

## Always tested

**Both paths, never one without the other.** A feature that only proves it works under
correct input has not been tested, it has been demonstrated.

```
POST /payments
  with valid params      -> creates the payment, returns 201
  with invalid params    -> rejects, returns 422, creates nothing
  with missing auth      -> returns 401
```

- One test per `GIVEN/WHEN/THEN` scenario in the spec, named after the scenario.
- Business logic and domain rules.
- Edge cases named in the spec.
- Fixed bugs: the test that reproduces the bug is written first and must fail before the
  fix exists.
- User interface.

## Shared code is tested through a caller

A unit tested only on its own is tested against the types its author had in mind. Its
callers are what the system actually passes it.

```
helper.custom_sender(channel)
  tested with  "in_app"   passes
  called with  :in_app    raises
```

That is a real run. A helper was corrected, tested with a string, and called by a worker
that passes a symbol. The test passed, the review passed, and the defect arrived from
outside the pipeline — which is the part that matters: both quality gates were blind to it
in the same way.

So a change to code other code calls — a helper, a concern, a shared signature — is
exercised from at least one real call site, with the types that call site passes, and
without stubbing the chain under test. Stubbing the resolution turns the test into an
assertion that the stub works.

One caller is enough. The point is not coverage of every caller, it is that the test has
touched the real one once, which is what tells you the contract holds outside the
laboratory.

Finding the callers is the change's job, not the test's: `design` traces them and `tasks`
lists them among the files a task touches. A caller nobody found is the defect this rule
exists for.

## Never tested

- Framework behaviour. That the router routes is not your test.
- Getters and setters without logic.
- Third-party libraries.
- Configuration.

## External services

Always mocked. A test that reaches a real external service is not a test: it fails when
the network fails, it passes for reasons unrelated to the code, and it cannot exercise the
failure path on demand.

The mock covers both directions: the expected response and the failure the service can
return.

## Coverage

**100% of the requirement. Not 100% of the repository.**

Coverage is measured over the lines the change introduced or modified, as described by the
spec. Adding authentication to a codebase with 40% overall coverage requires the new
authentication code to be fully covered; it does not require covering ten years of
pre-existing code.

`verify` fails when a line belonging to the requirement is uncovered.

## Test naming

Tests are named after the scenario they come from, so a failing test points at the
requirement it broke rather than at a function name.

```
scenario in spec:  "rejects a payment with an expired card"
test name:         rejects a payment with an expired card
```

When a test fails, the spec that defines the expected behaviour is one search away.
