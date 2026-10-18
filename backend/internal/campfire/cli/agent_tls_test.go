package cli

import "testing"

func TestAgentPublicAddress(t *testing.T) {
	for _, test := range []struct{ listen, advertised, panel, want string }{
		{defaultAgentListen, "", "https://panel.example.test", "https://panel.example.test:8443"},
		{"0.0.0.0:8443", "https://agents.example.test:9443/", "https://panel.example.test", "https://agents.example.test:9443"},
		{"0.0.0.0:8443", "", "", "https://localhost:8443"},
		{"[::]:8443", "", "", "https://localhost:8443"},
		{"127.0.0.1:9443", "", "", "https://127.0.0.1:9443"},
		{"[::1]:9443", "", "", "https://[::1]:9443"},
		{"", "", "https://panel.example.test", ""},
	} {
		got, err := agentPublicAddress(test.listen, test.advertised, test.panel)
		if err != nil || got != test.want {
			t.Fatal(test, got, err)
		}
	}

	for _, address := range []string{"http://agents.test", "https://user:pass@agents.test", "https://agents.test/path", "https://agents.test?query=1", "https://agents.test#fragment", "https://0.0.0.0:8443", "https://[::]:8443", "https://agents.test:99999"} {
		if _, err := agentPublicAddress(defaultAgentListen, address, ""); err == nil {
			t.Fatal("accepted invalid agent address", address)
		}
	}
}
