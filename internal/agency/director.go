package agency

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type DirectorTicketStatus string

const (
	DirectorTicketOpen       DirectorTicketStatus = "open"
	DirectorTicketDispatched DirectorTicketStatus = "dispatched"
	DirectorTicketClosed     DirectorTicketStatus = "closed"
)

type DirectorRisk string

const (
	DirectorRiskLow     DirectorRisk = "low"
	DirectorRiskMedium  DirectorRisk = "medium"
	DirectorRiskHigh    DirectorRisk = "high"
	DirectorRiskUnknown DirectorRisk = "unknown"
)

type DirectorPriority string

const (
	DirectorPriorityLow    DirectorPriority = "low"
	DirectorPriorityNormal DirectorPriority = "normal"
	DirectorPriorityHigh   DirectorPriority = "high"
	DirectorPriorityUrgent DirectorPriority = "urgent"
)

type DirectorPolicy struct {
	AutoDispatchRisks         []DirectorRisk     `json:"autoDispatchRisks"`
	AutoDispatchPriorities    []DirectorPriority `json:"autoDispatchPriorities"`
	RequireApprovalRisks      []DirectorRisk     `json:"requireApprovalRisks"`
	RequireApprovalPriorities []DirectorPriority `json:"requireApprovalPriorities"`
	PauseWhenApprovalsPending bool               `json:"pauseWhenApprovalsPending"`
}

func DefaultDirectorPolicy() DirectorPolicy {
	return DirectorPolicy{
		AutoDispatchRisks:         []DirectorRisk{DirectorRiskLow},
		AutoDispatchPriorities:    []DirectorPriority{DirectorPriorityLow, DirectorPriorityNormal},
		RequireApprovalRisks:      []DirectorRisk{DirectorRiskMedium, DirectorRiskHigh, DirectorRiskUnknown},
		RequireApprovalPriorities: []DirectorPriority{DirectorPriorityHigh, DirectorPriorityUrgent},
		PauseWhenApprovalsPending: true,
	}
}

type DirectorTicket struct {
	ID             string               `json:"id"`
	OrganizationID string               `json:"organizationId"`
	Title          string               `json:"title"`
	Body           string               `json:"body"`
	Source         string               `json:"source"`
	Priority       string               `json:"priority,omitempty"`
	Risk           string               `json:"risk,omitempty"`
	Status         DirectorTicketStatus `json:"status"`
	AssignedRole   string               `json:"assignedRole,omitempty"`
	CreatedAt      int64                `json:"createdAt"`
	UpdatedAt      int64                `json:"updatedAt"`
	DispatchedAt   int64                `json:"dispatchedAt,omitempty"`
	LastSummary    string               `json:"lastSummary,omitempty"`
}

type DirectorEvent struct {
	ID             string            `json:"id"`
	OrganizationID string            `json:"organizationId"`
	TicketID       string            `json:"ticketId,omitempty"`
	Kind           string            `json:"kind"`
	Message        string            `json:"message"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	CreatedAt      int64             `json:"createdAt"`
}

type DirectorStatus struct {
	Agent            AgentIdentity  `json:"agent"`
	BaseDir          string         `json:"baseDir"`
	OrganizationID   string         `json:"organizationId"`
	SharedWorkplace  string         `json:"sharedWorkplace"`
	OpenTickets      int            `json:"openTickets"`
	Dispatched       int            `json:"dispatched"`
	ClosedTickets    int            `json:"closedTickets"`
	LedgerSequence   int64          `json:"ledgerSequence"`
	PendingApprovals int            `json:"pendingApprovals"`
	LastSignal       *WakeSignal    `json:"lastSignal,omitempty"`
	LastEvent        *DirectorEvent `json:"lastEvent,omitempty"`
	UpdatedAt        int64          `json:"updatedAt"`
}

type DirectorTicketRequest struct {
	Title        string `json:"title"`
	Body         string `json:"body"`
	Source       string `json:"source"`
	Priority     string `json:"priority,omitempty"`
	Risk         string `json:"risk,omitempty"`
	AssignedRole string `json:"assignedRole,omitempty"`
	AutoDispatch bool   `json:"autoDispatch,omitempty"`
}

type DirectorConfig struct {
	BaseDir         string
	OrganizationID  string
	SharedWorkplace string
	Ledger          *LedgerService
	Bus             EventBus
	Router          *ModelRouter
	Policy          DirectorPolicy
}

type DirectorService struct {
	cfg          DirectorConfig
	agent        *DirectorAgent
	mu           sync.Mutex
	dir          string
	ticketsPath  string
	eventsPath   string
	// traceFetcher is the read-only side door used by the lattice
	// inspector route. Optional - when nil, /lattice/* returns 503.
	traceFetcher GISTTraceFetcher
}

func NewDirectorService(cfg DirectorConfig) (*DirectorService, error) {
	if cfg.BaseDir == "" {
		return nil, fmt.Errorf("director base dir is required")
	}
	if cfg.OrganizationID == "" {
		return nil, fmt.Errorf("director organization id is required")
	}
	if cfg.Ledger == nil {
		return nil, fmt.Errorf("director ledger is required")
	}
	if cfg.Bus == nil {
		cfg.Bus = NewMemoryEventBus()
	}
	if len(cfg.Policy.AutoDispatchRisks) == 0 && len(cfg.Policy.RequireApprovalRisks) == 0 {
		cfg.Policy = DefaultDirectorPolicy()
	}
	dir := filepath.Join(cfg.BaseDir, "director")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	svc := &DirectorService{
		cfg:         cfg,
		dir:         dir,
		ticketsPath: filepath.Join(dir, "tickets.jsonl"),
		eventsPath:  filepath.Join(dir, "events.jsonl"),
	}
	svc.agent = NewDirectorAgent(cfg.OrganizationID)
	for _, path := range []string{svc.ticketsPath, svc.eventsPath} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				return nil, err
			}
		}
	}
	return svc, nil
}

func (d *DirectorService) Agent() *DirectorAgent {
	return d.agent
}

func (d *DirectorService) SubmitTicket(ctx context.Context, req DirectorTicketRequest) (DirectorTicket, error) {
	if d == nil {
		return DirectorTicket{}, fmt.Errorf("director service is nil")
	}
	now := time.Now().UnixMilli()
	title := strings.TrimSpace(req.Title)
	body := strings.TrimSpace(req.Body)
	if title == "" {
		title = firstSentence(body)
	}
	if title == "" {
		title = "Untitled Director ticket"
	}
	if body == "" {
		body = title
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "director.portal"
	}
	ticket := DirectorTicket{
		ID:             uuid.NewString(),
		OrganizationID: d.cfg.OrganizationID,
		Title:          title,
		Body:           body,
		Source:         source,
		Priority:       string(normalizeDirectorPriority(req.Priority)),
		Risk:           string(normalizeDirectorRisk(req.Risk)),
		Status:         DirectorTicketOpen,
		AssignedRole:   strings.TrimSpace(req.AssignedRole),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := d.writeTicket(ticket); err != nil {
		return DirectorTicket{}, err
	}
	if err := d.appendEvent(DirectorEvent{
		ID:             uuid.NewString(),
		OrganizationID: d.cfg.OrganizationID,
		TicketID:       ticket.ID,
		Kind:           "ticket.opened",
		Message:        d.agent.summarizeTicket(ticket),
		CreatedAt:      now,
	}); err != nil {
		return DirectorTicket{}, err
	}
	if req.AutoDispatch {
		return d.AutoDispatchTicket(ctx, ticket.ID, "submit.auto_dispatch")
	}
	return ticket, nil
}

func (d *DirectorService) DispatchTicket(ctx context.Context, ticketID string) (DirectorTicket, error) {
	return d.dispatchTicket(ctx, ticketID, "manual")
}

func (d *DirectorService) AutoDispatchTicket(ctx context.Context, ticketID string, source string) (DirectorTicket, error) {
	tickets, err := d.ListTickets()
	if err != nil {
		return DirectorTicket{}, err
	}
	for i := range tickets {
		if tickets[i].ID != ticketID {
			continue
		}
		ticket := tickets[i]
		allowed, reason := d.Policy().AllowsAutoDispatch(ticket)
		if !allowed {
			ticket.LastSummary = reason
			ticket.UpdatedAt = time.Now().UnixMilli()
			tickets[i] = ticket
			if err := d.writeTickets(tickets); err != nil {
				return DirectorTicket{}, err
			}
			_ = d.appendEvent(DirectorEvent{
				ID:             uuid.NewString(),
				OrganizationID: ticket.OrganizationID,
				TicketID:       ticket.ID,
				Kind:           "auto_dispatch.blocked",
				Message:        reason,
				Metadata:       map[string]string{"source": source, "risk": ticket.Risk, "priority": ticket.Priority},
				CreatedAt:      time.Now().UnixMilli(),
			})
			return ticket, nil
		}
		return d.dispatchTicket(ctx, ticketID, source)
	}
	return DirectorTicket{}, fmt.Errorf("director ticket %q not found", ticketID)
}

func (d *DirectorService) dispatchTicket(ctx context.Context, ticketID string, source string) (DirectorTicket, error) {
	tickets, err := d.ListTickets()
	if err != nil {
		return DirectorTicket{}, err
	}
	for i := range tickets {
		if tickets[i].ID != ticketID {
			continue
		}
		ticket := tickets[i]
		if ticket.Status != DirectorTicketOpen {
			return ticket, nil
		}
		now := time.Now().UnixMilli()
		targetChannel := OrganizationChannel(ticket.OrganizationID)
		if strings.TrimSpace(ticket.AssignedRole) != "" {
			targetChannel = ActorChannel(ticket.AssignedRole)
		}
		signal := WakeSignal{
			ID:             "director-" + ticket.ID,
			OrganizationID: ticket.OrganizationID,
			ActorID:        ticket.AssignedRole,
			Channel:        targetChannel,
			Kind:           SignalDirector,
			Payload: map[string]string{
				"entrySource":      "director.dispatch",
				"directorAgent":    d.agent.identity.ID,
				"directorTicketId": ticket.ID,
				"dispatchSource":   source,
				"title":            ticket.Title,
				"priority":         ticket.Priority,
				"risk":             ticket.Risk,
				"source":           ticket.Source,
				"prompt_injection": d.agent.dispatchPrompt(ticket),
			},
			CreatedAt: now,
		}
		if _, err := d.cfg.Ledger.AppendSignal(ctx, signal); err != nil {
			return DirectorTicket{}, err
		}
		if err := d.cfg.Bus.Publish(ctx, signal); err != nil {
			return DirectorTicket{}, err
		}
		ticket.Status = DirectorTicketDispatched
		ticket.DispatchedAt = now
		ticket.UpdatedAt = now
		ticket.LastSummary = "Sent to Agency for execution or clarification."
		tickets[i] = ticket
		if err := d.writeTickets(tickets); err != nil {
			return DirectorTicket{}, err
		}
		if err := d.appendEvent(DirectorEvent{
			ID:             uuid.NewString(),
			OrganizationID: ticket.OrganizationID,
			TicketID:       ticket.ID,
			Kind:           "ticket.dispatched",
			Message:        "Director dispatched: " + ticket.Title,
			Metadata:       map[string]string{"channel": targetChannel, "signalId": signal.ID, "source": source},
			CreatedAt:      now,
		}); err != nil {
			return DirectorTicket{}, err
		}
		return ticket, nil
	}
	return DirectorTicket{}, fmt.Errorf("director ticket %q not found", ticketID)
}

func (d *DirectorService) Monitor(ctx context.Context) (DirectorStatus, error) {
	status, err := d.Status(ctx)
	if err != nil {
		return DirectorStatus{}, err
	}
	autoDispatched := 0
	if !d.Policy().PauseWhenApprovalsPending || status.PendingApprovals == 0 {
		autoDispatched, err = d.AutoDispatchOpenTickets(ctx, "monitor")
		if err != nil {
			return DirectorStatus{}, err
		}
		if autoDispatched > 0 {
			status, err = d.Status(ctx)
			if err != nil {
				return DirectorStatus{}, err
			}
		}
	}
	message := d.agent.monitorSummary(status)
	_ = d.appendEvent(DirectorEvent{
		ID:             uuid.NewString(),
		OrganizationID: d.cfg.OrganizationID,
		Kind:           "monitor.checked",
		Message:        message,
		Metadata: map[string]string{
			"openTickets":      fmt.Sprintf("%d", status.OpenTickets),
			"autoDispatched":   fmt.Sprintf("%d", autoDispatched),
			"pendingApprovals": fmt.Sprintf("%d", status.PendingApprovals),
			"ledgerSequence":   fmt.Sprintf("%d", status.LedgerSequence),
		},
		CreatedAt: time.Now().UnixMilli(),
	})
	return status, nil
}

func (d *DirectorService) AutoDispatchOpenTickets(ctx context.Context, source string) (int, error) {
	tickets, err := d.ListTickets()
	if err != nil {
		return 0, err
	}
	dispatched := 0
	for _, ticket := range tickets {
		if ticket.Status != DirectorTicketOpen {
			continue
		}
		allowed, _ := d.Policy().AllowsAutoDispatch(ticket)
		if !allowed {
			continue
		}
		if _, err := d.dispatchTicket(ctx, ticket.ID, source+".auto"); err != nil {
			return dispatched, err
		}
		dispatched++
	}
	return dispatched, nil
}

func (d *DirectorService) Policy() DirectorPolicy {
	if d == nil {
		return DefaultDirectorPolicy()
	}
	if len(d.cfg.Policy.AutoDispatchRisks) == 0 && len(d.cfg.Policy.RequireApprovalRisks) == 0 {
		return DefaultDirectorPolicy()
	}
	return d.cfg.Policy
}

func (d *DirectorService) Status(ctx context.Context) (DirectorStatus, error) {
	tickets, err := d.ListTickets()
	if err != nil {
		return DirectorStatus{}, err
	}
	events, err := d.ListEvents()
	if err != nil {
		return DirectorStatus{}, err
	}
	snapshot, err := d.cfg.Ledger.LatestSnapshot(ctx, d.cfg.OrganizationID)
	if err != nil {
		return DirectorStatus{}, err
	}
	pending, err := d.cfg.Ledger.Pending(ctx, d.cfg.OrganizationID)
	if err != nil {
		return DirectorStatus{}, err
	}
	status := DirectorStatus{
		Agent:            d.agent.Identity(),
		BaseDir:          d.cfg.BaseDir,
		OrganizationID:   d.cfg.OrganizationID,
		SharedWorkplace:  d.cfg.SharedWorkplace,
		LedgerSequence:   snapshot.LedgerSequence,
		PendingApprovals: len(pending),
		LastSignal:       snapshot.LastSignal,
		UpdatedAt:        time.Now().UnixMilli(),
	}
	for _, ticket := range tickets {
		switch ticket.Status {
		case DirectorTicketDispatched:
			status.Dispatched++
		case DirectorTicketClosed:
			status.ClosedTickets++
		default:
			status.OpenTickets++
		}
	}
	if len(events) > 0 {
		status.LastEvent = &events[len(events)-1]
	}
	return status, nil
}

func (d *DirectorService) ListTickets() ([]DirectorTicket, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return readJSONL[DirectorTicket](d.ticketsPath)
}

func (d *DirectorService) ListEvents() ([]DirectorEvent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return readJSONL[DirectorEvent](d.eventsPath)
}

func (d *DirectorService) writeTicket(ticket DirectorTicket) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tickets, err := readJSONL[DirectorTicket](d.ticketsPath)
	if err != nil {
		return err
	}
	tickets = append(tickets, ticket)
	return writeJSONL(d.ticketsPath, tickets)
}

func (d *DirectorService) writeTickets(tickets []DirectorTicket) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return writeJSONL(d.ticketsPath, tickets)
}

func (d *DirectorService) appendEvent(event DirectorEvent) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	file, err := os.OpenFile(d.eventsPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	return err
}

type DirectorAgent struct {
	identity AgentIdentity
}

func NewDirectorAgent(organizationID string) *DirectorAgent {
	return &DirectorAgent{
		identity: AgentIdentity{
			ID:             "director",
			Name:           "Director",
			Role:           "personal-director",
			OrganizationID: organizationID,
			Metadata: map[string]string{
				"posture": "minimal-pi-like",
				"scope":   "daily-interface-over-agency",
			},
		},
	}
}

func (a *DirectorAgent) Identity() AgentIdentity {
	return a.identity
}

func (a *DirectorAgent) Capabilities() CapabilityPack {
	return CapabilityPack{
		Skills: []string{"intake", "triage", "summarize", "dispatch", "monitor"},
		Tools:  []string{"ledger", "bus", "director_portal"},
		ActionConstraints: []ActionType{
			ActionBroadcast,
			ActionUpdateTask,
			ActionRequestReview,
		},
		Metadata: map[string]string{
			"style": "short, calm, plainspoken, decision-oriented",
		},
	}
}

func (a *DirectorAgent) Handle(_ context.Context, obs ObservationSnapshot, signal WakeSignal) ([]ActionProposal, error) {
	message := "I am watching the office and will only pull you in when a decision matters."
	if signal.Kind == SignalDirector {
		message = "I received a Director ticket and routed it into Agency with approval boundaries intact."
	}
	return []ActionProposal{
		{
			OrganizationID: obs.OrganizationID,
			ActorID:        a.identity.ID,
			Type:           ActionBroadcast,
			ProposedAt:     time.Now().UnixMilli(),
			Payload: map[string]any{
				"message":     message,
				"signalKind":  string(signal.Kind),
				"signalID":    signal.ID,
				"entrySource": "director.agent",
			},
		},
	}, nil
}

func (a *DirectorAgent) summarizeTicket(ticket DirectorTicket) string {
	return fmt.Sprintf("Got it. I opened %q and will keep it moving without making risky changes silently.", ticket.Title)
}

func (a *DirectorAgent) dispatchPrompt(ticket DirectorTicket) string {
	return fmt.Sprintf(`Director request: %s

User intent:
%s

Operate as the Agency office, not as a standalone chatbot. Start with a concise status update, identify the next concrete action, and ask for approval before consequential file, shell, network, billing, credential, or publishing operations.

Ask GIST to preserve the causal unknowns: what contradiction, missing evidence, or counterfactual branch would make this unsafe?`, ticket.Title, ticket.Body)
}

func (a *DirectorAgent) monitorSummary(status DirectorStatus) string {
	if status.PendingApprovals > 0 {
		return fmt.Sprintf("You have %d approval item(s) waiting. I will keep the office paused at the decision boundary.", status.PendingApprovals)
	}
	if status.OpenTickets > 0 {
		return fmt.Sprintf("I am tracking %d open ticket(s) and %d dispatched ticket(s).", status.OpenTickets, status.Dispatched)
	}
	return "Office check complete. No urgent decisions need you right now."
}

func (p DirectorPolicy) AllowsAutoDispatch(ticket DirectorTicket) (bool, string) {
	risk := normalizeDirectorRisk(ticket.Risk)
	priority := normalizeDirectorPriority(ticket.Priority)
	if containsRisk(p.RequireApprovalRisks, risk) {
		return false, fmt.Sprintf("Director held %q for review because risk is %s.", ticket.Title, risk)
	}
	if containsPriority(p.RequireApprovalPriorities, priority) {
		return false, fmt.Sprintf("Director held %q for review because priority is %s.", ticket.Title, priority)
	}
	if !containsRisk(p.AutoDispatchRisks, risk) {
		return false, fmt.Sprintf("Director held %q for review because risk is %s.", ticket.Title, risk)
	}
	if !containsPriority(p.AutoDispatchPriorities, priority) {
		return false, fmt.Sprintf("Director held %q for review because priority is %s.", ticket.Title, priority)
	}
	return true, "Director policy allows automatic dispatch."
}

func normalizeDirectorRisk(value string) DirectorRisk {
	switch DirectorRisk(strings.ToLower(strings.TrimSpace(value))) {
	case DirectorRiskLow:
		return DirectorRiskLow
	case DirectorRiskMedium:
		return DirectorRiskMedium
	case DirectorRiskHigh:
		return DirectorRiskHigh
	default:
		return DirectorRiskUnknown
	}
}

func normalizeDirectorPriority(value string) DirectorPriority {
	switch DirectorPriority(strings.ToLower(strings.TrimSpace(value))) {
	case DirectorPriorityLow:
		return DirectorPriorityLow
	case DirectorPriorityHigh:
		return DirectorPriorityHigh
	case DirectorPriorityUrgent:
		return DirectorPriorityUrgent
	default:
		return DirectorPriorityNormal
	}
}

func containsRisk(values []DirectorRisk, target DirectorRisk) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsPriority(values []DirectorPriority, target DirectorPriority) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func readJSONL[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := []T{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item T
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func writeJSONL[T any](path string, items []T) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, item := range items {
		data, err := json.Marshal(item)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func firstSentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, sep := range []string{".", "!", "?", "\n"} {
		if idx := strings.Index(text, sep); idx > 0 && idx < 80 {
			return strings.TrimSpace(text[:idx])
		}
	}
	if len(text) > 80 {
		return strings.TrimSpace(text[:80])
	}
	return text
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
