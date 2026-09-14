---
change: {change}
phase: design
parent: {workspace:change or absent}
---

# {Change title}

## Approach
The chosen design, stated so a reader can check it against each requirement.

## Components affected

## Interfaces
Contracts that change or appear: signatures, endpoints, events, message shapes.
`tasks` reads this section to order the work.

## Data
Schema, migrations, what is stored and for how long.

## Trade-offs considered
One line per rejected alternative and why. Without this the next change reopens the
question.

## Risks
What could go wrong with this approach, and the mitigation.

## Out of scope
What this design deliberately does not solve, so `review` does not report it as missing.
