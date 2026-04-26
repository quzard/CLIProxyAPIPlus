package usage

import (
	"fmt"
	"strings"
	"time"
)

// PeriodSummaryWindow describes a usage aggregation window.
type PeriodSummaryWindow struct {
	ID          string
	AuthIndex   string
	StartAt     time.Time
	EndAt       time.Time
	ModelFilter string
}

// PeriodSummaryCostFunc calculates display cost for a request detail.
type PeriodSummaryCostFunc func(modelName string, detail RequestDetail) float64

// PeriodSummary contains aggregated request usage for a window.
type PeriodSummary struct {
	ID                  string  `json:"id"`
	Requests            int64   `json:"requests"`
	Tokens              int64   `json:"tokens"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	ReasoningTokens     int64   `json:"reasoning_tokens"`
	CachedTokens        int64   `json:"cached_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	Cost                float64 `json:"cost"`
}

type normalizedPeriodWindow struct {
	PeriodSummaryWindow
	modelFilter string
}

// SummarizePeriods aggregates request statistics for the supplied windows without
// copying the full detail snapshot to callers.
func (s *RequestStatistics) SummarizePeriods(windows []PeriodSummaryWindow, costFn PeriodSummaryCostFunc) []PeriodSummary {
	results := make([]PeriodSummary, len(windows))
	if len(windows) == 0 || s == nil {
		return results
	}

	normalizedWindows := make([]normalizedPeriodWindow, 0, len(windows))
	for i, window := range windows {
		window.ID = strings.TrimSpace(window.ID)
		if window.ID == "" {
			window.ID = fmt.Sprintf("%d", i)
		}
		window.AuthIndex = strings.TrimSpace(window.AuthIndex)
		window.ModelFilter = strings.TrimSpace(window.ModelFilter)
		results[i].ID = window.ID
		if window.StartAt.IsZero() || window.EndAt.IsZero() || !window.EndAt.After(window.StartAt) {
			continue
		}
		normalizedWindows = append(normalizedWindows, normalizedPeriodWindow{
			PeriodSummaryWindow: window,
			modelFilter:         normalizePeriodModelName(window.ModelFilter),
		})
	}
	if len(normalizedWindows) == 0 {
		return results
	}

	resultIndexes := make(map[string]int, len(results))
	for i := range results {
		resultIndexes[results[i].ID] = i
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, apiStatsValue := range s.apis {
		if apiStatsValue == nil {
			continue
		}
		for modelName, modelStatsValue := range apiStatsValue.Models {
			if modelStatsValue == nil {
				continue
			}
			normalizedModelName := normalizePeriodModelName(modelName)
			for _, detail := range modelStatsValue.Details {
				detailAuthIndex := strings.TrimSpace(detail.AuthIndex)
				detailTimestamp := detail.Timestamp
				if detailTimestamp.IsZero() {
					continue
				}

				for _, window := range normalizedWindows {
					if window.AuthIndex != "" && detailAuthIndex != window.AuthIndex {
						continue
					}
					if detailTimestamp.Before(window.StartAt) || !detailTimestamp.Before(window.EndAt) {
						continue
					}
					if window.modelFilter != "" && !periodModelMatches(normalizedModelName, window.modelFilter) {
						continue
					}

					resultIndex, ok := resultIndexes[window.ID]
					if !ok {
						continue
					}
					addPeriodDetail(&results[resultIndex], modelName, detail, costFn)
				}
			}
		}
	}

	return results
}

func addPeriodDetail(summary *PeriodSummary, modelName string, detail RequestDetail, costFn PeriodSummaryCostFunc) {
	if summary == nil {
		return
	}
	tokens := normaliseTokenStats(detail.Tokens)
	summary.Requests++
	summary.Tokens += tokens.TotalTokens
	summary.InputTokens += tokens.InputTokens
	summary.OutputTokens += tokens.OutputTokens
	summary.ReasoningTokens += tokens.ReasoningTokens
	summary.CachedTokens += tokens.CachedTokens
	summary.CacheReadTokens += tokens.CacheReadTokens
	summary.CacheCreationTokens += tokens.CacheCreationTokens
	if costFn != nil {
		cost := costFn(modelName, detail)
		if cost > 0 {
			summary.Cost += cost
		}
	}
}

func normalizePeriodModelName(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return ""
	}
	replacer := strings.NewReplacer("_", "-", " ", "-", "/", "-")
	normalized = replacer.Replace(normalized)
	normalized = strings.Trim(normalized, "-")
	for strings.Contains(normalized, "--") {
		normalized = strings.ReplaceAll(normalized, "--", "-")
	}
	return normalized
}

func periodModelMatches(modelName, filter string) bool {
	if modelName == "" || filter == "" {
		return false
	}
	return strings.Contains(modelName, filter) || strings.Contains(filter, modelName)
}
