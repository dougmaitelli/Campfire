//go:build !production

package integrations

import (
	"encoding/json"
	"net/http"
	"time"

	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"

	"gorm.io/gorm"
)

// DeliveryHandler publishes isolated events for browser tests of the real queue.
func DeliveryHandler(db *gorm.DB, now time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/__test/integrations/event" || r.Header.Get("X-Campfire-E2E") != "integration" {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		var event core.Event
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&event) != nil || event.Validate() != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := repository.NewIntegrationRepository(db).RouteEvent(r.Context(), event, now); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
