package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestLargeFileDownloadUsesBoundedChunksAndServerPermissions(t *testing.T) {
	f := newAccessFixture(t)
	_, other := f.user("other", database.UserRole)
	_, _, input := serverFixture(t, f.admin)

	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	data := bytes.Repeat([]byte{7}, 2*model.FileLimit+3)
	chunks := 0
	f.runtime.files = func(q model.FileRequest) (model.FileResult, error) {
		if q.ServerID != server.ID || q.Path != "world.bin" {
			t.Fatal("file placement changed", q)
		}

		result := model.FileResult{ServerID: q.ServerID, Path: q.Path, Revision: "snapshot", Size: int64(len(data))}
		switch q.Operation {
		case "stat":
			result.Entries = []model.FileEntry{{Name: "world.bin", Kind: "file"}}
		case "read.chunk":
			chunks++
			result.Data = data[q.Offset:min(q.Offset+model.FileLimit, int64(len(data)))]
		default:
			t.Fatal("unexpected download operation", q.Operation)
		}

		return result, nil
	}
	endpoint := "/api/v1/servers/" + server.ID + "/files/download?path=world.bin"
	before := f.runtime.calls.Load()

	limits := request(f.admin, "GET", "/api/v1/servers/"+server.ID+"/files/limits", "")
	if limits.Code != http.StatusOK || limits.Body.String() != "{\"max_upload_bytes\":1073741824}\n" {
		t.Fatal("incorrect upload limit response", limits.Code, limits.Body)
	}

	expectAccess(t, other, "GET", endpoint, "", http.StatusNotFound)

	if f.runtime.calls.Load() != before {
		t.Fatal("unauthorized download reached Spark")
	}

	response = request(f.admin, "GET", endpoint, "")
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), data) || chunks != 3 {
		t.Fatal("large HTTP download failed", response.Code, chunks, response.Body.Len())
	}

	if response.Header().Get("Content-Length") != "2097155" || response.Header().Get("Content-Disposition") == "" {
		t.Fatal("download headers missing", response.Header())
	}

	expectAccess(t, f.admin, "POST", "/api/v1/servers/"+server.ID+"/files", `{"operation":"backup.read","path":"world.bin"}`, http.StatusBadRequest)
}
