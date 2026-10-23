# 0003: Keep the separate metadata directory, keyed by directory

- Status: Accepted (2026-10-22)

## Context

Shortly before the fork, upstream added an optional `separate-metadata` flag in the storage's `nesting` file:
metadata chunks (file lists, chunk lists) go to `metadata/` instead of `chunks/`, so that `chunks/` can use a
cheaper, slower storage class while `list`, `check` and prune only touch `metadata/`.

The review found that backup deduplicated chunks by id only (B2): a file chunk with the same id as an existing
metadata chunk was considered uploaded but was never written to `chunks/`, and a later restore failed. `copy`
already keyed chunks by directory.

## Decision

- Keep the feature; it's opt-in and useful.
- Key every chunk-existence cache by directory plus chunk id (backup, copy, `UploadSnapshot`), so a chunk in one
  directory never stands in for the other.
- Readers look for metadata chunks in `metadata/` first and fall back to `chunks/`, so turning the option on for
  an existing storage works without moving anything. Fossils of metadata chunks stay under `metadata/` (or `metadata-fossils/`).
- Document the layout in FORMAT.md and test it with a golden fixture.

## Consequences

- Clients from before upstream added the feature can't find metadata chunks in `metadata/`, so a storage with the
  option on needs a current client everywhere. This is the one place where "earlier versions can read storages
  written by this one" holds only while the option is off.
- Revisit if the flag is ever to become the default, which would need a format-version plan.
