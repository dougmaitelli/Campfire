//go:build !production

package devmock

import (
	"context"
	"errors"
	"testing"

	"campfire/internal/shared/model"
)

func TestMockUploadsDiscardContentsAndSupportCancellation(t *testing.T) {
	runtime, _, _, servers := fixture(t)
	server := servers[0]
	call := func(input model.FileRequest) (model.FileResult, error) {
		input.ServerID = server.ID
		input.UploadLimit = model.DefaultUploadLimit

		return runtime.Files(t.Context(), server.SparkID, input)
	}

	for _, name := range []string{"simulated.txt", "empty.txt", "cancelled.txt"} {
		size := int64(3)
		if name == "empty.txt" {
			size = 0
		}

		started, err := call(model.FileRequest{Operation: "upload.begin", Path: name, Revision: "absent", Size: size})
		if err != nil || started.UploadID == "" {
			t.Fatal("upload did not start", started, err)
		}

		input := model.FileRequest{Operation: "upload.chunk", Path: name, UploadID: started.UploadID, Data: []byte("abc")}
		if size > 0 {
			input.Offset = 1
			if _, err := call(input); err == nil {
				t.Fatal("accepted out-of-order chunk")
			}

			input.Offset = 0
			if _, err := call(input); err != nil {
				t.Fatal(err)
			}
		}

		input.Operation = "upload.commit"
		if name == "cancelled.txt" {
			input.Operation = "upload.cancel"
		}

		if _, err := call(input); err != nil {
			t.Fatal(err)
		}

		if _, err := call(model.FileRequest{Operation: "read", Path: name}); err == nil {
			t.Fatal("simulated upload saved a file", name)
		}
	}

	if len(runtime.uploads) != 0 {
		t.Fatal("completed uploads retained their sessions")
	}

	if _, err := call(model.FileRequest{Operation: "upload.begin", Path: "server.properties", Revision: "absent"}); err == nil {
		t.Fatal("accepted an existing destination")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := runtime.Files(ctx, server.SparkID, model.FileRequest{Operation: "upload.chunk"}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled chunk did not stop", err)
	}
}
