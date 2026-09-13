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
