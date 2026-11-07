package api

import (
	"net/http"
	"strconv"
	"strings"

	"campfire/internal/campfire/database/repository"
)

func (a *API) gameEventFilterOptions(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil || a.deployment.Games == nil || a.deployment.Games.Events == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "game events unavailable"})
		return
	}

	options, err := a.deployment.Games.Events.Store.FilterOptions(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, options)
}

func (a *API) gameEvents(w http.ResponseWriter, r *http.Request) {
	if a.deployment == nil || a.deployment.Games == nil || a.deployment.Games.Events == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "game events unavailable"})
		return
	}

	query := repository.GameEventQuery{Limit: 50, Type: r.URL.Query().Get("type"), Search: r.URL.Query().Get("search"), Fields: map[string]string{}}

	for name, values := range r.URL.Query() {
		if !strings.HasPrefix(name, "data.") {
			continue
		}

		if len(values) != 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "use each game event field filter once"})
			return
		}

		query.Fields[strings.TrimPrefix(name, "data.")] = values[0]
	}

	for name, target := range map[string]*int64{"before": &query.Before, "from": &query.From, "to": &query.To} {
		if raw := r.URL.Query().Get(name); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 1 || (name != "before" && value > 253402300799) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid game event cursor or time range"})
				return
			}

			*target = value
		}
	}

	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event limit must be 1–100"})
			return
		}

		query.Limit = limit
	}

	if query.From > 0 && query.To > 0 && query.From >= query.To || !query.ValidFilters() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid game event filter"})
		return
	}

	page, err := a.deployment.Games.Events.Store.List(r.Context(), r.PathValue("id"), query)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, page)
}
