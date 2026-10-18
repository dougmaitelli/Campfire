// Package cli provides command-line startup for the standalone Spark agent.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"campfire/internal/shared/buildinfo"
	"campfire/internal/shared/cliutil"
	"campfire/internal/shared/protocol"
	"campfire/internal/spark/agent"
)

// Run parses agent configuration and manages graceful signal cancellation.
func Run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("spark", flag.ContinueOnError)
	panel := flags.String("panel", cliutil.Environment("SPARK_PANEL", ""), "HTTPS agent URL for first enrollment (SPARK_PANEL)")
	data := flags.String("data", cliutil.Environment("SPARK_DATA", "spark-data"), "persistent private agent state directory (SPARK_DATA)")
	ca := flags.String("ca-file", cliutil.Environment("SPARK_CA_FILE", ""), "private panel HTTPS CA certificate (SPARK_CA_FILE)")
	version := flags.Bool("version", false, "print release version and exit")
	updateInfo := flags.Bool("update-info", false, "print version and wire compatibility for verified updates")
	enrollOnly := flags.Bool("enroll-only", false, "enroll and save the agent identity, then exit")

	tokenFile := flags.String("token-file", cliutil.Environment("SPARK_TOKEN_FILE", ""), "private one-time enrollment token file (SPARK_TOKEN_FILE)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}

	if *version {
		return cliutil.PrintVersion("Spark")
	}

	if *updateInfo {
		return json.NewEncoder(os.Stdout).Encode(protocol.SparkUpdateInfo{Version: buildinfo.SparkVersion(), MinimumProtocol: protocol.MinimumProtocolVersion, Protocol: protocol.ProtocolVersion})
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

	err := agent.Run(ctx, agent.Config{PanelURL: *panel, DataDir: *data, CAFile: *ca, Token: token, EnrollOnly: *enrollOnly})

	var restart *agent.RestartError
	if errors.As(err, &restart) {
		if err := syscall.Exec(restart.Executable, os.Args, os.Environ()); err != nil {
			rollbackErr := os.Rename(restart.Executable+".previous", restart.Executable)
			return errors.Join(err, rollbackErr)
		}
	}

	return err
}
