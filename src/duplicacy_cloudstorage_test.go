// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// exerciseCloudStorage checks every storage operation of a backend against a fake of its service: missing files,
// upload and download, listing 'count' chunks and as many snapshot ids (enough to need several pages where the
// fake pages results), fossil renames and deletes.
func exerciseCloudStorage(t *testing.T, storage Storage, count int) {
	var err error
	// Missing files are reported as not existing, not as errors
	exist, _, _, err := storage.GetFileInfo(0, "config")
	if err != nil || exist {
		t.Fatalf("GetFileInfo on a missing file: exist %t, err %v", exist, err)
	}

	content := []byte("config content")
	if err = storage.UploadFile(0, "config", content); err != nil {
		t.Fatal(err)
	}
	exist, isDir, size, err := storage.GetFileInfo(0, "config")
	if err != nil || !exist || isDir || size != int64(len(content)) {
		t.Fatalf("GetFileInfo: exist %t, isDir %t, size %d, err %v", exist, isDir, size, err)
	}
	chunk := CreateChunk(CreateConfig(), true)
	if err = storage.DownloadFile(0, "config", chunk); err != nil || !bytes.Equal(chunk.GetBytes(), content) {
		t.Fatalf("DownloadFile returned %q, %v", chunk.GetBytes(), err)
	}

	var expectedChunks []string
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("%02x/%062x", i%256, i)
		expectedChunks = append(expectedChunks, name)
		if err = storage.UploadFile(0, "chunks/"+name, []byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
	}
	var expectedIDs []string
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("host%04d", i)
		expectedIDs = append(expectedIDs, id+"/")
		if err = storage.UploadFile(0, "snapshots/"+id+"/1", []byte("snapshot")); err != nil {
			t.Fatal(err)
		}
	}

	files, sizes, err := storage.ListFiles(0, "chunks")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	sort.Strings(expectedChunks)
	if strings.Join(files, ",") != strings.Join(expectedChunks, ",") || len(sizes) != len(files) {
		t.Fatalf("listed %d chunks and %d sizes, expected %d", len(files), len(sizes), len(expectedChunks))
	}
	for _, size := range sizes {
		if size != 1 {
			t.Fatalf("chunk listed with size %d", size)
		}
	}

	ids, _, err := storage.ListFiles(0, "snapshots/")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(ids)
	if strings.Join(ids, ",") != strings.Join(expectedIDs, ",") {
		t.Fatalf("listed %d snapshot ids, expected %d", len(ids), len(expectedIDs))
	}

	revisions, _, err := storage.ListFiles(0, "snapshots/host0001")
	if err != nil || len(revisions) != 1 || revisions[0] != "1" {
		t.Fatalf("listed revisions %v, %v", revisions, err)
	}

	// Fossilizing renames a chunk
	if err = storage.MoveFile(0, "chunks/"+expectedChunks[0], "chunks/"+expectedChunks[0]+".fsl"); err != nil {
		t.Fatal(err)
	}
	exist, _, _, err = storage.GetFileInfo(0, "chunks/"+expectedChunks[0])
	if err != nil || exist {
		t.Fatalf("moved chunk still exists: %t, %v", exist, err)
	}
	exist, _, _, err = storage.GetFileInfo(0, "chunks/"+expectedChunks[0]+".fsl")
	if err != nil || !exist {
		t.Fatalf("fossil doesn't exist: %t, %v", exist, err)
	}

	if err = storage.DeleteFile(0, "chunks/"+expectedChunks[0]+".fsl"); err != nil {
		t.Fatal(err)
	}
	exist, _, _, err = storage.GetFileInfo(0, "chunks/"+expectedChunks[0]+".fsl")
	if err != nil || exist {
		t.Fatalf("deleted fossil still exists: %t, %v", exist, err)
	}
}

var fakeS3Endpoint, fakeGCSServer = "", (*fakestorage.Server)(nil)
var fakeDriveServer *httptest.Server
var fakeAzuriteContainer string
var fakeCloudLock sync.Mutex

// loadFakeCloudStorage returns a storage on an in-process fake for '-storage fake-s3', 'fake-gcs', 'fake-gcd' or 'azurite', so
// that the backup, restore, prune and copy tests can run against those backends without credentials.  It returns
// nil for other storage names.
func loadFakeCloudStorage(threads int) (Storage, error) {
	fakeCloudLock.Lock()
	defer fakeCloudLock.Unlock()

	switch *testStorageName {
	case "fake-s3":
		if fakeS3Endpoint == "" {
			backend := s3mem.New()
			if err := backend.CreateBucket("bucket"); err != nil {
				return nil, err
			}
			fakeS3Endpoint = strings.TrimPrefix(httptest.NewServer(gofakes3.New(backend).Server()).URL, "http://")
		}
		storage, err := CreateS3Storage("", fakeS3Endpoint, "bucket", "storage", "id", "secret", threads, false, true)
		if err != nil {
			return nil, err
		}
		storage.SetDefaultNestingLevels([]int{2, 3}, 2)
		return storage, nil
	case "fake-gcs":
		if fakeGCSServer == nil {
			fakeGCSServer = fakestorage.NewServer(nil)
			fakeGCSServer.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: "bucket"})
		}
		storage, err := createGCSStorage("bucket", "storage", threads, option.WithEndpoint(fakeGCSServer.URL()+"/storage/v1/"),
			option.WithHTTPClient(fakeGCSServer.HTTPClient()))
		if err != nil {
			return nil, err
		}
		storage.SetDefaultNestingLevels([]int{2, 3}, 2)
		return storage, nil
	case "fake-gcd":
		if fakeDriveServer == nil {
			fakeDriveServer = newFakeDrive()
		}
		storage, err := createGCDStorage("", "storage", threads, false, drive.DriveScope,
			option.WithEndpoint(fakeDriveServer.URL+"/drive/v3/"), option.WithHTTPClient(fakeDriveServer.Client()))
		if err != nil {
			return nil, err
		}
		storage.SetDefaultNestingLevels([]int{2, 3}, 2)
		return storage, nil
	case "azurite":
		serviceURL, account, key := azuriteSettings()
		if serviceURL == "" {
			return nil, fmt.Errorf("DUPLICACY_TEST_AZURITE_URL is not set")
		}
		if fakeAzuriteContainer == "" {
			name, err := createAzuriteContainer()
			if err != nil {
				return nil, err
			}
			fakeAzuriteContainer = name
		}
		storage, err := createAzureStorage(serviceURL, account, key, fakeAzuriteContainer, threads)
		if err != nil {
			return nil, err
		}
		storage.SetDefaultNestingLevels([]int{2, 3}, 2)
		return storage, nil
	}
	return nil, nil
}
