package management

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

type usageExportPayload struct {
	Version    int                      `json:"version"`
	ExportedAt time.Time                `json:"exported_at"`
	Usage      usage.StatisticsSnapshot `json:"usage"`
}

type usageImportPayload struct {
	Version int                      `json:"version"`
	Usage   usage.StatisticsSnapshot `json:"usage"`
}

type usageModelPricesBody struct {
	Value                 map[string]config.UsageModelPrice `json:"value"`
	DisabledDefaultModels []string                          `json:"disabledDefaultModels"`
	DisabledDefaults      []string                          `json:"disabled-default-models"`
}

func sanitizeUsageModelPrices(input map[string]config.UsageModelPrice) map[string]config.UsageModelPrice {
	if len(input) == 0 {
		return map[string]config.UsageModelPrice{}
	}

	out := make(map[string]config.UsageModelPrice, len(input))
	for model, price := range input {
		key := strings.TrimSpace(model)
		if key == "" {
			continue
		}

		sanitized := config.UsageModelPrice{
			Prompt:        clampNonNegativeFloat(price.Prompt),
			Completion:    clampNonNegativeFloat(price.Completion),
			Cache:         clampNonNegativeFloat(price.Cache),
			CacheRead:     clampNonNegativeFloat(price.CacheRead),
			CacheCreation: clampNonNegativeFloat(price.CacheCreation),
		}

		if sanitized.CacheRead == 0 && sanitized.Cache > 0 {
			sanitized.CacheRead = sanitized.Cache
		}
		if sanitized.CacheCreation == 0 && sanitized.CacheRead > 0 {
			sanitized.CacheCreation = sanitized.CacheRead
		}
		if sanitized.Cache == 0 && sanitized.CacheRead > 0 {
			sanitized.Cache = sanitized.CacheRead
		}

		out[key] = sanitized
	}
	return out
}

func sanitizeDisabledDefaultModels(input []string) []string {
	if len(input) == 0 {
		return nil
	}

	out := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, item := range input {
		model := strings.TrimSpace(item)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func clampNonNegativeFloat(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}

// GetUsageStatistics returns the in-memory request statistics snapshot.
func (h *Handler) GetUsageStatistics(c *gin.Context) {
	var snapshot usage.StatisticsSnapshot
	if h != nil && h.usageStats != nil {
		snapshot = h.usageStats.Snapshot()
	}
	c.JSON(http.StatusOK, gin.H{
		"usage":           snapshot,
		"failed_requests": snapshot.FailureCount,
	})
}

// GetUsageModelPrices returns shared server-side model pricing for usage cost estimation.
func (h *Handler) GetUsageModelPrices(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusOK, gin.H{
			"usage-model-prices":      map[string]config.UsageModelPrice{},
			"disabled-default-models": []string{},
		})
		return
	}

	prices := make(map[string]config.UsageModelPrice, len(h.cfg.UsageModelPrices))
	for model, price := range h.cfg.UsageModelPrices {
		prices[model] = price
	}
	disabledDefaults := append([]string(nil), h.cfg.UsageDisabledDefaultModels...)
	if disabledDefaults == nil {
		disabledDefaults = []string{}
	}
	c.JSON(http.StatusOK, gin.H{
		"usage-model-prices":      prices,
		"disabled-default-models": disabledDefaults,
	})
}

// PutUsageModelPrices updates shared server-side model pricing for usage cost estimation.
func (h *Handler) PutUsageModelPrices(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "config unavailable"})
		return
	}

	var body usageModelPricesBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	disabledDefaults := body.DisabledDefaultModels
	if len(disabledDefaults) == 0 && len(body.DisabledDefaults) > 0 {
		disabledDefaults = body.DisabledDefaults
	}

	h.cfg.UsageModelPrices = sanitizeUsageModelPrices(body.Value)
	h.cfg.UsageDisabledDefaultModels = sanitizeDisabledDefaultModels(disabledDefaults)
	h.persist(c)
}

// ExportUsageStatistics returns a complete usage snapshot for backup/migration.
func (h *Handler) ExportUsageStatistics(c *gin.Context) {
	var snapshot usage.StatisticsSnapshot
	if h != nil && h.usageStats != nil {
		snapshot = h.usageStats.Snapshot()
	}
	c.JSON(http.StatusOK, usageExportPayload{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Usage:      snapshot,
	})
}

// ImportUsageStatistics merges a previously exported usage snapshot into memory.
func (h *Handler) ImportUsageStatistics(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	data, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	var payload usageImportPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if payload.Version != 0 && payload.Version != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported version"})
		return
	}

	result := h.usageStats.MergeSnapshot(payload.Usage)
	snapshot := h.usageStats.Snapshot()
	c.JSON(http.StatusOK, gin.H{
		"added":           result.Added,
		"skipped":         result.Skipped,
		"total_requests":  snapshot.TotalRequests,
		"failed_requests": snapshot.FailureCount,
	})
}
