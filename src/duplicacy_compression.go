// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"encoding/binary"
	"fmt"
	"sync"

	golz4 "github.com/bkaradzic/go-lz4"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

// The LZ4 data in a chunk is the 4-byte little-endian length of the uncompressed data followed by a standard LZ4
// block, the format of github.com/bkaradzic/go-lz4.  github.com/pierrec/lz4 reads and writes the same blocks
// about 2.5 times faster, so it is used for both directions; go-lz4 is kept for data that LZ4 can't compress,
// where pierrec returns no block but go-lz4 still produces a valid one.

var lz4Compressors = sync.Pool{New: func() interface{} { return new(lz4.Compressor) }}

// lz4Encode writes the LZ4 encoding of 'src' into 'dst', which must have room for lz4EncodedBound(len(src))
// bytes, and returns the encoded part of 'dst'.
func lz4Encode(dst []byte, src []byte) ([]byte, error) {
	binary.LittleEndian.PutUint32(dst, uint32(len(src)))
	compressor := lz4Compressors.Get().(*lz4.Compressor)
	n, err := compressor.CompressBlock(src, dst[4:])
	lz4Compressors.Put(compressor)
	if err == nil && n > 0 {
		return dst[:4+n], nil
	}
	return golz4.Encode(dst, src)
}

func lz4EncodedBound(size int) int {
	bound := lz4.CompressBlockBound(size)
	if golz4Bound := golz4.CompressBound(size); golz4Bound > bound {
		bound = golz4Bound
	}
	return 4 + bound
}

// lz4Decode decodes LZ4 data into 'dst' (reusing its capacity when possible), refusing more than 'limit' bytes.
func lz4Decode(dst []byte, src []byte, limit int) ([]byte, error) {
	if len(src) < 4 {
		return nil, fmt.Errorf("LZ4 data truncated (%d bytes)", len(src))
	}
	length := binary.LittleEndian.Uint32(src)
	if uint64(length) > uint64(limit) {
		return nil, fmt.Errorf("LZ4 data claims %d bytes, more than the maximum of %d", length, limit)
	}
	if length == 0 {
		return dst[:0], nil
	}
	if cap(dst) < int(length) {
		dst = make([]byte, length)
	}
	dst = dst[:length]
	n, err := lz4.UncompressBlock(src[4:], dst)
	if err != nil {
		return nil, fmt.Errorf("LZ4 decompression error: %v", err)
	}
	if n != int(length) {
		return nil, fmt.Errorf("LZ4 data decoded to %d bytes instead of %d", n, length)
	}
	return dst, nil
}

// zstd encoders and decoders are expensive to create (megabytes of state each), so they are reused.  A pooled
// encoder compresses one chunk at a time on the calling goroutine; chunks are already compressed in parallel by
// the uploading threads.
var zstdEncoderPools sync.Map // zstd.EncoderLevel -> *sync.Pool

func zstdEncode(level zstd.EncoderLevel, dst []byte, src []byte) ([]byte, error) {
	poolValue, _ := zstdEncoderPools.LoadOrStore(level, &sync.Pool{})
	pool := poolValue.(*sync.Pool)
	encoder, _ := pool.Get().(*zstd.Encoder)
	if encoder == nil {
		var err error
		encoder, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(level), zstd.WithEncoderConcurrency(1))
		if err != nil {
			return nil, err
		}
	}
	dst = encoder.EncodeAll(src, dst)
	pool.Put(encoder)
	return dst, nil
}

var zstdDecoders sync.Map // limit -> *zstd.Decoder

// zstdDecode appends the decompression of 'src' to 'dst', refusing more than 'limit' bytes.  A decoder may be
// used by several goroutines at once.
func zstdDecode(dst []byte, src []byte, limit int) ([]byte, error) {
	decoderValue, found := zstdDecoders.Load(limit)
	if !found {
		decoder, err := zstd.NewReader(nil, zstd.WithDecoderMaxMemory(uint64(limit)), zstd.WithDecoderConcurrency(0))
		if err != nil {
			return nil, err
		}
		decoderValue, _ = zstdDecoders.LoadOrStore(limit, decoder)
	}
	decoded, err := decoderValue.(*zstd.Decoder).DecodeAll(src, dst)
	if err != nil {
		return nil, err
	}
	if len(decoded)-len(dst) > limit {
		return nil, fmt.Errorf("Decompressed data exceeds the maximum of %d bytes", limit)
	}
	return decoded, nil
}
