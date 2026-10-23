# Duplicacy (qmaden fork)

Duplicacy is a command-line backup tool built on
[lock-free deduplication](https://github.com/gilbertchen/duplicacy/wiki/Lock-Free-Deduplication): any number of
computers can back up to the same cloud or local storage at the same time, identical data is stored once across
all of them, and old backups can be pruned while others keep running, all without a lock server or a chunk
database. The design is described in the paper published in
[IEEE Transactions on Cloud Computing](https://ieeexplore.ieee.org/document/9310668)
([draft PDF](duplicacy_paper.pdf)).

This repository is a maintained fork of [gilbertchen/duplicacy](https://github.com/gilbertchen/duplicacy) (CLI
3.2.x). It fixes security and data-safety bugs, modernizes the toolchain and dependencies, and makes backup and
restore faster, **while keeping the storage format unchanged**: storages written by upstream Duplicacy (CLI and
GUI) are read by this fork, and storages written by this fork are read by them. Checked-in golden storages of every
format are restored byte for byte in CI.

## What's different from upstream

- **Restore safety**: snapshot paths are validated and symbolic links in the restore tree are never followed.
- **SFTP**: host keys are verified against `known_hosts` by host name; ssh-agent keys are tried first.
- **Encryption**: 600,000 PBKDF2 iterations for new storages, `password -iterations` to upgrade existing ones,
  malformed chunks rejected without crashing.
- **Correctness**: fixes to `check`, resume, prune retention and clock-skew handling, restore with `-delete`, the
  separate metadata directory, and many races. `backup -strict` exits non-zero when files were skipped.
- **Performance**: faster LZ4 and pooled zstd codecs, read-ahead and chunk hashing spread over several cores,
  parallel metadata downloads, prefetching across files and parallel attribute setting on restore, automatic
  retries of failed chunk transfers, and a `-memory-limit` option.
- **Dependencies**: current SDKs for S3 (aws-sdk-go-v2), Azure (azblob) and Google; cloud backends tested in CI
  against fakes and emulators.
- **Tooling**: Go 1.26.10 (Go 1.27 also builds), CI with race tests, fuzzing, staticcheck and govulncheck.

The full list is in [CHANGELOG.md](CHANGELOG.md); behaviour changes you may need to act on are in
[docs/UPGRADING.md](docs/UPGRADING.md).

**Platforms**: Linux x86-64 and macOS (Apple Silicon and Intel). The Windows code is still in the tree but is not
built or tested.

## Build

Go 1.21 or later is enough to start; the pinned toolchain is downloaded automatically.

```bash
go build -o duplicacy ./duplicacy
```

See [docs/BUILDING.md](docs/BUILDING.md) for release builds and cross-compiling.

## Quick start

```bash
cd ~/Documents
duplicacy init -e documents sftp://user@backup.example.com/backups   # -e: encrypt the storage
duplicacy backup -stats
duplicacy list
duplicacy restore -r 1 -overwrite                                     # restore revision 1 in place
duplicacy prune -keep 0:360 -keep 30:90 -keep 7:30 -keep 1:7
```

`init` turns the current directory into a *repository* with the snapshot id `documents`, backed up to the given
storage. Credentials are prompted for and saved in the keyring, or can be passed through environment variables
(see [docs/storage-backends.md](docs/storage-backends.md)). Read [docs/PRUNE_SAFETY.md](docs/PRUNE_SAFETY.md)
before automating prune.

## Storage backends

Local disk, SFTP, SMB, Amazon S3 and S3-compatible services (MinIO, Wasabi, DigitalOcean Spaces and others),
Backblaze B2, Microsoft Azure, Google Cloud Storage, Google Drive, Microsoft OneDrive, Dropbox, OpenStack Swift,
WebDAV (including pCloud and Box), Storj and File Fabric. URL formats and credentials:
[docs/storage-backends.md](docs/storage-backends.md).

## Documentation

| Document | Contents |
|----------|----------|
| [docs/COMMANDS.md](docs/COMMANDS.md) | Every command and option (generated from the code) |
| [docs/storage-backends.md](docs/storage-backends.md) | Storage URLs, credentials, environment variables |
| [docs/PRUNE_SAFETY.md](docs/PRUNE_SAFETY.md) | How prune stays safe with concurrent backups, and how to keep it safe |
| [docs/UPGRADING.md](docs/UPGRADING.md) | Behaviour changes that may need action |
| [docs/SECURITY_MODEL.md](docs/SECURITY_MODEL.md) | What is protected against whom; known limits |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Code layout and data flow |
| [docs/FORMAT.md](docs/FORMAT.md) | The storage format |
| [docs/BUILDING.md](docs/BUILDING.md), [docs/TESTING.md](docs/TESTING.md) | Building and testing |
| [docs/adr/](docs/adr/README.md) | Design decisions and their reasons |
| [CONTRIBUTING.md](CONTRIBUTING.md), [AGENTS.md](AGENTS.md) | Rules for contributors and coding agents |
| [SECURITY.md](SECURITY.md) | Reporting vulnerabilities |

The [upstream wiki](https://github.com/gilbertchen/duplicacy/wiki) has more background and how-to guides; where it
disagrees with the documents above, they describe this fork.

## Comparison with other tools

Duplicacy differs from tools such as restic, Borg, Kopia and Duplicati mainly in concurrency: chunks are stored as
individual files named by their hash, with no index or lock, so several computers can back up to one storage
concurrently and deduplicate against each other, and pruning doesn't block backups. The upstream README has a
longer [comparison](https://github.com/gilbertchen/duplicacy#comparison-with-other-backup-tools) and
[benchmarks](https://github.com/gilbertchen/benchmarking).

## License

* Free for personal use or commercial trial
* Non-trial commercial use requires per-computer CLI licenses available from [duplicacy.com](https://duplicacy.com/buy.html) at a cost of $50 per year
* The computer with a valid commercial license for the GUI version may run the CLI version without a CLI license
* CLI licenses are not required to restore or manage backups; only the backup command requires valid CLI licenses
* Modification and redistribution are permitted, but commercial use of derivative works is subject to the same requirements of this license
