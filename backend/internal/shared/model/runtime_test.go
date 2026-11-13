package model

import (
	"encoding/json"
	"testing"
)

func TestRuntimeStatusFileSystemBusyCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
		want bool
	}{
		{"older idle Spark", `{"server_id":"game","busy":false}`, false},
		{"older busy Spark", `{"server_id":"game","busy":true}`, true},
		{"power operation", `{"server_id":"game","busy":true,"file_system_busy":false}`, false},
		{"filesystem operation", `{"server_id":"game","busy":true,"file_system_busy":true}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var status RuntimeStatus
			if err := json.Unmarshal([]byte(tc.json), &status); err != nil {
				t.Fatal(err)
			}

			if status.FileSystemBusy != tc.want || status.ServerID != "game" {
				t.Fatalf("incorrect decoded runtime status: %+v", status)
			}

			data, err := json.Marshal(status)
			if err != nil {
				t.Fatal(err)
			}

			var forwarded struct {
				FileSystemBusy *bool `json:"file_system_busy"`
			}
			if err := json.Unmarshal(data, &forwarded); err != nil {
				t.Fatal(err)
			}

			if forwarded.FileSystemBusy == nil || *forwarded.FileSystemBusy != tc.want {
				t.Fatalf("filesystem busy state lost when forwarding: %s", data)
			}
		})
	}
}
