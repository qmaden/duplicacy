# Testing

## Quick reference

```bash
go test -race -count=1 ./...                         # everything; offline; about 3 minutes
go test -count=1 -run TestGoldenFixtures ./src       # storage-format compatibility; about 1 second
go test -count=1 ./internal/...                      # wronghighwayhash golden vectors
```

All tests run offline against local file storages created under the system temp directory.

## Test suites

| Test | What it covers |
|------|----------------|
| `TestGoldenFixtures` | Restores checked-in storages of every supported format and compares the result with committed manifests. Fails if any change breaks reading existing backups. See `src/testdata/fixtures/README.md` |
| `TestBackupManager`, `TestPersistRestore` | Backup and restore round trips: incremental backups, in-place and temporary-file restores, `-persist` with corrupted chunks |
| `TestPrune*`, `TestIsDeletable` | Two-step fossil collection, retention policies, tags, ghost snapshots, multiple hosts |
| `TestStorage`, `TestMetadataChunkNesting` | Storage operations, chunk nesting levels, separate metadata directory |
| `TestChunk*`, `TestErasureCoding*` | Chunk encryption, compression, RSA, erasure coding, in-memory checksum |
| `TestChunkMaker`, `TestEntry*`, `TestEntryList` | Chunker boundaries, entry encoding and ordering, on-disk entry lists |
| `TestRestorePath*` | Restore path and symlink safety |
| `Fuzz*` | Fuzz targets for decoders of untrusted data (see below) |

## Storage options

The backup, restore, storage and prune tests accept flags after `-args`:

```bash
cd src
go test -count=1 -run 'TestBackupManager|TestPersistRestore|TestStorage|TestPrune' . -args -erasure-coding
go test -count=1 -run 'TestBackupManager|TestPersistRestore|TestStorage|TestPrune' . -args -separate-metadata
go test -count=1 -run 'TestBackupManager|TestPersistRestore|TestStorage|TestPrune' . -args -rsa
go test -count=1 -run 'TestBackupManager|TestPersistRestore|TestStorage|TestPrune' . -args -fixed-chunk-size
```

CI runs each of these.

## Real storage backends

`-storage <name>` selects a backend described in `src/test_storage.conf` (JSON, one object per storage name;
see `loadStorage` in `src/duplicacy_storage_test.go` for the keys each backend needs). That file holds
credentials: it is git-ignored and must never be committed. Use a dedicated, empty bucket or directory, since the
tests delete everything in it.

```bash
cd src && go test -count=1 -run 'TestStorage|TestBackupManager' . -args -storage sftp -threads 4
```

## Fuzzing

Fuzz targets cover the decoders that read untrusted storage data:

```bash
cd src
go test -run '^$' -fuzz FuzzChunkDecrypt -fuzztime 60s .
go test -run '^$' -fuzz FuzzEntryDecode -fuzztime 60s .
go test -run '^$' -fuzz FuzzSnapshotDescription -fuzztime 60s .
go test -run '^$' -fuzz FuzzConfigUnmarshal -fuzztime 60s .
```

Without `-fuzz`, `go test` runs only the seed corpus, so these targets are part of the normal suite. A crash found
by the fuzzer is saved under `testdata/fuzz/<Target>/`. Commit it together with the fix.

## Golden fixtures

Regenerate only for an intended and reviewed format addition, and keep old fixtures:

```bash
cd src && DUPLICACY_GENERATE_FIXTURES=1 go test -run TestGenerateFixtures .
```

## Benchmarks

Chunk encryption and decryption (LZ4 and zstd) and the chunker:

```bash
cd src && go test -run '^$' -bench . -benchmem .
```

Compare before and after a performance change on the same machine (for example with `benchstat`), and measure
end to end as well: most gains in this code come from overlapping I/O, which micro-benchmarks don't show.

## Generated command reference

`docs/COMMANDS.md` is generated from the CLI definitions, and `TestCommandReference` fails when it's out of date.
After changing a command or option:

```bash
DUPLICACY_UPDATE_DOCS=1 go test -run TestCommandReference ./duplicacy
```

## Integration scripts

`integration_tests/*.sh` are the original shell tests. They expect a binary at `../duplicacy_main` relative to
the script directory and use `$HOME/DUPLICACY_TEST_ZONE`:

```bash
go build -o duplicacy_main ./duplicacy && cd integration_tests && ./test.sh
```
