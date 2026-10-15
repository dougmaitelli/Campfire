package agent

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"campfire/internal/campfire/agents"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/agentidentity"
	"campfire/internal/shared/model"
)

func testWireTransfers(t *testing.T, hub *agents.Hub, nodeID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	d := deploymentFixture(nodeID, "wire-transfer")
	if _, err := hub.Deploy(ctx, nodeID, d); err != nil {
		t.Fatal(err)
	}

	q := model.TransferRequest{ID: "wire-transfer-id", ServerID: d.ServerID, Operation: "export"}

	var result model.TransferResult

	for {
		var err error

		result, err = hub.Transfer(ctx, nodeID, q)
		if err != nil {
			t.Fatal(err)
		}

		if result.State == "ready" {
			break
		}

		if result.State == "failed" {
			t.Fatal(result.Error)
		}

		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}

	if _, err := hub.Action(ctx, nodeID, model.RuntimeRequest{ServerID: d.ServerID, Action: "install", OperationID: "fenced-install"}); err == nil {
		t.Fatal("wire transfer did not fence install")
	}

	q.Operation = "read"

	chunk, err := hub.Transfer(ctx, nodeID, q)
	if err != nil || len(chunk.Data) == 0 || chunk.Offset != chunk.Size || agentidentity.Fingerprint(chunk.Data) != chunk.Digest {
		t.Fatal("wire archive chunk was not intact", err)
	}

	if _, err := hex.DecodeString(result.Digest); err != nil {
		t.Fatal(err)
	}

	q.Operation = "release"
	if released, err := hub.Transfer(ctx, nodeID, q); err != nil || released.State != "released" {
		t.Fatal("wire transfer did not release source", err)
	}
}

func testWireRenewal(ctx context.Context, t *testing.T, path, nodeID string, store *repository.EnrollmentRepository, nodes *service.SparkService, dial func(context.Context, string, string) (net.Conn, error)) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var original agentIdentity
	if err := json.Unmarshal(data, &original); err != nil {
		t.Fatal(err)
	}

	updated, err := renewAgent(ctx, original, path, dial)
	if err != nil {
		t.Fatal("mTLS certificate renewal failed", err)
	}

	if !bytes.Equal(updated.Key, original.Key) || updated.ID != original.ID || bytes.Equal(updated.Certificate, original.Certificate) {
		t.Fatal("renewal changed Spark identity or did not replace certificate")
	}

	if err := agentidentity.CheckPermissions(path); err != nil {
		t.Fatal(err)
	}

	repeated, err := renewAgent(ctx, original, path, dial)
	if err != nil || !bytes.Equal(repeated.Certificate, updated.Certificate) {
		t.Fatal("renewal retry did not recover lost response", err)
	}

	oldTLS, err := identityTLS(original)
	if err != nil {
		t.Fatal(err)
	}

	newTLS, err := identityTLS(updated)
	if err != nil {
		t.Fatal(err)
	}

	if !store.Authorized(ctx, nodeID, agentidentity.Fingerprint(oldTLS.Certificates[0].Certificate[0])) || !store.Authorized(ctx, nodeID, agentidentity.Fingerprint(newTLS.Certificates[0].Certificate[0])) {
		t.Fatal("renewal overlap did not authorize both certificates")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(struct {
		CSR []byte `json:"csr"`
	}{pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})})
	if err != nil {
		t.Fatal(err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, original.PanelURL+"/spark/renew", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	transport := &http.Transport{TLSClientConfig: oldTLS, DialContext: dial}
	defer transport.CloseIdleConnections()

	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatal("renewal accepted an unrelated private key", response.StatusCode)
	}

	disabled := true
	if _, err := nodes.Update(ctx, nodeID, model.SparkChanges{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}

	if _, err := renewAgent(ctx, updated, path, dial); err == nil {
		t.Fatal("disabled Spark renewed its certificate")
	}

	if err := nodes.Delete(ctx, nodeID); err != nil {
		t.Fatal(err)
	}

	if store.Authorized(ctx, nodeID, agentidentity.Fingerprint(newTLS.Certificates[0].Certificate[0])) {
		t.Fatal("removed Spark retained access")
	}
}
