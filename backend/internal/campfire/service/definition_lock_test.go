package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

type definitionReadBarrier struct {
	ServerStore
	read chan struct{}
	once sync.Once
}

func (s *definitionReadBarrier) Get(ctx context.Context, id string) (model.Server, error) {
	server, err := s.ServerStore.Get(ctx, id)
	s.once.Do(func() { close(s.read) })

	return server, err
}

func TestDefinitionUpdateWhileWaitingForLockRejectsStaleServer(t *testing.T) {
	for _, action := range []string{"transfer", "sync", "copy"} {
		t.Run(action, func(t *testing.T) {
			f, copies, _ := newCopyFixture(t)
			d := copies.Deployment

			ctx, release, err := holdDefinition(t.Context(), f.server.EmberID)
			if err != nil {
				t.Fatal(err)
			}
			defer release()

			barrier := &definitionReadBarrier{ServerStore: d.Servers.store, read: make(chan struct{})}
			d.Servers = NewServerService(barrier, d.Embers).WithRuntimeGuard(d)
			result := make(chan error, 1)

			go func() {
				var err error

				switch action {
				case "transfer":
					_, err = f.service.Begin(t.Context(), f.server.ID, "target", f.ports)
				case "sync":
					_, err = d.Sync(t.Context(), f.server.ID)
				case "copy":
					_, err = copies.Create(t.Context(), f.server.ID, model.ServerConfiguration{SparkID: "target"}, 1, true, true, nil)
				}

				result <- err
			}()

			select {
			case <-barrier.read:
			case <-time.After(5 * time.Second):
				t.Fatal("operation did not read the source server")
			}

			ember, err := d.Embers.Get(ctx, f.server.EmberID)
			if err != nil {
				t.Fatal(err)
			}

			ember.Startup = "sleep 9876"
			if _, err := d.Embers.Replace(ctx, ember.ID, ember); err != nil {
				t.Fatal(err)
			}

			release()

			select {
			case err := <-result:
				if !errors.Is(err, ErrInvalidServer) {
					t.Fatal("operation used an unapplied definition", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("operation did not release the definition lock")
			}

			if err := copies.Store.Guard(t.Context(), f.server.ID); err != nil {
				t.Fatal("rejected operation retained a reservation", err)
			}
		})
	}
}

type definitionEditBarrier struct {
	EmberStore
	mu    sync.Mutex
	reads int
	read  chan struct{}
}

func (s *definitionEditBarrier) Get(ctx context.Context, id string) (model.Ember, error) {
	ember, err := s.EmberStore.Get(ctx, id)
	s.mu.Lock()
	s.reads++
	// BeforeEdit and Update each resolve hidden variables before save takes
	// the definition lock. Wait until both have read the old definition.
	if s.reads == 2 {
		close(s.read)
	}
	s.mu.Unlock()

	return ember, err
}

func TestServerEditRechecksDefinitionAfterWaitingForLock(t *testing.T) {
	for _, metadataOnly := range []bool{false, true} {
		name := "runtime configuration"
		if metadataOnly {
			name = "metadata only"
		}

		t.Run(name, func(t *testing.T) {
			f, copies, _ := newCopyFixture(t)
			d := copies.Deployment

			ember, err := d.Embers.Get(t.Context(), f.server.EmberID)
			if err != nil {
				t.Fatal(err)
			}

			ctx, release, err := holdDefinition(t.Context(), ember.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer release()

			barrier := &definitionEditBarrier{EmberStore: d.Embers.store, read: make(chan struct{})}
			d.Embers = NewEmberService(barrier)
			d.Servers.embers = d.Embers
			input := f.server.ServerConfiguration

			input.Name = "Renamed server"
			if !metadataOnly {
				input.MemoryMB = 256
			}

			result := make(chan error, 1)

			go func() {
				_, err := d.Servers.Update(t.Context(), f.server.ID, input)
				result <- err
			}()

			select {
			case <-barrier.read:
			case <-time.After(5 * time.Second):
				t.Fatal("edit did not read the old definition")
			}

			ember.Startup = "sleep 9876"
			if _, err := d.Embers.Replace(ctx, ember.ID, ember); err != nil {
				t.Fatal(err)
			}

			release()

			select {
			case err := <-result:
				if metadataOnly && err != nil {
					t.Fatal("metadata edit was rejected", err)
				}

				if !metadataOnly && !errors.Is(err, ErrInvalidServer) {
					t.Fatal("edit used an unapplied definition", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("edit did not release the definition lock")
			}

			server, err := d.Servers.Get(t.Context(), f.server.ID)
			if err != nil {
				t.Fatal(err)
			}

			if server.EmberHash != f.server.EmberHash || server.MemoryMB != f.server.MemoryMB {
				t.Fatal("edit changed the pinned definition or runtime configuration", server)
			}

			if metadataOnly && server.Name != input.Name {
				t.Fatal("metadata edit was not saved")
			}
		})
	}
}
