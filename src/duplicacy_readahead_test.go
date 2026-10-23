// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"
)

func TestReadAheadReader(t *testing.T) {
	config := CreateConfig()
	for _, size := range []int{0, 1, readAheadBlockSize - 1, readAheadBlockSize, readAheadBlockSize + 1, 5*readAheadBlockSize + 3} {
		data := compressionTestData(size, int64(size), 0.5)
		expected := config.NewFileHasher()
		expected.Write(data)

		// Read in odd-sized pieces, as the chunker does
		reader := newReadAheadReader(bytes.NewReader(data), config.NewFileHasher())
		var output bytes.Buffer
		if _, err := io.CopyBuffer(&output, struct{ io.Reader }{reader}, make([]byte, 7777)); err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if !bytes.Equal(output.Bytes(), data) {
			t.Errorf("size %d: the data read differs", size)
		}
		if reader.FileHash() != hex.EncodeToString(expected.Sum(nil)) {
			t.Errorf("size %d: wrong file hash", size)
		}
		reader.Close()
	}
}

func TestReadAheadReaderError(t *testing.T) {
	failure := errors.New("disk error")
	data := compressionTestData(3*readAheadBlockSize, 1, 0.5)
	reader := newReadAheadReader(io.MultiReader(bytes.NewReader(data), iotest.ErrReader(failure)), CreateConfig().NewFileHasher())
	output, err := io.ReadAll(reader)
	if err != failure {
		t.Errorf("The read error was %v instead of %v", err, failure)
	}
	if !bytes.Equal(output, data) {
		t.Errorf("%d bytes were returned before the error instead of %d", len(output), len(data))
	}

	// Abandoning a reader must not leave its goroutine blocked forever
	abandoned := newReadAheadReader(bytes.NewReader(make([]byte, 10*readAheadBlockSize)), CreateConfig().NewFileHasher())
	abandoned.Read(make([]byte, 10))
	abandoned.Close()
}

// Files read ahead get the right file hash, which restore verifies.
func TestBackupWithReadAhead(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	repository := t.TempDir()
	for i, size := range []int{readAheadThreshold - 1, readAheadThreshold, 3*readAheadThreshold + 12345} {
		os.WriteFile(filepath.Join(repository, string(rune('a'+i))), compressionTestData(size, int64(i), 0.5), 0600)
	}
	storageDir := backupForRestoreTest(t, repository)
	top := t.TempDir()
	if failed := restoreForTest(t, storageDir, top, true, false); failed != 0 {
		t.Fatalf("%d files failed to restore", failed)
	}
	for i, size := range []int{readAheadThreshold - 1, readAheadThreshold, 3*readAheadThreshold + 12345} {
		restored, _ := os.ReadFile(filepath.Join(top, string(rune('a'+i))))
		if !bytes.Equal(restored, compressionTestData(size, int64(i), 0.5)) {
			t.Errorf("The file of %d bytes was not restored correctly", size)
		}
	}
}

// slowDownloadStorage adds a delay to every download, like a remote storage, and records how many downloads
// were in progress at the same time.
type slowDownloadStorage struct {
	*FileStorage
	delay       time.Duration
	inFlight    int32
	maxInFlight int32
}

func (storage *slowDownloadStorage) DownloadFile(threadIndex int, filePath string, chunk *Chunk) error {
	current := atomic.AddInt32(&storage.inFlight, 1)
	for {
		maximum := atomic.LoadInt32(&storage.maxInFlight)
		if current <= maximum || atomic.CompareAndSwapInt32(&storage.maxInFlight, maximum, current) {
			break
		}
	}
	time.Sleep(storage.delay)
	atomic.AddInt32(&storage.inFlight, -1)
	return storage.FileStorage.DownloadFile(threadIndex, filePath, chunk)
}

// Upstream #541: restore prefetched chunks only within the current file, so restoring many small files from a
// remote storage downloaded about one chunk at a time.  With 8 threads, the downloads must overlap.
func TestRestorePrefetchesAcrossFiles(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	repository := t.TempDir()
	for i := 0; i < 200; i++ {
		os.WriteFile(filepath.Join(repository, fmt.Sprintf("file%03d", i)), compressionTestData(6000, int64(i), 1), 0600)
	}
	storageDir := backupForRestoreTest(t, repository)

	top := t.TempDir()
	os.MkdirAll(filepath.Join(top, DUPLICACY_DIRECTORY), 0700)
	SetDuplicacyPreferencePath(filepath.Join(top, DUPLICACY_DIRECTORY))
	fileStorage, _ := CreateFileStorage(storageDir, false, 8)
	storage := &slowDownloadStorage{FileStorage: fileStorage, delay: 50 * time.Millisecond}
	fileStorage.DerivedStorage = storage
	manager := CreateBackupManager("test", storage, top, "", "", "", false)
	manager.SetupSnapshotCache("default")

	start := time.Now()
	if failed := manager.Restore(top, 1, true, false, 8, true, false, false, false, nil, false); failed != 0 {
		t.Fatalf("%d files failed to restore", failed)
	}
	elapsed := time.Since(start)
	chunks, _ := filepath.Glob(filepath.Join(storageDir, "chunks", "*", "*"))
	sequential := time.Duration(len(chunks)) * storage.delay
	// The delay dominates the local work, so this ratio is the average number of downloads in progress
	concurrency := float64(sequential) / float64(elapsed)
	t.Logf("Restored %d chunks in %v (%v if downloaded one at a time): %.1f downloads in progress on average",
		len(chunks), elapsed, sequential, concurrency)
	if concurrency < 3 {
		t.Errorf("Only %.1f of 8 threads were downloading on average", concurrency)
	}
}
