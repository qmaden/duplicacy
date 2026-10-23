# 0004: Encode and decode LZ4 with pierrec/lz4, keeping the go-lz4 framing

- Status: Accepted (2026-10-23)

## Context

LZ4 is the default compression. Chunks were compressed with `bkaradzic/go-lz4`, unmaintained since 2016, which
allocates a 512 KiB hash table per call and runs at 527 MB/s encode and 826 MB/s decode on the test machine. It
also has no output limit, so a crafted chunk could make the decoder allocate unbounded memory (S5). Its stored
form is a 4-byte little-endian uncompressed length followed by one raw LZ4 block, after the `LZ4 ` prefix.

`pierrec/lz4/v4` exposes the raw block API, is maintained, reuses its tables, and runs at 1,311 MB/s encode and
2,148 MB/s decode.

## Decision

- Encode with `pierrec/lz4` `CompressBlock`, writing the same 4-byte length framing. When `CompressBlock` returns
  0 (incompressible input), fall back to go-lz4 so the output is always a valid block that old clients decode.
- Decode with `UncompressBlock`, rejecting a declared length above the chunk size limit before allocating.
- Keep go-lz4 as a dependency for the fallback and as the reference in `TestLZ4InteroperatesWithGoLZ4`, which
  checks both directions up to 16 MiB.

## Consequences

- Stored bytes may differ from what go-lz4 would have written for the same input (both are valid LZ4); chunk ids
  don't change because they are computed from the uncompressed content. All clients decode both.
- Together with the pooled zstd codecs and buffer reuse, backup CPU time on the 1.1 GB test dataset dropped by 17%
  and peak memory by 40% (REVIEW_PLAN.md, section 9.3).
- Revisit dropping go-lz4 once `CompressBlock` handles incompressible input itself (or by writing a literal-only
  block directly).
