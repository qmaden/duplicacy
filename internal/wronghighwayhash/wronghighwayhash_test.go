// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package wronghighwayhash

import (
	"bufio"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
)

// fill generates the same pseudo-random bytes as the program that produced testdata/golden.txt.
func fill(b []byte, seed int) {
	x := uint32(seed)*2654435761 + 1
	for i := range b {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		b[i] = byte(x)
	}
}

// testdata/golden.txt was produced on arm64 by the original assembly in
// github.com/gilbertchen/highwayhash (identical output with Go 1.21 and Go 1.26).  Each line is
// 'keySeed length hash', where keySeed 0 is the all-zero key that Duplicacy uses.
func TestGoldenVectors(t *testing.T) {
	file, err := os.Open("testdata/golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		keySeed, _ := strconv.Atoi(fields[0])
		length, _ := strconv.Atoi(fields[1])

		key := make([]byte, Size)
		if keySeed > 0 {
			fill(key, 1000+keySeed)
		}
		msg := make([]byte, length)
		fill(msg, length*7+keySeed)

		h, err := New(key)
		if err != nil {
			t.Fatal(err)
		}
		h.Write(msg)
		if actual := hex.EncodeToString(h.Sum(nil)); actual != fields[2] {
			t.Errorf("key %d, length %d: hash is %s instead of %s", keySeed, length, actual, fields[2])
		}

		// Writing in uneven pieces must not change the result
		h.Reset()
		for i, step := 0, 1; i < len(msg); i, step = i+step, step%37+1 {
			end := i + step
			if end > len(msg) {
				end = len(msg)
			}
			h.Write(msg[i:end])
		}
		if actual := hex.EncodeToString(h.Sum(nil)); actual != fields[2] {
			t.Errorf("key %d, length %d (incremental): hash is %s instead of %s", keySeed, length, actual, fields[2])
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 844 {
		t.Errorf("Only %d golden vectors were checked", count)
	}
}

func TestInvalidKey(t *testing.T) {
	if _, err := New(make([]byte, 16)); err == nil {
		t.Error("A 16-byte key should be rejected")
	}
}
