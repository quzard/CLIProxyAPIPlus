package helps

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/tidwall/gjson"
)

func TestApplyPayloadConfigWithRoot_OverrideMatchesRequestedModelWildcardVariants(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Payload: config.PayloadConfig{
			Override: []config.PayloadRule{
				{
					Models: []config.PayloadModelRule{
						{Name: "gpt-5.4*"},
					},
					Params: map[string]any{
						"service_tier": "priority",
					},
				},
			},
		},
	}

	payload := []byte(`{"input":"hello"}`)
	got := ApplyPayloadConfigWithRoot(cfg, "provider-model", "openai", "", payload, payload, "gpt-5.4(high)")

	if value := gjson.GetBytes(got, "service_tier").String(); value != "priority" {
		t.Fatalf("service_tier = %q, want %q; payload=%s", value, "priority", string(got))
	}
}
