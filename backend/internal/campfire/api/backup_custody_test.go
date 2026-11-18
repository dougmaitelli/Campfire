package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func TestArchiveMutationsCannotUseServerAuthorizationAfterDetachment(t *testing.T) {
	for _, method := range []string{"DELETE", "PATCH"} {
		t.Run(method, func(t *testing.T) {
			f := newAccessFixture(t)
			user, owner := f.user("former-owner", database.UserRole)
			_, _, config := serverFixture(t, f.admin)
			response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, config))

			var server model.Server
			if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
				t.Fatal(response.Code, response.Body)
			}

			expectAccess(t, f.admin, "PUT", "/api/v1/servers/"+server.ID+"/owner", fmt.Sprintf(`{"user_id":%d}`, user.ID), http.StatusOK)

			row := database.Backup{ServerID: server.ID, Name: "snapshot.tar.gz", Local: true, Size: 4, Key: "key", LocalStore: "spark:" + server.SparkID, LocalKey: server.ID + "/snapshot.tar.gz"}
			if err := f.db.ORM().Create(&row).Error; err != nil {
				t.Fatal(err)
			}

			queries := 0

			if err := f.db.ORM().Callback().Query().Before("gorm:query").Register("detach-after-authorization", func(tx *gorm.DB) {
				if tx.Statement.Table != "backups" {
					return
				}

				queries++
				if queries == 2 {
					// Complete the custody transition after middleware authorizes the old
					// server owner, but before the mutation reads/claims the archive.
					if err := repository.NewServerRepository(f.db.ORM()).Delete(t.Context(), server.ID); err != nil {
						t.Fatal(err)
					}
				}
			}); err != nil {
				t.Fatal(err)
			}

			deletes := 0
			f.runtime.files = func(q model.FileRequest) (model.FileResult, error) {
				if q.Operation == model.FileOperationBackupDelete {
					deletes++
				}

				return model.FileResult{}, nil
			}

			body := ""
			if method == "PATCH" {
				body = `{"pinned":true}`
			}

			result := request(owner, method, "/api/v1/backups/"+row.ID, body)
			if result.Code != http.StatusForbidden && result.Code != http.StatusNotFound && result.Code != http.StatusConflict {
				t.Fatal(result.Code, result.Body)
			}

			saved, err := repository.NewBackupRepository(f.db.ORM()).ByID(t.Context(), row.ID)
			if err != nil || deletes != 0 || saved.Pinned || saved.ServerID != "" {
				t.Fatalf("stale authorization mutated archive: %+v deletes=%d error=%v", saved, deletes, err)
			}
		})
	}
}
