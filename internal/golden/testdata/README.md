# Recorded output

What the installer's helper emits, byte for byte. Other programs read it, so a change here
is a change to an interface. Validation rules and report wording are unit-tested in
`internal/workflow` and `internal/agents`; what is recorded here is the binary's output end
to end.

## Inputs

```
fixture.json                 the state tree and payload, the edits behind each compare
                             classification, and the paths of every fixture below
profiles/                    uniform, every-option (every key set) and minimal (every
                             optional key omitted, default workflow not installed)
workflows/shipped/           ventas, with its own phases, and soporte
workflows/custom/            inventario, and cobranzas, whose own phase has no model
workflows/rejected/          one refusal per stage (decode, validation, scan, resolve) and
                             the unsafe values: a path-escaping phase, a model and a tool
                             that would inject frontmatter, and "forged  row"
workflows/rejected-custom/   the second root, holding the name that is in both
workflows/collision/         two workflows generating one subagent name, and one that registers
workflows/injection/         a description carrying a newline
agents/migration/            what the previous generator left: adopted legacy files,
                             alfred-init and alfred-explore as subagents, a colliding
                             alfred-ventas, the orphan alfred-notes and the foreign key mi-agente
project/repo/                a repository with two workflows, one named ventas, an orphan
                             alfred-stale and a .gitignore
project/bare/                a repository defining no workflow
```

`workflows/` is testdata rather than the shipped `workflows/`, so tuning `sdd` does not
turn this suite red. `forged  row` is a directory name holding the report's column
separator (two spaces); it must be printed quoted.

## Recordings

```
state/                       state write, the compare reports, both answers of version
agents/<case>.report         what a machine-scope run printed and how it exited
agents/every-option/         every file that run wrote
agents/minimal/              only the files whose shape every-option does not cover:
                             the /alfred stub, alfred-worktree and opencode.json
agents/injection.command     the one command the injection case generates
project/<case>.report        what a project-scope run printed and how it exited
project/<case>.exclude       the local exclude file it wrote
```

Anything else a case needs on disk (what was removed, what was left alone) is asserted by
name in the test rather than recorded.

## Format

```
exit 1

--- stdout
...
--- stderr      (only when there was any)
...
```

Three things are normalised because they differ per machine: `<work>` (the temporary
tree), `<alfred>` (this checkout) and `<sha256>` (manifest hashes, taken over bytes holding
the other two).

Nothing under `testdata/` may sit at a path some checkout cannot hold — a `.git`
component, a control character, a name Windows refuses — or the comparison is silently
skipped. `TestNoRecordingLivesAtAPathACheckoutCannotHold` enforces it, and is why the
exclude files are recorded flat rather than at `.git/info/exclude`.

## Re-recording

Only when the output is meant to change, in the commit that justifies it. There is no
`-update` flag on purpose: read the diff the test prints, then edit or regenerate the file.

```
go test ./internal/golden/ -v
```
