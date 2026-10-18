package repository_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestCertificateRenewalOverlapRollbackAndRevocation(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(ctx, testDatabaseSource(t))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	nodes := repository.NewSparkRepository(db.ORM())
	if err := nodes.Create(ctx, model.Spark{ID: "node", Name: "Node", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	store := repository.NewEnrollmentRepository(db.ORM())
	if err := store.CreateToken(ctx, "node", "token", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Redeem(ctx, "token", func(string) (string, error) { return "old-hash", nil }); err != nil {
		t.Fatal(err)
	}

	signingError := errors.New("signing unavailable")
	if _, err := store.Renew(ctx, "node", "old-hash", time.Now().Add(24*time.Hour), func() ([]byte, string, error) { return nil, "", signingError }); !errors.Is(err, signingError) {
		t.Fatal(err)
	}

	if !store.Authorized(ctx, "node", "old-hash") {
		t.Fatal("failed renewal revoked old certificate")
	}

	certificate := []byte("signed public certificate")

	if _, err := store.Renew(ctx, "node", "old-hash", time.Now().Add(24*time.Hour), func() ([]byte, string, error) { return certificate, "new-hash", nil }); err != nil {
		t.Fatal(err)
	}

	if !store.Authorized(ctx, "node", "old-hash") || !store.Authorized(ctx, "node", "new-hash") {
		t.Fatal("renewal overlap missing")
	}

	repeated, err := store.Renew(ctx, "node", "old-hash", time.Now().Add(24*time.Hour), func() ([]byte, string, error) { t.Fatal("retry reissued a certificate"); return nil, "", nil })
	if err != nil || !bytes.Equal(repeated, certificate) {
		t.Fatal("lost response could not be recovered", err)
	}

	if err := db.ORM().Model(&database.SparkIdentity{}).Where("spark_id = ?", "node").Update("previous_expires_at", time.Now().Add(-time.Second).Unix()).Error; err != nil {
		t.Fatal(err)
	}

	if store.Authorized(ctx, "node", "old-hash") {
		t.Fatal("old certificate remained authorized after overlap")
	}

	if _, err := store.Renew(ctx, "node", "old-hash", time.Now().Add(time.Hour), func() ([]byte, string, error) { t.Fatal("expired overlap reached signer"); return nil, "", nil }); err == nil {
		t.Fatal("expired overlap renewed")
	}

	disabled := true
	if _, err := nodes.Update(ctx, "node", model.SparkChanges{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}

	if store.Authorized(ctx, "node", "new-hash") {
		t.Fatal("disabled Spark remained authorized")
	}

	if _, err := store.Renew(ctx, "node", "new-hash", time.Now().Add(time.Hour), func() ([]byte, string, error) { t.Fatal("disabled Spark reached signer"); return nil, "", nil }); err == nil {
		t.Fatal("disabled Spark renewed")
	}

	disabled = false
	if _, err := nodes.Update(ctx, "node", model.SparkChanges{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateToken(ctx, "node", "replacement-token", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Redeem(ctx, "replacement-token", func(string) (string, error) { return "replacement-hash", nil }); err != nil {
		t.Fatal(err)
	}

	if store.Authorized(ctx, "node", "new-hash") || store.Authorized(ctx, "node", "old-hash") {
		t.Fatal("re-enrollment retained previous credentials")
	}

	if err := nodes.Delete(ctx, "node"); err != nil {
		t.Fatal(err)
	}

	if store.Authorized(ctx, "node", "replacement-hash") {
		t.Fatal("removed Spark remained authorized")
	}
}
