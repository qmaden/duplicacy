# Building

## Requirements

- Go **1.26.10** (pinned in `go.mod` via the `toolchain` line). With `GOTOOLCHAIN=auto` (the default), any
  installed Go ≥ 1.21 downloads it automatically. Go 1.27.x also builds and passes the tests (checked in CI).
- Supported platforms: **Linux x86-64** and **macOS** (Apple Silicon and Intel). The Windows code is still in the
  tree but is not built, tested or supported by this fork.

## Build the CLI

```bash
go build -o duplicacy_main ./duplicacy
./duplicacy_main -h | grep -A1 VERSION
```

Release-style builds for the supported targets:

```bash
GOOS=linux  GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o duplicacy_linux_x64   ./duplicacy
GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o duplicacy_osx_arm64   ./duplicacy
GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o duplicacy_osx_x64     ./duplicacy
```

All storage backends are compiled in; no cgo is required.

## Things not to do

- **Don't build with `-tags noasm`.** It used to be a workaround for Go 1.27 link errors. It disables the
  assembly in `klauspost/reedsolomon` and `klauspost/compress` (slower), and the original reason for it is gone:
  the incorrect arm64 HighwayHash now lives in `internal/wronghighwayhash` as pure Go (see `REVIEW_PLAN.md`
  section 5.1).
- **Don't swap compression, hashing or erasure-coding libraries** without running `TestGoldenFixtures` and reading
  `docs/FORMAT.md`. Their output is part of the storage format.

## Releasing

Pushing a version tag publishes a release. `.github/workflows/release.yml` runs the tests, builds the binaries and
the Debian package, and attaches them to a GitHub release with a `SHA256SUMS` file:

```bash
git checkout master && git pull
git tag v3.3.0          # or v3.3.0-rc1 for a pre-release
git push origin v3.3.0
```

| File | Contents |
|------|----------|
| `duplicacy_linux_x64_<version>` | Static Linux x86-64 binary |
| `duplicacy_<version>_amd64.deb` | The same binary installed as `/usr/bin/duplicacy`, with the license, README and changelog |
| `duplicacy_osx_arm64_<version>`, `duplicacy_osx_x64_<version>` | macOS binaries for Apple Silicon and Intel |

- The version shown by `duplicacy -h` comes from the tag (`-X main.Version`), so the source doesn't need editing.
- macOS binaries are built on a macOS runner because Keychain support needs cgo. They are not signed or
  notarized: after downloading one with a browser, run `xattr -d com.apple.quarantine <file>` (or right-click,
  Open) the first time.
- The package's `Maintainer` field is the author of the tagged commit, unless the repository variable
  `DEB_MAINTAINER` (Settings, Secrets and variables, Actions, Variables) is set, for example to
  `Name <email>`.
- To check a release locally: `.github/scripts/build-release.sh binary linux amd64 3.3.0` and
  `.github/scripts/build-release.sh deb 3.3.0` (needs `dpkg-deb`, for example in a Debian container); macOS
  binaries can only be built on macOS.

## Upgrading Go

1. Change the `toolchain` line in `go.mod`; keep the `go` directive at the lowest version the code needs.
2. Run the full test suite, `go vet ./...` and `govulncheck ./...`.
3. The `go-next` CI job already builds and tests with the next Go release, so problems usually show up there
   first.

## Upgrading dependencies

- Run `go get <module>@<version> && go mod tidy`, then the full test suite (the golden fixtures catch format
  changes).
- Regenerate `ACKNOWLEDGEMENTS.md` (commands at the end of that file).
- `govulncheck ./...` must report no reachable vulnerabilities.
