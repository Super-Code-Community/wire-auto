# wire-auto — project rules

## Git discipline (hard rule)

- **Never auto-create branches.** Do not run `git branch`, `git checkout -b`, or `git switch -c` on your own initiative.
- **Never auto-commit or auto-push.** Do not run `git commit` or `git push` unless the user explicitly asks for it in that same message.
- Finishing a task is NOT permission to commit. Wait for an explicit instruction every time.
- This rule is also enforced mechanically: git write operations are set to always prompt in `.claude/settings.json`.

## Go workspace discipline (hard rule)

- **No committed `go.work` in the repo root.** `go.work` (and `go.work.sum`) is a purely local, per-machine dev convenience and is gitignored. Never `git add`/commit it, and never treat it as a tracked repo artifact.
- **Every module builds independently without it.** Each Go module (`runtime/<model>/`, `apps/<client>/`) must build, vet, and test on its own via `cd <module> && go build ./... && go vet ./... && go test ./...`. Nothing in the committed repo may depend on a root `go.work`.
- A developer who wants root-level `go build/test ./...` to span modules may create a local `go.work` (`go work init ./runtime/basic ./apps/deview`), but it stays local.

## Documentation discipline (guide/)

The project principle is **nothing monolithic** — this applies to docs too.

- **`guide/` is not one big README.** Do not pile architecture, conventions, and how-tos into a single file. Split them into **many small, focused `.md` files**, one topic per file.
- **Organize by topic, keep it sorted.** Give files clear, sortable names (e.g. a numeric or kebab-case prefix that reflects reading order or area: `01-overview.md`, `runtime-models.md`, `cores.md`, `protocol.md`, `writing-a-script.md`). New instructions go into the right topic file, not appended to an unrelated one.
- **`guide/README.md` is only an index** — a short table of contents linking to the topic files, nothing more. Substance lives in the topic files.
- When a topic file grows to cover two distinct things, split it — same rule as code: one file, one responsibility.
