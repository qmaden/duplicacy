// Copyright (c) 2017 Minio Inc. All rights reserved.
// Use of this source code is governed by a license that can be
// found in the LICENSE file.

// Package wronghighwayhash reproduces, in pure Go, the incorrect HighwayHash-256 that
// github.com/gilbertchen/highwayhash (a fork of github.com/minio/highwayhash 1.0.1) computes on
// arm64.  Duplicacy CLI 3.0.1 used that hash for the shard checksums of erasure coded chunks, so
// it is needed to read those chunks.  It must never be used to write new data.
//
// The arm64 assembly in that fork declared its TBL constant table as the global symbol
// ·zipperMerge, which collides with the Go function of the same name.  The linker resolved the
// assembly reference to the ABI0 wrapper of the Go function, so the 48 bytes loaded as the
// table were machine code instead of the intended constants.  Those bytes are pinned below,
// which makes the result independent of the Go compiler and of the architecture.  (Go 1.27
// refuses to link the original assembly at all.)
package wronghighwayhash

import (
	"encoding/binary"
	"errors"
	"hash"
)

// Size is the size of the HighwayHash-256 checksum in bytes.
const Size = 32

var errKeySize = errors.New("wronghighwayhash: invalid key size")

// The 48 bytes the arm64 assembly loaded into V28, V29 and V30: the code of the ABI0 wrapper
// generated for zipperMerge.  Every byte of the two multiply tables is 32 or larger, so the TBL
// lookups that feed the multiplies always produce zeros, and only two bytes of the zipper merge
// table select data.  The values are those of the binaries built with Go 1.21 through 1.26.
var asmTable = [48]byte{
	// V28: zipper merge indices
	0xfe, 0x0f, 0x1d, 0xf8, 0xfd, 0x83, 0x1f, 0xf8, 0xfd, 0x23, 0x00, 0xd1, 0xe0, 0x1f, 0x40, 0xf9,
	// V29: indices that set up the first operand of the multiplies
	0xe1, 0x23, 0x40, 0xf9, 0xe2, 0x27, 0x40, 0xf9, 0xe3, 0x2b, 0x40, 0xf9, 0x41, 0xfe, 0xff, 0x97,
	// V30: indices that set up the second operand of the multiplies
	0xfd, 0x83, 0x5f, 0xf8, 0xfe, 0x07, 0x43, 0xf8, 0xc0, 0x03, 0x5f, 0xd6, 0x00, 0x00, 0x00, 0x00,
}

const (
	v0   = 0
	v1   = 4
	mul0 = 8
	mul1 = 12
)

var (
	init0 = [4]uint64{0xdbe6d5d5fe4cce2f, 0xa4093822299f31d0, 0x13198a2e03707344, 0x243f6a8885a308d3}
	init1 = [4]uint64{0x3bd39e10cb0ef593, 0xc0acf169b5f18a8c, 0xbe5466cf34e90c6c, 0x452821e638d01377}
)

// New returns a hash.Hash computing the arm64 variant of HighwayHash-256.
// It returns a non-nil error if the key is not 32 bytes long.
func New(key []byte) (hash.Hash, error) {
	if len(key) != Size {
		return nil, errKeySize
	}
	h := &digest{}
	copy(h.key[:], key)
	h.Reset()
	return h, nil
}

type digest struct {
	state [16]uint64 // v0 | v1 | mul0 | mul1

	key, buffer [Size]byte
	offset      int
}

func (d *digest) Size() int { return Size }

func (d *digest) BlockSize() int { return Size }

func (d *digest) Reset() {
	initialize(&d.state, d.key[:])
	d.offset = 0
}

func (d *digest) Write(p []byte) (n int, err error) {
	n = len(p)
	if d.offset > 0 {
		remaining := Size - d.offset
		if n < remaining {
			d.offset += copy(d.buffer[d.offset:], p)
			return
		}
		copy(d.buffer[d.offset:], p[:remaining])
		update(&d.state, d.buffer[:])
		p = p[remaining:]
		d.offset = 0
	}
	if nn := len(p) & (^(Size - 1)); nn > 0 {
		update(&d.state, p[:nn])
		p = p[nn:]
	}
	if len(p) > 0 {
		d.offset = copy(d.buffer[d.offset:], p)
	}
	return
}

func (d *digest) Sum(b []byte) []byte {
	state := d.state
	if d.offset > 0 {
		hashBuffer(&state, &d.buffer, d.offset)
	}
	var hash [Size]byte
	finalize(hash[:], &state)
	return append(b, hash[:]...)
}

func hashBuffer(state *[16]uint64, buffer *[32]byte, offset int) {
	var block [Size]byte
	mod32 := (uint64(offset) << 32) + uint64(offset)
	for i := range state[:4] {
		state[i] += mod32
	}
	for i := range state[4:8] {
		t0 := uint32(state[i+4])
		t0 = (t0 << uint(offset)) | (t0 >> uint(32-offset))

		t1 := uint32(state[i+4] >> 32)
		t1 = (t1 << uint(offset)) | (t1 >> uint(32-offset))

		state[i+4] = (uint64(t1) << 32) | uint64(t0)
	}

	mod4 := offset & 3
	remain := offset - mod4

	copy(block[:], buffer[:remain])
	if offset >= 16 {
		copy(block[28:], buffer[offset-4:])
	} else if mod4 != 0 {
		last := uint32(buffer[remain])
		last += uint32(buffer[remain+mod4>>1]) << 8
		last += uint32(buffer[offset-1]) << 16
		binary.LittleEndian.PutUint32(block[16:], last)
	}
	update(state, block[:])
}

// initialize matches both the generic code and initializeArm64, which loaded the correct constants.
func initialize(state *[16]uint64, k []byte) {
	var key [4]uint64

	key[0] = binary.LittleEndian.Uint64(k[0:])
	key[1] = binary.LittleEndian.Uint64(k[8:])
	key[2] = binary.LittleEndian.Uint64(k[16:])
	key[3] = binary.LittleEndian.Uint64(k[24:])

	copy(state[mul0:], init0[:])
	copy(state[mul1:], init1[:])

	for i, k := range key {
		state[v0+i] = init0[i] ^ k
	}

	key[0] = key[0]>>32 | key[0]<<32
	key[1] = key[1]>>32 | key[1]<<32
	key[2] = key[2]>>32 | key[2]<<32
	key[3] = key[3]>>32 | key[3]<<32

	for i, k := range key {
		state[v1+i] = init1[i] ^ k
	}
}

// update emulates the UPDATE macro of updateArm64 instruction by instruction, with the TBL
// lookups reading asmTable.  'msg' must be a multiple of 32 bytes long.
func update(state *[16]uint64, msg []byte) {
	for len(msg) >= Size {
		// Add message, then v1 += mul0
		for i := 0; i < 4; i++ {
			state[v1+i] += binary.LittleEndian.Uint64(msg[8*i:])
			state[v1+i] += state[mul0+i]
		}

		// First pair of multiplies
		product0 := multiply(lookup32(asmTable[16:32], laneBytes32(state[v0:v0+4])),
			lookup32(asmTable[32:48], laneBytes32(state[v1:v1+4])))

		// v0 += mul1
		for i := 0; i < 4; i++ {
			state[v0+i] += state[mul1+i]
		}

		// Second pair of multiplies
		product1 := multiply(lookup32(asmTable[16:32], laneBytes32(state[v1:v1+4])),
			lookup32(asmTable[32:48], laneBytes32(state[v0:v0+4])))

		for i := 0; i < 4; i++ {
			state[mul0+i] ^= product0[i]
		}

		// Zipper merges, one 128-bit register at a time
		zipperAdd(state[v0:v0+2], state[v1:v1+2])
		zipperAdd(state[v0+2:v0+4], state[v1+2:v1+4])
		zipperAdd(state[v1:v1+2], state[v0:v0+2])
		zipperAdd(state[v1+2:v1+4], state[v0+2:v0+4])

		for i := 0; i < 4; i++ {
			state[mul1+i] ^= product1[i]
		}

		msg = msg[Size:]
	}
}

// laneBytes32 returns the little-endian bytes of four 64-bit lanes, i.e. a pair of NEON registers.
func laneBytes32(lanes []uint64) (b [32]byte) {
	for i, lane := range lanes {
		binary.LittleEndian.PutUint64(b[8*i:], lane)
	}
	return b
}

// lookup32 emulates 'VTBL indices.B16, [Va.B16, Vb.B16], Vd.B16': indices out of range yield 0.
func lookup32(indices []byte, table [32]byte) (out [16]byte) {
	for i, index := range indices {
		if index < 32 {
			out[i] = table[index]
		}
	}
	return out
}

// multiply emulates the umull/umull2 pair on the 32-bit lanes of two registers.
func multiply(a, b [16]byte) (product [4]uint64) {
	for i := 0; i < 4; i++ {
		product[i] = uint64(binary.LittleEndian.Uint32(a[4*i:])) * uint64(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return product
}

// zipperAdd emulates 'VTBL V28.B16, [src.B16], tmp.B16' followed by 'VADD tmp.D2, dst.D2, dst.D2'.
func zipperAdd(dst []uint64, src []uint64) {
	var table [16]byte
	binary.LittleEndian.PutUint64(table[0:], src[0])
	binary.LittleEndian.PutUint64(table[8:], src[1])

	var merged [16]byte
	for i, index := range asmTable[:16] {
		if index < 16 {
			merged[i] = table[index]
		}
	}
	dst[0] += binary.LittleEndian.Uint64(merged[0:])
	dst[1] += binary.LittleEndian.Uint64(merged[8:])
}

// finalize matches finalizeGeneric; the arm64 version differs only through update.
func finalize(out []byte, state *[16]uint64) {
	var perm [4]uint64
	var tmp [32]byte
	for i := 0; i < 10; i++ {
		perm[0] = state[v0+2]>>32 | state[v0+2]<<32
		perm[1] = state[v0+3]>>32 | state[v0+3]<<32
		perm[2] = state[v0+0]>>32 | state[v0+0]<<32
		perm[3] = state[v0+1]>>32 | state[v0+1]<<32

		binary.LittleEndian.PutUint64(tmp[0:], perm[0])
		binary.LittleEndian.PutUint64(tmp[8:], perm[1])
		binary.LittleEndian.PutUint64(tmp[16:], perm[2])
		binary.LittleEndian.PutUint64(tmp[24:], perm[3])

		update(state, tmp[:])
	}

	h0, h1 := reduceMod(state[v0+0]+state[mul0+0], state[v0+1]+state[mul0+1], state[v1+0]+state[mul1+0], state[v1+1]+state[mul1+1])
	binary.LittleEndian.PutUint64(out[0:], h0)
	binary.LittleEndian.PutUint64(out[8:], h1)

	h0, h1 = reduceMod(state[v0+2]+state[mul0+2], state[v0+3]+state[mul0+3], state[v1+2]+state[mul1+2], state[v1+3]+state[mul1+3])
	binary.LittleEndian.PutUint64(out[16:], h0)
	binary.LittleEndian.PutUint64(out[24:], h1)
}

// reduce v = [v0, v1, v2, v3] mod the irreducible polynomial x^128 + x^2 + x
func reduceMod(v0, v1, v2, v3 uint64) (r0, r1 uint64) {
	v3 &= 0x3FFFFFFFFFFFFFFF

	r0, r1 = v2, v3

	v3 = (v3 << 1) | (v2 >> (64 - 1))
	v2 <<= 1
	r1 = (r1 << 2) | (r0 >> (64 - 2))
	r0 <<= 2

	r0 ^= v0 ^ v2
	r1 ^= v1 ^ v3
	return
}
