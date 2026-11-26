//go:build !production

package devmock

import (
	"testing"

	"campfire/internal/shared/model"
)

func TestMockFileSystemBusyMatchesFileAccess(t *testing.T) {
	for _, action := range []model.ServerAction{
		model.ServerActionStart,
		model.ServerActionStop,
		model.ServerActionRestart,
		model.ServerActionKill,
		model.ServerActionReinstall,
	} {
		t.Run(string(action), func(t *testing.T) {
			runtime, _, _, servers := fixture(t)

			server := onlineServer(t, runtime, servers)
			if action == model.ServerActionStart || action == model.ServerActionReinstall {
				if _, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: model.ServerActionStop, OperationID: "prepare"}); err != nil {
					t.Fatal(err)
				}

				settleRuntime(t, runtime, server)
			}

			status, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: action, OperationID: "maintenance"})

			wantBusy := action == model.ServerActionStart || action == model.ServerActionStop || action == model.ServerActionRestart
			if err != nil || status.Busy != wantBusy || status.FileSystemBusy != (wantBusy && action.LocksFileSystem()) {
				t.Fatalf("incorrect mock operation status: %+v, %v", status, err)
			}

			for _, request := range []model.FileRequest{
				{ServerID: server.ID, Operation: model.FileOperationList, Path: "."},
				{ServerID: server.ID, Operation: model.FileOperationWrite, Path: "notes.txt", Data: []byte("Maintenance notes\n"), Revision: "absent"},
			} {
				if _, err := runtime.Files(t.Context(), server.SparkID, request); err != nil {
					t.Fatalf("available mock filesystem blocked file access: %v", err)
				}
			}

			if status = settleRuntime(t, runtime, server); status.FileSystemBusy {
				t.Fatalf("mock filesystem remained locked after completion: %+v", status)
			}

			if _, err := runtime.Files(t.Context(), server.SparkID, model.FileRequest{ServerID: server.ID, Operation: model.FileOperationList, Path: "."}); err != nil {
				t.Fatalf("file access did not recover: %v", err)
			}
		})
	}
}
