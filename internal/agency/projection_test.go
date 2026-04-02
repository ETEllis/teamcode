package agency

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProjectionDaemonWritesLatestSnapshot(t *testing.T) {
	t.Parallel()

	ledger, err := NewLedgerService(t.TempDir())
	require.NoError(t, err)

	_, err = ledger.Append(context.Background(), LedgerEntry{
		OrganizationID: "org-1",
		Kind:           LedgerEntrySignal,
		Signal: &WakeSignal{
			ID:             "sig-1",
			OrganizationID: "org-1",
			Channel:        OrganizationChannel("org-1"),
			Kind:           SignalBroadcast,
			CreatedAt:      time.Now().UnixMilli(),
		},
	})
	require.NoError(t, err)

	outputPath := filepath.Join(t.TempDir(), "snapshot.json")
	daemon := NewProjectionDaemon(ledger, "org-1", outputPath, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = daemon.Run(ctx)
	}()

	require.Eventually(t, func() bool {
		data, err := os.ReadFile(outputPath)
		return err == nil && len(data) > 0
	}, time.Second, 20*time.Millisecond)
}
