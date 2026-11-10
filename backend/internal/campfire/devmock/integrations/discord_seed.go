//go:build !production

package integrations

import (
	"context"

	fixture "campfire/internal/campfire/devfixture/integrations"
	core "campfire/internal/campfire/integrations"
	service "campfire/internal/campfire/service/integrations"
	"campfire/internal/shared/model"
	"campfire/internal/shared/permissions"

	"gorm.io/gorm"
)

type mockAdministrator struct{}

func (mockAdministrator) CanManageIntegrations() bool { return true }

// Seed exercises normal setup and credential storage against the fake provider.
func (d *Discord) Seed(ctx context.Context, db *gorm.DB, secretsDirectory string) error {
	setup := service.NewDiscordIntegrationService(db, service.Secrets{Directory: secretsDirectory}, d.Client())

	status, err := setup.Status(ctx, mockAdministrator{})
	if err != nil || status.Configured {
		return err
	}

	if _, err := setup.Save(ctx, mockAdministrator{}, service.DiscordIntegrationInput{Enabled: true, GuildID: "200", Token: fixture.DiscordToken}); err != nil {
		return err
	}

	_, err = setup.SaveServerSettings(ctx, "survival", "", []model.PermissionID{permissions.ServerView, permissions.ServerConfigure}, service.ServerIntegrationInput{
		Enabled: true, ChannelID: "400",
		Subscriptions: []core.Subscription{{Category: core.GameEvents}, {Category: core.PowerEvents}},
		Card:          core.CardSettings{Enabled: true, Fields: []string{"players", "capacity"}},
	})

	return err
}
