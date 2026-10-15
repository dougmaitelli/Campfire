package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func (a *API) streamConsole(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil {
		writeError(w, service.ErrDeploymentUnavailable)
		return
	}

	server, err := a.servers.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	events := make(chan model.ConsoleEvent, 16)
	go func() {
		defer close(events)

		ended := false

		_ = a.deployment.StreamConsole(ctx, server.ID, func(event model.ConsoleEvent) error {
			ended = event.End
			select {
			case events <- event:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if !ended && ctx.Err() == nil {
			select {
			case events <- model.ConsoleEvent{End: true, Error: "Console unavailable or interrupted. Reconnecting…"}:
			case <-ctx.Done():
			}
		}
	}()

	controller := http.NewResponseController(w)
	write := func(kind string, event model.ConsoleEvent) error {
		if err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}

		data, err := json.Marshal(event)
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data); err != nil {
			return err
		}

		return controller.Flush()
	}
	authorized := func() bool {
		checkCtx, stop := context.WithTimeout(ctx, 3*time.Second)
		defer stop()

		session, _, err := a.auth.Session(checkCtx, browserToken(r))
		if err != nil {
			return false
		}

		permissions, err := a.access.ServerPermissions(checkCtx, service.PrincipalFromSession(session), server.ID)
		if err != nil || !slices.Contains(permissions, "server.operate") {
			return false
		}

		current, err := a.servers.Get(checkCtx, server.ID)
		if err != nil || current.SparkID != server.SparkID {
			return false
		}

		node, err := a.sparks.Get(checkCtx, server.SparkID)

		return err == nil && !node.Disabled
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if err := write("connecting", model.ConsoleEvent{}); err != nil {
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	lastCheck := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !authorized() {
				_ = write("denied", model.ConsoleEvent{Error: "Console access ended."})
				return
			}

			lastCheck = time.Now()

			if err := write("heartbeat", model.ConsoleEvent{}); err != nil {
				return
			}
		case event, ok := <-events:
			if !ok {
				return
			}

			if time.Since(lastCheck) >= time.Second {
				if !authorized() {
					_ = write("denied", model.ConsoleEvent{Error: "Console access ended."})
					return
				}

				lastCheck = time.Now()
			}

			kind := "output"
			if event.Ready {
				kind = "ready"
			}

			if event.End {
				kind = "ended"
			}

			if err := write(kind, event); err != nil {
				return
			}
		}
	}
}
