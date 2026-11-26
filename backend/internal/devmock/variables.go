//go:build !production

package devmock

import (
	"campfire/internal/devfixture"
	"campfire/internal/shared/model"
)

func mockMinecraftVariables() []model.Variable { return devfixture.MinecraftVariables() }
