# 0001: Keep the Go implementation; try Rust only as an independent verifier

- Status: Accepted (2026-10-22)

## Context

The review of this codebase (REVIEW_PLAN.md, section 7) asked whether a rewrite in Rust would fix its problems.
Most findings were logic and design bugs (restore path traversal, SFTP host-key trust, key derivation strength,
swallowed `check` errors, prune and resume logic), which a language change doesn't prevent. Rust would have
caught a minority: chunk-pool ownership bugs and data races, and it would lower memory use.

A rewrite must read and write existing storages byte for byte: the buzhash chunker and its table, keyed
BLAKE2b, AES-GCM, RSA-OAEP, PBKDF2 with both config formats, go-lz4 framing, zstd, zlib, Reed-Solomon with the
incorrect arm64 HighwayHash, the hand-written msgpack entry format, and fossil-collection semantics shared with
other clients, over 17 backends. The estimate was 6 to 12 engineer-months plus a long soak period, for a tool
where a bug means lost backups. The license also treats derivative works specially, whatever the language.

## Decision

Fix and modernize the Go code (phases 0 to 5 of the plan). If Rust becomes a goal, start with a read-only
`duplicacy-verify` (parse config, snapshots and chunks, check chunks, restore to a directory), tested against the
golden fixtures and against output of the Go implementation.

## Consequences

- Effort goes into tests (golden fixtures, fuzzing, race tests) that any future implementation can reuse.
- `docs/FORMAT.md` is the specification a second implementation would follow.
- Revisit if a verifier reaches full fixture compatibility within about 6 to 8 weeks of work, or if memory use
  of the Go client becomes the main complaint despite the Phase 4 work.
