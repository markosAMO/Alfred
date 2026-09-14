---
tracker: none
---

# No-tracker adapter

The default. Tasks live in `docs/changes/{change}/tasks.md` and nowhere else.

## Operation mapping

| Contract | Behaviour |
|---|---|
| `fetch_task` | unavailable; work enters through `refine` or `diagnose` |
| `create_task` | no-op, the task is already in `tasks.md` |
| `create_subtask` | no-op |
| `update_task` | no-op, status lives in `.alfred/state/` |
| `close_task` | no-op |
| `link_tasks` | no-op, dependencies are declared in `tasks.md` |

## What is lost

Visibility for people who do not read the repository. Everything the pipeline needs is in
the file: the tasks, their dependencies, their scenario coverage and their status.

## Adding a tracker later

Configure a backend and the next `tasks` run mirrors its tasks. Changes already completed
are not backfilled, because a board full of closed cards for work nobody tracked is noise.
