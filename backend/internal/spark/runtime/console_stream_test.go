package runtime

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestConsoleProcess(_ *testing.T) {
	if os.Getenv("CAMPFIRE_CONSOLE_HELPER") != "1" {
		return
	}

	_, _ = os.Stdout.WriteString(strings.Repeat("continuous output\n", 20000) + "café 🎮\n")
	_, _ = os.Stderr.WriteString("stderr output\n")

	if os.Getenv("CAMPFIRE_CONSOLE_WAIT") == "1" {
		time.Sleep(30 * time.Second)
	}

	os.Exit(0)
}

func helperConsoleCommand(ctx context.Context, wait bool) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConsoleProcess$")

	cmd.Env = append(os.Environ(), "CAMPFIRE_CONSOLE_HELPER=1")
	if wait {
		cmd.Env = append(cmd.Env, "CAMPFIRE_CONSOLE_WAIT=1")
	}

	return cmd
}

func TestConsoleStreamsBeyondSnapshotLimitAndCancels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var output strings.Builder

	err := followCommand(ctx, helperConsoleCommand(ctx, false), func(text string) error {
		if len(text) > 8196 || !utf8.ValidString(text) {
			t.Error("invalid output chunk")
		}

		output.WriteString(text)

		return nil
	})
	if err != nil || !strings.Contains(output.String(), "café 🎮\n") || !strings.Contains(output.String(), "stderr output\n") || strings.Count(output.String(), "continuous output\n") != 20000 {
		t.Fatal("output lost", output.Len(), err)
	}

	ctx, stop := context.WithCancel(ctx)
	finished := make(chan error, 1)
	first := make(chan struct{}, 1)

	go func() {
		finished <- followCommand(ctx, helperConsoleCommand(ctx, true), func(string) error {
			select {
			case first <- struct{}{}:
			default:
			}

			return nil
		})
	}()

	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("console did not emit")
	}

	stop()

	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancelled console succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel leaked subprocess")
	}

	sentinel := errors.New("consumer stopped")
	if err := followCommand(context.Background(), helperConsoleCommand(context.Background(), true), func(string) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatal("consumer error lost", err)
	}
}
