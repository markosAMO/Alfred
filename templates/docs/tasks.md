---
change: {change}
phase: tasks
parent: {workspace:change or absent}
---

# {Change title}

## Tasks

- [ ] 1. {task}
      covers: [{scenario}, {scenario}]
      depends_on: []
      files: {paths}
      external_id: {tracker id, when configured}

## Dependency order

```
1, 2 in parallel
3 after 2
```

## Scenario coverage

| Scenario | Task |
|---|---|

Every scenario in the specification appears exactly once. A scenario with no task is not
implemented; a scenario with two is implemented twice.

## Handoff

{Where the next phase starts: paths with lines, commands with what they printed, what was ruled out, what is still open. Fifteen lines at most, or `none`.}
