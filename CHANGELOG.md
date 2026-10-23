# Changelog

All notable changes to this fork are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Upstream history before the fork is in the git log.

## Unreleased

### Added

- Golden-fixture tests that restore checked-in storages of every supported format (`TestGoldenFixtures`).
- GitHub Actions CI for Linux and macOS: race-enabled tests, storage-option variants, Go 1.27 build, gofmt,
  go vet, staticcheck and govulncheck.
- Contributor and agent documentation: `AGENTS.md`, `CONTRIBUTING.md`, `docs/BUILDING.md`, `docs/TESTING.md`,
  `docs/FORMAT.md` (storage format), `docs/SECURITY_MODEL.md`, `SECURITY.md`, `docs/UPGRADING.md`.
- `backup -strict`: exit with code 4 when files or directories were skipped because of access errors (upstream #540).
- `password -iterations N` can keep the same password to upgrade the key derivation of an existing storage.
- Fuzz targets for the decoders of untrusted storage data (`FuzzChunkDecrypt`, `FuzzEntryDecode`,
  `FuzzSnapshotDescription`, `FuzzConfigUnmarshal`).
- `check -chunks -reverify-days N` verifies again chunks last verified more than N days ago.
- `prune` takes an advisory `prune.lock` on the storage; `-ignore-lock` overrides it.
- Restore with `-overwrite` replaces paths that changed between a file and a directory (upstream #429).
- Global option `-memory-limit <MB>` sets a soft memory limit for the Go runtime (upstream #340).
- Failed chunk uploads and downloads are retried up to three times, after 2, 8 and 30 seconds (upstream #683).
- Benchmarks for chunk encryption, decryption and chunking (`go test -bench . ./src`).
- Documentation: generated command reference (`docs/COMMANDS.md`), `docs/storage-backends.md`,
  `docs/ARCHITECTURE.md`, `docs/PRUNE_SAFETY.md` and decision records in `docs/adr/`; the README describes the fork.

### Security

- Restore never writes outside the restore directory: snapshot entry paths are validated (upstream #692), and symbolic links
  in the restore tree are not followed (upstream #691). setuid/setgid bits are restored only together with ownership.
- SFTP host keys are verified by host name against `known_hosts` files (including `~/.ssh/known_hosts`); new keys
  must be confirmed and changed keys are refused (upstream #639). The SSH dial times out after 30 seconds.
- Chunk decoding validates erasure-coding headers and limits decompressed sizes, so malformed or crafted chunks
  can't crash the client or exhaust memory.
- New storages use 600,000 PBKDF2 iterations; iteration counts above 100 million in a config are rejected.
- OAuth token files, the snapshot cache and new `.duplicacy` directories are private to their owner.
- Cloud secrets are no longer echoed at prompts.
- RSA private keys in the OpenSSH format and unencrypted PKCS#8 are supported; inline keys are no longer printed in
  error messages.
- Dependency upgrades fix GO-2026-5841 (klauspost/compress), GO-2022-0391 (aws-sdk-go) and GO-2024-2611
  (protobuf).

### Changed

- Toolchain pinned to Go 1.26.10 (fixes GO-2026-6809 in `crypto/x509`).
- The incorrect arm64 HighwayHash used by CLI 3.0.1 for erasure-coding shard checksums is now computed by a
  pure Go package (`internal/wronghighwayhash`). The code now builds with Go 1.27, and chunks written by CLI 3.0.1
  on arm64 can be read on any architecture.
- Supported platforms are Linux x86-64 and macOS.
- `prune -keep` policies are sorted by age; policies given out of order used to be skipped silently.
- Fossils are deleted only after snapshots ending at least 15 minutes after the collection on strongly consistent
  storages, to tolerate clock skew.
- Faster backup and restore with less memory: LZ4 through `pierrec/lz4` with the same chunk format, pooled zstd
  codecs, chunk buffers sized from the average chunk size, single-write local chunk uploads (upstream #670), large
  files read ahead and hashed on a second core, metadata chunks downloaded in parallel, and restore prefetching
  across files (upstream #541). Extended attributes are read when each entry is stored instead of being kept for
  the whole backup (backing up 200,000 files on macOS: peak memory 743 MB to 198 MB). Waiting for chunk
  operations no longer polls every 100 ms, which made prune with many chunks much slower than needed.
- B2, OneDrive, WebDAV, File Fabric and Wasabi reuse one connection per thread and time out stalled connections
  (30 s to connect, 5 minutes for a response to start) instead of hanging (upstream #644, #480).
- SFTP tries keys from a running ssh-agent before asking for a password or a key file (upstream #687); the SSH key
  prompts say they are for logging in, not the RSA encryption key (upstream #695).
- BLAKE2b is computed by `golang.org/x/crypto/blake2b` instead of the unmaintained `minio/blake2b-simd` (same
  hashes, checked against recorded vectors). `klauspost/compress` 1.20.3, `aws-sdk-go` 1.55.8, protobuf 1.36.12.
- `io/ioutil` replaced and unused code removed.

### Fixed

- `check -files` reported structurally broken (version 0) snapshots as valid: `CheckSnapshot` discarded every error
  it found.
- With `-persist`, a chunk that failed to download could be shared by two goroutines (upstream #675).
- Restore leaked one chunk buffer per file that reused local data.
- Swift: every operation failed once the connection timeout had elapsed since start (upstream #664).
- With `separate-metadata`, a file chunk identical to a metadata chunk of the previous revision was never uploaded,
  so the file couldn't be restored.
- Erasure-coded chunks were decoded with the shard counts of the config instead of the chunk header.
- `password` could leave a storage without a config file when saving the new one failed.
- Samba storages on IPv6 addresses; FileFabric upload error handling; `%` in pre/post script output.
- A resumed backup could reference chunks that prune had removed since the interruption.
- `check -rewrite` could leave a chunk only as a fossil when uploading the repaired copy failed.
- `restore -delete` deleted a file it had just restored in place of a directory.
- With patterns, restore left the directories above restored files with default permissions (upstream #84).
- The chunk operator could report completion before all queued tasks had finished; copy counted chunks with a data
  race and reported existing chunks as copied; restore progress used a stale total.
- Version 0 snapshot entries were not checked against the chunk list; file hashes ignored read errors; restore
  removed the old file before renaming the new one over it; a path prefix test matched sibling directories.
