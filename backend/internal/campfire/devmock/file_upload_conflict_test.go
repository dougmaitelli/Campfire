//go:build !production

package devmock

import (
	"errors"
	"testing"

	"campfire/internal/shared/model"
)

func TestMockUploadOverwriteRevisionChecks(t *testing.T) {
	runtime, _, _, servers := fixture(t)
	server := servers[0]
	call := func(input model.FileRequest) (model.FileResult, error) {
		input.ServerID = server.ID
		input.UploadLimit = model.DefaultUploadLimit

		return runtime.Files(t.Context(), server.SparkID, input)
	}

	info, err := call(model.FileRequest{Operation: model.FileOperationStat, Path: "server.properties"})
	if err != nil || info.Revision == "" || len(info.Entries) != 1 || info.Entries[0].Kind != model.FileKindFile {
		t.Fatal("missing overwrite metadata", info, err)
	}

	if _, err := call(model.FileRequest{Operation: model.FileOperationUploadBegin, Path: "server.properties", Revision: "stale"}); err == nil {
		t.Fatal("stale overwrite admitted")
	}

	started, err := call(model.FileRequest{Operation: model.FileOperationUploadBegin, Path: "server.properties", Revision: info.Revision})
	if err != nil {
		t.Fatal("explicit overwrite not admitted", err)
	}

	changed, err := call(model.FileRequest{Operation: model.FileOperationWrite, Path: "server.properties", Revision: info.Revision, Data: []byte("concurrent change")})
	if err != nil {
		t.Fatal(err)
	}

	_, err = call(model.FileRequest{Operation: model.FileOperationUploadCommit, Path: "server.properties", UploadID: started.UploadID})

	var failure *model.FileError
	if !errors.As(err, &failure) || failure.Code != model.FileConflict {
		t.Fatal("concurrent change was overwritten", err)
	}

	if _, err := call(model.FileRequest{Operation: model.FileOperationUploadCancel, Path: "server.properties", UploadID: started.UploadID}); err != nil {
		t.Fatal(err)
	}

	started, err = call(model.FileRequest{Operation: model.FileOperationUploadBegin, Path: "server.properties", Revision: changed.Revision})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := call(model.FileRequest{Operation: model.FileOperationUploadCommit, Path: "server.properties", UploadID: started.UploadID}); err != nil {
		t.Fatal("unchanged explicit overwrite failed", err)
	}

	if _, err := call(model.FileRequest{Operation: model.FileOperationMkdir, Path: "conflict-folder"}); err != nil {
		t.Fatal(err)
	}

	directory, err := call(model.FileRequest{Operation: model.FileOperationStat, Path: "conflict-folder"})
	if err != nil || len(directory.Entries) != 1 || directory.Entries[0].Kind != model.FileKindDirectory {
		t.Fatal("missing directory conflict metadata", directory, err)
	}

	if _, err := call(model.FileRequest{Operation: model.FileOperationUploadBegin, Path: "conflict-folder", Revision: directory.Revision}); err == nil {
		t.Fatal("directory overwrite admitted")
	}
}
