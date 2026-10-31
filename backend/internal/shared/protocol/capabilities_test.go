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

func TestExecutionConfigurationRequiresNewContractOnly(t *testing.T) {
	_, capabilities, err := Negotiate(Message{Protocol: ProtocolVersion, Capabilities: []string{"server.configure.v1", "server.startup-override.v1", CapabilityLifecycle, CapabilitySelfUpdate}})
	if err != nil {
		t.Fatal(err)
	}

	command := Command{Name: ConfigureServer, Deployment: &model.Deployment{Ember: model.RuntimeDefinition{Startup: "echo custom"}}}
	if err := Require(capabilities, RequiredCapabilities(command)...); err == nil {
		t.Fatal("old Spark was allowed to receive the new configuration format")
	}

	if err := Require([]string{CapabilityConfigure}, RequiredCapabilities(command)...); err != nil {
		t.Fatal("resolved custom startup required a separate capability", err)
	}

	if err := Require(capabilities, RequiredCapabilities(Command{Name: UpdateSpark})...); err != nil {
		t.Fatal("old Spark cannot be remotely upgraded", err)
	}
}

func TestCommandRequirementsPreventOptionalFeatureFallback(t *testing.T) {
	cases := []struct {
		command    Command
		capability string
	}{
		{Command{Name: ConfigureServer}, CapabilityConfigure},
		{Command{Name: ReinstallSpark}, CapabilitySelfReinstall},
		{Command{Name: ConfigureServer, Deployment: &model.Deployment{Configuration: model.RuntimeConfiguration{GameIntegration: true}}}, CapabilityGameIntegration},
		{Command{Name: ConfigureServer, Deployment: &model.Deployment{Ember: model.RuntimeDefinition{Readiness: &model.Readiness{Any: []model.ReadinessMatch{{Value: "Ready"}}}}}}, CapabilityConfigure},
		{Command{Name: ConfigureServer, Deployment: &model.Deployment{Ember: model.RuntimeDefinition{Configs: []model.ConfigFile{{Parser: "json", Create: true}}}}}, CapabilityConfigure},
		{Command{Name: ServerFiles}, CapabilityFiles},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "chmod"}}, CapabilityFilePermissions},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "upload.begin"}}, CapabilityFileTransfer},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "read.chunk"}}, CapabilityFileTransfer},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "extract"}}, CapabilityFileArchive},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "compress"}}, CapabilityFileArchive},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "backup.read"}}, CapabilityBackups},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "backup.delete"}}, CapabilityBackups},
		{Command{Name: ServerTransfer}, CapabilityTransfer},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "metrics"}}, CapabilityMetrics},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "update"}}, CapabilityReinstall},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "reinstall"}}, CapabilityReinstall},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "reinstall", Content: "clean"}}, CapabilityReinstall},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "start"}}, CapabilityLifecycle},
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

func TestRequiredCapabilitiesKeepOperationSpecificRules(t *testing.T) {
	for _, test := range []struct {
		command Command
		want    []string
	}{
		{Command{Name: "unknown", Files: &model.FileRequest{Operation: "chmod"}}, nil},
		{Command{Name: PruneSpark}, []string{CapabilityPrune}},
		{Command{Name: UpdateSpark}, []string{CapabilitySelfUpdate}},
		{Command{Name: ReinstallSpark}, []string{CapabilitySelfReinstall}},
		{Command{Name: ServerTransfer}, []string{CapabilityTransfer}},
		{Command{Name: ConfigureServer}, []string{CapabilityConfigure}},
		{Command{Name: ConfigureServer, Deployment: &model.Deployment{Configuration: model.RuntimeConfiguration{GameIntegration: true}}}, []string{CapabilityConfigure, CapabilityGameIntegration}},
		{Command{Name: ServerFiles}, []string{CapabilityFiles}},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "unknown"}}, []string{CapabilityFiles}},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "chmod"}}, []string{CapabilityFiles, CapabilityFilePermissions}},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "upload.chunk"}}, []string{CapabilityFiles, CapabilityFileTransfer}},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "extract"}}, []string{CapabilityFiles, CapabilityFileArchive}},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "backup.list"}}, []string{CapabilityBackups}},
		{Command{Name: ServerFiles, Files: &model.FileRequest{Operation: "backup.unknown"}}, []string{CapabilityBackups}},
		{Command{Name: ServerAction}, []string{CapabilityLifecycle}},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "unknown"}}, []string{CapabilityLifecycle}},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "stop", BackupSource: &model.BackupSource{}}}, []string{CapabilityLifecycle}},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "metrics"}}, []string{CapabilityMetrics}},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "backup"}}, []string{CapabilityLifecycle, CapabilityBackups}},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "restore"}}, []string{CapabilityLifecycle, CapabilityBackups}},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "restore", BackupSource: &model.BackupSource{}}}, []string{CapabilityLifecycle, CapabilityBackups, CapabilityBackupRemote}},
		{Command{Name: ServerAction, Runtime: &model.RuntimeRequest{Action: "reinstall"}}, []string{CapabilityLifecycle, CapabilityReinstall}},
	} {
		got := RequiredCapabilities(test.command)
		if !slices.Equal(got, test.want) {
			t.Errorf("command %+v: got %v, want %v", test.command, got, test.want)
		}

		for i := range got {
			got[i] = "mutated"
		}

		if !slices.Equal(RequiredCapabilities(test.command), test.want) {
			t.Errorf("caller mutated shared capability requirements for %+v", test.command)
		}
	}
}
