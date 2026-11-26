package gameaddress

import (
	"testing"

	"campfire/internal/shared/model"
)

func TestResolve(t *testing.T) {
	for _, test := range []struct {
		override, host string
		port           int
		want           string
	}{
		{"https://Play.Example.test:32100/Join?World=Case", "ignored.example", 25565, "https://Play.Example.test:32100/Join?World=Case"},
		{"play.example.test", "ignored.example", 25565, "play.example.test"},
		{"connect 192.0.2.1:27015", "", 0, "connect 192.0.2.1:27015"},
		{"", "games.example.test", 25565, "games.example.test:25565"},
		{"", "2001:db8::1", 25565, "[2001:db8::1]:25565"},
		{"", "192.0.2.1", 25565, "192.0.2.1:25565"},
		{"", "", 25565, ""}, {"", "games.example.test", 0, ""}, {"", "games.example.test", 65536, ""},
	} {
		if got := Resolve(test.override, test.host, test.port); got != test.want {
			t.Fatalf("%+v: %q", test, got)
		}
	}
}

func TestUnusableDefaultsAreSuppressedWithoutRestrictingOverrides(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::", "224.0.0.1", "239.255.255.255", "ff02::1", "::ffff:224.0.0.1", "0:0:0:0:0:0:0:0"} {
		if UsableDefaultHost(host) || Resolve("", host, 25565) != "" {
			t.Fatal("unusable inherited address", host)
		}

		if Resolve("custom connection", host, 25565) != "custom connection" {
			t.Fatal("override was interpreted")
		}
	}

	for _, host := range []string{"192.168.1.10", "127.0.0.1", "games.example.test", "2001:db8::1"} {
		if !UsableDefaultHost(host) || Resolve("", host, 25565) == "" {
			t.Fatal("usable default rejected", host)
		}
	}
}

func TestAddressUsesOnlyAnExplicitMappedPrimaryPort(t *testing.T) {
	game := model.ServerPort{BindIP: "192.0.2.1", ContainerPort: 25565, HostPort: 25570, Protocol: model.NetworkTCP}
	required := []model.RequiredPort{{Container: 25565, Protocol: model.NetworkTCP, Primary: true}}

	port := model.PrimaryHostPort([]model.ServerPort{game}, required)
	if Resolve("", "games.example.test", port) != "games.example.test:25570" {
		t.Fatal(port)
	}

	if Resolve("", "", port) != "" {
		t.Fatal("bind interface used as public host")
	}

	if Resolve("", "games.example.test", model.PrimaryHostPort([]model.ServerPort{game}, nil)) != "" {
		t.Fatal("undesignated port inferred")
	}
}
