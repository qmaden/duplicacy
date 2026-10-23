// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	gcs "google.golang.org/api/storage/v1"
)

// The backend uses the generated client for the Cloud Storage JSON API.  cloud.google.com/go/storage would add
// gRPC, OpenTelemetry and Cloud Monitoring to the binary (about 19 MB) for the same six HTTP calls.

type GCSStorage struct {
	StorageBase

	service    *gcs.Service
	bucket     string
	storageDir string

	numberOfThreads int
	TestMode        bool
}

type GCSConfig struct {
	ClientID     string          `json:"client_id"`
	ClientSecret string          `json:"client_secret"`
	Endpoint     oauth2.Endpoint `json:"end_point"`
	Token        oauth2.Token    `json:"token"`
}

// CreateGCSStorage creates a GCD storage object.
func CreateGCSStorage(tokenFile string, bucketName string, storageDir string, threads int) (storage *GCSStorage, err error) {

	ctx := context.Background()

	description, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, err
	}

	var object map[string]interface{}

	err = json.Unmarshal(description, &object)
	if err != nil {
		return nil, err
	}

	isServiceAccount := false
	if value, ok := object["type"]; ok {
		if authType, ok := value.(string); ok && authType == "service_account" {
			isServiceAccount = true
		}
	}

	var tokenSource oauth2.TokenSource

	if isServiceAccount {
		config, err := google.JWTConfigFromJSON(description, gcs.DevstorageReadWriteScope)
		if err != nil {
			return nil, err
		}
		tokenSource = config.TokenSource(ctx)
	} else {
		gcsConfig := &GCSConfig{}
		if err := json.Unmarshal(description, gcsConfig); err != nil {
			return nil, err
		}

		config := oauth2.Config{
			ClientID:     gcsConfig.ClientID,
			ClientSecret: gcsConfig.ClientSecret,
			Endpoint:     gcsConfig.Endpoint,
		}
		tokenSource = config.TokenSource(ctx, &gcsConfig.Token)
	}

	httpClient := newHTTPClient(threads)
	httpClient.Transport = &oauth2.Transport{Source: tokenSource, Base: httpClient.Transport}
	return createGCSStorage(bucketName, storageDir, threads, option.WithHTTPClient(httpClient))
}

// createGCSStorage creates a GCS storage object using a client with the given options.
func createGCSStorage(bucketName string, storageDir string, threads int, options ...option.ClientOption) (storage *GCSStorage, err error) {
	service, err := gcs.NewService(context.Background(), options...)
	if err != nil {
		return nil, err
	}

	if len(storageDir) > 0 && storageDir[len(storageDir)-1] != '/' {
		storageDir += "/"
	}

	storage = &GCSStorage{
		service:         service,
		bucket:          bucketName,
		storageDir:      storageDir,
		numberOfThreads: threads,
	}

	storage.DerivedStorage = storage
	storage.SetDefaultNestingLevels([]int{0}, 0)
	return storage, nil
}

// isGCSNotFound returns true if the error is a 404 response.
func isGCSNotFound(err error) bool {
	e, ok := err.(*googleapi.Error)
	return ok && e.Code == 404
}

// retry calls 'operation' until it succeeds or fails with an error that shouldn't be retried.
func (storage *GCSStorage) retry(operation func() error) error {
	backoff := 1
	for {
		err := operation()
		if retry, e := storage.shouldRetry(&backoff, err); e == nil && !retry {
			return nil
		} else if !retry {
			return e
		}
	}
}

func (storage *GCSStorage) shouldRetry(backoff *int, err error) (bool, error) {

	retry := false
	message := ""
	if err == nil {
		return false, nil
	} else if e, ok := err.(*googleapi.Error); ok {
		if 500 <= e.Code && e.Code < 600 {
			// Retry for 5xx response codes.
			message = fmt.Sprintf("HTTP status code %d", e.Code)
			retry = true
		} else if e.Code == 429 {
			// Too many requests{
			message = "HTTP status code 429"
			retry = true
		} else if e.Code == 403 {
			// User Rate Limit Exceeded
			message = "User Rate Limit Exceeded"
			retry = true
		}
	} else if e, ok := err.(*url.Error); ok {
		message = e.Error()
		retry = true
	} else if err == io.ErrUnexpectedEOF {
		// Retry on unexpected EOFs and temporary network errors.
		message = "Unexpected EOF"
		retry = true
	} else if err, ok := err.(net.Error); ok {
		message = "Temporary network error"
		retry = err.Temporary()
	}

	if !retry || *backoff >= 256 {
		return false, err
	}

	delay := float32(*backoff) * rand.Float32()
	LOG_INFO("GCS_RETRY", "%s; retrying after %.2f seconds", message, delay)
	time.Sleep(time.Duration(float32(*backoff) * float32(time.Second)))
	*backoff *= 2
	return true, nil
}

// ListFiles return the list of files and subdirectories under 'dir' (non-recursively)
func (storage *GCSStorage) ListFiles(threadIndex int, dir string) ([]string, []int64, error) {
	for len(dir) > 0 && dir[len(dir)-1] == '/' {
		dir = dir[:len(dir)-1]
	}

	prefix := storage.storageDir + dir + "/"
	// Only the 'snapshots' directory is listed by subdirectory; everywhere else all files below 'dir' are listed
	dirOnly := dir == "snapshots"

	var files []string
	var sizes []int64
	err := storage.retry(func() error {
		files = []string{}
		sizes = []int64{}
		call := storage.service.Objects.List(storage.bucket).Prefix(prefix).
			Fields("nextPageToken", "prefixes", "items(name,size)")
		if dirOnly {
			call = call.Delimiter("/")
		}
		return call.Pages(context.Background(), func(objects *gcs.Objects) error {
			if dirOnly {
				for _, subDir := range objects.Prefixes {
					files = append(files, subDir[len(prefix):])
				}
				return nil
			}
			for _, object := range objects.Items {
				files = append(files, object.Name[len(prefix):])
				sizes = append(sizes, int64(object.Size))
			}
			return nil
		})
	})
	if err != nil {
		return nil, nil, err
	}

	return files, sizes, nil
}

// DeleteFile deletes the file or directory at 'filePath'.
func (storage *GCSStorage) DeleteFile(threadIndex int, filePath string) (err error) {
	return storage.retry(func() error {
		err := storage.service.Objects.Delete(storage.bucket, storage.storageDir+filePath).Do()
		if isGCSNotFound(err) {
			return nil
		}
		return err
	})
}

// MoveFile renames the file.
func (storage *GCSStorage) MoveFile(threadIndex int, from string, to string) (err error) {
	err = storage.retry(func() error {
		call := storage.service.Objects.Rewrite(storage.bucket, storage.storageDir+from, storage.bucket,
			storage.storageDir+to, &gcs.Object{}).Fields("done", "rewriteToken")
		// Large objects may take several calls; each continues where the last one stopped
		for {
			response, err := call.Do()
			if err != nil {
				return err
			}
			if response.Done {
				return nil
			}
			call = call.RewriteToken(response.RewriteToken)
		}
	})
	if err != nil {
		return err
	}

	return storage.DeleteFile(threadIndex, from)
}

// CreateDirectory creates a new directory.
func (storage *GCSStorage) CreateDirectory(threadIndex int, dir string) (err error) {
	return nil
}

// GetFileInfo returns the information about the file or directory at 'filePath'.
func (storage *GCSStorage) GetFileInfo(threadIndex int, filePath string) (exist bool, isDir bool, size int64, err error) {
	err = storage.retry(func() error {
		object, err := storage.service.Objects.Get(storage.bucket, storage.storageDir+filePath).Fields("size").Do()
		if isGCSNotFound(err) {
			exist = false
			return nil
		} else if err != nil {
			return err
		}
		exist, size = true, int64(object.Size)
		return nil
	})
	if err != nil {
		return false, false, 0, err
	}
	return exist, false, size, nil
}

// DownloadFile reads the file at 'filePath' into the chunk.
func (storage *GCSStorage) DownloadFile(threadIndex int, filePath string, chunk *Chunk) (err error) {
	var response *http.Response
	err = storage.retry(func() (err error) {
		response, err = storage.service.Objects.Get(storage.bucket, storage.storageDir+filePath).Download()
		return err
	})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, err = RateLimitedCopy(chunk, response.Body, storage.DownloadRateLimit/storage.numberOfThreads)
	return err
}

// UploadFile writes 'content' to the file at 'filePath'.
func (storage *GCSStorage) UploadFile(threadIndex int, filePath string, content []byte) (err error) {
	return storage.retry(func() error {
		reader := CreateRateLimitedReader(content, storage.UploadRateLimit/storage.numberOfThreads)
		_, err := storage.service.Objects.Insert(storage.bucket, &gcs.Object{Name: storage.storageDir + filePath}).
			Media(reader, googleapi.ContentType("application/octet-stream")).Fields("name").Do()
		return err
	})
}

// If a local snapshot cache is needed for the storage to avoid downloading/uploading chunks too often when
// managing snapshots.
func (storage *GCSStorage) IsCacheNeeded() bool { return true }

// If the 'MoveFile' method is implemented.
func (storage *GCSStorage) IsMoveFileImplemented() bool { return true }

// If the storage can guarantee strong consistency.
func (storage *GCSStorage) IsStrongConsistent() bool { return true }

// If the storage supports fast listing of files names.
func (storage *GCSStorage) IsFastListing() bool { return true }

// Enable the test mode.
func (storage *GCSStorage) EnableTestMode() { storage.TestMode = true }
