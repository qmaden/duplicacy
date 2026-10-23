# Prune safety

`prune` is the only command that deletes chunks, and the only one that can lose data that a backup still needs.
This page explains how it stays safe while other computers keep backing up to the same storage, and which options
and situations remove that safety. The code is `PruneSnapshots` and `FossilCollection.IsDeletable` in
`src/duplicacy_snapshotmanager.go`.

## The problem

Backups never lock the storage. A backup that is running while prune works may reuse a chunk that prune has just
decided is unreferenced (the backup saw it on the storage a moment ago), and the backup's snapshot file, which
proves the chunk is needed, appears only when the backup finishes. Deleting unreferenced chunks right away could
therefore delete a chunk that a snapshot will reference a few minutes later.

## Two-step fossil collection

**Step 1, collect.** Prune deletes the selected snapshot files and renames the chunks that only they referenced
to `<chunk>.fsl` ("fossils"). Fossils are invisible to backups: a backup that needs one of those chunks uploads it
again. Prune writes a *fossil collection* listing the fossils, the temporary files it found, the latest revision
of every snapshot id, and the time. The collection is kept in the local cache,
`.duplicacy/cache/<storage>/fossils/<n>`, not on the storage.

**Step 2, delete (a later prune run).** For each saved collection, prune checks that every snapshot id that was
active has finished a new backup since the collection:

- a new revision must *end* after the collection time plus a margin: 15 minutes on strongly consistent storages
  (local disk, SFTP, SMB, B2, Azure, GCS, Wasabi), 12 hours on the others, where a newly written snapshot may not
  be listed right away;
- a snapshot id whose last revision is more than 7 days old is treated as inactive and doesn't block deletion;
- ids given with `-ignore <id>` don't block deletion.

Any backup that might have reused a fossil while step 1 ran has then finished, and its snapshot is visible. Fossils
referenced by snapshots created after the collection are renamed back (resurrected); the rest are deleted,
together with the temporary files. If a revision that the collection deleted shows up again ("ghost snapshot",
from an eventually consistent listing), the collection is dropped without deleting anything.

The latest revision of a snapshot id is never deleted by a retention policy (except with `-exclusive`), so the chunks the next backup is most
likely to reuse are never fossilized.

## Rules for safe operation

- **Run both steps from the same repository and computer.** Fossil collections live in that computer's cache. If
  the cache is lost, the fossils stay on the storage; `prune -exhaustive` collects them again.
- **Run one prune per storage at a time.** Two prunes can each delete chunks the other still needs. Prune takes an
  advisory `prune.lock` in the storage root and refuses to start while another prune's lock is less than 24
  hours old. The lock is not atomic (storages have no create-if-absent), so two prunes starting in the same
  second may both run, and versions before this fork ignore it. Use `-ignore-lock` only after checking that no
  other prune is running.
- **Keep clocks in sync.** The deletion test compares the end time of a snapshot, from the clock of the computer
  that made the backup, with the collection time, from the prune computer's clock. A backup computer whose clock
  runs ahead can make fossils deletable too early. Prune warns about snapshots that end more than 15 minutes in the
  future.
- **Inactive snapshot ids**: a computer that backs up less often than every 7 days, or is offline during a
  collection and comes back later, is treated as inactive. If it resumes a backup started before the collection, it
  could reference fossils. Use `-ignore` only for ids that are really retired.
- **Use `-dry-run` first** after changing a retention policy; it lists the snapshots and chunks that would be removed.

## Options that bypass the protection

| Option | Effect | When it's safe |
|--------|--------|----------------|
| `-exclusive` | Skips the fossil stage: unreferenced chunks are deleted immediately, and saved collections are deleted without the checks above. | Only when no backup, copy or other prune can touch the storage for the whole run. A backup running at the same time can end up referencing deleted chunks, and its snapshot can't be restored. |
| `-ignore-lock` | Runs even if `prune.lock` shows another prune. | Only after confirming the other prune has stopped. |
| `-ignore <id>` | The id doesn't block fossil deletion. | For ids that will never back up again. |
| `-exhaustive` | Collects every unreferenced chunk and temporary file, not only those of the deleted snapshots. Still two-step unless combined with `-exclusive`. | Safe; slow because it lists every chunk. |
| `-delete-only`, `-collect-only` | Run only step 2 or only step 1. | Safe. |

## Recovering from mistakes

- A missing chunk reported by `check` can often be found as a fossil: `check -fossils` searches fossils too, and
  `check -resurrect` renames referenced fossils back.
- A snapshot that references chunks deleted by an `-exclusive` prune can't be repaired; if the files still exist
  on the source computer, a new backup uploads the chunks again. `check -persist` lists the affected files.
- Chunks can also be lost to storage-side problems unrelated to prune; see `check -chunks` in
  [COMMANDS.md](COMMANDS.md).
