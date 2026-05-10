package agency

import (
	"context"
	"testing"
)

func TestDirectorSubmitAndDispatch(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	ledger, err := NewLedgerService(base + "/ledger")
	if err != nil {
		t.Fatal(err)
	}
	bus := NewMemoryEventBus()
	defer bus.Close(ctx)

	director, err := NewDirectorService(DirectorConfig{
		BaseDir:         base,
		OrganizationID:  "org-1",
		SharedWorkplace: base + "/work",
		Ledger:          ledger,
		Bus:             bus,
	})
	if err != nil {
		t.Fatal(err)
	}

	ch, err := bus.Subscribe(ctx, OrganizationChannel("org-1"))
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := director.SubmitTicket(ctx, DirectorTicketRequest{
		Title: "Ship the portal",
		Body:  "Make the Director portal available locally.",
		Risk:  "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Status != DirectorTicketOpen {
		t.Fatalf("expected open ticket, got %s", ticket.Status)
	}

	dispatched, err := director.DispatchTicket(ctx, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dispatched.Status != DirectorTicketDispatched {
		t.Fatalf("expected dispatched ticket, got %s", dispatched.Status)
	}

	select {
	case signal := <-ch:
		if signal.Kind != SignalDirector {
			t.Fatalf("expected director signal, got %s", signal.Kind)
		}
		if signal.Payload["directorTicketId"] != ticket.ID {
			t.Fatalf("expected ticket id in payload")
		}
	default:
		t.Fatalf("expected director dispatch signal")
	}

	status, err := director.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Dispatched != 1 {
		t.Fatalf("expected one dispatched ticket, got %d", status.Dispatched)
	}
}

func TestDirectorMonitorWritesEvent(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	ledger, err := NewLedgerService(base + "/ledger")
	if err != nil {
		t.Fatal(err)
	}
	director, err := NewDirectorService(DirectorConfig{
		BaseDir:        base,
		OrganizationID: "org-1",
		Ledger:         ledger,
		Bus:            NewMemoryEventBus(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := director.Monitor(ctx); err != nil {
		t.Fatal(err)
	}
	events, err := director.ListEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Kind != "monitor.checked" {
		t.Fatalf("expected monitor event, got %#v", events)
	}
}
