## What and why

<!-- What does this change, and why? Reference REVIEW_PLAN.md finding IDs or issues (e.g. "Fixes S1"). -->

## How it was tested

<!-- Commands run, new tests, manual checks. -->

## Checklist

- [ ] `go build ./...`, `go vet ./...`, `gofmt -l src duplicacy internal` (empty) and staticcheck pass
- [ ] `go test -race -count=1 ./...` passes
- [ ] `TestGoldenFixtures` passes **without** regenerating fixtures (or the format change is intended and reviewed)
- [ ] Storages written by earlier versions can still be read
- [ ] Flags, exit codes and log IDs are unchanged, or the change is documented
- [ ] Regression test added for bug fixes
- [ ] `CHANGELOG.md` updated under "Unreleased"
