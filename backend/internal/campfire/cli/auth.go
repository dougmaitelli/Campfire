package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/settings"
)

func loopbackHost(host string) bool {
	ip, err := netip.ParseAddr(host)
	return host == "localhost" || err == nil && ip.IsLoopback()
}

func adminOrigin(listen, publicURL string) (string, error) {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return "", errors.New("admin listener must specify a host and port")
	}

	if publicURL == "" {
		if !loopbackHost(host) {
			return "", fmt.Errorf("non-loopback admin access requires --%s https://HOST behind an HTTPS reverse proxy", settings.CampfirePublicURL.FlagName)
		}

		return "", nil
	}

	parsed, err := url.Parse(publicURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" || parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("--%s must be an origin without credentials, path, query or fragment", settings.CampfirePublicURL.FlagName)
	}

	localHTTP := parsed.Scheme == "http" && loopbackHost(parsed.Hostname()) && loopbackHost(host)
	if parsed.Scheme != "https" && !localHTTP {
		return "", fmt.Errorf("--%s requires HTTPS except for local loopback development", settings.CampfirePublicURL.FlagName)
	}

	authority := strings.ToLower(parsed.Host)
	if parsed.Scheme == "https" && parsed.Port() == "443" || parsed.Scheme == "http" && parsed.Port() == "80" {
		authority = strings.ToLower(parsed.Hostname())
		if strings.Contains(authority, ":") {
			authority = "[" + authority + "]"
		}
	}

	return parsed.Scheme + "://" + authority, nil
}

func resetPassword(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("campfire reset-password", flag.ContinueOnError)
	dbSource := settings.CampfireDatabase.String(flags, "data/panel.db", "SQLite path or PostgreSQL URL")
	passwordFile := settings.RecoveryPasswordFile.String(flags, "", "private regular file containing the new administrator password")
	username := settings.RecoveryUsername.String(flags, "", "administrator to recover (defaults to the first administrator)")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if flags.NArg() != 0 {
		return fmt.Errorf("usage: campfire reset-password --%s PATH_OR_URL [--%s PRIVATE_FILE]", settings.CampfireDatabase.FlagName, settings.RecoveryPasswordFile.FlagName)
	}

	password, err := recoveryPassword(*passwordFile)
	if err != nil {
		return err
	}
	db, err := database.Open(ctx, *dbSource)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := service.NewAuthService(repository.NewAuthRepository(db.ORM())).ResetPasswordFor(ctx, *username, password); err != nil {
		return fmt.Errorf("reset administrator password: %w", err)
	}
	slog.Info("Administrator password reset; account sessions revoked")
	return nil
}

func recoveryPassword(passwordFile string) (string, error) {
	if passwordFile == "" {
		fd := int(os.Stdin.Fd())
		if !term.IsTerminal(fd) {
			return "", errors.New("interactive password reset requires a terminal; use --password-file for automation")
		}
		return promptRecoveryPassword(os.Stderr, func() ([]byte, error) { return term.ReadPassword(fd) })
	}
	file, err := os.OpenFile(passwordFile, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", errors.New("could not open private password file")
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("password file must be a private regular file (mode 0600)")
	}

	data, err := io.ReadAll(io.LimitReader(file, 1027))
	if err != nil {
		return "", errors.New("could not read password file")
	}

	return strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"), nil
}

func promptRecoveryPassword(output io.Writer, readPassword func() ([]byte, error)) (string, error) {
	if _, err := fmt.Fprintln(output, "Password requirements: "+service.PasswordRequirements()); err != nil {
		return "", err
	}
	if _, err := fmt.Fprint(output, "New password: "); err != nil {
		return "", err
	}
	password, err := readPassword()
	_, _ = fmt.Fprintln(output)
	if err != nil {
		return "", errors.New("could not read password from terminal")
	}
	defer clear(password)
	if _, err := fmt.Fprint(output, "Repeat password: "); err != nil {
		return "", err
	}
	repeated, err := readPassword()
	_, _ = fmt.Fprintln(output)
	if err != nil {
		return "", errors.New("could not read password confirmation from terminal")
	}
	defer clear(repeated)
	if string(password) != string(repeated) {
		return "", errors.New("passwords do not match")
	}
	return string(password), nil
}

func resetTwoFactor(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("campfire reset-2fa", flag.ContinueOnError)
	dbSource := settings.CampfireDatabase.String(flags, "data/panel.db", "SQLite path or PostgreSQL URL")
	username := settings.RecoveryUsername.String(flags, "", "account whose two-factor authentication must be recovered")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*username) == "" || flags.NArg() != 0 {
		return fmt.Errorf("usage: campfire reset-2fa --%s PATH_OR_URL --username ACCOUNT", settings.CampfireDatabase.FlagName)
	}
	db, err := database.Open(ctx, *dbSource)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := service.NewAuthService(repository.NewAuthRepository(db.ORM())).ResetMFA(ctx, *username); err != nil {
		return fmt.Errorf("reset two-factor authentication: %w", err)
	}
	slog.Info("Two-factor authentication removed; browser sessions revoked; API tokens preserved; password sign-in restored")
	return nil
}
