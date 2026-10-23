// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bufio"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
)

// testdata/blake2b_vectors.txt was produced with github.com/minio/blake2b-simd, which computed every chunk hash and
// chunk id of existing storages.  Each line is 'keyed <key> <size> <hash>' (keys: "duplicacy", 32 and 64 bytes) or
// 'file <size> <hash>' for the unkeyed file hash.
func TestBlake2bVectors(t *testing.T) {
	file, err := os.Open("testdata/blake2b_vectors.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	config := CreateConfig()
	keys := [][]byte{[]byte("duplicacy"), fixtureBytes(32, 99), fixtureBytes(64, 98)}
	checked := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		var actual string
		if fields[0] == "keyed" {
			key, _ := strconv.Atoi(fields[1])
			size, _ := strconv.Atoi(fields[2])
			hasher := config.NewKeyedHasher(keys[key])
			hasher.Write(fixtureBytes(size, uint32(size)))
			actual = hex.EncodeToString(hasher.Sum(nil))
		} else {
			size, _ := strconv.Atoi(fields[1])
			hasher := config.NewFileHasher()
			hasher.Write(fixtureBytes(size, uint32(size)))
			actual = hex.EncodeToString(hasher.Sum(nil))
		}
		if expected := fields[len(fields)-1]; actual != expected {
			t.Errorf("%s: %s instead of %s", strings.Join(fields[:len(fields)-1], " "), actual, expected)
		}
		checked++
	}
	if checked != 26 {
		t.Errorf("Only %d vectors were checked", checked)
	}
}
