// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func newConfigTestStorage(t *testing.T) Storage {
	storage, err := CreateFileStorage(t.TempDir(), false, 1)
	if err != nil {
		t.Fatal(err)
	}
	return storage
}

// S4: the iteration count stored in a config file must not be able to stall the client.
func TestDownloadConfigRejectsExcessiveIterations(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	storage := newConfigTestStorage(t)
	config := CreateConfigFromParameters(DEFAULT_COMPRESSION_LEVEL, 4096, 16384, 1024, true, nil, false)
	if err := SaveConfig(storage, config, "a long test password", 1000); err != nil {
		t.Fatal(err)
	}

	chunk := CreateChunk(CreateConfig(), true)
	if err := storage.DownloadFile(0, "config", chunk); err != nil {
		t.Fatal(err)
	}
	content := append([]byte(nil), chunk.GetBytes()...)
	binary.LittleEndian.PutUint32(content[len(CONFIG_BANNER)+CONFIG_SALT_LENGTH:], 1<<31)
	if err := storage.UploadFile(0, "config", content); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, _, err := DownloadConfig(storage, "a long test password")
	if err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Errorf("A config with 2^31 iterations was not rejected: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("The excessive iteration count was used before being rejected")
	}
}

func TestSaveConfigReturnsErrors(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	storage := newConfigTestStorage(t)
	config := CreateConfigFromParameters(DEFAULT_COMPRESSION_LEVEL, 4096, 16384, 1024, true, nil, false)

	if err := SaveConfig(storage, config, "short", CONFIG_DEFAULT_ITERATIONS); err == nil {
		t.Error("A password shorter than 8 characters was accepted")
	}
	if err := SaveConfig(storage, config, "a long test password", 0); err == nil {
		t.Error("Zero iterations were accepted")
	}
	if err := SaveConfig(storage, config, "a long test password", CONFIG_MAXIMUM_ITERATIONS+1); err == nil {
		t.Error("Too many iterations were accepted")
	}
	if exist, _, _, _ := storage.GetFileInfo(0, "config"); exist {
		t.Error("A config was written although every attempt failed")
	}

	if err := SaveConfig(storage, config, "a long test password", 1000); err != nil {
		t.Fatal(err)
	}
	downloaded, _, err := DownloadConfig(storage, "a long test password")
	if err != nil || downloaded == nil || string(downloaded.ChunkKey) != string(config.ChunkKey) {
		t.Errorf("The saved config could not be read back: %v", err)
	}
}

func TestDefaultIterationCost(t *testing.T) {
	start := time.Now()
	GenerateKeyFromPassword("a long test password", make([]byte, CONFIG_SALT_LENGTH), CONFIG_DEFAULT_ITERATIONS)
	t.Logf("Deriving a key with %d iterations took %v", CONFIG_DEFAULT_ITERATIONS, time.Since(start))
	if CONFIG_LEGACY_ITERATIONS != 16384 {
		t.Error("The iteration count of the old config format must not change")
	}
}
