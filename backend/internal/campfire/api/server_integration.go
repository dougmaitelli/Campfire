package api

import (
	"net/http"
	"slices"
	"strconv"

	core "campfire/internal/campfire/integrations"
	"campfire/internal/campfire/service/integrations"
	"campfire/internal/campfire/service/integrations/discord"
	"campfire/internal/shared/model"
)

func (a *API) serverDiscordCard(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.integrationServerPermissions(w, r); !ok {
		return
	}

	settings := core.CardSettings{Enabled: true, Fields: r.URL.Query()["field"]}
	if settings.Validate() != nil {
		writeError(w, integrations.ErrIntegrationInput)
		return
	}

	if a.deployment == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "server-card preview unavailable"})
		return
	}

	card, err := a.deployment.IntegrationCard(r.Context(), r.PathValue("id"), settings)
	if err != nil {
		writeError(w, err)
		return
	}

	expired := card
	expired.State = model.RuntimeStateUnknown

	expired.Fields = slices.Clone(card.Fields)
	for i := range expired.Fields {
		expired.Fields[i].Available = false
	}

	writeJSON(w, http.StatusOK, struct {
		core.ServerCard
		Preview        discord.CardMessage `json:"preview"`
		ExpiredPreview discord.CardMessage `json:"expired_preview"`
	}{card, discord.PreviewServerCard(card), discord.PreviewServerCard(expired)})
}

func (a *API) retryServerDiscordCard(w http.ResponseWriter, r *http.Request) {
	permissions, ok := a.integrationServerPermissions(w, r)
	if !ok {
		return
	}

	var input struct{}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if err := a.integrations.RetryServerCard(r.Context(), r.PathValue("id"), r.PathValue("bindingID"), permissions); err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "server.integration.discord.card.retry", "server:"+r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) integrationServerPermissions(w http.ResponseWriter, r *http.Request) ([]model.PermissionID, bool) {
	if a.integrations == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "integrations unavailable"})
		return nil, false
	}

	permissions, err := a.access.ServerPermissions(r.Context(), currentUser(r), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return nil, false
	}

	return permissions, true
}

func (a *API) serverDiscordMessages(w http.ResponseWriter, r *http.Request) {
	permissions, ok := a.integrationServerPermissions(w, r)
	if !ok {
		return
	}

	before := int64(0)

	if raw := r.URL.Query().Get("before"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 1 {
			writeError(w, integrations.ErrIntegrationInput)
			return
		}

		before = value
	}

	items, err := a.integrations.ServerMessages(r.Context(), r.PathValue("id"), r.PathValue("bindingID"), permissions, before, r.URL.Query().Get("state"))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (a *API) retryServerDiscordMessage(w http.ResponseWriter, r *http.Request) {
	permissions, ok := a.integrationServerPermissions(w, r)
	if !ok {
		return
	}

	var input struct{}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	id, err := strconv.ParseInt(r.PathValue("messageID"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, integrations.ErrIntegrationInput)
		return
	}

	if err := a.integrations.RetryServerMessage(r.Context(), r.PathValue("id"), r.PathValue("bindingID"), permissions, id); err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "server.integration.discord.retry", "server:"+r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) serverDiscordIntegration(w http.ResponseWriter, r *http.Request) {
	permissions, ok := a.integrationServerPermissions(w, r)
	if !ok {
		return
	}

	settings, err := a.integrations.ServerSettings(r.Context(), r.PathValue("id"), r.PathValue("bindingID"), permissions)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

func (a *API) serverDiscordChannels(w http.ResponseWriter, r *http.Request) {
	permissions, ok := a.integrationServerPermissions(w, r)
	if !ok {
		return
	}

	channels, err := a.integrations.ServerChannels(r.Context(), r.PathValue("id"), r.URL.Query().Get("integration_id"), permissions)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, channels)
}

func (a *API) saveServerDiscordIntegration(w http.ResponseWriter, r *http.Request) {
	permissions, ok := a.integrationServerPermissions(w, r)
	if !ok {
		return
	}

	var input integrations.ServerIntegrationInput
	if !decodeRequest(w, r, &input, 32<<10) {
		return
	}

	settings, err := a.integrations.SaveServerSettings(r.Context(), r.PathValue("id"), r.PathValue("bindingID"), permissions, input)
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "server.integration.discord.save", "server:"+r.PathValue("id"))

	status := http.StatusOK
	if r.Method == http.MethodPost {
		status = http.StatusCreated
	}

	writeJSON(w, status, settings)
}

func (a *API) saveServerDiscordIntegrationChanges(w http.ResponseWriter, r *http.Request) {
	permissions, ok := a.integrationServerPermissions(w, r)
	if !ok {
		return
	}

	var input integrations.ServerIntegrationChanges
	if !decodeRequest(w, r, &input, 4<<20) {
		return
	}

	settings, err := a.integrations.SaveServerIntegrationChanges(r.Context(), r.PathValue("id"), permissions, input)
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "server.integration.discord.save", "server:"+r.PathValue("id"))
	writeJSON(w, http.StatusOK, settings)
}

func (a *API) serverDiscordIntegrations(w http.ResponseWriter, r *http.Request) {
	permissions, ok := a.integrationServerPermissions(w, r)
	if !ok {
		return
	}

	items, err := a.integrations.ListServerIntegrations(r.Context(), r.PathValue("id"), permissions)
	if err != nil {
		writeError(w, err)
		return
	}

	var labels model.GameFieldLabels
	if a.deployment != nil && a.deployment.Games != nil {
		labels, err = a.deployment.Games.StateLabels(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, struct {
		integrations.ServerIntegrationList
		FieldLabels model.GameFieldLabels `json:"field_labels,omitempty"`
	}{items, labels})
}

func (a *API) removeServerDiscordIntegration(w http.ResponseWriter, r *http.Request) {
	permissions, ok := a.integrationServerPermissions(w, r)
	if !ok {
		return
	}

	if err := a.integrations.RemoveServerIntegration(r.Context(), r.PathValue("id"), r.PathValue("bindingID"), permissions); err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "server.integration.discord.remove", "server:"+r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}
