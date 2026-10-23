// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The attributes of restored files are applied on several goroutines; every file, including empty ones, must end
// up with its own permissions and modification time, and directories with theirs.
func TestRestoreAppliesAttributesToEveryFile(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	repository := t.TempDir()
	permissions := []os.FileMode{0600, 0640, 0644, 0700, 0755}
	type expectation struct {
		mode  os.FileMode
		mtime int64
	}
	expected := make(map[string]expectation)
	for i := 0; i < 300; i++ {
		dir := filepath.Join(repository, fmt.Sprintf("dir%02d", i%10))
		os.MkdirAll(dir, 0755)
		name := filepath.Join(dir, fmt.Sprintf("file%03d", i))
		size := i % 7 * 1000 // some files are empty
		os.WriteFile(name, compressionTestData(size, int64(i), 1), 0600)
		mode := permissions[i%len(permissions)]
		mtime := time.Unix(1600000000+int64(i)*3600, 0)
		os.Chmod(name, mode)
		os.Chtimes(name, mtime, mtime)
		relative, _ := filepath.Rel(repository, name)
		expected[relative] = expectation{mode, mtime.Unix()}
	}
	directoryTime := time.Unix(1500000000, 0)
	os.Chtimes(filepath.Join(repository, "dir03"), directoryTime, directoryTime)

	storageDir := backupForRestoreTest(t, repository)
	top := t.TempDir()
	if failed := restoreForTest(t, storageDir, top, false, false); failed != 0 {
		t.Fatalf("%d files failed to restore", failed)
	}

	for relative, want := range expected {
		info, err := os.Lstat(filepath.Join(top, relative))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want.mode || info.ModTime().Unix() != want.mtime {
			t.Errorf("%s restored with mode %v and time %d instead of %v and %d", relative, info.Mode().Perm(),
				info.ModTime().Unix(), want.mode, want.mtime)
		}
	}
	if info, err := os.Lstat(filepath.Join(top, "dir03")); err != nil || info.ModTime().Unix() != directoryTime.Unix() {
		t.Errorf("dir03 restored with time %v, %v", info.ModTime(), err)
	}
}
