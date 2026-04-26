package management

import (
	"encoding/json"
	"math"
	"net/http"
	"regexp"
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

type usagePeriodSummaryWindowBody struct {
	ID             string `json:"id"`
	AuthIndex      string `json:"auth_index"`
	AuthIndexCamel string `json:"authIndex"`
	StartAtMs      int64  `json:"start_at_ms"`
	StartAtMsCamel int64  `json:"startAtMs"`
	EndAtMs        int64  `json:"end_at_ms"`
	EndAtMsCamel   int64  `json:"endAtMs"`
	ModelFilter    string `json:"model_filter"`
	ModelFilterAlt string `json:"modelFilter"`
}

type usagePeriodSummaryBody struct {
	Windows          []usagePeriodSummaryWindowBody    `json:"windows"`
	ModelPrices      map[string]config.UsageModelPrice `json:"model_prices"`
	ModelPricesCamel map[string]config.UsageModelPrice `json:"modelPrices"`
}

type usagePeriodSummaryResponse struct {
	Items       []usage.PeriodSummary `json:"items"`
	GeneratedAt time.Time             `json:"generated_at"`
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

		tier := sanitizeUsageModelPriceTier(config.UsageModelPriceTier{
			Prompt:        price.Prompt,
			Completion:    price.Completion,
			Cache:         price.Cache,
			CacheRead:     price.CacheRead,
			CacheCreation: price.CacheCreation,
		})
		sanitized := config.UsageModelPrice{
			Prompt:        tier.Prompt,
			Completion:    tier.Completion,
			Cache:         tier.Cache,
			CacheRead:     tier.CacheRead,
			CacheCreation: tier.CacheCreation,
		}
		if price.Priority != nil {
			priority := sanitizeUsageModelPriceTier(*price.Priority)
			sanitized.Priority = &priority
		}

		out[key] = sanitized
	}
	return out
}

func sanitizeUsageModelPriceTier(price config.UsageModelPriceTier) config.UsageModelPriceTier {
	sanitized := config.UsageModelPriceTier{
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

	return sanitized
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

// PostUsagePeriodSummary returns aggregated request usage for client-supplied windows.
func (h *Handler) PostUsagePeriodSummary(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	var body usagePeriodSummaryBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if len(body.Windows) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many windows"})
		return
	}

	windows := make([]usage.PeriodSummaryWindow, 0, len(body.Windows))
	for _, item := range body.Windows {
		authIndex := strings.TrimSpace(item.AuthIndex)
		if authIndex == "" {
			authIndex = strings.TrimSpace(item.AuthIndexCamel)
		}
		startAtMs := item.StartAtMs
		if startAtMs == 0 {
			startAtMs = item.StartAtMsCamel
		}
		endAtMs := item.EndAtMs
		if endAtMs == 0 {
			endAtMs = item.EndAtMsCamel
		}
		if startAtMs <= 0 || endAtMs <= startAtMs {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid window time range"})
			return
		}
		modelFilter := strings.TrimSpace(item.ModelFilter)
		if modelFilter == "" {
			modelFilter = strings.TrimSpace(item.ModelFilterAlt)
		}
		windows = append(windows, usage.PeriodSummaryWindow{
			ID:          item.ID,
			AuthIndex:   authIndex,
			StartAt:     time.UnixMilli(startAtMs).UTC(),
			EndAt:       time.UnixMilli(endAtMs).UTC(),
			ModelFilter: modelFilter,
		})
	}

	modelPrices := body.ModelPrices
	if len(modelPrices) == 0 && len(body.ModelPricesCamel) > 0 {
		modelPrices = body.ModelPricesCamel
	}
	if len(modelPrices) == 0 && h.cfg != nil {
		modelPrices = h.cfg.UsageModelPrices
	}
	modelPrices = sanitizeUsageModelPrices(modelPrices)

	items := h.usageStats.SummarizePeriods(windows, func(modelName string, detail usage.RequestDetail) float64 {
		return calculateUsageDetailCost(modelName, detail, modelPrices)
	})
	c.JSON(http.StatusOK, usagePeriodSummaryResponse{
		Items:       items,
		GeneratedAt: time.Now().UTC(),
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

const tokensPerPriceUnit = 1_000_000

var (
	usageModelDateSuffixRegex = regexp.MustCompile(`(?:-\d{8}|-\d{4}-\d{2}-\d{2})$`)
	usageOpenAIBaseRegex      = regexp.MustCompile(`^(gpt-\d+(?:\.\d+)?)(?:-|$)`)
)

func calculateUsageDetailCost(modelName string, detail usage.RequestDetail, modelPrices map[string]config.UsageModelPrice) float64 {
	priceKey, price, ok := resolveUsageModelPrice(modelName, modelPrices)
	if !ok {
		return 0
	}

	priceTier := config.UsageModelPriceTier{
		Prompt:        price.Prompt,
		Completion:    price.Completion,
		Cache:         price.Cache,
		CacheRead:     price.CacheRead,
		CacheCreation: price.CacheCreation,
	}
	if strings.EqualFold(strings.TrimSpace(detail.ServiceTier), "priority") && price.Priority != nil {
		priceTier = *price.Priority
	}

	tokens := detail.Tokens
	inputTokens := nonNegativeInt(tokens.InputTokens)
	completionTokens := nonNegativeInt(tokens.OutputTokens)
	cacheReadTokens := nonNegativeInt(tokens.CacheReadTokens)
	if cacheReadTokens == 0 {
		cacheReadTokens = nonNegativeInt(tokens.CachedTokens)
	}
	cacheCreationTokens := nonNegativeInt(tokens.CacheCreationTokens)

	promptTokens := inputTokens
	if !strings.HasPrefix(priceKey, "claude") {
		promptTokens = inputTokens - cacheReadTokens
		if promptTokens < 0 {
			promptTokens = 0
		}
	}

	promptCost := (float64(promptTokens) / tokensPerPriceUnit) * nonNegativeFloat(priceTier.Prompt)
	cacheReadCost := (float64(cacheReadTokens) / tokensPerPriceUnit) * getUsageCacheReadPrice(priceTier)
	cacheCreationCost := (float64(cacheCreationTokens) / tokensPerPriceUnit) * getUsageCacheCreationPrice(priceTier)
	completionCost := (float64(completionTokens) / tokensPerPriceUnit) * nonNegativeFloat(priceTier.Completion)
	total := promptCost + cacheReadCost + cacheCreationCost + completionCost
	if math.IsInf(total, 0) || math.IsNaN(total) || total <= 0 {
		return 0
	}
	return total
}

func resolveUsageModelPrice(modelName string, modelPrices map[string]config.UsageModelPrice) (string, config.UsageModelPrice, bool) {
	if strings.TrimSpace(modelName) == "" || len(modelPrices) == 0 {
		return "", config.UsageModelPrice{}, false
	}

	normalizedPrices := make(map[string]config.UsageModelPrice, len(modelPrices))
	for key, value := range modelPrices {
		normalizedKey := normalizeUsageModelLookupCandidate(key)
		if normalizedKey == "" {
			continue
		}
		normalizedPrices[normalizedKey] = value
	}
	if len(normalizedPrices) == 0 {
		return "", config.UsageModelPrice{}, false
	}

	candidates := buildUsageModelLookupCandidates(modelName)
	for _, candidate := range candidates {
		if price, ok := normalizedPrices[candidate]; ok {
			return candidate, price, true
		}
	}
	for _, candidate := range candidates {
		baseName := extractUsageBaseModelName(candidate)
		if price, ok := normalizedPrices[baseName]; ok {
			return baseName, price, true
		}
	}
	for _, candidate := range candidates {
		if match := matchUsageOpenAIModel(candidate, normalizedPrices); match != "" {
			return match, normalizedPrices[match], true
		}
	}
	return "", config.UsageModelPrice{}, false
}

func buildUsageModelLookupCandidates(modelName string) []string {
	normalized := normalizeUsageModelLookupCandidate(modelName)
	trimmed := strings.ToLower(strings.TrimSpace(modelName))
	candidates := []string{
		normalized,
		strings.ReplaceAll(strings.ReplaceAll(normalized, "-4-5", "-4.5"), "-4-6", "-4.6"),
		trimmed,
		strings.TrimPrefix(trimmed, "models/"),
	}

	seen := make(map[string]struct{}, len(candidates))
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		value := strings.TrimSpace(candidate)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeUsageModelLookupCandidate(modelName string) string {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	if normalized == "" {
		return ""
	}
	normalized = strings.NewReplacer(" ", "-", "_", "-").Replace(normalized)
	normalized = strings.TrimPrefix(normalized, "models/")
	normalized = strings.TrimPrefix(normalized, "publishers/google/models/")
	if index := strings.LastIndex(normalized, "/publishers/google/models/"); index != -1 {
		normalized = normalized[index+len("/publishers/google/models/"):]
	}
	if index := strings.LastIndex(normalized, "/models/"); index != -1 {
		normalized = normalized[index+len("/models/"):]
	}
	if index := strings.LastIndex(normalized, "/"); index != -1 {
		normalized = normalized[index+1:]
	}
	return normalized
}

func extractUsageBaseModelName(model string) string {
	parts := strings.Split(model, "-")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 8 && isDigits(part) {
			continue
		}
		if strings.Contains(part, ":") {
			continue
		}
		result = append(result, part)
	}
	return strings.Join(result, "-")
}

func matchUsageOpenAIModel(model string, prices map[string]config.UsageModelPrice) string {
	if !strings.Contains(model, "gpt-") && !strings.Contains(model, "chatgpt-") && !strings.Contains(model, "codex") {
		return ""
	}
	if strings.HasPrefix(model, "gpt-5.3-codex-spark") {
		if _, ok := prices["gpt-5.1-codex"]; ok {
			return "gpt-5.1-codex"
		}
	}

	for _, variant := range generateUsageOpenAIModelVariants(model) {
		if _, ok := prices[variant]; ok {
			return variant
		}
	}

	prefixMappings := [][2]string{
		{"gpt-5.5-pro", "gpt-5.5-pro"},
		{"gpt-5.5", "gpt-5.5"},
		{"gpt-5.4-mini", "gpt-5.4-mini"},
		{"gpt-5.4-nano", "gpt-5.4-nano"},
		{"gpt-5.4-pro", "gpt-5.4-pro"},
		{"gpt-5.4", "gpt-5.4"},
		{"gpt-5.3-chat", "gpt-5.3-chat-latest"},
		{"gpt-5.3-codex", "gpt-5.3-codex"},
		{"gpt-5.2-chat", "gpt-5.2-chat-latest"},
		{"gpt-5.2-codex", "gpt-5.2-codex"},
		{"gpt-5.2-pro", "gpt-5.2-pro"},
		{"gpt-5.2", "gpt-5.2"},
		{"gpt-5.1-codex-mini", "gpt-5.1-codex-mini"},
		{"gpt-5.1-codex-max", "gpt-5.1-codex-max"},
		{"gpt-5.1-codex", "gpt-5.1-codex"},
		{"gpt-5.1-chat", "gpt-5.1-chat-latest"},
		{"gpt-5.1", "gpt-5.1"},
		{"gpt-5-codex", "gpt-5-codex"},
		{"gpt-5-search", "gpt-5-search-api"},
		{"gpt-5-chat", "gpt-5-chat-latest"},
		{"gpt-5-mini", "gpt-5-mini"},
		{"gpt-5-nano", "gpt-5-nano"},
		{"gpt-5-pro", "gpt-5-pro"},
		{"gpt-5", "gpt-5"},
		{"codex-mini-latest", "codex-mini-latest"},
	}
	for _, mapping := range prefixMappings {
		if strings.HasPrefix(model, mapping[0]) {
			if _, ok := prices[mapping[1]]; ok {
				return mapping[1]
			}
		}
	}
	return ""
}

func generateUsageOpenAIModelVariants(model string) []string {
	var variants []string
	seen := map[string]struct{}{}
	add := func(value string) {
		if value == "" || value == model {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		variants = append(variants, value)
	}

	withoutDate := usageModelDateSuffixRegex.ReplaceAllString(model, "")
	if withoutDate != model {
		add(withoutDate)
	}
	if match := usageOpenAIBaseRegex.FindStringSubmatch(model); len(match) > 1 {
		add(match[1])
	}
	if withoutDate != model {
		if match := usageOpenAIBaseRegex.FindStringSubmatch(withoutDate); len(match) > 1 {
			add(match[1])
		}
	}
	return variants
}

func getUsageCacheReadPrice(price config.UsageModelPriceTier) float64 {
	value := price.CacheRead
	if value == 0 {
		value = price.Cache
	}
	if value >= 0 {
		return value
	}
	return nonNegativeFloat(price.Prompt)
}

func getUsageCacheCreationPrice(price config.UsageModelPriceTier) float64 {
	value := price.CacheCreation
	if value == 0 {
		value = price.CacheRead
	}
	if value == 0 {
		value = price.Cache
	}
	if value >= 0 {
		return value
	}
	return nonNegativeFloat(price.Prompt)
}

func nonNegativeInt(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func nonNegativeFloat(value float64) float64 {
	if value < 0 || math.IsInf(value, 0) || math.IsNaN(value) {
		return 0
	}
	return value
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
