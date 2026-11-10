package api

import (
	"net/http"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service/integrations"
)

func (a *API) integrationAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).CanManageIntegrations() {
			writeError(w, repository.ErrAccessDenied)
			return
		}

		if a.integrations == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "integrations unavailable"})
			return
		}

		next(w, r)
	}
}

func (a *API) discordIntegration(w http.ResponseWriter, r *http.Request) {
	status, err := a.integrations.Status(r.Context(), currentUser(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (a *API) previewDiscordIntegration(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token string `json:"token"`
	}
	if !decodeRequest(w, r, &input, 2048) {
		return
	}

	preview, err := a.integrations.Preview(r.Context(), currentUser(r), input.Token)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

func (a *API) saveDiscordIntegration(w http.ResponseWriter, r *http.Request) {
	var input integrations.DiscordIntegrationInput
	if !decodeRequest(w, r, &input, 2048) {
		return
	}

	status, err := a.integrations.Save(r.Context(), currentUser(r), input)
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "integration.discord.save", "integrations:discord")
	writeJSON(w, http.StatusOK, status)
}

func (a *API) discordIntegrationChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := a.integrations.Channels(r.Context(), currentUser(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, channels)
}

func (a *API) removeDiscordIntegration(w http.ResponseWriter, r *http.Request) {
	if err := a.integrations.Remove(r.Context(), currentUser(r)); err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "integration.discord.remove", "integrations:discord")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) testDiscordIntegration(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ChannelID string `json:"channel_id"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if err := a.integrations.Test(r.Context(), currentUser(r), input.ChannelID); err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "integration.discord.test", "integrations:discord")
	w.WriteHeader(http.StatusNoContent)
}
