package database_test

import (
	"os"
	"path/filepath"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUpgradeSchema5PreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	legacy, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	ddl, err := os.ReadFile("testdata/schema5.sql")
	if err != nil {
		t.Fatal(err)
	}

	if err := legacy.Exec(string(ddl)).Error; err != nil {
		t.Fatal(err)
	}

	queries := []string{
		"INSERT INTO schema_migrations VALUES (5)",
		"INSERT INTO roles(id,name,scope,builtin) VALUES (1,'Admin','platform',true),(2,'Creator','platform',true),(3,'User','platform',true),(4,'Owner','server',true),(5,'Operator','server',true)",
		"INSERT INTO role_permissions VALUES (5,'server.view')",
		"INSERT INTO users(id,username,password_hash,version,role_id,created_at) VALUES (1,'owner','preserved',7,3,10),(2,'member','preserved',1,3,10)",
		"INSERT INTO user_sessions(hash,user_id,csrf,credential_version,expires_at,last_seen_at) VALUES ('session',1,'csrf',7,9999999999,10)",
		"INSERT INTO oidc_identities VALUES ('issuer','subject',1)",
		"INSERT INTO sparks(id,name,kind,created_at) VALUES ('node','Node','remote','2026-01-01T00:00:00Z')",
		`INSERT INTO embers(id,name,definition,created_at) VALUES ('ember','Game','{"id":"ember","schema":1,"name":"Game"}','2026-01-01T00:00:00Z')`,
		"INSERT INTO servers(id,name,spark_id,ember_id,image,variables,cpu_limit,memory_mb,disk_mb,ports,created_at) VALUES ('server','Server','node','ember','alpine','{}',1,128,1024,'[]','2026-01-01T00:00:00Z')",
		"INSERT INTO server_ownerships VALUES ('server',1)",
		"INSERT INTO server_members VALUES ('server',2,5)",
		"INSERT INTO schedules(id,server_id,name,action,command,next_run_at,enabled,user_id,username,created_at,version) VALUES ('nightly','server','Nightly','restart','',123,true,1,'owner',10,1)",
		"INSERT INTO schedule_runs(only_when_running,id,schedule_id,schedule_version,recurring,retry_of,server_id,server_name,user_id,username,action,content,state,message,created_at,started_at,finished_at,polled_at) VALUES (false,'manual-pending','',0,false,'','server','Server',1,'owner','command','pending-command','queued','',100,0,0,0),(false,'recurring-pending','nightly',1,true,'','server','Server',1,'owner','restart','','queued','',90,0,0,0),(false,'completed','nightly',1,true,'','server','Server',1,'owner','restart','','succeeded','',50,50,60,0)",
		"INSERT INTO audit_events(id,user_id,username,action,target,outcome,status,created_at) VALUES ('known',1,'owner','test','api','succeeded',200,10),('removed',999,'former','test','api','succeeded',200,10),('anonymous',0,'','test','api','failed',401,10)",
		"INSERT INTO spark_roles VALUES ('node',2)",
		"INSERT INTO spark_enrollments VALUES ('hashed-token','node',9999999999)",
		"INSERT INTO spark_identities(spark_id,certificate_hash) VALUES ('node','certificate')",
		"INSERT INTO ember_updates VALUES ('ember','https://example.test/ember',123,true,'')",
	}
	for _, query := range queries {
		if err := legacy.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}

	pool, err := legacy.DB()
	if err != nil {
		t.Fatal(err)
	}

	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var user database.User
	if err := db.ORM().First(&user, 1).Error; err != nil || user.CredentialVersion != 7 || user.PasswordHash != "preserved" {
		t.Fatal(user, err)
	}

	var server database.Server
	if err := db.ORM().Preload("Ember").Take(&server).Error; err != nil || server.OwnerUserID == nil || *server.OwnerUserID != 1 || server.EmberHash != model.EmberHash(server.Ember.Definition) {
		t.Fatal(server, err)
	}

	var spark database.Spark
	if err := db.ORM().Take(&spark).Error; err != nil || spark.EnrollmentTokenHash == nil || *spark.EnrollmentTokenHash != "hashed-token" || spark.EnrollmentExpiresAt != 9999999999 {
		t.Fatal(spark, err)
	}

	if !server.Ember.UpdateAvailable || server.Ember.UpdateCheckedAt != 123 {
		t.Fatal("lost update check", server.Ember)
	}

	for _, record := range []any{&database.UserSession{}, &database.OIDCIdentity{}, &database.SparkIdentity{}, &database.ServerAccessGrant{}, &database.SparkAccessGrant{}} {
		var count int64
		if err := db.ORM().Model(record).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("lost %T: %d %v", record, count, err)
		}
	}

	for _, table := range []string{"server_ownerships", "server_members", "spark_roles", "spark_enrollments", "ember_updates"} {
		if db.ORM().Migrator().HasTable(table) {
			t.Fatal("retained retired table", table)
		}
	}

	for _, column := range []struct{ table, name string }{{"users", "version"}, {"sparks", "kind"}, {"schedules", "username"}} {
		if db.ORM().Migrator().HasColumn(column.table, column.name) {
			t.Fatal("retained retired column", column)
		}
	}

	var schedule database.Schedule
	if err := db.ORM().Take(&schedule).Error; err != nil || schedule.RunAsUserID != 1 {
		t.Fatal("lost schedule execution identity", schedule, err)
	}

	var events []database.AuditEvent
	if err := db.ORM().Order("id").Find(&events).Error; err != nil || len(events) != 3 || events[0].UserID != nil || events[1].UserID == nil || *events[1].UserID != 1 || events[2].UserID != nil || events[2].Username != "former" {
		t.Fatal("lost legacy audit identity", events, err)
	}

	var pending database.Schedule
	if err := db.ORM().Take(&pending, "id = ?", "pending-manual-pending").Error; err != nil || pending.CronExpression != "" || pending.Command != "pending-command" || !pending.Enabled || pending.NextRunAt != 100 {
		t.Fatal("lost pending one-off definition", pending, err)
	}

	var runCount int64
	if err := db.ORM().Model(&database.ScheduleRun{}).Count(&runCount).Error; err != nil || runCount != 1 {
		t.Fatal("retained unstarted runs or lost completed history", runCount, err)
	}

	var versions []int
	if err := db.ORM().Table("schema_migrations").Pluck("version", &versions).Error; err != nil || len(versions) != 1 || versions[0] != 1 {
		t.Fatal("did not consolidate migration history", versions, err)
	}

	var enabled int
	if err := db.ORM().Raw("PRAGMA foreign_keys").Scan(&enabled).Error; err != nil || enabled != 1 {
		t.Fatal("foreign keys not restored", enabled, err)
	}
}

func TestConsolidateReviewedPrereleaseSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviewed.db")

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	user := database.User{Username: "retained", Email: "retained@example.test", CredentialVersion: 2, RoleID: database.UserRole}
	if err := db.ORM().Omit("Role").Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("ALTER TABLE schedules ADD COLUMN version INTEGER NOT NULL DEFAULT 1").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("UPDATE schema_migrations SET version = 6").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var retained database.User
	if err := db.ORM().Take(&retained, user.ID).Error; err != nil || retained.Email != user.Email || retained.CredentialVersion != user.CredentialVersion {
		t.Fatal(retained, err)
	}

	var versions []int
	if err := db.ORM().Table("schema_migrations").Pluck("version", &versions).Error; err != nil || len(versions) != 1 || versions[0] != 1 {
		t.Fatal(versions, err)
	}
}
