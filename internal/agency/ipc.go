package agency

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// IPCMessageType identifies the kind of event sent over the socket.
type IPCMessageType string

const (
	IPCTypeBroadcast IPCMessageType = "broadcast"
	IPCTypeApproval  IPCMessageType = "approval"
	IPCTypeBulletin  IPCMessageType = "bulletin"
	IPCTypeVote      IPCMessageType = "vote"
	IPCTypeHandshake IPCMessageType = "handshake"
	IPCTypePing      IPCMessageType = "ping"
	IPCTypePong      IPCMessageType = "pong"
)

// IPCMessage is the newline-delimited JSON envelope exchanged over the socket.
type IPCMessage struct {
	Type    IPCMessageType  `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// IPCHandshake is the first message a client sends after connecting.
type IPCHandshake struct {
	OrgID      string `json:"orgId"`
	ClientType string `json:"clientType"` // "desktop", "cli", "web"
}

// IPCVote is sent by a client to approve or reject a pending proposal.
type IPCVote struct {
	ProposalID string `json:"proposalId"`
	Approved   bool   `json:"approved"`
}

// IPCBroadcastPayload carries a live agent message.
type IPCBroadcastPayload struct {
	ActorID   string `json:"actorId"`
	Message   string `json:"message"`
	CreatedAt int64  `json:"createdAt"`
}

// IPCApprovalPayload carries a pending action proposal.
type IPCApprovalPayload struct {
	ProposalID  string `json:"proposalId"`
	ActorID     string `json:"actorId"`
	ActionType  string `json:"actionType"`
	Target      string `json:"target"`
	GISTVerdict string `json:"gistVerdict,omitempty"`
	GISTRisk    string `json:"gistRisk,omitempty"`
	GISTTraceID string `json:"gistTraceId,omitempty"`
	GISTReason  string `json:"gistReason,omitempty"`
	CreatedAt   int64  `json:"createdAt"`
}

// IPCBulletinPayload carries a performance record.
type IPCBulletinPayload struct {
	ActorID   string  `json:"actorId"`
	Directive string  `json:"directive"`
	Output    string  `json:"output"`
	Score     float64 `json:"score"`
	Provider  string  `json:"provider"`
	ModelID   string  `json:"modelId"`
	CreatedAt int64   `json:"createdAt"`
}

// IPCServer listens on a Unix socket and fans out office events to connected clients.
// Clients subscribe by sending a handshake; they can send votes back.
type IPCServer struct {
	socketPath string
	bus        EventBus
	mu         sync.RWMutex
	clients    map[string]*ipcClient // keyed by orgID
	listener   net.Listener
}

type ipcClient struct {
	orgID string
	conn  net.Conn
	send  chan IPCMessage
}

// IPCSocketPath returns the conventional socket path for an organization.
func IPCSocketPath(baseDir, orgID string) string {
	return filepath.Join(baseDir, ".agency", fmt.Sprintf("ipc-%s.sock", orgID))
}

// NewIPCServer creates an IPC server that will listen on socketPath.
func NewIPCServer(socketPath string, bus EventBus) *IPCServer {
	return &IPCServer{
		socketPath: socketPath,
		bus:        bus,
		clients:    make(map[string]*ipcClient),
	}
}

// Serve starts accepting connections. Blocks until ctx is cancelled.
func (s *IPCServer) Serve(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0o755); err != nil {
		return fmt.Errorf("ipc: create socket dir: %w", err)
	}
	_ = os.Remove(s.socketPath) // remove stale socket

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("ipc: listen %s: %w", s.socketPath, err)
	}
	s.listener = ln
	defer func() {
		ln.Close()
		os.Remove(s.socketPath)
	}()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("ipc: accept: %w", err)
			}
		}
		go s.handleConn(ctx, conn)
	}
}

func (s *IPCServer) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)

	// First message must be a handshake.
	if !scanner.Scan() {
		return
	}
	var msg IPCMessage
	if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
		return
	}
	if msg.Type != IPCTypeHandshake {
		return
	}
	var hs IPCHandshake
	if err := json.Unmarshal(msg.Payload, &hs); err != nil || hs.OrgID == "" {
		return
	}

	client := &ipcClient{
		orgID: hs.OrgID,
		conn:  conn,
		send:  make(chan IPCMessage, 64),
	}

	// Register client.
	s.mu.Lock()
	s.clients[hs.OrgID+":"+fmt.Sprintf("%p", conn)] = client
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, hs.OrgID+":"+fmt.Sprintf("%p", conn))
		s.mu.Unlock()
	}()

	// Subscribe to all three channels for this org.
	broadcastCh, _ := s.bus.Subscribe(ctx, OrganizationChannel(hs.OrgID))
	approvalCh, _ := s.bus.Subscribe(ctx, ApprovalChannel(hs.OrgID))
	bulletinCh, _ := s.bus.Subscribe(ctx, BulletinChannel(hs.OrgID))

	// Write pump.
	go func() {
		for m := range client.send {
			if err := encoder.Encode(m); err != nil {
				return
			}
		}
	}()

	// Fan-in from bus channels → client send queue.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-broadcastCh:
				if !ok {
					return
				}
				if sig.Kind != SignalBroadcast {
					continue
				}
				p, _ := json.Marshal(IPCBroadcastPayload{
					ActorID:   sig.ActorID,
					Message:   sig.Payload["message"],
					CreatedAt: sig.CreatedAt,
				})
				select {
				case client.send <- IPCMessage{Type: IPCTypeBroadcast, Payload: p}:
				default:
				}

			case sig, ok := <-approvalCh:
				if !ok {
					return
				}
				if sig.Kind != SignalReview {
					continue
				}
				p, _ := json.Marshal(IPCApprovalPayload{
					ProposalID:  sig.Payload["proposalId"],
					ActorID:     sig.ActorID,
					ActionType:  sig.Payload["actionType"],
					Target:      sig.Payload["target"],
					GISTVerdict: sig.Payload["gistVerdict"],
					GISTRisk:    sig.Payload["gistRisk"],
					GISTTraceID: sig.Payload["gistTraceId"],
					GISTReason:  sig.Payload["gistReason"],
					CreatedAt:   sig.CreatedAt,
				})
				select {
				case client.send <- IPCMessage{Type: IPCTypeApproval, Payload: p}:
				default:
				}

			case sig, ok := <-bulletinCh:
				if !ok {
					return
				}
				if sig.Kind != SignalProjection {
					continue
				}
				score := 0.0
				fmt.Sscanf(sig.Payload["score"], "%f", &score)
				p, _ := json.Marshal(IPCBulletinPayload{
					ActorID:   sig.ActorID,
					Directive: sig.Payload["directive"],
					Output:    sig.Payload["output"],
					Score:     score,
					Provider:  sig.Payload["provider"],
					ModelID:   sig.Payload["modelId"],
					CreatedAt: sig.CreatedAt,
				})
				select {
				case client.send <- IPCMessage{Type: IPCTypeBulletin, Payload: p}:
				default:
				}
			}
		}
	}()

	// Read pump — handle incoming votes + pings.
	for scanner.Scan() {
		var in IPCMessage
		if err := json.Unmarshal(scanner.Bytes(), &in); err != nil {
			continue
		}
		switch in.Type {
		case IPCTypePing:
			pong, _ := json.Marshal(IPCMessage{Type: IPCTypePong})
			conn.Write(append(pong, '\n'))

		case IPCTypeVote:
			var vote IPCVote
			if err := json.Unmarshal(in.Payload, &vote); err == nil && vote.ProposalID != "" {
				action := "reject"
				if vote.Approved {
					action = "approve"
				}
				_ = s.bus.Publish(ctx, WakeSignal{
					OrganizationID: hs.OrgID,
					Channel:        OrganizationChannel(hs.OrgID),
					Kind:           SignalCorrection,
					Payload: map[string]string{
						"proposalId": vote.ProposalID,
						"vote":       action,
						"source":     "ipc." + hs.ClientType,
					},
					CreatedAt: time.Now().UnixMilli(),
				})
			}
		}
	}
}

// Broadcast sends a message directly to all connected clients for an org.
// Used for server-initiated notifications (e.g. office boot/stop).
func (s *IPCServer) Broadcast(orgID string, msg IPCMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefix := orgID + ":"
	for key, client := range s.clients {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			select {
			case client.send <- msg:
			default:
			}
		}
	}
}
