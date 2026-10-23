// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"google.golang.org/api/option"
)

// TestGCSStorage exercises the Google Cloud Storage backend against an in-process fake-gcs-server.
func TestGCSStorage(t *testing.T) {
	setTestingT(t)
	server := fakestorage.NewServer(nil)
	t.Cleanup(server.Stop)
	server.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: "bucket"})

	storage, err := createGCSStorage("bucket", "dir/sub", 4, option.WithEndpoint(server.URL()+"/storage/v1/"),
		option.WithHTTPClient(server.HTTPClient()))
	if err != nil {
		t.Fatal(err)
	}
	exerciseCloudStorage(t, storage, 1003)

	// A failed upload must be reported (earlier versions ignored the error returned when the upload completed)
	missing, err := createGCSStorage("missing-bucket", "", 1, option.WithEndpoint(server.URL()+"/storage/v1/"),
		option.WithHTTPClient(server.HTTPClient()))
	if err != nil {
		t.Fatal(err)
	}
	if err = missing.UploadFile(0, "config", []byte("content")); err == nil {
		t.Errorf("Uploading to a missing bucket succeeded")
	}
}
