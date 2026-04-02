package agency

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ProjectionDaemon struct {
	ledger         *LedgerService
	organizationID string
	outputPath     string
	interval       time.Duration
	mu             sync.Mutex
}

func NewProjectionDaemon(ledger *LedgerService, organizationID, outputPath string, interval time.Duration) *ProjectionDaemon {
	if interval <= 0 {
		interval = time.Second
	}
	return &ProjectionDaemon{
		ledger:         ledger,
		organizationID: organizationID,
		outputPath:     outputPath,
		interval:       interval,
	}
}

func (d *ProjectionDaemon) Run(ctx context.Context) error {
	if d == nil {
		return fmt.Errorf("projection daemon is nil")
	}
	if d.ledger == nil {
		return fmt.Errorf("projection daemon requires a ledger")
	}
	if d.outputPath == "" {
		return fmt.Errorf("projection daemon output path is required")
	}

	if err := d.writeSnapshot(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := d.writeSnapshot(ctx); err != nil {
				return err
			}
		}
	}
}

func (d *ProjectionDaemon) writeSnapshot(ctx context.Context) error {
	snapshot, err := d.ledger.LatestSnapshot(ctx, d.organizationID)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(d.outputPath), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(d.outputPath, data, 0o644)
}
