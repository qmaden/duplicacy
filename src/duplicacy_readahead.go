// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"encoding/hex"
	"hash"
	"io"
)

// The chunker runs on a single goroutine for the whole backup, and used to read each file and compute its hash
// itself.  For large files, readAheadReader reads ahead on another goroutine and computes the file hash there, so
// that disk reads and file hashing overlap with chunking.

const (
	readAheadBlockSize = 1 << 20
	readAheadBlocks    = 4
	// Smaller files are read directly; starting a goroutine per file would cost more than it saves
	readAheadThreshold = readAheadBlockSize
)

type readAheadReader struct {
	blocks  chan []byte // blocks read and hashed, in order
	free    chan []byte // buffers available for reading
	stop    chan struct{}
	errs    chan error // the error that ended reading (io.EOF at the end of the file)
	current []byte
	offset  int
	err     error
	hasher  hash.Hash
}

func newReadAheadReader(reader io.Reader, hasher hash.Hash) *readAheadReader {
	r := &readAheadReader{
		blocks: make(chan []byte, readAheadBlocks),
		free:   make(chan []byte, readAheadBlocks+1),
		stop:   make(chan struct{}),
		errs:   make(chan error, 1),
		hasher: hasher,
	}
	for i := 0; i < readAheadBlocks+1; i++ {
		r.free <- make([]byte, readAheadBlockSize)
	}

	go func() {
		defer close(r.blocks)
		for {
			var buffer []byte
			select {
			case buffer = <-r.free:
			case <-r.stop:
				return
			}
			n, err := io.ReadFull(reader, buffer)
			if n > 0 {
				r.hasher.Write(buffer[:n])
				select {
				case r.blocks <- buffer[:n]:
				case <-r.stop:
					return
				}
			}
			if err != nil {
				if err == io.ErrUnexpectedEOF {
					err = io.EOF
				}
				r.errs <- err
				return
			}
		}
	}()
	return r
}

// Read implements io.Reader.  It returns the read error only after all data read before it.
func (r *readAheadReader) Read(p []byte) (int, error) {
	for r.offset >= len(r.current) {
		if r.current != nil {
			r.free <- r.current[:cap(r.current)]
			r.current = nil
		}
		if r.err != nil {
			return 0, r.err
		}
		block, ok := <-r.blocks
		if !ok {
			r.err = <-r.errs
			return 0, r.err
		}
		r.current, r.offset = block, 0
	}
	n := copy(p, r.current[r.offset:])
	r.offset += n
	return n, nil
}

// FileHash returns the hash of everything read; it is only complete once Read has returned io.EOF.
func (r *readAheadReader) FileHash() string {
	return hex.EncodeToString(r.hasher.Sum(nil))
}

// Close stops the reading goroutine if the reader is abandoned before the end of the file.
func (r *readAheadReader) Close() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}

// fileHashProvider is implemented by readers that compute the file hash themselves.
type fileHashProvider interface {
	FileHash() string
}
