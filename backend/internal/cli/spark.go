package cli

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"campfire/internal/spark"
)

// RunSpark parses agent configuration and manages graceful signal cancellation.
func RunSpark(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("spark", flag.ContinueOnError)
	panel := flags.String("panel", "", "HTTPS agent listener URL (required for first enrollment)")
	data := flags.String("data", "spark-data", "persistent private agent state directory")
	ca := flags.String("ca-file", "", "CA certificate for a private panel HTTPS certificate")

	tokenFile := flags.String("token-file", "", "private file containing a one-time enrollment token")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}

	token := strings.TrimSpace(os.Getenv("CAMPFIRE_ENROLLMENT_TOKEN"))
	if *tokenFile != "" {
		if token != "" {
			return errors.New("use either --token-file or CAMPFIRE_ENROLLMENT_TOKEN")
		}

		info, err := os.Stat(*tokenFile)
		if err != nil {
			return err
		}

		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 256 {
			return errors.New("enrollment token file must be private (0600) and at most 256 bytes")
		}

		value, err := os.ReadFile(*tokenFile)
		if err != nil {
			return err
		}

		token = strings.TrimSpace(string(value))
	}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return spark.RunAgent(ctx, spark.AgentConfig{PanelURL: *panel, DataDir: *data, CAFile: *ca, Token: token})
}
