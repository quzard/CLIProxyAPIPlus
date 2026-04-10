package claude

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildKiroPayload_UsesConfiguredThinkingDefaults(t *testing.T) {
	input := []byte(`{
		"model": "kiro-claude-sonnet-4-5",
		"max_tokens": 4096,
		"messages": [
			{"role": "user", "content": "hello"}
		]
	}`)

	result, thinkingEnabled := BuildKiroPayload(input, "kiro-model", "", "CLI", false, false, nil, map[string]any{
		"force_thinking":  "true",
		"thinking_budget": "24000",
	})
	if !thinkingEnabled {
		t.Fatal("expected thinking to be enabled from metadata")
	}

	var payload KiroPayload
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	content := payload.ConversationState.CurrentMessage.UserInputMessage.Content
	if !strings.Contains(content, "<thinking_mode>enabled</thinking_mode>") {
		t.Fatalf("expected thinking mode tag in content, got %q", content)
	}
	if !strings.Contains(content, "<max_thinking_length>24000</max_thinking_length>") {
		t.Fatalf("expected configured thinking budget in content, got %q", content)
	}
}
