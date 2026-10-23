# 0005: Reimplement the incorrect arm64 HighwayHash in pure Go

- Status: Accepted (2026-10-22)

## Context

Erasure-coded chunks carry a HighwayHash per shard. CLI 3.0.1 on arm64 used `gilbertchen/highwayhash`, whose
assembly loaded its lookup table from the wrong symbol: a compiler-generated code wrapper. The resulting hash
differs from real HighwayHash and depended on compiler output. Go 1.27's linker rejects that symbol collision, so
the dependency blocked toolchain upgrades. Chunks written that way could only be verified on arm64.

## Decision

Copy the code into `internal/wronghighwayhash` as pure Go (Apache-2.0 license kept) that emulates the arm64
instructions step by step, with the 48 table bytes pinned as data. Shard verification tries the correct hash first
and falls back to this one on every architecture, flagging such chunks for `check -rewrite`.

## Consequences

- Builds with Go 1.26 and 1.27, with no assembly and no dependency on code layout.
- Chunks from CLI 3.0.1 on arm64 can now be read on amd64 too.
- Verified against 844 golden vectors from the original assembly and a 200,000-case differential test.
- The pinned bytes have not been confirmed against an actual 3.0.1 release binary; behaviour of the Go 1.21 to
  1.26 builds is preserved exactly.
