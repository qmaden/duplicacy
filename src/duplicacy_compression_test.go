// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"math/rand"
	"testing"

	golz4 "github.com/bkaradzic/go-lz4"
	"github.com/klauspost/compress/zstd"
)

func compressionTestData(size int, seed int64, randomFraction float64) []byte {
	r := rand.New(rand.NewSource(seed))
	data := make([]byte, size)
	random := int(float64(size) * randomFraction)
	r.Read(data[:random])
	words := [][]byte{[]byte("backup "), []byte("chunk "), []byte("snapshot\n"), []byte("0123456789")}
	for i := random; i < size; {
		i += copy(data[i:], words[r.Intn(len(words))])
	}
	return data
}

// The LZ4 framing must stay identical to github.com/bkaradzic/go-lz4, which wrote every existing LZ4 chunk: data
// encoded by either library must decode with the other.
func TestLZ4InteroperatesWithGoLZ4(t *testing.T) {
	for _, size := range []int{0, 1, 13, 64, 1000, 4096, 65536, 1 << 20, 4 << 20} {
		for _, randomFraction := range []float64{0, 0.5, 1} {
			data := compressionTestData(size, int64(size), randomFraction)

			encoded, err := lz4Encode(make([]byte, lz4EncodedBound(size)), data)
			if err != nil {
				t.Fatalf("size %d: %v", size, err)
			}
			decodedByGoLZ4, err := golz4.Decode(nil, encoded)
			if err != nil || !bytes.Equal(decodedByGoLZ4, data) {
				t.Errorf("size %d, random %v: go-lz4 can't decode the new encoding: %v", size, randomFraction, err)
			}

			oldEncoding, _ := golz4.Encode(nil, data)
			decoded, err := lz4Decode(nil, oldEncoding, 64<<20)
			if err != nil || !bytes.Equal(decoded, data) {
				t.Errorf("size %d, random %v: can't decode go-lz4 data: %v", size, randomFraction, err)
			}
		}
	}
}

// Pooled zstd encoders and shared decoders must produce and read standard zstd data, including the streamed frames
// written by earlier versions.
func TestZstdPooledCodecs(t *testing.T) {
	data := compressionTestData(1<<20, 7, 0.3)
	for _, level := range []zstd.EncoderLevel{zstd.SpeedFastest, zstd.SpeedDefault, zstd.SpeedBetterCompression, zstd.SpeedBestCompression} {
		encoded, err := zstdEncode(level, nil, data)
		if err != nil {
			t.Fatal(err)
		}
		reader, _ := zstd.NewReader(bytes.NewReader(encoded))
		var standard bytes.Buffer
		standard.ReadFrom(reader)
		reader.Close()
		if !bytes.Equal(standard.Bytes(), data) {
			t.Errorf("level %v: a standard decoder can't read the pooled encoder's output", level)
		}
	}

	var streamed bytes.Buffer
	writer, _ := zstd.NewWriter(&streamed)
	writer.Write(data)
	writer.Close()
	decoded, err := zstdDecode(nil, streamed.Bytes(), 64<<20)
	if err != nil || !bytes.Equal(decoded, data) {
		t.Errorf("Can't decode a streamed zstd frame: %v", err)
	}
}
