package agents

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"campfire/internal/shared/agentidentity"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

// WithGameMedia installs the binary ingestion boundary on the authenticated listener.
func (h *Hub) WithGameMedia(receive func(context.Context, string, string, string, model.GameMedia, io.Reader) error) *Hub {
	h.receiveMedia = receive
	return h
}

func (h *Hub) gameMedia(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
		http.Error(w, "verified agent certificate required", http.StatusUnauthorized)
		return
	}

	leaf := r.TLS.PeerCertificates[0]

	sparkID := leaf.Subject.CommonName
	if !h.store.Authorized(r.Context(), sparkID, agentidentity.Fingerprint(leaf.Raw)) {
		http.Error(w, "unknown or disabled Spark", http.StatusUnauthorized)
		return
	}

	if h.receiveMedia == nil || h.CheckCapabilities(sparkID, protocol.CapabilityGameMedia) != nil {
		http.Error(w, "media ingestion unavailable or not negotiated", http.StatusServiceUnavailable)
		return
	}

	if r.ContentLength > model.GameMediaFileLimit {
		http.Error(w, "media file too large", http.StatusRequestEntityTooLarge)
		return
	}

	media := model.GameMedia{ID: r.PathValue("attachment"), Name: r.Header.Get("X-Campfire-Media-Name"), ContentType: r.Header.Get("Content-Type")}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Minute))
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2*time.Minute + 5*time.Second))
	r.Body = http.MaxBytesReader(w, r.Body, model.GameMediaFileLimit+1)
	err := h.receiveMedia(ctx, sparkID, r.PathValue("server"), r.PathValue("event"), media, r.Body)
	status := http.StatusServiceUnavailable

	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
		return
	case errors.Is(err, model.ErrGameMediaInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, model.ErrGameMediaLimit):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, model.ErrGameMediaConflict), errors.Is(err, model.ErrGameMediaEventMissing):
		status = http.StatusConflict
	case errors.Is(err, model.ErrGameMediaPlacement):
		status = http.StatusForbidden
	case errors.Is(err, model.ErrGameMediaBusy):
		status = http.StatusTooManyRequests
	}

	if status == http.StatusServiceUnavailable || status == http.StatusTooManyRequests || errors.Is(err, model.ErrGameMediaEventMissing) {
		w.Header().Set("Retry-After", "5")
	}

	message := "media ingestion temporarily unavailable"
	if status != http.StatusServiceUnavailable {
		message = err.Error()
	}

	http.Error(w, message, status)
}
