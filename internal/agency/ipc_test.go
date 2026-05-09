package agency

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIPCServerFansOutOfficeEvents(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	orgID := "org-ipc-test"
	bus := NewMemoryEventBus()
	socketDir, err := os.MkdirTemp("/tmp", "agency-ipc-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	requireUnixSocketBind(t, filepath.Join(socketDir, "probe.sock"))
	socketPath := filepath.Join(socketDir, "ipc.sock")
	server := NewIPCServer(socketPath, bus)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
				t.Fatalf("ipc server returned unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("ipc server did not stop")
		}
	})

	conn := dialUnixSocket(t, socketPath)
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writeIPCMessage(t, conn, IPCMessage{
		Type: IPCTypeHandshake,
		Payload: mustJSON(t, IPCHandshake{
			OrgID:      orgID,
			ClientType: "cli",
		}),
	})

	// Give the server a brief moment to register channel subscriptions after
	// the handshake before publishing test events.
	require.Eventually(t, func() bool {
		return memoryBusHasSubscriber(bus, OrganizationChannel(orgID)) &&
			memoryBusHasSubscriber(bus, ApprovalChannel(orgID)) &&
			memoryBusHasSubscriber(bus, BulletinChannel(orgID))
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, bus.Publish(ctx, WakeSignal{
		OrganizationID: orgID,
		ActorID:        "tester",
		Channel:        OrganizationChannel(orgID),
		Kind:           SignalBroadcast,
		Payload:        map[string]string{"message": "hello office"},
		CreatedAt:      time.Now().UnixMilli(),
	}))
	require.NoError(t, bus.Publish(ctx, WakeSignal{
		OrganizationID: orgID,
		ActorID:        "tester",
		Channel:        ApprovalChannel(orgID),
		Kind:           SignalReview,
		Payload: map[string]string{
			"proposalId": "prop-1",
			"actionType": string(ActionBroadcast),
			"target":     "team",
		},
		CreatedAt: time.Now().UnixMilli(),
	}))
	require.NoError(t, bus.Publish(ctx, WakeSignal{
		OrganizationID: orgID,
		ActorID:        "tester",
		Channel:        BulletinChannel(orgID),
		Kind:           SignalProjection,
		Payload: map[string]string{
			"directive": "ship",
			"output":    "done",
			"score":     "0.91",
			"provider":  "test",
			"modelId":   "model-test",
		},
		CreatedAt: time.Now().UnixMilli(),
	}))

	seen := map[IPCMessageType]IPCMessage{}
	require.Eventually(t, func() bool {
		_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		msg, err := readIPCMessage(reader)
		if err == nil {
			seen[msg.Type] = msg
		}
		return len(seen) >= 3
	}, 2*time.Second, 25*time.Millisecond)

	require.Contains(t, seen, IPCTypeBroadcast)
	require.Contains(t, seen, IPCTypeApproval)
	require.Contains(t, seen, IPCTypeBulletin)

	var broadcast IPCBroadcastPayload
	require.NoError(t, json.Unmarshal(seen[IPCTypeBroadcast].Payload, &broadcast))
	require.Equal(t, "tester", broadcast.ActorID)
	require.Equal(t, "hello office", broadcast.Message)

	var approval IPCApprovalPayload
	require.NoError(t, json.Unmarshal(seen[IPCTypeApproval].Payload, &approval))
	require.Equal(t, "prop-1", approval.ProposalID)
	require.Equal(t, string(ActionBroadcast), approval.ActionType)

	var bulletin IPCBulletinPayload
	require.NoError(t, json.Unmarshal(seen[IPCTypeBulletin].Payload, &bulletin))
	require.Equal(t, "ship", bulletin.Directive)
	require.Equal(t, "done", bulletin.Output)
	require.InDelta(t, 0.91, bulletin.Score, 0.001)
}

func requireUnixSocketBind(t *testing.T, socketPath string) {
	t.Helper()

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("unix socket bind unavailable in this environment: %v", err)
	}
	require.NoError(t, ln.Close())
	_ = os.Remove(socketPath)
}

func memoryBusHasSubscriber(bus *MemoryEventBus, channel string) bool {
	bus.mu.RLock()
	defer bus.mu.RUnlock()
	return len(bus.subs[channel]) > 0
}

func dialUnixSocket(t *testing.T, socketPath string) net.Conn {
	t.Helper()

	var lastErr error
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			return conn
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("dial unix socket %s: %v", socketPath, lastErr)
	return nil
}

func writeIPCMessage(t *testing.T, conn net.Conn, msg IPCMessage) {
	t.Helper()

	data, err := json.Marshal(msg)
	require.NoError(t, err)
	_, err = conn.Write(append(data, '\n'))
	require.NoError(t, err)
}

func readIPCMessage(reader *bufio.Reader) (IPCMessage, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return IPCMessage{}, err
	}
	var msg IPCMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return IPCMessage{}, err
	}
	return msg, nil
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}
