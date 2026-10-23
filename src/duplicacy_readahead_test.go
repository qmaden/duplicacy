// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/iotest"
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
