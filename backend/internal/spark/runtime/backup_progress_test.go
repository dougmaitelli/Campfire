package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/workprogress"
)

func TestRestoreProgressDoesNotConfirmSuccessBeforeCommit(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "failure"
		}

		t.Run(name, func(t *testing.T) {
			m, d := fileTestManager(t)

			m.Ownership = func(string, string) error { return nil }
			if status := awaitOperation(t, m, d, "install", "install"); status.Error != "" {
				t.Fatal(status)
			}

			path := filepath.Join(m.root(d.ServerID), "data", "world")
			if err := os.WriteFile(path, []byte("saved"), 0600); err != nil {
				t.Fatal(err)
			}

			created := awaitOperation(t, m, d, "backup", "snapshot")
			if created.Error != "" || created.OperationState != model.OperationSucceeded || created.WorkProgress == nil || created.WorkProgress.Phase != workprogress.Committing {
				t.Fatal(created)
			}

			if err := os.WriteFile(path, []byte("current"), 0600); err != nil {
				t.Fatal(err)
			}

			entered, release := make(chan struct{}), make(chan struct{})

			var once sync.Once

			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()

			m.Ownership = func(string, string) error {
				close(entered)
				<-release

				if fail {
					return errors.New("ownership unavailable")
				}

				return nil
			}

			request := model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionRestore, OperationID: "restore", Content: "full-snapshot.tar.gz"}
			if _, err := m.Execute(t.Context(), d, request); err != nil {
				t.Fatal(err)
			}

			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("restore did not reach ownership")
			}

			status, err := m.Execute(t.Context(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStatus})
			if err != nil || !status.Busy || status.OperationState != model.OperationRunning || status.WorkProgress == nil || status.WorkProgress.Phase != workprogress.Restoring {
				t.Fatal(status, err)
			}

			data, err := os.ReadFile(path)
			if err != nil || string(data) != "current" {
				t.Fatal("data committed prematurely", string(data), err)
			}

			unblock()

			final := awaitBackup(t, m, d, request)
			if fail {
				if final.OperationState != model.OperationFailed || final.Error == "" {
					t.Fatal(final)
				}
			} else if final.OperationState != model.OperationSucceeded || final.Error != "" {
				t.Fatal(final)
			}
		})
	}
}
