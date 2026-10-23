// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"testing"
)

// Benchmarks for the CPU-heavy parts of backup and restore.  Run them with
//
//	go test -run '^$' -bench . -benchmem ./src
//
// The data is half random and half text-like, in chunks of the default average size (4 MiB).

const benchmarkChunkSize = 4 << 20

func benchmarkConfig(compressionLevel int, encrypted bool) *Config {
	return CreateConfigFromParameters(compressionLevel, 4<<20, 16<<20, 1<<20, encrypted, nil, false)
}

func benchmarkEncrypt(b *testing.B, compressionLevel int, encrypted bool) {
	config := benchmarkConfig(compressionLevel, encrypted)
	data := compressionTestData(benchmarkChunkSize, 1, 0.5)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunk := config.GetChunk()
		chunk.Reset(true)
		chunk.Write(data)
		if err := chunk.Encrypt(config.ChunkKey, chunk.GetHash(), false); err != nil {
			b.Fatal(err)
		}
		config.PutChunk(chunk)
	}
}

func BenchmarkEncryptLZ4(b *testing.B)      { benchmarkEncrypt(b, DEFAULT_COMPRESSION_LEVEL, true) }
func BenchmarkEncryptLZ4Plain(b *testing.B) { benchmarkEncrypt(b, DEFAULT_COMPRESSION_LEVEL, false) }
func BenchmarkEncryptZstdDefault(b *testing.B) {
	benchmarkEncrypt(b, ZSTD_COMPRESSION_LEVEL_DEFAULT, true)
}
func BenchmarkEncryptZstdBest(b *testing.B) { benchmarkEncrypt(b, ZSTD_COMPRESSION_LEVEL_BEST, true) }
func BenchmarkEncryptZlib(b *testing.B)     { benchmarkEncrypt(b, 6, true) }

func benchmarkDecrypt(b *testing.B, compressionLevel int) {
	config := benchmarkConfig(compressionLevel, true)
	data := compressionTestData(benchmarkChunkSize, 1, 0.5)
	chunk := config.GetChunk()
	chunk.Reset(true)
	chunk.Write(data)
	hash := chunk.GetHash()
	if err := chunk.Encrypt(config.ChunkKey, hash, false); err != nil {
		b.Fatal(err)
	}
	encrypted := append([]byte(nil), chunk.GetBytes()...)
	config.PutChunk(chunk)

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunk := config.GetChunk()
		chunk.Reset(false)
		chunk.Write(encrypted)
		if err, _ := chunk.Decrypt(config.ChunkKey, hash); err != nil {
			b.Fatal(err)
		}
		config.PutChunk(chunk)
	}
}

func BenchmarkDecryptLZ4(b *testing.B)         { benchmarkDecrypt(b, DEFAULT_COMPRESSION_LEVEL) }
func BenchmarkDecryptZstdDefault(b *testing.B) { benchmarkDecrypt(b, ZSTD_COMPRESSION_LEVEL_DEFAULT) }

func BenchmarkChunkHash(b *testing.B) {
	config := benchmarkConfig(DEFAULT_COMPRESSION_LEVEL, true)
	data := compressionTestData(benchmarkChunkSize, 1, 0.5)
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		hasher := config.NewKeyedHasher(config.HashKey)
		hasher.Write(data)
		hasher.Sum(nil)
	}
}

// The content-defined chunker, including the chunk and file hashes it computes as it goes
func BenchmarkChunkMaker(b *testing.B) {
	config := benchmarkConfig(DEFAULT_COMPRESSION_LEVEL, true)
	data := compressionTestData(64<<20, 1, 0.5)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		maker := CreateFileChunkMaker(config, false)
		send := func(chunk *Chunk) { chunk.GetHash(); config.PutChunk(chunk) }
		maker.AddData(bytes.NewReader(data), send)
		maker.AddData(nil, send)
	}
}
