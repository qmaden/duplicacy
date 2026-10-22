// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
	// The entry is rejected while the file list is decoded (SNAPSHOT_ENTRY) or by CheckSnapshot itself
	var err error
	message := expectLogError(func() { err = snapshotManager.CheckSnapshot(broken) })
	if err == nil && message == "" {
		t.Fatal("A version 0 snapshot with a file ending before it starts passed the check")
	}
	t.Logf("Expected error: %v %s", err, message)
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

// B2: with metadata chunks stored separately, a file chunk with the same id as a metadata chunk of the previous
// snapshot was assumed to exist and never uploaded to the chunk directory, so the file couldn't be restored.
func TestSeparateMetadataDoesNotHideFileChunks(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	testDir := t.TempDir()
	storageDir := filepath.Join(testDir, "storage")
	storage, err := CreateFileStorage(storageDir, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !ConfigStorage(storage, CONFIG_DEFAULT_ITERATIONS, DEFAULT_COMPRESSION_LEVEL, 4096, 16384, 1024, "", nil, false, "", 0, 0) {
		t.Fatal("Failed to initialize the storage")
	}
	if err = storage.UploadFile(0, "nesting", []byte(`{"read-levels":[1],"write-level":1,"separate-metadata":true}`)); err != nil {
		t.Fatal(err)
	}

	repository := filepath.Join(testDir, "repository")
	os.MkdirAll(filepath.Join(repository, DUPLICACY_DIRECTORY), 0700)
	os.WriteFile(filepath.Join(repository, "a.bin"), fixtureBytes(30000, 7), 0600)
	os.WriteFile(filepath.Join(repository, "b.txt"), []byte("small file\n"), 0600)

	backup := func(quickMode bool) *BackupManager {
		SetDuplicacyPreferencePath(filepath.Join(repository, DUPLICACY_DIRECTORY))
		storage, err := CreateFileStorage(storageDir, false, 1)
		if err != nil {
			t.Fatal(err)
		}
		manager := CreateBackupManager("test", storage, repository, "", "", "", false)
		manager.SetupSnapshotCache("default")
		if !manager.Backup(repository, quickMode, 1, "", false, false, 0, false, 1024, 1024) {
			t.Fatal("Backup failed")
		}
		return manager
	}
	manager := backup(false)

	// Find a metadata chunk of revision 1 that is smaller than the minimum chunk size: a new file with exactly
	// this content becomes a single file chunk with the same id
	snapshot := manager.SnapshotManager.DownloadSnapshot("test", 1)
	var content []byte
	sequences := append(append(append([]string{}, snapshot.FileSequence...), snapshot.ChunkSequence...), snapshot.LengthSequence...)
	for _, hash := range sequences {
		if data := manager.SnapshotManager.DownloadSequence([]string{hash}); len(data) > 0 && len(data) < 1024 {
			content = data
			break
		}
	}
	if content == nil {
		t.Fatal("No small metadata chunk found")
	}
	os.WriteFile(filepath.Join(repository, "same-as-metadata.bin"), content, 0600)
	backup(true)

	top := filepath.Join(testDir, "restore")
	os.MkdirAll(filepath.Join(top, DUPLICACY_DIRECTORY), 0700)
	SetDuplicacyPreferencePath(filepath.Join(top, DUPLICACY_DIRECTORY))
	storage, err = CreateFileStorage(storageDir, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	restoreManager := CreateBackupManager("test", storage, top, "", "", "", false)
	restoreManager.SetupSnapshotCache("default")
	if failed := restoreManager.Restore(top, 2, true, false, 1, true, false, false, false, nil, false); failed != 0 {
		t.Fatalf("%d files failed to restore", failed)
	}
	restored, err := os.ReadFile(filepath.Join(top, "same-as-metadata.bin"))
	if err != nil || !bytes.Equal(restored, content) {
		t.Errorf("The file with the same content as a metadata chunk was not restored correctly (error %v)", err)
	}
}

// N5 (upstream #540): the number of entries skipped because of access errors is reported, so that the CLI can
// fail 'backup -strict' instead of exiting with 0.
func TestBackupCountsSkippedEntries(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read files without read permission")
	}
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	repository := t.TempDir()
	os.WriteFile(filepath.Join(repository, "readable"), []byte("ok"), 0600)
	unreadable := filepath.Join(repository, "unreadable")
	os.WriteFile(unreadable, []byte("secret"), 0600)
	os.Chmod(unreadable, 0)
	defer os.Chmod(unreadable, 0600)

	storageDir := filepath.Join(t.TempDir(), "storage")
	storage, err := CreateFileStorage(storageDir, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !ConfigStorage(storage, CONFIG_DEFAULT_ITERATIONS, DEFAULT_COMPRESSION_LEVEL, 4096, 16384, 1024, "", nil, false, "", 0, 0) {
		t.Fatal("Failed to initialize the storage")
	}
	os.MkdirAll(filepath.Join(repository, DUPLICACY_DIRECTORY), 0700)
	SetDuplicacyPreferencePath(filepath.Join(repository, DUPLICACY_DIRECTORY))
	manager := CreateBackupManager("test", storage, repository, "", "", "", false)
	manager.SetupSnapshotCache("default")
	if !manager.Backup(repository, false, 1, "", false, false, 0, false, 1024, 1024) {
		t.Fatal("Backup failed")
	}
	if manager.SkippedEntries != 1 {
		t.Errorf("%d skipped entries reported instead of 1", manager.SkippedEntries)
	}
}

// S9: the snapshot cache holds decrypted metadata, so it must only be readable by its owner.  Local storages keep
// their permissions, since they are sometimes shared on purpose.
func TestSnapshotCacheIsPrivate(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	manager, top := openFixture(t, "plain-lz4")
	cacheDir := filepath.Join(top, DUPLICACY_DIRECTORY, "cache", "default")
	if info, err := os.Stat(cacheDir); err != nil || info.Mode().Perm() != 0700 {
		t.Errorf("The snapshot cache directory has mode %v (error %v)", info.Mode().Perm(), err)
	}
	if err := manager.snapshotCache.UploadFile(0, "chunks/ab/test", []byte("decrypted metadata")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"chunks/ab", "chunks/ab/test"} {
		info, err := os.Stat(filepath.Join(cacheDir, name))
		expected := os.FileMode(0600)
		if info != nil && info.IsDir() {
			expected = 0700
		}
		if err != nil || info.Mode().Perm() != expected {
			t.Errorf("%s in the snapshot cache has mode %v instead of %v (error %v)", name, info.Mode().Perm(), expected, err)
		}
	}

	storageDir := t.TempDir()
	storage, _ := CreateFileStorage(storageDir, false, 1)
	if err := storage.UploadFile(0, "chunks/ab/test", []byte("chunk")); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(filepath.Join(storageDir, "chunks", "ab", "test")); info.Mode().Perm() != 0644 {
		t.Errorf("A local storage file has mode %v instead of 0644", info.Mode().Perm())
	}
}

// B4: resuming an interrupted backup reused the chunks it had uploaded without checking that they still exist.
// They belong to no snapshot, so prune may have removed them in the meantime.
func TestResumeChecksIncompleteChunks(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	testDir := t.TempDir()
	repository := filepath.Join(testDir, "repository")
	os.MkdirAll(filepath.Join(repository, DUPLICACY_DIRECTORY), 0700)
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(repository, "file"+string(rune('a'+i))), fixtureBytes(20000, uint32(10+i)), 0600)
	}
	storageDir := filepath.Join(testDir, "storage")
	storage, err := CreateFileStorage(storageDir, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !ConfigStorage(storage, CONFIG_LEGACY_ITERATIONS, DEFAULT_COMPRESSION_LEVEL, 4096, 16384, 1024, "", nil, false, "", 0, 0) {
		t.Fatal("Failed to initialize the storage")
	}

	newManager := func(top string) *BackupManager {
		SetDuplicacyPreferencePath(filepath.Join(top, DUPLICACY_DIRECTORY))
		storage, err := CreateFileStorage(storageDir, false, 1)
		if err != nil {
			t.Fatal(err)
		}
		manager := CreateBackupManager("test", storage, top, "", "", "", false)
		manager.SetupSnapshotCache("default")
		return manager
	}

	// Interrupt the first backup; the process would save the incomplete snapshot on its way out
	os.Setenv("DUPLICACY_FAIL_CHUNK", "10")
	func() {
		// The interruption is expected; don't let its LOG_ERROR fail the test
		testingT = nil
		defer func() {
			setTestingT(t)
			if r := recover(); r != nil {
				RunAtError()
			}
		}()
		newManager(repository).Backup(repository, false, 1, "", false, false, 0, false, 1024, 1024)
		t.Fatal("The backup was not interrupted")
	}()
	os.Unsetenv("DUPLICACY_FAIL_CHUNK")
	RunAtError = func() {}
	if _, err := os.Stat(filepath.Join(repository, DUPLICACY_DIRECTORY, "cache", "default", "incomplete_snapshot")); err != nil {
		t.Fatalf("No incomplete snapshot was saved: %v", err)
	}

	// Remove every chunk, as a prune would remove unreferenced chunks
	os.RemoveAll(filepath.Join(storageDir, "chunks"))
	os.MkdirAll(filepath.Join(storageDir, "chunks"), 0700)

	if !newManager(repository).Backup(repository, false, 1, "", false, false, 0, false, 1024, 1024) {
		t.Fatal("The resumed backup failed")
	}

	top := filepath.Join(testDir, "restore")
	os.MkdirAll(filepath.Join(top, DUPLICACY_DIRECTORY), 0700)
	checker := newManager(top)
	if !checker.SnapshotManager.CheckSnapshots("test", []int{1}, "", false, false, false, false, false, false, false, 1, false) {
		t.Fatal("The resumed backup references missing chunks")
	}
	if failed := checker.Restore(top, 1, true, false, 1, true, false, false, false, nil, false); failed != 0 {
		t.Fatalf("%d files failed to restore", failed)
	}
	for i := 0; i < 5; i++ {
		restored, _ := os.ReadFile(filepath.Join(top, "file"+string(rune('a'+i))))
		if !bytes.Equal(restored, fixtureBytes(20000, uint32(10+i))) {
			t.Errorf("file%c was not restored correctly", 'a'+i)
		}
	}
}

// B5: 'check -chunks' never verified a chunk again once it had passed, so later corruption went unnoticed.
func TestCheckReverifiesOldChunks(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	manager, top := openFixture(t, "plain-lz4")
	snapshotManager := manager.SnapshotManager
	check := func(reverifyDays int) string {
		snapshotManager.ReverifyAfterDays = reverifyDays
		return expectLogError(func() {
			snapshotManager.CheckSnapshots(fixtureSnapshotID, []int{1, 2}, "", false, false, false, true, false, false,
				false, 1, true)
		})
	}
	if message := check(0); message != "" {
		t.Fatalf("The first check failed: %s", message)
	}

	// Corrupt one chunk in the storage
	storageDir := filepath.Join(filepath.Dir(top), "storage")
	var corrupted string
	filepath.Walk(filepath.Join(storageDir, "chunks"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && corrupted == "" && info.Size() > 100 {
			corrupted = path
		}
		return nil
	})
	content, _ := os.ReadFile(corrupted)
	for i := 50; i < 60; i++ {
		content[i] ^= 0xff
	}
	os.WriteFile(corrupted, content, 0600)

	if message := check(0); message != "" {
		t.Fatalf("Chunks verified before should have been skipped: %s", message)
	}

	// Pretend the chunks were verified 10 days ago
	cacheFile := filepath.Join(top, DUPLICACY_DIRECTORY, "cache", "default", "verified_chunks")
	var verified map[string]int64
	description, _ := os.ReadFile(cacheFile)
	if err := json.Unmarshal(description, &verified); err != nil || len(verified) == 0 {
		t.Fatalf("No verified chunks were recorded: %v", err)
	}
	for chunkID, verifiedAt := range verified {
		verified[chunkID] = verifiedAt - 10*secondsInDay
	}
	description, _ = json.Marshal(verified)
	os.WriteFile(cacheFile, description, 0600)

	if message := check(30); message != "" {
		t.Errorf("Chunks verified less than 30 days ago were verified again: %s", message)
	}
	if message := check(5); !strings.Contains(message, "corrupted") {
		t.Errorf("Re-verification didn't detect the corrupted chunk: %q", message)
	}
}

// B15: the preference directory used to count as inside the restore directory by a plain string prefix test.
func TestIsPathInside(t *testing.T) {
	cases := []struct {
		child, parent string
		inside        bool
	}{
		{"/data/a/.duplicacy", "/data/a", true},
		{"/data/a", "/data/a", true},
		{"/data/ab/.duplicacy", "/data/a", false},
		{"/data", "/data/a", false},
		{"/data/a/../b", "/data/a", false},
	}
	for _, c := range cases {
		if isPathInside(c.child, c.parent) != c.inside {
			t.Errorf("isPathInside(%q, %q) should be %v", c.child, c.parent, c.inside)
		}
	}
}

// B13: a read error used to produce the hash of whatever had been read so far.
func TestComputeFileHashReadError(t *testing.T) {
	SetLoggingLevel(ERROR)
	config := CreateConfig()
	if hash := config.ComputeFileHash(t.TempDir(), make([]byte, 4096)); hash != "" {
		t.Errorf("Hashing a directory returned %q instead of an empty hash", hash)
	}
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, []byte("content"), 0600)
	if hash := config.ComputeFileHash(file, make([]byte, 4096)); hash == "" {
		t.Error("Hashing a readable file failed")
	}
}

// B10: Stop() used a negative task count as the "stopped" marker, which a task finishing before it was counted
// could produce.  Stop must be safe to call twice, and WaitForCompletion must wait for queued tasks.
func TestChunkOperatorStopAndWait(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	config := CreateConfigFromParameters(DEFAULT_COMPRESSION_LEVEL, 4096, 16384, 1024, false, nil, false)
	storage, err := CreateFileStorage(t.TempDir(), false, 2)
	if err != nil {
		t.Fatal(err)
	}
	storage.SetDefaultNestingLevels([]int{1}, 1)
	storage.CreateDirectory(0, "chunks")

	operator := CreateChunkOperator(config, storage, nil, false, false, 2, false)
	var uploaded int64
	operator.UploadCompletionFunc = func(chunk *Chunk, chunkIndex int, inCache bool, chunkSize int, uploadSize int) {
		atomic.AddInt64(&uploaded, 1)
		config.PutChunk(chunk)
	}
	for i := 0; i < 50; i++ {
		chunk := config.GetChunk()
		chunk.Reset(true)
		chunk.Write(fixtureBytes(2000, uint32(i)))
		operator.Upload(chunk, i, false)
	}
	operator.WaitForCompletion()
	if n := atomic.LoadInt64(&uploaded); n != 50 {
		t.Errorf("WaitForCompletion returned after %d of 50 uploads", n)
	}

	done := make(chan bool)
	go func() {
		operator.Stop()
		operator.Stop()
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Stopping the chunk operator twice hung")
	}
}

// B11: copy counted copied chunks from several upload goroutines without synchronization.  This also covers copy
// itself, which had no test.
func TestCopySnapshots(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	testDir := t.TempDir()
	repository := filepath.Join(testDir, "repository")
	os.MkdirAll(filepath.Join(repository, DUPLICACY_DIRECTORY), 0700)
	for i := 0; i < 4; i++ {
		os.WriteFile(filepath.Join(repository, "file"+string(rune('a'+i))), fixtureBytes(30000, uint32(20+i)), 0600)
	}
	sourceDir := backupForRestoreTest(t, repository)

	SetDuplicacyPreferencePath(filepath.Join(repository, DUPLICACY_DIRECTORY))
	sourceStorage, _ := CreateFileStorage(sourceDir, false, 4)
	source := CreateBackupManager("test", sourceStorage, repository, "", "", "", false)
	source.SetupSnapshotCache("default")

	destinationDir := filepath.Join(testDir, "destination")
	destinationStorage, _ := CreateFileStorage(destinationDir, false, 4)
	if !ConfigStorage(destinationStorage, CONFIG_LEGACY_ITERATIONS, DEFAULT_COMPRESSION_LEVEL, 4096, 16384, 1024, "",
		source.config, true, "", 0, 0) {
		t.Fatal("Failed to initialize the destination storage")
	}
	destinationStorage, _ = CreateFileStorage(destinationDir, false, 4)
	destination := CreateBackupManager("test", destinationStorage, repository, "", "", "", false)
	destination.SetupSnapshotCache("destination")

	if !source.CopySnapshots(destination, "test", nil, 4, 4) {
		t.Fatal("Copy failed")
	}

	top := filepath.Join(testDir, "restore")
	if failed := restoreForTest(t, destinationDir, top, true, false); failed != 0 {
		t.Fatalf("%d files failed to restore from the copy", failed)
	}
	for i := 0; i < 4; i++ {
		restored, _ := os.ReadFile(filepath.Join(top, "file"+string(rune('a'+i))))
		if !bytes.Equal(restored, fixtureBytes(30000, uint32(20+i))) {
			t.Errorf("file%c was not copied correctly", 'a'+i)
		}
	}
}

// failingUploadStorage is a file storage whose uploads fail.
type failingUploadStorage struct {
	*FileStorage
}

func (storage *failingUploadStorage) UploadFile(threadIndex int, filePath string, content []byte) error {
	return fmt.Errorf("simulated upload failure")
}

// B16: 'check -rewrite' moved a recoverable chunk aside as a fossil before uploading the repaired copy; if the
// upload failed, the chunk was left only as a fossil.
func TestRewriteChunkRollsBack(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	data := bytes.Repeat([]byte("rewrite me "), 2000)
	config, encoded := erasureCodedChunk(t, data)
	// Damage one data shard so that the chunk can only be read through recovery, which asks for a rewrite
	encoded[len(ERASURE_CODING_BANNER)+14+7*32+5] ^= 0xff

	hasher := CreateChunk(config, true)
	hasher.Reset(true)
	hasher.Write(data)
	chunkHash := hasher.GetHash()
	chunkID := config.GetChunkIDFromHash(chunkHash)

	for _, failing := range []bool{true, false} {
		fileStorage, err := CreateFileStorage(t.TempDir(), false, 1)
		if err != nil {
			t.Fatal(err)
		}
		fileStorage.SetDefaultNestingLevels([]int{1}, 1)
		fileStorage.CreateDirectory(0, "chunks")
		chunkPath, _, _, _ := fileStorage.FindChunk(0, chunkID, false, false)
		if err = fileStorage.UploadFile(0, chunkPath, encoded); err != nil {
			t.Fatal(err)
		}

		var storage Storage = fileStorage
		if failing {
			storage = &failingUploadStorage{fileStorage}
		}
		operator := CreateChunkOperator(config, storage, nil, false, true, 1, false)
		chunk := operator.Download(chunkHash, 0, false)
		operator.Stop()
		if !bytes.Equal(chunk.GetBytes(), data) {
			t.Fatalf("failing=%v: the chunk was not recovered", failing)
		}

		stored, err := os.ReadFile(filepath.Join(fileStorage.storageDir, chunkPath))
		if err != nil {
			t.Fatalf("failing=%v: the chunk no longer exists: %v", failing, err)
		}
		_, fossilErr := os.Stat(filepath.Join(fileStorage.storageDir, chunkPath+".fsl"))
		if failing {
			if !bytes.Equal(stored, encoded) || fossilErr == nil {
				t.Error("After a failed rewrite the original chunk was not put back")
			}
		} else {
			if bytes.Equal(stored, encoded) || fossilErr != nil {
				t.Error("The chunk was not rewritten, or the old copy was not kept as a fossil")
			}
			if _, err := decodeChunk(config, stored); err != nil {
				t.Errorf("The rewritten chunk can't be decoded: %v", err)
			}
		}
	}
}
