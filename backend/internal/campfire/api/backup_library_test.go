package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	accessperm "campfire/internal/campfire/permissions"
	"campfire/internal/devfixture"
	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
)

func TestUploadedArchiveSurvivesWizardCancellationAndCreatesServer(t *testing.T) {
	f := newAccessFixtureWithAudit(t, true)
	_, owner := f.user("archive-owner", database.CreatorRole)
	outsiderUser, outsider := f.user("archive-outsider", database.UserRole)
	_, ember, configuration := serverFixture(t, f.admin)
	configuration.Variables = map[string]string{"SECONDS": "3600"}
	manifest := backuparchive.Manifest{Format: backuparchive.Format, BackupOptions: model.BackupOptions{Mode: model.BackupModeFull, Definition: model.NewBackupDefinition(model.Server{ServerConfiguration: configuration}, ember)}}

	metadata, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	data, err := devfixture.Archive(map[string]string{model.BackupMetadataName: string(metadata), "world.dat": "world"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	token := stagePortableBackup(t, owner, "", data)
	preview := request(owner, "POST", "/api/v1/backup-imports/"+token+"/inspect", `{}`)

	var inspected struct {
		BackupID string `json:"backup_id"`
	}
	if preview.Code != http.StatusOK || json.Unmarshal(preview.Body.Bytes(), &inspected) != nil || inspected.BackupID == "" {
		t.Fatal(preview.Code, preview.Body)
	}

	endpoint := "/api/v1/backups/" + inspected.BackupID

	expectAccess(t, owner, "DELETE", "/api/v1/backup-imports/"+token, "", http.StatusNoContent)

	downloaded := request(owner, "GET", endpoint, "")
	if downloaded.Code != http.StatusOK || !bytes.Equal(downloaded.Body.Bytes(), data) {
		t.Fatal("wizard cancellation removed archive", downloaded.Code, downloaded.Body)
	}

	assertAuditOutcome(t, f, "server.backup.download", model.AuditSucceeded, "backup:"+inspected.BackupID)

	var chunks int64
	if err := f.db.ORM().Model(&database.AuditEvent{}).Where("action = ?", "backup.import.chunk").Count(&chunks).Error; err != nil || chunks != 0 {
		t.Fatal("successful backup chunks were audited", chunks, err)
	}

	expectAccess(t, outsider, "GET", endpoint, "", http.StatusForbidden)
	assertAuditOutcome(t, f, "server.backup.download", model.AuditFailed, "backup:"+inspected.BackupID)
	expectAccess(t, outsider, "PATCH", endpoint, `{"pinned":true}`, http.StatusForbidden)
	_, view := previewAs(t, f.admin, outsiderUser.ID)
	expectAccess(t, view, "GET", endpoint, "", http.StatusForbidden)

	scopes, _ := json.Marshal(accessperm.Expand([]model.PermissionID{accessperm.ServerBackupsRead}))
	_, secret := createTestAPIToken(t, f.admin, string(scopes))
	expectAccess(t, bearerAPI(f.panel, secret), "GET", endpoint, "", http.StatusForbidden)
	// The library entry has no parent and exposes no storage locator.
	catalog := request(owner, "GET", "/api/v1/backups", "")

	var entries []model.CatalogBackup
	if catalog.Code != http.StatusOK || json.Unmarshal(catalog.Body.Bytes(), &entries) != nil || len(entries) != 1 || entries[0].ID != inspected.BackupID || entries[0].ServerID != "" || entries[0].ServerName != "" {
		t.Fatal(catalog.Code, catalog.Body)
	}

	if bytes.Contains(catalog.Body.Bytes(), []byte("local_key")) || bytes.Contains(catalog.Body.Bytes(), []byte("local_store")) {
		t.Fatal("catalog exposed private storage locator")
	}

	prepared := request(f.admin, "POST", endpoint+"/prepare", `{}`)

	var source struct {
		Token string `json:"token"`
	}
	if prepared.Code != http.StatusOK || json.Unmarshal(prepared.Body.Bytes(), &source) != nil || source.Token == "" {
		t.Fatal(prepared.Code, prepared.Body)
	}

	configuration.Name = "Created from library"
	input, _ := json.Marshal(struct {
		model.ServerConfiguration
		BackupToken string `json:"backup_token"`
	}{configuration, source.Token})
	created := request(f.admin, "POST", "/api/v1/servers", string(input))

	var server model.Server
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &server) != nil {
		t.Fatal(created.Code, created.Body)
	}

	repeated := request(f.admin, "POST", "/api/v1/servers", string(input))

	var again model.Server
	if repeated.Code != http.StatusCreated || json.Unmarshal(repeated.Body.Bytes(), &again) != nil || again.ID != server.ID {
		t.Fatal("creation retry changed identity", repeated.Code, repeated.Body)
	}

	expectAccess(t, f.admin, "DELETE", "/api/v1/backup-imports/"+source.Token, "", http.StatusNoContent)

	downloaded = request(owner, "GET", endpoint, "")
	if downloaded.Code != http.StatusOK || !bytes.Equal(downloaded.Body.Bytes(), data) {
		t.Fatal("creation consumed source archive")
	}

	expectAccess(t, owner, "PATCH", endpoint, `{"pinned":true,"display_name":"Saved world"}`, http.StatusOK)
	expectAccess(t, owner, "DELETE", endpoint, "", http.StatusConflict)
	expectAccess(t, owner, "PATCH", endpoint, `{"pinned":false}`, http.StatusOK)
	expectAccess(t, owner, "DELETE", endpoint, "", http.StatusAccepted)
	expectAccess(t, owner, "GET", endpoint, "", http.StatusNotFound)
}

func TestDeletingServerDetachesArchivesWithoutFormerMemberAccess(t *testing.T) {
	f := newAccessFixture(t)
	user, owner := f.user("former-owner", database.UserRole)
	_, _, config := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, config))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	serverURL := "/api/v1/servers/" + server.ID
	expectAccess(t, f.admin, "PUT", serverURL+"/owner", fmt.Sprintf(`{"user_id":%d}`, user.ID), http.StatusOK)
	f.runtime.files = func(q model.FileRequest) (model.FileResult, error) {
		if q.Operation == model.FileOperationBackupList {
			return model.FileResult{ServerID: q.ServerID, Entries: []model.FileEntry{{Name: "snapshot.tar.gz", Size: 4, ModifiedAt: time.Now(), Kind: model.FileKindFile}}}, nil
		}

		return model.FileResult{ServerID: q.ServerID, Path: q.Path, Size: 4, Data: []byte("kept")}, nil
	}

	expectAccess(t, owner, "GET", serverURL+"/backups", "", http.StatusOK)

	var rows []database.Backup
	if err := f.db.ORM().Find(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}

	endpoint := "/api/v1/backups/" + rows[0].ID
	expectAccess(t, owner, "PATCH", endpoint, `{"pinned":true}`, http.StatusOK)
	expectAccess(t, f.admin, "DELETE", serverURL, "", http.StatusNoContent)
	expectAccess(t, owner, "GET", endpoint, "", http.StatusForbidden)
	expectAccess(t, owner, "PATCH", endpoint, `{"pinned":false}`, http.StatusForbidden)
	expectAccess(t, f.admin, "GET", endpoint, "", http.StatusOK)

	catalog := request(owner, "GET", "/api/v1/backups", "")

	var entries []model.CatalogBackup
	if catalog.Code != http.StatusOK || json.Unmarshal(catalog.Body.Bytes(), &entries) != nil || len(entries) != 0 {
		t.Fatal("former owner retained archive access", catalog.Code, catalog.Body)
	}

	catalog = request(f.admin, "GET", "/api/v1/backups", "")
	if catalog.Code != http.StatusOK || json.Unmarshal(catalog.Body.Bytes(), &entries) != nil || len(entries) != 1 || entries[0].ServerID != "" || entries[0].LocalSparkID != server.SparkID || entries[0].LocalSparkName == "" {
		t.Fatal("current storage location missing or confused with association", catalog.Code, catalog.Body)
	}

	if bytes.Contains(catalog.Body.Bytes(), []byte("local_key")) || bytes.Contains(catalog.Body.Bytes(), []byte("local_store")) {
		t.Fatal("private storage keys exposed", catalog.Body)
	}

	blocked := request(f.admin, "DELETE", "/api/v1/sparks/"+server.SparkID, "")
	if blocked.Code != http.StatusConflict || !bytes.Contains(blocked.Body.Bytes(), []byte(server.SparkID)) || bytes.Contains(blocked.Body.Bytes(), []byte(rows[0].ID)) {
		t.Fatal("missing storage filter or leaked archive identity", blocked.Code, blocked.Body)
	}

	var detached int64
	if err := f.db.ORM().Model(&database.Backup{}).Where("id = ? AND server_id IS NULL AND pinned = true", rows[0].ID).Count(&detached).Error; err != nil || detached != 1 {
		t.Fatal("association not detached atomically", err)
	}

	expectAccess(t, f.admin, "DELETE", endpoint, "", http.StatusConflict)
}
