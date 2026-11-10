package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func TestEmberEditorKeepsProposalAndRevisionConsistentDuringAnUpdate(t *testing.T) {
	f := newAccessFixture(t)
	_, ember, input := serverFixture(t, f.admin)
	created := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &server) != nil {
		t.Fatal(created.Code, created.Body)
	}

	ember.Variables = append(ember.Variables, model.Variable{Name: "REVIEWED", Default: "one", Editable: true})

	store := repository.NewEmberRepository(f.db.ORM())
	if err := store.Replace(t.Context(), ember); err != nil {
		t.Fatal(err)
	}

	newer := ember
	newer.Variables = append(newer.Variables, model.Variable{Name: "UNREVIEWED", Default: "two", Editable: true})
	armed := true

	if err := f.db.ORM().Callback().Query().After("gorm:query").Register("test:ember-editor-update", func(tx *gorm.DB) {
		if row, ok := tx.Statement.Dest.(*database.Ember); armed && ok && row.ID == ember.ID {
			armed = false

			if err := store.Replace(t.Context(), newer); err != nil {
				t.Fatal(err)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID
	response := request(f.admin, "GET", endpoint+"/editor", "")

	var editor struct {
		Embers   []model.Ember             `json:"embers"`
		Proposal model.ServerConfiguration `json:"ember_configuration"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &editor) != nil {
		t.Fatal(response.Code, response.Body)
	}

	if armed || len(editor.Embers) != 1 || editor.Embers[0].Revision != model.EmberHash(ember) {
		t.Fatal("editor paired its proposal with a different Ember revision", armed, editor.Embers)
	}

	if _, exists := editor.Proposal.Variables["UNREVIEWED"]; exists || editor.Proposal.Variables["REVIEWED"] != "one" {
		t.Fatal("editor did not retain the proposal from its selected revision", editor.Proposal.Variables)
	}

	body, err := json.Marshal(struct {
		model.ServerConfiguration
		ExpectedID       string `json:"expected_ember_id"`
		ExpectedRevision string `json:"expected_ember_revision"`
	}{editor.Proposal, ember.ID, editor.Embers[0].Revision})
	if err != nil {
		t.Fatal(err)
	}

	expectAccess(t, f.admin, "POST", endpoint+"/ember", string(body), http.StatusConflict)

	response = request(f.admin, "GET", endpoint, "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	if _, exists := server.Variables["UNREVIEWED"]; exists {
		t.Fatal("stale review persisted an unreviewed server value")
	}
}
