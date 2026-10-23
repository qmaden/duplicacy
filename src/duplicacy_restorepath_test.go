// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestorePathValidation(t *testing.T) {
	for _, valid := range []string{"a", "a/b", "dir/", "dir/sub/", "..hidden", "a..b", "a/.b/c", "ünïcødé/ñ.txt", "a b"} {
		if err := validateEntryPath(valid); err != nil {
			t.Errorf("Valid path %q was rejected: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "/", "../x", "a/../../x", "a/..", "/etc/passwd", "a//b", "./a", "a/./b",
		"a\x00b", `C:\Windows`, "c:/x", `\\server\share`} {
		if err := validateEntryPath(invalid); err == nil {
			t.Errorf("Invalid path %q was accepted", invalid)
		}
	}
}

func TestRestorePathGuard(t *testing.T) {
	top := t.TempDir()
	outside := t.TempDir()
	os.MkdirAll(filepath.Join(top, "real", "sub"), 0700)
	os.WriteFile(filepath.Join(top, "file"), []byte("x"), 0600)
	if err := os.Symlink(outside, filepath.Join(top, "link")); err != nil {
		t.Fatal(err)
	}

	guard := newRestorePathGuard(top)
	for _, ok := range []string{"real/sub/x", "real/new/deeper/x", "new/x", "link", "file"} {
		if _, err := guard.path(ok); err != nil {
			t.Errorf("%s was rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"link/x", "link/sub/", "file/x", "../x"} {
		if _, err := guard.path(bad); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}

	// A directory verified earlier and then replaced by a symbolic link must be checked again
	if _, err := guard.path("real/sub/x"); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(top, "real", "sub"))
	os.Symlink(outside, filepath.Join(top, "real", "sub"))
	guard.invalidate("real/sub")
	if _, err := guard.path("real/sub/x"); err == nil {
		t.Error("A path through a directory replaced by a symbolic link was accepted")
	}
}

// backupForRestoreTest backs up 'repository' to a new local storage and returns the storage directory.
func backupForRestoreTest(t *testing.T, repository string) string {
	storageDir := filepath.Join(t.TempDir(), "storage")
	storage, err := CreateFileStorage(storageDir, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !ConfigStorage(storage, CONFIG_DEFAULT_ITERATIONS, DEFAULT_COMPRESSION_LEVEL, 4096, 16384, 1024, "", nil, false, "", 0, 0) {
		t.Fatal("Failed to initialize the storage")
	}
	os.MkdirAll(filepath.Join(repository, DUPLICACY_DIRECTORY), 0700)
	SetDuplicacyPreferencePath(filepath.Join(repository, DUPLICACY_DIRECTORY))
	manager := CreateBackupManager("test", storage, repository, "", "", "", false)
	manager.SetupSnapshotCache("default")
	if !manager.Backup(repository, false, 1, "", false, false, 0, false, 1024, 1024) {
		t.Fatal("Backup failed")
	}
	return storageDir
}

// restoreForTest restores revision 1 in place into 'top' and returns the number of failed entries.
func restoreForTest(t *testing.T, storageDir string, top string, overwrite bool, setOwner bool) int {
	os.MkdirAll(filepath.Join(top, DUPLICACY_DIRECTORY), 0700)
	SetDuplicacyPreferencePath(filepath.Join(top, DUPLICACY_DIRECTORY))
	storage, err := CreateFileStorage(storageDir, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	manager := CreateBackupManager("test", storage, top, "", "", "", false)
	manager.SetupSnapshotCache("default")
	return manager.Restore(top, 1, true, false, 1, overwrite, false, setOwner, false, nil, true)
}

func assertFileContent(t *testing.T, path string, expected string) {
	content, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("Failed to read %s: %v", path, err)
	} else if string(content) != expected {
		t.Errorf("%s contains %q instead of %q", path, content, expected)
	}
}

// S1 / upstream PR #691: a symbolic link in the restore directory where the snapshot has a directory must not
// be followed.
func TestRestoreDoesNotFollowSymlinkedDirectory(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	repository := t.TempDir()
	os.MkdirAll(filepath.Join(repository, "d"), 0700)
	os.WriteFile(filepath.Join(repository, "d", "f"), []byte("restored content"), 0600)
	storageDir := backupForRestoreTest(t, repository)

	top := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(top, "d")); err != nil {
		t.Fatal(err)
	}

	if failed := restoreForTest(t, storageDir, top, false, true); failed == 0 {
		t.Error("Restoring through a symbolic link without -overwrite reported no failures")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("Restore wrote %d entries through the symbolic link", len(entries))
	}

	if failed := restoreForTest(t, storageDir, top, true, true); failed != 0 {
		t.Errorf("Restore with -overwrite reported %d failures", failed)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("Restore with -overwrite wrote %d entries through the symbolic link", len(entries))
	}
	if isSymlink(filepath.Join(top, "d")) {
		t.Error("The symbolic link was not replaced by a directory")
	}
	assertFileContent(t, filepath.Join(top, "d", "f"), "restored content")
}

// S1 / upstream PR #691: a symbolic link where the snapshot has a file must not be written through.
func TestRestoreDoesNotWriteThroughSymlinkedFile(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	repository := t.TempDir()
	os.WriteFile(filepath.Join(repository, "f"), []byte("restored content"), 0600)
	os.WriteFile(filepath.Join(repository, "empty"), nil, 0600)
	storageDir := backupForRestoreTest(t, repository)

	top := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret")
	os.WriteFile(secret, []byte("must not change"), 0600)
	if err := os.Symlink(secret, filepath.Join(top, "f")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(top, "empty")); err != nil {
		t.Fatal(err)
	}

	if failed := restoreForTest(t, storageDir, top, false, true); failed == 0 {
		t.Error("Restoring over symbolic links without -overwrite reported no failures")
	}
	assertFileContent(t, secret, "must not change")

	if failed := restoreForTest(t, storageDir, top, true, true); failed != 0 {
		t.Errorf("Restore with -overwrite reported %d failures", failed)
	}
	assertFileContent(t, secret, "must not change")
	if isSymlink(filepath.Join(top, "f")) || isSymlink(filepath.Join(top, "empty")) {
		t.Error("The symbolic links were not replaced by files")
	}
	assertFileContent(t, filepath.Join(top, "f"), "restored content")
	assertFileContent(t, filepath.Join(top, "empty"), "")
}

func TestRestoreSetuidOnlyWithOwner(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)

	repository := t.TempDir()
	program := filepath.Join(repository, "program")
	os.WriteFile(program, []byte("#!/bin/sh\n"), 0755)
	if err := os.Chmod(program, 0755|os.ModeSetuid); err != nil {
		t.Skipf("Can't set the setuid bit: %v", err)
	}
	if info, _ := os.Stat(program); info.Mode()&os.ModeSetuid == 0 {
		t.Skip("The file system doesn't keep the setuid bit")
	}
	storageDir := backupForRestoreTest(t, repository)

	withoutOwner := t.TempDir()
	restoreForTest(t, storageDir, withoutOwner, true, false)
	if info, err := os.Stat(filepath.Join(withoutOwner, "program")); err != nil || info.Mode()&os.ModeSetuid != 0 {
		t.Errorf("The setuid bit was restored without the owner (mode %v, error %v)", info.Mode(), err)
	}

	withOwner := t.TempDir()
	restoreForTest(t, storageDir, withOwner, true, true)
	if info, err := os.Stat(filepath.Join(withOwner, "program")); err != nil || info.Mode()&os.ModeSetuid == 0 {
		t.Errorf("The setuid bit was not restored with the owner (mode %v, error %v)", info.Mode(), err)
	}
}
