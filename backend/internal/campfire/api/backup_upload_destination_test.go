package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/devfixture"
	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
)

func TestExistingServerUploadOnlyFinalizesOnSpark(t *testing.T) {
	f := newAccessFixture(t)
	_, ember, configuration := serverFixture(t, f.admin)
	configuration.Variables = map[string]string{"SECONDS": "3600"}
	created := request(f.admin, "POST", "/api/v1/servers", configJSON(t, configuration))

	var server model.Server
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &server) != nil {
		t.Fatal(created.Code, created.Body)
	}

	manifest := backuparchive.Manifest{Format: backuparchive.Format, BackupOptions: model.BackupOptions{Mode: model.BackupModeFull, Definition: model.NewBackupDefinition(server, ember)}}

	metadata, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	data, err := devfixture.Archive(map[string]string{model.BackupMetadataName: string(metadata), "world.dat": "world"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	var (
		uploaded  bytes.Buffer
		published string
	)

	f.runtime.files = func(q model.FileRequest) (model.FileResult, error) {
		result := model.FileResult{ServerID: q.ServerID, Path: q.Path, UploadID: "upload"}
		switch q.Operation {
		case model.FileOperationBackupUploadChunk:
			if q.Offset != int64(uploaded.Len()) {
				t.Fatal("invalid chunk offset", q.Offset)
			}

			_, _ = uploaded.Write(q.Data)
		case model.FileOperationBackupUploadCommit:
			published = q.Path
		case model.FileOperationBackupList:
			if published != "" {
				result.Entries = []model.FileEntry{{Name: published, Kind: model.FileKindFile, Size: int64(len(data)), ModifiedAt: time.Now(), Portable: true, BackupMode: model.BackupModeFull}}
			}
		}

		return result, nil
	}
	for _, cancel := range []bool{true, false} {
		token := stagePortableBackup(t, f.admin, server.ID, data)

		preview := request(f.admin, "POST", "/api/v1/backup-imports/"+token+"/inspect", `{}`)
		if preview.Code != http.StatusOK || bytes.Contains(preview.Body.Bytes(), []byte(`"backup_id"`)) {
			t.Fatal("server upload finalized in panel library", preview.Code, preview.Body)
		}

		var count int64
		if err := f.db.ORM().Model(&database.Backup{}).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("inspection created permanent archive", count, err)
		}

		if !cancel {
			expectAccess(t, f.admin, "POST", "/api/v1/servers/"+server.ID+"/backup-imports/"+token, `{}`, http.StatusCreated)
			// Publication retries do not upload a second copy.
			expectAccess(t, f.admin, "POST", "/api/v1/servers/"+server.ID+"/backup-imports/"+token, `{}`, http.StatusCreated)
		}

		expectAccess(t, f.admin, "DELETE", "/api/v1/backup-imports/"+token, "", http.StatusNoContent)
	}

	var archives []database.Backup
	if err := f.db.ORM().Find(&archives).Error; err != nil || len(archives) != 1 {
		t.Fatal(archives, err)
	}

	saved := archives[0]
	if saved.ID == "" || saved.ServerID != server.ID || saved.LocalStore != "spark:"+server.SparkID || saved.OwnerUserID != nil || !bytes.Equal(uploaded.Bytes(), data) {
		t.Fatal("wrong final storage or uploaded bytes", saved)
	}

	expectAccess(t, f.admin, "POST", "/api/v1/backup-imports", `{"server_id":"`+server.ID+`","size":1,"store_only":true}`, http.StatusBadRequest)
}
