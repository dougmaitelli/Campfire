package api

import (
	"bytes"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"campfire/internal/shared/model"
)

func (a *API) gameMedia(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil || a.deployment.Games == nil || a.deployment.Games.Media == nil {
		http.Error(w, "game media unavailable", http.StatusServiceUnavailable)
		return
	}

	eventID, err := strconv.ParseInt(r.PathValue("event"), 10, 64)
	if err != nil || eventID < 1 {
		http.NotFound(w, r)
		return
	}

	media, err := a.deployment.Games.Media.Store.Get(r.Context(), r.PathValue("id"), eventID, r.PathValue("attachment"))
	if errors.Is(err, model.ErrGameMediaNotFound) {
		http.NotFound(w, r)
		return
	}

	if err != nil {
		writeError(w, err)
		return
	}

	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}

	w.Header().Set("Content-Type", media.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": media.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	http.ServeContent(w, r, media.Name, time.Time{}, bytes.NewReader(media.Data))
}
