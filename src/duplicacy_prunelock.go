// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Two prunes running against the same storage at the same time can delete chunks the other one still needs
// (each keeps its own fossil collections).  prune.lock in the storage root marks a prune in progress.  The lock
// is advisory: storages offer no atomic create-if-absent, so two prunes starting at the same moment may both
// proceed, but the common case of a second prune started while one is running is caught.  Earlier versions
// ignore the file.

const pruneLockFile = "prune.lock"

// A lock older than this is considered left behind by a prune that was interrupted
var pruneLockTimeout = int64(24 * 3600)

type pruneLock struct {
	Host       string `json:"host"`
	PID        int    `json:"pid"`
	SnapshotID string `json:"snapshot_id"`
	Time       int64  `json:"time"`
	Token      string `json:"token"`
}

func readPruneLock(storage Storage) (*pruneLock, error) {
	exist, _, _, err := storage.GetFileInfo(0, pruneLockFile)
	if err != nil || !exist {
		return nil, err
	}
	chunk := CreateChunk(CreateConfig(), true)
	if err = storage.DownloadFile(0, pruneLockFile, chunk); err != nil {
		return nil, err
	}
	var lock pruneLock
	if err = json.Unmarshal(chunk.GetBytes(), &lock); err != nil {
		// An unreadable lock is treated as stale rather than blocking prune forever
		return &pruneLock{}, nil
	}
	return &lock, nil
}

// acquirePruneLock creates prune.lock unless another recent prune holds it.  It returns a function that removes
// the lock.
func acquirePruneLock(storage Storage, snapshotID string, ignoreLock bool) (release func(), err error) {
	existing, err := readPruneLock(storage)
	if err != nil {
		return nil, fmt.Errorf("Failed to check for a running prune: %v", err)
	}
	if existing != nil {
		age := time.Now().Unix() - existing.Time
		description := fmt.Sprintf("started on %s (pid %d, snapshot id %s) at %s", existing.Host, existing.PID,
			existing.SnapshotID, time.Unix(existing.Time, 0).Format("2006-01-02 15:04:05"))
		switch {
		case ignoreLock:
			LOG_WARN("PRUNE_LOCK", "Ignoring the lock of another prune %s", description)
		case age < pruneLockTimeout:
			return nil, fmt.Errorf("Another prune %s may still be running.  If it isn't, delete %s from the storage "+
				"or run prune with -ignore-lock", description, pruneLockFile)
		default:
			LOG_WARN("PRUNE_LOCK", "Taking over the lock of a prune %s, which is older than %d hours", description,
				pruneLockTimeout/3600)
		}
	}

	host, _ := os.Hostname()
	token := make([]byte, 16)
	rand.Read(token)
	lock := pruneLock{Host: host, PID: os.Getpid(), SnapshotID: snapshotID, Time: time.Now().Unix(),
		Token: hex.EncodeToString(token)}
	description, _ := json.Marshal(lock)
	if err = storage.UploadFile(0, pruneLockFile, description); err != nil {
		return nil, fmt.Errorf("Failed to create %s: %v", pruneLockFile, err)
	}

	// If another prune wrote its lock at the same time, only the last writer proceeds
	if current, err := readPruneLock(storage); err == nil && current != nil && current.Token != lock.Token && !ignoreLock {
		return nil, fmt.Errorf("Another prune started on %s at the same time", current.Host)
	}

	return func() {
		if current, err := readPruneLock(storage); err == nil && current != nil && current.Token == lock.Token {
			if err = storage.DeleteFile(0, pruneLockFile); err != nil {
				LOG_WARN("PRUNE_LOCK", "Failed to delete %s: %v", pruneLockFile, err)
			}
		}
	}, nil
}
