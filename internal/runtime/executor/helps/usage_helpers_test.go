package helps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestParseOpenAIUsageChatCompletions(t *testing.T) {
	data := []byte(`{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":5}}}`)
	detail := ParseOpenAIUsage(data)
	if detail.InputTokens != 1 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 1)
	}
	if detail.OutputTokens != 2 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 2)
	}
	if detail.TotalTokens != 3 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 3)
	}
	if detail.CachedTokens != 4 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 4)
	}
	if detail.ReasoningTokens != 5 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 5)
	}
}

func TestParseOpenAIUsageResponses(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30,"input_tokens_details":{"cached_tokens":7},"output_tokens_details":{"reasoning_tokens":9}}}`)
	detail := ParseOpenAIUsage(data)
	if detail.InputTokens != 10 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 10)
	}
	if detail.OutputTokens != 20 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 20)
	}
	if detail.TotalTokens != 30 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 30)
	}
	if detail.CachedTokens != 7 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 7)
	}
	if detail.CacheReadTokens != 7 {
		t.Fatalf("cache read tokens = %d, want %d", detail.CacheReadTokens, 7)
	}
	if detail.ReasoningTokens != 9 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 9)
	}
}

func TestParseClaudeUsageKeepsSplitCacheTokens(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":13,"output_tokens":4,"cache_read_input_tokens":22000,"cache_creation_input_tokens":31}}`)
	detail := ParseClaudeUsage(data)

	if detail.InputTokens != 13 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 13)
	}
	if detail.OutputTokens != 4 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 4)
	}
	if detail.CachedTokens != 22000 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 22000)
	}
	if detail.CacheReadTokens != 22000 {
		t.Fatalf("cache read tokens = %d, want %d", detail.CacheReadTokens, 22000)
	}
	if detail.CacheCreationTokens != 31 {
		t.Fatalf("cache creation tokens = %d, want %d", detail.CacheCreationTokens, 31)
	}
}

func TestParseClaudeUsageFallsBackCachedTokensToCreationWhenReadMissing(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":13,"output_tokens":4,"cache_creation_input_tokens":31}}`)
	detail := ParseClaudeUsage(data)

	if detail.CachedTokens != 31 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 31)
	}
	if detail.CacheReadTokens != 0 {
		t.Fatalf("cache read tokens = %d, want %d", detail.CacheReadTokens, 0)
	}
	if detail.CacheCreationTokens != 31 {
		t.Fatalf("cache creation tokens = %d, want %d", detail.CacheCreationTokens, 31)
	}
}

func TestUsageReporterBuildRecordIncludesLatency(t *testing.T) {
	reporter := &UsageReporter{
		provider:    "openai",
		model:       "gpt-5.4",
		requestedAt: time.Now().Add(-1500 * time.Millisecond),
	}

	record := reporter.buildRecord(usage.Detail{TotalTokens: 3}, false)
	if record.Latency < time.Second {
		t.Fatalf("latency = %v, want >= 1s", record.Latency)
	}
	if record.Latency > 3*time.Second {
		t.Fatalf("latency = %v, want <= 3s", record.Latency)
	}
}

func TestUsageReporterBuildRecordIncludesThinkingEffortFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ginCtx.Set(usageThinkingEffortKey, "high")

	ctx := context.WithValue(context.Background(), "gin", ginCtx)

	reporter := &UsageReporter{
		provider: "codex",
		model:    "gpt-5.4",
	}
	reporter.captureThinkingEffort(ctx)

	record := reporter.buildRecord(usage.Detail{TotalTokens: 3}, false)
	if record.ThinkingEffort != "high" {
		t.Fatalf("thinking effort = %q, want %q", record.ThinkingEffort, "high")
	}
}

func TestUsageReporterBuildRecordIncludesServiceTierFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ginCtx.Set(usageServiceTierKey, "Priority")

	ctx := context.WithValue(context.Background(), "gin", ginCtx)

	reporter := &UsageReporter{
		provider: "openai",
		model:    "gpt-5.5",
	}
	reporter.captureServiceTier(ctx)

	record := reporter.buildRecord(usage.Detail{TotalTokens: 3}, false)
	if record.ServiceTier != "priority" {
		t.Fatalf("service tier = %q, want %q", record.ServiceTier, "priority")
	}
}
