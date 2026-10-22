package service

import (
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestEmberPortDescriptionsAreValidatedAndTrimmed(t *testing.T) {
	ember := model.Ember{Schema: 1, Name: "Game", Images: map[string]string{"Game": "alpine:3.22"}, Startup: "sleep 60", WorkDir: "/home/container", User: "1000:1000", RequiredPorts: []model.RequiredPort{
		{Container: 27015, Protocol: "tcp", Description: " Main "},
		{Container: 27015, Protocol: "udp", Description: "Query"},
		{Container: 27016, Protocol: "udp"},
	}}
	if err := validateEmber(&ember); err != nil {
		t.Fatal(err)
	}

	labels := ember.PortDescriptions()
	if labels["27015/tcp"] != "Main" || labels["27015/udp"] != "Query" || len(labels) != 2 {
		t.Fatal("descriptions must distinguish protocols and omit unlabeled ports", labels)
	}

	for _, invalid := range []string{strings.Repeat("x", 81), "Main\x00port", "Query\nport"} {
		ember.RequiredPorts[0].Description = invalid
		if err := validateEmber(&ember); err == nil {
			t.Fatal("invalid description accepted", invalid)
		}
	}
}
