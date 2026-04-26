package usage

import (
	"context"
	"math"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatisticsSummarizePeriodsFiltersByAuthWindowAndModel(t *testing.T) {
	stats := NewRequestStatistics()
	base := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "codex",
		Model:       "gpt-5.3-codex-spark",
		AuthIndex:   "codex-user-a.json",
		RequestedAt: base.Add(10 * time.Minute),
		Detail: coreusage.Detail{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "codex",
		Model:       "gpt-5-codex",
		AuthIndex:   "codex-user-a.json",
		RequestedAt: base.Add(20 * time.Minute),
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "codex",
		Model:       "gpt-5.3-codex-spark",
		AuthIndex:   "codex-user-b.json",
		RequestedAt: base.Add(10 * time.Minute),
		Detail: coreusage.Detail{
			InputTokens:  1000,
			OutputTokens: 1000,
			TotalTokens:  2000,
		},
	})

	summaries := stats.SummarizePeriods([]PeriodSummaryWindow{
		{
			ID:          "all",
			AuthIndex:   "codex-user-a.json",
			StartAt:     base,
			EndAt:       base.Add(time.Hour),
			ModelFilter: "",
		},
		{
			ID:          "spark",
			AuthIndex:   "codex-user-a.json",
			StartAt:     base,
			EndAt:       base.Add(time.Hour),
			ModelFilter: "spark",
		},
	}, func(_ string, detail RequestDetail) float64 {
		return float64(detail.Tokens.TotalTokens) / 1000
	})

	if len(summaries) != 2 {
		t.Fatalf("summaries len = %d, want 2", len(summaries))
	}
	if summaries[0].Requests != 2 || summaries[0].Tokens != 180 {
		t.Fatalf("all summary = %+v, want requests=2 tokens=180", summaries[0])
	}
	if math.Abs(summaries[0].Cost-0.18) > 0.000001 {
		t.Fatalf("all cost = %v, want 0.18", summaries[0].Cost)
	}
	if summaries[1].Requests != 1 || summaries[1].Tokens != 150 {
		t.Fatalf("spark summary = %+v, want requests=1 tokens=150", summaries[1])
	}
}
