package backupstorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type testS3 struct {
	mu             sync.Mutex
	data           []byte
	parts          [][]byte
	digest         string
	puts, aborts   int
	failPart       bool
	deletedVersion string
}

func (s *testS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !strings.HasPrefix(r.URL.Path, "/bucket/prefix/") || r.Header.Get("Authorization") == "" {
		http.Error(w, "invalid signed request", http.StatusBadRequest)
		return
	}

	query := r.URL.Query()
	if r.Method == http.MethodHead {
		if s.data == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Length", strconv.Itoa(len(s.data)))
		w.Header().Set("X-Amz-Meta-Sha256", s.digest)
		w.Header().Set("X-Amz-Version-Id", "version-1")

		return
	}

	if r.Method == http.MethodPost && query.Has("uploads") {
		s.digest = r.Header.Get("X-Amz-Meta-Sha256")
		_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><Bucket>bucket</Bucket><Key>key</Key><UploadId>upload-1</UploadId></InitiateMultipartUploadResult>`)

		return
	}

	if r.Method == http.MethodPut {
		if s.failPart && query.Has("uploadId") {
			http.Error(w, "part rejected", http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}

		hash := sha256.Sum256(body)

		checksum := base64.StdEncoding.EncodeToString(hash[:])
		if r.Header.Get("X-Amz-Checksum-Sha256") != checksum {
			http.Error(w, "checksum failed", http.StatusBadRequest)
			return
		}

		w.Header().Set("ETag", `"etag"`)
		w.Header().Set("X-Amz-Checksum-Sha256", checksum)

		if query.Has("uploadId") {
			s.parts = append(s.parts, body)
		} else {
			s.data = body
			s.digest = r.Header.Get("X-Amz-Meta-Sha256")
			s.puts++

			w.Header().Set("X-Amz-Version-Id", "version-1")
		}

		return
	}

	if r.Method == http.MethodPost && query.Has("uploadId") {
		s.data = bytes.Join(s.parts, nil)
		s.puts++

		w.Header().Set("X-Amz-Version-Id", "version-1")
		_, _ = io.WriteString(w, `<CompleteMultipartUploadResult><Location>http://storage/key</Location><Bucket>bucket</Bucket><Key>key</Key><ETag>etag</ETag></CompleteMultipartUploadResult>`)

		return
	}

	if r.Method == http.MethodDelete {
		if query.Has("uploadId") {
			s.aborts++
			s.parts = nil
		} else {
			s.deletedVersion = query.Get("versionId")
			s.data = nil
		}

		w.WriteHeader(http.StatusNoContent)

		return
	}

	if r.Method == http.MethodGet {
		var start, end int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil || start < 0 || end >= len(s.data) {
			http.Error(w, "bad range", http.StatusRequestedRangeNotSatisfiable)
			return
		}

		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(s.data)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(s.data[start : end+1])

		return
	}

	http.Error(w, "unsupported operation", http.StatusBadRequest)
}

func testStore(t *testing.T, fixture *testS3) *S3 {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "missing"))

	server := httptest.NewServer(fixture)
	t.Cleanup(server.Close)

	store, err := New(context.Background(), Config{Bucket: "bucket", Region: "us-east-1", Endpoint: server.URL, Prefix: "prefix", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}

	return store
}

func TestS3ChecksummedUploadsRangesAndVersionDeletion(t *testing.T) {
	for _, size := range []int{71, partSize + 19} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			fixture := &testS3{}
			store := testStore(t, fixture)
			ctx := context.Background()
			data := bytes.Repeat([]byte("x"), size)
			hash := sha256.Sum256(data)
			digest := hex.EncodeToString(hash[:])

			version, err := store.Put(ctx, "server/backup.tar.gz", int64(size), digest, bytes.NewReader(data))
			if err != nil || version != "version-1" {
				t.Fatal("upload", version, err)
			}

			if _, err := store.Put(ctx, "server/backup.tar.gz", int64(size), digest, bytes.NewReader(nil)); err != nil {
				t.Fatal("lost acknowledgement retry", err)
			}

			fixture.mu.Lock()
			puts := fixture.puts
			fixture.mu.Unlock()

			if puts != 1 {
				t.Fatal("immutable upload duplicated")
			}

			chunk, err := store.Read(ctx, "server/backup.tar.gz", version, 3, 7)
			if err != nil || !bytes.Equal(chunk, data[3:10]) {
				t.Fatal("download range", err)
			}

			address, err := store.RestoreURL(ctx, "server/backup.tar.gz", version)
			if err != nil {
				t.Fatal(err)
			}

			parsed, err := url.Parse(address)
			if err != nil || parsed.Query().Get("X-Amz-Signature") == "" || parsed.Query().Get("versionId") != version {
				t.Fatal("restore URL not version-bound")
			}

			if err := store.Delete(ctx, "server/backup.tar.gz", version); err != nil {
				t.Fatal(err)
			}

			fixture.mu.Lock()
			defer fixture.mu.Unlock()

			if fixture.data != nil || fixture.deletedVersion != version {
				t.Fatal("wrong object version deleted")
			}
		})
	}
}

func TestS3AbortsFailedMultipartAndRejectsChangedBytes(t *testing.T) {
	fixture := &testS3{failPart: true}
	store := testStore(t, fixture)
	data := bytes.Repeat([]byte("x"), partSize+1)

	hash := sha256.Sum256(data)
	if _, err := store.Put(context.Background(), "server/failed.tar.gz", int64(len(data)), hex.EncodeToString(hash[:]), bytes.NewReader(data)); err == nil {
		t.Fatal("failed part accepted")
	}

	fixture.mu.Lock()
	aborts := fixture.aborts
	fixture.mu.Unlock()

	if aborts != 1 {
		t.Fatal("multipart upload was not aborted")
	}

	if _, err := store.Put(context.Background(), "server/changed.tar.gz", 3, strings.Repeat("0", 64), strings.NewReader("bad")); err == nil {
		t.Fatal("changed backup uploaded")
	}
}
