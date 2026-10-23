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
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

// startFakeS3 runs an in-process S3 server with one bucket and returns its host:port.
func startFakeS3(t *testing.T, bucket string) string {
	backend := s3mem.New()
	if err := backend.CreateBucket(bucket); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(gofakes3.New(backend).Server())
	t.Cleanup(server.Close)
	return strings.TrimPrefix(server.URL, "http://")
}

// TestS3Storage exercises every storage operation of the S3 backend against an in-process S3 server, including
// listings longer than one page.
func TestS3Storage(t *testing.T) {
	setTestingT(t)
	endpoint := startFakeS3(t, "bucket")

	storage, err := CreateS3Storage("", endpoint, "bucket", "dir/sub", "id", "secret", 4, false, true)
	if err != nil {
		t.Fatal(err)
	}

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

	// More than one page of chunks (pages hold 1,000 keys) and of snapshot ids
	var expectedChunks []string
	for i := 0; i < 1003; i++ {
		name := fmt.Sprintf("%02x/%062x", i%256, i)
		expectedChunks = append(expectedChunks, name)
		if err = storage.UploadFile(0, "chunks/"+name, []byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
	}
	var expectedIDs []string
	for i := 0; i < 1002; i++ {
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
