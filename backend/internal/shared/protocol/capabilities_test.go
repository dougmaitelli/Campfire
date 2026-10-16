package protocol

import (
	"slices"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestNegotiationUsesVersionOverlapAndExplicitFeatures(t *testing.T) {
	hello := Message{Protocol: ProtocolVersion + 1, MinimumProtocol: MinimumProtocolVersion, Capabilities: []string{CapabilityLifecycle, "future.feature.v1", CapabilityMetrics}}

	version, features, err := Negotiate(hello)
	if err != nil || version != ProtocolVersion || !slices.Equal(features, []string{CapabilityLifecycle, CapabilityMetrics}) {
		t.Fatal("negotiation failed", version, features, err)
	}

	hello.Capabilities = nil

	_, features, err = Negotiate(hello)
	if err != nil || len(features) != 0 {
		t.Fatal("missing advertisement implied feature support", features, err)
	}

	for _, message := range []Message{
		{Protocol: MinimumProtocolVersion - 1},
		{Protocol: ProtocolVersion + 1, MinimumProtocol: ProtocolVersion + 1},
		{Protocol: ProtocolVersion, MinimumProtocol: ProtocolVersion + 1},
		{Protocol: ProtocolVersion, MinimumProtocol: -1},
		{Protocol: ProtocolVersion, Capabilities: []string{CapabilityMetrics, CapabilityMetrics}},
		{Protocol: ProtocolVersion, Capabilities: []string{strings.Repeat("a", 65)}},
		{Protocol: ProtocolVersion, Capabilities: []string{"bad\nfeature"}},
		{Protocol: ProtocolVersion, Capabilities: make([]string, 65)},
	} {
		if _, _, err := Negotiate(message); err == nil {
			t.Fatal("invalid advertisement accepted", message.Protocol)
		}
	}

	if _, _, err := Negotiate(Message{Protocol: ProtocolVersion}); err != nil {
		t.Fatal("exact-version handshake failed", err)
	}
}

func TestWelcomeCannotExpandAnOffer(t *testing.T) {
	offered := []string{CapabilityLifecycle}
	for _, welcome := range []Message{
		{Type: "welcome", Protocol: ProtocolVersion, Capabilities: []string{CapabilityTransfer}},
		{Type: "welcome", Protocol: ProtocolVersion + 1},
		{Type: "welcome", Protocol: MinimumProtocolVersion - 1},
		{Type: "hello", Protocol: ProtocolVersion},
	} {
		if err := ValidateWelcome(welcome, offered); err == nil {
			t.Fatal("unoffered welcome accepted")
		}
	}

	if err := ValidateWelcome(Message{Type: "welcome", Protocol: ProtocolVersion, Capabilities: offered}, offered); err != nil {
		t.Fatal(err)
	}
}

func TestCommandRequirementsPreventOptionalFeatureFallback(t *testing.T) {
	cases := []struct {
		command    Command
		capability string
	}{
		{Command{Name: ConfigureServer}, CapabilityConfigure},
		{Command{Name: ConfigureServer, Deployment: &model.Deployment{Ember: model.Ember{Readiness: &model.Readiness{Any: []model.ReadinessMatch{{Value: "Ready"}}}}}}, CapabilityEmberBehavior},
		{Command{Name: ConfigureServer, Deployment: &model.Deployment{Ember: model.Ember{Configs: []model.ConfigFile{{Parser: "json", Create: true}}}}}, CapabilityEmberBehavior},
		{Command{Name: ServerFiles}, CapabilityFiles},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "upload.begin"}}, CapabilityFileTransfer},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "read.chunk"}}, CapabilityFileTransfer},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "extract"}}, CapabilityFileArchive},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "compress"}}, CapabilityFileArchive},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "backup.read"}}, CapabilityBackups},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "backup.delete"}}, CapabilityBackupDelete},
		{Command{Name: ServerTransfer}, CapabilityTransfer},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "metrics"}}, CapabilityMetrics},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "update"}}, CapabilityUpgrade},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "reinstall"}}, CapabilityUpgrade},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "start"}}, CapabilityDiskQuota},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "restore", BackupSource: &model.BackupSource{}}}, CapabilityBackupRemote},
	}
	for _, test := range cases {
		required := RequiredCapabilities(test.command)
		all := SupportedCapabilities()

		limited := slices.DeleteFunc(all, func(item string) bool { return item == test.capability })
		if err := Require(limited, required...); err == nil {
			t.Fatal("missing capability was not enforced", test.capability)
		}

		if err := Require(SupportedCapabilities(), required...); err != nil {
			t.Fatal(err)
		}
	}

	if err := Require([]string{CapabilityLifecycle}, RequiredCapabilities(Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "stop"}})...); err != nil {
		t.Fatal("quota capability unnecessarily blocked stop", err)
	}
}
