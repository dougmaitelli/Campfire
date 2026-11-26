package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"campfire/internal/shared/model"
)

type crashEvent struct {
	Action   string `json:"Action"`
	TimeNano int64  `json:"timeNano"`
	Actor    struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
}

type crashEventProvider interface {
	followCrashEvents(context.Context, func(crashEvent) error) error
}

var _ crashEventProvider = Docker{}

func (Docker) followCrashEvents(ctx context.Context, emit func(crashEvent) error) error {
	// Docker retains only a bounded event history. Reconnects replay what Docker
	// still has; per-server durable cursors suppress duplicates. This is not a
	// guarantee to recover every exit while Docker or Spark was unavailable.
	command, err := dockerCommand(ctx, "events", "--since", time.Now().Add(-model.CrashRetention).UTC().Format(time.RFC3339), "--filter", "type=container", "--filter", "label=campfire.server", "--filter", "event=start", "--filter", "event=die", "--filter", "event=oom", "--filter", "event=kill", "--format", "{{json .}}")
	if err != nil {
		return err
	}

	pending := ""

	return followCommand(ctx, command, func(chunk string) error {
		pending += chunk
		for {
			line, rest, found := strings.Cut(pending, "\n")
			if !found {
				break
			}

			if len(line) > 64<<10 {
				return errors.New("oversized Docker event")
			}

			pending = rest

			var event crashEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				return errors.New("invalid Docker event")
			}

			if err := emit(event); err != nil {
				return err
			}
		}

		if len(pending) > 64<<10 {
			return errors.New("oversized Docker event")
		}

		return nil
	})
}
