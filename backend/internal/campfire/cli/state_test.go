package cli

import "testing"

func TestStateDirectory(t *testing.T) {
	for _, test := range []struct{ source, configured, want string }{
		{"data/panel.db", "", "data"},
		{"/var/lib/campfire/panel.db", "", "/var/lib/campfire"},
		{"postgres://user:secret@db/campfire", "", "data"},
		{"postgresql://user:secret@db/campfire", "/var/lib/campfire", "/var/lib/campfire"},
		{"data/panel.db", "/srv/campfire", "/srv/campfire"},
	} {
		if got := stateDirectory(test.source, test.configured); got != test.want {
			t.Errorf("state directory = %q; want %q", got, test.want)
		}
	}
}
