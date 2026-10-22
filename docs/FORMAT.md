# Storage format

This document describes what Duplicacy writes to a storage, as implemented in this repository. Every statement
here must stay true for storages written by earlier versions. The golden fixtures in `src/testdata/fixtures`
(restored by `TestGoldenFixtures`) cover each format listed in their README.

Byte strings are shown in Go notation (`"duplicacy\001"` is the 9 letters followed by a byte with value 1).
Integers are little-endian.

## Layout

| Path | Contents |
|------|----------|
| `config` | Storage parameters and keys; see [Config](#config) |
| `nesting` | Optional JSON: `{"read-levels": [1], "write-level": 1, "separate-metadata": true}` |
| `chunks/<nesting>/<chunk id>` | Chunks. `<nesting>` is one or more levels of two hex digits taken from the start of the id (normally one level: `chunks/ab/cdef…`) |
| `metadata/<2 hex>/<chunk id>` | Metadata chunks, only when `separate-metadata` is true. Always one nesting level. Readers fall back to `chunks/` |
| `…/<chunk id>.fsl` | A fossil: a chunk marked for deletion by `prune` (see [Fossils](#fossils)) |
| `fossils/`, `metadata-fossils/` | Fossils on storages that can't rename in place (Amazon Cloud Drive, Google Drive, OneDrive) |
| `…/<name>.<random>.tmp` | A file being uploaded; renamed when complete |
| `snapshots/<snapshot id>/<revision>` | A snapshot file; see [Snapshot files](#snapshot-files) |
| `prune.lock` | JSON (`host`, `pid`, `snapshot_id`, `time`, `token`) written by a running prune; advisory, ignored by older versions |

Storages created before nesting existed may use read levels such as `[1, 2]`; the `nesting` file and the
`fixed-nesting` key in the config select the levels.

## Config

### Fields

The config is a JSON object:

| Key | Meaning |
|-----|---------|
| `compression-level` | `-1`…`9` zlib (and legacy hashing, see below), `100` LZ4, `200`…`203` zstd fastest/default/better/best |
| `average-chunk-size`, `max-chunk-size`, `min-chunk-size` | Chunker parameters (the average is a power of 2; minimum = maximum means fixed-size chunks) |
| `fixed-nesting` | `true` for configs written by 2.0.10 and later |
| `chunk-seed`, `hash-key`, `id-key`, `chunk-key`, `file-key` | Hex-encoded keys (below) |
| `DataShards`, `ParityShards` | Erasure coding parameters; `0` means off. These key names are the result of a malformed struct tag and must not change |
| `rsa-public-key` | Hex-encoded DER (PKIX) RSA public key, or empty |

Without encryption, `chunk-seed`, `hash-key` and `id-key` are the 9 bytes `"duplicacy"`, and `chunk-key` and
`file-key` are empty. With encryption all five are 32 random bytes.

### Encoding of the config file

| Variant | Bytes |
|---------|-------|
| Unencrypted | The JSON text |
| Encrypted (current) | `"duplicacy\001"` · 32-byte salt · uint32 iterations · 12-byte nonce · AES-256-GCM ciphertext |
| Encrypted (old format) | `"duplicacy\000"` · 12-byte nonce · AES-256-GCM ciphertext |

The master key is `PBKDF2-HMAC-SHA256(password, salt, iterations, 32 bytes)`. The old format uses the salt
`"duplicacy"` and 16384 iterations. The plaintext under AES-GCM is the JSON compressed and padded exactly as a chunk
(see [Chunks](#chunks)) with compression level 100 (LZ4). The AES key is the master key itself (no derivation).

New storages use 600,000 iterations (16,384 before this fork). Readers reject more than 100 million.

## Chunking

Files are concatenated in snapshot order and split by a buzhash rolling hash over a window of
`min-chunk-size` bytes. The 256-entry table of 64-bit values comes from 64 digests: `SHA-256(chunk-seed)` and
63 successive SHA-256 re-hashes, each digest giving 4 little-endian values. A boundary is placed where `hash & (average − 1) == 0`, never
before the minimum and always at the maximum size. With minimum = maximum, chunks have a fixed size. Chunks can
span files.

Metadata (the file list and the chunk and length sequences) is chunked the same way with an average of
`-metadata-chunk-size` (default 1 MiB), a minimum of a quarter and a maximum of four times that.

## Hashes and chunk ids

| Compression level | Chunk hash | File hash (stored in entries) |
|-------------------|------------|-------------------------------|
| ≥ 100 | BLAKE2b-256 keyed with `hash-key` | unkeyed BLAKE2b-256 |
| < 100 | HMAC-SHA256 with `hash-key` | SHA-256 |

The chunk hash is computed over the plaintext and never stored in clear on the storage. The chunk id (the
file name) is `hex(keyed-hash(id-key, chunk hash))` with the same algorithm. Snapshot files and sequences refer
to chunks by hash.

## Chunks

A chunk is encoded in three steps:

1. **Compression**, by compression level:
   - zlib (`-1`…`9`): a raw zlib stream, no prefix
   - LZ4 (`100`): `"LZ4 "` · uint32 uncompressed length · LZ4 block
   - zstd (`200`…`203`): `"ZSTD"` · zstd frame(s)
2. **Encryption** (encrypted storages only). The compressed data is padded to a multiple of 256 bytes with `n`
   bytes of value `n mod 256` (1 ≤ n ≤ 256, so a value of 0 means 256), then sealed with AES-256-GCM:
   - File chunks without RSA: `"duplicacy\000"` · 12-byte nonce · ciphertext. The AES key is
     `keyed-hash(chunk hash, chunk-key)`: the chunk hash is the hash key and `chunk-key` the message.
     (`DUPLICACY_DECRYPT_WITH_HMACSHA256` forces HMAC-SHA256 here, for Vertical Backup compatibility.)
   - File chunks with RSA: `"duplicacy\002"` · uint16 length · RSA-OAEP-SHA256 encryption of a random 32-byte
     AES key · 12-byte nonce · ciphertext.
   - Metadata chunks are never RSA-encrypted, so `list` and similar commands work without the private key.
3. **Erasure coding** (when `DataShards` and `ParityShards` are set), applied to the output of step 2:
   `"duplicacy\003"` · header · shard hashes · shards · header. The 14-byte header is uint64 encoded size,
   uint16 data shards, uint16 parity shards, then two XOR check bytes (`h[12] = h0^h2^h4^h6^h8^h10`,
   `h[13] = h1^h3^h5^h7^h9^h11`). Shard size is `ceil(size / data shards)`; the last data shard is zero-padded;
   parity shards use Reed-Solomon as implemented by `klauspost/reedsolomon`. Each shard has a 32-byte
   HighwayHash-256 with an all-zero key. CLI 3.0.1 on arm64 wrote these hashes with an incorrect HighwayHash;
   readers accept those too (`internal/wronghighwayhash`). Readers use the shard counts from the header.

Unencrypted chunks are the output of step 1 (and step 3 if enabled). Readers limit decompressed data to twice
the maximum chunk size, with a floor of 256 MiB.

## Snapshot files

`snapshots/<id>/<revision>` holds JSON, encoded like a chunk (step 1, and step 2 with `file-key` and the
derivation key being the file path, truncated to its last 64 characters):

| Key | Meaning |
|-----|---------|
| `version` | `1` (current) or `0` (old file list format) |
| `id`, `revision`, `tag`, `options` | Identification; `options` records `-hash` and `-vss` |
| `start_time`, `end_time` | Unix seconds, from the backup client's clock |
| `file_size`, `number_of_files` | Totals (omitted when zero) |
| `files`, `chunks`, `lengths` | Hex-encoded hashes of the metadata chunks holding each sequence |

The **chunk sequence** is a JSON array of hex chunk hashes and the **length sequence** a JSON array of chunk
lengths. They list, in order, the file chunks this snapshot references. The **file list** is the concatenation of
the `files` metadata chunks:

- Version 1: a stream of msgpack-encoded entries, each with these fields in order: path, size, modification time,
  mode, symlink target, file hash, start chunk, start offset, end chunk, end offset, uid, gid, attribute count,
  then attribute name/value pairs. For files, the start chunk is stored relative to the previous file's end chunk
  and the end chunk relative to the start chunk.
- Version 0: a JSON array of entry objects (see `Entry.UnmarshalJSON`).

Entries are sorted by path with files before subdirectories of the same directory; directory paths end with `/`.
A size of 0 means no chunks. Chunk numbers index the snapshot's chunk sequence.

## Fossils

`prune` deletes in two steps so that it can run while backups are in progress (lock-free deduplication):

1. Unreferenced chunks are renamed to `<chunk>.fsl` (or moved to `fossils/`), and a fossil collection recording
   the fossils, temporary files and the latest revision of every snapshot id is saved **locally** in the prune
   client's cache (`.duplicacy/cache/<storage>/fossils/<n>`).
2. A later prune deletes the fossils once every snapshot id has a new revision finished after the collection
   (at least 15 minutes after on strongly consistent storages, 12 hours on others) or has been inactive for 7
   days, after resurrecting any fossil that new revisions reference.

Readers that can't find a chunk look for its fossil before failing.

## Local files

These are not on the storage but in the preference directory (`.duplicacy`): `preferences` (JSON, may contain
credentials, mode 0600), `known_hosts` (SFTP host keys in OpenSSH format), `cache/<storage>/` (decrypted
metadata chunks and snapshot files, fossil collections, `verified_chunks`; private to the owner),
`incomplete_snapshot`/`incomplete_chunks` (resumable backup state), `filters`, `logs/` and `scripts/`.
