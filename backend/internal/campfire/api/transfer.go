package api

import (
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"net/http"
)

func (a *API) transfers(w http.ResponseWriter) *service.TransferService {
	if a.deployment == nil || a.deployment.Transfers == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Spark transfers unavailable"})
		return nil
	}

	return a.deployment.Transfers
}

func (a *API) listTransfers(w http.ResponseWriter, r *http.Request) {
	transfers := a.transfers(w)
	if transfers == nil {
		return
	}

	items, err := transfers.Store.List(r.Context(), r.PathValue("id"), false)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, items)
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
	writeJSON(w, http.StatusAccepted, item)
}

func (a *API) retryTransfer(w http.ResponseWriter, r *http.Request)  { a.changeTransfer(w, r, false) }
func (a *API) cancelTransfer(w http.ResponseWriter, r *http.Request) { a.changeTransfer(w, r, true) }

func (a *API) changeTransfer(w http.ResponseWriter, r *http.Request, cancel bool) {
	transfers := a.transfers(w)
	if transfers == nil {
		return
	}

	var input struct{}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	item, err := transfers.Store.Get(r.Context(), r.PathValue("transferID"))
	if err != nil {
		writeError(w, err)
		return
	}

	action := "retry"
	if cancel {
		action = "cancel"
		err = transfers.Store.Cancel(r.Context(), item.ID)
	} else {
		err = transfers.Store.Retry(r.Context(), item.ID)
	}

	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "server.transfer."+action, "server:"+item.ServerID)

	updated, err := transfers.Store.Get(r.Context(), item.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, updated)
}
