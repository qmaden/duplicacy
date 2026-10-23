// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"fmt"
	"os"
	"testing"
)

// An interrupted backup saves its entry list as an incomplete snapshot, whether the list was still in memory or
// had already been moved to disk.  Loading it must give back each entry exactly once, in order.
func TestIncompleteSnapshotEntries(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)

	for _, maximumInMemoryEntries := range []int{1000, 5} {
		cachePath := t.TempDir()
		entryList, err := CreateEntryList("test", cachePath, maximumInMemoryEntries)
		if err != nil {
			t.Fatal(err)
		}
		var expected []string
		for i := 0; i < 20; i++ {
			path := fmt.Sprintf("dir%02d/", i)
			entryList.AddEntry(CreateEntry(path, 0, 1600000000, uint32(os.ModeDir|0755)))
			expected = append(expected, path)
		}
		entryList.SaveIncompleteSnapshot()

		loaded := loadIncompleteSnapshot("test", cachePath)
		if loaded == nil {
			t.Fatalf("%d in memory: the incomplete snapshot couldn't be loaded", maximumInMemoryEntries)
		}
		var paths []string
		err = loaded.ReadEntries(func(entry *Entry) error {
			paths = append(paths, entry.Path)
			return nil
		})
		loaded.CloseOnDiskFile()
		if err != nil {
			t.Fatalf("%d in memory: %v", maximumInMemoryEntries, err)
		}
		if fmt.Sprint(paths) != fmt.Sprint(expected) {
			t.Errorf("%d in memory: loaded %d entries %v", maximumInMemoryEntries, len(paths), paths)
		}
	}
}

// M3: the entry list moves to disk when its entries use too much memory, not only past a number of entries, and
// the in-memory copies are released once they are on disk.
func TestEntryListSpillsBySize(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	saved := maximumInMemoryEntryBytes
	maximumInMemoryEntryBytes = 10000
	defer func() { maximumInMemoryEntryBytes = saved }()

	entryList, err := CreateEntryList("test", t.TempDir(), 1000000)
	if err != nil {
		t.Fatal(err)
	}
	defer entryList.CloseOnDiskFile()
	for i := 0; i < 200; i++ {
		entryList.AddEntry(CreateEntry(fmt.Sprintf("directory-with-a-long-name-%04d/", i), 0, 1600000000,
			uint32(os.ModeDir|0755)))
	}
	if entryList.onDiskFile == nil {
		t.Fatal("The entry list was not moved to disk")
	}
	if len(entryList.entries) != 0 || entryList.inMemoryBytes != 0 {
		t.Errorf("%d entries (%d bytes) are still kept in memory", len(entryList.entries), entryList.inMemoryBytes)
	}
	count := 0
	if err = entryList.ReadEntries(func(entry *Entry) error { count++; return nil }); err != nil || count != 200 {
		t.Errorf("Read %d entries back, %v", count, err)
	}
}
