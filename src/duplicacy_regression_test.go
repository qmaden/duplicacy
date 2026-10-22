// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// openFixture copies a golden fixture to a temporary directory and returns a backup manager for it.
func openFixture(t *testing.T, name string) (manager *BackupManager, top string) {
	testDir := t.TempDir()
	storageDir := filepath.Join(testDir, "storage")
	copyDirectory(t, filepath.Join(fixtureDir, name, "storage"), storageDir)

	top = filepath.Join(testDir, "repository")
	if err := os.MkdirAll(filepath.Join(top, DUPLICACY_DIRECTORY), 0700); err != nil {
		t.Fatal(err)
	}
	SetDuplicacyPreferencePath(filepath.Join(top, DUPLICACY_DIRECTORY))

	storage, err := CreateFileStorage(storageDir, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	password := ""
	if strings.HasPrefix(name, "encrypted") || name == "legacy-config" {
		password = fixturePassword
	}
	manager = CreateBackupManager(fixtureSnapshotID, storage, top, password, "", "", false)
	if manager == nil {
		t.Fatal("Failed to create the backup manager")
	}
	manager.SetupSnapshotCache("default")
	return manager, top
}

// B1: CheckSnapshot used to discard every error it found.  Version 1 snapshots are also validated while their
// file list is decoded, but version 0 (JSON) snapshots are not, so 'check -files' passed broken ones and then
// crashed while reading their files.
func TestCheckSnapshotReportsErrors(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(WARN)
	defer catchFixtureException(t)

	manager, _ := openFixture(t, "plain-lz4")
	snapshotManager := manager.SnapshotManager
	snapshotManager.CreateChunkOperator(false, false, 1, false)
	defer func() {
		snapshotManager.chunkOperator.Stop()
		snapshotManager.chunkOperator = nil
	}()

	// A valid version 1 snapshot passes; the checks that now take effect must not reject real snapshots
	for revision := 1; revision <= fixtureRevisions; revision++ {
		snapshot := snapshotManager.DownloadSnapshot(fixtureSnapshotID, revision)
		snapshotManager.DownloadSnapshotSequences(snapshot)
		if err := snapshotManager.CheckSnapshot(snapshot); err != nil {
			t.Fatalf("Revision %d failed the check: %v", revision, err)
		}
	}

	// Version 0 snapshots store the file list as JSON.  The file list chunk is placed in the snapshot cache, from
	// which metadata chunks are read in plain form after their ids are verified.
	makeVersion0Snapshot := func(entries []*Entry) *Snapshot {
		description, err := json.Marshal(entries)
		if err != nil {
			t.Fatal(err)
		}
		chunk := CreateChunk(manager.config, true)
		chunk.Reset(true)
		chunk.Write(description)
		cachePath, _, _, err := snapshotManager.snapshotCache.FindChunk(0, chunk.GetID(), false, true)
		if err != nil {
			t.Fatal(err)
		}
		if err = snapshotManager.snapshotCache.UploadFile(0, cachePath, description); err != nil {
			t.Fatal(err)
		}
		return &Snapshot{
			ID:           fixtureSnapshotID,
			Revision:     100,
			Version:      0,
			FileSequence: []string{chunk.GetHash()},
			ChunkHashes:  []string{"hash0", "hash1"},
			ChunkLengths: []int{100, 100},
		}
	}

	valid := makeVersion0Snapshot([]*Entry{
		{Path: "a", Size: 150, Mode: 0644, StartChunk: 0, StartOffset: 0, EndChunk: 1, EndOffset: 50},
	})
	if err := snapshotManager.CheckSnapshot(valid); err != nil {
		t.Fatalf("A valid version 0 snapshot failed the check: %v", err)
	}

	broken := makeVersion0Snapshot([]*Entry{
		{Path: "a", Size: 50, Mode: 0644, StartChunk: 1, StartOffset: 0, EndChunk: 0, EndOffset: 50},
	})
	err := snapshotManager.CheckSnapshot(broken)
	if err == nil {
		t.Fatal("A version 0 snapshot with a file ending before it starts passed the check")
	}
	t.Logf("Expected error: %v", err)
}

func TestEntryCheckRejectsInvalidChunkRanges(t *testing.T) {
	chunkLengths := []int{100, 100, 100}
	valid := Entry{Path: "f", Size: 150, StartChunk: 0, StartOffset: 50, EndChunk: 1, EndOffset: 100}
	if err := valid.check(chunkLengths); err != nil {
		t.Fatalf("A valid entry was rejected: %v", err)
	}

	invalid := map[string]Entry{
		"end before start":    {Path: "f", Size: 50, StartChunk: 2, StartOffset: 0, EndChunk: 1, EndOffset: 50},
		"negative start":      {Path: "f", Size: 50, StartChunk: -1, StartOffset: 0, EndChunk: 0, EndOffset: 50},
		"end past last chunk": {Path: "f", Size: 50, StartChunk: 0, StartOffset: 0, EndChunk: 3, EndOffset: 50},
		"offset past chunk":   {Path: "f", Size: 50, StartChunk: 0, StartOffset: 100, EndChunk: 1, EndOffset: 50},
		"wrong size":          {Path: "f", Size: 51, StartChunk: 0, StartOffset: 0, EndChunk: 0, EndOffset: 50},
		"negative size":       {Path: "f", Size: -1},
	}
	for name, entry := range invalid {
		if err := entry.check(chunkLengths); err == nil {
			t.Errorf("Entry with %s was accepted", name)
		}
	}
}

// N1 (upstream #675): with -persist, a chunk that failed to download was handed to the caller and also put back
// into the chunk pool, so two goroutines could end up using the same chunk.
func TestFailedDownloadIsNotReturnedToPool(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	config := CreateConfigFromParameters(DEFAULT_COMPRESSION_LEVEL, 4096, 16384, 1024, false, nil, false)
	storage, err := CreateFileStorage(t.TempDir(), false, 1)
	if err != nil {
		t.Fatal(err)
	}
	storage.SetDefaultNestingLevels([]int{1}, 1)
	for _, dir := range []string{"chunks", "snapshots"} {
		storage.CreateDirectory(0, dir)
	}

	operator := CreateChunkOperator(config, storage, nil, false, false, 1, true)
	defer operator.Stop()

	chunk := operator.Download("hash of a chunk that does not exist", 0, false)
	if chunk == nil || !chunk.isBroken {
		t.Fatalf("Expected a broken chunk, got %v", chunk)
	}
	operator.WaitForCompletion()

	for {
		select {
		case pooled := <-config.chunkPool:
			if pooled == chunk {
				t.Fatal("The broken chunk handed to the caller was also returned to the chunk pool")
			}
			continue
		default:
		}
		break
	}
}
