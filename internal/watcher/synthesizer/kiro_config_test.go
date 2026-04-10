package synthesizer

import (
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestSynthesizeKiroKeys_IncludesThinkingConfigAttrs(t *testing.T) {
	forceThinking := true
	ctx := &SynthesisContext{
		Config: &config.Config{
			KiroKey: []config.KiroKey{
				{
					AccessToken:    "aoa-test-token",
					ThinkingBudget: 24000,
					ForceThinking:  &forceThinking,
				},
			},
		},
		Now:         time.Unix(123, 0),
		IDGenerator: NewStableIDGenerator(),
	}

	auths := NewConfigSynthesizer().synthesizeKiroKeys(ctx)
	if len(auths) != 1 {
		t.Fatalf("expected 1 auth entry, got %d", len(auths))
	}

	if got := auths[0].Attributes["thinking_budget"]; got != "24000" {
		t.Fatalf("expected thinking_budget attr 24000, got %q", got)
	}
	if got := auths[0].Attributes["force_thinking"]; got != "true" {
		t.Fatalf("expected force_thinking attr true, got %q", got)
	}
}
