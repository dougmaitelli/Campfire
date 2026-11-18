package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func TestIndependentCatalogReusesPermissionsWithoutSharingCustody(t *testing.T) {
	f := newAccessFixture(t)
	owner, client := f.user("library-owner", database.CreatorRole)
	other, _ := f.user("other-library-owner", database.CreatorRole)
	roles := 0

	if err := f.db.ORM().Callback().Query().Before("gorm:query").Register("count-catalog-role-reads", func(tx *gorm.DB) {
		if tx.Statement.Table == "roles" {
			roles++
		}
	}); err != nil {
		t.Fatal(err)
	}

	response := request(f.admin, "GET", "/api/v1/backups", "")
	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body)
	}

	baseline := roles
	rows := make([]database.Backup, 1000)
	owned := 0

	for i := range rows {
		rows[i] = database.Backup{Name: "snapshot.tar.gz", Local: true, LocalStore: "panel", Size: 4}
		switch i % 3 {
		case 1:
			rows[i].OwnerUserID = &owner.ID
			owned++
		case 2:
			rows[i].OwnerUserID = &other.ID
		}
	}

	if err := f.db.ORM().Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	roles = 0
	response = request(f.admin, "GET", "/api/v1/backups", "")

	var catalog []model.CatalogBackup
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &catalog) != nil || len(catalog) != len(rows) {
		t.Fatal(response.Code, response.Body)
	}

	if roles > baseline+1 {
		t.Fatalf("role reads grew per archive: baseline=%d catalog=%d", baseline, roles)
	}

	response = request(client, "GET", "/api/v1/backups", "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &catalog) != nil || len(catalog) != owned {
		t.Fatal("cached permissions crossed custody", response.Code, response.Body)
	}

	allowed := make(map[string]bool)

	for _, row := range rows {
		if row.OwnerUserID != nil && *row.OwnerUserID == owner.ID {
			allowed[row.ID] = true
		}
	}

	for _, row := range catalog {
		if !allowed[row.ID] {
			t.Fatal("foreign archive exposed", row.ID)
		}
	}
	// Changing the role between requests must not leave a stale permission cache.
	if err := f.db.ORM().Where("role_id = ?", database.OwnerRole).Delete(&database.RolePermission{}).Error; err != nil {
		t.Fatal(err)
	}

	response = request(f.admin, "GET", "/api/v1/backups", "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &catalog) != nil || len(catalog) != 0 {
		t.Fatal("permission cache outlived request", response.Code, response.Body)
	}
}
