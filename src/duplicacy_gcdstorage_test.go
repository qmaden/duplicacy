// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// fakeDrive implements the part of the Google Drive v3 API that the GCD backend uses: listing children of a folder
// (optionally by name or by folder/non-folder), creating folders, multipart uploads, downloads, moves between
// parents, deletes and looking up shared drives by name.
type fakeDrive struct {
	lock   sync.Mutex
	files  map[string]*fakeDriveFile
	nextID int
}

type fakeDriveFile struct {
	file    drive.File
	content []byte
}

const fakeSharedDriveID = "shared-drive-id"

var (
	fakeDriveParentRegex   = regexp.MustCompile(`'([^']+)' in parents`)
	fakeDriveNameRegex     = regexp.MustCompile(`name ?= ?'([^']*)'`)
	fakeDriveMimeTypeRegex = regexp.MustCompile(`mimeType (!?=) '([^']*)'`)
)

func newFakeDrive() *httptest.Server {
	fake := &fakeDrive{files: make(map[string]*fakeDriveFile)}
	return httptest.NewServer(fake)
}

func writeDriveError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error": {"code": %d, "message": %q, "errors": [{"reason": "test", "message": %q}]}}`, code,
		message, message)
}

func writeDriveJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}

func (fake *fakeDrive) create(metadata *drive.File, content []byte) *drive.File {
	fake.nextID++
	file := *metadata
	file.Id = fmt.Sprintf("id%06d", fake.nextID)
	file.Size = int64(len(content))
	fake.files[file.Id] = &fakeDriveFile{file: file, content: content}
	return &drive.File{Id: file.Id}
}

func (fake *fakeDrive) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fake.lock.Lock()
	defer fake.lock.Unlock()

	query := r.URL.Query()
	switch {
	case r.Method == "GET" && r.URL.Path == "/drive/v3/drives":
		name := fakeDriveNameRegex.FindStringSubmatch(query.Get("q"))
		list := &drive.DriveList{}
		if name != nil && name[1] == "Shared" {
			list.Drives = []*drive.Drive{{Id: fakeSharedDriveID, Name: "Shared"}}
		}
		writeDriveJSON(w, list)

	case r.Method == "GET" && r.URL.Path == "/drive/v3/files":
		q := query.Get("q")
		if strings.Contains(q, "sharedWithMe") {
			writeDriveJSON(w, &drive.FileList{})
			return
		}
		parent := fakeDriveParentRegex.FindStringSubmatch(q)
		if parent == nil {
			writeDriveError(w, 400, "unsupported query "+q)
			return
		}
		name := fakeDriveNameRegex.FindStringSubmatch(q)
		mimeType := fakeDriveMimeTypeRegex.FindStringSubmatch(q)

		var matches []*drive.File
		for _, f := range fake.files {
			if len(f.file.Parents) != 1 || f.file.Parents[0] != parent[1] {
				continue
			}
			if name != nil && f.file.Name != name[1] {
				continue
			}
			if mimeType != nil && (f.file.MimeType == mimeType[2]) != (mimeType[1] == "=") {
				continue
			}
			file := f.file
			matches = append(matches, &file)
		}
		sort.Slice(matches, func(i, j int) bool { return matches[i].Id < matches[j].Id })

		start, _ := strconv.Atoi(query.Get("pageToken"))
		pageSize := 100
		if size, err := strconv.Atoi(query.Get("pageSize")); err == nil && size > 0 {
			pageSize = size
		}
		list := &drive.FileList{}
		end := start + pageSize
		if end < len(matches) {
			list.NextPageToken = strconv.Itoa(end)
		} else {
			end = len(matches)
		}
		if start < end {
			list.Files = matches[start:end]
		}
		writeDriveJSON(w, list)

	case r.Method == "POST" && r.URL.Path == "/drive/v3/files":
		metadata := &drive.File{}
		if err := json.NewDecoder(r.Body).Decode(metadata); err != nil {
			writeDriveError(w, 400, err.Error())
			return
		}
		writeDriveJSON(w, fake.create(metadata, nil))

	case r.Method == "POST" && r.URL.Path == "/upload/drive/v3/files":
		if query.Get("uploadType") != "multipart" {
			writeDriveError(w, 501, "only multipart uploads are implemented")
			return
		}
		_, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			writeDriveError(w, 400, err.Error())
			return
		}
		reader := multipart.NewReader(r.Body, parameters["boundary"])
		part, err := reader.NextPart()
		if err != nil {
			writeDriveError(w, 400, err.Error())
			return
		}
		metadata := &drive.File{}
		if err = json.NewDecoder(part).Decode(metadata); err != nil {
			writeDriveError(w, 400, err.Error())
			return
		}
		part, err = reader.NextPart()
		if err != nil {
			writeDriveError(w, 400, err.Error())
			return
		}
		content, err := io.ReadAll(part)
		if err != nil {
			writeDriveError(w, 400, err.Error())
			return
		}
		writeDriveJSON(w, fake.create(metadata, content))

	case strings.HasPrefix(r.URL.Path, "/drive/v3/files/"):
		id := strings.TrimPrefix(r.URL.Path, "/drive/v3/files/")
		f, found := fake.files[id]
		if !found {
			writeDriveError(w, 404, "File not found: "+id)
			return
		}
		switch r.Method {
		case "GET":
			if query.Get("alt") != "media" {
				file := f.file
				writeDriveJSON(w, &file)
				return
			}
			w.Write(f.content)
		case "PATCH":
			if remove := query.Get("removeParents"); remove != "" && (len(f.file.Parents) != 1 || f.file.Parents[0] != remove) {
				writeDriveError(w, 400, "not a parent: "+remove)
				return
			}
			if add := query.Get("addParents"); add != "" {
				f.file.Parents = []string{add}
			}
			writeDriveJSON(w, &drive.File{Id: id})
		case "DELETE":
			delete(fake.files, id)
			w.WriteHeader(204)
		default:
			writeDriveError(w, 405, r.Method)
		}

	default:
		writeDriveError(w, 404, "unknown request "+r.Method+" "+r.URL.Path)
	}
}

func createFakeGCDStorage(t *testing.T, server *httptest.Server, driveID string, storagePath string,
	threads int) *GCDStorage {
	storage, err := createGCDStorage(driveID, storagePath, threads, false, drive.DriveScope,
		option.WithEndpoint(server.URL+"/drive/v3/"), option.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return storage
}

// TestGCDStorage exercises the Google Drive backend against an in-process fake of the Drive API, on the user's
// drive and on a shared drive given by name.
func TestGCDStorage(t *testing.T) {
	setTestingT(t)
	server := newFakeDrive()
	t.Cleanup(server.Close)

	for _, driveID := range []string{"", "Shared"} {
		storage := createFakeGCDStorage(t, server, driveID, "backups/storage", 4)
		storage.EnableTestMode() // small pages, so that listings need several
		exerciseCloudStorage(t, storage, 40)
	}
}

// ListFiles lists the chunk tree with one goroutine per directory.  A goroutine for a directory missing from the id
// cache returned without reporting back, so the listing waited forever; prune and check could hang this way.
func TestGCDListFilesWithUncachedDirectory(t *testing.T) {
	setTestingT(t)
	server := newFakeDrive()
	t.Cleanup(server.Close)
	storage := createFakeGCDStorage(t, server, "", "uncached", 2)
	if err := storage.UploadFile(0, "chunks/ab/cdef", []byte("chunk")); err != nil {
		t.Fatal(err)
	}
	storage.deletePathID("fossils")

	done := make(chan error, 1)
	go func() {
		files, _, err := storage.ListFiles(0, "chunks")
		if err == nil && (len(files) != 1 || files[0] != "ab/cdef") {
			err = fmt.Errorf("listed %v", files)
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("ListFiles didn't return")
	}
}

// Storages that keep per-thread state can only be used with as many threads as they were created with.  The
// snapshot manager downloaded metadata on 4 threads whatever the storage allowed, so with -threads 1 (the default,
// and always for list, cat, diff and history) Google Drive panicked on every call, and Azure, Dropbox and S3C on
// metadata downloads.
func TestBackupAndRestoreOnOneThreadStorage(t *testing.T) {
	setTestingT(t)
	SetLoggingLevel(ERROR)
	defer catchFixtureException(t)
	server := newFakeDrive()
	t.Cleanup(server.Close)

	storage := createFakeGCDStorage(t, server, "", "one-thread", 1)
	if !ConfigStorage(storage, CONFIG_DEFAULT_ITERATIONS, DEFAULT_COMPRESSION_LEVEL, 4096, 16384, 1024, "", nil, false, "", 0, 0) {
		t.Fatal("Failed to initialize the storage")
	}

	repository := t.TempDir()
	for i := 0; i < 20; i++ {
		os.WriteFile(filepath.Join(repository, fmt.Sprintf("file%02d", i)), compressionTestData(5000, int64(i), 1), 0600)
	}
	os.MkdirAll(filepath.Join(repository, DUPLICACY_DIRECTORY), 0700)
	SetDuplicacyPreferencePath(filepath.Join(repository, DUPLICACY_DIRECTORY))
	manager := CreateBackupManager("test", storage, repository, "", "", "", false)
	manager.SetupSnapshotCache("default")
	for revision := 1; revision <= 2; revision++ {
		os.WriteFile(filepath.Join(repository, "changing"), compressionTestData(9000, int64(100+revision), 1), 0600)
		if !manager.Backup(repository, false, 1, "", false, false, 0, false, 1024, 1024) {
			t.Fatal("Backup failed")
		}
	}

	top := t.TempDir()
	os.MkdirAll(filepath.Join(top, DUPLICACY_DIRECTORY), 0700)
	SetDuplicacyPreferencePath(filepath.Join(top, DUPLICACY_DIRECTORY))
	storage = createFakeGCDStorage(t, server, "", "one-thread", 1)
	manager = CreateBackupManager("test", storage, top, "", "", "", false)
	manager.SetupSnapshotCache("default")
	if failed := manager.Restore(top, 2, true, false, 1, false, false, false, false, nil, false); failed != 0 {
		t.Fatalf("%d files failed to restore", failed)
	}
	for _, name := range []string{"file00", "file19", "changing"} {
		expected, _ := os.ReadFile(filepath.Join(repository, name))
		restored, err := os.ReadFile(filepath.Join(top, name))
		if err != nil || !bytes.Equal(expected, restored) {
			t.Errorf("%s was not restored correctly: %v", name, err)
		}
	}
	if listed := manager.SnapshotManager.ListSnapshots("test", nil, "", true, false); listed != 2 {
		t.Errorf("Listed %d snapshots instead of 2", listed)
	}
}
