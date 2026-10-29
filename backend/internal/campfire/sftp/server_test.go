package sftp

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"

	pkg "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type testFiles struct {
	mu            sync.Mutex
	data, staging []byte
	calls         int
}

func (f *testFiles) Deploy(_ context.Context, _ string, d model.Deployment) (model.ConfigurationReceipt, error) {
	return model.ConfigurationReceipt{ServerID: d.ServerID}, nil
}
func (f *testFiles) Files(_ context.Context, node string, q model.FileRequest) (model.FileResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	result := model.FileResult{ServerID: q.ServerID, Path: q.Path, Revision: "revision", Size: int64(len(f.data))}
	if node != "node" || q.ServerID != "game" {
		return result, os.ErrPermission
	}

	if q.Path != "world" {
		return result, &model.FileError{Code: "file_not_found", Message: "Missing"}
	}

	switch q.Operation {
	case "stat":
		if f.data == nil {
			return result, &model.FileError{Code: "file_not_found", Message: "Missing"}
		}

		result.Entries = []model.FileEntry{{Name: "world", Kind: "file", Size: result.Size, ModifiedAt: time.Now()}}
	case "upload.begin":
		f.staging = []byte{}
		result.UploadID = "upload"
	case "upload.chunk":
		end := q.Offset + int64(len(q.Data))
		if end > int64(len(f.staging)) {
			f.staging = append(f.staging, make([]byte, int(end)-len(f.staging))...)
		}

		copy(f.staging[q.Offset:], q.Data)
	case "upload.commit":
		f.data = f.staging
		f.staging = nil
	case "upload.cancel":
		f.staging = nil
	case "read.chunk":
		end := min(q.Offset+q.Size, int64(len(f.data)))
		result.Data = bytes.Clone(f.data[q.Offset:end])
	}

	return result, nil
}

func TestSFTPGatewayTransfersAndLiveTokenRevocation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	for _, item := range []any{
		&database.User{ID: 2, Username: "owner", RoleID: database.UserRole, CredentialVersion: 1},
		&database.Spark{ID: "node", Name: "Node", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
		&database.Ember{ID: "ember", Name: "Game", Definition: model.Ember{Schema: 1, ID: "ember", Name: "Game"}},
		&database.Server{OwnerUserID: new(int64(2)), ID: "game", Name: "Game", SparkID: "node", EmberID: "ember", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	} {
		if err := db.ORM().Omit("Role", "Spark", "Ember", "Server", "User").Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}

	auth := service.NewAuthService(repository.NewAuthRepository(db.ORM()))
	owner := service.Principal{UserID: 2, CredentialVersion: 1}

	token, raw, err := auth.CreateAPIToken(ctx, owner, "SFTP", 1, []string{"server.view", "server.files.delete"})
	if err != nil {
		t.Fatal(err)
	}

	keyPath := filepath.Join(t.TempDir(), "host_key")

	key, err := HostKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}

	same, err := HostKey(keyPath)
	if err != nil || !bytes.Equal(key.PublicKey().Marshal(), same.PublicKey().Marshal()) {
		t.Fatal("host key did not persist", err)
	}

	remote := &testFiles{}
	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	deployment := &service.DeploymentService{Servers: service.NewServerService(repository.NewServerRepository(db.ORM()), embers), Sparks: service.NewSparkService(repository.NewSparkRepository(db.ORM()), nil), Remote: remote, MaxUploadBytes: 2 * model.FileLimit}
	gateway := &Server{Auth: auth, Deployment: deployment, Audit: repository.NewActivityRepository(db.ORM()), HostKey: key}
	listenConfig := net.ListenConfig{}

	listener, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	stopped := make(chan error, 1)
	go func() { stopped <- gateway.Serve(ctx, listener) }()

	defer func() {
		cancel()

		if err := <-stopped; err != nil {
			t.Error(err)
		}
	}()

	dial := func(secret string) (*ssh.Client, error) {
		return ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{User: "game", Auth: []ssh.AuthMethod{ssh.Password(secret)}, HostKeyCallback: ssh.FixedHostKey(key.PublicKey()), Timeout: 5 * time.Second})
	}

	_, restricted, err := auth.CreateAPIToken(ctx, owner, "read only", 1, []string{"server.view"})
	if err != nil {
		t.Fatal(err)
	}

	if conn, err := dial(restricted); err == nil {
		_ = conn.Close()

		t.Fatal("token without file access authenticated")
	}

	connection, err := dial(raw)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = connection.Close() }()

	client, err := pkg.NewClient(connection)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = client.Close() }()

	file, err := client.Create("/world")
	if err != nil {
		t.Fatal(err)
	}

	data := bytes.Repeat([]byte{42}, 2*model.FileLimit-3)
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	file, err = client.Open("/world")
	if err != nil {
		t.Fatal(err)
	}

	_, readSecret, err := auth.CreateAPIToken(ctx, owner, "file reader", 1, []string{accessperm.ServerView, accessperm.ServerFilesRead})
	if err != nil {
		t.Fatal(err)
	}

	readConnection, err := dial(readSecret)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = readConnection.Close() }()

	reader, err := pkg.NewClient(readConnection)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = reader.Close() }()

	readFile, err := reader.Open("/world")
	if err != nil {
		t.Fatal(err)
	}

	readData, err := io.ReadAll(readFile)
	if err != nil || !bytes.Equal(readData, data) {
		t.Fatal("read-only SFTP failed", err)
	}

	_ = readFile.Close()
	if created, err := reader.Create("/blocked"); err == nil {
		_ = created.Close()

		t.Fatal("read-only SFTP created a file")
	}

	if err := reader.Chmod("/world", 0o600); err == nil {
		t.Fatal("read-only SFTP changed permissions")
	}

	if err := reader.Remove("/world"); err == nil {
		t.Fatal("read-only SFTP deleted a file")
	}

	read, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(read, data) {
		t.Fatal("SFTP data changed", err, len(read))
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	file, err = client.Open("/world")
	if err != nil {
		t.Fatal(err)
	}

	tooLarge, err := client.Create("/world")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tooLarge.Write(make([]byte, 2*model.FileLimit+1)); err == nil {
		t.Fatal("SFTP ignored the configured upload limit")
	}

	_ = tooLarge.Close()

	if err := auth.RevokeAPIToken(ctx, owner, token.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := file.Read(make([]byte, 10)); err == nil {
		t.Fatal("revoked token read from an open handle")
	}

	_ = file.Close()

	var audits int64
	if err := db.ORM().Model(&database.AuditEvent{}).Where("api_token_id = ? AND action = 'server.files.sftp.upload.commit'", token.ID).Count(&audits).Error; err != nil || audits != 1 {
		t.Fatal("missing token-attributed SFTP audit", audits, err)
	}
}
