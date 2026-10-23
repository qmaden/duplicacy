# Architecture decision records

Short records of decisions that are expensive to reverse, with the reasons, so that they aren't re-litigated
without new information. Add one for any change to the storage format, a dependency that touches stored bytes,
or a change in direction.

| ADR | Decision | Status |
|-----|----------|--------|
| [0001](0001-keep-go-rust-verifier-first.md) | Keep the Go implementation; try Rust only as an independent verifier | Accepted |
| [0002](0002-kdf-iterations.md) | Raise PBKDF2 iterations for new storages instead of changing the KDF | Accepted |
| [0003](0003-separate-metadata-compatibility.md) | Keep the separate metadata directory, keyed by directory | Accepted |
| [0004](0004-lz4-library.md) | Encode and decode LZ4 with pierrec/lz4, keeping the go-lz4 framing | Accepted |
| [0005](0005-wrong-highwayhash-in-pure-go.md) | Reimplement the incorrect arm64 HighwayHash in pure Go | Accepted |

Template: Context (what forces the decision), Decision, Consequences (including what would make us revisit it).
