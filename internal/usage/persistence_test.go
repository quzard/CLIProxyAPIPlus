package usage

import (
	"context"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestResolveStatisticsPersistencePath(t *testing.T) {
	path := ResolveStatisticsPersistencePath("/tmp/cliproxy/config.yaml", "", true)
	if path != "/tmp/cliproxy/.usage_statistics.json" {
		t.Fatalf("path = %q, want default next to config", path)
	}

	path = ResolveStatisticsPersistencePath("/tmp/cliproxy/config.yaml", "stats/usage.json", true)
	if path != "/tmp/cliproxy/stats/usage.json" {
		t.Fatalf("path = %q, want relative to config dir", path)
	}

	path = ResolveStatisticsPersistencePath("/tmp/cliproxy/config.yaml", "", false)
	if path != "" {
		t.Fatalf("path = %q, want empty when disabled and not configured", path)
	}
}

func TestStatisticsPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/usage.json"
	base := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	stats := NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "codex",
		Model:       "gpt-5-codex",
		AuthIndex:   "codex-user.json",
		RequestedAt: base,
		Detail: coreusage.Detail{
			InputTokens:  1,
			OutputTokens: 2,
			TotalTokens:  3,
		},
	})

	if err := saveStatisticsSnapshot(path, stats); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	loaded := NewRequestStatistics()
	if err := loadStatisticsSnapshot(path, loaded); err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	snapshot := loaded.Snapshot()
	if snapshot.TotalRequests != 1 || snapshot.TotalTokens != 3 {
		t.Fatalf("loaded snapshot = %+v, want requests=1 tokens=3", snapshot)
	}
}
