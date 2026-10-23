// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

// Fuzz targets for the code that parses data read from a storage, which must return errors rather than panic,
// hang or allocate without bound.  Without -fuzz only the seed corpus runs, as part of the normal test suite.
//
//	go test -run '^$' -fuzz FuzzChunkDecrypt -fuzztime 60s ./src

var fuzzChunkKey = bytes.Repeat([]byte{0x5a}, 32)

const fuzzDerivationKey = "fuzz chunk hash"

func fuzzConfig(compressionLevel int, dataShards int, parityShards int) *Config {
	config := CreateConfig()
	config.CompressionLevel = compressionLevel
	config.MaximumChunkSize = 64 * 1024
	config.DataShards = dataShards
	config.ParityShards = parityShards
	return config
}

func encodeFuzzSeed(f *testing.F, config *Config, key []byte, data []byte) []byte {
	chunk := CreateChunk(config, true)
	chunk.Reset(true)
	chunk.Write(data)
	if err := chunk.Encrypt(key, fuzzDerivationKey, false); err != nil {
		f.Fatal(err)
	}
	return append([]byte(nil), chunk.GetBytes()...)
}

func FuzzChunkDecrypt(f *testing.F) {
	SetLoggingLevel(FATAL)
	plain := bytes.Repeat([]byte("fuzz seed data "), 200)
	for _, level := range []int{DEFAULT_COMPRESSION_LEVEL, ZSTD_COMPRESSION_LEVEL_DEFAULT, 6} {
		f.Add(encodeFuzzSeed(f, fuzzConfig(level, 0, 0), nil, plain), false)
		f.Add(encodeFuzzSeed(f, fuzzConfig(level, 0, 0), fuzzChunkKey, plain), true)
	}
	f.Add(encodeFuzzSeed(f, fuzzConfig(DEFAULT_COMPRESSION_LEVEL, 3, 2), nil, plain), false)
	f.Add(encodeFuzzSeed(f, fuzzConfig(DEFAULT_COMPRESSION_LEVEL, 3, 2), fuzzChunkKey, plain), true)
	f.Add([]byte("duplicacy\002\x10\x00short"), true)
	f.Add([]byte("LZ4 \xff\xff\xff\x7f"), false)

	f.Fuzz(func(t *testing.T, data []byte, encrypted bool) {
		saved := minimumDecompressionLimit
		minimumDecompressionLimit = 1 << 20
		defer func() { minimumDecompressionLimit = saved }()

		key := []byte(nil)
		if encrypted {
			key = fuzzChunkKey
		}
		chunk := CreateChunk(fuzzConfig(DEFAULT_COMPRESSION_LEVEL, 0, 0), true)
		chunk.Reset(false)
		chunk.Write(data)
		err, _ := chunk.Decrypt(key, fuzzDerivationKey)
		if err == nil && chunk.GetLength() > minimumDecompressionLimit {
			t.Fatalf("Decrypted %d bytes, more than the limit", chunk.GetLength())
		}
	})
}

func FuzzEntryDecode(f *testing.F) {
	entry := &Entry{Path: "dir/file", Size: 150, Time: 1700000000, Mode: 0644, Hash: "abcd",
		StartChunk: 0, StartOffset: 50, EndChunk: 1, EndOffset: 100, UID: 501, GID: 20,
		Attributes: &map[string][]byte{"user.test": []byte("value")}}
	packed, _ := msgpack.Marshal(entry)
	f.Add(packed)
	var withHash bytes.Buffer
	entry.EncodeWithHash(msgpack.NewEncoder(&withHash))
	f.Add(withHash.Bytes())
	description, _ := json.Marshal(entry)
	f.Add(description)

	f.Fuzz(func(t *testing.T, data []byte) {
		chunkLengths := []int{100, 100}
		var decoded Entry
		if msgpack.Unmarshal(data, &decoded) == nil {
			decoded.check(chunkLengths)
			validateEntryPath(decoded.Path)
		}
		if withHash, err := DecodeEntryWithHash(msgpack.NewDecoder(bytes.NewReader(data))); err == nil {
			withHash.check(chunkLengths)
		}
		var fromJSON Entry
		if fromJSON.UnmarshalJSON(data) == nil {
			fromJSON.check(chunkLengths)
			validateEntryPath(fromJSON.Path)
		}
	})
}

func FuzzSnapshotDescription(f *testing.F) {
	snapshot := CreateEmptySnapshot("fuzz")
	snapshot.Revision = 3
	snapshot.SetSequence("files", []string{"aa", "bb"})
	snapshot.SetSequence("chunks", []string{"cc"})
	snapshot.SetSequence("lengths", []string{"dd"})
	if description, err := snapshot.MarshalJSON(); err == nil {
		f.Add(description)
	}
	f.Add([]byte(`{"id":"x","revision":1,"version":1,"files":["00"],"chunks":["01"],"lengths":["02"]}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		if snapshot, err := CreateSnapshotFromDescription(data); err == nil && snapshot == nil {
			t.Fatal("No snapshot and no error")
		}
	})
}

func FuzzConfigUnmarshal(f *testing.F) {
	if description, err := os.ReadFile(filepath.Join(fixtureDir, "plain-lz4", "storage", "config")); err == nil {
		f.Add(description)
	}
	f.Add([]byte(`{"compression-level":100,"rsa-public-key":"3059301306072a8648ce3d020106082a8648ce3d03010703420004"}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		config := CreateConfig()
		config.UnmarshalJSON(data)
	})
}
