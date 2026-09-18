# Routing rules of the {name} workflow

Before any phase runs, the orchestrator chooses a route, states the signals it was based
on, and waits for the user to accept it.

## The question

> {routing_question}

## Routes

```
{routes_block}
```

## Stating the route

Every route decision names the signals it was based on. A route presented without its
reasons cannot be corrected, because the user cannot see which input was wrong.

The user can always shorten the route. The orchestrator never lengthens it without saying
so.
