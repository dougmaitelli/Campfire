package api

import (
	"net/http"
	"slices"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
)

// editorEmbers excludes hidden defaults from the server creation/editing API.
// Full definitions remain available through the Ember management API.
func editorEmbers(embers []model.Ember) []model.Ember {
	result := slices.Clone(embers)
	for i := range result {
		result[i].Variables = slices.DeleteFunc(slices.Clone(result[i].Variables), func(v model.Variable) bool { return v.Hidden })
	}

	return result
}

func visibleServer(server model.Server, ember model.Ember) model.Server {
	variables := make(map[string]string)

	for _, variable := range ember.Variables {
		if value, ok := server.Variables[variable.Name]; ok && !variable.Hidden {
			variables[variable.Name] = value
		}
	}

	server.Variables = variables

	return server
}

func (a *API) writeServer(w http.ResponseWriter, r *http.Request, status int, server model.Server) {
	allowed, err := a.access.CanManageServer(r.Context(), currentUser(r), server.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	server.CanManageAccess = allowed

	ember, err := a.embers.Get(r.Context(), server.EmberID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, status, visibleServer(server, ember))
}

// Hidden variables are managed by Ember managers, independently of editability.
func (a *API) checkHiddenVariables(r *http.Request, input model.ServerConfiguration, serverID string) error {
	if currentUser(r).Allows(accessperm.EmbersManage) {
		return nil
	}

	values := map[string]string{}
	emberID := input.EmberID

	if serverID != "" {
		server, err := a.servers.Get(r.Context(), serverID)
		if err != nil {
			return err
		}

		values, emberID = server.Variables, server.EmberID
	}

	ember, err := a.embers.Get(r.Context(), emberID)
	if err != nil {
		return err
	}

	for _, variable := range ember.Variables {
		if !variable.Hidden {
			continue
		}

		expected, exists := values[variable.Name]
		if !exists {
			expected = variable.Default
		}

		if supplied, ok := input.Variables[variable.Name]; ok && supplied != expected {
			return repository.ErrAccessDenied
		}
	}

	return nil
}
