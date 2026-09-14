---
channel: openclaw
required: false
blocking_capable: true
---

# OpenClaw adapter

Reaches the user on WhatsApp or Telegram through OpenClaw, for when the run was started
from a phone or the user is away from the terminal.

## Requirements

OpenClaw configured with at least one messaging channel. Absent, this adapter is skipped
per `notify.skip_unavailable_channels`, and every event falls back to the terminal.

## Operation mapping

| Contract | Behaviour |
|---|---|
| `notify` | send a message, do not wait |
| `ask` | send the question, wait for a reply on the same conversation |
| `confirm` | send the action, wait for a yes or no |

## Routing

This channel receives blocking events and terminal states, not progress. Per
`notify/CONTRACT.md`:

```
needs_input         here and terminal
needs_confirmation  here and terminal
feature_completed   here and terminal
error               here and terminal
phase_completed     terminal only
```

Sending every completed phase to a phone trains the user to ignore the channel that also
carries the questions that block the run.

## Message shape

A phone has no scrollback worth reading. Messages state the change, what happened, and what
is needed.

```
login-google

design needs a decision:

1. OAuth redirect — simpler, needs a server-side session
2. PKCE — no session, more moving parts

Reply 1 or 2.
```

## Origin channel

A run started from OpenClaw records it in state, and blocking questions return here rather
than to a terminal nobody is watching, per `notify/CONTRACT.md`.

The terminal stays in the delivery list for blocking events even then, so an unreachable
phone does not strand the run.

## Failure

A send that fails is skipped for `notify` and falls back to the terminal for `ask` and
`confirm`. An unreachable messaging channel never fails a run.

## Waiting

A blocking question sent here may wait a long time. The run stays in
`waiting_for_input` with the question recorded in state, so `continue` resumes it after an
interrupted session rather than asking again.
