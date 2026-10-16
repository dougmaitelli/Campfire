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

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
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
			return "", errors.New("non-loopback admin access requires --public-url https://HOST behind an HTTPS reverse proxy")
		}

		return "", nil
	}

	parsed, err := url.Parse(publicURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" || parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("--public-url must be an origin without credentials, path, query or fragment")
	}

	localHTTP := parsed.Scheme == "http" && loopbackHost(parsed.Hostname()) && loopbackHost(host)
	if parsed.Scheme != "https" && !localHTTP {
		return "", errors.New("--public-url requires HTTPS except for local loopback development")
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
	dbSource := flags.String("database", environment("CAMPFIRE_DATABASE", "data/panel.db"), "SQLite path or PostgreSQL URL (CAMPFIRE_DATABASE)")
	passwordFile := flags.String("password-file", "", "private regular file containing the new administrator password")
	username := flags.String("username", "", "administrator to recover (defaults to the first administrator)")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if *passwordFile == "" || flags.NArg() != 0 {
		return errors.New("usage: campfire reset-password --database PATH_OR_URL --password-file PRIVATE_FILE")
	}

	file, err := os.OpenFile(*passwordFile, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return errors.New("could not open private password file")
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("password file must be a private regular file (mode 0600)")
	}

	data, err := io.ReadAll(io.LimitReader(file, 1027))
	if err != nil {
		return errors.New("could not read password file")
	}

	password := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")

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
