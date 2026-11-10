//go:build !production

package devmock

import (
	"context"

	"campfire/internal/campfire/devmock/integrations"
)

const integrationSeedMarkerID = 13

// EnableIntegrationExample connects Discord once per disposable mock session.
// Removing or disabling it in the UI survives watcher-triggered rebuilds.
func (r *Runtime) EnableIntegrationExample(ctx context.Context, provider *integrations.Discord, secretsDirectory string) error {
	var count int64
	if err := r.db.WithContext(ctx).Model(&seedMarker{}).Where("id = ?", integrationSeedMarkerID).Count(&count).Error; err != nil || count > 0 {
		return err
	}

	if err := provider.Seed(ctx, r.db, secretsDirectory); err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(&seedMarker{ID: integrationSeedMarkerID}).Error
}
