// Package sftp exposes a contained, token-authenticated gateway to Spark files.
package sftp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Server accepts only the SFTP subsystem, with one game root per connection.
type Server struct {
	Auth       *service.AuthService
	Deployment *service.DeploymentService
	Audit      *repository.ActivityRepository
	HostKey    ssh.Signer
}

// HostKey loads or exclusively creates a persistent Ed25519 SSH host identity.
func HostKey(name string) (ssh.Signer, error) {
	data, err := os.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return nil, err
		}

		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}

		block, err := ssh.MarshalPrivateKey(key, "")
		if err != nil {
			return nil, err
		}

		data = pem.EncodeToMemory(block)

		file, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}

		_, writeErr := file.Write(data)

		syncErr, closeErr := file.Sync(), file.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			return nil, errors.New("could not persist SFTP host key")
		}
	} else if err != nil {
		return nil, err
	}

	return ssh.ParsePrivateKey(data)
}

// Serve bounds connections and closes sessions when Campfire shuts down.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	var mu sync.Mutex

	connections := make(map[net.Conn]bool)

	var workers sync.WaitGroup

	done := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()

			mu.Lock()
			for conn := range connections {
				_ = conn.Close()
			}
			mu.Unlock()
		case <-done:
		}
	}()

	defer close(done)
	defer workers.Wait()
	defer func() {
		mu.Lock()
		for conn := range connections {
			_ = conn.Close()
		}
		mu.Unlock()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return err
		}

		mu.Lock()
		if len(connections) >= 64 {
			mu.Unlock()

			_ = conn.Close()

			continue
		}

		connections[conn] = true
		mu.Unlock()

		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { _ = conn.Close(); mu.Lock(); delete(connections, conn); mu.Unlock() }()

			s.connection(ctx, conn)
		}()
	}
}

func (s *Server) connection(ctx context.Context, conn net.Conn) {
	var token, serverID string

	config := &ssh.ServerConfig{MaxAuthTries: 3, ServerVersion: "SSH-2.0-Campfire", PasswordCallback: func(metadata ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		authCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		id := metadata.User()
		if len(id) == 0 || len(id) > 128 {
			return nil, os.ErrPermission
		}

		for _, r := range id {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
				return nil, os.ErrPermission
			}
		}

		principal, err := s.Auth.AuthenticateAPIToken(authCtx, string(password))
		if err != nil {
			return nil, os.ErrPermission
		}

		permissions, err := s.Auth.Access().ServerPermissions(authCtx, principal, id)
		if err != nil || !slices.Contains(permissions, accessperm.ServerView) || !slices.Contains(permissions, accessperm.ServerFilesRead) {
			return nil, os.ErrPermission
		}

		token, serverID = string(password), id

		return nil, nil
	}}
	config.AddHostKey(s.HostKey)

	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	peer, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}

	defer func() { _ = peer.Close() }()

	_ = conn.SetDeadline(time.Now().Add(45 * time.Minute))

	go ssh.DiscardRequests(requests)

	channel, ok := <-channels
	if !ok {
		return
	}

	go func() {
		for extra := range channels {
			_ = extra.Reject(ssh.Prohibited, "Only one SFTP session is allowed.")
		}
	}()

	if channel.ChannelType() != "session" {
		_ = channel.Reject(ssh.UnknownChannelType, "Only SFTP is available.")
		return
	}

	stream, incoming, err := channel.Accept()
	if err != nil {
		return
	}

	defer func() { _ = stream.Close() }()

	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for request := range incoming {
		var subsystem struct{ Name string }
		if request.Type != "subsystem" || ssh.Unmarshal(request.Payload, &subsystem) != nil || subsystem.Name != "sftp" {
			_ = request.Reply(false, nil)
			continue
		}

		_ = request.Reply(true, nil)

		go ssh.DiscardRequests(incoming)

		handler := &fileHandler{server: s, ctx: sessionCtx, token: token, id: serverID}

		files := sftp.NewRequestServer(stream, sftp.Handlers{FileGet: handler, FilePut: handler, FileCmd: handler, FileList: handler})
		defer func() { _ = files.Close() }()

		_ = files.Serve()

		return
	}
}

func safeError(err error) error {
	if err == nil {
		return nil
	}

	var failure *model.FileError
	if errors.As(err, &failure) {
		switch failure.Code {
		case "file_not_found":
			return os.ErrNotExist
		case "invalid_file":
			return sftp.ErrSSHFxBadMessage
		case "file_conflict", "file_busy":
			return sftp.ErrSSHFxFailure
		case "file_limit":
			return sftp.ErrSSHFxFailure
		}
	}

	if errors.Is(err, io.EOF) {
		return io.EOF
	}

	return os.ErrPermission
}
