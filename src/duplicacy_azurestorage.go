// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/streaming"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

type AzureStorage struct {
	StorageBase

	container       *container.Client
	numberOfThreads int
}

func CreateAzureStorage(accountName string, accountKey string,
	containerName string, threads int) (azureStorage *AzureStorage, err error) {
	return createAzureStorage(fmt.Sprintf("https://%s.blob.core.windows.net/", accountName), accountName, accountKey,
		containerName, threads)
}

// createAzureStorage creates an Azure storage object for the blob service at 'serviceURL'.
func createAzureStorage(serviceURL string, accountName string, accountKey string, containerName string,
	threads int) (azureStorage *AzureStorage, err error) {

	credential, err := container.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		return nil, err
	}

	// The client is safe for concurrent use; it retries transient errors itself
	options := &container.ClientOptions{ClientOptions: policy.ClientOptions{Transport: newHTTPClient(threads)}}
	client, err := container.NewClientWithSharedKeyCredential(strings.TrimSuffix(serviceURL, "/")+"/"+
		url.PathEscape(containerName), credential, options)
	if err != nil {
		return nil, err
	}

	_, err = client.GetProperties(context.Background(), nil)
	if bloberror.HasCode(err, bloberror.ContainerNotFound) {
		return nil, fmt.Errorf("container %s does not exist", containerName)
	} else if err != nil {
		return nil, err
	}

	if threads < 1 {
		threads = 1
	}
	azureStorage = &AzureStorage{
		container:       client,
		numberOfThreads: threads,
	}

	azureStorage.DerivedStorage = azureStorage
	azureStorage.SetDefaultNestingLevels([]int{0}, 0)
	return
}

// isAzureNotFound returns true if the error is a 404 response.
func isAzureNotFound(err error) bool {
	var responseError *azcore.ResponseError
	return bloberror.HasCode(err, bloberror.BlobNotFound) ||
		(errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound)
}

// ListFiles return the list of files and subdirectories under 'dir' (non-recursively)
func (azureStorage *AzureStorage) ListFiles(threadIndex int, dir string) (files []string, sizes []int64, err error) {

	if len(dir) > 0 && dir[len(dir)-1] != '/' {
		dir += "/"
	}
	dirLength := len(dir)

	subDirs := make(map[string]bool)

	pager := azureStorage.container.NewListBlobsFlatPager(&container.ListBlobsFlatOptions{Prefix: &dir})
	for pager.More() {
		page, err := pager.NextPage(context.Background())
		if err != nil {
			return nil, nil, err
		}

		for _, item := range page.Segment.BlobItems {
			name := (*item.Name)[dirLength:]
			if dir == "snapshots/" {
				subDirs[strings.Split(name, "/")[0]+"/"] = true
			} else {
				files = append(files, name)
				var size int64
				if item.Properties != nil && item.Properties.ContentLength != nil {
					size = *item.Properties.ContentLength
				}
				sizes = append(sizes, size)
			}
		}
	}

	if dir == "snapshots/" {
		for subDir := range subDirs {
			files = append(files, subDir)
		}
	}

	return files, sizes, nil
}

// DeleteFile deletes the file or directory at 'filePath'.
func (storage *AzureStorage) DeleteFile(threadIndex int, filePath string) (err error) {
	_, err = storage.container.NewBlobClient(filePath).Delete(context.Background(), nil)
	if isAzureNotFound(err) {
		return nil
	}
	return err
}

// MoveFile renames the file.
func (storage *AzureStorage) MoveFile(threadIndex int, from string, to string) (err error) {
	source := storage.container.NewBlobClient(from)
	destination := storage.container.NewBlobClient(to)
	response, err := destination.StartCopyFromURL(context.Background(), source.URL(), nil)
	if err != nil {
		return err
	}

	// A copy within an account usually completes immediately, but the service may finish it asynchronously
	status := response.CopyStatus
	for wait := 100 * time.Millisecond; status != nil && *status == blob.CopyStatusTypePending; {
		time.Sleep(wait)
		if wait < 5*time.Second {
			wait *= 2
		}
		properties, err := destination.GetProperties(context.Background(), nil)
		if err != nil {
			return err
		}
		status = properties.CopyStatus
		if status != nil && *status != blob.CopyStatusTypePending && *status != blob.CopyStatusTypeSuccess {
			description := ""
			if properties.CopyStatusDescription != nil {
				description = *properties.CopyStatusDescription
			}
			return fmt.Errorf("copying %s to %s ended with status %s: %s", from, to, *status, description)
		}
	}

	return storage.DeleteFile(threadIndex, from)
}

// CreateDirectory creates a new directory.
func (storage *AzureStorage) CreateDirectory(threadIndex int, dir string) (err error) {
	return nil
}

// GetFileInfo returns the information about the file or directory at 'filePath'.
func (storage *AzureStorage) GetFileInfo(threadIndex int, filePath string) (exist bool, isDir bool, size int64, err error) {
	properties, err := storage.container.NewBlobClient(filePath).GetProperties(context.Background(), nil)
	if err != nil {
		if isAzureNotFound(err) {
			return false, false, 0, nil
		} else {
			return false, false, 0, err
		}
	}

	if properties.ContentLength != nil {
		size = *properties.ContentLength
	}
	return true, false, size, nil
}

// DownloadFile reads the file at 'filePath' into the chunk.
func (storage *AzureStorage) DownloadFile(threadIndex int, filePath string, chunk *Chunk) (err error) {
	response, err := storage.container.NewBlobClient(filePath).DownloadStream(context.Background(), nil)
	if err != nil {
		return err
	}

	defer response.Body.Close()

	_, err = RateLimitedCopy(chunk, response.Body, storage.DownloadRateLimit/storage.numberOfThreads)
	return err
}

// UploadFile writes 'content' to the file at 'filePath'.
func (storage *AzureStorage) UploadFile(threadIndex int, filePath string, content []byte) (err error) {
	reader := CreateRateLimitedReader(content, storage.UploadRateLimit/storage.numberOfThreads)
	_, err = storage.container.NewBlockBlobClient(filePath).Upload(context.Background(), streaming.NopCloser(reader), nil)
	return err
}

// If a local snapshot cache is needed for the storage to avoid downloading/uploading chunks too often when
// managing snapshots.
func (storage *AzureStorage) IsCacheNeeded() bool { return true }

// If the 'MoveFile' method is implemented.
func (storage *AzureStorage) IsMoveFileImplemented() bool { return true }

// If the storage can guarantee strong consistency.
func (storage *AzureStorage) IsStrongConsistent() bool { return true }

// If the storage supports fast listing of files names.
func (storage *AzureStorage) IsFastListing() bool { return true }

// Enable the test mode.
func (storage *AzureStorage) EnableTestMode() {}
