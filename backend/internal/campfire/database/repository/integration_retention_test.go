package repository_test

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

func TestIntegrationRetentionPreservesWorkAndDeduplication(t *testing.T) {
	db, store, _ := integrationStore(t)
	now := time.Now()

	states := []model.DeliveryState{model.DeliveryDelivered, model.DeliveryFailed, model.DeliveryPending, core.MessageUncertain, core.MessageCredentialsBlocked, core.MessageDestinationBlocked, core.CardReady}
	for index, state := range states {
		category := core.GameEvents
		if state == core.CardReady {
			category = core.CardCategory
		}

		row := database.IntegrationMessage{BindingID: "binding", EventID: "retention-" + strconv.Itoa(index), Category: category, DestinationID: "channel", Payload: json.RawMessage(`{"private":"data"}`), State: state, Error: "old error", CreatedAt: now.Add(-40 * 24 * time.Hour).Unix()}
		if err := db.ORM().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := store.PruneMessageHistory(t.Context(), now); err != nil {
		t.Fatal(err)
	}

	var rows []database.IntegrationMessage
	if err := db.ORM().Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}

	for index, row := range rows {
		if index < 2 {
			if row.State != core.MessageReceipt || string(row.Payload) != "{}" || row.Error != "" {
				t.Fatal("history was not compacted", row)
			}
		} else if row.State != states[index] || string(row.Payload) == "{}" {
			t.Fatal("live work was pruned", row)
		}
	}

	replay, err := store.EnqueueMessage(t.Context(), "binding", "retention-0", core.GameEvents, json.RawMessage(`{}`), now)
	if err != nil || replay.ID != rows[0].ID || replay.State != core.MessageReceipt {
		t.Fatal("receipt allowed replay", replay, err)
	}

	history, err := store.MessageStatuses(t.Context(), "binding", []core.Category{core.GameEvents}, 0, "")
	if err != nil || len(history) != 4 {
		t.Fatal("receipts appeared in history", history, err)
	}

	if err := store.PruneMessageHistory(t.Context(), now.Add(51*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	var receipts int64
	if err := db.ORM().Model(&database.IntegrationMessage{}).Where("state = ?", core.MessageReceipt).Count(&receipts).Error; err != nil || receipts != 0 {
		t.Fatal("expired receipts retained", receipts, err)
	}
}

func TestIntegrationRetentionBoundsHistoryWithEqualTimestamps(t *testing.T) {
	db, store, _ := integrationStore(t)
	now := time.Now()

	rows := make([]database.IntegrationMessage, 10002)
	for i := range rows {
		rows[i] = database.IntegrationMessage{BindingID: "binding", EventID: "limit-" + strconv.Itoa(i), Category: core.GameEvents, DestinationID: "channel", Payload: json.RawMessage(`{}`), State: model.DeliveryDelivered, CreatedAt: now.Unix()}
	}

	if err := db.ORM().CreateInBatches(rows, 500).Error; err != nil {
		t.Fatal(err)
	}

	if err := store.PruneMessageHistory(t.Context(), now); err != nil {
		t.Fatal(err)
	}

	var live, receipts int64
	if err := db.ORM().Model(&database.IntegrationMessage{}).Where("state = ?", model.DeliveryDelivered).Count(&live).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Model(&database.IntegrationMessage{}).Where("state = ?", core.MessageReceipt).Count(&receipts).Error; err != nil {
		t.Fatal(err)
	}

	if live != 10000 || receipts != 2 {
		t.Fatal("history count was not bounded", live, receipts)
	}
}
