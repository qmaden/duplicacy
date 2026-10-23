// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"net/http/httptest"
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
	exerciseCloudStorage(t, storage, 1003)
}
