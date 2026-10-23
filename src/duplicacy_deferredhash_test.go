// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// C3b: backup chunks are hashed and deduplicated on the upload goroutines.  Identical chunks sent at the same time
// must be uploaded once, every chunk must get the hash it would have had if hashed while written, and the
// completions must report the rest as cached.
func TestUploadUnhashedDeduplicates(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	config := CreateConfigFromParameters(DEFAULT_COMPRESSION_LEVEL, 4096, 16384, 1024, false, nil, false)
	storageDir := t.TempDir()
	storage, err := CreateFileStorage(storageDir, false, 4)
	if err != nil {
		t.Fatal(err)
	}
	storage.SetDefaultNestingLevels([]int{1}, 1)
	storage.CreateDirectory(0, "chunks")

	operator := CreateChunkOperator(config, storage, nil, false, false, 4, false)
	defer operator.Stop()

	var lock sync.Mutex
	seen := make(map[string]bool)
	operator.UploadFilter = func(chunk *Chunk) bool {
		lock.Lock()
		defer lock.Unlock()
		found := seen[chunk.GetID()]
		seen[chunk.GetID()] = true
		return found
	}
	var cached, uploaded int32
	hashes := make([]string, 40)
	operator.UploadCompletionFunc = func(chunk *Chunk, chunkIndex int, inCache bool, chunkSize int, uploadSize int) {
		if inCache {
			atomic.AddInt32(&cached, 1)
		} else {
			atomic.AddInt32(&uploaded, 1)
		}
		hashes[chunkIndex] = chunk.GetHash()
		config.PutChunk(chunk)
	}

	expected := make([]string, 40)
	for i := 0; i < 40; i++ {
		data := compressionTestData(5000, int64(i%4), 1) // 4 distinct contents, each sent 10 times
		reference := CreateChunk(config, true)
		reference.Reset(true)
		reference.Write(data)
		expected[i] = reference.GetHash()

		chunk := config.GetChunk()
		chunk.Reset(true)
		chunk.hasher = nil
		chunk.deferredHash = true
		chunk.Write(data)
		operator.UploadUnhashed(chunk, i)
	}
	operator.WaitForCompletion()

	for i := range expected {
		if hashes[i] != expected[i] {
			t.Errorf("Chunk %d has the wrong hash", i)
		}
	}
	files, _ := filepath.Glob(filepath.Join(storageDir, "chunks", "*", "*"))
	if len(files) != 4 || uploaded != 4 || cached != 36 {
		t.Errorf("%d chunk files, %d uploaded and %d cached; expected 4, 4 and 36", len(files), uploaded, cached)
	}
}
