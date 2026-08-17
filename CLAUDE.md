# wire-auto — project rules

## Git discipline (hard rule)

- **Never auto-create branches.** Do not run `git branch`, `git checkout -b`, or `git switch -c` on your own initiative.
- **Never auto-commit or auto-push.** Do not run `git commit` or `git push` unless the user explicitly asks for it in that same message.
- Finishing a task is NOT permission to commit. Wait for an explicit instruction every time.
- This rule is also enforced mechanically: git write operations are set to always prompt in `.claude/settings.json`.

## Go workspace discipline

- **A root `go.work` is allowed.** A single `go.work` at the repo root may list every module (`go work init ./cores/regular ./cores/duplex ./apps/deview`) so `go run`/build/test span modules and `apps/deview` can spawn a core out of the box. Keep it local or commit it — either is fine (it is currently gitignored, so nothing is committed by default). Keep exactly one workspace at the root and add each new module to its `use` list.
- **Every module still builds independently.** Each Go module (`cores/<name>/`, `apps/<client>/`) must build, vet, and test on its own — verify with `cd <module> && GOWORK=off go build ./... && go vet ./... && go test ./...`. The workspace is a convenience, never a dependency: no module may rely on another through it (no hidden cross-module `replace`).

## Documentation discipline (guide/)

The project principle is **nothing monolithic** — this applies to docs too.

- **`guide/` is not one big README.** Do not pile architecture, conventions, and how-tos into a single file. Split them into **many small, focused `.md` files**, one topic per file.
- **Organize by topic, keep it sorted.** Give files clear, sortable names (e.g. a numeric or kebab-case prefix that reflects reading order or area: `01-overview.md`, `runtime-models.md`, `cores.md`, `protocol.md`, `writing-a-script.md`). New instructions go into the right topic file, not appended to an unrelated one.
- **`guide/README.md` is only an index** — a short table of contents linking to the topic files, nothing more. Substance lives in the topic files.
- When a topic file grows to cover two distinct things, split it — same rule as code: one file, one responsibility.
