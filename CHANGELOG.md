# Changelog

All notable changes to this fork are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Upstream history before the fork is in the git log.

## Unreleased

### Added

- Golden-fixture tests that restore checked-in storages of every supported format (`TestGoldenFixtures`).
- GitHub Actions CI for Linux and macOS: race-enabled tests, storage-option variants, Go 1.27 build, gofmt,
  go vet, staticcheck and govulncheck.
- Contributor and agent documentation: `AGENTS.md`, `CONTRIBUTING.md`, `docs/BUILDING.md`, `docs/TESTING.md`.

### Changed

- Toolchain pinned to Go 1.26.10 (fixes GO-2026-6809 in `crypto/x509`).
- The incorrect arm64 HighwayHash used by CLI 3.0.1 for erasure-coding shard checksums is now computed by a
  pure Go package (`internal/wronghighwayhash`). The code now builds with Go 1.27, and chunks written by CLI 3.0.1
  on arm64 can be read on any architecture.
- Supported platforms are Linux x86-64 and macOS.
