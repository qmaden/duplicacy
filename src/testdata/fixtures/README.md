# Golden fixtures

Small storages written by this code base, restored by `TestGoldenFixtures` (`src/duplicacy_fixtures_test.go`)
on every test run. They guard the on-disk formats: if a change makes any of them unreadable, or changes what
they restore to, the test fails.

| Directory | What it covers |
|-----------|----------------|
| `plain-lz4` | Unencrypted storage, LZ4 (default compression level 100) |
| `encrypted-lz4` | Password-encrypted storage (config banner `duplicacy\001`, random salt) |
| `encrypted-zstd` | zstd compression (level 201) |
| `encrypted-zlib-legacy` | zlib level 6, which also selects the legacy HMAC-SHA256 hashing |
| `encrypted-rsa` | RSA encryption of file chunks (`duplicacy\002`); metadata chunks stay AES-only |
| `encrypted-erasure-coding` | Reed-Solomon erasure coding, 5 data + 2 parity shards (`duplicacy\003`) |
| `encrypted-separate-metadata` | `nesting` file with `separate-metadata`; metadata chunks under `metadata/` |
| `encrypted-fixed-chunk-size` | Fixed-size chunking (minimum = maximum) |
| `legacy-config` | Old config format: banner `duplicacy\000`, static salt, 16384 PBKDF2 iterations |

Every storage holds snapshot `fixture` revisions 1 and 2. `manifest-r1.txt` and `manifest-r2.txt` list the
expected restored content (path, size, SHA-256). The password for the encrypted storages is
`fixture-password`.

`rsa-test-key.pem` is a **test-only** private key that protects nothing but `encrypted-rsa`.

Not covered yet: snapshots in the version 0 (JSON) file-list format and chunks written by CLI 3.0.1 on arm64
(see `internal/wronghighwayhash` for the latter), since this code base can no longer write them.

## Regenerating

Only regenerate when a format change is intended and has been reviewed. Old fixtures should normally be kept and
new ones added next to them.

```bash
cd src && DUPLICACY_GENERATE_FIXTURES=1 go test -run TestGenerateFixtures .
```
