package runtime

import (
	"context"
	"reflect"
	"testing"
)

func TestStopDispatchesExplicitSignals(t *testing.T) {
	for stop, signal := range map[string]string{
		"": "", "^C": "SIGINT", "^SIGINT": "SIGINT", "^SIGTERM": "SIGTERM",
		"^SIGABRT": "SIGABRT", "^SIGKILL": "SIGKILL", "^SIGQUIT": "SIGQUIT",
		"^sigusr1": "SIGUSR1", "^SIGRTMIN+1": "SIGRTMIN+1",
	} {
		t.Run(stop, func(t *testing.T) {
			ctx := context.Background()
			d := runtimeFixture()
			d.Ember.Stop = stop
			docker := &fakeDocker{states: map[string]string{container(d.ServerID): "running"}}
			m := newTestManager(ctx, t.TempDir())

			m.Docker = docker
			if err := m.stop(ctx, d); err != nil {
				t.Fatal(err)
			}

			want := []string{"stop", "--time", "30"}
			if signal != "" {
				want = append(want, "--signal", signal)
			}

			want = append(want, container(d.ServerID))
			if got := docker.calls[len(docker.calls)-1]; !reflect.DeepEqual(got, want) {
				t.Fatalf("Docker arguments = %v; want %v", got, want)
			}
		})
	}
}

func TestStopRejectsInvalidSignalBeforeDocker(t *testing.T) {
	d := runtimeFixture()
	d.Ember.Stop = "^INVALID"
	docker := &fakeDocker{}
	m := newTestManager(context.Background(), t.TempDir())

	m.Docker = docker
	if err := m.stop(context.Background(), d); err == nil || len(docker.calls) != 0 {
		t.Fatalf("invalid stop reached Docker: %v, %v", err, docker.calls)
	}
}
