//go:build !production

package devmock

import (
	"testing"

	"campfire/internal/campfire/database"
)

func TestSessionFixturesPersistRevocation(t *testing.T) {
	_, db, auth, _ := fixture(t)
	_, token, err := auth.Login(t.Context(), Username, Password, "")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.Sessions(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	examples := 0
	for _, session := range sessions {
		if session.UserAgent != "" {
			examples++
			if session.Current || session.CreatedAt == 0 || session.LastSeenAt < session.CreatedAt {
				t.Fatal(session)
			}
		}
	}
	if examples != 3 {
		t.Fatalf("got %d example sessions", examples)
	}
	if err := auth.RevokeOtherSessions(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	if err := seedSessionFixtures(t.Context(), db.ORM()); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.ORM().Model(&database.UserSession{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("revoked fixtures recreated", count, err)
	}
}
