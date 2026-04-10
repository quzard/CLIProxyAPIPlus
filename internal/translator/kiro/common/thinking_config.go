package common

import (
	"strconv"
	"strings"
)

const (
	DefaultThinkingBudget = 16000
	MaxThinkingBudget     = 32000
)

type ThinkingConfig struct {
	ForceEnabled bool
	Budget       int
}

func ParseThinkingConfigMetadata(metadata map[string]any) ThinkingConfig {
	var cfg ThinkingConfig
	if len(metadata) == 0 {
		return cfg
	}

	cfg.ForceEnabled = parseMetadataBool(metadata["force_thinking"])
	cfg.Budget = clampThinkingBudget(parseMetadataInt(metadata["thinking_budget"]))
	return cfg
}

func clampThinkingBudget(budget int) int {
	if budget <= 0 {
		return 0
	}
	if budget > MaxThinkingBudget {
		return MaxThinkingBudget
	}
	return budget
}

func parseMetadataBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		return err == nil && parsed
	default:
		return false
	}
}

func parseMetadataInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}
