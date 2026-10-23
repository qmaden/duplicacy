// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

// Snapshot file lists are msgpack-encoded entries, and interrupted backups are saved as msgpack too.
// testdata/msgpack_golden.txt holds the bytes written by vmihailenco/msgpack v4.0.4 for entries covering every
// integer and string size class; the library in use must produce and read exactly the same bytes.  The entry list
// of an interrupted backup only lives in the local cache, so for it reading the old bytes is enough (v5 writes the
// []int chunk lengths in their shortest form, v4 always used 9 bytes; both decode either).  Regenerate
// only to add cases, never after changing the msgpack library:
//
//	DUPLICACY_GENERATE_MSGPACK=1 go test -run TestMsgpackGolden .

const msgpackGoldenPath = "testdata/msgpack_golden.txt"

func msgpackTestEntries() []*Entry {
	long := strings.Repeat("d", 300)
	huge := strings.Repeat("e", 70000)
	return []*Entry{
		{Path: "", Mode: 0},
		{Path: "a", Size: 1, Time: 1, Mode: 0644, Hash: "00", UID: 0, GID: 0, StartChunk: 0, StartOffset: 0,
			EndChunk: 0, EndOffset: 1},
		{Path: "dir/", Mode: uint32(os.ModeDir) | 0755, Time: 1700000000, UID: 501, GID: 20},
		{Path: "link", Mode: uint32(os.ModeSymlink) | 0777, Link: "../target/" + long, Time: -1},
		{Path: "dir/" + strings.Repeat("f", 40), Size: 127, Time: 128, Mode: 0600, Hash: strings.Repeat("ab", 32),
			UID: 255, GID: 256, StartChunk: 65535, StartOffset: 65536, EndChunk: 1 << 31, EndOffset: 4294967296},
		{Path: huge, Size: 1<<62 + 5, Time: -129, Mode: 0xffffffff, UID: -1, GID: -32769, StartChunk: -33,
			StartOffset: -32, EndChunk: 7, EndOffset: 8},
		{Path: "attrs", Size: 10, Mode: 0644, Attributes: &map[string][]byte{
			"com.apple.provenance": {1, 2, 3},
			"user.empty":           {},
			"user.binary":          {0, 0xff, 0x80, '\n'},
			"user.long":            []byte(long),
		}},
		{Path: "unicode/árvíztűrő tükörfúrógép 日本", Size: 3, Mode: 0644, Hash: "ff"},
	}
}

func msgpackTestEntryList() *EntryList {
	return &EntryList{
		SnapshotID: "host@repo",
		Token:      "0123456789abcdef0123456789abcdef",
		ModifiedEntries: []ModifiedEntry{
			{Path: "a", Size: 1, Hash: "00"},
			{Path: "dir/b", Size: -1, Hash: ""},
		},
		UploadedChunkHashes:   []string{strings.Repeat("1", 64), strings.Repeat("2", 64)},
		UploadedChunkLengths:  []int{4194304, 1},
		PreservedChunkHashes:  []string{},
		PreservedChunkLengths: nil,
		Checksum:              strings.Repeat("c", 64),
		NumberOfEntries:       12345,
	}
}

// msgpackGoldenCases returns the encoded bytes of every case, keyed by name, in a fixed order.
func msgpackGoldenCases(t *testing.T) (names []string, encoded map[string][]byte) {
	encoded = make(map[string][]byte)
	add := func(name string, encode func(encoder *msgpack.Encoder) error) {
		buffer := new(bytes.Buffer)
		if err := encode(msgpack.NewEncoder(buffer)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		names = append(names, name)
		encoded[name] = buffer.Bytes()
	}

	for i, entry := range msgpackTestEntries() {
		entry := entry
		add("entry"+string(rune('0'+i)), func(encoder *msgpack.Encoder) error { return encoder.Encode(entry) })
		add("hashed"+string(rune('0'+i)), func(encoder *msgpack.Encoder) error { return entry.EncodeWithHash(encoder) })
	}
	add("entrylist", func(encoder *msgpack.Encoder) error { return encoder.Encode(msgpackTestEntryList()) })
	add("token", func(encoder *msgpack.Encoder) error { return encoder.EncodeString(msgpackTestEntryList().Token) })
	return names, encoded
}

func TestMsgpackGolden(t *testing.T) {
	names, encoded := msgpackGoldenCases(t)

	if os.Getenv("DUPLICACY_GENERATE_MSGPACK") != "" {
		var b strings.Builder
		for _, name := range names {
			b.WriteString(name + " " + hex.EncodeToString(encoded[name]) + "\n")
		}
		if err := os.WriteFile(msgpackGoldenPath, []byte(b.String()), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}

	file, err := os.Open(msgpackGoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	golden := make(map[string][]byte)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			t.Fatalf("malformed line in %s", msgpackGoldenPath)
		}
		golden[fields[0]], err = hex.DecodeString(fields[1])
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	// Encoding must produce the recorded bytes
	for _, name := range names {
		if name == "entrylist" {
			continue
		}
		if !bytes.Equal(encoded[name], golden[name]) {
			t.Errorf("%s: encoded bytes differ from %s", name, msgpackGoldenPath)
		}
	}

	// The recorded bytes must decode to the original values
	for i, expected := range msgpackTestEntries() {
		var entry Entry
		if err := msgpack.Unmarshal(golden["entry"+string(rune('0'+i))], &entry); err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if !reflect.DeepEqual(&entry, expected) {
			t.Errorf("entry %d decoded as %+v", i, entry)
		}

		hashed, err := DecodeEntryWithHash(msgpack.NewDecoder(bytes.NewReader(golden["hashed"+string(rune('0'+i))])))
		if err != nil {
			t.Fatalf("hashed entry %d: %v", i, err)
		}
		if !reflect.DeepEqual(hashed, expected) {
			t.Errorf("hashed entry %d decoded as %+v", i, hashed)
		}
	}

	expectedList := msgpackTestEntryList()
	for _, source := range [][]byte{golden["entrylist"], encoded["entrylist"]} {
		checkMsgpackEntryList(t, source, expectedList)
	}

	token, err := msgpack.NewDecoder(bytes.NewReader(golden["token"])).DecodeString()
	if err != nil || token != expectedList.Token {
		t.Errorf("token decoded as %q, %v", token, err)
	}
}

func checkMsgpackEntryList(t *testing.T, encoded []byte, expectedList *EntryList) {
	entryList := &EntryList{}
	if err := msgpack.NewDecoder(bytes.NewReader(encoded)).Decode(&entryList); err != nil {
		t.Fatal(err)
	}
	if entryList.SnapshotID != expectedList.SnapshotID || entryList.Token != expectedList.Token ||
		!reflect.DeepEqual(entryList.ModifiedEntries, expectedList.ModifiedEntries) ||
		!reflect.DeepEqual(entryList.UploadedChunkHashes, expectedList.UploadedChunkHashes) ||
		!reflect.DeepEqual(entryList.UploadedChunkLengths, expectedList.UploadedChunkLengths) ||
		len(entryList.PreservedChunkHashes) != 0 || len(entryList.PreservedChunkLengths) != 0 ||
		entryList.Checksum != expectedList.Checksum || entryList.NumberOfEntries != expectedList.NumberOfEntries {
		t.Errorf("entry list decoded as %+v", entryList)
	}
}
