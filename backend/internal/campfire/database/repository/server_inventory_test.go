package repository_test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func TestVisibleInventoryBatchesGrantsAndRedactsHiddenValues(t *testing.T) {
	db, auth, _ := authStore(t)
	for _, user := range []database.User{{ID: 10, Username: "member", RoleID: database.UserRole}, {ID: 11, Username: "other", RoleID: database.UserRole}} {
		if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}

	ember := model.Ember{Schema: 1, Variables: []model.Variable{{Name: "PUBLIC"}, {Name: "SECRET", Hidden: true}}}
	for _, row := range []any{&database.Spark{ID: "node", Name: "Node"}, &database.Ember{ID: "ember", Name: "Game", Definition: ember}} {
		if err := db.ORM().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	rows := make([]database.Server, 200)

	grants := make([]database.ServerAccessGrant, 0, len(rows)/2)
	for i := range rows {
		owner := int64(10)
		if i%2 != 0 {
			owner = 11
		}

		rows[i] = database.Server{ID: fmt.Sprint("server-", i), Name: fmt.Sprint("Server ", i), SparkID: "node", EmberID: "ember", OwnerUserID: &owner, Variables: map[string]string{"PUBLIC": "value", "SECRET": "hidden"}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if owner == 11 {
			grant := database.ServerAccessGrant{ServerID: rows[i].ID, AccessRoleID: database.OperatorRole}
			if i == 1 {
				grant.UserID = new(int64(10))
			} else {
				grant.RoleID = new(database.UserRole)
			}

			grants = append(grants, grant)
		}
	}

	if err := db.ORM().Omit("Spark", "Ember", "Owner").CreateInBatches(rows, 50).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Omit("User", "Role", "AccessRole").CreateInBatches(grants, 50).Error; err != nil {
		t.Fatal(err)
	}
	// Inaccessible records must be excluded before display metadata is decoded.
	if err := db.ORM().Omit("Spark", "Ember", "Owner").Create(&database.Server{ID: "unassigned", Name: "Unassigned", SparkID: "node", EmberID: "ember", CreatedAt: "invalid"}).Error; err != nil {
		t.Fatal(err)
	}

	var reads atomic.Int64

	count := func(*gorm.DB) { reads.Add(1) }
	if err := db.ORM().Callback().Query().After("gorm:query").Register("inventory_count", count); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Callback().Row().After("gorm:row").Register("inventory_count", count); err != nil {
		t.Fatal(err)
	}

	store := repository.NewServerRepository(db.ORM())

	servers, err := store.ListVisible(t.Context(), 10, false)
	if err != nil || len(servers) != len(rows) {
		t.Fatal("visible inventory missing assigned servers", len(servers), err)
	}

	access := service.NewAuthService(auth).Access()
	if err := access.ApplyServerPermissions(t.Context(), service.Principal{UserID: 10}, servers); err != nil {
		t.Fatal(err)
	}

	if reads.Load() > 10 {
		t.Fatal("inventory reads grew with server count", reads.Load())
	}

	for _, server := range servers {
		if len(server.Permissions) == 0 || server.CanManageAccess != (server.OwnerUserID == 10) || len(server.Variables) != 1 || server.Variables["PUBLIC"] != "value" {
			t.Fatal("inventory changed access or exposed hidden variables", server.ID)
		}
	}
}
