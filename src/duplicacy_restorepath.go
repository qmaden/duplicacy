// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"fmt"
	"os"
	"strings"
)

// Entry paths come from the snapshot, i.e., from the storage, and must not be trusted: a crafted path such as
// '../../.ssh/authorized_keys' or a path through a symbolic link must never make restore write outside the
// directory being restored to.

// validateEntryPath checks that an entry path from a snapshot is a clean relative path.  Directory paths end
// with '/'.
func validateEntryPath(entryPath string) error {
	trimmed := strings.TrimSuffix(entryPath, "/")
	if trimmed == "" {
		return fmt.Errorf("empty path")
	}
	if strings.ContainsRune(trimmed, 0) {
		return fmt.Errorf("path contains a NUL character")
	}
	if strings.HasPrefix(trimmed, "/") {
		return fmt.Errorf("absolute path")
	}
	if isWindowsAbsolutePath(trimmed) {
		return fmt.Errorf("absolute path")
	}
	for _, component := range strings.Split(trimmed, "/") {
		switch component {
		case "":
			return fmt.Errorf("empty path component")
		case ".", "..":
			return fmt.Errorf("path component '%s'", component)
		}
	}
	return nil
}

// isWindowsAbsolutePath detects drive-letter and UNC forms, which must be rejected on every platform since a
// snapshot may be restored anywhere.
func isWindowsAbsolutePath(entryPath string) bool {
	if strings.HasPrefix(entryPath, `\\`) {
		return true
	}
	return len(entryPath) >= 2 && entryPath[1] == ':' &&
		((entryPath[0] >= 'a' && entryPath[0] <= 'z') || (entryPath[0] >= 'A' && entryPath[0] <= 'Z'))
}

// restorePathGuard maps snapshot entry paths to locations under the restore directory, refusing paths that would
// leave it.  Directories already verified not to be symbolic links are cached; the cache is invalidated
// whenever restore creates a symbolic link or removes something.
type restorePathGuard struct {
	top      string
	verified map[string]bool // relative directory paths (without trailing '/') known to be real directories
}

func newRestorePathGuard(top string) *restorePathGuard {
	return &restorePathGuard{top: top, verified: make(map[string]bool)}
}

// path validates 'entryPath' and returns the full path to restore it to.  Every existing ancestor directory
// between the restore directory and the entry must be a real directory, not a symbolic link.  The entry itself
// may be anything; callers must not follow it if it is a symbolic link.
func (guard *restorePathGuard) path(entryPath string) (string, error) {
	if err := validateEntryPath(entryPath); err != nil {
		return "", err
	}

	components := strings.Split(strings.TrimSuffix(entryPath, "/"), "/")
	parent := ""
	for _, component := range components[:len(components)-1] {
		if parent == "" {
			parent = component
		} else {
			parent += "/" + component
		}
		if guard.verified[parent] {
			continue
		}

		info, err := os.Lstat(joinPath(guard.top, parent))
		if os.IsNotExist(err) {
			// Neither this directory nor anything below it exists yet; restore will create real directories
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("'%s' is a symbolic link", parent)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("'%s' is not a directory", parent)
		}
		guard.verified[parent] = true
	}

	return joinPath(guard.top, entryPath), nil
}

// invalidate forgets what is known about 'entryPath' and everything below it.
func (guard *restorePathGuard) invalidate(entryPath string) {
	prefix := strings.TrimSuffix(entryPath, "/")
	for verified := range guard.verified {
		if verified == prefix || strings.HasPrefix(verified, prefix+"/") {
			delete(guard.verified, verified)
		}
	}
}

// isSymlink returns true if 'fullPath' exists and is a symbolic link (which is not followed).
func isSymlink(fullPath string) bool {
	info, err := os.Lstat(fullPath)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}
