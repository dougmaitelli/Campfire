package service

import (
	"campfire/internal/shared/model"
	"encoding/json"
	"testing"
)

func TestNativePrimaryPortImportRoundTrip(t *testing.T) {
	for _, primary := range []bool{false, true} {
		e := model.Ember{Schema: 2, Name: "Game", Images: map[string]string{"Game": "alpine:3.22"}, Startup: "sleep 60", WorkDir: "/home/container", User: "1000:1000", RequiredPorts: []model.RequiredPort{{Container: 25565, Protocol: model.NetworkTCP, Primary: primary}}}

		encoded, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}

		report := PreviewEmber(encoded, ImportOptions{})
		if len(report.Errors) != 0 || report.Ember == nil {
			t.Fatal(report)
		}

		if report.Ember.Schema != 2 || report.Ember.RequiredPorts[0].Primary != primary {
			t.Fatal(report.Ember)
		}

		again := PreviewEmber([]byte(report.Definition), ImportOptions{})
		if len(again.Errors) != 0 || again.Ember.RequiredPorts[0].Primary != primary {
			t.Fatal(again)
		}
	}
}
