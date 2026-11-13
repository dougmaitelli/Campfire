package runtime

import "testing"

func TestRecoveredRuntimeClearsFileSystemBusy(t *testing.T) {
	for _, busy := range []bool{false, true} {
		name := "stale filesystem lock"
		if busy {
			name = "interrupted operation"
		}

		t.Run(name, func(t *testing.T) {
			m, d := fileTestManager(t)

			e, err := m.load(d.ServerID)
			if err != nil {
				t.Fatal(err)
			}

			e.Status.Busy, e.Status.FileSystemBusy = busy, true
			if err := m.persist(d.ServerID, e); err != nil {
				t.Fatal(err)
			}

			m.Close()
			recovered := newTestManager(t.Context(), m.Directory)
			t.Cleanup(recovered.Close)

			e, err = recovered.load(d.ServerID)
			if err != nil {
				t.Fatal(err)
			}

			if e.Status.Busy || e.Status.FileSystemBusy {
				t.Fatalf("recovered runtime remained busy: %+v", e.Status)
			}
		})
	}
}
