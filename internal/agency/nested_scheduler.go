package agency

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// NestedScheduler wraps Scheduler to support a tree of schedule nodes.
// Each node may carry a prompt_injection directive that is embedded in the
// fired WakeSignal payload and consumed by the actor daemon as a high-weight
// GIST directive atom.
type NestedScheduler struct {
	inner *Scheduler
	mu    sync.RWMutex
	nodes map[string]ScheduleNode
}

// NewNestedScheduler creates a NestedScheduler backed by the given event bus.
func NewNestedScheduler(bus EventBus) *NestedScheduler {
	return &NestedScheduler{
		inner: NewScheduler(bus),
		nodes: make(map[string]ScheduleNode),
	}
}

// RegisterNode adds a node to the schedule tree and starts its cron ticker.
// The node's PromptInjection (if set) is embedded in every WakeSignal fired.
func (n *NestedScheduler) RegisterNode(ctx context.Context, node ScheduleNode) error {
	if node.ID == "" {
		node.ID = uuid.NewString()
	}
	if node.ActorID == "" {
		return fmt.Errorf("schedule node: actor_id is required")
	}

	n.mu.Lock()
	n.nodes[node.ID] = node
	n.mu.Unlock()

	schedule := AgentSchedule{
		ID:                node.ID,
		ActorID:           node.ActorID,
		Expression:        node.Expression,
		Enabled:           node.Enabled,
		DefaultSignalKind: SignalSchedule,
	}

	payload := map[string]string{
		"schedule_node_id": node.ID,
		"schedule_layer":   fmt.Sprintf("%d", node.Layer),
	}
	if node.PromptInjection != "" {
		payload["prompt_injection"] = node.PromptInjection
	}
	if node.ParentID != "" {
		payload["parent_node_id"] = node.ParentID
	}

	base := WakeSignal{
		OrganizationID: node.OrganizationID,
		Channel:        ActorChannel(node.ActorID),
		Kind:           SignalSchedule,
		Payload:        payload,
	}

	return n.inner.Register(ctx, schedule, base)
}

// Nodes returns a snapshot of all registered schedule nodes.
func (n *NestedScheduler) Nodes() []ScheduleNode {
	n.mu.RLock()
	defer n.mu.RUnlock()
	out := make([]ScheduleNode, 0, len(n.nodes))
	for _, node := range n.nodes {
		out = append(out, node)
	}
	return out
}

// Stop cancels all running schedule tickers.
func (n *NestedScheduler) Stop() {
	n.inner.Stop()
}

// SetPublisher delegates to the inner Scheduler's SetPublisher.
func (n *NestedScheduler) SetPublisher(fn func(context.Context, AgentSchedule, WakeSignal) error) {
	n.inner.SetPublisher(fn)
}
