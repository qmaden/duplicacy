// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

//go:build !windows

package duplicacy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/xattr"
)

// M2: extended attributes are no longer kept in memory for every entry during the backup but read when the entry
// is stored; they must still be backed up and restored.
func TestExtendedAttributesRoundTrip(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	repository := t.TempDir()
	file := filepath.Join(repository, "file")
	os.WriteFile(file, []byte("content"), 0600)
	if err := xattr.Set(file, "user.duplicacy_test", []byte("attribute value")); err != nil {
		t.Skipf("Extended attributes are not supported here: %v", err)
	}
	directory := filepath.Join(repository, "directory")
	os.Mkdir(directory, 0700)
	xattr.Set(directory, "user.duplicacy_test", []byte("directory value"))

	storageDir := backupForRestoreTest(t, repository)
	top := t.TempDir()
	if failed := restoreForTest(t, storageDir, top, true, false); failed != 0 {
		t.Fatalf("%d files failed to restore", failed)
	}
	for path, expected := range map[string]string{"file": "attribute value", "directory": "directory value"} {
		value, err := xattr.Get(filepath.Join(top, path), "user.duplicacy_test")
		if err != nil || string(value) != expected {
			t.Errorf("The attribute of %s was restored as %q (error %v) instead of %q", path, value, err, expected)
		}
	}
}
