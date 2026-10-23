// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
)

// The golden fixtures are small storages checked in under testdata/fixtures.  They were written by this code
// base when the fixtures were generated and must stay restorable byte for byte by every future version, which
// guards the on-disk formats (config, chunks, snapshots) against accidental changes.  Each fixture holds two
// revisions of the same repository content; testdata/fixtures/manifest-r<N>.txt lists what each revision must
// restore to.
//
// To regenerate the fixtures (only when a format change is intended and reviewed):
//
//	DUPLICACY_GENERATE_FIXTURES=1 go test -run TestGenerateFixtures .

const (
	fixtureDir        = "testdata/fixtures"
	fixtureSnapshotID = "fixture"
	fixturePassword   = "fixture-password"
	fixtureRevisions  = 2
)

type fixtureVariant struct {
	name             string
	encrypted        bool
	compressionLevel int
	averageChunkSize int
	maximumChunkSize int
	minimumChunkSize int
	rsa              bool
	dataShards       int
	parityShards     int
	separateMetadata bool
	legacyConfig     bool // the config file uses the old format with a static salt and fixed iterations
}

var fixtureVariants = []fixtureVariant{
	{name: "plain-lz4", compressionLevel: DEFAULT_COMPRESSION_LEVEL},
	{name: "encrypted-lz4", encrypted: true, compressionLevel: DEFAULT_COMPRESSION_LEVEL},
	{name: "encrypted-zstd", encrypted: true, compressionLevel: ZSTD_COMPRESSION_LEVEL_DEFAULT},
	{name: "encrypted-zlib-legacy", encrypted: true, compressionLevel: 6},
	{name: "encrypted-rsa", encrypted: true, compressionLevel: DEFAULT_COMPRESSION_LEVEL, rsa: true},
	{name: "encrypted-erasure-coding", encrypted: true, compressionLevel: DEFAULT_COMPRESSION_LEVEL, dataShards: 5, parityShards: 2},
	{name: "encrypted-separate-metadata", encrypted: true, compressionLevel: DEFAULT_COMPRESSION_LEVEL, separateMetadata: true},
	{name: "encrypted-fixed-chunk-size", encrypted: true, compressionLevel: DEFAULT_COMPRESSION_LEVEL,
		averageChunkSize: 8192, maximumChunkSize: 8192, minimumChunkSize: 8192},
	{name: "legacy-config", encrypted: true, compressionLevel: DEFAULT_COMPRESSION_LEVEL, legacyConfig: true},
}

func (variant fixtureVariant) chunkSizes() (average int, maximum int, minimum int) {
	if variant.averageChunkSize != 0 {
		return variant.averageChunkSize, variant.maximumChunkSize, variant.minimumChunkSize
	}
	return 4096, 16384, 1024
}

func (variant fixtureVariant) password() string {
	if variant.encrypted {
		return fixturePassword
	}
	return ""
}

// fixtureBytes returns deterministic pseudo-random bytes so that the fixture repository is reproducible.
func fixtureBytes(size int, seed uint32) []byte {
	data := make([]byte, size)
	x := seed*2654435761 + 1
	for i := range data {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		data[i] = byte(x)
	}
	return data
}

// writeFixtureRepository creates the content of the given revision under 'top'.
func writeFixtureRepository(t *testing.T, top string, revision int) {
	files := map[string][]byte{
		"empty":                  {},
		"dir/random.bin":         fixtureBytes(50000, 1),
		"dir/sub/pattern.bin":    bytes.Repeat([]byte("duplicacy fixture pattern\n"), 800),
		"duplicate1.bin":         fixtureBytes(10000, 2),
		"dir/duplicate2.bin":     fixtureBytes(10000, 2),
		"names/ünïcødé ñame.txt": []byte("non-ASCII file name\n"),
		"names/with space.txt":   []byte("file name with a space\n"),
	}
	if revision == 1 {
		files["deleted-in-r2.txt"] = []byte("only in the first revision\n")
	} else {
		// Change the middle of a multi-chunk file so that some chunks are reused and some are new
		modified := fixtureBytes(50000, 1)
		copy(modified[20000:], fixtureBytes(3000, 3))
		files["dir/random.bin"] = modified
		files["added-in-r2.bin"] = fixtureBytes(7000, 4)
	}

	for name, content := range files {
		fullPath := filepath.Join(top, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if revision == 2 {
		os.Remove(filepath.Join(top, "deleted-in-r2.txt"))
	}
}

// fixtureManifest lists every directory and file under 'top' (excluding .duplicacy) as '<path> <size> <sha256>'.
func fixtureManifest(t *testing.T, top string) string {
	var lines []string
	err := filepath.Walk(top, func(fullPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(top, fullPath)
		relative = filepath.ToSlash(relative)
		if relative == "." {
			return nil
		}
		if relative == DUPLICACY_DIRECTORY || strings.HasPrefix(relative, DUPLICACY_DIRECTORY+"/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			lines = append(lines, relative+"/")
			return nil
		}
		file, err := os.Open(fullPath)
		if err != nil {
			return err
		}
		defer file.Close()
		hasher := sha256.New()
		size, err := io.Copy(hasher, file)
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%s %d %s", relative, size, hex.EncodeToString(hasher.Sum(nil))))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

func copyDirectory(t *testing.T, from string, to string) {
	err := filepath.Walk(from, func(fullPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(from, fullPath)
		target := filepath.Join(to, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// catchFixtureException turns a LOG_ERROR raised during a fixture operation into a test failure.
func catchFixtureException(t *testing.T) {
	if r := recover(); r != nil {
		if e, ok := r.(Exception); ok {
			t.Errorf("%s %s", e.LogID, e.Message)
		} else {
			t.Errorf("%v", r)
		}
		debug.PrintStack()
	}
}

func TestGoldenFixtures(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(WARN)

	privateKey, err := os.ReadFile(filepath.Join(fixtureDir, "rsa-test-key.pem"))
	if err != nil {
		t.Fatalf("Failed to read the RSA test key: %v", err)
	}

	for _, variant := range fixtureVariants {
		variant := variant
		t.Run(variant.name, func(t *testing.T) {
			setTestingT(t)
			defer catchFixtureException(t)

			testDir := t.TempDir()
			storageDir := filepath.Join(testDir, "storage")
			copyDirectory(t, filepath.Join(fixtureDir, variant.name, "storage"), storageDir)

			for revision := 1; revision <= fixtureRevisions; revision++ {
				expected, err := os.ReadFile(filepath.Join(fixtureDir, fmt.Sprintf("manifest-r%d.txt", revision)))
				if err != nil {
					t.Fatal(err)
				}

				restoreDir := filepath.Join(testDir, fmt.Sprintf("restore-r%d", revision))
				os.MkdirAll(filepath.Join(restoreDir, DUPLICACY_DIRECTORY), 0700)
				SetDuplicacyPreferencePath(filepath.Join(restoreDir, DUPLICACY_DIRECTORY))

				storage, err := CreateFileStorage(storageDir, false, 1)
				if err != nil {
					t.Fatal(err)
				}
				manager := CreateBackupManager(fixtureSnapshotID, storage, restoreDir, variant.password(), "", "", false)
				if manager == nil {
					t.Fatal("Failed to create the backup manager")
				}
				if variant.rsa {
					manager.LoadRSAPrivateKey(string(privateKey), "")
				}
				manager.SetupSnapshotCache("default")

				failed := manager.Restore(restoreDir, revision, true, false, 1, true, false, false, false, nil, false)
				if failed != 0 {
					t.Fatalf("Revision %d: %d files failed to restore", revision, failed)
				}
				if actual := fixtureManifest(t, restoreDir); actual != string(expected) {
					t.Errorf("Revision %d restored to\n%s\ninstead of\n%s", revision, actual, expected)
				}
			}
		})
	}
}

// writeLegacyConfig rewrites the config file in the old format (banner 'duplicacy\000', static salt, 16384
// iterations), which is still supported for storages initialized by early versions.
func writeLegacyConfig(t *testing.T, storage Storage, password string) {
	config, _, err := DownloadConfig(storage, password)
	if err != nil || config == nil {
		t.Fatalf("Failed to download the config: %v", err)
	}
	description, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	chunk := CreateChunk(CreateConfig(), true)
	chunk.Write(description)
	masterKey := GenerateKeyFromPassword(password, DEFAULT_KEY, CONFIG_LEGACY_ITERATIONS)
	if err = chunk.Encrypt(masterKey, "", true); err != nil {
		t.Fatal(err)
	}
	if err = storage.UploadFile(0, "config", chunk.GetBytes()); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateFixtures(t *testing.T) {
	if os.Getenv("DUPLICACY_GENERATE_FIXTURES") == "" {
		t.Skip("set DUPLICACY_GENERATE_FIXTURES=1 to regenerate the golden fixtures")
	}
	setTestingT(t)
	SetLoggingLevel(WARN)
	defer catchFixtureException(t)

	if err := os.MkdirAll(fixtureDir, 0755); err != nil {
		t.Fatal(err)
	}

	// A test-only RSA key; it protects nothing but the fixture
	keyPath := filepath.Join(fixtureDir, "rsa-test-key.pem")
	privateKeyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		privateKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
		if err = os.WriteFile(keyPath, privateKeyPEM, 0644); err != nil {
			t.Fatal(err)
		}
	}
	block, _ := pem.Decode(privateKeyPEM)
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyBytes, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	publicKeyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicKeyBytes}))

	workDir := t.TempDir()
	repository := filepath.Join(workDir, "repository")

	for revision := 1; revision <= fixtureRevisions; revision++ {
		os.RemoveAll(repository)
		writeFixtureRepository(t, repository, revision)
		manifest := fixtureManifest(t, repository)
		if err := os.WriteFile(filepath.Join(fixtureDir, fmt.Sprintf("manifest-r%d.txt", revision)), []byte(manifest), 0644); err != nil {
			t.Fatal(err)
		}
	}

	for _, variant := range fixtureVariants {
		storageDir := filepath.Join(fixtureDir, variant.name, "storage")
		os.RemoveAll(filepath.Join(fixtureDir, variant.name))
		if err := os.MkdirAll(storageDir, 0755); err != nil {
			t.Fatal(err)
		}

		storage, err := CreateFileStorage(storageDir, false, 1)
		if err != nil {
			t.Fatal(err)
		}
		average, maximum, minimum := variant.chunkSizes()
		keyFile := ""
		if variant.rsa {
			keyFile = publicKeyPEM
		}
		if !ConfigStorage(storage, CONFIG_LEGACY_ITERATIONS, variant.compressionLevel, average, maximum, minimum,
			variant.password(), nil, false, keyFile, variant.dataShards, variant.parityShards) {
			t.Fatalf("Failed to initialize the storage for %s", variant.name)
		}
		if variant.legacyConfig {
			writeLegacyConfig(t, storage, variant.password())
		}
		if variant.separateMetadata {
			err = storage.UploadFile(0, "nesting", []byte(`{"read-levels":[1],"write-level":1,"separate-metadata":true}`))
			if err != nil {
				t.Fatal(err)
			}
		}

		os.RemoveAll(repository)
		for revision := 1; revision <= fixtureRevisions; revision++ {
			writeFixtureRepository(t, repository, revision)
			os.MkdirAll(filepath.Join(repository, DUPLICACY_DIRECTORY), 0700)
			SetDuplicacyPreferencePath(filepath.Join(repository, DUPLICACY_DIRECTORY))

			storage, err := CreateFileStorage(storageDir, false, 1)
			if err != nil {
				t.Fatal(err)
			}
			manager := CreateBackupManager(fixtureSnapshotID, storage, repository, variant.password(), "", "", false)
			manager.SetupSnapshotCache("default")
			if !manager.Backup(repository, false, 1, fmt.Sprintf("r%d", revision), false, false, 0, false, 1024, 1024) {
				t.Fatalf("Backup of %s revision %d failed", variant.name, revision)
			}
		}
		os.RemoveAll(filepath.Join(repository, DUPLICACY_DIRECTORY))
	}
}
