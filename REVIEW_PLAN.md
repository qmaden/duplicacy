# Duplicacy fork: code review and remediation plan

Repository: https://github.com/qmaden/duplicacy (HEAD `5debf42`, 833 commits, about 26k lines of Go)
Reviewed: 2026-10-22

## 0. What was done

- Read all the core code: chunk/crypto (`duplicacy_chunk.go`, `duplicacy_config.go`), the chunk pipeline (`chunkoperator`, `chunkdownloader`, `chunkmaker`), backup/restore/copy (`backupmanager`), check/prune (`snapshotmanager`), entries (`entry`, `entrylist`, `snapshot`), and the storage layer (`storage`, `filestorage`, `sftpstorage`). Skimmed the cloud backends and the CLI (`duplicacy_main.go`).
- Reviewed the three newest commits closely: the chunk-id checksum, the new **separate metadata directory** feature (`0c0392d`), and the Go 1.26 upgrade.
- Ran the tools with the pinned toolchain (`GOTOOLCHAIN=go1.26.9`):
  - `go build ./...`: **passes**
  - `go test -race` (Chunk, Entry, EntryList, Utils, ChunkMaker, ChunkOperator, Backup, Snapshot): **passes**, no races found
  - `go vet`: 6 findings (listed below)
  - `staticcheck`: about 35 correctness or dead-code findings, plus style noise
  - `govulncheck`: **1 reachable vulnerability** (GO-2026-6809 in `crypto/x509`, fixed in go1.26.10)
- Environment note: the local Go **1.27.3** linker crashes while building the test binary (`index out of range` in `cmd/link`). This is a toolchain bug, not a problem in this code. Build with 1.26.x.

Severity scale: **Critical**, **High**, **Medium**, **Low**, **Info**. "Confirmed" means verified by reading the code path end to end or by tool output.

---

## 1. Findings

### 1.1 Security

| # | Sev | Finding | Location |
|---|-----|---------|----------|
| S1 | **High** | **Restore has a path traversal and a symlink traversal.** Entry paths and symlink targets come straight from snapshot metadata and are joined with `path.Join(top, entry.Path)`, with no validation. A path such as `../../home/u/.ssh/authorized_keys` writes outside the restore root. So does a crafted entry pair: a symlink `a` pointing to `/etc`, followed by the file `a/passwd`. `MkdirAll`, `OpenFile` and `Rename` all follow symlinks. Restore also re-applies uid/gid and mode bits, including setuid. Anyone who can write to an **unencrypted** storage (no password is the default) can use this, as can a malicious source in `copy`. Encrypted storages are protected only because forging metadata requires the keys. | `backupmanager.go:743-787, 836-860, 1157-1548`; `utils_others.go:89` |
| S2 | **High** | **SFTP host-key checking is trust-on-first-use, keyed by `IP:port`.** An unknown key is written to `known_hosts` silently. If DNS points the hostname at a new IP, the attacker's key is accepted with no prompt, which allows a silent man-in-the-middle and capture of passwords or data. `~/.ssh/known_hosts` is not consulted. The `remoteAddress` variable (meant to strip `:22`) is computed and then never used. | `storage.go:270-316` |
| S3 | Medium | **OAuth refresh-token files are written world-readable (0644)** for ACD, Hubic and OneDrive. | `acdclient.go:211`, `hubicclient.go:243`, `oneclient.go:267` |
| S4 | Medium | **The default key derivation is weak**: PBKDF2-SHA256 with 16,384 iterations (current guidance is ≥600k, or Argon2id), and the minimum password length is 8. The iteration count is stored per config, so a stronger default for new storages is backward compatible. | `config.go:58`, `utils.go:160` |
| S5 | Medium | **Malformed chunks can crash the client, and memory use is unbounded.** The erasure-coding header is parsed **before** AEAD authentication, so this applies to encrypted storages too: (a) `shardSize` divides by `config.DataShards` instead of the header value, so a planted EC chunk on a storage without EC causes a divide-by-zero panic; (b) `chunkSize` comes from the header and is passed unchecked to `Truncate`, causing a panic; (c) `lz4.Decode` and `zstd.NewReader` have no output-size limit (a decompression bomb). | `chunk.go:470-602, 701, 719` |
| S6 | Low | **Secrets are echoed to the terminal** (`showPassword=true`) for the S3 secret, B2 key, Azure key, OAuth client secret, Swift key, WebDAV/SMB password, and Storj key and passphrase. | `storage.go:588, 662, 680, 700, 774, 801, 828, 857-858, 894` |
| S7 | Low | **RSA private keys are decrypted with the legacy, deprecated `x509.DecryptPEMBlock`** (unauthenticated legacy PEM encryption), and its error is discarded. A wrong passphrase therefore shows up as a confusing parse error. | `config.go:662-666` |
| S8 | Low | **The `sftpc` mode enables arcfour, 3des-cbc and DH-group1-SHA1.** The mode is opt-in but there is no warning. There is also no SSH dial timeout, so an unresponsive host hangs forever. | `sftpstorage.go:54-76` |
| S9 | Low | **The snapshot cache holds decrypted metadata** (file names, sizes, hashes) in plaintext with file mode 0644. Only the 0744 directory mode, which lacks the traverse bit, protects it, and that is accidental. Use 0700/0600. | `filestorage.go:182`, `backupmanager.go:112` |
| S10 | Low | **Script output is logged as a format string** (`LOG_INFO(id, line)`), so `%` sequences in output are mangled. Scripts also receive the full `os.Args`. | `duplicacy_main.go:203-207` |
| S11 | Info | **Unencrypted storages use the public key `"duplicacy"`** for chunk IDs and the chunk seed. Anyone with read access can test whether a known file is stored (confirmation-of-file). Encrypted chunks still leak chunk sizes (256-byte padding). Document this. | `config.go:32, 250-254` |
| S12 | Low | **Dependencies are stale.** The stdlib vulnerability is reachable (fixed in 1.26.10). `aws-sdk-go` v1.30.7 dates from 2020 and v1 is end-of-life. `cloud.google.com/go` is v0.38 and `google.golang.org/api` is v0.21. A 2018 Azure SDK fork, a 2017 goamz fork, `msgpack` v4 and the unmaintained `minio/blake2b-simd` remain. govulncheck reports 13 more vulnerabilities in imported packages or required modules that the code does not call. | `go.mod` |

### 1.2 Business logic and correctness

| # | Sev | Finding | Location |
|---|-----|---------|----------|
| B1 | **High** (confirmed: vet + staticcheck) | **`check` silently passes a corrupt snapshot.** `fmt.Errorf(...)` is missing `err =`. When an entry has `EndChunk < StartChunk`, the callback returns `false`, iteration stops, and `err` is still `nil`. The snapshot is reported OK and **all remaining entries go unchecked**. | `snapshotmanager.go:2609-2612` |
| B2 | Medium | **The new separate-metadata feature deduplicates by chunk ID across both directories.** `chunkCache` (the previous snapshot's chunks, including metadata) is keyed by ID only. A file chunk whose content matches an existing metadata chunk is treated as already uploaded and never written to `chunks/`. Only metadata lookups fall back across directories, so a later **restore fails with "chunk can't be found"**. `copy` was fixed to key by directory; `backup` was not. The probability is low but the impact is data loss. | `backupmanager.go:172-181, 449-464, 1034-1040`; `storage.go` `FindChunk` |
| B3 | Medium | **Erasure-coding decode uses the config's shard count instead of the header's**, so any chunk whose EC parameters differ from the current config decodes incorrectly or panics (see S5). | `chunk.go:487` |
| B4 | Medium | **Resuming an incomplete backup reuses chunks without checking that they still exist.** This is checked only on fast-listing storages, and only for the very first backup. The leftover chunks belong to no snapshot, so prune fossilizes them. If the host stays "dormant" for 7 days (or another `repo@host` ID makes the collection deletable), they are deleted, and the resumed snapshot then references missing chunks. | `backupmanager.go:183-218`; `snapshotmanager.go:142-155` |
| B5 | Medium | **`check -chunks` never re-verifies a chunk once it has passed.** The `verified_chunks` cache stores a timestamp but never uses it, so later bit rot is never detected. | `snapshotmanager.go:1059-1068` |
| B6 | Medium | **Prune safety depends on client clocks and local state.** Fossil deletion compares the snapshot `EndTime` (backup client clock) with the collection `EndTime` (prune machine clock), and strongly consistent storages get **zero** margin. Fossil collections exist only in the local cache, and nothing prevents two concurrent prunes. Losing the cache leaks fossils; clock skew can delete fossils too early. | `snapshotmanager.go:69-160, 1864+` |
| B7 | Low/Med | **`-keep` retention policies must be given in descending age order, but are neither sorted nor validated.** Misordered input silently produces a different retention outcome. | `snapshotmanager.go:1924-1970, 2207-2212` |
| B8 | Low | **Restore deletes the existing file before renaming the temporary file over it.** A crash in between loses the file. On POSIX, `rename` already replaces atomically. | `backupmanager.go:1538-1548` |
| B9 | Low | **Restore and quick-mode backup trust `StartChunk`/`EndChunk` from the snapshot without a bounds check**, so a corrupt snapshot causes an index panic. | `backupmanager.go:316-323, 816, 1246-1250` |
| B10 | Low | **`ChunkOperator.AddTask` enqueues before incrementing the active counter.** The counter can briefly go negative, and `Stop()` treats `<0` as "already stopped". Waiting is done by `Sleep(100ms)` polling. | `chunkoperator.go:105-145` |
| B11 | Low | **Restore progress and ETA use a stale total** (`downloader.totalChunkSize` is adjusted but `operator.totalChunkSize` is not). In copy, `copiedChunks++` is a data race, and every chunk is reported as "Copied", even ones that were skipped. | `chunkdownloader.go:151`; `backupmanager.go:1735-1752` |
| B12 | Low | **`json:'data-shards'` is an invalid struct tag**, so the fields serialize as `DataShards`/`ParityShards`. **Do not just fix the tag**: existing configs on disk use these keys. Make it explicit with `json:"DataShards"`. | `config.go:88-89` |
| B13 | Low | **`ComputeFileHash` ignores read errors** and returns the hash of a partial file. | `config.go:375-392` |
| B14 | Low | **Exhaustive prune collects in-flight `.tmp` uploads** and deletes them on the next run, which breaks slow, long uploads. Add an mtime age threshold. | `snapshotmanager.go:2480-2497` |
| B15 | Low | **The in-place restore check uses a raw string `HasPrefix` on paths**, so `/data/a` matches `/data/ab/...`. | `backupmanager.go:638` |
| B16 | Low | **`check -rewrite` fossilizes the original chunk before re-uploading it.** If the upload fails, only a WARN is logged and the chunk survives only as a fossil. | `chunkoperator.go:467-497` |
| B17 | Low | **go vet findings**: the Samba `host:port` format breaks IPv6; a Swift `context.WithTimeout` cancel function is discarded (context leak). | `sambastorage.go:34`, `swiftstorage.go:116` |
| B18 | Info | **License**: commercial use of derivative works requires per-computer licenses. Settle this before distributing the fork commercially. | `LICENSE.md` |

### 1.3 Performance

| # | Impact | Finding | Location |
|---|--------|---------|----------|
| P1 | **High** | **Restore leaks a chunk per file.** `defer ...PutChunk(localChunk)` evaluates `localChunk` (nil) when the defer is registered. Every restored file that reuses local data allocates a new buffer up to the maximum chunk size (16 MiB by default) that never returns to the pool. The result is GC churn and "N chunks have been allocated" warnings. | `backupmanager.go:1465-1466` |
| P2 | **High** | **A new zstd encoder and decoder is created for every chunk.** These are heavyweight objects (MBs of state). Pool them, or use `EncodeAll`/`DecodeAll` with shared instances. | `chunk.go:327, 719` |
| P3 | Medium | **Metadata sequences download serially** (one blocking `Download` at a time). `prune`, `check`, `list` and the backup preamble walk every snapshot's chunk sequence this way, so a high-latency backend pays one round trip per chunk. Add prefetching or parallel fetches. | `snapshotmanager.go:298-307`; `snapshot.go` `ListRemoteFiles` |
| P4 | Medium | **`CleanSnapshotCache` sends a HEAD storm after every backup.** For every cached snapshot it checks each sequence chunk against the cache and then the remote storage, one call at a time. | `snapshotmanager.go:420-440` |
| P5 | Medium | **Each new chunk costs a storage round trip** (a `FindChunk` HEAD or stat) whenever it is missing from the previous snapshot's cache. The full listing is used only for the very first backup on fast-listing storages. Consider an optional listing cache or Bloom filter. | `chunkoperator.go:560`; `backupmanager.go:188-212` |
| P6 | Medium | **The memory ceiling grows with CPU count.** There are two pools (`NumCPU*16` chunks and `NumCPU*16` buffers), and every chunk buffer is grown to `MaximumChunkSize`. On a 32-core host, retained memory after a burst can reach several GB. Cap the pools by thread count instead. | `config.go:274, 288-315`; `chunk.go:38, 107-123` |
| P7 | Low | **`WaitForCompletion`/`Stop` busy-wait with 100 ms sleeps.** Replace with `sync.WaitGroup`. | `chunkoperator.go:104-125` |
| P8 | Low | **`GetChunkIDFromHash` is recomputed repeatedly** (keyed BLAKE2 per call) in prefetch logs and loops. Compute it only when debug logging is on, or memoize it. | `chunkdownloader.go:115-126, 175, 196` |

### 1.4 Engineering hygiene

- There is no CI (`.github` holds only an issue template). Integration tests are shell scripts that need real backends.
- 38 files are not `gofmt`-clean, and indentation mixes tabs and spaces.
- There are no fuzz tests for the untrusted decoders: `Chunk.Decrypt`, `Entry.DecodeMsgpack`, snapshot JSON, and the config.
- Dead code: `fileEncoder`, `ListBlobsParameters`, unused fields and vars (staticcheck U1000).
- `io/ioutil` is used throughout, and `math/rand` serves as the temp-file suffix generator.

---

## 2. Compatibility guardrails (apply to every change)

Duplicacy's value is that existing storages stay readable. Every fix must keep:

1. **Chunk format**: the banners `duplicacy\000`, `\002` (RSA) and `\003` (EC), the PKCS7-style 256-byte padding, `LZ4 ` and `ZSTD` prefixes, and the "wrong highwayhash" arm64 fallback.
2. **Config JSON keys**, including the accidental `DataShards`/`ParityShards`, and the old-format config with static salt.
3. **Hashing**: keyed BLAKE2b-256 for level ≥100 and HMAC-SHA256 for legacy levels. If `minio/blake2b-simd` is swapped for `x/crypto/blake2b`, add golden-vector tests first.
4. **Snapshot formats**: v0 JSON and the msgpack entries.
5. **Two-step fossil collection semantics**, which other clients (the GUI and older CLIs) rely on.

Add a **golden-fixture test suite**: a checked-in storage directory with encrypted, RSA, EC, LZ4/zstd/zlib, v0 and v1 snapshots, restored byte-for-byte in CI, before touching any of the above.

---

## 3. Remediation plan

### Phase 0: Baseline and safety net (1–2 days)

| Task | Done when |
|------|-----------|
| ~~Bump `toolchain` to **go1.26.10** (S12)~~ **Done 2026-10-22** in `go.mod` | Verified: build OK, race tests OK, `govulncheck` reports 0 reachable vulnerabilities |
| ~~Unblock Go 1.27 (section 5)~~ **Done 2026-10-22** (`internal/wronghighwayhash`) | Verified: build, vet and race tests pass on 1.27.3 without `-tags noasm` |
| Add GitHub Actions: build (linux/mac/windows), `go vet`, `staticcheck` (SA*/U1000 only), `govulncheck`, `go test -race` for unit tests | CI is green on main and required for PRs |
| Add golden-fixture restore tests (section 2) | Fixtures restore identically in CI |
| Run `gofmt` in a single isolated commit | `gofmt -l` is empty; no behavior change |

### Phase 1: Critical fixes (week 1)

| Task | Fix | Test |
|------|-----|------|
| **B1** check bug | `err = fmt.Errorf(...)` | Unit test: an entry with `EndChunk < StartChunk` makes `CheckSnapshot` return an error |
| **S1** restore traversal | Add `safeJoin(top, rel)`: reject absolute paths, `..` components, NUL, and Windows drive/UNC prefixes; resolve each parent with `Lstat` and refuse to traverse symlinks created inside `top` (or use `os.Root`, available in Go ≥1.24, for all restore file operations); strip setuid/setgid unless `-set-owner` is given and the user is root; validate every entry when it is decoded | Tests with malicious entries (`../x`, `/etc/x`, symlink-then-child, `C:\x`) restore nothing outside `top` |
| **P1** chunk leak | `defer func(){ PutChunk(localChunk) }()` | Restore 1,000 small files with local copies; chunk-allocation count stays bounded |
| **B3 + S5** EC decode | Use the header's `dataShards`/`parityShards`; validate `0 < data ≤ 256`, `parity ≤ 256`, and `chunkSize ≤ len(buffer)` and within a sane maximum; return errors instead of panicking | Fuzz `Chunk.Decrypt` for 10 minutes in CI with no panics |
| **B2** metadata dedup | Key `chunkCache` by `dir+id` (as copy now does), **or** make `FindChunk(isMetadata=false)` fall back to `metadata/`; also scan `metadata/` during the first-backup listing | Test: with separate metadata on, a file whose content equals a metadata chunk restores correctly |

### Phase 2: Security hardening (weeks 2–3)

| Task | Fix |
|------|-----|
| **S2** SFTP host keys | Key entries by **hostname**:port (as OpenSSH does), also read `~/.ssh/known_hosts` via `x/crypto/ssh/knownhosts`, and **prompt** on an unknown key in interactive mode (fail in background mode unless a `-accept-new-host-key`-style flag or a stored fingerprint exists); add `Timeout` to `ClientConfig` |
| **S3** token files | Write them 0600 (and chmod existing files on load) |
| **S9** cache permissions | 0700 directories and 0600 files for the cache and preferences |
| **S4** KDF | New default ≥600k PBKDF2 iterations (or add an Argon2id config version); `duplicacy password` re-wraps with the new parameters; recommend a 12+ character password; cap iterations read from storage to prevent denial of service |
| **S5** decompression limits | Bound LZ4 and zstd output at `MaximumChunkSize` (`zstd.WithDecoderMaxMemory`, check the LZ4 header length) |
| **S6** secret echo | Prompt masked for anything named key, secret, password or passphrase |
| **S7** RSA keys | Support encrypted PKCS#8; warn on legacy PEM encryption; surface the passphrase error |
| **S8** legacy SSH | Print a warning when `sftpc` legacy algorithms are negotiated |
| **S10** script output | Log `LOG_INFO(id, "%s", line)`; document that scripts receive the CLI arguments |
| Fuzzing | Native Go fuzz targets for `Decrypt`, `DecodeMsgpack`, `CreateSnapshotFromDescription` and config `UnmarshalJSON` |

### Phase 3: Business-logic robustness (weeks 3–4)

| Task | Fix |
|------|-----|
| **B4** resume safety | Before reusing an incomplete snapshot, verify that its uploaded and preserved chunks exist (batched `FindChunk`, or a listing on fast storages) and store the incomplete snapshot's creation time; discard it if any chunk is missing |
| **B5** re-verification | Add `-reverify-days N` (default, say, 90) that ignores `verified_chunks` entries older than N days; optionally re-verify a random X% each run |
| **B6** prune safety | Always apply a minimum skew margin (for example 1 h, even on strongly consistent storages); write a lease file on the storage (`locks/prune`) with owner and expiry to block concurrent prunes; optionally mirror fossil collections to the storage so a lost cache doesn't leak fossils |
| **B7** retention | Sort policies by age descending and reject duplicate ages; log the effective policy |
| **B8** atomic restore | Rename directly on POSIX; keep remove-then-rename only on Windows |
| **B9** bounds | Run `entry.check(chunkLengths)` on every remote entry in restore, quick-mode backup and `cat`/`diff` |
| **B10/B11** concurrency and progress | Increment before enqueueing (or switch to a `WaitGroup`); atomic counters in copy; report "Skipped" correctly; restore progress uses the adjusted total |
| **B12–B17** | Explicit config tags, read-error handling, `.tmp` age threshold (for example >24 h), `filepath.Rel`-based prefix check, upload-then-fossilize ordering for rewrite, `net.JoinHostPort`, `defer cancel()` |

### Phase 4: Performance (weeks 4–6, measure before and after)

| Task | Expected gain |
|------|---------------|
| **P2** pool zstd encoders/decoders (`sync.Pool` per level) | Large CPU and allocation reduction with zstd |
| **P3** prefetch N metadata chunks in parallel in `sequenceReader` / `DownloadSequence` | `prune`/`check`/`list` speedup roughly proportional to latency × thread count |
| **P4** in `CleanSnapshotCache`, use a single listing of the cache plus a storage listing (or skip the remote check when the snapshot file still exists) | Removes O(snapshots × chunks) HEAD calls after each backup |
| **P5** optional chunk-existence index (listing cache with a TTL, or a Bloom filter) for non-first backups | Fewer per-chunk HEAD/stat calls on large incremental backups |
| **P6** size pools by `threads` (not `NumCPU*16`) and shrink oversized buffers on release | Lower and predictable resident memory |
| **P7/P8** `WaitGroup` instead of sleep polling; compute IDs only when debug logging is on | Small latency and CPU wins |
| Add `go test -bench` for chunk encrypt/decrypt and the chunk maker; track results in CI | Catches regressions |

### Phase 5: Dependency modernization (ongoing, after the golden fixtures exist)

1. `minio/blake2b-simd` → `golang.org/x/crypto/blake2b` (verify keyed-mode golden vectors).
2. `aws-sdk-go` v1 → v2 (S3, Wasabi, MinIO paths).
3. `cloud.google.com/go`, `google.golang.org/api`, Azure SDK fork → current SDKs.
4. `vmihailenco/msgpack` v4 → v5 (byte-compat tests on snapshot entries).
5. `io/ioutil` → `io`/`os`; remove dead code flagged by staticcheck.

---

## 4. Suggested order and effort

| Order | Items | Effort |
|-------|-------|--------|
| 1 | Phase 0, including AGENTS.md, CONTRIBUTING, BUILDING/TESTING docs (section 8.5) | S |
| 2 | B1, P1, toolchain bump: one-line fixes with high value | XS |
| 3 | S1, B3/S5, B2 | M |
| 4 | S2, S3, S9, S6 | M |
| 5 | B4, B5, B6, B7 | M–L |
| 6 | P2, P3, P4, P6 | M |
| 7 | S4, S7, dependency upgrades | L |
| Ongoing | FORMAT.md with fixtures, SECURITY docs, CHANGELOG/UPGRADING, generated command reference (section 8) | M, spread across phases |

XS < ½ day, S ≈ 1–2 days, M ≈ 3–5 days, L ≈ 1–2 weeks.

---

## 5. Go version

| Version | Status |
|---------|--------|
| go1.26.9 (old pin) | Builds; one reachable stdlib vulnerability (GO-2026-6809, `crypto/x509`) |
| **go1.26.10 (new pin)** | **Builds, race tests pass, 0 reachable vulnerabilities.** `go.mod` `toolchain` line updated. Go 1.26 stays supported until Go 1.28 ships (~Feb 2027). |
| go1.27.3 | Originally **failed to link** (`non-function sym ... gilbertchen/highwayhash.zipperMerge t=SRODATA passed to GetFuncDwarfAuxSyms`). **Fixed 2026-10-22**, see 5.1. Now builds, vets, passes race tests, and runs an encrypted erasure-coded backup/check/restore end to end |

### 5.1 The highwayhash fork: root cause and fix (done, not yet committed)

**Root cause.** `gilbertchen/highwayhash` (a copy of minio/highwayhash 1.0.1) declares its arm64 constant table as the **global** assembly symbol `·zipperMerge`. That name collides with the Go function `zipperMerge` in `highwayhash_generic.go`. The linker resolved the assembly's `MOVD $·zipperMerge(SB)` to `zipperMerge.abi0`, a compiler-generated **code** wrapper, so the NEON `TBL` instructions used 48 bytes of machine code as their lookup table. This is *why* the arm64 hash was "wrong" in 1.0.1 (minio fixed it in 1.0.2 by renaming the symbol to `asmZipperMerge`). Go 1.27's linker now rejects the collision outright.

**Hidden risk found.** The "wrong" hash depended on compiler output:
- Bytes 0x00–0x0F (zipper indices) are the wrapper's prologue; only two of them select data.
- Bytes 0x10–0x1F (multiply setup) are all ≥ 32, so `TBL` returns zeros and the multiply step does nothing. But byte 0x1C is the low byte of a relative `CALL` offset, so a change in code layout could have pushed it below 32 and silently changed the hash.
- Bytes 0x20–0x2F differ between Go 1.21 and 1.26 but don't affect the result.

The output happened to be identical on Go 1.21 through 1.26 (verified), but nothing guaranteed it.

**Fix.** Instead of forking the GitHub repository, the code was copied into this repo as **`internal/wronghighwayhash`** (Apache-2.0 license kept):
- **Pure Go, no assembly.** It emulates the arm64 `UPDATE` macro instruction by instruction (`TBL` lookups, `umull`/`umull2`, zipper merges), with the 48 table bytes **pinned** as data. It no longer depends on the compiler, the linker or the CPU.
- `src/duplicacy_chunk.go` imports it, and the shard-hash fallback no longer requires `runtime.GOARCH == "arm64"`. Erasure-coded chunks written by CLI 3.0.1 on arm64 can now be read on amd64 too; before, all their shards failed verification there.
- `github.com/gilbertchen/highwayhash` is removed from `go.mod`.

**Verification.**
| Check | Result |
|-------|--------|
| 844 golden vectors (4 keys including the all-zero key Duplicacy uses; lengths 0–200 plus large sizes up to 1 MiB), generated from the original assembly on arm64 with Go 1.26.10 and checked identical on Go 1.21.13 | All match, one-shot and incremental writes (`internal/wronghighwayhash/testdata/golden.txt`) |
| Differential test against the live original assembly: 200,000 random keys, messages (up to 4 MiB) and write splits | 0 mismatches |
| Golden tests on amd64 (Rosetta) and on Go 1.27.3 | Pass |
| New `TestErasureCodingWrongHighwayHash` (an EC chunk whose shard hashes are rewritten with the wrong hash decodes correctly and is flagged for rewrite), arm64 and amd64 | Pass |
| `go build`, `go vet`, `go test -race` on Go 1.26.10 and 1.27.3; `govulncheck` | Pass; 0 reachable vulnerabilities |

**Remaining options:**
- Raise the `go` directive or the toolchain pin to 1.27.x once Go 1.27 has been through a release cycle in CI (both toolchains now work).
- Optional: confirm the pinned bytes against an actual CLI 3.0.1 arm64 release binary or a storage it wrote. The current behaviour is preserved exactly; this would only confirm that 3.0.1 itself matched.

---

## 6. Upstream issues and PRs (gilbertchen/duplicacy) cross-checked against this code

Upstream has 338 open issues and 34 open PRs. Only 3 commits landed in 2026. Issue #690 ("THIS SOFTWARE IS ABANDONED") and its thread show that users see the project as unmaintained, and that the maintainer works through the forum, not GitHub. That makes a maintained fork valuable, and it also means fixes won't come from upstream.

### 6.1 Confirmed still broken in this code

| Upstream | Problem | Evidence here | Severity | Plan item |
|----------|---------|---------------|----------|-----------|
| #692, PR #693 | Restore path traversal | Same finding as S1, reported independently | High | S1 (Phase 1). PR #693 only rejects `..` paths; it does **not** cover symlinks (below) |
| PR #691 | Restore writes through an existing/stale symlink (CWE-59): `os.OpenFile` follows symlinks | Same root cause as S1 | High | S1: use `O_NOFOLLOW` / `os.Root` |
| #675 | Nil-pointer panic in `check -chunks -persist` | **New root cause found**: in `DownloadChunk`, `completeFailedChunk()` hands the broken chunk to the caller, but `chunk` is not set to nil, so the deferred `PutChunk` **also returns it to the pool** (`chunkoperator.go:309-313, 352-359`). Two goroutines then share one chunk; one `Reset`s it (nil hasher) while the other uses it. With `restore -persist`, the downloader later `PutChunk`s it again (double insertion into the pool) | **High** | **New N1** (Phase 1): set `chunk = nil` after the completion callback in `completeFailedChunk` |
| #664 (PR) | Swift: every operation fails once `timeout` seconds have passed since start | One `context.WithTimeout` created at init and stored in `storage.ctx`, then reused for all calls (`swiftstorage.go:116, 152`); `go vet` flags it too | High (Swift users) | N2: `context.Background()` per call; rely on `swift.Connection` timeouts |
| #666, #657 (PRs) | `-exclude-by-attribute` does nothing on Linux | `excludedByAttribute` looks for `duplicacy_exclude`, but Linux xattr names are namespaced (`user.duplicacy_exclude`) (`utils_linux.go`) | Medium | N3 |
| #658 | Every command (even `-h`) hangs for about 2 minutes on some Linux hosts | Stack trace in the issue: `gilbertchen/keyring` opens a D-Bus Secret Service session **inside `init()`**, with no timeout | Medium | N4: initialize the keyring lazily on first use, with a timeout; skip it when env or preference passwords are present |
| #540 | Backup exits **0** even when files were skipped | **Reproduced**: an unreadable file gives "1 file was not included due to access errors" with exit=0 | Medium (automation silently misses data) | N5: new exit code (or opt-in flag `-strict`) when anything was skipped |
| #429 | Restore aborts when a path changed between directory and file | **Reproduced**: "The path …/test is not a directory" (`backupmanager.go:769-772`) | Medium | N6: with `-overwrite`, replace mismatched types (remove the file or empty directory, then create) |
| #309, PR #559 | One read error in the middle of a file (Windows locked range, network glitch) **aborts the whole backup** | `chunkmaker.go:173-175, 211-212` → `LOG_ERROR` (fatal) | Medium | N7: treat it like an open failure (skip the file, record it as skipped, keep chunk-stream consistency) |
| #84 | Partial restore leaves parent directories root-owned with mode 0744 | `MkdirAll(parent, 0744)` (`backupmanager.go:856`); parent directory entries filtered out by patterns are never restored | Low/Med | N8: apply metadata for all ancestor directory entries in the snapshot even when filtered out |
| #363 | An interrupted *incremental* backup cannot resume; only hash-mode backups save an incomplete snapshot | `RunAtError` save is set only `if hashMode` (`backupmanager.go:384`) | Medium (large incrementals restart from scratch) | N9 (pairs with B4 resume verification) |
| #541, PR #542 | Restore is slow on many small files; prefetch stops at the first "not needed" chunk | `chunkdownloader.go:191-193` `if !task.needed { break }` | Medium (perf) | P9: apply PR #542 (`continue`), then parallelize the existing-file scan |
| #683, #689, #531 | A transient network error kills the backup; the OneDrive retry budget is consumed by honored `Retry-After` | Upload errors are fatal; per-backend retry logic is inconsistent | Medium | N10: shared retry-with-backoff wrapper in `ChunkOperator` (idempotent ops), time-based budget |
| #688 | Process exit sends TCP RST on S3 keep-alive connections | `S3Storage` has no `Close()`; process exits via `os.Exit` | Low | N11: add `Close()` to the `Storage` interface; call `CloseIdleConnections` on exit |
| #670 | Very slow writes to SMR USB drives (32 KiB writes) | `FileStorage.UploadFile` uses `io.Copy` through a rate-limited reader, which writes in 32 KiB pieces (`filestorage.go:185-186`) | Low/Med | P10: write `content` in one call (or 1 MiB blocks) when no rate limit is set |
| #639 | SFTP host authentication | Same as S2 | High | S2 |
| #638, #668 | Key derivation questions (derivation key used as BLAKE2 key, digesting the secret; KDF strength) | Matches S4; the key/message order is unusual but not exploitable; it cannot change without a format version | Info/Med | S4 |
| #676 | Content-defined chunking attacks (Truong et al., 2025) | Matches S11. Restic mitigated this in 0.18; this code has no mitigation | Low/Med | S11: evaluate keyed chunker-parameter hardening for new storages |
| #695, PR #696 | SSH-key prompt is confused with the RSA key prompt | Prompt text in `storage.go` | Low | Apply PR #696 |
| #687, #260 | Asks for a password or key file even when ssh-agent is available | Auth method ordering in `storage.go:516-542` | Low | Use the agent first when `SSH_AUTH_SOCK` is set |
| #680 | B2 keys created by the CLI (v4 API) loop forever on authorize | B2 client still uses the v1 API | Medium (B2 users) | N12: move to the B2 v3/v4 API |

### 6.2 Already fixed in this code (no action)

| Upstream | Notes |
|----------|-------|
| #631, #411 | Backup with only empty files: **reproduced as working** (revisions 1 and 2 OK) |
| #523 | Zero-byte snapshot files: `FileStorage` now `fsync`s before rename |
| #560 | `cat` of a large file running out of memory (fixed upstream in 2018) |
| PR #662 | Periodic save of `verified_chunks` (now every 5 minutes) |
| PR #655 (duplicate listing entries leading to destructive exhaustive prune) | Exhaustive prune now de-duplicates listed names (`uniqueFiles`). Switching to `ListObjectsV2` is still worth doing for performance |

### 6.3 Open PRs worth porting (after review and tests)

`#693` + `#691` (restore safety, superseded by the fuller S1 fix), `#696` (prompts), `#542` (restore prefetch), `#664` (Swift), `#657`/`#666` (xattr), `#559` (locked files/zero-length), `#655` (S3 ListObjectsV2), `#654` (B2 listing), `#645`/`#646` (OneDrive listing and retry), `#684` (read-only files on overwrite), `#642` (check/prune optimization, claimed 18x; needs careful review), `#623` (SFTP proxy), `#624` (AWS STS tokens), `#663` (ioutil removal). Avoid `#604` (FastCDC): it changes chunk boundaries, so it is a new storage format.

### 6.4 Updated Phase 1 (critical) list

1. B1 (`check` error swallowed), **N1** (chunk double-ownership, #675), P1 (restore chunk leak): one-line fixes.
2. S1 + PR #691 scope (restore traversal and symlinks).
3. **N2** (Swift context) and B3/S5 (EC header).
4. B2 (separate-metadata dedup).
5. **N5** (exit code on skipped files), behind a flag for compatibility.

---

## 7. Should this be rewritten in Rust?

**Recommendation: no full rewrite now.** Fix and modernize the Go code (phases 0–5). If Rust is a strategic goal, start with a small, independent Rust component (below), and decide on a full rewrite only after it proves format compatibility.

### 7.1 What Rust would and would not fix

| Finding class | Would Rust prevent it? |
|---------------|------------------------|
| Path traversal (S1), SFTP host key trust (S2), file permissions (S3/S9), KDF strength (S4) | **No**: these are design and logic bugs |
| `check` error swallowed (B1), resume and prune logic (B4–B7), exit codes (#540), dir/file swap (#429) | **No** |
| Chunk double ownership in the pool (N1/#675), restore chunk leak (P1), counter races (B10/B11) | **Yes**: ownership and `Send`/`Sync` would reject these at compile time |
| Panics on malformed input (S5) | Partly: becomes a clean error with `Result` and checked indexing, but still needs explicit validation |
| Memory use (#602, P6) | Yes: no GC and explicit buffer reuse would lower memory |
| CPU throughput | Small gain: hot paths (AES-GCM, BLAKE2, zstd, Reed-Solomon) are already SIMD/assembly in Go; backups are mostly I/O- or network-bound |

Most of what is wrong here is logic, not memory safety. Go is already memory-safe; a rewrite would fix about 20% of the findings and risk introducing new logic bugs in a tool where a bug means data loss.

### 7.2 Cost of a compatible rewrite

The rewrite must read and write existing storages **byte for byte**:

- **Chunker**: buzhash with a random table derived from `SHA256(seed)` iterated, power-of-two masks, metadata-chunk variant. Must match boundaries exactly.
- **Crypto and hashing**: keyed BLAKE2b-256 (Rust `blake2::Blake2bMac` is compatible), HMAC-SHA256 legacy mode, AES-256-GCM, RSA-OAEP-SHA256, PBKDF2-SHA256 with old and new config formats, and the reversed derivation-key order (#638).
- **Compression**: the `bkaradzic/go-lz4` framing (4-byte length + block; likely matches `lz4_flex` size-prepended mode, verify with fixtures), zstd, zlib.
- **Erasure coding**: the `klauspost/reedsolomon` matrix layout (Rust `reed-solomon-erasure` ports the same Backblaze design; must verify), HighwayHash, and the **incorrect arm64 HighwayHash variant**.
- **Metadata**: hand-rolled msgpack entry encoding, v0 JSON snapshots, Go-specific JSON details, the `DataShards` key quirk.
- **Fossil collection semantics** shared with the GUI and older CLIs.
- **17 storage backends**. Apache OpenDAL covers most of them (S3/Wasabi/MinIO, GCS, Azure Blob, B2, Google Drive, OneDrive, Dropbox, Swift, WebDAV, SFTP). Hubic and Amazon Cloud Drive are dead services; Storj, SMB and FileFabric need custom work.
- **Platform features**: Windows VSS, macOS APFS snapshots, xattrs, keyrings on three OSes, Windows path quirks (`\\?\`, junctions).

Rough estimate: **6–12 engineer-months** to reach parity, plus a long soak period before it can be trusted with real backups.

### 7.3 Other constraints

- **License**: `LICENSE.md` makes commercial use of derivative works subject to per-computer licenses. A Rust port written from this code is most likely still a derivative work; a new implementation of the published format is less clear-cut. Get legal advice before investing heavily, whatever the language.
- **Mature alternatives exist**: restic (Go, BSD-2), **rustic** (Rust, restic-compatible repositories, Apache/MIT), Kopia (Go, Apache-2), Borg 2. If the goal is "a modern, maintained, permissively licensed backup tool" rather than "keep existing Duplicacy storages", migrating may be cheaper than rewriting.

### 7.4 Low-risk way to try Rust

1. Build a **read-only Rust verifier/restorer** (`duplicacy-verify`) for the storage format: parse config, snapshots and chunks; `check -chunks`; `restore` to a directory. Start with the local filesystem backend, then S3 via OpenDAL.
2. Run it in CI against the golden fixtures (section 2), and against the Go implementation's output for randomized repositories.
3. Value even if the rewrite never happens: an **independent implementation** that cross-checks backups is a strong integrity guarantee, and it specifies the format precisely.
4. Go/no-go point: if the verifier reaches 100% fixture compatibility within about 6–8 weeks, consider writing (backup and prune) in Rust next, behind the same fixture suite. Otherwise keep investing in Go.

---

## 8. Documentation and AI-agent guidance (AGENTS.md)

### 8.1 Current state

| Item | Problem |
|------|---------|
| `README.md` | Describes upstream, not this fork. Every "how to" link (Quick Start, commands, building, storage setup) points to the **upstream GitHub wiki**, which this fork can't edit and which isn't versioned with the code. The comparison table and benchmark claims are from 2017 (and questioned in upstream #679) |
| `DESIGN.md`, `GUIDE.md` | Only links to the upstream wiki |
| `ACKNOWLEDGEMENTS.md` | Out of date: lists `tmc/keyring`, `pcwizz/xattr`, `tj/go-dropbox`, but not `klauspost/compress`, `klauspost/reedsolomon`, `minio/highwayhash`, `pkg/sftp`, `storj.io/uplink`, `cloudsoda/go-smb2`, `vmihailenco/msgpack`, `golang.org/x/*` |
| `.github/ISSUE_TEMPLATE.md` | Tells people not to use GitHub issues ("use the forum"). Wrong for this fork |
| Storage-format spec | **Missing.** Only scattered wiki pages and code comments. Needed for compatibility, the golden fixtures, and any Rust verifier (section 7) |
| Security policy and threat model | Missing (no `SECURITY.md`, no statement of what an untrusted storage can do) |
| Building and testing | Integration tests are undocumented shell scripts that expect `../duplicacy_main`; nothing explains the toolchain pin or the Go 1.27 blocker |
| CHANGELOG / upgrade notes | Missing. Behaviour changes in this plan (exit codes, KDF default, restore safety) need user-facing notes |
| `AGENTS.md` / `CLAUDE.md` | Missing. AI coding agents have no guidance on build commands, compatibility invariants, or dangerous operations |
| Godoc | Many exported types and functions have no doc comments; error strings are inconsistent |

**Licensing note:** the upstream wiki and forum content is Acrosync's. Write new docs from the code; don't copy wiki pages verbatim unless permission is granted.

### 8.2 Target documentation set (all versioned in the repo)

```
AGENTS.md                  # guidance for AI coding agents (cross-tool standard)
CLAUDE.md                  # one line: "See AGENTS.md" (Claude Code reads CLAUDE.md)
README.md                  # rewritten: fork identity, status, install/build, quick start, docs index
CONTRIBUTING.md            # dev setup, branch/PR rules, test requirements, compatibility checklist
SECURITY.md                # supported versions, private reporting, disclosure policy
CHANGELOG.md               # Keep-a-Changelog format, one entry per release
ACKNOWLEDGEMENTS.md        # regenerated from go.mod (go-licenses), checked in CI
docs/
  ARCHITECTURE.md          # components, data flow (backup/restore/copy/check/prune), concurrency model
  FORMAT.md                # storage format spec: config (both formats), chunk (banners, AES-GCM,
                           #   padding, LZ4/ZSTD/zlib, RSA, erasure coding incl. arm64 wrong-hash quirk),
                           #   chunk IDs and nesting, metadata dir, snapshot JSON + msgpack entries,
                           #   fossils and fossil collections, incomplete snapshots, cache layout
  SECURITY_MODEL.md        # trust boundaries: what a storage attacker / local user / network attacker
                           #   can do with encrypted vs unencrypted storages; known limitations (CDC, sizes)
  PRUNE_SAFETY.md          # two-step fossil collection explained, safe-operation rules, -exclusive risks
  BUILDING.md              # toolchain pin, cross-compiling, release builds, why not Go 1.27 / -tags noasm
  TESTING.md               # unit, race, fuzz, golden fixtures, integration scripts, backend test config
  UPGRADING.md             # behaviour changes per version (exit codes, KDF, restore safety, separate-metadata)
  commands/                # one page per command, generated from the CLI definitions (8.4)
  storage-backends.md      # URL syntax, credentials and env vars, per-backend caveats (retries, consistency)
  adr/                     # architecture decision records (e.g., "0001 keep Go, Rust verifier first")
.github/
  ISSUE_TEMPLATE/bug.yml, feature.yml   # replace "use the forum"
  PULL_REQUEST_TEMPLATE.md               # includes the compatibility checklist from section 2
```

### 8.3 AGENTS.md: proposed content

Keep it short (roughly 150–250 lines) and command-oriented. Agents follow it literally, so every command in it must work. Sections:

1. **Project in one paragraph**: what Duplicacy is, what this fork changes, and the rule that existing storages must stay readable.
2. **Repository map**: `duplicacy/` (CLI), `src/` by area (chunk/crypto, chunk pipeline, backup/restore, snapshot manager and prune, entries, storage interface plus one file per backend), `integration_tests/`, `docs/`.
3. **Build and test commands** (exact, copy-pasteable):
   - `GOTOOLCHAIN=go1.26.10 go build ./...`
   - `cd src && go test -race -count=1 -run '<pattern>' .` (which test patterns are safe offline)
   - `go vet ./...`, `staticcheck ./...`, `govulncheck ./...`, golden-fixture tests, fuzz targets
   - **Do not** build with Go 1.27 or `-tags noasm` (explain why in one line, link BUILDING.md)
4. **Compatibility invariants**: never change without a format-version plan: chunk banners and layout, padding, compression prefixes, config JSON keys (including `DataShards`/`ParityShards`), the hash and key-derivation order, the wrong-HighwayHash fallback, the msgpack entry layout, and fossil-collection semantics. Link `docs/FORMAT.md`.
5. **Error-handling conventions**: `LOG_ERROR` panics and the process exits through `CatchLogException` (so code after it doesn't run); when to use `LOG_WERROR`; log-ID naming; return errors from decoders instead of panicking; wrap with `%w`.
6. **Concurrency rules**: chunk pool ownership (whoever holds a `*Chunk` must `PutChunk` exactly once; set the variable to nil after handing it off; see N1), `ChunkOperator` lifecycle, which maps are touched from worker goroutines.
7. **Security-sensitive areas** (need extra review and tests): restore path handling, `checkHostKey`, `config.go` key derivation, `Chunk.Decrypt`, credential prompts and storage, file permissions.
8. **Safety rules for agents**:
   - Never run `prune`, `prune -exclusive`, `check -rewrite` or `copy` against a real storage; use temporary local storages under the test directory only
   - Never commit credentials; backend tests read from a git-ignored config
   - Don't reformat untouched files (gofmt is applied in its own commit)
   - Keep changes minimal and add a regression test for every bug fix
9. **Definition of done**: build, vet, race tests, fixtures and govulncheck pass; CHANGELOG entry; docs updated when behaviour or flags change.
10. **Where to find things**: pointers to `REVIEW_PLAN.md` findings IDs (B1, S1, N1 …) so agents can reference them in commits.

Optionally add short nested files later (for example `src/AGENTS.md` for backend-specific rules) only if the root file grows beyond about 250 lines.

### 8.4 Keeping docs correct automatically

| Mechanism | Purpose |
|-----------|---------|
| Generate `docs/commands/*.md` from the `urfave/cli` definitions (small `go run ./tools/gendocs`) | Flags and help text can't drift from the code; CI fails if generated docs are stale |
| CI job runs the commands listed in AGENTS.md and BUILDING.md | Agent and contributor instructions stay executable |
| Markdown lint plus link check (e.g., `lychee`) | No dead links (important after moving off the upstream wiki) |
| `go-licenses report` → ACKNOWLEDGEMENTS check | Third-party license list matches `go.mod` |
| Golden fixtures referenced by FORMAT.md section | Every format statement in the spec has a fixture that proves it |
| PR template checklist | Asks: format change? behaviour change (UPGRADING)? docs updated? |
| Doc comments on exported identifiers in `src/` (revive/staticcheck ST1020-1022 enabled gradually) | Better godoc and better context for agents |

### 8.5 Order and effort (integrated with sections 3–4)

| When | Docs work | Effort |
|------|-----------|--------|
| **Phase 0** (before code fixes) | `AGENTS.md` + `CLAUDE.md` pointer, `CONTRIBUTING.md`, `BUILDING.md`, `TESTING.md`, PR/issue templates, regenerated ACKNOWLEDGEMENTS. Agents and contributors will do the Phase 1 fixes, so they need the rules first | S |
| Phase 0–1 (with golden fixtures) | `docs/FORMAT.md` (written alongside the fixtures, section by section) | M |
| Phase 1 (with S1/S2) | `SECURITY.md`, `docs/SECURITY_MODEL.md` | S |
| Phase 1 onward | `CHANGELOG.md` and `UPGRADING.md`, updated in every behaviour-changing PR (exit code N5, restore safety, KDF) | ongoing |
| Phase 2–3 | `docs/ARCHITECTURE.md`, `docs/PRUNE_SAFETY.md`, `docs/storage-backends.md`, generated command reference, README rewrite | M |
| Phase 3+ | Doc comments on exported API, ADRs for major decisions (Go vs Rust, KDF upgrade, separate metadata) | S, ongoing |

**Done when:** the README no longer depends on the upstream wiki, every command in AGENTS.md runs in CI, FORMAT.md sections each have a passing fixture, and generated docs and acknowledgements are checked in CI.
