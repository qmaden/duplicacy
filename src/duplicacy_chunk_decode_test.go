// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func erasureCodedChunk(t *testing.T, data []byte) (config *Config, encoded []byte) {
	config = CreateConfig()
	config.MaximumChunkSize = 64 * 1024
	config.DataShards = 5
	config.ParityShards = 2

	chunk := CreateChunk(config, true)
	chunk.Reset(true)
	chunk.Write(data)
	if err := chunk.Encrypt(nil, "", false); err != nil {
		t.Fatal(err)
	}
	return config, append([]byte(nil), chunk.GetBytes()...)
}

func decodeChunk(config *Config, encoded []byte) (*Chunk, error) {
	chunk := CreateChunk(config, true)
	chunk.Reset(false)
	chunk.Write(encoded)
	err, _ := chunk.Decrypt(nil, "")
	return chunk, err
}

// setErasureCodingHeader rewrites a header field and its XOR checksum, as a corrupted or crafted chunk would.
func setErasureCodingHeader(encoded []byte, offset int, value []byte) []byte {
	modified := append([]byte(nil), encoded...)
	header := modified[len(ERASURE_CODING_BANNER) : len(ERASURE_CODING_BANNER)+14]
	copy(header[offset:], value)
	header[12] = header[0] ^ header[2] ^ header[4] ^ header[6] ^ header[8] ^ header[10]
	header[13] = header[1] ^ header[3] ^ header[5] ^ header[7] ^ header[9] ^ header[11]
	return modified
}

// B3: the shard size used to be computed from the config instead of the chunk header, which divided by zero
// when the config had no erasure coding.
func TestErasureCodingUsesHeaderParameters(t *testing.T) {
	data := bytes.Repeat([]byte("erasure coding "), 1000)
	_, encoded := erasureCodedChunk(t, data)

	plainConfig := CreateConfig()
	plainConfig.MaximumChunkSize = 64 * 1024
	chunk, err := decodeChunk(plainConfig, encoded)
	if err != nil {
		t.Fatalf("Failed to decode an erasure coded chunk with a config without erasure coding: %v", err)
	}
	if !bytes.Equal(chunk.GetBytes(), data) {
		t.Error("Decoded data doesn't match")
	}
}

// S5: a corrupted or crafted header must produce an error, not a panic.
func TestErasureCodingRejectsInvalidHeaders(t *testing.T) {
	config, encoded := erasureCodedChunk(t, bytes.Repeat([]byte("x"), 5000))

	zero := make([]byte, 2)
	huge := make([]byte, 8)
	binary.LittleEndian.PutUint64(huge, 1<<62)
	many := make([]byte, 2)
	binary.LittleEndian.PutUint16(many, 300)

	cases := map[string][]byte{
		"zero data shards":   setErasureCodingHeader(encoded, 8, zero),
		"zero parity shards": setErasureCodingHeader(encoded, 10, zero),
		"too many shards":    setErasureCodingHeader(encoded, 8, many),
		"huge chunk size":    setErasureCodingHeader(encoded, 0, huge),
		"zero chunk size":    setErasureCodingHeader(encoded, 0, make([]byte, 8)),
	}
	for name, modified := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: decoding panicked: %v", name, r)
				}
			}()
			if _, err := decodeChunk(config, modified); err == nil {
				t.Errorf("%s: decoding succeeded", name)
			}
		}()
	}
}

// S5: compressed data from the storage must not decompress to more than the limit.
func TestDecompressionLimit(t *testing.T) {
	saved := minimumDecompressionLimit
	minimumDecompressionLimit = 1 << 20
	defer func() { minimumDecompressionLimit = saved }()

	config := CreateConfig()
	config.MaximumChunkSize = 64 * 1024
	limit := config.maximumDecompressedSize()
	if limit != 1<<20 {
		t.Fatalf("Unexpected limit %d", limit)
	}

	zstdData := func(size int) []byte {
		var buffer bytes.Buffer
		buffer.WriteString("ZSTD")
		// A small window keeps the frame acceptable so that the limit on the output is what's tested (in
		// production the limit is at least 256 MiB, far above any window size)
		writer, _ := zstd.NewWriter(&buffer, zstd.WithWindowSize(1<<17))
		writer.Write(make([]byte, size))
		writer.Close()
		return buffer.Bytes()
	}
	zlibData := func(size int) []byte {
		var buffer bytes.Buffer
		writer := zlib.NewWriter(&buffer)
		writer.Write(make([]byte, size))
		writer.Close()
		return buffer.Bytes()
	}
	lz4Claim := make([]byte, 64)
	copy(lz4Claim, "LZ4 ")
	binary.LittleEndian.PutUint32(lz4Claim[4:], 100<<20)

	for name, data := range map[string][]byte{
		"zstd": zstdData(limit + 1),
		"zlib": zlibData(limit + 1),
		"lz4":  lz4Claim,
	} {
		if _, err := decodeChunk(config, data); err == nil {
			t.Errorf("%s: data larger than the limit was accepted", name)
		} else if !strings.Contains(err.Error(), "maximum") && !strings.Contains(err.Error(), "exceed") {
			t.Errorf("%s: unexpected error: %v", name, err)
		}
	}

	for name, data := range map[string][]byte{"zstd": zstdData(limit), "zlib": zlibData(limit)} {
		chunk, err := decodeChunk(config, data)
		if err != nil {
			t.Errorf("%s: data at the limit was rejected: %v", name, err)
		} else if chunk.GetLength() != limit {
			t.Errorf("%s: decoded %d bytes instead of %d", name, chunk.GetLength(), limit)
		}
	}
}
