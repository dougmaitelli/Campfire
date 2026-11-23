//go:build !production

package devmock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
	"campfire/internal/campfire/service/integrations"

	"gorm.io/gorm"
)

// These intentionally public credentials exist only in the isolated mock panel.
const MockMFASecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
const MockMFABackupSecret = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
const MockMFARecoveryCode = "11111111-22222222-33333333-44444444"
const mfaSeedMarkerID = 19

// EnableMFAExamples adds a protected account for testing login and administrator recovery.
// A persistent seed marker preserves administrator resets and enrollment changes.
func (r *Runtime) EnableMFAExamples(ctx context.Context, auth *service.AuthService, directory string) error {
	var count int64
	if err := r.db.WithContext(ctx).Model(&seedMarker{}).Where("id = ?", mfaSeedMarkerID).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	secrets := integrations.Secrets{Directory: filepath.Join(directory, "auth-secrets")}
	ref, err := secrets.Put(MockMFASecret)
	if err != nil {
		return err
	}
	// Use the real account creation path so fixture passwords follow production hashing.
	backupRef, err := secrets.Put(MockMFABackupSecret)
	if err != nil {
		return err
	}
	var user database.User
	if err := r.db.WithContext(ctx).Where("username = ?", "mfa-demo").Find(&user).Error; err != nil {
		return err
	}
	if user.ID == 0 {
		user, err = auth.Access().CreateUser(ctx, service.CreateUserInput{Username: "mfa-demo", Password: Password, RoleID: database.UserRole})
		if err != nil {
			return err
		}
	}
	digest := sha256.Sum256([]byte("11111111222222223333333344444444"))
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		factor := database.UserMFA{UserID: user.ID, SecretRef: ref, RecoveryHashes: []string{hex.EncodeToString(digest[:])}}
		factor.Devices = []database.MFADevice{
			{ID: "mock-phone", Name: "Personal phone", SecretRef: ref, CreatedAt: time.Now().Add(-48 * time.Hour).Unix()},
			{ID: "mock-backup", Name: "Backup tablet", SecretRef: backupRef, CreatedAt: time.Now().Add(-24 * time.Hour).Unix()},
		}
		if err := tx.Omit("User").Create(&factor).Error; err != nil {
			return err
		}
		return tx.Create(&seedMarker{ID: mfaSeedMarkerID}).Error
	})
}
