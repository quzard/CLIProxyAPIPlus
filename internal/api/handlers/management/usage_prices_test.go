package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestGetUsageModelPrices_Empty(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage-model-prices", nil)

	handler := &Handler{cfg: &config.Config{}}
	handler.GetUsageModelPrices(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		UsageModelPrices      map[string]config.UsageModelPrice `json:"usage-model-prices"`
		DisabledDefaultModels []string                          `json:"disabled-default-models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(body.UsageModelPrices) != 0 {
		t.Fatalf("expected empty usage-model-prices, got %+v", body.UsageModelPrices)
	}
	if len(body.DisabledDefaultModels) != 0 {
		t.Fatalf("expected empty disabled-default-models, got %+v", body.DisabledDefaultModels)
	}
}

func TestPutUsageModelPrices_PersistsSanitizedPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("usage-model-prices: {}\n"), 0o600); err != nil {
		t.Fatalf("failed to seed config file: %v", err)
	}

	body := `{
	  "value": {
	    " claude-sonnet-4 ": {
	      "prompt": 3,
	      "completion": 15,
	      "cacheRead": 0.3,
	      "cacheCreation": 3.75
	    },
	    "gpt-5.4": {
	      "prompt": -1,
	      "completion": 15,
	      "cache": 0.25,
	      "priority": {
	        "prompt": 5,
	        "completion": 30,
	        "cache": 0.5
	      }
	    }
	  },
	  "disabledDefaultModels": ["claude-3-opus", " claude-3-opus ", "gpt-5.1"]
	}`

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/v0/management/usage-model-prices", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler := &Handler{
		cfg:            &config.Config{},
		configFilePath: configPath,
	}
	handler.PutUsageModelPrices(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	price, ok := handler.cfg.UsageModelPrices["claude-sonnet-4"]
	if !ok {
		t.Fatalf("expected normalized claude-sonnet-4 price in config")
	}
	if price.CacheRead != 0.3 || price.CacheCreation != 3.75 || price.Cache != 0.3 {
		t.Fatalf("unexpected claude-sonnet-4 price: %+v", price)
	}

	gpt, ok := handler.cfg.UsageModelPrices["gpt-5.4"]
	if !ok {
		t.Fatalf("expected gpt-5.4 price in config")
	}
	if gpt.Prompt != 0 || gpt.CacheRead != 0.25 || gpt.CacheCreation != 0.25 {
		t.Fatalf("unexpected gpt-5.4 price after sanitization: %+v", gpt)
	}
	if gpt.Priority == nil {
		t.Fatalf("expected gpt-5.4 priority price")
	}
	if gpt.Priority.Prompt != 5 || gpt.Priority.Completion != 30 || gpt.Priority.CacheRead != 0.5 || gpt.Priority.CacheCreation != 0.5 {
		t.Fatalf("unexpected gpt-5.4 priority price after sanitization: %+v", gpt.Priority)
	}
	if len(handler.cfg.UsageDisabledDefaultModels) != 2 {
		t.Fatalf("unexpected disabled defaults after sanitization: %+v", handler.cfg.UsageDisabledDefaultModels)
	}

	saved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}
	content := string(saved)
	if !strings.Contains(content, "usage-model-prices:") {
		t.Fatalf("expected persisted usage-model-prices in config, got %s", content)
	}
	if !strings.Contains(content, "claude-sonnet-4:") {
		t.Fatalf("expected normalized model key persisted, got %s", content)
	}
	if !strings.Contains(content, "priority:") {
		t.Fatalf("expected priority model price persisted, got %s", content)
	}
}
