# Architecture

How the code is organized and how the main commands move data. The on-storage format is in
[FORMAT.md](FORMAT.md), prune's safety argument in [PRUNE_SAFETY.md](PRUNE_SAFETY.md).

## Repository layout

| Path | Contents |
|------|----------|
| `duplicacy/duplicacy_main.go` | The CLI: command and flag definitions (`newApp`), preference handling, and one function per command that sets up a `BackupManager` and calls into `src/`. |
| `src/` | The `duplicacy` package. All logic lives here; files are named `duplicacy_<area>.go`. |
| `src/testdata/fixtures/` | Golden storages of every supported format, restored by `TestGoldenFixtures`. |
| `internal/wronghighwayhash/` | Pure Go version of the incorrect arm64 HighwayHash that CLI 3.0.1 used for erasure-coding shard checksums. |
| `integration_tests/` | Shell scripts that drive the binary against a local storage. |
| `docs/` | This documentation; `COMMANDS.md` is generated from the CLI definitions. |

## Main types in `src/`

| Type (file) | Role |
|-------------|------|
| `Config` (`config.go`) | The storage's `config` file: chunk size parameters, the hash, id, chunk and file keys, compression level, RSA and erasure-coding settings. Also owns the chunk buffer pool (`GetChunk` / `PutChunk`). |
| `Chunk` (`chunk.go`) | A chunk's bytes plus its hasher. `Encrypt` and `Decrypt` turn plaintext into the stored form and back (compression, AES-GCM, RSA, erasure coding). Codec helpers are in `compression.go`. |
| `ChunkMaker` (`chunkmaker.go`) | Splits a stream of files into variable-size chunks with a buzhash rolling hash, and computes each file's hash. For backups it leaves the chunk hash to the receiver (`deferHashing`). |
| `ChunkOperator` (`chunkoperator.go`) | A pool of `threads` goroutines that upload, download, find, fossilize, resurrect and delete chunks. Retries failed transfers (2, 8 and 30 seconds). Also `sequenceDownloader`, which fetches the chunks of one metadata sequence in parallel and returns them in order. |
| `ChunkDownloader` (`chunkdownloader.go`) | Restore-side wrapper around `ChunkOperator`: keeps the list of chunks the restore will need and prefetches ahead of the file being written. |
| `Entry` (`entry.go`) | One file, directory or link: path, size, times, mode, ownership, extended attributes, and the range of chunks holding its content. Also `ListEntries`, the directory walker with include/exclude patterns. |
| `EntryList` (`entrylist.go`) | The list of entries of a backup in progress, spilled to a file in the cache past `-max-in-memory-entries` entries or about 128 MiB. Saved as an *incomplete snapshot* when a backup is interrupted, so the next backup can resume. |
| `Snapshot` (`snapshot.go`) | One revision: id, revision, times, tag, and the chunk sequences that hold its file list, chunk hashes and chunk lengths. |
| `BackupManager` (`backupmanager.go`) | `Backup`, `Restore`, `CopySnapshots` and `UploadSnapshot`. |
| `SnapshotManager` (`snapshotmanager.go`) | Everything that reads snapshots: `list`, `check`, `cat`, `diff`, `history`, `prune`, and the snapshot cache. |
| `Storage` interface, `StorageBase` (`storage.go`) | A flat file API (list, get info, download, upload, move, delete, create directory) implemented once per backend in `<backend>storage.go`. `StorageBase` adds chunk nesting, the separate metadata directory and rate limits. `CreateStorage` parses storage URLs and gathers credentials. |
| `Preference` (`preference.go`) | One entry of `.duplicacy/preferences`: storage URL, snapshot id, options and stored keys. |

Logging goes through `LOG_DEBUG` … `LOG_ERROR` (`log.go`). `LOG_ERROR` panics; the CLI recovers in
`CatchLogException` and exits, so code after a `LOG_ERROR` doesn't run.

## Backup

1. Download the latest revision of this snapshot id and put the chunks it references in an in-memory chunk cache
   (on the first backup to a storage that lists quickly, list every chunk on the storage instead).
2. Walk the repository with `ListEntries` (or the shadow copy / snapshot root with `-vss`). In quick mode an entry
   whose size and modification time match the previous revision keeps its old chunk references; other files are
   marked modified. In hash mode (the first backup, or `-hash`) an interrupted backup saves an incomplete snapshot,
   which the next hash-mode backup reuses if all its chunks still exist.
3. Feed the modified files, one after another, to the `ChunkMaker`. Files of 1 MiB or more are read and hashed on a
   read-ahead goroutine. Every chunk goes to the `ChunkOperator` unhashed; a worker computes its hash and id, skips
   it if it is in the chunk cache (`UploadFilter`, under a lock), and otherwise checks whether it already exists on
   the storage, compresses, encrypts and uploads it.
4. Encode the entry list and the chunk hash and length lists into metadata chunks (`UploadSnapshot`), upload those,
   and finally upload the snapshot file `snapshots/<id>/<revision>`. The revision exists only once that file does,
   so an interrupted backup leaves at most unreferenced chunks.

Backups never lock the storage and never delete anything. Several computers can back up to the same storage at
once; identical chunks have identical ids, so they are stored once.

## Restore

1. Download the snapshot and its metadata chunks, and list the files to restore (all, or those matching patterns).
2. Compare them with the local files. Files that already match are skipped. For files that differ, the existing
   local file is read at the chunk boundaries of the backup, so that chunks already present locally are kept;
   only the other chunks are downloaded.
3. The `ChunkDownloader` downloads the needed chunks with `threads` workers, prefetching chunks of upcoming files,
   while one goroutine writes the files in order and 8 more set their permissions, times and extended attributes.
   The CLI restores in place: changed files are updated where they
   are, and every restored file is checked against its file hash.
4. Paths are validated and symbolic links are never followed inside the restore tree; see
   [SECURITY_MODEL.md](SECURITY_MODEL.md).

## Copy

`CopySnapshots` reads snapshots from one storage and writes them to another, which may use different keys,
encryption, compression or chunk nesting, but must have the same chunk size parameters and id keys (created with
`add -copy`) so chunk ids stay the same. Chunks missing on the destination are downloaded by one `ChunkOperator`
and uploaded by another; snapshot files are written last.

## Check

`check` verifies that every chunk referenced by the selected snapshots exists (one listing of the chunk
directories). `-chunks` downloads and decrypts every chunk once and records verified chunks in the cache;
`-files` restores every file in memory and compares file hashes. `-fossils` and `-resurrect` deal with chunks
found only as fossils, `-rewrite` re-uploads chunks repaired by erasure coding.

## Prune

Deletes snapshot files by revision, tag or retention policy, then removes chunks no longer referenced, in two
steps across runs (fossil collection), so that backups running concurrently are never broken. See
[PRUNE_SAFETY.md](PRUNE_SAFETY.md).

## Caches and local state

Everything lives under `.duplicacy/` in the repository (or the directory given by `-pref-dir`):

| Path | Contents |
|------|----------|
| `preferences` | Storage URLs, snapshot ids, options and optional stored keys (mode 0600). |
| `cache/<storage>/snapshots/` | Downloaded snapshot files. |
| `cache/<storage>/chunks/` | Metadata chunks, decrypted, so that later commands don't download them again. Cleaned to those referenced by the latest revision. |
| `cache/<storage>/fossils/` | Prune's fossil collections. |
| `cache/<storage>/incomplete_snapshot`, `incomplete_chunks` | The entry list and uploaded chunks of an interrupted backup. |
| `cache/<storage>/verified_chunks` | Chunks verified by `check -chunks`. |
| `known_hosts` | SFTP host keys accepted for this repository. |
| `logs/` | Prune logs. |

## Concurrency model

- **One goroutine** walks the file system and runs the chunker; with read-ahead, a second goroutine reads and
  computes the file hash of large files. Chunk hashes are computed by the upload workers.
- **`threads` workers** in each `ChunkOperator` do all storage I/O. Chunks travel between goroutines as pooled
  `*Chunk` values: whoever holds a chunk passes it on or returns it with `PutChunk` exactly once.
- Metadata sequences are fetched with a small parallel window (`sequenceDownloader`) because they are read in
  order.
- Storage backends must be safe for concurrent use by different thread indexes; the `threadIndex` argument lets a
  backend keep one client per thread. Such backends (B2, Dropbox, S3C, Google Drive) implement
  `threadLimitedStorage` and can only be used with as many threads as they were created with; code that picks its
  own number of threads (the metadata downloads) caps it with `usableThreads`.
- `-memory-limit` sets the Go runtime's soft memory limit for large repositories.

## Where to make changes

- **A new backend**: implement `Storage` in `src/duplicacy_<name>storage.go`, embed `StorageBase`, call
  `SetDefaultNestingLevels`, add the URL scheme to `CreateStorage`, and document it in
  [storage-backends.md](storage-backends.md). Prefer one client that is safe for concurrent use; implement
  `threadLimitedStorage` if the backend keeps per-thread state. Add a fake or emulator to
  `loadFakeCloudStorage` so the storage tests can run against it.
- **A new command or flag**: edit `newApp` in `duplicacy/duplicacy_main.go`, then regenerate
  [COMMANDS.md](COMMANDS.md).
- **Anything touching the storage format**: read the compatibility rules in [AGENTS.md](../AGENTS.md) first; the
  golden fixtures must keep passing without being regenerated.
