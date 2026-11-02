package model

// VisibleServer exposes readable configuration without hidden Ember variables.
func VisibleServer(server Server, ember Ember) Server {
	variables := make(map[string]string)
	types := make(map[string]VariableType)

	for _, variable := range ember.Variables {
		if value, ok := server.Variables[variable.Name]; ok && !variable.Hidden {
			variables[variable.Name] = value
			if variable.Validation != nil {
				types[variable.Name] = variable.Validation.Type
			}
		}
	}

	server.Variables = variables
	server.VariableTypes = types
	server.SavedataAvailable = len(ember.SavedataPaths) > 0 && !server.EmberUpdateAvailable

	return server
}
