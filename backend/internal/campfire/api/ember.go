package api

import (
	"campfire/internal/campfire/service"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (a *API) previewEmberURL(w http.ResponseWriter, r *http.Request) {
	var input struct {
		URL            string `json:"url"`
		StartupCommand string `json:"startup_command,omitempty"`
	}
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	ember, err := service.FetchEmberURL(r.Context(), input.URL, service.ImportOptions{StartupCommand: input.StartupCommand})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, ember)
}

func (a *API) importEmber(w http.ResponseWriter, r *http.Request) {
	data, options, ok := definitionRequest(w, r)
	if !ok {
		return
	}

	ember, err := a.embers.ImportDefinition(r.Context(), data, options)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/embers/"+ember.ID)
	auditDetail(r, "ember.import", "ember:"+ember.ID)
	writeJSON(w, http.StatusCreated, ember)
}

func (a *API) previewEmber(w http.ResponseWriter, r *http.Request) {
	data, options, ok := definitionRequest(w, r)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, service.PreviewEmber(data, options))
}

func definitionRequest(w http.ResponseWriter, r *http.Request) ([]byte, service.ImportOptions, bool) {
	var raw json.RawMessage
	if !decodeRequest(w, r, &raw, 2<<20) {
		return nil, service.ImportOptions{}, false
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		writeError(w, fmt.Errorf("%w: definition request must be an object", service.ErrInvalidEmber))
		return nil, service.ImportOptions{}, false
	}

	if _, wrapped := object["definition"]; !wrapped {
		return raw, service.ImportOptions{}, true
	}

	var input struct {
		Definition          string `json:"definition"`
		StartupCommand      string `json:"startup_command,omitempty"`
		AcknowledgeWarnings bool   `json:"acknowledge_warnings,omitempty"`
		SourceURL           string `json:"source_url,omitempty"`
	}

	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		writeError(w, fmt.Errorf("%w: invalid definition import request", service.ErrInvalidEmber))
		return nil, service.ImportOptions{}, false
	}

	return []byte(input.Definition), service.ImportOptions{StartupCommand: input.StartupCommand, AcknowledgeWarnings: input.AcknowledgeWarnings, SourceURL: input.SourceURL}, true
}

func (a *API) listEmbers(w http.ResponseWriter, r *http.Request) {
	embers, err := a.embers.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, embers)
}

func (a *API) getEmber(w http.ResponseWriter, r *http.Request) {
	ember, err := a.embers.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, ember)
}

func (a *API) deleteEmber(w http.ResponseWriter, r *http.Request) {
	if err := a.embers.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "ember.delete", "ember:"+r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}
