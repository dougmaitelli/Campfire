package database

import (
	"encoding/json"
	"errors"
	"slices"

	"campfire/internal/shared/model"

	core "campfire/internal/campfire/integrations"

	"gorm.io/gorm"
)

// migrateIntegrationDestinations replaces the former per-server singleton index.
// AutoMigrate recreates it with destination_id; rows and message ownership remain.
func migrateIntegrationDestinations(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&ServerIntegration{}) {
		return nil
	}

	// Discard grants from earlier development versions without promoting their
	// holders to the broader server configuration permission.
	const obsoletePermission = "server.integrations.manage"
	if err := tx.Where("permission = ?", obsoletePermission).Delete(&RolePermission{}).Error; err != nil {
		return err
	}

	var tokens []APIToken
	if err := tx.Where("scopes LIKE ?", "%"+obsoletePermission+"%").Find(&tokens).Error; err != nil {
		return err
	}

	for _, token := range tokens {
		token.Scopes = slices.DeleteFunc(token.Scopes, func(scope model.PermissionID) bool { return scope == obsoletePermission })
		if err := tx.Model(&token).Select("Scopes").Updates(&token).Error; err != nil {
			return err
		}
	}

	if err := migrateIntegrationRules(tx); err != nil {
		return err
	}

	indexes, err := tx.Migrator().GetIndexes(&ServerIntegration{})
	if err != nil {
		return err
	}

	for _, index := range indexes {
		columns := index.Columns()
		if index.Name() == "server_integration_binding" && len(columns) == 2 && columns[0] == "integration_id" && columns[1] == "server_id" {
			return tx.Migrator().DropIndex(&ServerIntegration{}, index.Name())
		}
	}

	return nil
}

// Old subscriptions shared conditions across every selected type. Copying those
// conditions into each new rule preserves their exact meaning and message IDs.
func migrateIntegrationRules(tx *gorm.DB) error {
	var rows []struct {
		ID            string
		Subscriptions string
	}
	if err := tx.Table("server_integrations").Select("id, subscriptions").Find(&rows).Error; err != nil {
		return err
	}

	for _, row := range rows {
		var subscriptions []map[string]json.RawMessage
		if err := json.Unmarshal([]byte(row.Subscriptions), &subscriptions); err != nil {
			return err
		}

		changed := false

		for _, subscription := range subscriptions {
			_, hadTypes := subscription["types"]

			_, hadFields := subscription["fields"]
			if !hadTypes && !hadFields {
				continue
			}

			if _, exists := subscription["rules"]; exists {
				return errors.New("integration subscription mixes legacy and scoped rules")
			}

			var types []string

			var fields map[string]string

			if hadTypes {
				if err := json.Unmarshal(subscription["types"], &types); err != nil {
					return err
				}
			}

			if hadFields {
				if err := json.Unmarshal(subscription["fields"], &fields); err != nil {
					return err
				}
			}

			rules := []core.EventRule{}
			for _, kind := range types {
				rules = append(rules, core.EventRule{Type: kind, Fields: fields})
			}

			if len(types) == 0 && len(fields) > 0 {
				rules = append(rules, core.EventRule{Fields: fields})
			}

			delete(subscription, "types")
			delete(subscription, "fields")

			if len(rules) > 0 {
				encoded, err := json.Marshal(rules)
				if err != nil {
					return err
				}

				subscription["rules"] = encoded
			}

			changed = true
		}

		if changed {
			encoded, err := json.Marshal(subscriptions)
			if err != nil {
				return err
			}

			if err := tx.Table("server_integrations").Where("id = ?", row.ID).Update("subscriptions", string(encoded)).Error; err != nil {
				return err
			}
		}
	}

	return nil
}
