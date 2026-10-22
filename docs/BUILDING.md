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
