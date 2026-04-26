package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatisticsRecordIncludesLatency(t *testing.T) {
	stats := NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "test-key",
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Latency:     1500 * time.Millisecond,
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	if details[0].LatencyMs != 1500 {
		t.Fatalf("latency_ms = %d, want 1500", details[0].LatencyMs)
	}
}

func TestRequestStatisticsMergeSnapshotDedupIgnoresLatency(t *testing.T) {
	stats := NewRequestStatistics()
	timestamp := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	first := StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"test-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp: timestamp,
							LatencyMs: 0,
							Source:    "user@example.com",
							AuthIndex: "0",
							Tokens: TokenStats{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
						}},
					},
				},
			},
		},
	}
	second := StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"test-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp: timestamp,
							LatencyMs: 2500,
							Source:    "user@example.com",
							AuthIndex: "0",
							Tokens: TokenStats{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
						}},
					},
				},
			},
		},
	}

	result := stats.MergeSnapshot(first)
	if result.Added != 1 || result.Skipped != 0 {
		t.Fatalf("first merge = %+v, want added=1 skipped=0", result)
	}

	result = stats.MergeSnapshot(second)
	if result.Added != 0 || result.Skipped != 1 {
		t.Fatalf("second merge = %+v, want added=0 skipped=1", result)
	}

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
}

func TestRequestStatisticsRecordIncludesRequestMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodGet, "/v1/responses?trace=1", nil)
	logging.SetGinRequestID(ginCtx, "deadbeef")

	ctx := context.WithValue(context.Background(), "gin", ginCtx)

	stats := NewRequestStatistics()
	stats.Record(ctx, coreusage.Record{
		APIKey:      "test-key",
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:  1,
			OutputTokens: 2,
			TotalTokens:  3,
		},
	})

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	if details[0].RequestID != "deadbeef" {
		t.Fatalf("request_id = %q, want %q", details[0].RequestID, "deadbeef")
	}
	if details[0].Method != http.MethodGet {
		t.Fatalf("method = %q, want %q", details[0].Method, http.MethodGet)
	}
	if details[0].Path != "/v1/responses" {
		t.Fatalf("path = %q, want %q", details[0].Path, "/v1/responses")
	}
	if details[0].Endpoint != "GET /v1/responses" {
		t.Fatalf("endpoint = %q, want %q", details[0].Endpoint, "GET /v1/responses")
	}
}

func TestRequestStatisticsRecordIncludesThinkingEffort(t *testing.T) {
	stats := NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:         "test-key",
		Model:          "gpt-5.4",
		ThinkingEffort: "high",
		RequestedAt:    time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	if details[0].ThinkingEffort != "high" {
		t.Fatalf("thinking_effort = %q, want %q", details[0].ThinkingEffort, "high")
	}
}

func TestRequestStatisticsRecordIncludesServiceTier(t *testing.T) {
	stats := NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "test-key",
		Model:       "gpt-5.5",
		ServiceTier: "Priority",
		RequestedAt: time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.5"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	if details[0].ServiceTier != "priority" {
		t.Fatalf("service_tier = %q, want %q", details[0].ServiceTier, "priority")
	}
}

func TestRequestStatisticsRecordPreservesSplitCacheTokens(t *testing.T) {
	stats := NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "test-key",
		Model:       "claude-sonnet-4",
		RequestedAt: time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Detail: coreusage.Detail{
			InputTokens:         13,
			OutputTokens:        4,
			CachedTokens:        22000,
			CacheReadTokens:     22000,
			CacheCreationTokens: 31,
			TotalTokens:         17,
		},
	})

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["claude-sonnet-4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	if details[0].Tokens.CacheReadTokens != 22000 {
		t.Fatalf("cache_read_tokens = %d, want %d", details[0].Tokens.CacheReadTokens, 22000)
	}
	if details[0].Tokens.CacheCreationTokens != 31 {
		t.Fatalf("cache_creation_tokens = %d, want %d", details[0].Tokens.CacheCreationTokens, 31)
	}
}

func TestRequestStatisticsMergeSnapshotDedupSeparatesThinkingEffort(t *testing.T) {
	stats := NewRequestStatistics()
	timestamp := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)

	first := StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"test-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp:      timestamp,
							ThinkingEffort: "low",
							Source:         "user@example.com",
							AuthIndex:      "0",
							Tokens: TokenStats{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
						}},
					},
				},
			},
		},
	}
	second := StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"test-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp:      timestamp,
							ThinkingEffort: "high",
							Source:         "user@example.com",
							AuthIndex:      "0",
							Tokens: TokenStats{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
						}},
					},
				},
			},
		},
	}

	result := stats.MergeSnapshot(first)
	if result.Added != 1 || result.Skipped != 0 {
		t.Fatalf("first merge = %+v, want added=1 skipped=0", result)
	}

	result = stats.MergeSnapshot(second)
	if result.Added != 1 || result.Skipped != 0 {
		t.Fatalf("second merge = %+v, want added=1 skipped=0", result)
	}

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 2 {
		t.Fatalf("details len = %d, want 2", len(details))
	}
}

func TestRequestStatisticsMergeSnapshotDedupSeparatesServiceTier(t *testing.T) {
	stats := NewRequestStatistics()
	timestamp := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)

	baseSnapshot := func(serviceTier string) StatisticsSnapshot {
		return StatisticsSnapshot{
			APIs: map[string]APISnapshot{
				"test-key": {
					Models: map[string]ModelSnapshot{
						"gpt-5.5": {
							Details: []RequestDetail{{
								Timestamp:   timestamp,
								ServiceTier: serviceTier,
								Source:      "user@example.com",
								AuthIndex:   "0",
								Tokens: TokenStats{
									InputTokens:  10,
									OutputTokens: 20,
									TotalTokens:  30,
								},
							}},
						},
					},
				},
			},
		}
	}

	result := stats.MergeSnapshot(baseSnapshot(""))
	if result.Added != 1 || result.Skipped != 0 {
		t.Fatalf("first merge = %+v, want added=1 skipped=0", result)
	}

	result = stats.MergeSnapshot(baseSnapshot("priority"))
	if result.Added != 1 || result.Skipped != 0 {
		t.Fatalf("second merge = %+v, want added=1 skipped=0", result)
	}

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.5"].Details
	if len(details) != 2 {
		t.Fatalf("details len = %d, want 2", len(details))
	}
}
