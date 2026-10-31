// Package cli provides command-line startup for the standalone Spark agent.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"campfire/internal/shared/buildinfo"
	"campfire/internal/shared/cliutil"
	"campfire/internal/shared/protocol"
	"campfire/internal/shared/settings"
	"campfire/internal/spark/agent"
)

// Run parses agent configuration and manages graceful signal cancellation.
func Run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("spark", flag.ContinueOnError)
	panel := settings.SparkPanel.String(flags, "", "HTTPS agent URL for first enrollment")
	data := settings.SparkData.String(flags, "spark-data", "persistent private agent state directory")
	ca := settings.SparkCAFile.String(flags, "", "private panel HTTPS CA certificate")
	version := flags.Bool(settings.Version.FlagName, false, "print release version and exit")
	updateInfo := flags.Bool(settings.SparkUpdateInfo.FlagName, false, "print version and wire compatibility for verified updates")
	enrollOnly := flags.Bool(settings.SparkEnrollOnly.FlagName, false, "enroll and save the agent identity, then exit")
	// Accept existing service arguments while control moves to server configuration.
	flags.Bool(settings.SparkGameIntegrations.FlagName, true, "deprecated: game integrations are managed through the dashboard")

	tokenFile := settings.SparkTokenFile.String(flags, "", "private one-time enrollment token file")
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

	token := strings.TrimSpace(settings.SparkEnrollmentToken.Value(""))
	if *tokenFile != "" {
		if token != "" {
			return fmt.Errorf("use either --%s or %s", settings.SparkTokenFile.FlagName, settings.SparkEnrollmentToken.EnvironmentName)
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
