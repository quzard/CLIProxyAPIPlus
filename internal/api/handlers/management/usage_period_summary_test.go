package management

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestPostUsagePeriodSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stats := usage.NewRequestStatistics()
	base := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "codex",
		Model:       "gpt-5.3-codex-spark",
		AuthIndex:   "codex-user.json",
		RequestedAt: base.Add(5 * time.Minute),
		Detail: coreusage.Detail{
			InputTokens:     1000,
			OutputTokens:    2000,
			CacheReadTokens: 100,
			CachedTokens:    100,
			TotalTokens:     3000,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "codex",
		Model:       "gpt-5-codex",
		AuthIndex:   "codex-user.json",
		RequestedAt: base.Add(10 * time.Minute),
		Detail: coreusage.Detail{
			InputTokens:  1000,
			OutputTokens: 1000,
			TotalTokens:  2000,
		},
	})

	body := map[string]any{
		"windows": []map[string]any{
			{
				"id":           "spark",
				"auth_index":   "codex-user.json",
				"start_at_ms":  base.UnixMilli(),
				"end_at_ms":    base.Add(time.Hour).UnixMilli(),
				"model_filter": "spark",
			},
		},
		"model_prices": map[string]config.UsageModelPrice{
			"gpt-5.1-codex": {
				Prompt:     1,
				Completion: 2,
				CacheRead:  0.5,
			},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/usage/period-summary", bytes.NewReader(data))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler := &Handler{usageStats: stats}
	handler.PostUsagePeriodSummary(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response struct {
		Items []usage.PeriodSummary `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(response.Items))
	}

	item := response.Items[0]
	if item.ID != "spark" || item.Requests != 1 || item.Tokens != 3000 {
		t.Fatalf("summary = %+v, want id=spark requests=1 tokens=3000", item)
	}
	wantCost := (900.0/1_000_000)*1 + (100.0/1_000_000)*0.5 + (2000.0/1_000_000)*2
	if math.Abs(item.Cost-wantCost) > 0.000001 {
		t.Fatalf("cost = %v, want %v", item.Cost, wantCost)
	}
}
