# AGENTS.md

Guidance for AI coding agents (and humans) working on this repository. Read it before changing code.

## The project in one paragraph

Duplicacy is a deduplicating, encrypted backup tool written in Go: files are split into content-defined chunks,
each chunk is stored under a name derived from its hash, and snapshots reference chunks. This repository is a
maintained fork of `gilbertchen/duplicacy` (the Go module path is still `github.com/gilbertchen/duplicacy`).
**The overriding rule: storages written by any earlier version must stay readable.** A bug that silently changes
an on-disk format loses users' backups. Supported platforms are **Linux x86-64 and macOS**; the Windows code is
kept but not built or tested.

## Repository map

| Path | What is there |
|------|---------------|
| `duplicacy/duplicacy_main.go` | CLI: commands, flags, preferences, exit codes |
| `src/duplicacy_chunk.go`, `duplicacy_config.go` | Chunk format: compression, AES-GCM, RSA, erasure coding; storage config and key derivation |
| `src/duplicacy_chunkmaker.go` | Content-defined chunker (buzhash) |
| `src/duplicacy_chunkoperator.go`, `duplicacy_chunkdownloader.go` | Multi-threaded upload/download/delete/fossilize of chunks |
| `src/duplicacy_backupmanager.go` | `backup`, `restore`, `copy` |
| `src/duplicacy_snapshotmanager.go` | `list`, `check`, `cat`, `diff`, `history`, `prune` (two-step fossil collection) |
| `src/duplicacy_entry.go`, `duplicacy_entrylist.go`, `duplicacy_snapshot.go` | File entries, their encoding, local and remote listing |
| `src/duplicacy_storage.go` | `Storage` interface, URL parsing, chunk nesting, SFTP host keys |
| `src/duplicacy_*storage.go`, `*client.go` | One file per storage backend |
| `internal/wronghighwayhash` | Pure Go copy of the incorrect arm64 HighwayHash needed to read chunks from CLI 3.0.1 |
| `src/testdata/fixtures` | Golden storages that must always restore identically (see its README) |
| `integration_tests/` | Old shell-based integration tests (need a built binary) |
| `REVIEW_PLAN.md` | Review findings with IDs (B1, S1, N1, P1 …) and the remediation plan |
| `docs/` | Command reference, storage backends, architecture, prune safety, format, security, building, testing, decision records (`docs/adr/`) |

## Build and test

Use the pinned toolchain. With a newer local Go, prefix commands with `GOTOOLCHAIN=go1.26.10`.

```bash
go build ./...                                   # build everything
go build -o duplicacy_main ./duplicacy           # build the CLI
go test -race -count=1 ./...                     # all tests, offline, about 3 minutes
go test -count=1 -run TestGoldenFixtures ./src   # format compatibility only, about 1 second
cd src && go test -count=1 -run 'TestBackupManager|TestStorage|TestPrune' . -args -erasure-coding
gofmt -l src duplicacy internal                  # must print nothing
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 -checks 'U1000,SA1*,SA2*,SA3*,SA4017,SA5*,SA6*,SA9*,-SA1019' ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

The `-args` flags for storage tests are `-erasure-coding`, `-separate-metadata`, `-rsa` and
`-fixed-chunk-size`. Tests use a local file storage under the system temp directory unless `-storage <name>` is
given: `fake-s3`, `fake-gcs` and `fake-gcd` use in-process fakes, `azurite` an Azurite emulator (see
`docs/TESTING.md`), and other names a git-ignored `src/test_storage.conf`. Changes to a cloud backend must pass
against its fake.

After changing a command or option, regenerate `docs/COMMANDS.md` (a test fails otherwise):
`DUPLICACY_UPDATE_DOCS=1 go test -run TestCommandReference ./duplicacy`. New storage backends and credential keys
go into `docs/storage-backends.md`.

Do **not** build with `-tags noasm` and don't add assembly. See `docs/BUILDING.md`.

Exit codes of the CLI: 0 success, 1 interrupted, 2 invalid command, 3 invalid arguments, 4 `backup -strict` with
skipped entries, 100 an error logged with `LOG_ERROR`, 101 an unexpected panic.

## Compatibility invariants

Never change these without a format-version plan, new fixtures, and a reviewed design (see `docs/FORMAT.md`):

- Chunk layout: banners `duplicacy\000` (AES-GCM), `duplicacy\002` (RSA), `duplicacy\003` (erasure coding);
  nonce placement; the 256-byte PKCS#7-style padding; compression prefixes `LZ4 ` and `ZSTD`, and raw zlib.
- Hashing: keyed BLAKE2b-256 when the compression level is ≥ 100, HMAC-SHA256 below that; chunk ID =
  hash(IDKey, chunk hash); key derivation order in `Chunk.Encrypt`/`Decrypt`.
- Config: both formats (`duplicacy\000` with static salt, `duplicacy\001` with salt and iterations) and every
  JSON key, including the accidental `DataShards`/`ParityShards`.
- The incorrect-HighwayHash fallback for erasure-coded shards (`internal/wronghighwayhash`).
- Snapshot file format, msgpack entry encoding, chunk/length sequences, version 0 JSON file lists.
- Two-step fossil collection semantics in `prune` (other clients rely on them).

`TestGoldenFixtures` must keep passing **without regenerating the fixtures**. Regenerate only for an intended,
reviewed format addition, and keep the old fixtures.

## Conventions

- **`LOG_ERROR` does not return.** It panics with an `Exception`; `CatchLogException` exits the process (code
  100). Code after a `LOG_ERROR` call never runs in production. Use `LOG_WERROR(allowFailures, …)` where failures
  may be tolerated. Return errors (don't log fatally) from parsers and decoders of untrusted data.
- Log IDs are upper-case `AREA_WHAT` strings (`DOWNLOAD_CHUNK`, `RESTORE_SYMLINK`); keep them stable, since
  users grep for them.
- **Chunk ownership:** a `*Chunk` obtained with `config.GetChunk()` must be returned with `config.PutChunk()`
  exactly once, by whoever owns it last. After handing a chunk to a callback, set your variable to `nil` so a
  deferred `PutChunk` can't return it a second time (this was bug N1, upstream #675).
- Completion callbacks of `ChunkOperator` run on worker goroutines: protect shared state (atomics or a mutex).
- Data read from a storage is untrusted. Validate sizes and indexes before slicing; never let a snapshot entry
  path escape the restore directory (use `restorePath` / `safeJoinPath`).
- Match the surrounding style: tab indentation (gofmt), the file header comment, existing error message
  capitalization in the file you edit.
- Add a regression test for every bug fix. Prefer local file storages under `t.TempDir()`.

## Security-sensitive areas (extra care, extra tests)

Restore path handling and symlinks (`backupmanager.go`), SFTP host-key checking (`storage.go`), key derivation
and config encryption (`config.go`), `Chunk.Decrypt` (untrusted input), credential prompts and storage
(`utils.go`, keyring), file and directory permissions of the cache and preferences.

## Safety rules for agents

- Never run `prune`, `prune -exclusive`, `check -rewrite`, `copy` or `password` against a real storage. Use
  temporary local storages only.
- Never commit credentials, tokens, `test_storage.conf`, or real backup data.
- Don't reformat files you didn't change; formatting changes go in their own commit.
- Keep changes minimal and focused; reference the finding ID from `REVIEW_PLAN.md` in the commit message when
  there is one.
- Don't push, open pull requests, or change repository settings unless asked.

## Definition of done

1. `go build ./...`, `go vet ./...`, `gofmt -l` (empty) and the staticcheck command above pass.
2. `go test -race -count=1 ./...` passes, including `TestGoldenFixtures` with unchanged fixtures.
3. New behaviour has tests; bug fixes have regression tests.
4. `CHANGELOG.md` has an entry under "Unreleased"; docs are updated when flags, behaviour or formats change.
