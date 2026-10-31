package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"slices"
	"testing"
	"time"

	"campfire/internal/campfire/agents"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
	"campfire/internal/spark/runtime"

	"github.com/gorilla/websocket"
)

func TestQuotaCapabilityReflectsRuntimeSupport(t *testing.T) {
	manager := &runtime.Manager{}
	store := &ConfigurationStore{Runtime: manager}

	offered := store.capabilities()
	if slices.Contains(offered, protocol.CapabilityDiskQuota) {
		t.Fatal("advertised disk quotas without filesystem support")
	}

	_, selected, err := protocol.Negotiate(protocol.Message{Protocol: protocol.ProtocolVersion, Capabilities: offered})
	if err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"install", "start", "restart", "restore", "update", "reinstall"} {
		command := protocol.Command{Name: protocol.ServerAction, Runtime: &model.RuntimeRequest{Action: model.ServerAction(action)}}
		if err := protocol.Require(selected, protocol.RequiredCapabilities(command)...); err != nil {
			t.Fatal("optional quotas blocked server operation", action, err)
		}
	}

	if err := protocol.Require(selected, protocol.RequiredCapabilities(protocol.Command{Name: protocol.ServerTransfer})...); err != nil {
		t.Fatal("optional quotas blocked transfer", err)
	}

	manager.Quota = wireQuota{}

	if !slices.Contains(store.capabilities(), protocol.CapabilityDiskQuota) {
		t.Fatal("working disk quotas were not advertised")
	}
}

func testWireCapabilities(t *testing.T, hub *agents.Hub, id string, config *tls.Config, dial func(context.Context, string, string) (net.Conn, error)) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dialer := websocket.Dialer{TLSClientConfig: config, NetDialContext: dial, HandshakeTimeout: time.Second}
	connect := func(hello protocol.Message) *websocket.Conn {
		t.Helper()

		conn, response, err := dialer.DialContext(ctx, "wss://panel.test/spark/connect", nil)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}

		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _ = conn.Close() })

		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatal(err)
		}

		if err := conn.WriteJSON(hello); err != nil {
			t.Fatal(err)
		}

		return conn
	}
	hello := protocol.Message{Type: "hello", SparkID: id, Protocol: protocol.ProtocolVersion + 1, MinimumProtocol: protocol.MinimumProtocolVersion, Capabilities: []string{protocol.CapabilityLifecycle, "future.feature.v1"}}
	conn := connect(hello)

	var welcome protocol.Message
	if err := conn.ReadJSON(&welcome); err != nil {
		t.Fatal(err)
	}

	if welcome.Protocol != protocol.ProtocolVersion || !slices.Equal(welcome.Capabilities, []string{protocol.CapabilityLifecycle}) {
		t.Fatal("partial handshake selected wrong features", welcome)
	}

	waitCondition(t, func() bool { return hub.Connected(id) })

	_, err := hub.Action(ctx, id, model.RuntimeRequest{ServerID: "game", Action: "metrics"})

	var failure *protocol.CommandError
	if !errors.As(err, &failure) || failure.Code != "unsupported_capability" {
		t.Fatal("unsupported metrics dispatched", err)
	}

	complete := make(chan error, 1)

	go func() {
		_, err := hub.Action(ctx, id, model.RuntimeRequest{ServerID: "game", Action: "status"})
		complete <- err
	}()

	var command protocol.Message
	if err := conn.ReadJSON(&command); err != nil {
		t.Fatal(err)
	}

	if command.Protocol != welcome.Protocol || command.Command == nil || command.Command.Runtime.Action != "status" {
		t.Fatal("session did not use negotiated version", command)
	}

	if err := conn.WriteJSON(protocol.Message{Type: "response", Protocol: welcome.Protocol, Response: &protocol.CommandResponse{ID: command.Command.ID, Status: &model.RuntimeStatus{ServerID: "game", State: "stopped"}}}); err != nil {
		t.Fatal(err)
	}

	if err := <-complete; err != nil {
		t.Fatal(err)
	}
	// A session cannot switch versions after its handshake.
	if err := conn.WriteJSON(protocol.Message{Type: "heartbeat", Protocol: hello.Protocol}); err != nil {
		t.Fatal(err)
	}

	if err := conn.ReadJSON(&welcome); err == nil {
		t.Fatal("post-handshake protocol change accepted")
	}

	_ = conn.Close()

	waitCondition(t, func() bool { return !hub.Connected(id) })

	hello.Protocol = protocol.ProtocolVersion
	hello.Capabilities = protocol.SupportedCapabilities()

	conn = connect(hello)
	if err := conn.ReadJSON(&welcome); err != nil {
		t.Fatal(err)
	}

	waitCondition(t, func() bool { return hub.Connected(id) })

	if err := hub.CheckCapabilities(id, protocol.CapabilityMetrics); err != nil {
		t.Fatal("reconnect retained stale capability set", err)
	}

	_ = conn.Close()

	waitCondition(t, func() bool { return !hub.Connected(id) })

	hello.Protocol, hello.MinimumProtocol = protocol.ProtocolVersion+1, protocol.ProtocolVersion+1

	conn = connect(hello)
	if err := conn.ReadJSON(&welcome); err == nil {
		t.Fatal("incompatible wire range accepted")
	}

	_ = conn.Close()

	if hub.Connected(id) {
		t.Fatal("incompatible session registered")
	}
}
