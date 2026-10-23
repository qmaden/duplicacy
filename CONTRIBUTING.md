# Contributing

Thanks for helping. This fork's priority is keeping people's backups safe: correctness and compatibility come
before features and speed.

## Before you start

- Read [AGENTS.md](AGENTS.md): repository map, build and test commands, compatibility invariants and coding
  conventions. It applies to human contributors too.
- For anything beyond a small fix, open an issue first and describe the change. Changes to the storage format,
  encryption, or prune need a design discussion.
- Security problems: don't open a public issue. See [SECURITY.md](SECURITY.md).

## Development setup

```bash
git clone https://github.com/qmaden/duplicacy && cd duplicacy
go build ./... && go test -race -count=1 ./...
```

Go is downloaded automatically at the pinned version (see [docs/BUILDING.md](docs/BUILDING.md)). Supported
platforms are Linux x86-64 and macOS.

## Making a change

1. Create a branch from `master`.
2. Keep the change focused. Formatting-only or refactoring-only changes go in separate commits.
3. Add tests: a regression test for every bug fix, and tests for new behaviour. Use local storages under
   `t.TempDir()`.
4. Run the checks from the "Definition of done" in AGENTS.md. CI runs the same checks.
5. Add an entry to [CHANGELOG.md](CHANGELOG.md) under "Unreleased". If users need to do something differently,
   also update `docs/UPGRADING.md` (create it if it doesn't exist yet).
6. Open a pull request and fill in the template.

## Compatibility checklist

Answer these in the pull request:

- Can every storage written by earlier versions still be read? (`TestGoldenFixtures` passes without
  regenerating fixtures.)
- Can earlier versions still read storages written with this change? If not, is that intended, opt-in, and
  documented?
- Does the change alter chunk boundaries, chunk IDs, or what `prune` deletes?
- Does it change command-line flags, exit codes, or log IDs that scripts may depend on?

## Commit messages

- First line: what changed, in the imperative mood ("Validate restore paths"), at most about 72 characters.
- Body: why, and how it was tested. Reference finding IDs from `REVIEW_PLAN.md` (e.g. "Fixes S1") or issues.

## License

By contributing you agree that your contribution is distributed under the terms in [LICENSE.md](LICENSE.md).
