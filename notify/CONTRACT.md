# Notification contract

Every skill talks to the user through the operations below and never through a concrete
channel. `notify/adapters/<channel>.md` maps each operation to what that channel actually
does.

A skill never knows whether the user is reading a terminal or a phone.

## Two kinds of operation

`notify` reports and moves on. `ask` and `confirm` stop the pipeline until an answer
arrives. Mixing them is the most common way to build a flow that either spams the user or
hangs waiting for an answer nobody was asked for.

| Operation | Blocks | Used by |
|---|---|---|
| `notify` | no | every phase, on completion or failure |
| `ask` | yes | `interactive` phases |
| `confirm` | yes | `confirm` phases |

## Operations

### notify

```
notify(event, message, change)
```

Reports that something happened. The pipeline continues without waiting. Delivery is
best-effort: an unreachable channel is skipped and never fails the phase.

### ask

```
ask(question, options, default) -> answer
```

Asks an open or multiple-choice question and waits. `options` may be empty for a free-form
answer. `default` is what an unanswered question resolves to after the channel times out;
without a default the phase stays blocked rather than guessing.

### confirm

```
confirm(action, details) -> yes | no
```

Asks for permission to proceed and waits. Used by `apply` before writing code, and by any
phase configured as `confirm`.

## Events

| Event | Raised when | Blocks |
|---|---|---|
| `needs_input` | an `interactive` phase asks a question | yes |
| `needs_confirmation` | a `confirm` phase asks permission to start | yes |
| `phase_completed` | a phase finished and wrote its document | no |
| `feature_completed` | `verify` and `review` passed, ready to commit | no |
| `error` | a phase failed or a check did not pass | no |

Routing per event is configured under `notify.routing` in `alfred.config.yaml`.

The default routing follows one rule: **if the pipeline needs the user, it reaches them
wherever they are; if it is only reporting, it stays in the terminal.** Notifying every
completed phase to a phone trains the user to ignore the channel that carries the
blocking questions.

## Origin channel

A blocking question returns to the channel the run was started from.

```
run started from the terminal   ->  ask() reaches the terminal
run started from OpenClaw       ->  ask() reaches WhatsApp or Telegram
```

The origin channel is recorded in the pipeline state when the run starts. Without it, a
run triggered from a phone would block on a question printed to a terminal nobody is
watching.

The terminal is always included as a fallback, so a blocking question is never delivered
to an unreachable channel alone.

## Unavailable channels

With `notify.skip_unavailable_channels: true`, a channel that is not configured or not
responding is dropped from the delivery list.

| Operation | Channel unavailable |
|---|---|
| `notify` | skipped, pipeline continues |
| `ask`, `confirm` | falls back to the terminal, pipeline waits there |

A missing optional channel never fails a run. The terminal is the only required channel
because it is the one that always exists where the agent is running.

## Message shape

Messages carry the change they belong to, so a user following several runs can tell them
apart.

```
notify("phase_completed", "spec written: 3 requirements, 7 scenarios", "login-google")
ask("Which auth strategy?", ["OAuth redirect", "PKCE"], "PKCE")
confirm("Write code for 8 tasks", "login-google, strict TDD")
```

Adapters decide presentation. The same `phase_completed` may print one line in a terminal
and send a formatted message on Telegram; the skill that raised it knows neither.
