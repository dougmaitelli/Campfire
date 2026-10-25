package agent

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"campfire/internal/shared/model"
)

type gameMediaSender func(context.Context, string, string, model.GameMedia, io.Reader) (*http.Response, error)

func (g *gameIntegrations) mediaUpload(w http.ResponseWriter, r *http.Request, serverID string) {
	if !g.authorize(serverID) {
		http.Error(w, "invalid game integration server", http.StatusForbidden)
		return
	}

	media := model.GameMedia{ID: r.PathValue("attachment"), Name: r.Header.Get("X-Campfire-Media-Name"), ContentType: r.Header.Get("Content-Type")}

	eventID := r.PathValue("event")
	if !media.Valid() || eventID == "" || len(eventID) > 128 {
		http.Error(w, "invalid media metadata", http.StatusBadRequest)
		return
	}

	if r.ContentLength > model.GameMediaFileLimit {
		http.Error(w, "media file too large", http.StatusRequestEntityTooLarge)
		return
	}

	select {
	case g.mediaSlots <- struct{}{}:
		defer func() { <-g.mediaSlots }()
	default:
		w.Header().Set("Retry-After", "5")
		http.Error(w, "media uploads busy", http.StatusTooManyRequests)

		return
	}

	g.mu.Lock()
	session := g.session

	var upload gameMediaSender
	if session != nil {
		upload = session.upload
	}
	g.mu.Unlock()

	if session == nil || session.ctx.Err() != nil || upload == nil {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "media requires a connected Campfire with media support", http.StatusServiceUnavailable)

		return
	}
	// Only this upload request receives a larger deadline than JSON publications.
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Minute))
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2*time.Minute + 5*time.Second))

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	stop := context.AfterFunc(session.ctx, cancel)
	defer stop()

	response, err := upload(ctx, serverID, eventID, media, io.LimitReader(r.Body, model.GameMediaFileLimit+1))
	if err != nil {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "media upload interrupted; retry with the same attachment ID", http.StatusServiceUnavailable)

		return
	}

	defer func() { _ = response.Body.Close() }()

	if retry := response.Header.Get("Retry-After"); retry != "" {
		w.Header().Set("Retry-After", retry)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(response.Body, 4096))
}

func mediaSender(address string, tlsConfig *tls.Config, dial func(context.Context, string, string) (net.Conn, error)) (gameMediaSender, func()) {
	transport := &http.Transport{TLSClientConfig: tlsConfig, DialContext: dial, MaxConnsPerHost: 2, ResponseHeaderTimeout: 2 * time.Minute}
	client := &http.Client{Transport: transport, Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	origin := strings.TrimSuffix(strings.Replace(address, "wss://", "https://", 1), "/spark/connect")

	return func(ctx context.Context, serverID, eventID string, media model.GameMedia, body io.Reader) (*http.Response, error) {
		path := origin + "/spark/game-media/" + url.PathEscape(serverID) + "/" + url.PathEscape(eventID) + "/" + url.PathEscape(media.ID)

		request, err := http.NewRequestWithContext(ctx, http.MethodPut, path, body)
		if err != nil {
			return nil, err
		}

		request.Header.Set("Content-Type", media.ContentType)
		request.Header.Set("X-Campfire-Media-Name", media.Name)

		return client.Do(request)
	}, transport.CloseIdleConnections
}
