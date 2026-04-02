package agency

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PerformanceRecord captures a single directive→output→score cycle for an actor.
// Published to the bulletin channel so the TUI can display the timeline.
type PerformanceRecord struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	ActorID        string  `json:"actorId"`
	Directive      string  `json:"directive"`      // prompt_injection or signal summary
	Output         string  `json:"output"`         // LLM inference result text
	Score          float64 `json:"score"`          // GIST confidence as proxy score
	SignalID       string  `json:"signalId"`
	Provider       string  `json:"provider"`
	ModelID        string  `json:"modelId"`
	CreatedAt      int64   `json:"createdAt"`
}

// BulletinChannel returns the pub/sub channel name for an organization's bulletin.
func BulletinChannel(organizationID string) string {
	return "agency.bulletin." + organizationID
}

// PublishPerformance serialises a PerformanceRecord into a WakeSignal and
// publishes it to the organization's bulletin channel.
func PublishPerformance(ctx context.Context, bus EventBus, rec PerformanceRecord) error {
	if rec.ID == "" {
		rec.ID = uuid.NewString()
	}
	if rec.CreatedAt == 0 {
		rec.CreatedAt = time.Now().UnixMilli()
	}

	signal := WakeSignal{
		ID:             rec.ID,
		OrganizationID: rec.OrganizationID,
		ActorID:        rec.ActorID,
		Channel:        BulletinChannel(rec.OrganizationID),
		Kind:           SignalProjection,
		Payload: map[string]string{
			"directive": rec.Directive,
			"output":    rec.Output,
			"score":     floatStr(rec.Score),
			"signalId":  rec.SignalID,
			"provider":  rec.Provider,
			"modelId":   rec.ModelID,
			"actorId":   rec.ActorID,
		},
		CreatedAt: rec.CreatedAt,
	}
	return bus.Publish(ctx, signal)
}

func floatStr(f float64) string {
	return fmt.Sprintf("%.2f", f)
}
