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

### Changed

- Toolchain pinned to Go 1.26.10 (fixes GO-2026-6809 in `crypto/x509`).
- The incorrect arm64 HighwayHash used by CLI 3.0.1 for erasure-coding shard checksums is now computed by a
  pure Go package (`internal/wronghighwayhash`). The code now builds with Go 1.27, and chunks written by CLI 3.0.1
  on arm64 can be read on any architecture.
- Supported platforms are Linux x86-64 and macOS.

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
