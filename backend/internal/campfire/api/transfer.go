package api

import (
	"net/http"

	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func (a *API) transfers(w http.ResponseWriter) *service.TransferService {
	if a.deployment == nil || a.deployment.Transfers == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Spark transfers unavailable"})
		return nil
	}

	return a.deployment.Transfers
}

func (a *API) beginTransfer(w http.ResponseWriter, r *http.Request) {
	transfers := a.transfers(w)
	if transfers == nil {
		return
	}

	var input struct {
		TargetSparkID string             `json:"target_spark_id"`
		Ports         []model.ServerPort `json:"ports"`
	}
	if !decodeRequest(w, r, &input, 16384) {
		return
	}

	item, err := transfers.Begin(r.Context(), r.PathValue("id"), input.TargetSparkID, input.Ports)
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "server.transfer", "server:"+item.ServerID)

	job, err := transfers.Store.Jobs().HistoryItem(r.Context(), item.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	progress, err := a.publicJob(r, job)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, progress)
}
