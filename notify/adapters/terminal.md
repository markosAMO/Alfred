---
channel: terminal
required: true
blocking_capable: true
---

# Terminal adapter

The channel that always exists, because it is where the agent is running. Every other
channel is optional and falls back to this one.

## Operation mapping

| Contract | Behaviour |
|---|---|
| `notify(event, message, change)` | one line, prefixed with the change |
| `ask(question, options, default)` | prompt, wait for the answer |
| `confirm(action, details)` | prompt, wait for yes or no |

## Output

```
login-google · spec written: 3 requirements, 7 scenarios
login-google · verify: 7 of 7 scenarios, coverage 100%
```

One line per event, prefixed by the change, so several runs in one session stay
distinguishable.

Blocking operations show what is being decided, not only the question.

```
login-google · design

  Two approaches differ materially here:

  1. OAuth redirect    simpler, requires a server-side session
  2. PKCE              no session needed, more moving parts

  Which one? [2]
```

The default is shown in brackets. An empty answer takes it; a timeout takes it only when
`ask` was given one.

## Blocking

This adapter can block, which is why it is the required fallback. A blocking question routed
only to a channel that cannot answer would hang the run.

## Failure

There is none to handle. If the terminal is unavailable there is no run.
