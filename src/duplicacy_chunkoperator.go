// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// These are operations that ChunkOperator will perform.
const (
	ChunkOperationDownload  = 0
	ChunkOperationUpload    = 1
	ChunkOperationDelete    = 2
	ChunkOperationFossilize = 3
	ChunkOperationResurrect = 4
	ChunkOperationFind      = 5
)

// chunkRetryDelays are the waits before each retry of a failed chunk transfer.  It is a variable so that tests can
// shorten it.
var chunkRetryDelays = []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}

// ChunkTask is used to pass parameters for different kinds of chunk operations.
type ChunkTask struct {
	operation  int    // The type of operation
	chunkID    string // The chunk id
	chunkHash  string // The chunk hash
	chunkIndex int    // The chunk index
	filePath   string // The path of the chunk file; it may be empty

	isMetadata bool // The chunk is a metadata chunk rather than a file chunk
	skipCache  bool // Bypass the snapshot cache, i.e., read the chunk from the storage itself
	chunk      *Chunk

	completionFunc func(chunk *Chunk, chunkIndex int)
}

// ChunkOperator is capable of performing multi-threaded operations on chunks.
type ChunkOperator struct {
	config         *Config // Associated config
	storage        Storage // This storage
	snapshotCache  *FileStorage
	showStatistics bool
	threads        int            // Number of threads
	taskQueue      chan ChunkTask // Operating goroutines are waiting on this channel for input
	stopChannel    chan bool      // Used to stop all the goroutines

	numberOfActiveTasks int64      // The number of chunks that are being operated on, guarded by taskLock
	taskLock            sync.Mutex // Guards numberOfActiveTasks
	tasksDone           *sync.Cond // Signalled when numberOfActiveTasks drops to 0
	stopped             int32      // Set by the first call to Stop()

	fossils        []string    // For fossilize operation, the paths of the fossils are stored in this slice
	collectionLock *sync.Mutex // The lock for accessing 'fossils'

	startTime           int64 // The time it starts downloading
	totalChunkSize      int64 // Total chunk size
	downloadedChunkSize int64 // Downloaded chunk size

	allowFailures        bool  // Whether to fail on download error, or continue
	NumberOfFailedChunks int64 // The number of chunks that can't be downloaded

	rewriteChunks bool // Whether to rewrite corrupted chunks when erasure coding is enabled

	UploadCompletionFunc func(chunk *Chunk, chunkIndex int, inCache bool, chunkSize int, uploadSize int)
}

// CreateChunkOperator creates a new ChunkOperator.
func CreateChunkOperator(config *Config, storage Storage, snapshotCache *FileStorage, showStatistics bool, rewriteChunks bool, threads int,
	allowFailures bool) *ChunkOperator {

	operator := &ChunkOperator{
		config:         config,
		storage:        storage,
		snapshotCache:  snapshotCache,
		showStatistics: showStatistics,
		threads:        threads,

		taskQueue:   make(chan ChunkTask, threads),
		stopChannel: make(chan bool),

		collectionLock: &sync.Mutex{},
		startTime:      time.Now().Unix(),
		allowFailures:  allowFailures,
		rewriteChunks:  rewriteChunks,
	}
	operator.tasksDone = sync.NewCond(&operator.taskLock)

	// Start the operator goroutines
	for i := 0; i < operator.threads; i++ {
		go func(threadIndex int) {
			defer CatchLogException()
			for {
				select {
				case task := <-operator.taskQueue:
					operator.Run(threadIndex, task)
				case <-operator.stopChannel:
					return
				}
			}
		}(i)
	}

	return operator
}

func (operator *ChunkOperator) Stop() {
	// Stop() may be called more than once
	if !atomic.CompareAndSwapInt32(&operator.stopped, 0, 1) {
		return
	}

	operator.WaitForCompletion()
	for i := 0; i < operator.threads; i++ {
		operator.stopChannel <- false
	}

}

// WaitForCompletion returns once every task added so far has finished.
func (operator *ChunkOperator) WaitForCompletion() {
	operator.taskLock.Lock()
	for operator.numberOfActiveTasks > 0 {
		operator.tasksDone.Wait()
	}
	operator.taskLock.Unlock()
}

func (operator *ChunkOperator) AddTask(operation int, chunkID string, chunkHash string, filePath string, chunkIndex int, chunk *Chunk, isMetadata bool, skipCache bool, completionFunc func(*Chunk, int)) {

	task := ChunkTask{
		operation:      operation,
		chunkID:        chunkID,
		chunkHash:      chunkHash,
		chunkIndex:     chunkIndex,
		filePath:       filePath,
		chunk:          chunk,
		isMetadata:     isMetadata,
		skipCache:      skipCache,
		completionFunc: completionFunc,
	}

	// Count the task before a worker can pick it up and finish it, so the count never drops below the number of
	// tasks still pending and WaitForCompletion can't return early
	operator.taskLock.Lock()
	operator.numberOfActiveTasks++
	operator.taskLock.Unlock()
	operator.taskQueue <- task

	return
}

func (operator *ChunkOperator) Download(chunkHash string, chunkIndex int, isMetadata bool) *Chunk {
	return operator.download(chunkHash, chunkIndex, isMetadata, false)
}

// DownloadFromStorage downloads a chunk from the storage, bypassing the snapshot cache.  This is needed when
// the copy of the chunk in the storage is to be verified.
func (operator *ChunkOperator) DownloadFromStorage(chunkHash string, chunkIndex int, isMetadata bool) *Chunk {
	return operator.download(chunkHash, chunkIndex, isMetadata, true)
}

func (operator *ChunkOperator) download(chunkHash string, chunkIndex int, isMetadata bool, skipCache bool) *Chunk {
	chunkID := operator.config.GetChunkIDFromHash(chunkHash)
	completionChannel := make(chan *Chunk)
	completionFunc := func(chunk *Chunk, chunkIndex int) {
		completionChannel <- chunk
	}
	operator.AddTask(ChunkOperationDownload, chunkID, chunkHash, "", chunkIndex, nil, isMetadata, skipCache, completionFunc)
	return <-completionChannel
}

func (operator *ChunkOperator) DownloadAsync(chunkHash string, chunkIndex int, isMetadata bool, completionFunc func(*Chunk, int)) {
	chunkID := operator.config.GetChunkIDFromHash(chunkHash)
	operator.AddTask(ChunkOperationDownload, chunkID, chunkHash, "", chunkIndex, nil, isMetadata, false, completionFunc)
}

func (operator *ChunkOperator) Upload(chunk *Chunk, chunkIndex int, isMetadata bool) {
	chunkHash := chunk.GetHash()
	chunkID := operator.config.GetChunkIDFromHash(chunkHash)
	operator.AddTask(ChunkOperationUpload, chunkID, chunkHash, "", chunkIndex, chunk, isMetadata, false, nil)
}

// Delete deletes the chunk file at 'filePath'.  If 'filePath' is empty the chunk will be located by its id,
// under the metadata directory if 'isMetadata' is true.
func (operator *ChunkOperator) Delete(chunkID string, filePath string, isMetadata bool) {
	operator.AddTask(ChunkOperationDelete, chunkID, "", filePath, 0, nil, isMetadata, false, nil)
}

// Fossilize turns the chunk file at 'filePath' into a fossil.  If 'filePath' is empty the chunk will be
// located by its id, under the metadata directory if 'isMetadata' is true.
func (operator *ChunkOperator) Fossilize(chunkID string, filePath string, isMetadata bool) {
	operator.AddTask(ChunkOperationFossilize, chunkID, "", filePath, 0, nil, isMetadata, false, nil)
}

// Resurrect turns the fossil at 'filePath' back into a chunk.  The chunk is restored to the directory the
// fossil resides in.
func (operator *ChunkOperator) Resurrect(chunkID string, filePath string) {
	operator.AddTask(ChunkOperationResurrect, chunkID, "", filePath, 0, nil, IsMetadataChunkPath(filePath), false, nil)
}

func (operator *ChunkOperator) Run(threadIndex int, task ChunkTask) {
	defer func() {
		operator.taskLock.Lock()
		operator.numberOfActiveTasks--
		if operator.numberOfActiveTasks == 0 {
			operator.tasksDone.Broadcast()
		}
		operator.taskLock.Unlock()
	}()

	if task.operation == ChunkOperationDownload {
		operator.DownloadChunk(threadIndex, task)
		return
	} else if task.operation == ChunkOperationUpload {
		operator.UploadChunk(threadIndex, task)
		return
	}

	// task.filePath may be empty.  If so, find the chunk first.
	if task.operation == ChunkOperationDelete || task.operation == ChunkOperationFossilize {
		if task.filePath == "" {
			filePath, exist, _, err := operator.storage.FindChunk(threadIndex, task.chunkID, false, task.isMetadata)
			if err != nil {
				LOG_ERROR("CHUNK_FIND", "Failed to locate the path for the chunk %s: %v", task.chunkID, err)
				return
			} else if !exist {
				if task.operation == ChunkOperationDelete {
					LOG_WARN("CHUNK_FIND", "Chunk %s does not exist in the storage", task.chunkID)
					return
				}

				fossilPath, exist, _, _ := operator.storage.FindChunk(threadIndex, task.chunkID, true, task.isMetadata)
				if exist {
					LOG_WARN("CHUNK_FOSSILIZE", "Chunk %s is already a fossil", task.chunkID)
					operator.collectionLock.Lock()
					operator.fossils = append(operator.fossils, fossilPath)
					operator.collectionLock.Unlock()
				} else {
					LOG_ERROR("CHUNK_FIND", "Chunk %s does not exist in the storage", task.chunkID)
				}
				return
			}
			task.filePath = filePath
		}
	}

	if task.operation == ChunkOperationFind {
		_, exist, _, err := operator.storage.FindChunk(threadIndex, task.chunkID, false, task.isMetadata)
		if err != nil {
			LOG_ERROR("CHUNK_FIND", "Failed to locate the path for the chunk %s: %v", task.chunkID, err)
		} else if !exist {
			LOG_ERROR("CHUNK_FIND", "Chunk %s does not exist in the storage", task.chunkID)
		} else {
			LOG_DEBUG("CHUNK_FIND", "Chunk %s exists in the storage", task.chunkID)
		}
	} else if task.operation == ChunkOperationDelete {
		err := operator.storage.DeleteFile(threadIndex, task.filePath)
		if err != nil {
			LOG_WARN("CHUNK_DELETE", "Failed to remove the file %s: %v", task.filePath, err)
		} else {
			if task.chunkID != "" {
				LOG_INFO("CHUNK_DELETE", "The chunk %s has been permanently removed", task.chunkID)
			} else {
				LOG_INFO("CHUNK_DELETE", "Deleted file %s from the storage", task.filePath)
			}
		}
	} else if task.operation == ChunkOperationFossilize {

		fossilPath := task.filePath + ".fsl"

		err := operator.storage.MoveFile(threadIndex, task.filePath, fossilPath)
		if err != nil {
			if _, exist, _, _ := operator.storage.FindChunk(threadIndex, task.chunkID, true, task.isMetadata); exist {
				err := operator.storage.DeleteFile(threadIndex, task.filePath)
				if err == nil {
					LOG_TRACE("CHUNK_DELETE", "Deleted chunk file %s as the fossil already exists", task.chunkID)
				}
				operator.collectionLock.Lock()
				operator.fossils = append(operator.fossils, fossilPath)
				operator.collectionLock.Unlock()
			} else {
				LOG_ERROR("CHUNK_DELETE", "Failed to fossilize the chunk %s: %v", task.chunkID, err)
			}
		} else {
			LOG_TRACE("CHUNK_FOSSILIZE", "The chunk %s has been marked as a fossil", task.chunkID)
			operator.collectionLock.Lock()
			operator.fossils = append(operator.fossils, fossilPath)
			operator.collectionLock.Unlock()
		}
	} else if task.operation == ChunkOperationResurrect {
		chunkPath, exist, _, err := operator.storage.FindChunk(threadIndex, task.chunkID, false, task.isMetadata)
		if err != nil {
			LOG_ERROR("CHUNK_FIND", "Failed to locate the path for the chunk %s: %v", task.chunkID, err)
		}

		if exist {
			operator.storage.DeleteFile(threadIndex, task.filePath)
			LOG_INFO("FOSSIL_RESURRECT", "The chunk %s already exists", task.chunkID)
		} else {
			err := operator.storage.MoveFile(threadIndex, task.filePath, chunkPath)
			if err != nil {
				LOG_ERROR("FOSSIL_RESURRECT", "Failed to resurrect the chunk %s from the fossil %s: %v",
					task.chunkID, task.filePath, err)
			} else {
				LOG_INFO("FOSSIL_RESURRECT", "The chunk %s has been resurrected", task.filePath)
			}
		}
	}
}

// Download downloads a chunk from the storage.
func (operator *ChunkOperator) DownloadChunk(threadIndex int, task ChunkTask) {

	cachedPath := ""
	chunk := operator.config.GetChunk()
	chunk.isMetadata = task.isMetadata
	chunkID := task.chunkID

	defer func() {
		if chunk != nil {
			operator.config.PutChunk(chunk)
		}
	}()

	if task.isMetadata && operator.snapshotCache != nil && !task.skipCache {

		var exist bool
		var err error

		// Reset the chunk with a hasher -- we're reading from the cache where chunk are not encrypted or compressed
		chunk.Reset(true)

		cachedPath, exist, _, err = operator.snapshotCache.FindChunk(threadIndex, chunkID, false, task.isMetadata)
		if err != nil {
			LOG_WARN("DOWNLOAD_CACHE", "Failed to find the cache path for the chunk %s: %v", chunkID, err)
		} else if exist {
			err = operator.snapshotCache.DownloadFile(0, cachedPath, chunk)
			if err != nil {
				LOG_WARN("DOWNLOAD_CACHE", "Failed to load the chunk %s from the snapshot cache: %v", chunkID, err)
			} else {
				actualChunkID := chunk.GetID()
				if actualChunkID != chunkID {
					LOG_WARN("DOWNLOAD_CACHE_CORRUPTED",
						"The chunk %s load from the snapshot cache has a hash id of %s", chunkID, actualChunkID)
				} else {
					LOG_DEBUG("CHUNK_CACHE", "Chunk %s has been loaded from the snapshot cache", chunkID)

					task.completionFunc(chunk, task.chunkIndex)
					chunk = nil
					return
				}
			}
		}
	}

	// Reset the chunk without a hasher -- the downloaded content will be encrypted and/or compressed and the hasher
	// will be set up before the encryption
	chunk.Reset(false)
	chunk.isMetadata = task.isMetadata

	// If failures are allowed, complete the task properly
	completeFailedChunk := func() {

		atomic.AddInt64(&operator.NumberOfFailedChunks, 1)
		if operator.allowFailures {
			chunk.isBroken = true
			task.completionFunc(chunk, task.chunkIndex)
			// The chunk now belongs to the caller; it must not also be returned to the pool by the deferred
			// PutChunk, or two goroutines would end up sharing it
			chunk = nil
		}
	}

	chunkPath := ""
	fossilPath := ""
	filePath := ""

	const MaxDownloadAttempts = 3
	for downloadAttempt := 0; ; downloadAttempt++ {

		exist := false
		var err error

		// Find the chunk by ID first.
		chunkPath, exist, _, err = operator.storage.FindChunk(threadIndex, chunkID, false, task.isMetadata)
		if err != nil {
			completeFailedChunk()
			LOG_WERROR(operator.allowFailures, "DOWNLOAD_CHUNK", "Failed to find the chunk %s: %v", chunkID, err)
			return
		}

		if exist {
			filePath = chunkPath
		} else {
			// No chunk is found.  Have to find it in the fossil pool again.
			fossilPath, exist, _, err = operator.storage.FindChunk(threadIndex, chunkID, true, task.isMetadata)
			if err != nil {
				completeFailedChunk()
				LOG_WERROR(operator.allowFailures, "DOWNLOAD_CHUNK", "Failed to find the chunk %s: %v", chunkID, err)
				return
			}

			if !exist {

				retry := false

				// Retry for Hubic or WebDAV as it may return 404 even when the chunk exists
				if _, ok := operator.storage.(*HubicStorage); ok {
					retry = true
				}

				if _, ok := operator.storage.(*WebDAVStorage); ok {
					retry = true
				}

				if retry && downloadAttempt < MaxDownloadAttempts {
					LOG_WARN("DOWNLOAD_RETRY", "Failed to find the chunk %s; retrying", chunkID)
					continue
				}

				// A chunk is not found.  This is a serious error and hopefully it will never happen.
				completeFailedChunk()
				if err != nil {
					LOG_WERROR(operator.allowFailures, "DOWNLOAD_CHUNK", "Chunk %s can't be found: %v", chunkID, err)
				} else {
					LOG_WERROR(operator.allowFailures, "DOWNLOAD_CHUNK", "Chunk %s can't be found", chunkID)
				}
				return
			}

			filePath = fossilPath
			LOG_WARN("DOWNLOAD_FOSSIL", "Chunk %s is a fossil", chunkID)
		}

		err = operator.storage.DownloadFile(threadIndex, filePath, chunk)
		if err != nil {
			// The chunk was found, so a failed download is retried (Hubic may also return 404 for an existing
			// chunk); errors other than a truncated transfer wait before the next attempt
			if downloadAttempt < MaxDownloadAttempts {
				LOG_WARN("DOWNLOAD_RETRY", "Failed to download the chunk %s: %v; retrying", chunkID, err)
				if err != io.ErrUnexpectedEOF && downloadAttempt < len(chunkRetryDelays) {
					time.Sleep(chunkRetryDelays[downloadAttempt])
				}
				chunk.Reset(false)
				chunk.isMetadata = task.isMetadata
				continue
			} else {
				completeFailedChunk()
				LOG_WERROR(operator.allowFailures, "DOWNLOAD_CHUNK", "Failed to download the chunk %s: %v", chunkID, err)
				return
			}
		}

		rewriteNeeded := false
		err, rewriteNeeded = chunk.Decrypt(operator.config.ChunkKey, task.chunkHash)
		if err != nil {
			if downloadAttempt < MaxDownloadAttempts {
				LOG_WARN("DOWNLOAD_RETRY", "Failed to decrypt the chunk %s: %v; retrying", chunkID, err)
				chunk.Reset(false)
				chunk.isMetadata = task.isMetadata
				continue
			} else {
				completeFailedChunk()
				LOG_WERROR(operator.allowFailures, "DOWNLOAD_DECRYPT", "Failed to decrypt the chunk %s: %v", chunkID, err)
				return
			}
		}

		actualChunkID := chunk.GetID()
		if actualChunkID != chunkID {
			if downloadAttempt < MaxDownloadAttempts {
				LOG_WARN("DOWNLOAD_RETRY", "The chunk %s has a hash id of %s; retrying", chunkID, actualChunkID)
				chunk.Reset(false)
				chunk.isMetadata = task.isMetadata
				continue
			} else {
				completeFailedChunk()
				LOG_WERROR(operator.allowFailures, "DOWNLOAD_CORRUPTED", "The chunk %s has a hash id of %s", chunkID, actualChunkID)
				return
			}
		}

		if rewriteNeeded && operator.rewriteChunks {
			operator.rewriteChunk(threadIndex, chunk, chunkID, chunkPath, filePath, fossilPath, task.isMetadata)
		}

		break
	}

	if chunk.isMetadata && len(cachedPath) > 0 {
		// Save a copy to the local snapshot cache
		err := operator.snapshotCache.UploadFile(threadIndex, cachedPath, chunk.GetBytes())
		if err != nil {
			LOG_WARN("DOWNLOAD_CACHE", "Failed to add the chunk %s to the snapshot cache: %v", chunkID, err)
		}
	}

	downloadedChunkSize := atomic.AddInt64(&operator.downloadedChunkSize, int64(chunk.GetLength()))

	totalChunkSize := atomic.LoadInt64(&operator.totalChunkSize)
	if (operator.showStatistics || IsTracing()) && totalChunkSize > 0 {

		now := time.Now().Unix()
		if now <= operator.startTime {
			now = operator.startTime + 1
		}
		speed := downloadedChunkSize / (now - operator.startTime)
		remainingTime := int64(0)
		if speed > 0 {
			remainingTime = (totalChunkSize-downloadedChunkSize)/speed + 1
		}
		percentage := float32(downloadedChunkSize * 1000 / totalChunkSize)
		LOG_INFO("DOWNLOAD_PROGRESS", "Downloaded chunk %d size %d, %sB/s %s %.1f%%",
			task.chunkIndex+1, chunk.GetLength(),
			PrettySize(speed), PrettyTime(remainingTime), percentage/10)
	} else {
		LOG_DEBUG("CHUNK_DOWNLOAD", "Chunk %s has been downloaded", chunkID)
	}

	task.completionFunc(chunk, task.chunkIndex)
	chunk = nil
	return
}

// UploadChunk is called by the task goroutines to perform the actual uploading
func (operator *ChunkOperator) UploadChunk(threadIndex int, task ChunkTask) bool {

	chunk := task.chunk
	chunkID := task.chunkID
	chunkSize := chunk.GetLength()

	// The data in the buffer is checked against the chunk id by Encrypt() below

	if task.isMetadata && operator.snapshotCache != nil && operator.storage.IsCacheNeeded() {
		// Save a copy to the local snapshot.
		chunkPath, exist, _, err := operator.snapshotCache.FindChunk(threadIndex, chunkID, false, task.isMetadata)
		if err != nil {
			LOG_WARN("UPLOAD_CACHE", "Failed to find the cache path for the chunk %s: %v", chunkID, err)
		} else if exist {
			LOG_DEBUG("CHUNK_CACHE", "Chunk %s already exists in the snapshot cache", chunkID)
		} else if err = operator.snapshotCache.UploadFile(threadIndex, chunkPath, chunk.GetBytes()); err != nil {
			LOG_WARN("UPLOAD_CACHE", "Failed to save the chunk %s to the snapshot cache: %v", chunkID, err)
		} else {
			LOG_DEBUG("CHUNK_CACHE", "Chunk %s has been saved to the snapshot cache", chunkID)
		}
	}

	// This returns the path the chunk file should be at.
	chunkPath, exist, _, err := operator.storage.FindChunk(threadIndex, chunkID, false, task.isMetadata)
	if err != nil {
		LOG_ERROR("UPLOAD_CHUNK", "Failed to find the path for the chunk %s: %v", chunkID, err)
		return false
	}

	if exist {
		// Chunk deduplication by name in effect here.
		LOG_DEBUG("CHUNK_DUPLICATE", "Chunk %s already exists", chunkID)

		operator.UploadCompletionFunc(chunk, task.chunkIndex, false, chunkSize, 0)
		return false
	}

	// Encrypt the chunk only after we know that it must be uploaded.
	err = chunk.Encrypt(operator.config.ChunkKey, chunk.GetHash(), task.isMetadata)
	if err != nil {
		LOG_ERROR("UPLOAD_CHUNK", "Failed to encrypt the chunk %s: %v", chunkID, err)
		return false
	}

	if !operator.config.dryRun {
		// A chunk upload can be repeated safely, so transient failures (a dropped connection, a busy server) are
		// retried with increasing delays before the backup gives up (upstream #683)
		for attempt := 0; ; attempt++ {
			err = operator.storage.UploadFile(threadIndex, chunkPath, chunk.GetBytes())
			if err == nil || attempt >= len(chunkRetryDelays) {
				break
			}
			LOG_WARN("UPLOAD_RETRY", "Failed to upload the chunk %s: %v; retrying in %v", chunkID, err, chunkRetryDelays[attempt])
			time.Sleep(chunkRetryDelays[attempt])
		}
		if err != nil {
			LOG_ERROR("UPLOAD_CHUNK", "Failed to upload the chunk %s: %v", chunkID, err)
			return false
		}
		LOG_DEBUG("CHUNK_UPLOAD", "Chunk %s has been uploaded", chunkID)
	} else {
		LOG_DEBUG("CHUNK_UPLOAD", "Uploading was skipped for chunk %s", chunkID)
	}

	operator.UploadCompletionFunc(chunk, task.chunkIndex, false, chunkSize, chunk.GetLength())
	return true
}

// rewriteChunk stores a freshly encoded copy of a chunk that could only be read by recovering it from its erasure
// coding parity.  The old copy is moved aside as a fossil first (some storages can't overwrite a file in place)
// and moved back if the new copy can't be stored, so the chunk is never left only as a fossil.
func (operator *ChunkOperator) rewriteChunk(threadIndex int, chunk *Chunk, chunkID string, chunkPath string,
	filePath string, fossilPath string, isMetadata bool) {

	// Encode the new copy before touching the old one
	newChunk := operator.config.GetChunk()
	defer operator.config.PutChunk(newChunk)
	newChunk.Reset(true)
	newChunk.Write(chunk.GetBytes())
	if err := newChunk.Encrypt(operator.config.ChunkKey, chunk.GetHash(), isMetadata); err != nil {
		LOG_WARN("CHUNK_REWRITE", "Failed to encode the chunk %s: %v", chunkID, err)
		return
	}

	movedPath := ""
	if filePath != fossilPath {
		movedPath = filePath + ".fsl"
		if err := operator.storage.MoveFile(threadIndex, chunkPath, movedPath); err != nil {
			LOG_WARN("CHUNK_REWRITE", "Failed to fossilize the chunk %s; it is left as it is: %v", chunkID, err)
			return
		}
		LOG_TRACE("CHUNK_REWRITE", "The existing chunk %s has been marked as a fossil for rewrite", chunkID)
	}

	if err := operator.storage.UploadFile(threadIndex, chunkPath, newChunk.GetBytes()); err != nil {
		LOG_WARN("CHUNK_REWRITE", "Failed to re-upload the chunk %s: %v", chunkID, err)
		if movedPath != "" {
			if err = operator.storage.MoveFile(threadIndex, movedPath, chunkPath); err != nil {
				LOG_WARN("CHUNK_REWRITE", "Failed to restore the chunk %s from %s: %v", chunkID, movedPath, err)
			}
		}
		return
	}

	LOG_INFO("CHUNK_REWRITE", "The chunk %s has been re-uploaded", chunkID)
	if movedPath != "" {
		operator.collectionLock.Lock()
		operator.fossils = append(operator.fossils, movedPath)
		operator.collectionLock.Unlock()
	}
}

// sequenceDownloader returns the chunks of a sequence in order while downloading up to 'window' of the following
// chunks in parallel.  Metadata (file lists, chunk and length sequences) used to be downloaded one chunk at a time,
// one round trip per chunk.
type sequenceDownloader struct {
	operator   *ChunkOperator
	hashes     []string
	isMetadata bool
	results    []chan *Chunk
	issued     int
	consumed   int
	window     int
}

// NewSequenceDownloader starts downloading the chunks of a sequence.  Close must be called if not every chunk is
// taken with Next.
func (operator *ChunkOperator) NewSequenceDownloader(hashes []string, isMetadata bool) *sequenceDownloader {
	window := operator.threads
	if window < 1 {
		window = 1
	} else if window > 16 {
		window = 16
	}
	downloader := &sequenceDownloader{
		operator:   operator,
		hashes:     hashes,
		isMetadata: isMetadata,
		results:    make([]chan *Chunk, len(hashes)),
		window:     window,
	}
	downloader.fill()
	return downloader
}

func (downloader *sequenceDownloader) fill() {
	for downloader.issued < len(downloader.hashes) && downloader.issued-downloader.consumed < downloader.window {
		index := downloader.issued
		result := make(chan *Chunk, 1)
		downloader.results[index] = result
		downloader.operator.DownloadAsync(downloader.hashes[index], index, downloader.isMetadata, func(chunk *Chunk, chunkIndex int) {
			result <- chunk
		})
		downloader.issued++
	}
}

// Next returns the next chunk of the sequence, which the caller must return with PutChunk, or nil at the end.
func (downloader *sequenceDownloader) Next() *Chunk {
	if downloader.consumed >= len(downloader.hashes) {
		return nil
	}
	chunk := <-downloader.results[downloader.consumed]
	downloader.results[downloader.consumed] = nil
	downloader.consumed++
	downloader.fill()
	return chunk
}

// Close waits for the chunks downloaded ahead but not taken and returns them to the pool.
func (downloader *sequenceDownloader) Close() {
	for ; downloader.consumed < downloader.issued; downloader.consumed++ {
		downloader.operator.config.PutChunk(<-downloader.results[downloader.consumed])
		downloader.results[downloader.consumed] = nil
	}
}
