//go:build !production

package devmock

import (
	"campfire/internal/campfire/devfixture"
	"campfire/internal/shared/model"
)

func mockMinecraftVariables() []model.Variable { return devfixture.MinecraftVariables() }
