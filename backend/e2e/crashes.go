//go:build e2e && !production

package main

import (
	"encoding/json"
	"net/http"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func crashFixture(db *database.Database, w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost || req.Header.Get("X-Campfire-E2E") != "crash" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	var input struct {
		ServerID string            `json:"server_id"`
		Record   model.CrashRecord `json:"record"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 128<<10)).Decode(&input); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var server database.Server
	if err := db.ORM().Take(&server, "id = ?", input.ServerID).Error; err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err := repository.NewCrashRepository(db.ORM()).Observe(req.Context(), server.SparkID, server.ID, []model.CrashRecord{input.Record}); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
