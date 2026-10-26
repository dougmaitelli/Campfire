package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"testing"

	"campfire/internal/shared/model"
)

func TestFileAPIUsesServerDataRootedPaths(t *testing.T) {
	f := newAccessFixture(t)
	_, _, input := serverFixture(t, f.admin)

	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID + "/files"

	for _, test := range []struct {
		name        string
		input       model.FileRequest
		path        string
		paths       []string
		destination string
		result      string
	}{
		{name: "root", input: model.FileRequest{Operation: "list", Path: "/"}, path: ".", result: "/"},
		{name: "legacy root", input: model.FileRequest{Operation: "list", Path: "."}, path: ".", result: "/"},
		{name: "folder", input: model.FileRequest{Operation: "list", Path: "/config/fabric/"}, path: "config/fabric", result: "/config/fabric/"},
		{name: "legacy folder", input: model.FileRequest{Operation: "list", Path: "config/fabric"}, path: "config/fabric", result: "/config/fabric/"},
		{name: "read", input: model.FileRequest{Operation: "read", Path: "/config/settings.json"}, path: "config/settings.json", result: "/config/settings.json"},
		{name: "virtual root", input: model.FileRequest{Operation: "read", Path: "/etc/config.json"}, path: "etc/config.json", result: "/etc/config.json"},
		{name: "create folder", input: model.FileRequest{Operation: "mkdir", Path: "/config/new/"}, path: "config/new", result: "/config/new/"},
		{name: "move", input: model.FileRequest{Operation: "rename", Path: "/config/file.txt", Destination: "/mods/file.txt"}, path: "config/file.txt", destination: "mods/file.txt", result: "/mods/file.txt"},
		{name: "archive", input: model.FileRequest{Operation: "compress", Path: "/config/", Paths: []string{"/config/fabric/", "/config/settings.json"}, Destination: "/config/archive.tar.gz"}, path: "config", paths: []string{"config/fabric", "config/settings.json"}, destination: "config/archive.tar.gz", result: "/config/archive.tar.gz"},
		{name: "extract", input: model.FileRequest{Operation: "extract", Path: "/config/archive.tar.gz", Destination: "/config/extracted/"}, path: "config/archive.tar.gz", destination: "config/extracted", result: "/config/extracted/"},
		{name: "upload", input: model.FileRequest{Operation: "upload.begin", Path: "/mods/new.jar"}, path: "mods/new.jar", result: "/mods/new.jar"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f.runtime.files = func(q model.FileRequest) (model.FileResult, error) {
				if q.ServerID != server.ID || q.Operation != test.input.Operation || q.Path != test.path || q.Destination != test.destination || !slices.Equal(q.Paths, test.paths) {
					t.Fatal("incorrect runtime path translation", q)
				}

				result := q.Path
				if q.Destination != "" {
					result = q.Destination
				}

				return model.FileResult{ServerID: q.ServerID, Path: result}, nil
			}

			body, err := json.Marshal(test.input)
			if err != nil {
				t.Fatal(err)
			}

			response := request(f.admin, "POST", endpoint, string(body))

			var result model.FileResult
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Path != test.result {
				t.Fatal("incorrect public path", response.Code, response.Body)
			}
		})
	}

	f.runtime.files = nil
	for _, query := range []string{"", "?path=/", "?path=/config/fabric/", "?path=config/fabric"} {
		response := request(f.admin, "GET", endpoint+query, "")

		var result model.FileResult

		want := "/config/fabric/"
		if query == "" || query == "?path=/" {
			want = "/"
		}

		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Path != want {
			t.Fatal("incorrect listing path", query, response.Code, response.Body)
		}
	}

	for _, value := range []string{"/../private", "/config/../private", "config/../../private", "/config\\private", "/config/\x00", "/config/\n", "/config//fabric"} {
		for _, field := range []string{"path", "destination", "paths"} {
			body := map[string]any{"operation": "compress", "path": "/config/", "destination": "/archive.tar.gz", "paths": []string{"/config/settings.json"}}
			if field == "paths" {
				body[field] = []string{value}
			} else {
				body[field] = value
			}

			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}

			before := f.runtime.calls.Load()

			response := request(f.admin, "POST", endpoint, string(encoded))
			if response.Code != http.StatusBadRequest || f.runtime.calls.Load() != before {
				t.Fatal("invalid path reached Spark", field, value, response.Code)
			}
		}

		before := f.runtime.calls.Load()

		response := request(f.admin, "GET", endpoint+"/download?path="+url.QueryEscape(value), "")
		if response.Code != http.StatusBadRequest || f.runtime.calls.Load() != before {
			t.Fatal("invalid download path reached Spark", value, response.Code)
		}
	}
}
