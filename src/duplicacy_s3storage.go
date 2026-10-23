// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

// NOTE: The code in the Wasabi storage module relies on all functions
// in this one except MoveFile(), IsMoveFileImplemented() and
// IsStrongConsistent().  Changes to the API here will need to be
// reflected there.

package duplicacy

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Storage struct {
	StorageBase

	client          *s3.Client
	bucket          string
	storageDir      string
	numberOfThreads int
}

// CreateS3Storage creates a amazon s3 storage object.  'endpoint' is a host name (with an optional port), or empty
// for Amazon S3.
func CreateS3Storage(regionName string, endpoint string, bucketName string, storageDir string,
	accessKey string, secretKey string, threads int,
	isSSLSupported bool, isMinioCompatible bool) (storage *S3Storage, err error) {

	httpClient := newHTTPClient(threads)
	newClient := func(region string) *s3.Client {
		return s3.New(s3.Options{
			Region:      region,
			Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
			HTTPClient:  httpClient,
			// Recent SDK versions add CRC checksums to every request by default, which many S3-compatible
			// services reject; send them only where the API requires them, as earlier versions did
			RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
			ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
		}, func(options *s3.Options) {
			if endpoint != "" {
				scheme := "https://"
				if !isSSLSupported {
					scheme = "http://"
				}
				options.BaseEndpoint = aws.String(scheme + endpoint)
			}
			options.UsePathStyle = isMinioCompatible
		})
	}

	if regionName == "" && endpoint == "" {
		response, err := newClient("us-east-1").GetBucketLocation(context.Background(),
			&s3.GetBucketLocationInput{Bucket: aws.String(bucketName)})
		if err != nil {
			return nil, err
		}

		regionName = "us-east-1"
		if response.LocationConstraint != "" {
			regionName = string(response.LocationConstraint)
		}
	} else if regionName == "" {
		// Requests must be signed for some region; S3-compatible services that don't use regions accept this one
		regionName = "us-east-1"
	}

	if len(storageDir) > 0 && storageDir[len(storageDir)-1] != '/' {
		storageDir += "/"
	}

	storage = &S3Storage{
		client:          newClient(regionName),
		bucket:          bucketName,
		storageDir:      storageDir,
		numberOfThreads: threads,
	}

	storage.DerivedStorage = storage
	storage.SetDefaultNestingLevels([]int{0}, 0)
	return storage, nil
}

// ListFiles return the list of files and subdirectories under 'dir' (non-recursively)
func (storage *S3Storage) ListFiles(threadIndex int, dir string) (files []string, sizes []int64, err error) {
	if len(dir) > 0 && dir[len(dir)-1] != '/' {
		dir += "/"
	}

	// Only the 'snapshots' directory is listed by subdirectory; everywhere else all files below 'dir' are listed
	listSubDirectories := dir == "snapshots/"
	dir = storage.storageDir + dir

	marker := ""
	for {
		input := &s3.ListObjectsInput{
			Bucket:  aws.String(storage.bucket),
			Prefix:  aws.String(dir),
			MaxKeys: aws.Int32(1000),
		}
		if marker != "" {
			input.Marker = aws.String(marker)
		}
		if listSubDirectories {
			input.Delimiter = aws.String("/")
		}

		output, err := storage.client.ListObjects(context.Background(), input)
		if err != nil {
			return nil, nil, err
		}

		for _, subDir := range output.CommonPrefixes {
			files = append(files, aws.ToString(subDir.Prefix)[len(dir):])
		}
		for _, object := range output.Contents {
			if listSubDirectories {
				continue
			}
			files = append(files, aws.ToString(object.Key)[len(dir):])
			sizes = append(sizes, aws.ToInt64(object.Size))
		}

		if !aws.ToBool(output.IsTruncated) {
			break
		}

		// NextMarker is only returned when a delimiter is given; otherwise the last key is the marker
		if output.NextMarker != nil {
			marker = *output.NextMarker
		} else if len(output.Contents) > 0 {
			marker = aws.ToString(output.Contents[len(output.Contents)-1].Key)
		} else {
			break
		}
	}

	if listSubDirectories {
		return files, nil, nil
	}
	return files, sizes, nil
}

// DeleteFile deletes the file or directory at 'filePath'.
func (storage *S3Storage) DeleteFile(threadIndex int, filePath string) (err error) {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(storage.bucket),
		Key:    aws.String(storage.storageDir + filePath),
	}
	_, err = storage.client.DeleteObject(context.Background(), input)
	return err
}

// MoveFile renames the file.
func (storage *S3Storage) MoveFile(threadIndex int, from string, to string) (err error) {

	input := &s3.CopyObjectInput{
		Bucket:     aws.String(storage.bucket),
		CopySource: aws.String(url.PathEscape(storage.bucket) + "/" + escapeS3Key(storage.storageDir+from)),
		Key:        aws.String(storage.storageDir + to),
	}

	_, err = storage.client.CopyObject(context.Background(), input)
	if err != nil {
		return err
	}

	return storage.DeleteFile(threadIndex, from)

}

// CreateDirectory creates a new directory.
func (storage *S3Storage) CreateDirectory(threadIndex int, dir string) (err error) {
	return nil
}

// GetFileInfo returns the information about the file or directory at 'filePath'.
func (storage *S3Storage) GetFileInfo(threadIndex int, filePath string) (exist bool, isDir bool, size int64, err error) {

	input := &s3.HeadObjectInput{
		Bucket: aws.String(storage.bucket),
		Key:    aws.String(storage.storageDir + filePath),
	}

	output, err := storage.client.HeadObject(context.Background(), input)
	if err != nil {
		var responseError interface{ HTTPStatusCode() int }
		if errors.As(err, &responseError) && (responseError.HTTPStatusCode() == 403 || responseError.HTTPStatusCode() == 404) {
			return false, false, 0, nil
		} else {
			return false, false, 0, err
		}
	}

	if output == nil || output.ContentLength == nil {
		return false, false, 0, nil
	} else {
		return true, false, *output.ContentLength, nil
	}
}

// DownloadFile reads the file at 'filePath' into the chunk.
func (storage *S3Storage) DownloadFile(threadIndex int, filePath string, chunk *Chunk) (err error) {

	input := &s3.GetObjectInput{
		Bucket: aws.String(storage.bucket),
		Key:    aws.String(storage.storageDir + filePath),
	}

	output, err := storage.client.GetObject(context.Background(), input)
	if err != nil {
		return err
	}

	defer output.Body.Close()

	_, err = RateLimitedCopy(chunk, output.Body, storage.DownloadRateLimit/storage.numberOfThreads)
	return err

}

// UploadFile writes 'content' to the file at 'filePath'.
func (storage *S3Storage) UploadFile(threadIndex int, filePath string, content []byte) (err error) {

	attempts := 0

	for {
		input := &s3.PutObjectInput{
			Bucket:        aws.String(storage.bucket),
			Key:           aws.String(storage.storageDir + filePath),
			ACL:           types.ObjectCannedACLPrivate,
			Body:          CreateRateLimitedReader(content, storage.UploadRateLimit/storage.numberOfThreads),
			ContentLength: aws.Int64(int64(len(content))),
			ContentType:   aws.String("application/duplicacy"),
		}

		_, err = storage.client.PutObject(context.Background(), input)
		if err == nil || attempts >= 3 || !strings.Contains(err.Error(), "XAmzContentSHA256Mismatch") {
			return err
		}

		LOG_INFO("S3_RETRY", "Retrying on %s: %v", reflect.TypeOf(err), err)
		attempts += 1
	}
}

// escapeS3Key escapes each segment of an object key for use in a copy source.
func escapeS3Key(key string) string {
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

// If a local snapshot cache is needed for the storage to avoid downloading/uploading chunks too often when
// managing snapshots.
func (storage *S3Storage) IsCacheNeeded() bool { return true }

// If the 'MoveFile' method is implemented.
func (storage *S3Storage) IsMoveFileImplemented() bool { return true }

// If the storage can guarantee strong consistency.
func (storage *S3Storage) IsStrongConsistent() bool { return false }

// If the storage supports fast listing of files names.
func (storage *S3Storage) IsFastListing() bool { return true }

// Enable the test mode.
func (storage *S3Storage) EnableTestMode() {}
