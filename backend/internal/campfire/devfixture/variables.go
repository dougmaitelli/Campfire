//go:build !production

package devfixture

import "campfire/internal/shared/model"

// MinecraftVariables describes the shared demo Ember's editable settings.
func MinecraftVariables() []model.Variable {
	return []model.Variable{
		{Name: "SERVER_JARFILE", Default: "server.jar", Required: true, Editable: true},
		{Name: "MC_VERSION", Default: "latest", Required: true, Editable: true},
		{Name: "FABRIC_VERSION", Default: "latest", Required: true, Editable: true},
		{Name: "LOADER_VERSION", Default: "latest", Required: true, Editable: true},
		{Name: "SERVER_NAME", Default: "Campfire survival", Required: true, Editable: true},
		{Name: "MOTD", Default: "A place for friends to build together", Editable: true},
		{Name: "MAX_PLAYERS", Default: "20", Required: true, Editable: true, Validation: &model.VariableValidation{Type: "integer"}},
		{Name: "VIEW_DISTANCE", Default: "10", Required: true, Editable: true, Validation: &model.VariableValidation{Type: "integer"}},
		{Name: "DIFFICULTY", Default: "normal", Editable: true, Validation: &model.VariableValidation{Type: "string", Choices: []string{"peaceful", "easy", "normal", "hard"}}},
		{Name: "WORLD_SEED", Default: "", Editable: true},
		{Name: "ENABLE_COMMAND_BLOCKS", Default: "0", Editable: true, Validation: &model.VariableValidation{Type: "boolean"}},
		{Name: "INSTALL_CHANNEL", Default: "stable", Required: true},
		{Name: "INTERNAL_FIXTURE", Default: "mock", Hidden: true},
	}
}
