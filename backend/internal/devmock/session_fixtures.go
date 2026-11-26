//go:build !production

package devmock

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/limits"

	"gorm.io/gorm"
)

const sessionsSeedMarkerID = 18

// Seed once so revoking examples remains effective across mock restarts.
func seedSessionFixtures(ctx context.Context, db *gorm.DB) error {
	return database.Transaction(db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&seedMarker{}).Where("id = ?", sessionsSeedMarkerID).Count(&count).Error; err != nil || count > 0 {
			return err
		}

		var user database.User
		if err := tx.Where("username = ?", Username).Take(&user).Error; err != nil {
			return err
		}

		now := time.Now()

		agents := []string{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 14.0) Gecko/20100101 Firefox/132.0",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148 Safari/604.1",
		}
		for index, agent := range agents {
			// Discard random secrets: these display fixtures cannot be used to sign in.
			hash := sha256.Sum256([]byte(rand.Text()))
			created := now.Add(-time.Duration(index+1) * time.Hour)

			session := database.UserSession{Hash: hex.EncodeToString(hash[:]), UserID: user.ID, CSRF: rand.Text(), CredentialVersion: user.CredentialVersion, UserAgent: agent, CreatedAt: created.Unix(), LastSeenAt: now.Add(-time.Duration(index+1) * 2 * time.Minute).Unix(), ExpiresAt: created.Add(limits.SessionLifetime).Unix()}
			if err := tx.Omit("User").Create(&session).Error; err != nil {
				return err
			}
		}

		return tx.Create(&seedMarker{ID: sessionsSeedMarkerID}).Error
	})
}
